package app

// The geometry of the two header lines: what the top bar keeps as the terminal
// narrows, and that neither line reaches past it. What a click on the bar does
// is in mouse_test.go.
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

// headerLines renders the header at width and returns its lines.
func headerLines(m Model, width int) []string {
	m.width = width
	return strings.Split(m.renderHeader(), "\n")
}

// drawnButtons is the labels of the buttons the top bar draws, left to right,
// read back from the rendered line and not from barCells.
func drawnButtons(t *testing.T, m Model, bar string) []string {
	t.Helper()

	var labels []string
	last := -1
	for _, action := range barActions {
		text := action.label + " " + m.keys.DisplayPrimary(config.ShellContext, action.action)
		if !strings.Contains(bar, text) {
			continue
		}
		x, _ := testui.FindCell(t, bar, text)
		if x <= last {
			t.Fatalf("button %q is drawn at column %d, left of the one before it:\n%s", text, x, bar)
		}
		last = x
		labels = append(labels, action.label)
	}
	return labels
}

// TestHeaderIsTwoLinesNoWiderThanTheTerminal renders the header across the
// widths a terminal is resized through. The workspace height is what is left
// under it, so a third line or a wrapped one pushes the footer off the screen.
func TestHeaderIsTwoLinesNoWiderThanTheTerminal(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())

	for _, active := range []mode.ID{mode.Board, mode.Docs, mode.Search, mode.Detail} {
		m.active = active
		for width := 0; width <= 220; width++ {
			lines := headerLines(m, width)
			if len(lines) != 2 {
				t.Fatalf("%s at width %d: the header is %d lines:\n%s", active, width, len(lines), strings.Join(lines, "\n"))
			}

			// The tabs are never dropped: a terminal narrower than the tab
			// strip cuts them off at its edge. From there up both lines fit.
			if width < headerTabsEnd() {
				continue
			}
			for idx, line := range lines {
				if got := lipgloss.Width(line); got > width {
					t.Fatalf("%s at width %d: header line %d is %d cells wide:\n%s", active, width, idx, got, line)
				}
			}
			for _, label := range []string{" Board ", " Docs ", " Search "} {
				if !strings.Contains(lines[0], label) {
					t.Fatalf("%s at width %d: the top bar lost the tab %q:\n%s", active, width, label, lines[0])
				}
			}
			if len(drawnButtons(t, m, lines[0])) > 0 && lipgloss.Width(lines[0]) != width {
				t.Fatalf("%s at width %d: the buttons are not flush right, the bar is %d cells:\n%s",
					active, width, lipgloss.Width(lines[0]), lines[0])
			}
		}
	}
}

// TestBarButtonsDropLeftmostFirstAndNeverTouchTheTabs narrows the terminal one
// column at a time. The tabs come first: a button that does not fit beside them
// goes, new before stores before help, and what stays keeps its gap.
func TestBarButtonsDropLeftmostFirstAndNeverTouchTheTabs(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())
	all := []string{"new", "stores", "help"}

	// The width each count of buttons first fits at, read from the render.
	firstFit := map[int]int{}
	previous := 0
	for width := 0; width <= 220; width++ {
		bar := headerLines(m, width)[0]
		drawn := drawnButtons(t, m, bar)

		if want := all[len(all)-len(drawn):]; strings.Join(drawn, ",") != strings.Join(want, ",") {
			t.Fatalf("at width %d the bar draws %v, want the rightmost %d of %v:\n%s", width, drawn, len(drawn), all, bar)
		}
		if len(drawn) < previous {
			t.Fatalf("the bar lost a button as the terminal grew to %d: %v", width, drawn)
		}
		if len(drawn) > previous+1 {
			t.Fatalf("the bar gained %d buttons in one column, at width %d", len(drawn)-previous, width)
		}
		if len(drawn) > previous {
			firstFit[len(drawn)] = width
		}
		previous = len(drawn)

		// What barCells says is what is drawn, and none of it is a tab's.
		m.width = width
		cells := m.barCells()
		if len(cells) != len(drawn) {
			t.Fatalf("at width %d barCells places %d buttons and the bar draws %d:\n%s", width, len(cells), len(drawn), bar)
		}
		searchX := 0
		if len(cells) > 0 {
			searchX, _ = testui.FindCell(t, bar, " Search ")
		}
		for _, cell := range cells {
			x, _ := testui.FindCell(t, bar, cell.text())
			if cell.x0 != x || cell.x1 != x+lipgloss.Width(cell.text()) {
				t.Fatalf("at width %d %q is drawn at column %d and placed at [%d, %d)", width, cell.text(), x, cell.x0, cell.x1)
			}
			if gap := cell.x0 - (searchX + len(" Search ")); gap < headerBarGap {
				t.Fatalf("at width %d %q starts %d cells after the last tab, want at least %d:\n%s", width, cell.text(), gap, headerBarGap, bar)
			}
			if cell.x1 > width {
				t.Fatalf("at width %d %q ends at column %d", width, cell.text(), cell.x1)
			}
			for column := cell.x0; column < cell.x1; column++ {
				if tab, ok := m.tabAt(column); ok {
					t.Fatalf("at width %d column %d is both the %q button and the %q tab", width, column, cell.text(), tab)
				}
			}
		}
	}

	if previous != len(all) {
		t.Fatalf("at width 220 the bar draws %d buttons, want all %d", previous, len(all))
	}
	// Exactly fitting: the first width a button is drawn at leaves the gap and
	// not a cell more, and one column less drops it.
	for count := 1; count <= len(all); count++ {
		width := firstFit[count]
		bar := headerLines(m, width)[0]
		m.width = width
		searchX, _ := testui.FindCell(t, bar, " Search ")
		if gap := m.barCells()[0].x0 - (searchX + len(" Search ")); gap != headerBarGap {
			t.Errorf("%d buttons first fit at width %d with a gap of %d cells, want exactly %d:\n%s", count, width, gap, headerBarGap, bar)
		}
		if got := len(drawnButtons(t, m, headerLines(m, width-1)[0])); got != count-1 {
			t.Errorf("at width %d, one short of fitting %d buttons, the bar draws %d", width-1, count, got)
		}
	}
}

