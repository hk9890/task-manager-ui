package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// selectTwoTitles drags from the first in-progress title to the second and
// releases, so the menu is up over a box that holds both.
func selectTwoTitles(t *testing.T) (Model, *[]string) {
	t.Helper()
	m, copied := newSelectingShell(t)
	return dragTwoTitles(t, m), copied
}

// dragTwoTitles is the drag and release of selectTwoTitles on m.
func dragTwoTitles(t *testing.T, m Model) Model {
	t.Helper()
	m = send(t, m, testui.Press("right"))
	x, y := testui.FindCell(t, testui.View(m), "progress-first")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	endX, endY := testui.FindCell(t, testui.View(m), "progress-second")
	endX += len("progress-second") - 1
	m = send(t, m, testui.Drag(endX, endY))
	m = send(t, m, testui.Release(endX, endY))
	if m.selMenu == nil {
		t.Fatal("the release of a selection opened no menu")
	}
	return m
}

// TestReleaseCopiesAndOpensTheMenuBesideThePointer: the copy does not wait
// for the menu, and the menu stands below and right of the release cell.
func TestReleaseCopiesAndOpensTheMenuBesideThePointer(t *testing.T) {
	m, copied := selectTwoTitles(t)
	if len(*copied) != 1 {
		t.Fatalf("release copied %d times, want once before any entry is chosen", len(*copied))
	}
	view := testui.View(m)
	_, endY := testui.FindCell(t, view, "progress-second")
	_, titleY := testui.FindCell(t, view, "Sent ")
	if titleY != endY+2 {
		t.Fatalf("the menu title is on row %d, want %d: the row under the release, below the border", titleY, endY+2)
	}
	if !strings.Contains(view, "\x1b[0;7m") {
		t.Fatalf("the box left the screen when the menu opened:\n%s", view)
	}
}

func TestQuoteInACommentOpensTheCommentEditorWithTheTextQuoted(t *testing.T) {
	m, _ := selectTwoTitles(t)
	x, y := testui.FindCell(t, testui.View(m), "Quote in a comment on tm-4")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	if m.selMenu != nil || m.active != mode.IssueEdit || m.editorTarget.issueID != "tm-4" {
		t.Fatalf("menu %v, active %v on %q: want the comment editor on tm-4", m.selMenu != nil, m.active, m.editorTarget.issueID)
	}
	view := testui.AnsiEscapePattern.ReplaceAllString(testui.View(m), "")
	if !strings.Contains(view, "> progress-first") || !strings.Contains(view, "> progress-second") {
		t.Fatalf("the comment does not quote the selection:\n%s", view)
	}
}

// TestTheQuoteGoesToTheIssueItsLabelNames: a refresh that moves the selection
// under the menu does not move the quote to another issue.
func TestTheQuoteGoesToTheIssueItsLabelNames(t *testing.T) {
	m, _ := selectTwoTitles(t)
	m = send(t, m, mode.SelectionChangedMsg{Mode: mode.Board, Selection: &mode.Selection{Issue: domain.IssueSummary{ID: "tm-2"}}})
	x, y := testui.FindCell(t, testui.View(m), "Quote in a comment on tm-4")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	if m.active != mode.IssueEdit || m.editorTarget.issueID != "tm-4" {
		t.Fatalf("active %v on %q: want the comment editor on tm-4, the issue the label names", m.active, m.editorTarget.issueID)
	}
}

// TestTheStartupErrorIsDrawnOverTheMenu: with the startup error up the shell
// takes no key and no click but quit, so a menu over it could not be closed.
func TestTheStartupErrorIsDrawnOverTheMenu(t *testing.T) {
	m, _ := selectTwoTitles(t)
	m.fatalErrTitle = "no task-manager store here"
	if view := testui.View(m); !strings.Contains(view, "no task-manager store here") {
		t.Fatalf("the menu hides the startup error:\n%s", view)
	}
}

// TestSearchTheStoreSearchesOnceForTheTextOnOneLine: the first open of the
// store search runs one search, and it is for the selected text.
func TestSearchTheStoreSearchesOnceForTheTextOnOneLine(t *testing.T) {
	m, gw := newMouseShellOn(t)
	var copied []string
	m.copyText = func(text string) tea.Cmd {
		copied = append(copied, text)
		return nil
	}
	m = dragTwoTitles(t, m)
	before := len(gw.CallsFor(fakes.MethodSearch))
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")
	if m.selMenu != nil || m.active != mode.Search {
		t.Fatalf("active %v: want the store search", m.active)
	}
	want := strings.Join(strings.Fields(copied[0]), " ")
	calls := gw.CallsFor(fakes.MethodSearch)[before:]
	if len(calls) != 1 || calls[0].Args.(domain.SearchIssuesQuery).Text != want {
		t.Fatalf("searched %#v, want one search for %q", calls, want)
	}
}

