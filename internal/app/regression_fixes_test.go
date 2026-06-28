package app

// regression_fixes_test.go locks in a batch of shell-level bug fixes so they
// cannot silently regress. Each Test below would fail against the pre-fix code:
//
//   - TestMutationUpdateClearingAssigneeUnassigns (unassign diff)
//   - TestMutationCreatePriorityRangeEnforced / TestMutationUpdatePriorityRangeEnforced (0..4 range)
//   - TestModeCyclePrevFromBoardKeepsSelectionAndAllowsEscape (lastBrowse invariant)
//   - TestEditPreparedBuildEditorCmdErrorRemovesTempFile (orphaned temp cleanup)
//   - TestToastStaleDismissDoesNotHideNewerToast (dismiss identity)

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// recordingUpdateRepository wraps a repository.Repository and captures the exact
// UpdateIssueInput passed to UpdateIssue. The package's standard test repo
// (ErrorInjectingRepository) records only method names, so this thin wrapper is
// needed to assert on the pointer-valued fields the shell builds — specifically
// the Assignee unassign semantics.
type recordingUpdateRepository struct {
	repository.Repository
	updateCalls []domain.UpdateIssueInput
}

func (r *recordingUpdateRepository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	r.updateCalls = append(r.updateCalls, input)
	return r.Repository.UpdateIssue(ctx, id, input)
}

// TestMutationUpdateClearingAssigneeUnassigns is the regression guard for the
// mutationUpdate assignee diff. Clearing a pre-filled assignee must send an
// Assignee pointer to "" (unassign) rather than being silently dropped, while
// leaving the assignee unchanged must send a nil Assignee (no change).
//
// Pre-fix the branch gated on `assignee != ""`, so:
//   - cleared field → Assignee left nil (the unassign was lost), and
//   - unchanged "alice" → Assignee pointer to "alice" (a redundant write).
//
// Both sub-cases below fail against that old behaviour.
func TestMutationUpdateClearingAssigneeUnassigns(t *testing.T) {
	t.Parallel()

	state := mutationDialogState{
		kind:  mutationUpdate,
		issue: domain.IssueSummary{ID: "tm-1", Title: "Task", Status: "open", Type: "task", Assignee: "alice"},
	}

	newServices := func(t *testing.T) (*recordingUpdateRepository, Services) {
		t.Helper()
		gw := newTestRepository()
		gw.seedIssueSummary(domain.IssueSummary{ID: "tm-1", Title: "Task", Status: "open", Type: "task", Assignee: "alice"})
		rec := &recordingUpdateRepository{Repository: gw}
		services, err := NewServices(rec, config.Default(), t.TempDir())
		if err != nil {
			t.Fatalf("NewServices: %v", err)
		}
		return rec, services
	}

	t.Run("cleared_field_sends_unassign", func(t *testing.T) {
		t.Parallel()
		rec, services := newServices(t)
		values := map[string]string{
			"title":    "Task",
			"priority": "1",
			"assignee": "", // pre-filled "alice" cleared by the user
			"labels":   "",
		}

		if res, ok := submitMutationCmd(services, state, values)().(mutationResultMsg); ok && res.err != nil {
			t.Fatalf("unexpected mutation error: %v", res.err)
		}
		if len(rec.updateCalls) != 1 {
			t.Fatalf("expected exactly one UpdateIssue call, got %d", len(rec.updateCalls))
		}
		got := rec.updateCalls[0].Assignee
		if got == nil {
			t.Fatal("expected non-nil Assignee pointer (unassign) after clearing a pre-filled assignee; got nil (regression: gated on non-empty)")
		}
		if *got != "" {
			t.Fatalf("expected Assignee to point to empty string (unassign), got %q", *got)
		}
	})

	t.Run("unchanged_field_leaves_assignee_nil", func(t *testing.T) {
		t.Parallel()
		rec, services := newServices(t)
		values := map[string]string{
			"title":    "Task",
			"priority": "1",
			"assignee": "alice", // unchanged from the issue's current assignee
			"labels":   "",
		}

		if res, ok := submitMutationCmd(services, state, values)().(mutationResultMsg); ok && res.err != nil {
			t.Fatalf("unexpected mutation error: %v", res.err)
		}
		if len(rec.updateCalls) != 1 {
			t.Fatalf("expected exactly one UpdateIssue call, got %d", len(rec.updateCalls))
		}
		if got := rec.updateCalls[0].Assignee; got != nil {
			t.Fatalf("expected nil Assignee when value unchanged, got pointer to %q", *got)
		}
	})
}

