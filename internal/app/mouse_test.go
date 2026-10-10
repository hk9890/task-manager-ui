package app

// Mouse routing through the shell: which surface an event reaches, in which
// coordinates, and what an overlay keeps from the surface under it. What each
// surface does with an event is tested in its own mode package.

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
)

// newMouseShell is a loaded shell on the Board at 160x30: one ready issue and
// two in progress, the first of which is related to the second.
//
// The clock stands still, so two clicks sent one after the other are a double
// click however long the machine takes between them.
func newMouseShell(t *testing.T) Model {
	t.Helper()
	withModelNow(t, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))
	// The row bands are backgrounds, which only a colour profile draws.
	testui.ForceTrueColor(t)

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

func pressKey(t *testing.T, m Model, name string) Model {
	t.Helper()
	return send(t, m, testKey(name))
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
// coordinates: the header lines are above the board and must be subtracted.
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
	}{{"Docs", mode.Docs}, {"Board", mode.Board}} {
		x, _ := testui.FindCell(t, tabLine(m), " "+tab.label+" ")
		// Both padding cells and the label belong to the tab.
		for _, column := range []int{x, x + 1, x + len(tab.label) + 1} {
			if got, ok := m.tabAt(column); !ok || got != tab.want {
				t.Fatalf("tabAt(%d) = %q, %v; want %q", column, got, ok, tab.want)
			}
		}
		m = send(t, m, leftClick(x+1, headerTabsRow))
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
	x, _ = testui.FindCell(t, tabLine(m), " Docs ")
	if m = send(t, m, leftClick(x+1, headerTabsRow)); m.active != mode.Docs {
		t.Fatalf("tab click from Detail left the shell on %q, want docs", m.active)
	}
}

// headerLine is one line of the shell's header, without colour, as a hit-test
// test finds a tab or a button on it with testui.FindCell.
func headerLine(m Model, row int) string {
	return testui.AnsiEscapePattern.ReplaceAllString(strings.Split(m.View(), "\n")[row], "")
}

// topBar is the menu bar: the buttons and the version.
func topBar(m Model) string { return headerLine(m, headerMenuRow) }

// tabLine is the line of the view tabs.
func tabLine(m Model) string { return headerLine(m, headerTabsRow) }

// barButton is where the menu bar draws the button of a shell action: its first
// column and its text, the label and the key bound to it.
func barButton(t *testing.T, m Model, label, action string) (x int, text string) {
	t.Helper()
	text = label + " " + m.keys.DisplayPrimary(config.ShellContext, action)
	x, _ = testui.FindCell(t, topBar(m), text)
	return x, text
}

// litButton is the label of the button the menu bar lights, or "".
func litButton(m Model) string {
	if m.barPointer == nil {
		return ""
	}
	cell, _ := m.buttonAt(*m.barPointer)
	return cell.label
}

func TestClickOffTheTabsAndTheButtonsOnTheHeaderDoesNothing(t *testing.T) {
	m := newMouseShell(t)

	storesX, storesText := barButton(t, m, "stores", config.ShellActionStorePicker)
	docsX, _ := testui.FindCell(t, tabLine(m), " Docs ")

	dead := map[string][][2]int{
		// The spinner cell, the cell before the first tab, the first cell
		// after the last tab, and the empty line right of the tabs.
		"the tab line": {{0, headerTabsRow}, {headerTabsStart() - 1, headerTabsRow}, {docsX + len(" Docs "), headerTabsRow}, {80, headerTabsRow}},
		// The cell before the first button, the `·` after it, and the space
		// before the version.
		"the menu bar": {{storesX - 1, headerMenuRow}, {storesX + len(storesText) + 1, headerMenuRow}, {80, headerMenuRow}},
		// The rule draws nothing to press, under a button or over a tab.
		"the rule": {{docsX + 1, 1}, {storesX + 1, 1}},
	}
	for name, cells := range dead {
		for _, cell := range cells {
			if got, ok := m.tabAt(cell[0]); ok && cell[1] == headerTabsRow {
				t.Errorf("%s: tabAt(%d) = %q, want no tab", name, cell[0], got)
			}
			next, cmd := m.Update(leftClick(cell[0], cell[1]))
			if cmd != nil {
				t.Errorf("%s: click on column %d returned a command", name, cell[0])
			}
			got := next.(Model)
			if got.active != mode.Board || got.showHelp || got.pendingDialog.active || litButton(got) != "" || got.hoverTab != "" {
				t.Errorf("%s: click on column %d: surface %q, help %v, pending dialog %v, hover %q/%q; want nothing",
					name, cell[0], got.active, got.showHelp, got.pendingDialog.active, got.hoverTab, litButton(got))
			}
			if firstSelectionID(got, mode.Board) != "tm-1" {
				t.Errorf("%s: click on column %d reached the board", name, cell[0])
			}
		}
	}
}

