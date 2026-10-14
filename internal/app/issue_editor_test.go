package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	issueeditmode "github.com/hk9890/task-manager-ui/internal/mode/issueedit"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// editorShell is a loaded shell on the Board at 80x24 with the selection on
// tm-1, an open task with a description, a label and an assignee.
func editorShell(t *testing.T) (Model, *fakes.TrackedRepository) {
	t.Helper()

	gw := fakes.NewTracked()
	gw.Memory.Seed(memoryrepo.Issue{
		ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1,
		Assignee: "hans", Labels: []string{"ui"}, Description: "The stored description.",
	})
	seedInProgress(gw, "tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := firstSelectionID(m, mode.Board); got != "tm-1" {
		t.Fatalf("the board selection is %q, want tm-1", got)
	}
	return m, gw
}

// openEditor presses key and expects the issue editor on screen.
func openEditor(t *testing.T, m Model, key string) Model {
	t.Helper()
	m = pressKey(t, m, key)
	if m.active != mode.IssueEdit {
		t.Fatalf("%q left the shell on %q, want the issue editor; toast %q", key, m.active, m.toast.View())
	}
	return m
}

// withEditorOn is m with the editor on screen over the surface m shows, as a
// save of kind on issueID finds it.
func withEditorOn(m Model, kind issueeditmode.Kind, issueID string) Model {
	m.showEditor(editorTarget{kind: kind, issueID: issueID}, issueeditmode.Options{})
	return m
}

func updateInputs(t *testing.T, gw *fakes.TrackedRepository) []domain.UpdateIssueInput {
	t.Helper()
	var out []domain.UpdateIssueInput
	for _, call := range gw.CallsFor(fakes.MethodUpdateIssue) {
		out = append(out, call.Args.(domain.UpdateIssueInput))
	}
	return out
}

func writeCalls(gw *fakes.TrackedRepository) []fakes.Method {
	var out []fakes.Method
	for _, call := range gw.Calls() {
		switch call.Method {
		case fakes.MethodCreateIssue, fakes.MethodUpdateIssue, fakes.MethodCloseIssue,
			fakes.MethodAddComment, fakes.MethodEditComment, fakes.MethodDeleteComment:
			out = append(out, call.Method)
		}
	}
	return out
}

func TestCreateKeyOpensTheEditorAndSaveCreatesTheIssue(t *testing.T) {
	m, gw := editorShell(t)

	m = openEditor(t, m, "alt+n")
	view := plainShell(m)
	testui.AssertContainsAll(t, view, "New issue", "a new issue", "Title", "Type", "Priority", "Assignee", "Labels", "Description", "Save ctrl+s", "Cancel esc")
	if strings.Contains(view, "Status") || strings.Contains(view, "Board") {
		t.Fatalf("a new issue draws a status row or the shell chrome:\n%s", view)
	}

	m = typeInto(t, m, "Written in the editor")
	m = pressKey(t, m, "tab")   // type
	m = pressKey(t, m, "right") // bug
	m = pressKey(t, m, "tab")   // priority
	m = pressKey(t, m, "left")  // P1
	m = pressKey(t, m, "tab")
	m = typeInto(t, m, " alice ")
	m = pressKey(t, m, "tab")
	m = typeInto(t, m, "ui, docs")
	m = pressKey(t, m, "tab")
	m = typeInto(t, m, "First line")
	m = pressKey(t, m, "enter")
	m = typeInto(t, m, "second line")
	m = pressKey(t, m, "ctrl+s")

	calls := gw.CallsFor(fakes.MethodCreateIssue)
	if len(calls) != 1 {
		t.Fatalf("expected one CreateIssue call, got %d; toast %q", len(calls), m.toast.View())
	}
	input := calls[0].Args.(domain.CreateIssueInput)
	if input.Title != "Written in the editor" || input.Type != "bug" || *input.Priority != 1 ||
		input.Assignee != "alice" || strings.Join(input.Labels, "|") != "ui|docs" || input.Description != "First line\nsecond line" {
		t.Errorf("CreateIssue received %+v (priority %d)", input, *input.Priority)
	}
	if m.active != mode.Board {
		t.Errorf("a save left the shell on %q, want the Board", m.active)
	}
	if !strings.Contains(m.toast.View(), "Created issue ") {
		t.Errorf("toast = %q, want the created issue named", m.toast.View())
	}
}

