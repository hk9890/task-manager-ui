package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// newShortShell is the mouse shell on tab at 140 columns and height rows.
func newShortShell(t *testing.T, tab mode.ID, height int) Model {
	t.Helper()
	m := send(t, newMouseShell(t), tea.WindowSizeMsg{Width: 140, Height: height})
	if tab != mode.Board {
		x, _ := testui.FindCell(t, tabLine(m), " "+tabLabels[tab]+" ")
		m = send(t, m, onScreen(m, leftClick(x+1, headerTabsRow)))
	}
	if m.active != tab {
		t.Fatalf("fixture: the shell is on %q, want %q", m.active, tab)
	}
	return m
}

// terminalScreen is the frame as the terminal shows it: Bubble Tea draws the
// last rows of a frame taller than the terminal, and a click reports the row
// on the screen.
func terminalScreen(m Model) string {
	lines := strings.Split(m.View(), "\n")
	return strings.Join(lines[max(0, len(lines)-m.marginTop-m.height-m.marginBottom):], "\n")
}

// TestTheFrameIsNeverTallerThanTheTerminal: a renderer keeps a floor of rows
// below which it draws more than the workspace holds. Bubble Tea drops the top
// of a frame taller than the terminal, so the shell must cut it instead, with
// the header on the first rows and the legend on the last.
func TestTheFrameIsNeverTallerThanTheTerminal(t *testing.T) {
	for height := 6; height <= 12; height++ {
		t.Run(fmt.Sprintf("%d rows", height), func(t *testing.T) {
			m := newShortShell(t, mode.Board, height)
			view := testui.AnsiEscapePattern.ReplaceAllString(m.View(), "")
			lines := strings.Split(view, "\n")
			if len(lines) != height {
				t.Fatalf("View() is %d lines in a terminal of %d:\n%s", len(lines), height, view)
			}
			if !strings.Contains(lines[headerMenuRow], "stores") || !strings.Contains(lines[headerTabsRow], "Docs") {
				t.Fatalf("the header is not on the first rows:\n%s", view)
			}
			// A terminal this short keeps the side margin and has no blank row.
			if legend := "  " + testui.AnsiEscapePattern.ReplaceAllString(m.renderFooter(), ""); strings.TrimRight(lines[height-1], " ") != legend {
				t.Fatalf("the last row is %q, want the key legend %q", lines[height-1], legend)
			}
		})
	}
}

// TestClicksLandOnWhatIsDrawnInAShortTerminal: at 12 rows a click on an issue
// title selects that issue and a click on a view tab switches to it.
func TestClicksLandOnWhatIsDrawnInAShortTerminal(t *testing.T) {
	m := newShortShell(t, mode.Board, 12)

	x, y := testui.FindCell(t, terminalScreen(m), "progress-second")
	if m = send(t, m, leftClick(x, y)); firstSelectionID(m, mode.Board) != "tm-2" {
		t.Fatalf("click on the title of tm-2 selected %q", firstSelectionID(m, mode.Board))
	}

	x, y = testui.FindCell(t, terminalScreen(m), " Docs ")
	if y != headerTabsRow {
		t.Fatalf("the view tabs are drawn on row %d, want %d", y, headerTabsRow)
	}
	if m = send(t, m, leftClick(x+1, y)); m.active != mode.Docs {
		t.Fatalf("click on the Docs tab left the shell on %q", m.active)
	}
}