// TestAQuoteSavedAsItIsKeepsNoTrailingLines: the quote opens with an empty
// line under it for the cursor, and a save with nothing typed drops it.
func TestAQuoteSavedAsItIsKeepsNoTrailingLines(t *testing.T) {
	m, gw := newMouseShellOn(t)
	m.copyText = func(string) tea.Cmd { return nil }
	m = dragTwoTitles(t, m)
	x, y := testui.FindCell(t, testui.View(m), "Quote in a comment on tm-4")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	m = pressKey(t, m, "ctrl+s")
	calls := gw.CallsFor(fakes.MethodAddComment)
	if len(calls) != 1 {
		t.Fatalf("AddComment calls = %#v", calls)
	}
	if body := calls[0].Args.(domain.AddCommentInput).Body; !strings.HasPrefix(body, "> progress-first") || strings.HasSuffix(body, "\n") {
		t.Fatalf("the saved quote is %q: want the quote, with no line break at its end", body)
	}
}

// TestAToastIsDrawnOverTheMenu:the toast's timer runs while the menu is up,
// so the toast must be seen there.
func TestAToastIsDrawnOverTheMenu(t *testing.T) {
	m, _ := selectTwoTitles(t)
	_ = m.showToast("Launch failed: no such tool", toaster.StyleError)
	if view := testui.View(m); !strings.Contains(view, "Launch failed") {
		t.Fatalf("the toast is hidden under the menu:\n%s", view)
	}
}

// TestAResizeEndsADragWithoutACopy: the box stands over a screen of the old
// size, so a resize drops it and the release copies nothing.
func TestAResizeEndsADragWithoutACopy(t *testing.T) {
	m, copied := newSelectingShell(t)
	m = send(t, m, testui.Press("right"))
	x, y := testui.FindCell(t, testui.View(m), "progress-first")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	m = send(t, m, testui.Drag(x+10, y+2))
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	if m.sel.active || m.press != nil {
		t.Fatal("the drag survived the resize")
	}
	if m = send(t, m, testui.Release(x+10, y+2)); len(*copied) != 0 || m.selMenu != nil {
		t.Fatalf("the release after a resize copied %q and opened a menu: %v", *copied, m.selMenu != nil)
	}
}

func TestQuotableDropsTheScreenChromeAndKeepsTheIndent(t *testing.T) {
	cases := []struct {
		name, box string
		want      []string
	}{
		{
			name: "pane border, gutter and padding",
			box:  "│▌ first line   │\n│▌   indented   │\n│▌ last         │",
			want: []string{"first line", "  indented", "last"},
		},
		{
			name: "border rows above and below",
			box:  "╭──────╮\n│ text │\n╰──────╯",
			want: []string{"text"},
		},
		{
			name: "text with no chrome",
			box:  "plain",
			want: []string{"plain"},
		},
	}
	for _, tc := range cases {
		if got := quotable(tc.box); strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%s: quotable gives %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestTheMenuHoldsTheScreenUntilItCloses: a key the board takes is dropped,
// Escape closes the menu, and a click outside closes it without selecting
// the row under the click.
func TestTheMenuHoldsTheScreenUntilItCloses(t *testing.T) {
	m, _ := selectTwoTitles(t)
	selected := m.currentSelection().Issue.ID
	// left would focus the column of ready-first and select it.
	if m = pressKey(t, m, "left"); m.selMenu == nil || m.currentSelection().Issue.ID != selected {
		t.Fatal("a key reached the board under the menu")
	}
	if m = pressKey(t, m, "esc"); m.selMenu != nil {
		t.Fatal("escape did not close the menu")
	}

	m, _ = selectTwoTitles(t)
	x, y := testui.FindCell(t, testui.View(m), "ready-first")
	if m = send(t, m, testui.Click(x, y, tea.MouseLeft)); m.selMenu != nil {
		t.Fatal("a click outside did not close the menu")
	}
	if got := m.currentSelection().Issue.ID; got != selected {
		t.Fatalf("the click that closed the menu selected %q, want %q kept", got, selected)
	}
}