func TestUpdateKeyReadsTheIssueAgainAndSaveWritesTitleAndDescription(t *testing.T) {
	m, gw := editorShell(t)

	mark := gw.CallCount()
	m = openEditor(t, m, "alt+u")
	if !gw.HasCallSince(mark, fakes.MethodIssue) || !gw.HasCallSince(mark, fakes.MethodCatalogs) {
		t.Fatalf("the editor opened without reading the issue and the catalogs again, calls=%#v", gw.Calls()[mark:])
	}
	testui.AssertContainsAll(t, plainShell(m), "Edit issue tm-1", "Ready first", "Status", "open", "hans", "ui", "The stored description.")

	m = typeInto(t, m, " again")
	for range 6 {
		m = pressKey(t, m, "tab")
	}
	m = typeInto(t, m, "New first words. ")
	m = pressKey(t, m, "ctrl+s")

	inputs := updateInputs(t, gw)
	if len(inputs) != 1 {
		t.Fatalf("expected one UpdateIssue call, got %d; toast %q", len(inputs), m.toast.View())
	}
	got := inputs[0]
	if got.Title == nil || *got.Title != "Ready first again" || got.Description == nil || *got.Description != "New first words. The stored description." {
		t.Errorf("UpdateIssue received title %v and description %v", got.Title, got.Description)
	}
	if got.Status != nil || got.Type != nil || got.Priority != nil || got.Assignee != nil || got.Labels != nil || got.ClearLabels {
		t.Errorf("UpdateIssue wrote a field nobody changed: %+v", got)
	}
	if state := issueState(gw, "tm-1"); state.Summary.Title != "Ready first again" || state.Description != "New first words. The stored description." {
		t.Errorf("the store holds title %q and description %q", state.Summary.Title, state.Description)
	}
	if m.active != mode.Board || !strings.Contains(m.toast.View(), "Updated issue tm-1") {
		t.Errorf("after the save the shell is on %q with toast %q", m.active, m.toast.View())
	}
}

func TestSaveWithoutAChangeWritesNothing(t *testing.T) {
	m, gw := editorShell(t)

	m = openEditor(t, m, "alt+u")
	m = pressKey(t, m, "ctrl+s")

	if writes := writeCalls(gw); len(writes) != 0 {
		t.Errorf("a save without a change wrote %v", writes)
	}
	if m.active != mode.Board || !strings.Contains(m.toast.View(), "No changes saved for issue tm-1") {
		t.Errorf("after the save the shell is on %q with toast %q", m.active, m.toast.View())
	}
}

func TestCloseKeyOpensTheEditorOnTheReasonAndSaveCloses(t *testing.T) {
	t.Run("the reason alone is one CloseIssue", func(t *testing.T) {
		m, gw := editorShell(t)

		m = openEditor(t, m, "delete")
		testui.AssertContainsAll(t, plainShell(m), "Edit issue tm-1", "closed", "Reason")
		if writes := writeCalls(gw); len(writes) != 0 {
			t.Fatalf("the close key wrote %v before a save", writes)
		}

		m = typeInto(t, m, " shipped ")
		m = pressKey(t, m, "ctrl+s")

		if writes := writeCalls(gw); len(writes) != 1 || writes[0] != fakes.MethodCloseIssue {
			t.Fatalf("writes = %v, want one CloseIssue", writes)
		}
		if got := gw.CallsFor(fakes.MethodCloseIssue)[0].Args.(domain.CloseIssueInput).Reason; got != "shipped" {
			t.Errorf("CloseIssue received the reason %q, want shipped", got)
		}
		if state := issueState(gw, "tm-1"); state.Summary.Status != "closed" || state.CloseReason != "shipped" {
			t.Errorf("the store holds status %q and reason %q", state.Summary.Status, state.CloseReason)
		}
		if !strings.Contains(m.toast.View(), "Closed issue tm-1") {
			t.Errorf("toast = %q", m.toast.View())
		}
	})

	t.Run("another changed field is written first, without the status", func(t *testing.T) {
		m, gw := editorShell(t)

		m = openEditor(t, m, "delete")
		m = typeInto(t, m, "done")
		m = pressKey(t, m, "shift+tab") // status
		m = pressKey(t, m, "shift+tab") // title
		m = typeInto(t, m, "!")
		m = pressKey(t, m, "ctrl+s")

		if writes := writeCalls(gw); len(writes) != 2 || writes[0] != fakes.MethodUpdateIssue || writes[1] != fakes.MethodCloseIssue {
			t.Fatalf("writes = %v, want UpdateIssue then CloseIssue", writes)
		}
		if input := updateInputs(t, gw)[0]; input.Status != nil || input.Title == nil || *input.Title != "Ready first!" {
			t.Errorf("UpdateIssue received %+v, want the title alone", input)
		}
		if state := issueState(gw, "tm-1"); state.Summary.Status != "closed" || state.CloseReason != "done" || state.Summary.Title != "Ready first!" {
			t.Errorf("the store holds %+v with reason %q", state.Summary, state.CloseReason)
		}
		if m.active != mode.Board {
			t.Errorf("the shell is on %q", m.active)
		}
	})

	t.Run("an issue already closed opens no editor", func(t *testing.T) {
		m, gw := editorShell(t)
		if err := gw.Memory.CloseIssue(t.Context(), "tm-1", domain.CloseIssueInput{Reason: "earlier"}); err != nil {
			t.Fatalf("CloseIssue: %v", err)
		}

		m = pressKey(t, m, "delete")
		if m.active != mode.Board || !strings.Contains(m.toast.View(), "Issue tm-1 is closed") {
			t.Errorf("the shell is on %q with toast %q", m.active, m.toast.View())
		}
	})
}

