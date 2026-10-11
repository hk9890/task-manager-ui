package detail

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uidetail "github.com/hk9890/task-manager-ui/internal/ui/detail"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	commentsWidth  = 100
	commentsHeight = 30
)

// commentsDetail is a loaded detail of an issue with count comments, each of
// three body lines. Comment 1 is the newest, so the pane draws it first.
func commentsDetail(t *testing.T, count int) *Model {
	t.Helper()
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	m := &Model{Keys: keys}
	m.selectionID = "tm-1"
	m.targetID = "tm-1"
	m.ApplyLoadedDetail("tm-1", issueWithComments(count))
	return m
}

func issueWithComments(count int) domain.IssueDetail {
	newest := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	issue := domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Issue A", Status: "open", Type: "task", Priority: 1},
		Description: "The description.",
	}
	for n := 1; n <= count; n++ {
		issue.Comments = append(issue.Comments, domain.IssueComment{
			ID:        fmt.Sprintf("c-%d", n),
			Author:    fmt.Sprintf("author-%d", n),
			Body:      fmt.Sprintf("body-%d first\n\nbody-%d last", n, n),
			CreatedAt: newest.Add(-time.Duration(n) * time.Hour),
		})
	}
	return issue
}

func key(t *testing.T, m *Model, msg tea.KeyMsg) tea.Cmd {
	t.Helper()
	consumed, cmd := m.HandleKey(msg, commentsWidth, commentsHeight)
	if !consumed {
		t.Fatalf("%q was not consumed", msg.String())
	}
	return cmd
}

func selectedCommentID(m *Model) string {
	comment, _ := m.SelectedComment()
	return comment.ID
}