// TestClickOnReloadAndQuit: the two buttons whose effect is a command, not a
// screen. Each returns what its key returns.
func TestClickOnReloadAndQuit(t *testing.T) {
	m := newMouseShell(t)

	x, _ := testui.FindCell(t, topBar(m), "reload "+m.keys.DisplayPrimary(config.BoardContext, config.BoardActionReload))
	next, cmd := m.Update(leftClick(x, headerMenuRow))
	if reloading := next.(Model); cmd == nil || !reloading.board.IsLoading() {
		t.Fatalf("click on reload: command %v, board loading %v; want a reload in flight", cmd != nil, reloading.board.IsLoading())
	}

	x, _ = barButton(t, m, "quit", config.ShellActionQuit)
	_, cmd = m.Update(leftClick(x, headerMenuRow))
	quit := false
	for _, msg := range runBatch(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			quit = true
		}
	}
	if !quit {
		t.Fatal("click on quit did not quit")
	}
}

// TestClickOnReloadReloadsTheSurfaceOnScreen: reload is one button, and the
// load it starts is the one of the surface under it and of no other.
func TestClickOnReloadReloadsTheSurfaceOnScreen(t *testing.T) {
	tab := tea.KeyMsg{Type: tea.KeyTab}
	detail := testKey("enter")

	for _, tc := range []struct {
		surface mode.ID
		reach   []tea.Msg
		want    loading.Scope
	}{
		{surface: mode.Board, want: loading.ScopeBoard},
		{surface: mode.Docs, reach: []tea.Msg{tab}, want: loading.ScopeDocs},
		{surface: mode.Detail, reach: []tea.Msg{detail}, want: loading.ScopeDetail},
	} {
		m := applyMessages(t, newMouseShell(t), tc.reach)
		if m.active != tc.surface || len(m.loadingStates()) != 0 {
			t.Fatalf("fixture: on %q with %d loads in flight, want %q at rest", m.active, len(m.loadingStates()), tc.surface)
		}

		x, _ := testui.FindCell(t, topBar(m), "reload "+m.reloadKey())
		next, cmd := m.Update(leftClick(x, headerMenuRow))
		states := next.(Model).loadingStates()
		if cmd == nil || len(states) != 1 || states[0].Scope != tc.want {
			t.Errorf("click on reload on %q: command %v, loading %v; want %q alone", tc.surface, cmd != nil, states, tc.want)
		}
	}
}