// A close that is refused after the other fields were written says so, keeps
// the editor open, and reloads what the first write changed.
func TestRefusedCloseAfterAWrittenUpdateSaysWhatIsSaved(t *testing.T) {
	m, gw := editorShell(t)
	gw.SetError(fakes.MethodCloseIssue, errors.New("close issue: a hook refused the close"))

	m = openEditor(t, m, "delete")
	m = pressKey(t, m, "shift+tab")
	m = pressKey(t, m, "shift+tab")
	m = typeInto(t, m, "!")
	mark := gw.CallCount()
	m = pressKey(t, m, "ctrl+s")

	if m.active != mode.IssueEdit {
		t.Fatalf("a refused close closed the editor; the shell is on %q", m.active)
	}
	testui.AssertContainsAll(t, plainShell(m), "a hook refused the close", "The other changes are saved.", "Ready first!")
	if state := issueState(gw, "tm-1"); state.Summary.Title != "Ready first!" || state.Summary.Status != "open" {
		t.Errorf("the store holds %+v", state.Summary)
	}
	if !gw.HasCallSince(mark, fakes.MethodIssue) {
		t.Error("the written update started no detail reload")
	}
	if !m.refreshStateBySurface[mode.Board].dirty {
		t.Error("the written update left the Board clean under the editor")
	}

	// The same save again sends the same update and then the close.
	gw.SetError(fakes.MethodCloseIssue, nil)
	m = pressKey(t, m, "ctrl+s")
	if m.active != mode.Board || issueState(gw, "tm-1").Summary.Status != "closed" {
		t.Errorf("the second save left the shell on %q and the issue %q", m.active, issueState(gw, "tm-1").Summary.Status)
	}
}

func TestSteppingTheStatusOffClosedReopensInOneUpdate(t *testing.T) {
	m, gw := editorShell(t)
	if err := gw.Memory.CloseIssue(t.Context(), "tm-1", domain.CloseIssueInput{Reason: "earlier"}); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}

	m = openEditor(t, m, "alt+u")
	testui.AssertContainsAll(t, plainShell(m), "closed", "Reason", "earlier")
	m = pressKey(t, m, "tab")   // status; the reason is read-only
	m = pressKey(t, m, "right") // wraps to open
	m = pressKey(t, m, "tab")
	if strings.Contains(plainShell(m), "Reason") {
		t.Fatalf("the reason row stays under an open status:\n%s", plainShell(m))
	}
	m = pressKey(t, m, "ctrl+s")

	if writes := writeCalls(gw); len(writes) != 1 || writes[0] != fakes.MethodUpdateIssue {
		t.Fatalf("writes = %v, want one UpdateIssue", writes)
	}
	if input := updateInputs(t, gw)[0]; input.Status == nil || *input.Status != "open" {
		t.Errorf("UpdateIssue received %+v, want the status open", input)
	}
	if state := issueState(gw, "tm-1"); state.Summary.Status != "open" {
		t.Errorf("the store holds status %q", state.Summary.Status)
	}
	if m.active != mode.Board {
		t.Errorf("the shell is on %q", m.active)
	}
}

