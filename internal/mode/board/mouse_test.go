package board

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
)

var mouseStart = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

// mouseAt is a mouse event on the cell where text is drawn, ms after the start.
func mouseAt(t *testing.T, m *Model, kind mode.MouseKind, text string, ms int) mode.MouseMsg {
	t.Helper()
	x, y := testui.FindCell(t, m.View(0), text)
	return mode.MouseMsg{Kind: kind, X: x, Y: y, At: mouseStart.Add(time.Duration(ms) * time.Millisecond)}
}

func mouseBoard(t *testing.T) *Model {
	t.Helper()
	m := newBoardModel(memoryrepo.New(fakes.FrozenClock()), resolvedBoardKeys(t))
	m.columns = []columnData{
		{title: sectionTitleReady, issues: []domain.IssueSummary{
			{ID: "tm-1", Title: "ready-one", Status: "open", Type: "task"},
			{ID: "tm-2", Title: "ready-two", Status: "open", Type: "task"},
		}, total: 2, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{
			{ID: "tm-7", Title: "progress-one", Status: "in_progress", Type: "task"},
			{ID: "tm-8", Title: "progress-two", Status: "in_progress", Type: "bug"},
			{ID: "tm-9", Title: "progress-three", Status: "in_progress", Type: "bug"},
		}, total: 3, exact: true},
	}
	m.SetSize(100, 24)
	return m
}

// selectionFrom is the selection the messages of cmd report, or "" for none.
func selectionFrom(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	for _, msg := range testui.DrainCmd(cmd) {
		if changed, ok := msg.(mode.SelectionChangedMsg); ok {
			if changed.Mode != mode.Board {
				t.Fatalf("selection reported for mode %q, want board", changed.Mode)
			}
			if changed.Selection == nil {
				return ""
			}
			return changed.Selection.Issue.ID
		}
	}
	return ""
}

func opensDetail(cmd tea.Cmd) bool {
	for _, msg := range testui.DrainCmd(cmd) {
		if request, ok := msg.(mode.ActionRequestMsg); ok && request.Mode == mode.Board && request.Action == mode.ActionOpenDetail {
			return true
		}
	}
	return false
}

// TestClickSelectsAndASecondClickOpens is the click contract: one click moves
// the focus and the selection to the row, a second on the same row inside the
// double-click window opens it, and anything else is a fresh single click.
func TestClickSelectsAndASecondClickOpens(t *testing.T) {
	t.Parallel()

	m := mouseBoard(t)

	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "progress-two", 0))
	if got := selectionFrom(t, cmd); got != "tm-8" {
		t.Fatalf("first click reported selection %q, want tm-8", got)
	}
	if m.focusedColumn != 1 || m.selectedRow[1] != 1 {
		t.Fatalf("first click left focus on column %d row %d, want column 1 row 1", m.focusedColumn, m.selectedRow[1])
	}
	if opensDetail(cmd) {
		t.Fatal("a single click opened Detail")
	}

	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "progress-two", 200)); !opensDetail(cmd) {
		t.Fatal("a second click on the same row inside the window did not open Detail")
	}

	// The double click was consumed: the next pair starts over, and a pair too
	// far apart is two single clicks.
	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "progress-two", 300)); opensDetail(cmd) {
		t.Fatal("a third click opened Detail again")
	}
	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "progress-two", 900)); opensDetail(cmd) {
		t.Fatal("a click outside the double-click window opened Detail")
	}

	// A second click on another row selects that row instead.
	_ = m.Update(mouseAt(t, m, mode.MouseClick, "ready-one", 2000))
	cmd = m.Update(mouseAt(t, m, mode.MouseClick, "ready-two", 2100))
	if opensDetail(cmd) {
		t.Fatal("clicks on two different rows opened Detail")
	}
	if got := selectionFrom(t, cmd); got != "tm-2" {
		t.Fatalf("click on another row reported selection %q, want tm-2", got)
	}
}

// TestClickOnEitherLineOfARowSelectsAndOpensIt: an issue is two lines tall, and
// the line under the title is as much the row as the title is. A click on one
// and a second on the other are a double click on the same row.
func TestClickOnEitherLineOfARowSelectsAndOpensIt(t *testing.T) {
	t.Parallel()

	m := mouseBoard(t)
	_, titleY := testui.FindCell(t, m.View(0), "progress-three")
	if _, idY := testui.FindCell(t, m.View(0), "tm-9"); idY != titleY+1 {
		t.Fatalf("setup: the ID is on line %d, want it directly under the title on line %d", idY, titleY)
	}

	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "tm-9", 0))
	if got := selectionFrom(t, cmd); got != "tm-9" {
		t.Fatalf("a click on the second line of a row reported selection %q, want tm-9", got)
	}
	if m.focusedColumn != 1 || m.selectedRow[1] != 2 {
		t.Fatalf("the click left focus on column %d row %d, want column 1 row 2", m.focusedColumn, m.selectedRow[1])
	}
	assertSelectionDrawn(t, m)

	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "progress-three", 200)); !opensDetail(cmd) {
		t.Fatal("a second click, on the other line of the same row, did not open Detail")
	}

	// The second line of the last row is its last line: the one below is empty.
	x, y := testui.FindCell(t, m.View(0), "tm-9")
	cmd = m.Update(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y + 1, At: mouseStart.Add(5 * time.Second)})
	if cmd != nil || m.selectedRow[1] != 2 {
		t.Fatalf("a click below the last row changed the selection (row %d, cmd %v)", m.selectedRow[1], cmd != nil)
	}
}