// TestTheLitButtonIsTheOneUnderThePointerAfterAKeyChangesTheSurface: the reload
// button is as wide as the key of the surface on screen, so a key that changes
// the surface moves the buttons after it under a pointer that sent no event.
func TestTheLitButtonIsTheOneUnderThePointerAfterAKeyChangesTheSurface(t *testing.T) {
	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{config.ShellActionReloadDetail: {"ctrl+alt+r"}},
	})
	m := send(t, newHeaderShell(t, cfg), tea.WindowSizeMsg{Width: 160, Height: 30})

	x, _ := barButton(t, m, "help", config.ShellActionHelp)
	if m = send(t, m, pointerMove(x, headerMenuRow)); litButton(m) != "help" {
		t.Fatalf("fixture: pointer over help lights %q", litButton(m))
	}

	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyEnter}})
	if moved, _ := barButton(t, m, "help", config.ShellActionHelp); m.active != mode.Detail || moved == x {
		t.Fatalf("fixture: on %q with help at column %d, want detail and help moved from column %d", m.active, moved, x)
	}
	under, ok := m.buttonAt(x)
	if !ok || litButton(m) != under.label {
		t.Errorf("the bar lights %q and draws %q under the pointer", litButton(m), under.text())
	}
}

// TestClickOnABarButtonDoesWhatItsKeyDoes clicks each button where the menu bar
// draws it and holds the shell against the one its key leaves: the bar is a
// second way to reach an action, never a different one.
func TestClickOnABarButtonDoesWhatItsKeyDoes(t *testing.T) {
	// An open dialog schedules a repeating cursor tick, so the flows are
	// stepped by hand rather than drained (docs/TESTING.md): the message, then
	// what its command produced, and no further.
	step := func(m Model, msg tea.Msg) Model {
		next, cmd := m.Update(msg)
		m = next.(Model)
		for _, produced := range runBatch(cmd) {
			next, _ = m.Update(produced)
			m = next.(Model)
		}
		return m
	}
	plain := func(m Model) string { return testui.AnsiEscapePattern.ReplaceAllString(m.View(), "") }

	cases := []struct {
		label, action string
		check         func(*testing.T, Model)
	}{
		{label: "stores", action: config.ShellActionStorePicker, check: func(t *testing.T, m Model) {
			t.Helper()
			if m.active != mode.StorePicker || m.pickerReturn != mode.Board {
				t.Errorf("on %q returning to %q, want the picker returning to the board", m.active, m.pickerReturn)
			}
		}},
		{label: "search", action: config.ShellActionOpenSearch, check: func(t *testing.T, m Model) {
			t.Helper()
			if m.active != mode.Search || m.searchFrom != mode.Board {
				t.Errorf("on %q opened from %q, want the search opened from the board", m.active, m.searchFrom)
			}
		}},
		{label: "help", action: config.ShellActionHelp, check: func(t *testing.T, m Model) {
			t.Helper()
			if !m.showHelp || !strings.Contains(plain(m), "Keyboard Help") {
				t.Errorf("the help overlay is not up (showHelp %v)", m.showHelp)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			base := newMouseShell(t)
			x, text := barButton(t, base, tc.label, tc.action)
			key := base.keys.Primary(config.ShellContext, tc.action)

			byKey := step(base, testKey(key))
			tc.check(t, byKey)

			// Every cell of the button is the button: the label, the space
			// and the key.
			for _, column := range []int{x, x + len(tc.label), x + len(text) - 1} {
				byClick := step(base, leftClick(column, 0))
				tc.check(t, byClick)
				if got, want := plain(byClick), plain(byKey); got != want {
					t.Errorf("click on column %d of %q drew another screen than the %q key:\n%s\n--- key ---\n%s", column, text, key, got, want)
				}
			}

			// One cell either side is not.
			for _, column := range []int{x - 1, x + len(text)} {
				next, cmd := base.Update(leftClick(column, 0))
				if got := next.(Model); cmd != nil || got.active != mode.Board || got.showHelp || got.pendingDialog.active {
					t.Errorf("click on column %d, beside %q, ran an action", column, text)
				}
			}
		})
	}
}

// TestBarButtonsWorkFromDetail: the bar stands over the drill-in too, and the
// picker a click opens from there returns to it, as the key's does.
func TestBarButtonsWorkFromDetail(t *testing.T) {
	m := pressKey(t, newMouseShell(t), "enter")
	if m.active != mode.Detail {
		t.Fatalf("fixture did not reach Detail, on %q", m.active)
	}

	x, _ := barButton(t, m, "stores", config.ShellActionStorePicker)
	m = send(t, m, leftClick(x, 0))
	if m.active != mode.StorePicker || m.pickerReturn != mode.Detail {
		t.Fatalf("click on stores from Detail: on %q returning to %q", m.active, m.pickerReturn)
	}
	if m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc}); m.active != mode.Detail {
		t.Fatalf("escape from the picker landed on %q, want Detail", m.active)
	}

	x, _ = barButton(t, m, "help", config.ShellActionHelp)
	if m = send(t, m, leftClick(x, 0)); !m.showHelp || m.active != mode.Detail {
		t.Fatalf("click on help from Detail: help %v, on %q", m.showHelp, m.active)
	}
}