// A field of a closed issue changed with the status left on closed is refused
// by the store, and the refusal stands on the editor.
func TestChangeToAClosedIssueIsRefusedOnTheEditor(t *testing.T) {
	m, gw := editorShell(t)
	if err := gw.Memory.CloseIssue(t.Context(), "tm-1", domain.CloseIssueInput{Reason: "earlier"}); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}

	m = openEditor(t, m, "alt+u")
	m = typeInto(t, m, " typed")
	m = pressKey(t, m, "ctrl+s")

	view := plainShell(m)
	if m.active != mode.IssueEdit || !strings.Contains(view, "issue is closed; reopen it before editing") || !strings.Contains(view, "Ready first typed") {
		t.Errorf("the refusal is not on the editor (shell on %q):\n%s", m.active, view)
	}
	if title := issueState(gw, "tm-1").Summary.Title; title != "Ready first" {
		t.Errorf("the store holds the title %q", title)
	}
}

func TestCommentKeyOpensTheTextAloneAndSaveAddsTheComment(t *testing.T) {
	m, gw := editorShell(t)

	mark := gw.CallCount()
	m = openEditor(t, m, "alt+a")
	if gw.CallCount() != mark {
		t.Errorf("a new comment read the store before it opened: %#v", gw.Calls()[mark:])
	}
	view := plainShell(m)
	testui.AssertContainsAll(t, view, "New comment on tm-1", "Ready first", "Comment", "Save ctrl+s")
	if strings.Contains(view, "Title") || strings.Contains(view, "Priority") {
		t.Fatalf("a comment draws issue fields:\n%s", view)
	}

	m = typeInto(t, m, "A comment")
	m = pressKey(t, m, "enter")
	m = typeInto(t, m, "of two lines")
	m = pressKey(t, m, "ctrl+s")

	calls := gw.CallsFor(fakes.MethodAddComment)
	if len(calls) != 1 || calls[0].IssueID != "tm-1" || calls[0].Args.(domain.AddCommentInput).Body != "A comment\nof two lines" {
		t.Fatalf("AddComment calls = %#v", calls)
	}
	if m.active != mode.Board || !strings.Contains(m.toast.View(), "Added comment to tm-1") {
		t.Errorf("after the save the shell is on %q with toast %q", m.active, m.toast.View())
	}
}

func TestSaveOfACommentEditReplacesTheCommentAndNoChangeWritesNothing(t *testing.T) {
	m, gw := editorShell(t)
	gw.Memory.SeedComments("tm-1", memoryrepo.Comment{ID: "c-1", Author: "hans", Body: "as it was"})

	open := func(m Model) Model {
		m.showEditor(editorTarget{kind: issueeditmode.KindCommentEdit, issueID: "tm-1", commentID: "c-1"},
			issueeditmode.Options{Values: issueeditmode.Values{Text: "as it was"}})
		return m
	}

	unchanged := pressKey(t, open(m), "ctrl+s")
	if writes := writeCalls(gw); len(writes) != 0 {
		t.Fatalf("an unchanged comment wrote %v", writes)
	}
	if !strings.Contains(unchanged.toast.View(), "No changes saved") {
		t.Errorf("toast = %q", unchanged.toast.View())
	}

	m = typeInto(t, open(m), "Now ")
	m = pressKey(t, m, "ctrl+s")

	calls := gw.CallsFor(fakes.MethodEditComment)
	if len(calls) != 1 || calls[0].IssueID != "tm-1" || calls[0].CommentID != "c-1" || calls[0].Args.(domain.EditCommentInput).Body != "Now as it was" {
		t.Fatalf("EditComment calls = %#v", calls)
	}
	if comments := issueState(gw, "tm-1").Comments; len(comments) != 1 || comments[0].Body != "Now as it was" {
		t.Errorf("the store holds the comments %#v", comments)
	}
	if !strings.Contains(m.toast.View(), "Edited comment on tm-1") {
		t.Errorf("toast = %q", m.toast.View())
	}
}