// barLines is the lines of the view that start a pane line with the selection
// bar, without colour.
func barLines(m *Model) []string {
	bar, _ := styles.SelectionPrefix(true, false)
	var lines []string
	for _, line := range strings.Split(testui.AnsiEscapePattern.ReplaceAllString(m.View(commentsWidth, commentsHeight, false, 0), ""), "\n") {
		if strings.HasPrefix(line, "│"+bar) {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestLeftAndRightWalkThePanesWithTheCommentsAfterContent(t *testing.T) {
	t.Parallel()

	m := commentsDetail(t, 2)
	m.FocusPane = uidetail.FocusPaneDependencies
	walk := []uidetail.FocusPane{uidetail.FocusPaneContent, uidetail.FocusPaneComments, uidetail.FocusPaneComments}
	for step, want := range walk {
		key(t, m, tea.KeyMsg{Type: tea.KeyRight})
		if m.FocusPane != want {
			t.Fatalf("right, step %d: focus %d, want %d", step, m.FocusPane, want)
		}
	}
	back := []uidetail.FocusPane{uidetail.FocusPaneContent, uidetail.FocusPaneDependencies, uidetail.FocusPaneDependencies}
	for step, want := range back {
		key(t, m, tea.KeyMsg{Type: tea.KeyLeft})
		if m.FocusPane != want {
			t.Fatalf("left, step %d: focus %d, want %d", step, m.FocusPane, want)
		}
	}

	// An issue without a comment has no such stop: Content is the last one.
	m = commentsDetail(t, 0)
	key(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.FocusPane != uidetail.FocusPaneContent {
		t.Fatalf("right from Content without a comment: focus %d, want Content", m.FocusPane)
	}
}

// The scroll keys move the comment cursor, clamped, and home and end go to
// the first and the last comment. The page keys scroll the pane.
func TestTheScrollKeysMoveTheCommentCursor(t *testing.T) {
	t.Parallel()

	m := commentsDetail(t, 3)
	m.FocusPane = uidetail.FocusPaneComments

	steps := []struct {
		key  tea.KeyType
		want string
	}{
		{tea.KeyUp, "c-1"}, {tea.KeyDown, "c-2"}, {tea.KeyDown, "c-3"}, {tea.KeyDown, "c-3"},
		{tea.KeyHome, "c-1"}, {tea.KeyEnd, "c-3"}, {tea.KeyUp, "c-2"},
	}
	for step, tc := range steps {
		key(t, m, tea.KeyMsg{Type: tc.key})
		if got := selectedCommentID(m); got != tc.want {
			t.Fatalf("step %d: the cursor is on %q, want %q", step, got, tc.want)
		}
	}

	m = commentsDetail(t, 12)
	m.FocusPane = uidetail.FocusPaneComments
	key(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if selectedCommentID(m) != "c-1" || m.ContentScrollOffset == 0 {
		t.Fatalf("page down: cursor %q, offset %d; want the pane scrolled and the cursor where it was", selectedCommentID(m), m.ContentScrollOffset)
	}
}

// The pane scrolls with the cursor: the selected comment is drawn whole, with
// its bar on each line, wherever the cursor goes.
func TestThePaneScrollsToTheSelectedComment(t *testing.T) {
	t.Parallel()

	m := commentsDetail(t, 12)
	key(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.FocusPane != uidetail.FocusPaneComments {
		t.Fatalf("fixture did not reach the comments, focus %d", m.FocusPane)
	}

	drawnWhole := func(step string, n int) {
		t.Helper()
		lines := barLines(m)
		if len(lines) != 4 || !strings.Contains(lines[0], fmt.Sprintf("author-%d", n)) || !strings.Contains(lines[3], fmt.Sprintf("body-%d last", n)) {
			t.Fatalf("%s: comment %d is not drawn whole under the bar:\n%s\n\n%s", step, n, strings.Join(lines, "\n"), m.View(commentsWidth, commentsHeight, false, 0))
		}
	}
	drawnWhole("arriving on the comments", 1)
	for n := 2; n <= 12; n++ {
		key(t, m, tea.KeyMsg{Type: tea.KeyDown})
		drawnWhole("down", n)
	}
	for n := 11; n >= 1; n-- {
		key(t, m, tea.KeyMsg{Type: tea.KeyUp})
		drawnWhole("up", n)
	}
	key(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	drawnWhole("end", 12)
}

// Enter and the delete key act on a comment only while the comments have the
// focus. Elsewhere the delete key is left to the shell.
func TestEnterAndTheDeleteKeyAskTheShellOnlyWithTheCommentsFocused(t *testing.T) {
	t.Parallel()

	deleteKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true}

	m := commentsDetail(t, 2)
	m.FocusPane = uidetail.FocusPaneComments
	testui.AssertActionRequest(t, key(t, m, tea.KeyMsg{Type: tea.KeyEnter})(), mode.Detail, mode.ActionEditComment)
	testui.AssertActionRequest(t, key(t, m, deleteKey)(), mode.Detail, mode.ActionDeleteComment)

	for _, focus := range []uidetail.FocusPane{uidetail.FocusPaneContent, uidetail.FocusPaneDependencies} {
		m.FocusPane = focus
		if consumed, cmd := m.HandleKey(deleteKey, commentsWidth, commentsHeight); consumed || cmd != nil {
			t.Errorf("focus %d: the delete key was consumed: %v", focus, consumed)
		}
	}
	m.FocusPane = uidetail.FocusPaneContent
	if consumed, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, commentsWidth, commentsHeight); consumed || cmd != nil {
		t.Errorf("enter on the Content pane was consumed: %v", consumed)
	}
}

// A click on a button sends the request its key sends, and a click on a
// comment puts the cursor there.
func TestClicksOnTheButtonsAndOnAComment(t *testing.T) {
	t.Parallel()

	m := commentsDetail(t, 3)
	click := func(text string) tea.Cmd {
		t.Helper()
		x, y := testui.FindCell(t, m.View(commentsWidth, commentsHeight, false, 0), text)
		return m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: mouseStart}, commentsWidth, commentsHeight)
	}

	testui.AssertActionRequest(t, click("edit alt+u")(), mode.Detail, mode.ActionEditIssue)
	testui.AssertActionRequest(t, click("add alt+a")(), mode.Detail, mode.ActionAddComment)
	if m.FocusPane != uidetail.FocusPaneContent {
		t.Fatalf("a click on a button moved the focus to %d", m.FocusPane)
	}

	if cmd := click("[2/3] author-2"); cmd != nil {
		t.Fatalf("a click on a comment produced %#v", cmd())
	}
	if m.FocusPane != uidetail.FocusPaneComments || selectedCommentID(m) != "c-2" {
		t.Fatalf("a click on comment 2: focus %d, cursor %q", m.FocusPane, selectedCommentID(m))
	}
	// A second click on the same comment opens nothing.
	if cmd := click("[2/3] author-2"); cmd != nil {
		t.Fatalf("a second click on a comment produced %#v", cmd())
	}

	testui.AssertActionRequest(t, click("edit enter")(), mode.Detail, mode.ActionEditComment)
	testui.AssertActionRequest(t, click("delete alt+d")(), mode.Detail, mode.ActionDeleteComment)
	if selectedCommentID(m) != "c-2" {
		t.Fatalf("a click on a comment button moved the cursor to %q", selectedCommentID(m))
	}
}

// A click that selects a comment draws its buttons, one of them under the
// pointer. The second click of a double click there presses nothing.
func TestASecondClickInPlaceOnACommentPressesNoButtonDrawnUnderIt(t *testing.T) {
	t.Parallel()

	m := commentsDetail(t, 3)
	m.FocusPane = uidetail.FocusPaneComments
	m.SelectComment(1)
	x, y := testui.FindCell(t, m.View(commentsWidth, commentsHeight, false, 0), "delete alt+d")
	m.SelectComment(0)

	clickAt := func(ms int) tea.Cmd {
		at := mouseStart.Add(time.Duration(ms) * time.Millisecond)
		return m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: at}, commentsWidth, commentsHeight)
	}

	if cmd := clickAt(0); cmd != nil || selectedCommentID(m) != "c-2" {
		t.Fatalf("the first click: command %v, cursor %q; want comment 2 selected", cmd != nil, selectedCommentID(m))
	}
	if cmd := clickAt(100); cmd != nil {
		t.Fatalf("the second click of a double click produced %#v", cmd())
	}
	testui.AssertActionRequest(t, clickAt(1000)(), mode.Detail, mode.ActionDeleteComment)
}

func TestThePointerLightsAPaneButton(t *testing.T) {
	testui.ForceTrueColor(t)

	m := commentsDetail(t, 1)
	idle := m.View(commentsWidth, commentsHeight, false, 0)
	x, y := testui.FindCell(t, idle, "add alt+a")
	m.HandleMouse(mode.MouseMsg{Kind: mode.MouseMove, X: x, Y: y, At: mouseStart}, commentsWidth, commentsHeight)
	if m.View(commentsWidth, commentsHeight, false, 0) == idle {
		t.Fatal("the button under the pointer is drawn as at rest")
	}
	m.HandleMouse(mode.MouseMsg{Kind: mode.MouseLeave}, commentsWidth, commentsHeight)
	if m.View(commentsWidth, commentsHeight, false, 0) != idle {
		t.Fatal("the button stayed lit after the pointer left")
	}
}

// A reload keeps the cursor inside the comments that are left, another issue
// starts on the first comment, and the focus leaves comments that are gone.
func TestALoadedDetailClampsTheCommentCursor(t *testing.T) {
	t.Parallel()

	m := commentsDetail(t, 3)
	m.FocusPane = uidetail.FocusPaneComments
	m.SelectComment(2)

	m.ApplyLoadedDetail("tm-1", issueWithComments(3))
	if selectedCommentID(m) != "c-3" {
		t.Fatalf("a reload of the same comments moved the cursor to %q", selectedCommentID(m))
	}

	m.ApplyLoadedDetail("tm-1", issueWithComments(2))
	if selectedCommentID(m) != "c-2" || m.FocusPane != uidetail.FocusPaneComments {
		t.Fatalf("after a comment was deleted: cursor %q, focus %d", selectedCommentID(m), m.FocusPane)
	}

	other := issueWithComments(2)
	other.Summary.ID = "tm-9"
	m.ApplyLoadedDetail("tm-9", other)
	if selectedCommentID(m) != "c-1" {
		t.Fatalf("another issue starts with the cursor on %q, want its first comment", selectedCommentID(m))
	}

	m.ApplyLoadedDetail("tm-9", domain.IssueDetail{Summary: other.Summary})
	if _, ok := m.SelectedComment(); ok || m.FocusPane != uidetail.FocusPaneContent {
		t.Fatalf("with no comment left: selected %v, focus %d; want none and Content", ok, m.FocusPane)
	}
}

// A comment another process wrote moves the others to another index. The
// cursor stays on its comment, and after SelectComment on the index the shell
// gave it.
func TestAReloadKeepsTheCursorOnItsComment(t *testing.T) {
	t.Parallel()

	withANewerComment := func() domain.IssueDetail {
		issue := issueWithComments(3)
		issue.Comments = append(issue.Comments, domain.IssueComment{
			ID: "c-0", Author: "agent", Body: "written meanwhile", CreatedAt: time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC),
		})
		return issue
	}

	m := commentsDetail(t, 3)
	m.FocusPane = uidetail.FocusPaneComments
	key(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m.ApplyLoadedDetail("tm-1", withANewerComment())
	m.ClampScroll(commentsWidth, commentsHeight)
	if got := selectedCommentID(m); got != "c-2" {
		t.Fatalf("a reload with a newer comment moved the cursor to %q, want c-2", got)
	}
	if lines := barLines(m); len(lines) != 4 || !strings.Contains(lines[0], "author-2") {
		t.Fatalf("the comment under the cursor is not drawn whole after the reload:\n%s", m.View(commentsWidth, commentsHeight, false, 0))
	}

	m = commentsDetail(t, 3)
	m.SelectComment(0)
	m.ApplyLoadedDetail("tm-1", withANewerComment())
	if got := selectedCommentID(m); got != "c-0" {
		t.Fatalf("after SelectComment(0) a reload put the cursor on %q, want the newest comment", got)
	}
}

// SelectComment is followed by a reload: the next ClampScroll scrolls to the
// comment, once.
func TestSelectCommentIsScrolledToByTheNextClampScroll(t *testing.T) {
	t.Parallel()

	m := commentsDetail(t, 12)
	m.FocusPane = uidetail.FocusPaneComments
	key(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	scrolled := m.ContentScrollOffset

	m.SelectComment(0)
	m.ClampScroll(commentsWidth, commentsHeight)
	if lines := barLines(m); len(lines) != 4 || !strings.Contains(lines[0], "author-1") {
		t.Fatalf("the first comment is not drawn after SelectComment(0):\n%s", m.View(commentsWidth, commentsHeight, false, 0))
	}

	m.ContentScrollOffset = scrolled
	m.ClampScroll(commentsWidth, commentsHeight)
	if m.ContentScrollOffset != scrolled {
		t.Fatalf("a later ClampScroll moved the pane from %d to %d", scrolled, m.ContentScrollOffset)
	}
}

// The Metadata pane only shows: no key and no click gives it the focus, and
// the wheel does not scroll it.
func TestTheMetadataPaneNeverTakesTheFocus(t *testing.T) {
	t.Parallel()

	// The screen of a 120x30 terminal, inside the margin.
	const width, height = 116, 28
	bar, _ := styles.SelectionPrefix(true, false)
	metadataHasNoCursor := func(m *Model) {
		t.Helper()
		view := testui.AnsiEscapePattern.ReplaceAllString(m.View(width, height, false, 0), "")
		for _, field := range []string{"Status", "Priority"} {
			if strings.Contains(view, bar+field) {
				t.Fatalf("the Metadata pane draws a cursor on %s:\n%s", field, view)
			}
		}
	}

	m := commentsDetail(t, 2)
	for step, want := range []uidetail.FocusPane{uidetail.FocusPaneComments, uidetail.FocusPaneComments} {
		if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyRight}, width, height); !consumed || m.FocusPane != want {
			t.Fatalf("right, step %d: focus %d, want %d", step, m.FocusPane, want)
		}
		metadataHasNoCursor(m)
	}
	_, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, width, height)
	testui.AssertActionRequest(t, cmd(), mode.Detail, mode.ActionEditComment)

	m = commentsDetail(t, 0)
	m.HandleKey(tea.KeyMsg{Type: tea.KeyRight}, width, height)
	metadataHasNoCursor(m)
	if consumed, cmd := m.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}, width, height); consumed || cmd != nil {
		t.Fatalf("enter after right from Content was consumed: %v", consumed)
	}

	m.FocusPane = uidetail.FocusPaneDependencies
	for _, kind := range []mode.MouseKind{mode.MouseClick, mode.MouseWheelDown} {
		x, y := testui.FindCell(t, m.View(width, height, false, 0), "Priority")
		if cmd := m.HandleMouse(mode.MouseMsg{Kind: kind, X: x, Y: y, At: mouseStart}, width, height); cmd != nil {
			t.Fatalf("mouse event %d on the Metadata pane produced %#v", kind, cmd())
		}
		if m.FocusPane != uidetail.FocusPaneDependencies || m.ContentScrollOffset != 0 {
			t.Fatalf("mouse event %d on the Metadata pane: focus %d, content offset %d", kind, m.FocusPane, m.ContentScrollOffset)
		}
	}
}