// TestABarButtonIsDeadUnderAnOverlayAndUnderThePicker clicks the cells the
// buttons were drawn on while something else holds the screen. An overlay
// ignores the mouse and the picker draws no bar, so nothing may run.
func TestABarButtonIsDeadUnderAnOverlayAndUnderThePicker(t *testing.T) {
	base := newMouseShell(t)
	buttons := map[string]int{}
	for label, action := range map[string]string{
		"stores": config.ShellActionStorePicker, "help": config.ShellActionHelp, "quit": config.ShellActionQuit,
	} {
		buttons[label], _ = barButton(t, base, label, action)
	}

	for name, key := range map[string]string{"help": "alt+h", "close dialog": "delete"} {
		// Stepped by hand: an open dialog schedules a repeating cursor tick.
		next, _ := newMouseShell(t).Update(testKey(key))
		m := next.(Model)
		if !m.showHelp && !m.showActionModal {
			t.Fatalf("%s: the overlay did not open", name)
		}
		helpWas, modalWas := m.showHelp, m.showActionModal

		for label, x := range buttons {
			for _, event := range []tea.MouseMsg{pointerMove(x, 0), leftClick(x, 0)} {
				next, cmd := m.Update(event)
				m = next.(Model)
				if cmd != nil {
					t.Errorf("%s: a mouse event on %q under the overlay returned a command", name, label)
				}
				if litButton(m) != "" {
					t.Errorf("%s: %q is lit under the overlay", name, label)
				}
			}
			if m.active != mode.Board || m.showHelp != helpWas || m.showActionModal != modalWas || m.pendingDialog.active {
				t.Errorf("%s: a click on %q under the overlay ran (surface %q, help %v, modal %v, pending dialog %v)",
					name, label, m.active, m.showHelp, m.showActionModal, m.pendingDialog.active)
			}
		}
	}

	m := pressKey(t, newMouseShell(t), "alt+s")
	if m.active != mode.StorePicker {
		t.Fatalf("fixture did not open the picker, on %q", m.active)
	}
	for label, x := range buttons {
		m = send(t, send(t, m, pointerMove(x, 0)), leftClick(x, 0))
		if m.active != mode.StorePicker || m.showHelp || m.showActionModal || m.pendingDialog.active || litButton(m) != "" {
			t.Errorf("a click where %q was drawn ran under the picker (surface %q, help %v, modal %v, pending dialog %v, hover %q)",
				label, m.active, m.showHelp, m.showActionModal, m.pendingDialog.active, litButton(m))
		}
	}
}