// TestAWheelNotchBetweenTwoClicksIsNotADoubleClick: a double click opens the
// selection, which the wheel has moved off the row that was clicked.
func TestAWheelNotchBetweenTwoClicksIsNotADoubleClick(t *testing.T) {
	t.Parallel()

	m := mouseBoard(t)
	_ = m.Update(mouseAt(t, m, mode.MouseClick, "progress-one", 0))
	_ = m.Update(mouseAt(t, m, mode.MouseWheelDown, "progress-one", 50))

	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "progress-one", 100))
	if opensDetail(cmd) {
		t.Fatal("a click after a wheel notch opened the row the wheel moved to")
	}
	if got := selectionFrom(t, cmd); got != "tm-7" {
		t.Fatalf("the click selected %q, want the clicked row tm-7", got)
	}
}

// TestClickOffTheRowsSelectsNothing covers the cells of a column that hold no
// issue, and the space outside the columns.
func TestClickOffTheRowsSelectsNothing(t *testing.T) {
	t.Parallel()

	m := mouseBoard(t)
	x, y := testui.FindCell(t, m.View(0), "progress-three")
	_, titleY := testui.FindCell(t, m.View(0), sectionTitleInProgress)

	for name, cell := range map[string][2]int{
		"column title":       {x, titleY},
		"below the last row": {x, y + issuerow.Height},
		"dashboard title":    {x, 0},
	} {
		cmd := m.Update(mode.MouseMsg{Kind: mode.MouseClick, X: cell[0], Y: cell[1], At: mouseStart})
		if cmd != nil || m.focusedColumn != 0 || m.selectedRow[0] != 0 {
			t.Errorf("click on the %s changed the selection (column %d, cmd %v)", name, m.focusedColumn, cmd != nil)
		}
	}
}

// TestWheelMovesTheSelectionOfTheFocusedColumn moves one row a notch and stops
// at the ends. The focus stays where a click or a key put it, whichever column
// the pointer is over.
func TestWheelMovesTheSelectionOfTheFocusedColumn(t *testing.T) {
	t.Parallel()

	m := mouseBoard(t)
	_ = m.Update(mouseAt(t, m, mode.MouseClick, "progress-one", 0))

	cmd := m.Update(mouseAt(t, m, mode.MouseWheelDown, "progress-one", 1000))
	if got := selectionFrom(t, cmd); got != "tm-8" {
		t.Fatalf("a notch down selected %q, want tm-8", got)
	}
	cmd = m.Update(mouseAt(t, m, mode.MouseWheelDown, "ready-one", 1010))
	if got := selectionFrom(t, cmd); got != "tm-9" || m.focusedColumn != 1 {
		t.Fatalf("a notch with the pointer over Ready selected %q in column %d, want tm-9 in the focused column 1", got, m.focusedColumn)
	}
	if cmd = m.Update(mouseAt(t, m, mode.MouseWheelDown, "progress-one", 1020)); cmd != nil {
		t.Fatal("a notch past the last row reported a selection change")
	}
	cmd = m.Update(mouseAt(t, m, mode.MouseWheelUp, "progress-one", 1030))
	if got := selectionFrom(t, cmd); got != "tm-8" {
		t.Fatalf("a notch up selected %q, want tm-8", got)
	}
}

// TestHoverFollowsThePointerAndClearsWhenItLeaves checks the hover band is
// drawn on the row under the pointer and nowhere once the pointer is gone. The
// selection sits in the other column, so the band there is the hover's, not the selection's.
func TestHoverFollowsThePointerAndClearsWhenItLeaves(t *testing.T) {
	testui.ForceTrueColor(t)

	m := mouseBoard(t)
	idle := m.View(0)

	if cmd := m.Update(mouseAt(t, m, mode.MouseMove, "progress-three", 0)); cmd != nil {
		t.Fatal("moving the pointer returned a command")
	}
	if m.focusedColumn != 0 || m.selectedRow[0] != 0 {
		t.Fatal("moving the pointer changed the selection")
	}
	hovered := m.View(0)
	band, selected := testui.RowBand(t, hovered, "progress-three"), testui.RowBand(t, hovered, "ready-one")
	if band == "" || band == selected {
		t.Fatalf("the row under the pointer carries the band %q, want one that is not the selection's %q", band, selected)
	}

	_ = m.Update(mode.MouseMsg{Kind: mode.MouseLeave})
	if m.View(0) != idle {
		t.Fatal("the hover band stayed after the pointer left")
	}
}
