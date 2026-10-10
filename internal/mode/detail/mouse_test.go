package detail

import (
	"testing"
	"time"

	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uidetail "github.com/hk9890/task-manager-ui/internal/ui/detail"
)

const (
	mouseWidth  = 160
	mouseHeight = 24
)

var mouseStart = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

func mouseDetail(t *testing.T) *Model {
	t.Helper()
	m := &Model{}
	m.selectionID = "tm-1"
	m.targetID = "tm-1"
	m.ApplyLoadedDetail("tm-1", issueWithLongContent("tm-1", "Issue A"))
	return m
}

func mouseAt(t *testing.T, m *Model, kind mode.MouseKind, text string, ms int) mode.MouseMsg {
	t.Helper()
	x, y := testui.FindCell(t, m.View(mouseWidth, mouseHeight, false, 0), text)
	return mode.MouseMsg{Kind: kind, X: x, Y: y, At: mouseStart.Add(time.Duration(ms) * time.Millisecond)}
}

func TestClickPutsTheCursorOnAReferenceAndASecondClickOpensIt(t *testing.T) {
	t.Parallel()

	m := mouseDetail(t)

	if cmd := m.HandleMouse(mouseAt(t, m, mode.MouseClick, "Dependency 4", 0), mouseWidth, mouseHeight); cmd != nil {
		t.Fatalf("a single click produced %#v, want no command", cmd())
	}
	if m.FocusPane != uidetail.FocusPaneDependencies || m.browserSelectedIssueID() != "tm-dep-04" {
		t.Fatalf("first click: focus %d cursor %q; want the Dependencies pane with the cursor on tm-dep-04", m.FocusPane, m.browserSelectedIssueID())
	}

	cmd := m.HandleMouse(mouseAt(t, m, mode.MouseClick, "Dependency 4", 200), mouseWidth, mouseHeight)
	if cmd == nil {
		t.Fatal("second click produced no command, want one that opens tm-dep-04")
	}
	if msg, ok := cmd().(OpenRelatedIssueMsg); !ok || msg.Ref.ID != "tm-dep-04" || msg.Ref.Title != "Dependency 4" {
		t.Fatalf("second click produced %#v, want an OpenRelatedIssueMsg for tm-dep-04", cmd())
	}

	if cmd = m.HandleMouse(mouseAt(t, m, mode.MouseClick, "Dependency 4", 900), mouseWidth, mouseHeight); cmd != nil {
		t.Fatal("a click outside the double-click window opened the reference")
	}
}

func TestClickFocusesThePaneUnderThePointer(t *testing.T) {
	t.Parallel()

	m := mouseDetail(t)
	for text, want := range map[string]uidetail.FocusPane{
		"Metadata ─":     uidetail.FocusPaneMetadata,
		"Dependencies ─": uidetail.FocusPaneDependencies,
		"Content ─":      uidetail.FocusPaneContent,
	} {
		if cmd := m.HandleMouse(mouseAt(t, m, mode.MouseClick, text, 0), mouseWidth, mouseHeight); cmd != nil || m.FocusPane != want {
			t.Errorf("click on %q: focus %d, command %v; want focus %d and no command", text, m.FocusPane, cmd != nil, want)
		}
	}
}

// TestWheelScrollsTheTextPanesAndMovesTheDependencyCursor checks each pane
// answers the wheel the way it answers its scroll keys, without taking focus.
func TestWheelScrollsTheTextPanesAndMovesTheDependencyCursor(t *testing.T) {
	t.Parallel()

	m := mouseDetail(t)

	m.HandleMouse(mouseAt(t, m, mode.MouseWheelDown, "Content ─", 0), mouseWidth, mouseHeight)
	if m.ContentScrollOffset != wheelLines {
		t.Fatalf("a notch over the content scrolled it to %d, want %d", m.ContentScrollOffset, wheelLines)
	}
	m.HandleMouse(mouseAt(t, m, mode.MouseWheelUp, "Content ─", 10), mouseWidth, mouseHeight)
	m.HandleMouse(mouseAt(t, m, mode.MouseWheelUp, "Content ─", 20), mouseWidth, mouseHeight)
	if m.ContentScrollOffset != 0 {
		t.Fatalf("scrolling up past the top left the offset at %d", m.ContentScrollOffset)
	}

	before := m.browserSelectedIssueID()
	m.HandleMouse(mouseAt(t, m, mode.MouseWheelDown, "Dependencies ─", 30), mouseWidth, mouseHeight)
	if after := m.browserSelectedIssueID(); after == before {
		t.Fatalf("a notch over the Dependencies pane left the cursor on %q", after)
	}
	if m.ContentScrollOffset != 0 {
		t.Fatal("the wheel over the Dependencies pane scrolled the content")
	}
	if m.FocusPane != uidetail.FocusPaneContent {
		t.Fatalf("the wheel moved the focus to %d", m.FocusPane)
	}
}

func TestHoverFollowsThePointerAndClearsWhenItLeaves(t *testing.T) {
	testui.ForceTrueColor(t)

	m := mouseDetail(t)
	idle := m.View(mouseWidth, mouseHeight, false, 0)
	cursor := m.browserSelectedIssueID()

	m.HandleMouse(mouseAt(t, m, mode.MouseMove, "Dependency 6", 0), mouseWidth, mouseHeight)
	if m.browserSelectedIssueID() != cursor {
		t.Fatal("moving the pointer moved the cursor")
	}
	if m.View(mouseWidth, mouseHeight, false, 0) == idle {
		t.Fatal("the reference under the pointer was not marked")
	}

	m.HandleMouse(mode.MouseMsg{Kind: mode.MouseLeave}, mouseWidth, mouseHeight)
	if m.View(mouseWidth, mouseHeight, false, 0) != idle {
		t.Fatal("the hover band stayed after the pointer left")
	}
}
