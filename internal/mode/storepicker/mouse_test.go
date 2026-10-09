package storepicker

import (
	"testing"
	"time"

	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

var mouseStart = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

func mousePicker(t *testing.T) *Model {
	t.Helper()
	m := newModel(t, nil)
	m.SetSize(100, 24)
	m.generation = 1
	m.Update(StoresLoadedMsg{Entries: entries("alpha", "bravo", "charlie"), Generation: 1})
	return m
}

func mouseAt(t *testing.T, m *Model, kind mode.MouseKind, text string, ms int) mode.MouseMsg {
	t.Helper()
	x, y := testui.FindCell(t, m.View(0, ""), text)
	return mode.MouseMsg{Kind: kind, X: x, Y: y, At: mouseStart.Add(time.Duration(ms) * time.Millisecond)}
}

func TestClickSelectsAStoreAndASecondClickOpensIt(t *testing.T) {
	t.Parallel()

	m := mousePicker(t)

	if cmd := m.Update(mouseAt(t, m, mode.MouseClick, "/dev/charlie", 0)); cmd != nil || m.selectedRow != 2 {
		t.Fatalf("first click: row %d, cmd %v; want row 2 selected and nothing opened", m.selectedRow, cmd != nil)
	}
	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "/dev/charlie", 200))
	if cmd == nil {
		t.Fatal("a second click on the same store returned no command")
	}
	if open, ok := cmd().(OpenMsg); !ok || open.Entry.Name != "charlie" {
		t.Fatalf("second click produced %#v, want OpenMsg for charlie", cmd())
	}

	// Two clicks too far apart, and a click off the rows, open nothing.
	_ = m.Update(mouseAt(t, m, mode.MouseClick, "/dev/alpha", 2000))
	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "/dev/alpha", 2600)); cmd != nil {
		t.Fatal("two slow clicks opened a store")
	}
	if cmd = m.Update(mouseAt(t, m, mode.MouseClick, "Task Stores", 5000)); cmd != nil || m.selectedRow != 0 {
		t.Fatal("a click on the title changed the selection")
	}
}

// TestDoubleClickOnAnActionRowAsksToCreateAStore covers the create rows the
// picker draws above the registry when nothing resolved for the directory.
func TestDoubleClickOnAnActionRowAsksToCreateAStore(t *testing.T) {
	t.Parallel()

	m := mousePicker(t)
	m.SetCreateTarget("/work/demo")

	_ = m.Update(mouseAt(t, m, mode.MouseClick, "Create a central store", 0))
	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "Create a central store", 100))
	if cmd == nil {
		t.Fatal("a double click on an action row returned no command")
	}
	if create, ok := cmd().(CreateMsg); !ok || create.Kind != CentralStore || create.Dir != "/work/demo" {
		t.Fatalf("double click produced %#v, want CreateMsg for a central store in /work/demo", cmd())
	}
}

func TestWheelAndHoverOnThePicker(t *testing.T) {
	testui.ForceTrueColor(t)

	m := mousePicker(t)

	_ = m.Update(mouseAt(t, m, mode.MouseWheelDown, "/dev/alpha", 0))
	_ = m.Update(mouseAt(t, m, mode.MouseWheelDown, "/dev/alpha", 10))
	_ = m.Update(mouseAt(t, m, mode.MouseWheelDown, "/dev/alpha", 20))
	if m.selectedRow != 2 {
		t.Fatalf("three notches down left the selection on row %d, want the last row 2", m.selectedRow)
	}
	_ = m.Update(mouseAt(t, m, mode.MouseWheelUp, "/dev/alpha", 30))
	if m.selectedRow != 1 {
		t.Fatalf("a notch up left the selection on row %d, want row 1", m.selectedRow)
	}

	_ = m.Update(mode.MouseMsg{Kind: mode.MouseLeave})
	idle := m.View(0, "")
	_ = m.Update(mouseAt(t, m, mode.MouseMove, "/dev/alpha", 40))
	if m.selectedRow != 1 || m.View(0, "") == idle {
		t.Fatal("moving the pointer must mark the row under it and leave the selection alone")
	}
	_ = m.Update(mode.MouseMsg{Kind: mode.MouseLeave})
	if m.View(0, "") != idle {
		t.Fatal("the hover band stayed after the pointer left")
	}
}
