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
		text := action.label + " " + action.key(m)
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
			for _, label := range []string{" Board ", " Docs ", " Search "} {
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
// column at a time. The version goes first, then quit, help, reload: stores is
// the last button standing.
func TestMenuBarDropsTheVersionThenTheRightmostButton(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())
	all := []string{"stores", "reload", "help", "quit"}

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
		Shell:  map[string][]string{config.ShellActionReloadDetail: {"f5"}},
		Search: map[string][]string{config.SearchActionReload: {"f6"}},
	})
	m := newHeaderShell(t, cfg)

	for active, want := range map[mode.ID]string{mode.Board: "reload r", mode.Docs: "reload r", mode.Search: "reload f6", mode.Detail: "reload f5"} {
		m.active = active
		if bar := headerLines(m, 120)[headerMenuRow]; !strings.Contains(bar, want+" ") {
			t.Errorf("on %s the bar does not draw %q:\n%s", active, want, bar)
		}
	}
}

// TestTabLineSetsTheContextFlushRightAndCutsIt pins the line of the view tabs:
// the context ends on the last column, keeps its gap from the tabs, and is cut
// with an ellipsis or gone when it does not fit beside them.
func TestTabLineSetsTheContextFlushRightAndCutsIt(t *testing.T) {
	t.Parallel()

	m := newHeaderShell(t, config.Default())

	for _, active := range []mode.ID{mode.Board, mode.Docs, mode.Search, mode.Detail} {
		m.active = active
		for width := headerTabsEnd(); width <= 220; width++ {
			line := headerLines(m, width)[headerTabsRow]
			m.width = width
			context := m.headerContext()
			free := width - headerTabsEnd() - headerBarGap

			switch {
			case free <= 0:
				if lipgloss.Width(line) != headerTabsEnd() {
					t.Fatalf("%s at width %d: want the tabs alone, got %q", active, width, line)
				}
			case lipgloss.Width(context) <= free:
				if !strings.HasSuffix(line, context) || lipgloss.Width(line) != width {
					t.Fatalf("%s at width %d: the context %q is not flush right: %q", active, width, context, line)
				}
			default:
				if !strings.HasSuffix(line, "…") || lipgloss.Width(line) != width {
					t.Fatalf("%s at width %d: want the context cut with an ellipsis at the edge, got %q", active, width, line)
				}
				if gap := line[headerTabsEnd() : headerTabsEnd()+headerBarGap]; strings.TrimSpace(gap) != "" {
					t.Fatalf("%s at width %d: the cut context touches the tabs: %q", active, width, line)
				}
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
