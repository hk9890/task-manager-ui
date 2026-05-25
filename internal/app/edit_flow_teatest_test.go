package app

// Regression guard for the tea.Exec-based 'e' (edit issue) flow introduced in
// beads-workbench-h3ql. These tests drive the real Bubble Tea program loop via
// teatest so that the goroutine-vs-tea.Exec bug cannot reappear undetected.
//
// Flow under test:
//
//	'e' keypress
//	  → prepareEditCmd (goroutine) → editIssuePreparedMsg
//	  → tea.Exec(fakeExecCmd) → FakeExecCommand.Run() (writes edited content)
//	  → editorExitedMsg
//	  → applyEditsCmd (goroutine) → editIssueResultMsg
//	  → toast + optional detail reload
//
// FakeExecCommand intercepts only the subprocess launch; PrepareDocument and
// ApplyEdits use the real IssueEditor wired to the memory repository so the
// full filesystem and repository round-trip is exercised.

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
)

// editFlowTimeout is the per-assertion budget for WaitFor in the edit flow
// tests. 3 seconds is generous enough to accommodate scheduler variance
// without making the suite slow on a healthy machine.
const editFlowTimeout = 5 * time.Second

// buildEditFlowServices creates a Services value suitable for the teatest edit
// flow tests. It wires the real IssueEditor against the provided repository and
// injects the FakeExecCommand factory as the ExecCommandFactory seam.
//
// The returned *fakes.FakeExecCommand lets callers configure EditedContent /
// RunErr and inspect RunCalled after the program settles.
func buildEditFlowServices(
	t *testing.T,
	gw *appTestRepository,
	fakeCmd *fakes.FakeExecCommand,
) Services {
	t.Helper()

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	services.ExecCommandFactory = fakeCmd.Factory()
	return services
}

// seedEditIssue seeds an issue into the repository and returns it. It also
// pre-seeds the board state so the initial board load succeeds.
func seedEditIssue(t *testing.T, gw *appTestRepository, issue domain.IssueDetail) {
	t.Helper()
	gw.seedIssueDetail(issue)
	gw.seedReady(issue.Summary.ID, issue.Summary.Title, issue.Summary.Type, issue.Summary.Priority)
}

// editableDocWithTitle builds a minimal but syntactically valid edit document
// that ParseIssueEditDocument can parse. The title is replaced by newTitle;
// status and type match the provided issue so that only the title differs.
func editableDocWithTitle(issue domain.IssueDetail, newTitle string) string {
	original := domain.RenderIssueEditDocument(issue)
	// Replace the title content between TITLE markers.
	begin := "<!-- BWB:FIELD:TITLE:BEGIN -->"
	end := "<!-- BWB:FIELD:TITLE:END -->"
	startIdx := strings.Index(original, begin)
	endIdx := strings.Index(original, end)
	if startIdx < 0 || endIdx < 0 {
		panic("editableDocWithTitle: could not find TITLE markers in rendered document")
	}
	afterBegin := startIdx + len(begin)
	return original[:afterBegin] + "\n" + newTitle + "\n" + original[endIdx:]
}

