package app

// Regression guard for the tea.Exec-based 'e' (edit issue) flow. These tests
// drive the real Bubble Tea program loop via
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
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
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
	begin := "<!-- TASKMGRUI:FIELD:TITLE:BEGIN -->"
	end := "<!-- TASKMGRUI:FIELD:TITLE:END -->"
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
// onEditIssueResult hook (editIssueResultMsg processed, toast set). The
// hook fires synchronously from the BubbleTea Update handler after showToast, so
// it is a precise zero-sleep signal. Final assertion uses FinalModel — post-tea.Exec
// View() frames do not reliably reach the output pipe under CI load.
func TestEditFlowSuccessPathTeatest(t *testing.T) {
	const issueID = "tm-edit-1"
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

	// mustNewModel sets sizeKnown=true and no-op schedulers; the model is then
	// handed to teatest which drives the full runtime loop.
	model := mustNewModel(t, services)

	// Wire the test-only hook so we get a precise signal when editIssueResultMsg
	// has been fully processed and the toast set — eliminating the time.Sleep.
	var editResultCount atomic.Int32
	model.onEditIssueResult = func() { editResultCount.Add(1) }

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
	// The onEditIssueResult hook fires synchronously after showToast, so this
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
// and the onEditIssueResult hook (editIssueResultMsg processed, toast
// set). Gating on repository.HasCall(UpdateIssue) is not available here (we are
// asserting the opposite). Final assertion uses FinalModel — output-buffer scanning
// is not reliable for post-tea.Exec frames under CI load.
func TestEditFlowNoChangeTeatest(t *testing.T) {
	const issueID = "tm-edit-2"

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

	model := mustNewModel(t, services)

	// Wire the test-only hook so we get a precise signal when editIssueResultMsg
	// has been fully processed and the toast set — eliminating the time.Sleep.
	var editResultCount atomic.Int32
	model.onEditIssueResult = func() { editResultCount.Add(1) }

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
// and the onEditIssueResult hook (editIssueResultMsg processed, toast
// set). The hook fires synchronously from Update after showToast — no sleep
// needed. Final assertion uses FinalModel — post-tea.Exec View() frames do not
// reliably reach the output pipe under CI load.
func TestEditFlowEditorErrorTeatest(t *testing.T) {
	const issueID = "tm-edit-3"

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

	model := mustNewModel(t, services)

	// Wire the test-only hook so we get a precise signal when editIssueResultMsg
	// has been fully processed and the toast set — eliminating the time.Sleep.
	var editResultCount atomic.Int32
	model.onEditIssueResult = func() { editResultCount.Add(1) }

	tm := testui.NewTestModelWithSize(t, model, 120, 34)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 34})

	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), editFlowTimeout, "Error Test Title")

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	// Gate 1: tea.Exec dispatched and FakeExecCommand.Run ran.
	testui.WaitForConditionWithTimeout(t, editFlowTimeout, func() bool {
		return fakeCmd.RunCount() >= 1
	})
	// Gate 2: editIssueResultMsg was processed by Update and the toast set.
	// The onEditIssueResult hook fires synchronously after showToast, giving a
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

func TestModelEditHotkeyUsesEditorService(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1, func(i *memoryrepo.Issue) {
		i.Assignee = "hans"
		i.Labels = []string{"infra"}
	})
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	services.Editor = fakeEditor

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(fakeEditor.Calls) != 1 {
		t.Fatalf("expected one editor call, got %d", len(fakeEditor.Calls))
	}
	if fakeEditor.Calls[0].IssueID != "tm-1" {
		t.Fatalf("expected selected issue tm-1, got %q", fakeEditor.Calls[0].IssueID)
	}

	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected edit hotkey to avoid launcher service, got %#v", fakeLauncher.Calls)
	}
}

func TestModelEditHotkeyShowsErrorToastWhenEditorFails(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{PrepareErr: errors.New("editor boom")}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	services.Editor = fakeEditor

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected launcher command after edit hotkey")
	}

	next, _ = m.Update(cmd())
	m = next.(Model)

	view := m.View()
	if !strings.Contains(view, "Failed to edit issue tm-1") {
		t.Fatalf("expected editor failure toast, got:\n%s", view)
	}

	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected no launcher calls when editor fails, got %#v", fakeLauncher.Calls)
	}
}

