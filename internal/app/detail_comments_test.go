package app

// Detail's own buttons and the comment cursor, through the shell: what a key
// or a click opens, what the delete question writes, and what is read again
// after a comment write. The cursor itself is tested in internal/mode/detail.

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	issueeditmode "github.com/hk9890/task-manager-ui/internal/mode/issueedit"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uidetail "github.com/hk9890/task-manager-ui/internal/ui/detail"
)

// commentShell is a shell in Detail on tm-1 at 100x40, with the focus on the
// comments. tm-1 has the comments c-old and c-new, and Detail draws c-new
// first. The store's clock steps a minute a reading, so a comment written in a
// test is newer than both.
func commentShell(t *testing.T) (Model, *fakes.TrackedRepository) {
	t.Helper()

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	gw := fakes.NewTracked(memoryrepo.WithClock(func() time.Time {
		now = now.Add(time.Minute)
		return now
	}))
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	gw.Memory.SeedComments("tm-1",
		memoryrepo.Comment{ID: "c-old", Author: "alice", Body: "the older comment", CreatedAt: now.Add(-2 * time.Hour)},
		memoryrepo.Comment{ID: "c-new", Author: "bob", Body: "the newer comment", CreatedAt: now.Add(-time.Hour)},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = pressKey(t, pressKey(t, m, "enter"), "right")
	if m.active != mode.Detail || m.detail.FocusPane != uidetail.FocusPaneComments {
		t.Fatalf("fixture is on %q with focus %d, want Detail on the comments", m.active, m.detail.FocusPane)
	}
	if comment, _ := m.detail.SelectedComment(); comment.ID != "c-new" {
		t.Fatalf("fixture starts on the comment %q, want c-new", comment.ID)
	}
	return m, gw
}

func commentBodies(gw *fakes.TrackedRepository) []string {
	var bodies []string
	for _, comment := range issueState(gw, "tm-1").Comments {
		bodies = append(bodies, comment.Body)
	}
	return bodies
}

// Enter on a comment opens the editor on its text, and the save replaces that
// comment. The detail is read again, and the cursor is on the edited comment,
// which is the newest now.
func TestEnterOnACommentEditsItAndTheDetailIsReadAgain(t *testing.T) {
	m, gw := commentShell(t)
	m = pressKey(t, m, "down")

	m = openEditor(t, m, "enter")
	if m.editorTarget != (editorTarget{kind: issueeditmode.KindCommentEdit, issueID: "tm-1", commentID: "c-old"}) {
		t.Fatalf("the editor writes to %+v", m.editorTarget)
	}
	testui.AssertContainsAll(t, plainShell(m), "Edit comment on tm-1", "the older comment")

	m = typeInto(t, m, "edited: ")
	mark := gw.CallCount()
	m = pressKey(t, m, "ctrl+s")

	calls := gw.CallsFor(fakes.MethodEditComment)
	if len(calls) != 1 || calls[0].CommentID != "c-old" || calls[0].Args.(domain.EditCommentInput).Body != "edited: the older comment" {
		t.Fatalf("EditComment calls = %#v", calls)
	}
	if m.active != mode.Detail || !gw.HasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("after the save the shell is on %q; issue read again: %v", m.active, gw.HasCallSince(mark, fakes.MethodIssue))
	}
	comment, _ := m.detail.SelectedComment()
	if comment.Body != "edited: the older comment" || m.detail.FocusPane != uidetail.FocusPaneComments {
		t.Errorf("the cursor is on %q with focus %d, want the edited comment", comment.Body, m.detail.FocusPane)
	}
	testui.AssertContainsAll(t, plainShell(m), "[1/2] memory-user", "edited: the older comment", "Edited comment on tm-1")
}

// The delete question names the comment as Detail draws it, also a comment
// that has no time.
func TestDeleteQuestionWritesTheTimeAsDetailDoes(t *testing.T) {
	m, _ := commentShell(t)
	m.detail.Detail.Comments = []domain.IssueComment{{ID: "c-1", Author: "carol"}}
	m.detail.SelectComment(0)

	m = pressKey(t, m, "alt+d")
	testui.AssertContainsAll(t, plainShell(m), "Delete comment", "carol · unknown time")
}

