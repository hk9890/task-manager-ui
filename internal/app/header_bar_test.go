package app

// The geometry of the three header lines: what the menu bar and the tab line
// keep as the terminal narrows, and that no line reaches past it. What a click
// on the header does is in mouse_test.go.
//
// Direct assertions rather than goldens: the property holds at every width, and
// a snapshot shows three of them.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
	"github.com/hk9890/task-manager-ui/internal/version"
)

// newHeaderShell is a shell with one ready issue, a store name and no size yet:
// each test sets the width it measures.
func newHeaderShell(t *testing.T, cfg config.Model) Model {
	t.Helper()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "A ready issue with a reasonably long title", "task", 1)
	services, err := NewServices(gw, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	services.StoreName = "task-manager-ui"
	m := mustNewModel(t, services)
	return applyMessages(t, m, runBatch(m.Init()))
}

// headerLines renders the header at width and returns its lines, without
// colour.
func headerLines(m Model, width int) []string {
	m.width = width
	return strings.Split(testui.AnsiEscapePattern.ReplaceAllString(m.renderHeader(), ""), "\n")
}

// drawnButtons is the labels of the buttons the menu bar draws, left to right,
// read back from the rendered line and not from barCells.
func drawnButtons(t *testing.T, m Model, bar string) []string {
	t.Helper()

	var labels []string
	last := -1
	for _, action := range barActions {
		text := action.label(m) + " " + action.key(m)
		if action.name {
			// The name is cut to the room the bar has; its key is not.
			text = " " + action.key(m)
		}
		if !strings.Contains(bar, text) {
			continue
		}
		x, _ := testui.FindCell(t, bar, text)
		if x <= last {
			t.Fatalf("button %q is drawn at column %d, left of the one before it:\n%s", text, x, bar)
		}
		last = x
		labels = append(labels, action.label(m))
	}
	return labels
}

// TestHeaderIsThreeLinesNoWiderThanTheTerminal renders the header across the
// widths a terminal is resized through. The workspace height is what is left
// under it, so a fourth line or a wrapped one pushes the footer off the screen.
func TestHeaderIsThreeLinesNoWiderThanTheTerminal(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())

	for _, active := range []mode.ID{mode.Board, mode.Docs, mode.Search, mode.Detail} {
		m.active = active
		for width := 0; width <= 220; width++ {
			lines := headerLines(m, width)
			if len(lines) != 3 {
				t.Fatalf("%s at width %d: the header is %d lines:\n%s", active, width, len(lines), strings.Join(lines, "\n"))
			}
			if got := lipgloss.Width(lines[1]); got != width || strings.Trim(lines[1], "─") != "" {
				t.Fatalf("%s at width %d: the rule is %q", active, width, lines[1])
			}
			if got := lipgloss.Width(lines[headerMenuRow]); got > width {
				t.Fatalf("%s at width %d: the menu bar is %d cells wide:\n%s", active, width, got, lines[headerMenuRow])
			}

			// The tabs are never dropped: a terminal narrower than the tab
			// strip cuts them off at its edge. From there up the line fits.
			tabs := lines[headerTabsRow]
			for _, label := range []string{" Board ", " Docs "} {
				if !strings.Contains(tabs, label) {
					t.Fatalf("%s at width %d: the tab line lost the tab %q:\n%s", active, width, label, tabs)
				}
			}
			if got := lipgloss.Width(tabs); width >= headerTabsEnd() && got > width {
				t.Fatalf("%s at width %d: the tab line is %d cells wide:\n%s", active, width, got, tabs)
			}
		}
	}
}