func TestModelEditIssueActionUsesEditorServiceAndUpdatesDetail(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-9", "Ninth", "task", 2)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// Seed initial detail (before edit) — memory repo returns last-seeded for a given ID,
	// so we seed "after edit" after Init() has loaded the "before" state.
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "detail before edit",
	})

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	fakeEditor := &fakes.FakeEditor{ApplyResult: launchereditor.Result{Updated: true}}
	services.Editor = fakeEditor

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if m.detail.Detail.Summary.ID != "tm-9" {
		t.Fatalf("expected initial detail load for selected issue tm-9, got %q", m.detail.Detail.Summary.ID)
	}

	// Re-seed with the "after edit" detail so subsequent Issue() call returns updated data.
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth edited", Status: "open", Type: "task", Priority: 2},
		Description: "detail after edit",
	})
	mark := gw.resetMark()

	// Phase 1: press 'e' → prepareEditCmd.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected edit command from edit hotkey")
	}

	// Phase 2: run prepareEditCmd → editIssuePreparedMsg; model returns tea.Exec cmd.
	preparedMsg := cmd()
	prepared, ok := preparedMsg.(editIssuePreparedMsg)
	if !ok {
		t.Fatalf("expected editIssuePreparedMsg, got %T", preparedMsg)
	}
	next, execCmd := m.Update(prepared)
	m = next.(Model)
	if execCmd == nil {
		t.Fatalf("expected tea.Exec command after prepare message")
	}

	// Phase 3: inject editorExitedMsg directly (bypasses real tea.Exec in unit tests).
	next, applyCmd := m.Update(editorExitedMsg{prepared: prepared.prepared, execErr: nil})
	m = next.(Model)
	if applyCmd == nil {
		t.Fatalf("expected apply command after editor exited message")
	}
	m = applyMessages(t, m, runBatch(applyCmd))

	if len(fakeEditor.Calls) != 1 {
		t.Fatalf("expected one editor call, got %d", len(fakeEditor.Calls))
	}
	if fakeEditor.Calls[0].IssueID != "tm-9" {
		t.Fatalf("expected editor call for tm-9, got %q", fakeEditor.Calls[0].IssueID)
	}

	if !gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected detail reload via Issue after successful update, calls=%#v", gw.Calls())
	}

	if m.detail.Detail.Summary.Title != "Ninth edited" {
		t.Fatalf("expected updated detail title after reload, got %q", m.detail.Detail.Summary.Title)
	}
	if m.detail.Detail.Description != "detail after edit" {
		t.Fatalf("expected updated detail description after reload, got %q", m.detail.Detail.Description)
	}
}

func TestModelEditHotkeyInDetailModeUsesEditorService(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-9", "Ninth", "task", 2)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "detail before edit",
	})

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	fakeEditor := &fakes.FakeEditor{}
	services.Editor = fakeEditor

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	mark := gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected editor command from edit hotkey")
	}

	next, _ = m.Update(cmd())
	m = next.(Model)

	if len(fakeEditor.Calls) != 1 {
		t.Fatalf("expected one editor call, got %d", len(fakeEditor.Calls))
	}
	if fakeEditor.Calls[0].IssueID != "tm-9" {
		t.Fatalf("expected selected detail issue tm-9, got %q", fakeEditor.Calls[0].IssueID)
	}

	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected no launcher calls for edit hotkey, got %#v", fakeLauncher.Calls)
	}

	if gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("did not expect issue reload from launcher action, calls=%#v", gw.Calls())
	}
}

func TestModelBuiltInLauncherHotkeysUseLauncherService(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1, func(i *memoryrepo.Issue) { i.Labels = []string{"ui"} })
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(fakeLauncher.Calls) != 1 || fakeLauncher.Calls[0].Action != "nvim" {
		t.Fatalf("expected nvim launcher call before toast assertion, got %#v", fakeLauncher.Calls)
	}

	next, _ = m.Update(launchActionResultMsg{action: "nvim", err: nil})
	m = next.(Model)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(fakeLauncher.Calls) != 3 {
		t.Fatalf("expected 3 launcher calls, got %d", len(fakeLauncher.Calls))
	}

	actions := []string{fakeLauncher.Calls[0].Action, fakeLauncher.Calls[1].Action, fakeLauncher.Calls[2].Action}
	if actions[0] != "nvim" || actions[1] != "opencode" || actions[2] != "shell-command" {
		t.Fatalf("expected launcher actions [nvim opencode shell-command], got %#v", actions)
	}
}

func TestModelLauncherSuccessToastClarifiesBackgroundLifecycle(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, _ := m.Update(launchActionResultMsg{action: "nvim", err: nil})
	m = next.(Model)

	view := m.View()
	if !strings.Contains(view, "background (no return flow)") || !strings.Contains(view, "Use e for edit/save round-trip") {
		t.Fatalf("expected launcher lifecycle guidance toast, got:\n%s", view)
	}
}