// The delete key asks first. Yes deletes the selected comment and reads the
// detail again; no and Escape write nothing.
func TestDeleteKeyAsksAndYesDeletesTheSelectedComment(t *testing.T) {
	m, gw := commentShell(t)

	m = pressKey(t, m, "alt+d")
	if !m.showActionModal || len(writeCalls(gw)) != 0 {
		t.Fatalf("the delete key: question up %v, writes %v; want the question and no write", m.showActionModal, writeCalls(gw))
	}
	testui.AssertContainsAll(t, plainShell(m), "Delete comment", "bob · 2026-05-01", "Delete", "Cancel")

	mark := gw.CallCount()
	m = pressKey(t, m, "y")

	calls := gw.CallsFor(fakes.MethodDeleteComment)
	if len(calls) != 1 || calls[0].IssueID != "tm-1" || calls[0].CommentID != "c-new" {
		t.Fatalf("DeleteComment calls = %#v", calls)
	}
	if got := commentBodies(gw); len(got) != 1 || got[0] != "the older comment" {
		t.Errorf("the store holds the comments %v", got)
	}
	if m.showActionModal || m.commentDelete != (commentDelete{}) {
		t.Errorf("the question is still up after yes: %v, %+v", m.showActionModal, m.commentDelete)
	}
	if !gw.HasCallSince(mark, fakes.MethodIssue) || len(m.detail.Detail.Comments) != 1 {
		t.Errorf("Detail holds %d comments after the delete; issue read again: %v", len(m.detail.Detail.Comments), gw.HasCallSince(mark, fakes.MethodIssue))
	}
	if !strings.Contains(m.toast.View(), "Deleted comment on tm-1") {
		t.Errorf("toast = %q", m.toast.View())
	}
	if comment, _ := m.detail.SelectedComment(); comment.ID != "c-old" {
		t.Errorf("the cursor is on %q, want the comment that is left", comment.ID)
	}
}

func TestNoAndEscapeOnTheDeleteQuestionWriteNothing(t *testing.T) {
	for _, answer := range []string{"n", "esc"} {
		m, gw := commentShell(t)
		m = pressKey(t, pressKey(t, m, "alt+d"), answer)

		if writes := writeCalls(gw); len(writes) != 0 {
			t.Errorf("%q wrote %v", answer, writes)
		}
		if m.showActionModal || m.commentDelete != (commentDelete{}) || m.active != mode.Detail {
			t.Errorf("%q: question up %v, target %+v, shell on %q", answer, m.showActionModal, m.commentDelete, m.active)
		}
	}
}

// The delete question is asked with a Cmd. A key that asks for the editor can
// be handled before the question is up, and the editor then does not open
// under it.
func TestEditorAskedForBeforeTheDeleteQuestionIsUpDoesNotOpenUnderIt(t *testing.T) {
	m, _ := commentShell(t)

	next, ask := m.Update(testKey("alt+d"))
	next, load := next.(Model).Update(testKey("alt+u"))
	m = applyMessages(t, next.(Model), runBatch(ask))
	if !m.showActionModal || m.pendingEditor == 0 {
		t.Fatalf("setup: question up %v, pending open %d; want the question over a pending open", m.showActionModal, m.pendingEditor)
	}
	m = applyMessages(t, m, runBatch(load))

	if m.active != mode.Detail || !m.showActionModal || m.pendingEditor != 0 {
		t.Errorf("the shell is on %q, question up %v, pending open %d; want Detail under the question and no open pending", m.active, m.showActionModal, m.pendingEditor)
	}
}

func TestARefusedCommentDeleteSaysWhy(t *testing.T) {
	m, gw := commentShell(t)
	gw.SetError(fakes.MethodDeleteComment, domain.RepositoryError{Code: domain.ErrorCodeHookDenied, Operation: "delete comment", Message: "comments are kept"})

	m = pressKey(t, pressKey(t, m, "alt+d"), "y")

	if !strings.Contains(m.toast.View(), "comments are kept") {
		t.Errorf("toast = %q", m.toast.View())
	}
	if len(m.detail.Detail.Comments) != 2 {
		t.Errorf("Detail holds %d comments after a refused delete", len(m.detail.Detail.Comments))
	}
}