// TestMenuBarDropsTheVersionThenTheRightmostButton narrows the terminal one
// column at a time. The version goes first, then quit, help, reload, search,
// each after the store's name is cut to its floor: the name is the last button
// standing.
func TestMenuBarDropsTheVersionThenTheRightmostButton(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())
	all := []string{"task-manager-ui", "search", "reload", "help", "quit"}

	previous, hadVersion := 0, false
	for width := 0; width <= 220; width++ {
		bar := headerLines(m, width)[headerMenuRow]
		drawn := drawnButtons(t, m, bar)

		if want := all[:len(drawn)]; strings.Join(drawn, ",") != strings.Join(want, ",") {
			t.Fatalf("at width %d the bar draws %v, want the leftmost %d of %v:\n%s", width, drawn, len(drawn), all, bar)
		}
		if len(drawn) < previous || len(drawn) > previous+1 {
			t.Fatalf("at width %d the bar went from %d buttons to %d", width, previous, len(drawn))
		}

		// What barCells says is what is drawn, and a button that first fits
		// ends on the last column.
		m.width = width
		cells := m.barCells()
		if len(cells) != len(drawn) {
			t.Fatalf("at width %d barCells places %d buttons and the bar draws %d:\n%s", width, len(cells), len(drawn), bar)
		}
		for _, cell := range cells {
			if x, _ := testui.FindCell(t, bar, cell.text()); cell.x0 != x || cell.x1 != x+lipgloss.Width(cell.text()) {
				t.Fatalf("at width %d %q is drawn at column %d and placed at [%d, %d)", width, cell.text(), x, cell.x0, cell.x1)
			}
		}
		if len(drawn) > previous && cells[len(cells)-1].x1 != width {
			t.Fatalf("at width %d the bar gained %q, which ends at column %d", width, drawn[len(drawn)-1], cells[len(cells)-1].x1)
		}
		previous = len(drawn)

		// The version stands flush right, clear of the buttons, or not at all.
		buttonsEnd := 0
		if len(cells) > 0 {
			buttonsEnd = cells[len(cells)-1].x1
		}
		hasVersion := strings.HasSuffix(bar, version.Version) && lipgloss.Width(bar) == width
		fits := len(drawn) == len(all) && width-buttonsEnd-lipgloss.Width(version.Version) >= headerBarGap
		if hasVersion != fits {
			t.Fatalf("at width %d with %v the version is drawn %v, want %v:\n%s", width, drawn, hasVersion, fits, bar)
		}
		if hadVersion && !hasVersion {
			t.Fatalf("the bar lost the version as the terminal grew to %d", width)
		}
		hadVersion = hasVersion
	}

	if previous != len(all) || !hadVersion {
		t.Fatalf("at width 220 the bar draws %d buttons and the version %v, want all of it", previous, hadVersion)
	}
}

// TestMenuBarMeasuresAWideKeyName rebinds help to a key with a long display
// name. A button is placed by what it draws, so quit moves right by what the
// name added.
func TestMenuBarMeasuresAWideKeyName(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{config.ShellActionHelp: {"ctrl+alt+f12"}},
	})
	wide := newHeaderShell(t, cfg)
	narrow := newHeaderShell(t, config.Default())

	quit := "quit " + wide.keys.DisplayPrimary(config.ShellContext, config.ShellActionQuit)
	grew := lipgloss.Width(wide.keys.DisplayPrimary(config.ShellContext, config.ShellActionHelp)) -
		lipgloss.Width(narrow.keys.DisplayPrimary(config.ShellContext, config.ShellActionHelp))
	if grew <= 0 {
		t.Fatal("fixture: the rebound help key is no wider than the default")
	}

	wideX, _ := testui.FindCell(t, headerLines(wide, 220)[headerMenuRow], quit)
	narrowX, _ := testui.FindCell(t, headerLines(narrow, 220)[headerMenuRow], quit)
	if wideX-narrowX != grew {
		t.Fatalf("quit is drawn at column %d, and at %d with the default key; want it %d cells further right", wideX, narrowX, grew)
	}
}

// TestReloadButtonShowsTheKeyOfTheActiveSurface: each surface binds its own
// reload key, and the button names the one that works where the operator is.
func TestReloadButtonShowsTheKeyOfTheActiveSurface(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{config.ShellActionReloadDetail: {"f5"}},
		Board: map[string][]string{config.BoardActionReload: {"f6"}},
	})
	m := newHeaderShell(t, cfg)

	for active, want := range map[mode.ID]string{mode.Board: "reload f6", mode.Docs: "reload f6", mode.Detail: "reload f5"} {
		m.active = active
		if bar := headerLines(m, 120)[headerMenuRow]; !strings.Contains(bar, want+" ") {
			t.Errorf("on %s the bar does not draw %q:\n%s", active, want, bar)
		}
	}
}