// TestMutationCreatePriorityRangeEnforced is the regression guard for parsePriority
// (create path). Priorities outside 0..4 must be rejected with a message that
// mentions the range, and CreateIssue must not be called. Pre-fix any integer was
// accepted, so "5"/"9" would have reached the repository.
func TestMutationCreatePriorityRangeEnforced(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		priority string
		accepted bool
	}{
		{"boundary_4_accepted", "4", true},
		{"boundary_5_rejected", "5", false},
		{"far_out_of_range_9_rejected", "9", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gw := newTestRepository()
			services, err := NewServices(gw, config.Default(), t.TempDir())
			if err != nil {
				t.Fatalf("NewServices: %v", err)
			}

			state := mutationDialogState{kind: mutationCreate}
			values := map[string]string{"title": "New issue", "type": "task", "priority": tc.priority}

			msg := submitMutationCmd(services, state, values)()
			res, ok := msg.(mutationResultMsg)
			if !ok {
				t.Fatalf("expected mutationResultMsg, got %T", msg)
			}

			if tc.accepted {
				if res.err != nil {
					t.Fatalf("expected priority %q to be accepted, got error %v", tc.priority, res.err)
				}
				if !gw.hasCreateIssueCall() {
					t.Fatalf("expected CreateIssue to be called for accepted priority %q", tc.priority)
				}
				return
			}

			if res.err == nil {
				t.Fatalf("expected error rejecting out-of-range priority %q", tc.priority)
			}
			if !strings.Contains(res.err.Error(), "between 0 and 4") {
				t.Fatalf("expected error to mention range 'between 0 and 4', got %q", res.err.Error())
			}
			if gw.hasCreateIssueCall() {
				t.Fatalf("expected CreateIssue NOT called for out-of-range priority %q, calls=%#v", tc.priority, gw.Calls())
			}
		})
	}
}

// TestMutationUpdatePriorityRangeEnforced is the regression guard for
// parseRequiredPriority (update/priority dialog path). Priorities outside 0..4
// must be rejected with a range message and must not reach UpdateIssue.
func TestMutationUpdatePriorityRangeEnforced(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		priority string
		accepted bool
	}{
		{"boundary_4_accepted", "4", true},
		{"boundary_5_rejected", "5", false},
		{"far_out_of_range_9_rejected", "9", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gw := newTestRepository()
			gw.seedIssueSummary(domain.IssueSummary{ID: "tm-1", Title: "Task", Status: "open", Type: "task"})
			services, err := NewServices(gw, config.Default(), t.TempDir())
			if err != nil {
				t.Fatalf("NewServices: %v", err)
			}

			state := mutationDialogState{
				kind:  mutationUpdate,
				issue: domain.IssueSummary{ID: "tm-1", Title: "Task", Status: "open", Type: "task"},
			}
			values := map[string]string{"priority": tc.priority, "labels": ""}

			msg := submitMutationCmd(services, state, values)()
			res, ok := msg.(mutationResultMsg)
			if !ok {
				t.Fatalf("expected mutationResultMsg, got %T", msg)
			}

			if tc.accepted {
				if res.err != nil {
					t.Fatalf("expected priority %q to be accepted, got error %v", tc.priority, res.err)
				}
				if !gw.hasUpdateIssueCall() {
					t.Fatalf("expected UpdateIssue to be called for accepted priority %q", tc.priority)
				}
				return
			}

			if res.err == nil {
				t.Fatalf("expected error rejecting out-of-range priority %q", tc.priority)
			}
			if !strings.Contains(res.err.Error(), "between 0 and 4") {
				t.Fatalf("expected error to mention range 'between 0 and 4', got %q", res.err.Error())
			}
			if gw.hasUpdateIssueCall() {
				t.Fatalf("expected UpdateIssue NOT called for out-of-range priority %q, calls=%#v", tc.priority, gw.Calls())
			}
		})
	}
}

// TestModeCyclePrevFromBoardKeepsSelectionAndAllowsEscape is the regression guard
// for applyModeCycle. prevMode(Board) == Detail, and cycling into Detail must
// keep lastBrowse a browse mode (Board) so that the selection is preserved and
// Escape can return to Board.
//
// Pre-fix cycling did `lastBrowse = active` unconditionally, leaving
// lastBrowse == Detail. That made currentSelection() return nil (blank Detail)
// and turned Escape (active = lastBrowse) into a no-op (stuck in Detail).
func TestModeCyclePrevFromBoardKeepsSelectionAndAllowsEscape(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	if m.active != mode.Board {
		t.Fatalf("expected board active after init, got %s", m.active)
	}
	if got := firstSelectionID(m, mode.Board); got != "tm-1" {
		t.Fatalf("expected board selection tm-1 after init, got %q", got)
	}

	// ModeCyclePrev (ctrl+pgup): prevMode(Board) == Detail.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlPgUp})
	m = next.(Model)
	if m.active != mode.Detail {
		t.Fatalf("expected ModeCyclePrev from Board to enter Detail, got %s", m.active)
	}
	sel := m.currentSelection()
	if sel == nil || sel.Issue.ID != "tm-1" {
		t.Fatalf("expected selection preserved as tm-1 after cycling into Detail, got %#v (regression: lastBrowse clobbered to Detail)", sel)
	}

	// Drain the detail load so Detail reflects the preserved selection.
	m = applyMessages(t, m, runBatch(cmd))
	if m.detail.TargetID != "tm-1" && m.detail.Detail.Summary.ID != "tm-1" {
		t.Fatalf("expected Detail to track tm-1 selection, target=%q detail=%q", m.detail.TargetID, m.detail.Detail.Summary.ID)
	}

	// Escape must return to Board, not stay stuck in Detail.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Board {
		t.Fatalf("expected Escape from cycled Detail to return to Board, got %s", m.active)
	}
}

