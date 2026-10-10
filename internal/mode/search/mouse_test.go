package search

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
)

var mouseStart = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

func mouseSearch(t *testing.T) *Model {
	t.Helper()
	gw := fakes.NewTracked()
	seedStore(gw)
	return openedModel(t, gw)
}

func mouseAt(t *testing.T, m *Model, kind mode.MouseKind, text string, ms int) mode.MouseMsg {
	t.Helper()
	x, y := testui.FindCell(t, m.View(0), text)
	return mode.MouseMsg{Kind: kind, X: x, Y: y, At: mouseStart.Add(time.Duration(ms) * time.Millisecond)}
}

func opensDetail(cmd tea.Cmd) bool {
	for _, msg := range testui.DrainCmd(cmd) {
		if request, ok := msg.(mode.ActionRequestMsg); ok && request.Mode == mode.Search && request.Action == mode.ActionOpenDetail {
			return true
		}
	}
	return false
}

// TestHitTestReportsTheResultTheRendererDrewThere asks the renderer where it
// drew each result and the hit test what is there, on both lines of the row.
func TestHitTestReportsTheResultTheRendererDrewThere(t *testing.T) {
	t.Parallel()

	m := mouseSearch(t)
	view := m.View(0)
	for row, issue := range m.issues {
		x, y := testui.FindCell(t, view, issue.Title)
		for line := 0; line < 2; line++ {
			hit, ok := uiboard.HitTest(m.viewState(0), x, y+line)
			if !ok || hit.Row != row {
				t.Errorf("line %d of %q: the hit test reports row %d (ok %v), want %d", line, issue.Title, hit.Row, ok, row)
			}
		}
	}

	// The query line is no result.
	x, y := testui.FindCell(t, view, "search the store")
	if hit, ok := uiboard.HitTest(m.viewState(0), x, y); ok {
		t.Errorf("the query line reports a hit: %#v", hit)
	}
}

func TestMouseSelectsOpensAndScrollsTheResults(t *testing.T) {
	t.Parallel()

	m := mouseSearch(t)

	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "Session notes", 0))
	if m.selectedRow != 2 || cmd == nil || opensDetail(cmd) {
		t.Fatalf("first click: selected row %d, cmd %v; want row 2 selected and Detail not opened", m.selectedRow, cmd != nil)
	}
	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "Session notes", 200)); !opensDetail(cmd) {
		t.Fatal("a second click on the same result did not open Detail")
	}

	m.Update(mouseAt(t, m, mode.MouseWheelUp, "Session notes", 1000))
	if m.selectedRow != 1 {
		t.Fatalf("a wheel notch up left row %d selected, want 1", m.selectedRow)
	}
	m.Update(mouseAt(t, m, mode.MouseWheelDown, "Session notes", 1100))
	if m.selectedRow != 2 {
		t.Fatalf("a wheel notch down left row %d selected, want 2", m.selectedRow)
	}
}

func TestPointerMarksTheResultUnderItUntilItLeaves(t *testing.T) {
	t.Parallel()

	m := mouseSearch(t)
	m.Update(mouseAt(t, m, mode.MouseMove, "Triage inbox", 0))
	state := m.viewState(0)
	if hover := m.hover(state); hover == nil || hover.Row != 1 {
		t.Fatalf("the pointer on the second result marks %#v", hover)
	}

	m.Update(mode.MouseMsg{Kind: mode.MouseLeave})
	if hover := m.hover(m.viewState(0)); hover != nil {
		t.Fatalf("the pointer left and %#v is still marked", hover)
	}
}