// A refused write leaves the editor open with what was typed and shows the
// refusal on it, for each kind of write.
func TestRefusedWriteKeepsTheEditorAndTheTypedValues(t *testing.T) {
	refusal := domain.RepositoryError{Code: domain.ErrorCodeHookDenied, Operation: "write", Message: "the body has no Context section"}

	tests := []struct {
		name   string
		key    string
		method fakes.Method
	}{
		{"create", "alt+n", fakes.MethodCreateIssue},
		{"update", "alt+u", fakes.MethodUpdateIssue},
		{"comment", "alt+a", fakes.MethodAddComment},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, gw := editorShell(t)
			gw.SetError(tt.method, refusal)

			m = openEditor(t, m, tt.key)
			m = typeInto(t, m, "typed-by-hand")
			m = pressKey(t, m, "ctrl+s")

			if m.active != mode.IssueEdit {
				t.Fatalf("a refused write closed the editor; the shell is on %q", m.active)
			}
			testui.AssertContainsAll(t, plainShell(m), "the body has no Context section", "typed-by-hand", "Save ctrl+s")
			if m.toast.Visible() {
				t.Errorf("a refused write raised the toast %q; the screen shows it", m.toast.View())
			}
			if m.refreshStateBySurface[mode.Board].dirty {
				t.Error("a refused write marked the Board dirty")
			}

			// The screen takes keys again, and the next save clears the error.
			gw.SetError(tt.method, nil)
			m = typeInto(t, m, "-more")
			m = pressKey(t, m, "ctrl+s")
			if m.active != mode.Board {
				t.Errorf("the second save left the shell on %q; error %q", m.active, plainShell(m))
			}
		})
	}
}

// The memory backend refuses an empty title and an empty comment itself: the
// editor checks nothing.
func TestStoreValidationIsShownOnTheEditor(t *testing.T) {
	for key, want := range map[string]string{"alt+n": "title", "alt+a": "comment"} {
		m, gw := editorShell(t)
		m = openEditor(t, m, key)
		m = pressKey(t, m, "ctrl+s")

		if m.active != mode.IssueEdit || !strings.Contains(plainShell(m), want) {
			t.Errorf("%s: an empty save left the shell on %q:\n%s", key, m.active, plainShell(m))
		}
		if writes := writeCalls(gw); len(writes) != 1 {
			t.Errorf("%s: writes = %v, want the one the store refused", key, writes)
		}
	}
}

func TestEscapeDiscardsTheTypedTextAndWritesNothing(t *testing.T) {
	m, gw := editorShell(t)

	m = openEditor(t, m, "alt+u")
	m = typeInto(t, m, " changed")
	m = pressKey(t, m, "esc")

	if m.active != mode.Board {
		t.Errorf("Escape left the shell on %q, want the Board", m.active)
	}
	if writes := writeCalls(gw); len(writes) != 0 {
		t.Errorf("Escape wrote %v", writes)
	}

	// The next open starts from the store, not from what was discarded.
	m = openEditor(t, m, "alt+u")
	if strings.Contains(plainShell(m), "changed") {
		t.Errorf("the discarded text is back:\n%s", plainShell(m))
	}
}

// A click on Save sends the request as a Cmd, so it can arrive after the
// editor is gone.
func TestSubmitThatArrivesWithNoEditorOpenWritesNothing(t *testing.T) {
	m, gw := editorShell(t)

	m = applyMessages(t, m, []tea.Msg{issueeditmode.SubmitMsg{After: issueeditmode.Values{Title: "late"}}})

	if m.active != mode.Board {
		t.Errorf("the submit left the shell on %q, want the Board", m.active)
	}
	if writes := writeCalls(gw); len(writes) != 0 {
		t.Errorf("the submit wrote %v", writes)
	}
}

func TestCancelButtonClosesTheEditorAndAClickOnSaveWrites(t *testing.T) {
	m, gw := editorShell(t)

	m = openEditor(t, m, "alt+u")
	x, y := testui.FindCell(t, testui.View(m), "Cancel esc")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	if m.active != mode.Board || len(writeCalls(gw)) != 0 {
		t.Fatalf("a click on Cancel left the shell on %q with writes %v", m.active, writeCalls(gw))
	}

	m = openEditor(t, m, "alt+u")
	x, y = testui.FindCell(t, testui.View(m), "Assignee")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	m = typeInto(t, m, "-two")
	x, y = testui.FindCell(t, testui.View(m), "Save ctrl+s")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))

	inputs := updateInputs(t, gw)
	if len(inputs) != 1 || inputs[0].Assignee == nil || *inputs[0].Assignee != "hans-two" {
		t.Fatalf("a click on the assignee, a key and a click on Save wrote %+v", inputs)
	}
	if m.active != mode.Board {
		t.Errorf("the shell is on %q", m.active)
	}
}

