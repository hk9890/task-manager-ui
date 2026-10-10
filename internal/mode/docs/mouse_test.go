package docs

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

var mouseStart = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

func mouseDocs(t *testing.T) *Model {
	t.Helper()
	m := NewModel(context.Background(), memoryrepo.New(fakes.FrozenClock()), nil)
	m.now = func() time.Time { return mouseStart }
	m.issues = []domain.IssueSummary{
		{ID: "tm-1", Title: "doc-one", Type: "doc", Status: "open"},
		{ID: "tm-2", Title: "doc-two", Type: "doc", Status: "open"},
		{ID: "tm-3", Title: "doc-three", Type: "doc", Status: "open"},
	}
	m.shown = m.issues
	m.total = len(m.issues)
	m.SetSize(100, 24)
	return m
}

func mouseAt(t *testing.T, m *Model, kind mode.MouseKind, text string, ms int) mode.MouseMsg {
	t.Helper()
	x, y := testui.FindCell(t, m.View(0), text)
	return mode.MouseMsg{Kind: kind, X: x, Y: y, At: mouseStart.Add(time.Duration(ms) * time.Millisecond)}
}

func opensDetail(cmd tea.Cmd) bool {
	return openedIssueID(cmd) != ""
}

// openedIssueID is the issue cmd asks the shell to open the detail of, or ""
// for none.
func openedIssueID(cmd tea.Cmd) string {
	for _, msg := range testui.DrainCmd(cmd) {
		if request, ok := msg.(mode.ActionRequestMsg); ok && request.Mode == mode.Docs && request.Action == mode.ActionOpenDetail && request.Selection != nil {
			return request.Selection.Issue.ID
		}
	}
	return ""
}

func TestMouseSelectsOpensAndScrollsTheDocList(t *testing.T) {
	t.Parallel()

	m := mouseDocs(t)

	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "doc-three", 0))
	if m.list.SelectedRow != 2 || cmd == nil || opensDetail(cmd) {
		t.Fatalf("first click: selected row %d, cmd %v; want row 2 selected and Detail not opened", m.list.SelectedRow, cmd != nil)
	}
	if got := openedIssueID(m.Update(mouseAt(t, m, mode.MouseClick, "doc-three", 200))); got != "tm-3" {
		t.Fatalf("a second click on the same doc opened %q, want Detail of tm-3", got)
	}

	if cmd = m.Update(mouseAt(t, m, mode.MouseWheelUp, "doc-one", 1000)); m.list.SelectedRow != 1 || cmd == nil {
		t.Fatalf("wheel up left the selection on row %d, want row 1", m.list.SelectedRow)
	}
	_ = m.Update(mouseAt(t, m, mode.MouseWheelUp, "doc-one", 1010))
	if cmd = m.Update(mouseAt(t, m, mode.MouseWheelUp, "doc-one", 1020)); m.list.SelectedRow != 0 || cmd != nil {
		t.Fatalf("a notch past the first row moved to row %d or reported a change", m.list.SelectedRow)
	}

	// The column title is not a row.
	x, y := testui.FindCell(t, m.View(0), "Docs ─")
	if cmd = m.Update(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: mouseStart}); cmd != nil || m.list.SelectedRow != 0 {
		t.Fatal("a click on the column title changed the selection")
	}
}

// TestClickOnEitherLineOfADocSelectsAndOpensIt: a doc is two lines tall, and
// the line under the title is as much the row as the title is. A click on one
// and a second on the other are a double click on the same doc.
func TestClickOnEitherLineOfADocSelectsAndOpensIt(t *testing.T) {
	t.Parallel()

	m := mouseDocs(t)
	_, titleY := testui.FindCell(t, m.View(0), "doc-three")
	if _, idY := testui.FindCell(t, m.View(0), "tm-3"); idY != titleY+1 {
		t.Fatalf("setup: the ID is on line %d, want it directly under the title on line %d", idY, titleY)
	}

	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "tm-3", 0))
	if m.list.SelectedRow != 2 || cmd == nil || opensDetail(cmd) {
		t.Fatalf("a click on the second line: selected row %d, cmd %v; want row 2 selected and Detail not opened", m.list.SelectedRow, cmd != nil)
	}
	assertSelectionDrawn(t, m)
	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "doc-three", 200)); !opensDetail(cmd) {
		t.Fatal("a second click, on the other line of the same doc, did not open Detail")
	}

	// The second line of the last doc is its last line: the one below is empty.
	x, y := testui.FindCell(t, m.View(0), "tm-3")
	cmd = m.Update(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y + 1, At: mouseStart.Add(5 * time.Second)})
	if cmd != nil || m.list.SelectedRow != 2 {
		t.Fatalf("a click below the last doc changed the selection (row %d, cmd %v)", m.list.SelectedRow, cmd != nil)
	}
}

func TestHoverFollowsThePointerAndClearsWhenItLeaves(t *testing.T) {
	testui.ForceTrueColor(t)

	m := mouseDocs(t)
	idle := m.View(0)

	_ = m.Update(mouseAt(t, m, mode.MouseMove, "doc-two", 0))
	if m.list.SelectedRow != 0 {
		t.Fatal("moving the pointer changed the selection")
	}
	if hovered := m.View(0); hovered == idle {
		t.Fatal("the row under the pointer was not marked")
	}

	_ = m.Update(mode.MouseMsg{Kind: mode.MouseLeave})
	if m.View(0) != idle {
		t.Fatal("the hover band stayed after the pointer left")
	}
}