// TestStoreButtonNamesTheActiveStore: the first button is the store's name
// with the key that opens the store list, and `stores` when no name is known.
func TestStoreButtonNamesTheActiveStore(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())
	key := m.keys.DisplayPrimary(config.ShellContext, config.ShellActionStorePicker)

	for name, want := range map[string]string{
		"task-manager-ui": "task-manager-ui",
		"":                "stores",
		"   ":             "stores",
	} {
		m.services.StoreName = name
		bar := headerLines(m, 120)[headerMenuRow]
		if !strings.HasPrefix(bar, strings.Repeat(" ", headerMenuStart)+want+" "+key+barSeparator) {
			t.Errorf("with the store name %q the bar does not start with %q and its key:\n%s", name, want, bar)
		}
	}
}

// TestStoreButtonCutsALongName: a store name is as long as its operator made
// it. The bar cuts it to the room it has: to storeLabelMax on a wide bar,
// further before a button drops, and to what is left when it stands alone.
func TestStoreButtonCutsALongName(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())
	m.services.StoreName = "a-store-with-a-name-far-longer-than-any-bar-has-room-for"

	for _, tc := range []struct {
		width, buttons, label int
	}{
		{width: 120, buttons: len(barActions), label: storeLabelMax},
		{width: 80, buttons: len(barActions)},
		{width: 20, buttons: 1},
	} {
		m.width = tc.width
		cells := m.barCells()
		if len(cells) != tc.buttons {
			t.Errorf("at width %d the bar places %d buttons, want %d", tc.width, len(cells), tc.buttons)
			continue
		}
		label := cells[0].label
		kept, cut := strings.CutSuffix(label, "…")
		if !cut || !strings.HasPrefix(m.services.StoreName, kept) || cells[len(cells)-1].x1 > tc.width {
			t.Errorf("at width %d the store label is %q and the bar ends at column %d", tc.width, label, cells[len(cells)-1].x1)
		}
		if tc.label != 0 && lipgloss.Width(label) != tc.label {
			t.Errorf("at width %d the store label %q is %d cells, want %d", tc.width, label, lipgloss.Width(label), tc.label)
		}
		if len(cells) > 1 && lipgloss.Width(label) < storeLabelFloor {
			t.Errorf("at width %d the store label %q is under the floor of %d cells beside other buttons", tc.width, label, storeLabelFloor)
		}
		if bar := headerLines(m, tc.width)[headerMenuRow]; !strings.Contains(bar, label+" ") {
			t.Errorf("at width %d the bar does not draw %q:\n%s", tc.width, label, bar)
		}
	}
}

// TestLongStoreNameGivesABarItsButtonsBackAtTheFloor widens the terminal one
// column at a time under a long name. A button comes back as soon as the name
// has storeLabelFloor cells beside it, and the name is never shorter beside
// another button. The store button alone starts at one cell.
func TestLongStoreNameGivesABarItsButtonsBackAtTheFloor(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())
	m.services.StoreName = "a-store-with-a-name-far-longer-than-any-bar-has-room-for"

	previous := 0
	for width := 0; width <= 120; width++ {
		m.width = width
		cells := m.barCells()
		if len(cells) == 0 {
			continue
		}
		got := lipgloss.Width(cells[0].label)

		want := storeLabelFloor
		if len(cells) == 1 {
			want = 1
		}
		switch {
		case len(cells) > previous && got != want:
			t.Errorf("at width %d the bar gained button %d with the store label at %d cells, want %d", width, len(cells), got, want)
		case got < want:
			t.Errorf("at width %d the store label is %d cells beside %d other buttons, want %d or more", width, got, len(cells)-1, want)
		}
		previous = len(cells)
	}

	if previous != len(barActions) {
		t.Fatalf("at width 120 the bar places %d of %d buttons", previous, len(barActions))
	}
}