// While a save is in flight the editor takes Escape, so the result of the
// write finds the typed values on screen.
func TestEscapeDuringASaveLeavesTheEditorOpen(t *testing.T) {
	m, gw := editorShell(t)
	gw.SetError(fakes.MethodUpdateIssue, errors.New("update issue: refused"))

	m = openEditor(t, m, "alt+u")
	m = typeInto(t, m, " typed")

	next, submit := m.Update(testui.Press("ctrl+s"))
	m = next.(Model)
	next, save := m.Update(submit())
	m = next.(Model)
	if !strings.Contains(plainShell(m), "Saving…") {
		t.Fatalf("a save in flight is not shown:\n%s", plainShell(m))
	}

	m = pressKey(t, m, "esc")
	m = typeInto(t, m, "ignored")
	x, y := testui.FindCell(t, testui.View(m), "Title")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	if m.active != mode.IssueEdit {
		t.Fatalf("Escape during a save closed the editor; the shell is on %q", m.active)
	}

	m = applyMessages(t, m, runBatch(save))
	view := plainShell(m)
	if m.active != mode.IssueEdit || !strings.Contains(view, "update issue: refused") || !strings.Contains(view, "Ready first typed") || strings.Contains(view, "ignored") {
		t.Errorf("the refused write did not find the typed values on screen (on %q):\n%s", m.active, view)
	}

	if m = pressKey(t, m, "esc"); m.active != mode.Board {
		t.Errorf("Escape after the refusal left the shell on %q", m.active)
	}
}

// A save is in flight from the key that asked for it. A second save key, an
// Enter on Save or an Escape that arrives before the shell hears of the first
// starts no second write and does not close the editor.
func TestSecondSaveBeforeTheFirstIsHandledWritesOnce(t *testing.T) {
	m, gw := editorShell(t)

	m = openEditor(t, m, "alt+n")
	m = typeInto(t, m, "Written once")
	next, first := m.Update(testui.Press("ctrl+s"))
	m = next.(Model)
	for _, key := range []string{"ctrl+s", "enter", "esc"} {
		var second tea.Cmd
		next, second = m.Update(testui.Press(key))
		m = next.(Model)
		if msgs := runBatch(second); len(msgs) != 0 {
			t.Fatalf("%q after the save key produced %v, want nothing", key, msgs)
		}
	}
	if m.active != mode.IssueEdit {
		t.Fatalf("the shell is on %q before the save is handled, want the issue editor", m.active)
	}

	m = applyMessages(t, m, runBatch(first))
	if calls := gw.CallsFor(fakes.MethodCreateIssue); len(calls) != 1 {
		t.Errorf("two save keys wrote %d issues, want 1", len(calls))
	}
	if m.active != mode.Board {
		t.Errorf("the save left the shell on %q, want the board", m.active)
	}
}

// A key, a click or a wheel notch that arrives while the editor is still
// reading what it opens with cancels the open.
func TestPendingOpenIsCancelledByAKeyAClickOrTheWheel(t *testing.T) {
	interrupts := map[string]tea.Msg{
		"a key":         testui.Press("down"),
		"a click":       testui.Click(40, 10, tea.MouseLeft),
		"a wheel notch": testui.Wheel(40, 10, tea.MouseWheelDown),
	}
	for name, interrupt := range interrupts {
		t.Run(name, func(t *testing.T) {
			m, _ := editorShell(t)

			next, load := m.Update(testui.Press("alt+u"))
			m = next.(Model)
			if m.pendingEditor == 0 {
				t.Fatal("the update key started no pending open")
			}
			m = send(t, m, interrupt)
			m = applyMessages(t, m, runBatch(load))

			if m.active == mode.IssueEdit {
				t.Error("the editor opened although the operator moved on")
			}
		})
	}

	t.Run("the pointer moving cancels nothing", func(t *testing.T) {
		m, _ := editorShell(t)
		next, load := m.Update(testui.Press("alt+u"))
		m = send(t, next.(Model), testui.Move(40, 10))
		if m = applyMessages(t, m, runBatch(load)); m.active != mode.IssueEdit {
			t.Errorf("a pointer move cancelled the open; the shell is on %q", m.active)
		}
	})
}

// An Escape that cancelled a pending open is spent on that: it does not also
// leave Detail.
func TestEscapeThatCancelsAPendingOpenDoesNotLeaveTheMode(t *testing.T) {
	m, _ := editorShell(t)
	m = pressKey(t, m, "enter")
	if m.active != mode.Detail {
		t.Fatalf("enter left the shell on %q, want Detail", m.active)
	}

	next, load := m.Update(testui.Press("alt+u"))
	m = pressKey(t, next.(Model), "esc")
	m = applyMessages(t, m, runBatch(load))

	if m.active != mode.Detail {
		t.Errorf("the Escape that cancelled the open also left Detail for %q", m.active)
	}
}