// TestBarButtonsMeasureAWideKeyName rebinds help to a key with a long display
// name. The button is placed by what it draws, so it is still flush right and
// the others are dropped sooner to make room for it.
func TestBarButtonsMeasureAWideKeyName(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{config.ShellActionHelp: {"ctrl+alt+f12"}},
	})
	wide := newHeaderShell(t, cfg)
	narrow := newHeaderShell(t, config.Default())

	text := "help " + wide.keys.DisplayPrimary(config.ShellContext, config.ShellActionHelp)
	grew := lipgloss.Width(text) - lipgloss.Width("help "+narrow.keys.DisplayPrimary(config.ShellContext, config.ShellActionHelp))
	if grew <= 0 {
		t.Fatalf("fixture: %q is no wider than the default help button", text)
	}

	for width := headerTabsEnd(); width <= 220; width++ {
		bar := headerLines(wide, width)[0]
		if got := lipgloss.Width(bar); got > width {
			t.Fatalf("at width %d the bar is %d cells wide:\n%s", width, got, bar)
		}
		drawn := drawnButtons(t, wide, bar)
		if len(drawn) == 0 {
			continue
		}
		if !strings.HasSuffix(bar, text) {
			t.Fatalf("at width %d the bar does not end on %q:\n%s", width, text, bar)
		}
		// The same buttons the default bar draws in a terminal narrower by
		// what the key name added.
		if want := drawnButtons(t, narrow, headerLines(narrow, width-grew)[0]); len(drawn) != len(want) {
			t.Fatalf("at width %d the bar draws %v; the default bar draws %v at width %d", width, drawn, want, width-grew)
		}
	}
}

// TestHeaderRuleSpansTheTerminalAndCutsItsContext pins the line under the top
// bar: exactly as wide as the terminal at every width, with the context cut
// when it does not fit and gone when not even one cell of it would.
func TestHeaderRuleSpansTheTerminalAndCutsItsContext(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())

	for _, active := range []mode.ID{mode.Board, mode.Docs, mode.Search, mode.Detail} {
		m.active = active
		for width := 1; width <= 220; width++ {
			m.width = width
			rule := m.renderRule()
			if strings.Contains(rule, "\n") {
				t.Fatalf("%s at width %d: the rule takes more than one line: %q", active, width, rule)
			}
			if got := lipgloss.Width(rule); got != width {
				t.Fatalf("%s at width %d: the rule is %d cells wide: %q", active, width, got, rule)
			}

			context := m.headerContext()
			switch {
			case width <= lipgloss.Width("── ")+1:
				// No room for the lead, one cell of context and the space
				// after it.
				if rule != strings.Repeat("─", width) {
					t.Fatalf("%s at width %d: want a bare rule, got %q", active, width, rule)
				}
			case lipgloss.Width("── "+context+" ") <= width:
				if !strings.HasPrefix(rule, "── "+context+" ") {
					t.Fatalf("%s at width %d: the rule does not carry the context %q: %q", active, width, context, rule)
				}
			default:
				if !strings.HasPrefix(rule, "── ") || !strings.HasSuffix(rule, "… ") {
					t.Fatalf("%s at width %d: want the context cut with an ellipsis at the edge, got %q", active, width, rule)
				}
			}
		}
	}

	// Before the first resize there is no width to fill: the context alone.
	m.width = 0
	if got := m.renderRule(); got != m.headerContext() {
		t.Fatalf("at width 0 the rule is %q, want the context %q", got, m.headerContext())
	}
}

// TestWorkspaceFillsTheTerminalUnderTheHeaderWithAndWithoutTheFooter holds the
// frame to the terminal's height. The workspace is what the two header lines
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
			if want := size.height - 3; workspace != want {
				t.Errorf("footer %v at %dx%d: the workspace is %d lines, want %d under a 2-line header and over the footer's line",
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