// buildEditorErrEditor extends FakeEditor so BuildEditorCmd returns an error,
// exercising the editIssuePreparedMsg failure path. PrepareDocument/ApplyEdits
// are inherited from the embedded *fakes.FakeEditor (unused in this flow).
type buildEditorErrEditor struct {
	*fakes.FakeEditor
	buildErr error
}

func (e buildEditorErrEditor) BuildEditorCmd(string) (*exec.Cmd, error) {
	return nil, e.buildErr
}

// TestEditPreparedBuildEditorCmdErrorRemovesTempFile is the regression guard for
// the editIssuePreparedMsg branch: when BuildEditorCmd fails, the temp document
// PrepareDocument already wrote must be removed so it does not leak on disk until
// the stale-temp sweep. Pre-fix this path returned without os.Remove, orphaning
// the file.
func TestEditPreparedBuildEditorCmdErrorRemovesTempFile(t *testing.T) {
	gw := newTestRepository()
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	services.Editor = buildEditorErrEditor{
		FakeEditor: &fakes.FakeEditor{},
		buildErr:   errors.New("no editor configured"),
	}

	m := mustNewModel(t, services)

	tmpPath := filepath.Join(t.TempDir(), "edit-doc.md")
	if err := os.WriteFile(tmpPath, []byte("# title\n\nbody\n"), 0o600); err != nil {
		t.Fatalf("write temp edit doc: %v", err)
	}

	next, _ := m.Update(editIssuePreparedMsg{
		issueID:  "tm-1",
		prepared: launchereditor.Prepared{IssueID: "tm-1", TempPath: tmpPath},
	})
	m = next.(Model)

	if _, statErr := os.Stat(tmpPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected orphaned temp file %q to be removed on BuildEditorCmd error, stat err=%v", tmpPath, statErr)
	}
	if !m.toast.Visible() {
		t.Fatalf("expected an error toast after BuildEditorCmd failure; toast=%+v", m.toast)
	}
	if view := m.toast.View(); !strings.Contains(view, "Failed to build editor command") {
		t.Fatalf("expected toast to mention 'Failed to build editor command', got: %q", view)
	}
}

// TestToastStaleDismissDoesNotHideNewerToast is the regression guard for the
// toaster.DismissMsg handler: it must hide the toast only when msg.Seq matches
// the currently shown toast. A stale timer from a superseded toast must be
// ignored. Pre-fix the handler hid unconditionally, so a stale dismiss would
// hide the newer toast early.
func TestToastStaleDismissDoesNotHideNewerToast(t *testing.T) {
	gw := newTestRepository()
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)

	// Show a first toast, capture its identity, then a second toast that
	// supersedes it (seq advances).
	m.showToast("first toast", toaster.StyleInfo)
	firstSeq := m.toast.Seq()
	m.showToast("second toast", toaster.StyleInfo)
	currentSeq := m.toast.Seq()
	if currentSeq == firstSeq {
		t.Fatalf("expected toast seq to advance between Show calls, got %d twice", currentSeq)
	}

	// A stale dismiss from the first toast must NOT hide the current one.
	next, _ := m.Update(toaster.DismissMsg{Seq: firstSeq})
	m = next.(Model)
	if !m.toast.Visible() {
		t.Fatal("expected current toast to remain visible after a stale DismissMsg (regression: dismissed unconditionally)")
	}

	// The dismiss matching the current toast must hide it.
	next, _ = m.Update(toaster.DismissMsg{Seq: currentSeq})
	m = next.(Model)
	if m.toast.Visible() {
		t.Fatal("expected current toast to be hidden after a matching DismissMsg")
	}
}

// TestShowToastSchedulesDismissWithCurrentSeq pins the showToast→scheduler
// wiring: the auto-dismiss timer must be scheduled with the CURRENT toast
// identity (m.toast.Seq() after Show) and the standard 3s duration. If showToast
// passed a stale or wrong seq, the DismissMsg.Seq guard could never match and
// the toast would never auto-dismiss. The default test stub ignores its args,
// so this installs a capturing scheduler to assert the values.
func TestShowToastSchedulesDismissWithCurrentSeq(t *testing.T) {
	gw := newTestRepository()
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)

	var gotDur time.Duration
	var gotSeq int
	var called bool
	m.scheduleToastDismiss = func(d time.Duration, seq int) tea.Cmd {
		gotDur, gotSeq, called = d, seq, true
		return nil
	}

	m.showToast("hello", toaster.StyleInfo)

	if !called {
		t.Fatal("showToast did not invoke the dismiss scheduler")
	}
	if gotSeq != m.toast.Seq() {
		t.Errorf("scheduled dismiss seq = %d, want current toast seq %d", gotSeq, m.toast.Seq())
	}
	if gotDur != 3*time.Second {
		t.Errorf("scheduled dismiss duration = %v, want 3s", gotDur)
	}
}