// TestHoverLightsABarButtonAndLeavingClearsIt follows the pointer onto a button
// and off it by every way there is: onto a tab, the rule, a row, the footer,
// and under the picker and an overlay.
func TestHoverLightsABarButtonAndLeavingClearsIt(t *testing.T) {
	m := newMouseShell(t)
	idle := m.View()

	x, text := barButton(t, m, "stores", config.ShellActionStorePicker)
	helpX, _ := barButton(t, m, "help", config.ShellActionHelp)
	docsX, _ := testui.FindCell(t, tabLine(m), " Docs ")
	rowX, rowY := testui.FindCell(t, idle, "progress-second")

	onButton := func(m Model) Model {
		t.Helper()
		m = send(t, m, pointerMove(x+1, 0))
		if litButton(m) != "stores" || m.hoverTab != "" || m.active != mode.Board {
			t.Fatalf("pointer over stores: hover %q, tab %q, surface %q; want the button lit and the board still up",
				litButton(m), m.hoverTab, m.active)
		}
		return m
	}

	m = onButton(m)
	lit := m.View()
	if lit == idle {
		t.Fatal("the button under the pointer is drawn as it is at rest")
	}
	if got, want := testui.AnsiEscapePattern.ReplaceAllString(lit, ""), testui.AnsiEscapePattern.ReplaceAllString(idle, ""); got != want {
		t.Fatalf("lighting a button moved text on the screen:\n%s", got)
	}

	// Onto the next button: one is lit at a time.
	if m = send(t, m, pointerMove(helpX, 0)); litButton(m) != "help" {
		t.Fatalf("pointer over help: hover %q", litButton(m))
	}
	// The wheel over a button is not a click.
	if m = send(t, m, wheel(helpX, 0, tea.MouseButtonWheelDown)); m.showHelp || firstSelectionID(m, mode.Board) != "tm-1" {
		t.Fatal("the wheel over a button ran it or moved the board selection")
	}

	leaves := map[string]tea.MouseMsg{
		"a tab":         pointerMove(docsX+1, headerTabsRow),
		"the separator": pointerMove(x+len(text)+1, 0),
		"the rule":      pointerMove(x+1, 1),
		"a row":         pointerMove(rowX, rowY),
		"the footer":    pointerMove(x+1, 29),
	}
	for name, leave := range leaves {
		if left := send(t, onButton(m), leave); litButton(left) != "" {
			t.Errorf("the button stayed lit (%q) after the pointer moved to %s", litButton(left), name)
		}
	}
	if left := send(t, onButton(m), leaves["the footer"]); left.View() != idle {
		t.Error("the screen is not as it was at rest after the pointer left the button for the footer")
	}
	if left := send(t, onButton(m), leaves["a tab"]); left.hoverTab != mode.Docs {
		t.Errorf("the tab the pointer moved to is not lit (%q)", left.hoverTab)
	}
	if left := send(t, send(t, m, pointerMove(docsX+1, headerTabsRow)), pointerMove(x+1, 0)); left.hoverTab != "" {
		t.Errorf("the tab stayed lit (%q) after the pointer moved to a button", left.hoverTab)
	}

	// The bar forgets the pointer once the picker is up, and under an overlay.
	picker := pressKey(t, onButton(m), "alt+s")
	picker = send(t, picker, pointerMove(x+1, 5))
	if picker = send(t, picker, tea.KeyMsg{Type: tea.KeyEsc}); picker.active != mode.Board || litButton(picker) != "" {
		t.Errorf("back on %q the %q button is lit under a cell the pointer left", picker.active, litButton(picker))
	}
	help := pressKey(t, onButton(m), "alt+h")
	if help = send(t, help, pointerMove(x+1, 0)); !help.showHelp || litButton(help) != "" {
		t.Errorf("under the help overlay (%v) the %q button is lit", help.showHelp, litButton(help))
	}
}