// TestEditFlowSuccessPathTeatest drives the full 'e' flow through the real
// Bubble Tea runtime. Verifies that:
//   - FakeExecCommand.Run is called exactly once
//   - UpdateIssue is recorded on the repository
//   - the "Updated issue <id>" success toast is set on the settled model
//
// Assertion strategy: we gate on three observable signals: FakeExecCommand.RunCount
// (subprocess invoked), repository HasCall (ApplyEdits produced a write), and the
// Services.OnEditIssueResult hook (editIssueResultMsg processed, toast set). The
// hook fires synchronously from the BubbleTea Update handler after showToast, so
// it is a precise zero-sleep signal. Final assertion uses FinalModel — post-tea.Exec
// View() frames do not reliably reach the output pipe under CI load.
func TestEditFlowSuccessPathTeatest(t *testing.T) {
	// Not parallel — modifies global scheduler vars via TestMain.
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })
	withSpinnerTickScheduler(t, func() tea.Cmd { return nil })
	withToastDismissScheduler(t, func(_ time.Duration) tea.Cmd { return nil })

	const issueID = "bw-edit-1"
	originalTitle := "Original Title"
	editedTitle := "Edited Title"

	issue := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       issueID,
			Title:    originalTitle,
			Status:   "open",
			Type:     "task",
			Priority: 1,
		},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}

	gw := newTestRepository()
	seedEditIssue(t, gw, issue)

	// Pre-configure the fake ExecCommand with content that changes the title.
	fakeCmd := &fakes.FakeExecCommand{
		EditedContent: editableDocWithTitle(issue, editedTitle),
	}

	services := buildEditFlowServices(t, gw, fakeCmd)

	// Wire the test-only hook so we get a precise signal when editIssueResultMsg
	// has been fully processed and the toast set — eliminating the time.Sleep.
	var editResultCount atomic.Int32
	services.OnEditIssueResult = func() { editResultCount.Add(1) }

	model, err := NewModel(services)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	// Set sizeKnown so View() renders content (not the empty pre-size guard).
	model.sizeKnown = true

	tm := testui.NewTestModelWithSize(t, model, 120, 34)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 34})

	// Drain the board init so there is a selected issue. This runs before any
	// tea.Exec, so the renderer is in steady state and output-buffer scanning
	// is reliable here.
	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), editFlowTimeout, originalTitle)

	// Press 'e' to trigger the edit flow.
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	// Gate 1: tea.Exec dispatched and FakeExecCommand.Run ran.
	testui.WaitForConditionWithTimeout(t, editFlowTimeout, func() bool {
		return fakeCmd.RunCount() >= 1
	})
	// Gate 2: applyEditsCmd's goroutine reached the repository. This proves the
	// editor's ApplyEdits path produced a real update (success path), and is
	// the last observable side-effect before editIssueResultMsg is returned
	// from the closure to the BubbleTea msg loop.
	testui.WaitForConditionWithTimeout(t, editFlowTimeout, func() bool {
		return gw.hasUpdateIssueCall()
	})
	// Gate 3: editIssueResultMsg was processed by Update and the toast set.
	// The OnEditIssueResult hook fires synchronously after showToast, so this
	// is a precise signal — no sleep needed.
	testui.WaitForConditionWithTimeout(t, editFlowTimeout, func() bool {
		return editResultCount.Load() >= 1
	})

	if err := tm.Quit(); err != nil {
		t.Fatalf("teatest Quit: %v", err)
	}

	// Assert on the settled model state via FinalModel — renderer-independent.
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(editFlowTimeout))
	m, ok := finalModel.(Model)
	if !ok {
		t.Fatalf("expected final model of type Model, got %T", finalModel)
	}
	if !m.toast.Visible() {
		t.Errorf("expected success toast to be visible after successful edit; toast=%+v", m.toast)
	}
	wantToast := fmt.Sprintf("Updated issue %s", issueID)
	if view := m.toast.View(); !strings.Contains(view, wantToast) {
		t.Errorf("expected toast to contain %q, got: %q", wantToast, view)
	}

	if n := fakeCmd.RunCount(); n != 1 {
		t.Errorf("expected FakeExecCommand.Run called once, got %d", n)
	}
	if !gw.hasUpdateIssueCall() {
		t.Errorf("expected UpdateIssue call on repository after successful edit, calls=%#v", gw.Calls())
	}
}

// TestEditFlowNoChangeTeatest verifies the no-change path: when the editor
// writes back an identical document, no UpdateIssue is called and the
// "No changes saved" toast is set on the settled model.
//
// Assertion strategy: we gate on FakeExecCommand.RunCount (subprocess invoked)
// and the Services.OnEditIssueResult hook (editIssueResultMsg processed, toast
// set). Gating on repository.HasCall(UpdateIssue) is not available here (we are
// asserting the opposite). Final assertion uses FinalModel — output-buffer scanning
// is not reliable for post-tea.Exec frames under CI load.
func TestEditFlowNoChangeTeatest(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })
	withSpinnerTickScheduler(t, func() tea.Cmd { return nil })
	withToastDismissScheduler(t, func(_ time.Duration) tea.Cmd { return nil })

	const issueID = "bw-edit-2"

	issue := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       issueID,
			Title:    "Unchanged Title",
			Status:   "open",
			Type:     "task",
			Priority: 1,
		},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}

	gw := newTestRepository()
	seedEditIssue(t, gw, issue)

	// EditedContent is the exact same rendered document — no changes.
	fakeCmd := &fakes.FakeExecCommand{
		EditedContent: domain.RenderIssueEditDocument(issue),
	}

	services := buildEditFlowServices(t, gw, fakeCmd)

	// Wire the test-only hook so we get a precise signal when editIssueResultMsg
	// has been fully processed and the toast set — eliminating the time.Sleep.
	var editResultCount atomic.Int32
	services.OnEditIssueResult = func() { editResultCount.Add(1) }

	model, err := NewModel(services)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	model.sizeKnown = true

	tm := testui.NewTestModelWithSize(t, model, 120, 34)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 34})

	// Drain the board init so a selection exists.
	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), editFlowTimeout, "Unchanged Title")

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	// Gate 1: tea.Exec dispatched and FakeExecCommand.Run ran.
	testui.WaitForConditionWithTimeout(t, editFlowTimeout, func() bool {
		return fakeCmd.RunCount() >= 1
	})
	// Gate 2: editIssueResultMsg was processed by Update and the toast set.
	// No further observable side-effect exists for the no-change path (the
	// test asserts UpdateIssue is NOT called); the hook is the precise signal.
	testui.WaitForConditionWithTimeout(t, editFlowTimeout, func() bool {
		return editResultCount.Load() >= 1
	})

	if err := tm.Quit(); err != nil {
		t.Fatalf("teatest Quit: %v", err)
	}

	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(editFlowTimeout))
	m, ok := finalModel.(Model)
	if !ok {
		t.Fatalf("expected final model of type Model, got %T", finalModel)
	}
	if !m.toast.Visible() {
		t.Errorf("expected info toast to be visible after no-change edit; toast=%+v", m.toast)
	}
	wantToast := fmt.Sprintf("No changes saved for issue %s", issueID)
	if view := m.toast.View(); !strings.Contains(view, wantToast) {
		t.Errorf("expected toast to contain %q, got: %q", wantToast, view)
	}

	if n := fakeCmd.RunCount(); n != 1 {
		t.Errorf("expected FakeExecCommand.Run called once, got %d", n)
	}
	if gw.hasUpdateIssueCall() {
		t.Errorf("expected no UpdateIssue call when document is unchanged, calls=%#v", gw.Calls())
	}
}

