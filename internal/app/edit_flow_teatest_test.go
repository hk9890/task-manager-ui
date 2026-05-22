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
// ApplyEdits use the real IssueEditor wired to the FakeBeadsGateway so the
// full filesystem and gateway round-trip is exercised.

import (
	"strings"
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
// flow tests. It wires the real IssueEditor against the provided gateway and
// injects the FakeExecCommand factory as the ExecCommandFactory seam.
//
// The returned *fakes.FakeExecCommand lets callers configure EditedContent /
// RunErr and inspect RunCalled after the program settles.
func buildEditFlowServices(
	t *testing.T,
	gateway *fakes.FakeBeadsGateway,
	fakeCmd *fakes.FakeExecCommand,
) Services {
	t.Helper()

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	services.ExecCommandFactory = fakeCmd.Factory()
	return services
}

// seedEditIssue seeds an issue into the fake gateway and returns it. It also
// pre-sets ShowIssueResponse so the initial board load does not fail.
func seedEditIssue(t *testing.T, gateway *fakes.FakeBeadsGateway, issue domain.IssueDetail) {
	t.Helper()
	gateway.SeedIssue(issue)
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{issue.Summary},
	}
	gateway.QueryResponse = []domain.IssueSummary{issue.Summary}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = issue
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
//   - UpdateIssue is recorded on the gateway
//   - "Updated issue <id>" toast appears in the rendered output
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

	gateway := fakes.NewFakeBeadsGateway()
	seedEditIssue(t, gateway, issue)

	// Pre-configure the fake ExecCommand with content that changes the title.
	fakeCmd := &fakes.FakeExecCommand{
		EditedContent: editableDocWithTitle(issue, editedTitle),
	}

	services := buildEditFlowServices(t, gateway, fakeCmd)

	model, err := NewModel(services)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	// Set sizeKnown so View() renders content (not the empty pre-size guard).
	model.sizeKnown = true

	tm := testui.NewTestModelWithSize(t, model, 120, 34)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 34})

	// Drain the board init so there is a selected issue.
	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), editFlowTimeout, originalTitle)

	// Press 'e' to trigger the edit flow.
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	// Wait for the success toast. "Updated issue" is the toast prefix.
	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), editFlowTimeout, "Updated issue")

	if err := tm.Quit(); err != nil {
		t.Fatalf("teatest Quit: %v", err)
	}

	// Post-run assertions on the fake.
	if n := fakeCmd.RunCount(); n != 1 {
		t.Errorf("expected FakeExecCommand.Run called once, got %d", n)
	}
	if !gateway.HasCall(string(fakes.MethodUpdateIssue)) {
		t.Errorf("expected UpdateIssue call on gateway after successful edit, calls=%#v", gateway.Calls)
	}
}

// TestEditFlowNoChangeTeatest verifies the no-change path: when the editor
// writes back an identical document, no UpdateIssue is called and the
// "No changes saved" toast is shown.
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

	gateway := fakes.NewFakeBeadsGateway()
	seedEditIssue(t, gateway, issue)

	// EditedContent is the exact same rendered document — no changes.
	fakeCmd := &fakes.FakeExecCommand{
		EditedContent: domain.RenderIssueEditDocument(issue),
	}

	services := buildEditFlowServices(t, gateway, fakeCmd)

	model, err := NewModel(services)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	model.sizeKnown = true

	tm := testui.NewTestModelWithSize(t, model, 120, 34)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 34})

	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), editFlowTimeout, "Unchanged Title")

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), editFlowTimeout, "No changes saved for issue")

	if err := tm.Quit(); err != nil {
		t.Fatalf("teatest Quit: %v", err)
	}

	if n := fakeCmd.RunCount(); n != 1 {
		t.Errorf("expected FakeExecCommand.Run called once, got %d", n)
	}
	if gateway.HasCall(string(fakes.MethodUpdateIssue)) {
		t.Errorf("expected no UpdateIssue call when document is unchanged, calls=%#v", gateway.Calls)
	}
}

// TestEditFlowEditorErrorTeatest verifies the editor-error path: when the
// FakeExecCommand.Run returns an error, the "Failed to edit issue" toast is
// shown and UpdateIssue is NOT called.
//
// Note on assertion strategy: the tea.Exec error path produces only a single
// View() frame with the error toast (no follow-on async cmd), so the renderer
// may not have flushed the toast frame to the output buffer before WaitFor
// polls. Instead of asserting on output-buffer content, we:
//  1. Poll fakeCmd.RunCalled to confirm the subprocess was invoked.
//  2. Sleep 200 ms to let the closure + p.Send hops complete under -race.
//  3. Quit and use FinalModel to read the settled model state directly.
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

	gateway := fakes.NewFakeBeadsGateway()
	seedEditIssue(t, gateway, issue)

	// RunErr simulates the editor exiting with a non-zero status.
	fakeCmd := &fakes.FakeExecCommand{
		RunErr: errFakeEditorFailed,
	}

	services := buildEditFlowServices(t, gateway, fakeCmd)

	model, err := NewModel(services)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	model.sizeKnown = true

	tm := testui.NewTestModelWithSize(t, model, 120, 34)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 34})

	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), editFlowTimeout, "Error Test Title")

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	// Wait until the fake subprocess Run() is called, then give the BubbleTea
	// runtime 200 ms to complete the closure + p.Send hops before quitting.
	testui.WaitForConditionWithTimeout(t, editFlowTimeout, func() bool {
		return fakeCmd.RunCount() >= 1
	})
	time.Sleep(200 * time.Millisecond)

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
	if gateway.HasCall(string(fakes.MethodUpdateIssue)) {
		t.Errorf("expected no UpdateIssue call when editor returns error, calls=%#v", gateway.Calls)
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