// TestNarrowBarDropsAButtonBeforeTheNameGoesUnderItsFloor: a short name is
// never cut while a button can still be dropped.
func TestNarrowBarDropsAButtonBeforeTheNameGoesUnderItsFloor(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())
	m.services.StoreName = "demo"
	m.width = 80
	full := m.barCells()

	m.width = full[len(full)-1].x1 - 1
	cells := m.barCells()
	if len(cells) != len(barActions)-1 || cells[0].label != "demo" {
		t.Errorf("one column short, the bar places %d buttons with the store label %q; want %d and the whole name", len(cells), cells[0].label, len(barActions)-1)
	}
}

// TestHoveredStoreButtonTakesTheCommonHover: the store's name rests bold in the
// buttons' colour, so the pointer changes its colour as it does on any button.
func TestHoveredStoreButtonTakesTheCommonHover(t *testing.T) {
	testui.ForceTrueColor(t)

	m := newHeaderShell(t, config.Default())
	m.width = 120
	rest := m.renderMenuBar()

	x := headerMenuStart
	m.barPointer = &x
	hovered := m.renderMenuBar()

	bold := lipgloss.NewStyle().Bold(true)
	if want := bold.Foreground(styles.ShellActionColor).Render(m.storeLabel()); !strings.HasPrefix(strings.TrimLeft(rest, " "), want) {
		t.Errorf("the store button at rest is not bold in the buttons' colour:\n%q", rest)
	}
	want := bold.Foreground(styles.ShellTabHoverColor).Render(m.storeLabel())
	if hovered == rest || !strings.HasPrefix(strings.TrimLeft(hovered, " "), want) {
		t.Errorf("the hovered store button is not drawn in the hover style:\nrest    %q\nhovered %q", rest, hovered)
	}
	if lipgloss.Width(hovered) != lipgloss.Width(rest) {
		t.Errorf("the hover changes the bar's width from %d to %d", lipgloss.Width(rest), lipgloss.Width(hovered))
	}
}

// TestTabLineHoldsTheTabsAlone: the menu bar names the store, the active tab
// the surface and the highlighted row the selection, so nothing stands right
// of the tabs on any surface.
func TestTabLineHoldsTheTabsAlone(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())

	for _, active := range []mode.ID{mode.Board, mode.Docs, mode.Search, mode.Detail} {
		m.active = active
		for _, width := range []int{headerTabsEnd(), 80, 220} {
			line := headerLines(m, width)[headerTabsRow]
			if lipgloss.Width(line) != headerTabsEnd() || strings.TrimSpace(line) != "Board   Docs" {
				t.Errorf("%s at width %d: want the tabs alone, got %q", active, width, line)
			}
		}
	}
}

// TestWorkspaceFillsTheTerminalUnderTheHeaderWithAndWithoutTheFooter holds the
// frame to the terminal's height. The workspace is what the three header lines
// and the footer leave, and a hidden footer still holds its line: the frame is
// joined from three parts whether or not the last one draws anything.
func TestWorkspaceFillsTheTerminalUnderTheHeaderWithAndWithoutTheFooter(t *testing.T) {
	t.Parallel()

	for _, showFooter := range []bool{true, false} {
		cfg := config.Default()
		cfg.UI.ShowModeSwitcherHelp = showFooter
		m := newHeaderShell(t, cfg)

		for _, size := range []struct{ width, height int }{{80, 24}, {120, 34}, {180, 50}, {30, 12}} {
			m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: size.width, Height: size.height}})

			_, workspace := m.workspaceSize()
			if want := size.height - 4; workspace != want {
				t.Errorf("footer %v at %dx%d: the workspace is %d lines, want %d under a 3-line header and over the footer's line",
					showFooter, size.width, size.height, workspace, want)
			}
			if got := lipgloss.Height(m.renderBody()); got != workspace {
				t.Errorf("footer %v at %dx%d: the surface draws %d lines into a workspace of %d",
					showFooter, size.width, size.height, got, workspace)
			}
			lines := strings.Split(m.View(), "\n")
			if len(lines) != size.height {
				t.Errorf("footer %v at %dx%d: the frame is %d lines", showFooter, size.width, size.height, len(lines))
			}
			if last := strings.TrimSpace(lines[len(lines)-1]); (last != "") != showFooter {
				t.Errorf("footer %v at %dx%d: the last line of the frame is %q", showFooter, size.width, size.height, last)
			}
		}
	}
}
