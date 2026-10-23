package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// selectTwoTitles drags from the first in-progress title to the second and
// releases, so the menu is up over a box that holds both.
func selectTwoTitles(t *testing.T) (Model, *[]string) {
	t.Helper()
	m, copied := newSelectingShell(t)
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
	return m, copied
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

func TestSearchTheStoreOpensTheSearchWithTheTextAsItsQuery(t *testing.T) {
	m, copied := selectTwoTitles(t)
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")
	if m.selMenu != nil || m.active != mode.Search {
		t.Fatalf("active %v: want the store search", m.active)
	}
	// The lines of the box joined by a space are drawn nowhere but in the
	// query.
	if query := strings.Join(strings.Fields((*copied)[0]), " "); !strings.Contains(testui.View(m), query[:30]) {
		t.Fatalf("the search query is not the selection on one line, %q:\n%s", query, testui.View(m))
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