// TestEditFlowEditorErrorTeatest verifies the editor-error path: when the
// FakeExecCommand.Run returns an error, the "Failed to edit issue" toast is
// shown and UpdateIssue is NOT called.
//
// Assertion strategy: we gate on FakeExecCommand.RunCount (subprocess invoked)
// and the Services.OnEditIssueResult hook (editIssueResultMsg processed, toast
// set). The hook fires synchronously from Update after showToast — no sleep
// needed. Final assertion uses FinalModel — post-tea.Exec View() frames do not
// reliably reach the output pipe under CI load.
func TestEditFlowEditorErrorTeatest(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })
	withSpinnerTickScheduler(t, func() tea.Cmd { return nil })
	withToastDismissScheduler(t, func(_ time.Duration) tea.Cmd { return nil })

	const issueID = "bw-edit-3"

	issue := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       issueID,
			Title:    "Error Test Title",
			Status:   "open",
			Type:     "task",
			Priority: 1,
		},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}

	gw := newTestRepository()
	seedEditIssue(t, gw, issue)

	// RunErr simulates the editor exiting with a non-zero status.
	fakeCmd := &fakes.FakeExecCommand{
		RunErr: errFakeEditorFailed,
	}

	services := buildEditFlowServices(t, gw, fakeCmd)

	// Wire the test-only hook so we get a precise signal when editIssueResultMsg
	// has been fully processed and the toast set — eliminating the time.Sleep.
	var editResultCount atomic.Int32
	services.OnEditIssueResult = func() { editResultCount.Add(1) }

	model, err := NewModel(services)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	model.sizeKnown = true

	tm := testui.NewTestModelWithSize(t, model, 120, 34)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 34})

	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), editFlowTimeout, "Error Test Title")

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	// Gate 1: tea.Exec dispatched and FakeExecCommand.Run ran.
	testui.WaitForConditionWithTimeout(t, editFlowTimeout, func() bool {
		return fakeCmd.RunCount() >= 1
	})
	// Gate 2: editIssueResultMsg was processed by Update and the toast set.
	// The OnEditIssueResult hook fires synchronously after showToast, giving a
	// precise signal without sleeping.
	testui.WaitForConditionWithTimeout(t, editFlowTimeout, func() bool {
		return editResultCount.Load() >= 1
	})

	if err := tm.Quit(); err != nil {
		t.Fatalf("teatest Quit: %v", err)
	}

	// Assert on the settled model state via FinalModel rather than output
	// buffer content, which may not have been flushed for a single-frame toast.
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(editFlowTimeout))
	m, ok := finalModel.(Model)
	if !ok {
		t.Fatalf("expected final model of type Model, got %T", finalModel)
	}
	if !m.toast.Visible() {
		t.Errorf("expected error toast to be visible after editor-error path; toast=%+v", m.toast)
	}
	if view := m.toast.View(); !strings.Contains(view, "Failed to edit issue") {
		t.Errorf("expected toast to contain 'Failed to edit issue', got: %q", view)
	}

	if n := fakeCmd.RunCount(); n != 1 {
		t.Errorf("expected FakeExecCommand.Run called once, got %d", n)
	}
	if gw.hasUpdateIssueCall() {
		t.Errorf("expected no UpdateIssue call when editor returns error, calls=%#v", gw.Calls())
	}
}

// errFakeEditorFailed is the sentinel error returned by FakeExecCommand in the
// editor-error path test.
var errFakeEditorFailed = &fakeEditorError{"fake editor: exit status 1"}

type fakeEditorError struct{ msg string }

func (e *fakeEditorError) Error() string { return e.msg }

// Verify the test-local teatest helpers compile. teatest.WaitFor is imported
// transitively through testui.WaitForOutputContainsAllWithTimeout.
var _ = teatest.WaitFor