// TestHoverLightsTheTabAndTheRowUnderThePointer checks the highlight follows
// the pointer between the strip and the surface and is gone once the pointer
// rests on neither.
func TestHoverLightsTheTabAndTheRowUnderThePointer(t *testing.T) {
	m := newMouseShell(t)
	idle := m.View()

	x, _ := testui.FindCell(t, tabLine(m), " Docs ")
	m = send(t, m, pointerMove(x+1, headerTabsRow))
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
	if testui.RowBand(t, hovered, "progress-second") == "" {
		t.Fatalf("the row on screen row %d under the pointer carries no band", rowY)
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

	for name, key := range map[string]string{"help": "alt+h", "close dialog": "delete"} {
		m := step(newMouseShell(t), testKey(key))
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
		if testui.RowBand(t, m.View(), "progress-second") != "" {
			t.Errorf("%s: a row under the overlay was lit by the pointer", name)
		}
	}

	// The help text is taller than this terminal, so the wheel has somewhere to go.
	m := pressKey(t, newMouseShell(t), "alt+h")
	top := m.View()
	if !strings.Contains(top, "more lines") {
		t.Fatalf("fixture: the help overlay is not clipped at this height:\n%s", top)
	}
	m = send(t, m, wheel(x, y, tea.MouseButtonWheelDown))
	if scrolled := m.View(); scrolled == top || !strings.Contains(scrolled, "earlier lines") {
		t.Fatal("the wheel did not scroll the help overlay")
	}
	// Reopening starts at the top again.
	m = pressKey(t, pressKey(t, m, "alt+h"), "alt+h")
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
	next, cmd := m.Update(leftClick(x, y))
	m, cmd = deliverDrill(t, next.(Model), cmd)
	if m.detail.TargetID() != "tm-2" || !m.detail.IsLoading() {
		t.Fatalf("second click: target %q loading %v, want an in-flight load of tm-2", m.detail.TargetID(), m.detail.IsLoading())
	}
	m = applyMessages(t, m, runBatch(cmd))
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

	m = pressKey(t, m, "alt+s")
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

// TestASurfaceLeftByAKeyForgetsThePointer: a key takes the shell to another
// surface and the pointer moves on there. The surface left behind must not
// light a row under the cell it last saw the pointer on when it is drawn again.
func TestASurfaceLeftByAKeyForgetsThePointer(t *testing.T) {
	m := newMouseShell(t)

	x, y := testui.FindCell(t, m.View(), "progress-second")
	m = send(t, m, pointerMove(x, y))
	if testui.RowBand(t, m.View(), "progress-second") == "" {
		t.Fatal("fixture: the row under the pointer is not lit")
	}

	m = pressKey(t, m, "enter")
	if m.active != mode.Detail {
		t.Fatalf("fixture did not reach Detail, on %q", m.active)
	}
	m = send(t, m, pointerMove(x, y+3))
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.active != mode.Board || testui.RowBand(t, m.View(), "progress-second") != "" {
		t.Fatalf("back on %q a row is lit under a cell the pointer left", m.active)
	}

	// The tab strip forgets the pointer the same way once the picker is up.
	tabX, _ := testui.FindCell(t, tabLine(m), " Docs ")
	m = send(t, m, pointerMove(tabX+1, headerTabsRow))
	if m.hoverTab != mode.Docs {
		t.Fatalf("fixture: hovered tab is %q, want docs", m.hoverTab)
	}
	m = pressKey(t, m, "alt+s")
	m = send(t, m, pointerMove(tabX+1, 5))
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.active != mode.Board || m.hoverTab != "" {
		t.Fatalf("back on %q the %q tab is lit under a cell the pointer left", m.active, m.hoverTab)
	}
}

// TestEditorExitTurnsMouseReportingBackOn: tea.Exec hands the terminal to the
// editor with mouse reporting off, and Bubble Tea does not restore it. Without
// the command the mouse is dead from the first edit to the end of the session.
func TestEditorExitTurnsMouseReportingBackOn(t *testing.T) {
	for name, execErr := range map[string]error{"clean exit": nil, "editor failed": errors.New("exit status 1")} {
		m := newMouseShell(t)
		m.services.Editor = &fakes.FakeEditor{}

		_, cmd := m.Update(editorExitedMsg{execErr: execErr})
		enabled := false
		for _, msg := range runBatch(cmd) {
			if msg == tea.EnableMouseAllMotion() {
				enabled = true
			}
		}
		if !enabled {
			t.Errorf("%s: the editor exit did not turn mouse reporting back on", name)
		}
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