func TestFailedLoadOpensNoEditorAndSaysWhy(t *testing.T) {
	m, gw := editorShell(t)
	gw.SetError(fakes.MethodCatalogs, errors.New("catalogs: store is gone"))

	for _, key := range []string{"alt+n", "alt+u", "delete"} {
		got := pressKey(t, m, key)
		if got.active != mode.Board || !strings.Contains(got.toast.View(), "Failed to open the editor") {
			t.Errorf("%q: the shell is on %q with toast %q", key, got.active, got.toast.View())
		}
	}
}

func TestIssueKeysWithoutASelectionOpenNoEditor(t *testing.T) {
	services, err := NewServices(fakes.NewTracked(), config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	for key, want := range map[string]string{
		"alt+u":  "No selected issue to edit",
		"delete": "No selected issue to close",
		"alt+a":  "No selected issue to comment on",
	} {
		got := pressKey(t, m, key)
		if got.active != mode.Board || !strings.Contains(got.toast.View(), want) {
			t.Errorf("%q: the shell is on %q with toast %q, want %q", key, got.active, got.toast.View(), want)
		}
	}
}

// From Detail the editor returns to Detail, on the issue Detail shows, and
// the detail is read again after the write.
func TestEditorOpenedFromDetailReturnsThereAndReloadsIt(t *testing.T) {
	m, gw := editorShell(t)
	m = pressKey(t, m, "enter")

	m = openEditor(t, m, "alt+u")
	m = typeInto(t, m, " edited")
	mark := gw.CallCount()
	m = pressKey(t, m, "ctrl+s")

	if m.active != mode.Detail {
		t.Fatalf("the save left the shell on %q, want Detail", m.active)
	}
	if !gw.HasCallSince(mark, fakes.MethodIssue) || m.detail.Detail.Summary.Title != "Ready first edited" {
		t.Errorf("Detail shows %q after the save; issue read again: %v", m.detail.Detail.Summary.Title, gw.HasCallSince(mark, fakes.MethodIssue))
	}
}

// A selection that moves under the editor does not retarget the write: the
// editor holds the issue it was opened on.
func TestSaveWritesTheIssueTheEditorWasOpenedOn(t *testing.T) {
	m, gw := editorShell(t)

	m = openEditor(t, m, "alt+u")
	m = send(t, m, mode.SelectionChangedMsg{Mode: mode.Board, Selection: &mode.Selection{Issue: domain.IssueSummary{ID: "tm-2", Title: "In progress"}}})
	if m.detail.TargetID() == "tm-2" {
		t.Error("a selection under the editor started a detail load")
	}
	m = typeInto(t, m, "!")
	m = pressKey(t, m, "ctrl+s")

	calls := gw.CallsFor(fakes.MethodUpdateIssue)
	if len(calls) != 1 || calls[0].IssueID != "tm-1" {
		t.Fatalf("UpdateIssue calls = %#v, want one on tm-1", calls)
	}
}

func TestIssueUpdateIsTheDifferenceField(t *testing.T) {
	t.Parallel()

	before := issueeditmode.Values{Title: "Task", Status: "open", Type: "task", Priority: 1, Assignee: "alice", Labels: "ui, docs", Text: "body"}
	str := func(s string) *string { return &s }

	tests := []struct {
		name        string
		change      func(*issueeditmode.Values)
		want        domain.UpdateIssueInput
		wantChanged bool
		wantClosing bool
	}{
		{name: "nothing", change: func(*issueeditmode.Values) {}},
		{name: "white space around a one-line field is no change", change: func(v *issueeditmode.Values) { v.Title, v.Assignee = " Task ", "alice " }},
		{name: "the labels in another order are no change", change: func(v *issueeditmode.Values) { v.Labels = "docs,ui, ui" }},
		{name: "a cleared assignee unassigns", change: func(v *issueeditmode.Values) { v.Assignee = "" }, want: domain.UpdateIssueInput{Assignee: str("")}, wantChanged: true},
		{name: "cleared labels clear the set", change: func(v *issueeditmode.Values) { v.Labels = " " }, want: domain.UpdateIssueInput{ClearLabels: true}, wantChanged: true},
		{name: "other labels replace the set", change: func(v *issueeditmode.Values) { v.Labels = "ui, api" }, want: domain.UpdateIssueInput{Labels: []string{"ui", "api"}}, wantChanged: true},
		{name: "the text is compared as typed", change: func(v *issueeditmode.Values) { v.Text = "body " }, want: domain.UpdateIssueInput{Description: str("body ")}, wantChanged: true},
		{name: "a status but closed is a field", change: func(v *issueeditmode.Values) { v.Status = "blocked" }, want: domain.UpdateIssueInput{Status: str("blocked")}, wantChanged: true},
		{name: "closed is a close, not a field", change: func(v *issueeditmode.Values) { v.Status = "closed" }, wantClosing: true},
		{name: "a close with another field", change: func(v *issueeditmode.Values) { v.Status, v.Type = "closed", "bug" }, want: domain.UpdateIssueInput{Type: str("bug")}, wantChanged: true, wantClosing: true},
	}
	for _, tt := range tests {
		after := before
		tt.change(&after)
		input, changed, closing := issueUpdate(before, after)

		if changed != tt.wantChanged || closing != tt.wantClosing {
			t.Errorf("%s: changed %v, closing %v; want %v, %v", tt.name, changed, closing, tt.wantChanged, tt.wantClosing)
		}
		deref := func(p *string) string {
			if p == nil {
				return "<nil>"
			}
			return *p
		}
		for field, pair := range map[string][2]*string{
			"title": {input.Title, tt.want.Title}, "status": {input.Status, tt.want.Status}, "type": {input.Type, tt.want.Type},
			"assignee": {input.Assignee, tt.want.Assignee}, "description": {input.Description, tt.want.Description},
		} {
			if deref(pair[0]) != deref(pair[1]) {
				t.Errorf("%s: %s = %s, want %s", tt.name, field, deref(pair[0]), deref(pair[1]))
			}
		}
		if strings.Join(input.Labels, "|") != strings.Join(tt.want.Labels, "|") || input.ClearLabels != tt.want.ClearLabels || input.Priority != nil {
			t.Errorf("%s: labels %v, clear %v, priority %v", tt.name, input.Labels, input.ClearLabels, input.Priority)
		}
	}

	after := before
	after.Priority = 3
	if input, changed, _ := issueUpdate(before, after); !changed || input.Priority == nil || *input.Priority != 3 {
		t.Errorf("a changed priority gives %+v (changed %v)", input, changed)
	}

	closedBefore := before
	closedBefore.Status = "closed"
	reopened := closedBefore
	reopened.Status = "open"
	if input, changed, closing := issueUpdate(closedBefore, reopened); !changed || closing || input.Status == nil || *input.Status != "open" {
		t.Errorf("a reopen gives %+v (changed %v, closing %v)", input, changed, closing)
	}
	if _, changed, closing := issueUpdate(closedBefore, closedBefore); changed || closing {
		t.Error("a closed issue left closed is a change")
	}
}

func TestEditorHintsNameTheBuiltInKeysAndTheBoundOnes(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	hint := func(kind issueeditmode.Kind) string {
		var parts []string
		for _, h := range editorHints(keys, kind) {
			parts = append(parts, h.Key+" "+h.Desc)
		}
		return strings.Join(parts, ", ")
	}
	if got, want := hint(issueeditmode.KindEdit), "tab/shift+tab field, ctrl+s save, esc cancel, left/right change, ctrl+c quit"; got != want {
		t.Errorf("issue hints = %q, want %q", got, want)
	}
	if got, want := hint(issueeditmode.KindCommentAdd), "tab/shift+tab field, ctrl+s save, esc cancel, ctrl+c quit"; got != want {
		t.Errorf("comment hints = %q, want %q", got, want)
	}
}

func TestIssueEditorGolden(t *testing.T) {
	withModelNow(t, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))
	m, _ := editorShell(t)

	m = openEditor(t, m, "alt+u")
	if m.editorTarget.kind != issueeditmode.KindEdit || m.editorReturn != mode.Board {
		t.Fatalf("the editor target is %+v, returning to %q", m.editorTarget, m.editorReturn)
	}
	testui.AssertMatchesGoldenNormalized(t, []byte(testui.View(m)), "model_issue_editor_w80.golden")
}

// A resize while the editor is up sizes it: every line fits the new screen.
func TestResizeSizesTheEditor(t *testing.T) {
	m, _ := editorShell(t)
	m = openEditor(t, m, "alt+u")
	m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})

	lines := strings.Split(plainShell(m), "\n")
	if len(lines) != 20 {
		t.Errorf("the editor is %d lines on a terminal of 20", len(lines))
	}
	for _, line := range lines {
		if width := len([]rune(line)); width > 60 {
			t.Errorf("a line is %d cells wide on a terminal of 60: %q", width, line)
		}
	}
}
