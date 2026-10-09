package app

// Mouse routing through the shell: which surface an event reaches, in which
// coordinates, and what an overlay keeps from the surface under it. What each
// surface does with an event is tested in its own mode package.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// newMouseShell is a loaded shell on the Board at 160x30: one ready issue and
// two in progress, the first of which is related to the second.
func newMouseShell(t *testing.T) Model {
	t.Helper()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "ready-first", "task", 1)
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-4", Title: "progress-first", Status: "in_progress", Type: "task", Priority: 1},
		Description: "detail-of-tm-4",
		Related:     []domain.IssueReference{{ID: "tm-2"}},
	})
	seedInProgress(gw, "tm-2", "progress-second", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	return applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 30}})
}

func leftClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func pointerMove(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone}
}

func wheel(x, y int, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: button}
}

// send delivers one message and everything it sets off.
func send(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	return applyMessages(t, m, []tea.Msg{msg})
}

func pressKey(t *testing.T, m Model, runes string) Model {
	t.Helper()
	return send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(runes)})
}

func TestMouseKindMapsOnlyTheEventsTheSurfacesActOn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  tea.MouseMsg
		want mode.MouseKind
		ok   bool
	}{
		{name: "left press", msg: leftClick(1, 1), want: mode.MouseClick, ok: true},
		{name: "pointer move", msg: pointerMove(1, 1), want: mode.MouseMove, ok: true},
		{name: "drag", msg: tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft}, want: mode.MouseMove, ok: true},
		{name: "wheel up", msg: wheel(1, 1, tea.MouseButtonWheelUp), want: mode.MouseWheelUp, ok: true},
		{name: "wheel down", msg: wheel(1, 1, tea.MouseButtonWheelDown), want: mode.MouseWheelDown, ok: true},
		{name: "left release", msg: tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}},
		{name: "right press", msg: tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonRight}},
		{name: "middle press", msg: tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonMiddle}},
	}
	for _, tc := range cases {
		got, ok := mouseKind(tc.msg)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("%s: mouseKind = %d, %v; want %d, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

// TestClickOnABoardRowSelectsItAndASecondClickOpensDetail clicks where the
// full frame draws the row, so it also proves the shell hands the board its own
// coordinates: the header line is above the board and must be subtracted.
//
// At this width the board draws three of its four columns, centred on the
// focused one, so the first click slides the row out from under the pointer.
// The second click, on the same cell, must still open the row just selected.
func TestClickOnABoardRowSelectsItAndASecondClickOpensDetail(t *testing.T) {
	m := newMouseShell(t)
	if got := firstSelectionID(m, mode.Board); got != "tm-1" {
		t.Fatalf("fixture starts on %q, want tm-1", got)
	}

	x, y := testui.FindCell(t, m.View(), "progress-second")
	m = send(t, m, leftClick(x, y))
	if got := firstSelectionID(m, mode.Board); got != "tm-2" || m.active != mode.Board {
		t.Fatalf("one click: selection %q on %q; want tm-2 selected and the board still up", got, m.active)
	}

	m = send(t, m, leftClick(x, y))
	if m.active != mode.Detail || m.detail.Detail.Summary.ID != "tm-2" {
		t.Fatalf("second click: surface %q showing %q; want Detail on tm-2", m.active, m.detail.Detail.Summary.ID)
	}
}

func TestWheelOverTheBoardMovesTheSelection(t *testing.T) {
	m := newMouseShell(t)

	x, y := testui.FindCell(t, m.View(), "progress-first")
	m = send(t, m, leftClick(x, y))
	x, y = testui.FindCell(t, m.View(), "progress-first")
	m = send(t, m, wheel(x, y, tea.MouseButtonWheelDown))
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("a notch down selected %q, want tm-2", got)
	}
	m = send(t, m, wheel(x, y, tea.MouseButtonWheelUp))
	if got := firstSelectionID(m, mode.Board); got != "tm-4" {
		t.Fatalf("a notch up selected %q, want tm-4", got)
	}

	// The wheel over the header strip is not over the board.
	if m = send(t, m, wheel(x, 0, tea.MouseButtonWheelDown)); firstSelectionID(m, mode.Board) != "tm-4" {
		t.Fatal("the wheel over the header moved the board selection")
	}
}

func TestClickOnATabSwitchesToIt(t *testing.T) {
	m := newMouseShell(t)

	for _, tab := range []struct {
		label string
		want  mode.ID
	}{{"Docs", mode.Docs}, {"Search", mode.Search}, {"Board", mode.Board}} {
		header := strings.SplitN(m.View(), "\n", 2)[0]
		x, _ := testui.FindCell(t, header, " "+tab.label+" ")
		// Both padding cells and the label belong to the tab.
		for _, column := range []int{x, x + 1, x + len(tab.label) + 1} {
			if got, ok := m.tabAt(column); !ok || got != tab.want {
				t.Fatalf("tabAt(%d) = %q, %v; want %q", column, got, ok, tab.want)
			}
		}
		m = send(t, m, leftClick(x+1, 0))
		if m.active != tab.want || m.lastBrowse != tab.want {
			t.Fatalf("click on %s left the shell on %q (last browse %q)", tab.label, m.active, m.lastBrowse)
		}
	}

	// From Detail a tab click leaves the drill-in, as its mode key does.
	x, y := testui.FindCell(t, m.View(), "progress-first")
	m = send(t, send(t, m, leftClick(x, y)), leftClick(x, y))
	if m.active != mode.Detail {
		t.Fatalf("fixture did not reach Detail, on %q", m.active)
	}
	x, _ = testui.FindCell(t, strings.SplitN(m.View(), "\n", 2)[0], " Docs ")
	if m = send(t, m, leftClick(x+1, 0)); m.active != mode.Docs {
		t.Fatalf("tab click from Detail left the shell on %q, want docs", m.active)
	}
}

func TestClickOffTheTabsOnTheHeaderDoesNothing(t *testing.T) {
	m := newMouseShell(t)

	for _, column := range []int{0, headerTabsStart() - 1, 150} {
		if got, ok := m.tabAt(column); ok {
			t.Errorf("tabAt(%d) = %q, want no tab", column, got)
		}
		if next := send(t, m, leftClick(column, 0)); next.active != mode.Board {
			t.Errorf("click on header column %d switched to %q", column, next.active)
		}
	}
}

// TestHoverLightsTheTabAndTheRowUnderThePointer checks the highlight follows
// the pointer between the strip and the surface and is gone once the pointer
// rests on neither.
func TestHoverLightsTheTabAndTheRowUnderThePointer(t *testing.T) {
	m := newMouseShell(t)
	idle := m.View()

	x, _ := testui.FindCell(t, strings.SplitN(idle, "\n", 2)[0], " Docs ")
	m = send(t, m, pointerMove(x+1, 0))
	if m.hoverTab != mode.Docs || m.active != mode.Board {
		t.Fatalf("pointer over Docs: hover %q, surface %q; want the tab lit and the board still up", m.hoverTab, m.active)
	}

	rowX, rowY := testui.FindCell(t, idle, "progress-second")
	m = send(t, m, pointerMove(rowX, rowY))
	if m.hoverTab != "" {
		t.Fatalf("the tab stayed lit (%q) after the pointer moved to a row", m.hoverTab)
	}
	if firstSelectionID(m, mode.Board) != "tm-1" {
		t.Fatal("moving the pointer changed the selection")
	}
	hovered := m.View()
	if _, y := testui.FindCell(t, hovered, "› T P2"); y != rowY {
		t.Fatalf("hover chevron drawn on screen row %d, want row %d", y, rowY)
	}

	// The footer is neither a tab nor a row.
	if m = send(t, m, pointerMove(rowX, 29)); m.View() != idle {
		t.Fatal("the row stayed lit after the pointer moved to the footer")
	}
}

// TestAnOverlayKeepsTheMouseFromTheSurfaceBelow covers both overlays: neither
// a click nor the wheel reaches the board while one is open, and the help
// overlay scrolls under the wheel.
func TestAnOverlayKeepsTheMouseFromTheSurfaceBelow(t *testing.T) {
	base := newMouseShell(t)
	x, y := testui.FindCell(t, base.View(), "progress-second")

	// An open dialog schedules a repeating cursor tick, so these flows are
	// stepped by hand rather than drained (docs/TESTING.md).
	step := func(m Model, msg tea.Msg) Model {
		next, _ := m.Update(msg)
		return next.(Model)
	}

	for name, key := range map[string]string{"help": "?", "close dialog": "x"} {
		m := step(newMouseShell(t), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if !m.showHelp && !m.showActionModal {
			t.Fatalf("%s: the overlay did not open", name)
		}

		for _, event := range []tea.MouseMsg{pointerMove(x, y), leftClick(x, y), leftClick(x, y), wheel(x, y, tea.MouseButtonWheelDown)} {
			next, cmd := m.Update(event)
			m = next.(Model)
			if cmd != nil {
				t.Errorf("%s: a mouse event under the overlay returned a command", name)
			}
		}
		if got := firstSelectionID(m, mode.Board); got != "tm-1" || m.active != mode.Board {
			t.Errorf("%s: the mouse reached the board under the overlay (selection %q, surface %q)", name, got, m.active)
		}
		if !m.showHelp && !m.showActionModal {
			t.Errorf("%s: the mouse closed the overlay", name)
		}

		m.showHelp, m.showActionModal = false, false
		if strings.Contains(m.View(), "› T P2") {
			t.Errorf("%s: a row under the overlay was lit by the pointer", name)
		}
	}

	// The help text is taller than this terminal, so the wheel has somewhere to go.
	m := pressKey(t, newMouseShell(t), "?")
	top := m.View()
	if !strings.Contains(top, "more lines") {
		t.Fatalf("fixture: the help overlay is not clipped at this height:\n%s", top)
	}
	m = send(t, m, wheel(x, y, tea.MouseButtonWheelDown))
	if scrolled := m.View(); scrolled == top || !strings.Contains(scrolled, "earlier lines") {
		t.Fatal("the wheel did not scroll the help overlay")
	}
	// Reopening starts at the top again.
	m = pressKey(t, pressKey(t, m, "?"), "?")
	if m.View() != top {
		t.Fatal("the help overlay reopened where it was scrolled to")
	}
}

// TestDoubleClickOnADependencyRowDrillsIntoIt drives Detail through the shell:
// the click lands on a row of the rail, and the second one navigates.
func TestDoubleClickOnADependencyRowDrillsIntoIt(t *testing.T) {
	m := newMouseShell(t)

	x, y := testui.FindCell(t, m.View(), "progress-first")
	m = send(t, send(t, m, leftClick(x, y)), leftClick(x, y))
	if m.active != mode.Detail || m.detail.Detail.Summary.ID != "tm-4" {
		t.Fatalf("fixture: on %q showing %q, want Detail on tm-4", m.active, m.detail.Detail.Summary.ID)
	}

	x, y = testui.FindCell(t, m.View(), "progress-second")
	m = send(t, m, leftClick(x, y))
	if m.drillSelection != nil || m.detail.Detail.Summary.ID != "tm-4" {
		t.Fatal("a single click on a dependency row navigated")
	}
	m = send(t, m, leftClick(x, y))
	if m.drillSelection == nil || m.drillSelection.Issue.ID != "tm-2" || m.detail.Detail.Summary.ID != "tm-2" {
		t.Fatalf("second click: drill %v, showing %q; want a drill-in to tm-2", m.drillSelection, m.detail.Detail.Summary.ID)
	}

	// The wheel over the content pane scrolls it and leaves the selection alone.
	x, y = testui.FindCell(t, m.View(), "Content ─")
	m = send(t, m, wheel(x, y, tea.MouseButtonWheelDown))
	if m.active != mode.Detail || m.drillSelection.Issue.ID != "tm-2" {
		t.Fatal("the wheel over Detail left it or moved its selection")
	}
}

// TestTheStorePickerTakesTheMouseInsteadOfTheBoard: the picker renders instead
// of the shell, so the board below it must see nothing.
func TestTheStorePickerTakesTheMouseInsteadOfTheBoard(t *testing.T) {
	m := newMouseShell(t)
	x, y := testui.FindCell(t, m.View(), "progress-second")

	m = pressKey(t, m, "s")
	if m.active != mode.StorePicker {
		t.Fatalf("fixture did not open the picker, on %q", m.active)
	}
	m = send(t, m, leftClick(x, y))
	m = send(t, m, leftClick(x, y))
	m = send(t, m, leftClick(30, 0))
	if got := firstSelectionID(m, mode.Board); got != "tm-1" || m.active != mode.StorePicker {
		t.Fatalf("the mouse reached the shell under the picker (selection %q, surface %q)", got, m.active)
	}
}

func TestTheFatalErrorScreenIgnoresTheMouse(t *testing.T) {
	m := newMouseShell(t)
	m.fatalErrTitle = "no task-manager store here"

	next, cmd := m.Update(leftClick(40, 5))
	if cmd != nil || firstSelectionID(next.(Model), mode.Board) != "tm-1" {
		t.Fatal("the fatal error screen acted on a click")
	}
}