// On a watched store the watch reloads after a write, so the handler of the
// delete must not: the two together read the detail twice.
func TestCommentDeleteReloadsOnceOnAWatchedAndAnUnwatchedStore(t *testing.T) {
	for name, runtime := range map[string]RuntimeOptions{
		"watched store":   {},
		"unwatched store": {DisableAutoRefresh: true},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newWatchedRepository()
			repo.Memory.SeedComments("tm-1", memoryrepo.Comment{ID: "c-1", Author: "alice", Body: "to delete"})
			m, _ := watchedModel(t, repo, runtime)
			m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
			m = pressKey(t, pressKey(t, m, "enter"), "right")
			m = pressKey(t, m, "alt+d")
			if !m.showActionModal {
				t.Fatalf("setup: no delete question; the shell is on %q with focus %d", m.active, m.detail.FocusPane)
			}

			mark := repo.CallCount()
			m = pressKey(t, m, "y")
			if got := repo.CallCountSince(mark, fakes.MethodDeleteComment); got != 1 {
				t.Fatalf("setup: comment deletes = %d, want 1", got)
			}
			if m.storeWatched() {
				if got := repo.CallCountSince(mark, fakes.MethodIssue); got != 0 {
					t.Fatalf("the handler read the detail %d times on a watched store", got)
				}
				m = send(t, m, storeChangedMsg{})
			}

			if got := repo.CallCountSince(mark, fakes.MethodIssue); got != 1 {
				t.Errorf("detail reads for one comment delete = %d, want 1", got)
			}
			if len(m.detail.Detail.Comments) != 0 || m.detail.FocusPane != uidetail.FocusPaneContent {
				t.Errorf("Detail holds %d comments with focus %d after the reload", len(m.detail.Detail.Comments), m.detail.FocusPane)
			}
		})
	}
}

// A click on a pane button does what the key shown on it does.
func TestClicksOnTheDetailButtonsDoWhatTheirKeysDo(t *testing.T) {
	click := func(t *testing.T, m Model, text string) Model {
		t.Helper()
		x, y := testui.FindCell(t, plainShell(m), text)
		return send(t, m, leftClick(x, y))
	}

	t.Run("edit", func(t *testing.T) {
		m, _ := commentShell(t)
		m = click(t, m, "edit alt+u")
		if m.active != mode.IssueEdit || m.editorTarget != (editorTarget{kind: issueeditmode.KindEdit, issueID: "tm-1"}) {
			t.Fatalf("the shell is on %q with the editor on %+v", m.active, m.editorTarget)
		}
		if m = pressKey(t, m, "esc"); m.active != mode.Detail {
			t.Errorf("Escape left the editor for %q, want Detail", m.active)
		}
	})

	t.Run("add", func(t *testing.T) {
		m, gw := commentShell(t)
		m = click(t, m, "add alt+a")
		if m.active != mode.IssueEdit || m.editorTarget != (editorTarget{kind: issueeditmode.KindCommentAdd, issueID: "tm-1"}) {
			t.Fatalf("the shell is on %q with the editor on %+v", m.active, m.editorTarget)
		}
		m = pressKey(t, pressKey(t, m, "down"), "down")
		m = pressKey(t, typeInto(t, m, "a third comment"), "ctrl+s")

		// The new comment is the newest, and the cursor is on it.
		if comment, _ := m.detail.SelectedComment(); comment.Body != "a third comment" {
			t.Errorf("after the add the cursor is on %q; the store holds %v", comment.Body, commentBodies(gw))
		}
	})

	t.Run("edit comment", func(t *testing.T) {
		m, _ := commentShell(t)
		m = click(t, m, "edit enter")
		if m.active != mode.IssueEdit || m.editorTarget != (editorTarget{kind: issueeditmode.KindCommentEdit, issueID: "tm-1", commentID: "c-new"}) {
			t.Fatalf("the shell is on %q with the editor on %+v", m.active, m.editorTarget)
		}
	})

	t.Run("delete comment", func(t *testing.T) {
		m, gw := commentShell(t)
		m = click(t, m, "delete alt+d")
		if !m.showActionModal || m.commentDelete != (commentDelete{issueID: "tm-1", commentID: "c-new"}) || len(writeCalls(gw)) != 0 {
			t.Fatalf("question up %v on %+v, writes %v", m.showActionModal, m.commentDelete, writeCalls(gw))
		}
	})

	t.Run("a comment", func(t *testing.T) {
		m, _ := commentShell(t)
		m = click(t, m, "[2/2] alice")
		if comment, _ := m.detail.SelectedComment(); comment.ID != "c-old" {
			t.Errorf("a click on the older comment left the cursor on %q", comment.ID)
		}
	})
}

// The delete key of the shell still closes the issue with a comment selected:
// it opens the editor on the close reason and writes nothing by itself.
func TestTheCloseKeyOnASelectedCommentOpensTheEditorOnTheIssue(t *testing.T) {
	m, gw := commentShell(t)
	m = openEditor(t, m, "delete")
	if m.editorTarget.kind != issueeditmode.KindEdit || len(writeCalls(gw)) != 0 {
		t.Errorf("the close key opened %+v and wrote %v", m.editorTarget, writeCalls(gw))
	}
}
