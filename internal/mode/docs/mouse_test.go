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
	for _, msg := range testui.DrainCmd(cmd) {
		if request, ok := msg.(mode.ActionRequestMsg); ok && request.Mode == mode.Docs && request.Action == mode.ActionOpenDetail {
			return true
		}
	}
	return false
}

func TestMouseSelectsOpensAndScrollsTheDocList(t *testing.T) {
	t.Parallel()

	m := mouseDocs(t)

	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "doc-three", 0))
	if m.selectedRow != 2 || cmd == nil || opensDetail(cmd) {
		t.Fatalf("first click: selected row %d, cmd %v; want row 2 selected and Detail not opened", m.selectedRow, cmd != nil)
	}
	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "doc-three", 200)); !opensDetail(cmd) {
		t.Fatal("a second click on the same doc did not open Detail")
	}

	if cmd = m.Update(mouseAt(t, m, mode.MouseWheelUp, "doc-one", 1000)); m.selectedRow != 1 || cmd == nil {
		t.Fatalf("wheel up left the selection on row %d, want row 1", m.selectedRow)
	}
	_ = m.Update(mouseAt(t, m, mode.MouseWheelUp, "doc-one", 1010))
	if cmd = m.Update(mouseAt(t, m, mode.MouseWheelUp, "doc-one", 1020)); m.selectedRow != 0 || cmd != nil {
		t.Fatalf("a notch past the first row moved to row %d or reported a change", m.selectedRow)
	}

	// The column title is not a row.
	x, y := testui.FindCell(t, m.View(0), "Docs ─")
	if cmd = m.Update(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: mouseStart}); cmd != nil || m.selectedRow != 0 {
		t.Fatal("a click on the column title changed the selection")
	}
}

func TestHoverFollowsThePointerAndClearsWhenItLeaves(t *testing.T) {
	t.Parallel()

	m := mouseDocs(t)
	idle := m.View(0)

	_ = m.Update(mouseAt(t, m, mode.MouseMove, "doc-two", 0))
	if m.selectedRow != 0 {
		t.Fatal("moving the pointer changed the selection")
	}
	if hovered := m.View(0); hovered == idle {
		t.Fatal("the row under the pointer was not marked")
	}

	_ = m.Update(mode.MouseMsg{Kind: mode.MouseLeave})
	if m.View(0) != idle {
		t.Fatal("the hover chevron stayed after the pointer left")
	}
}
