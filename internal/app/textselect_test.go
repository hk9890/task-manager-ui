package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
)

func leftDrag(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft}
}

func leftRelease(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
}

// newSelectingShell is the mouse shell with the clipboard replaced by a slice.
func newSelectingShell(t *testing.T) (Model, *[]string) {
	t.Helper()
	m := newMouseShell(t)
	var copied []string
	m.copyText = func(text string) tea.Cmd {
		copied = append(copied, text)
		return nil
	}
	return m, &copied
}

// TestDragSelectsABoxOfTheScreenAndCopiesIt drags across two rows of two
// columns: the box is drawn in reverse video over the screen as it was, and
// the release sends its text to the clipboard, one line per screen line, and
// says what it sent.
func TestDragSelectsABoxOfTheScreenAndCopiesIt(t *testing.T) {
	m, copied := newSelectingShell(t)

	// Focus the column with a key first: at this width the press would
	// re-centre the board, and the drag is measured on the settled screen.
	m = send(t, m, tea.KeyMsg{Type: tea.KeyRight})
	x, y := testui.FindCell(t, m.View(), "progress-first")
	m = send(t, m, leftClick(x, y))
	frozen := m.View()
	endX, endY := testui.FindCell(t, frozen, "progress-second")
	endX += len("progress-second") - 1

	m = send(t, m, leftDrag(endX, endY))
	if !m.sel.active {
		t.Fatal("a drag of more than two cells did not start a selection")
	}
	dragging := m.View()
	if !strings.Contains(dragging, "\x1b[0;7m") {
		t.Fatalf("the selected box is not drawn in reverse video:\n%s", dragging)
	}
	if got := testui.AnsiEscapePattern.ReplaceAllString(dragging, ""); got != testui.AnsiEscapePattern.ReplaceAllString(frozen, "") {
		t.Fatalf("drawing the box moved text on the screen:\n%s", got)
	}

	m = send(t, m, leftRelease(endX, endY))
	if m.sel.active || m.press != nil {
		t.Fatal("the selection stayed after the button came up")
	}
	if len(*copied) != 1 {
		t.Fatalf("release copied %d times, want once", len(*copied))
	}
	lines := strings.Split((*copied)[0], "\n")
	// An issue row is issuerow.Height lines, so the box from one title to the
	// next spans the first row's second line as well.
	if want := endY - y + 1; want != issuerow.Height+1 || len(lines) != want ||
		!strings.HasPrefix(lines[0], "progress-first") || !strings.HasSuffix(lines[len(lines)-1], "progress-second") {
		t.Fatalf("copied %q, want the %d lines of the box from one title to the next", (*copied)[0], want)
	}
	// OSC 52 has no reply, so the toast says what was sent and names the way
	// that needs no clipboard support — whole, in an 80-column terminal too.
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	toast := m.View()
	if strings.Contains(toast, "Copied") || !strings.Contains(toast, "Sent") || !strings.Contains(toast, "shift+drag if not copied") {
		t.Fatalf("the toast must say the text was sent and name shift+drag, without claiming a copy:\n%s", toast)
	}
}

// TestAShiftOfOneCellDuringAClickIsStillAClick: the hand moves a little while
// the button is down. That must not start a selection or copy two characters.
func TestAShiftOfOneCellDuringAClickIsStillAClick(t *testing.T) {
	m, copied := newSelectingShell(t)

	x, y := testui.FindCell(t, m.View(), "progress-second")
	m = send(t, m, leftClick(x, y))
	m = send(t, m, leftDrag(x+1, y))
	m = send(t, m, leftRelease(x+1, y))
	if m.sel.active || len(*copied) != 0 {
		t.Fatalf("a one-cell shift selected text (copied %q)", *copied)
	}
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("the click selected %q, want tm-2", got)
	}
}

// TestNothingChangesTheScreenUnderASelection: while the box is up, the wheel,
// another click and every key but Escape and quit are dropped, and Escape
// drops the selection without copying.
func TestNothingChangesTheScreenUnderASelection(t *testing.T) {
	m, copied := newSelectingShell(t)

	x, y := testui.FindCell(t, m.View(), "progress-first")
	m = send(t, m, leftClick(x, y))
	m = send(t, m, leftDrag(x+10, y+1))
	selecting := m.View()
	selection := firstSelectionID(m, mode.Board)

	for name, msg := range map[string]tea.Msg{
		"wheel":     wheel(x, y, tea.MouseButtonWheelDown),
		"click":     tea.MouseMsg{X: 5, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonRight},
		"move key":  tea.KeyMsg{Type: tea.KeyDown},
		"tab key":   tea.KeyMsg{Type: tea.KeyTab},
		"help key":  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")},
		"close key": tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")},
	} {
		next, cmd := m.Update(msg)
		m = next.(Model)
		if cmd != nil || m.View() != selecting || firstSelectionID(m, mode.Board) != selection || m.active != mode.Board {
			t.Errorf("%s changed the screen under the selection", name)
		}
	}

	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.sel.active || m.press != nil || len(*copied) != 0 {
		t.Fatalf("Escape left the selection up or copied it (copied %q)", *copied)
	}
	if m.active != mode.Board {
		t.Fatalf("Escape during a selection also left the surface, now on %q", m.active)
	}

	// The release that follows belongs to no drag.
	if m = send(t, m, leftRelease(x+10, y+1)); len(*copied) != 0 {
		t.Fatal("a release after Escape copied text")
	}
}

// TestAReleaseElsewhereForgetsThePress: motion with no button held says the
// button came up over another window.
func TestAReleaseElsewhereForgetsThePress(t *testing.T) {
	m, copied := newSelectingShell(t)

	x, y := testui.FindCell(t, m.View(), "progress-first")
	m = send(t, m, leftClick(x, y))
	m = send(t, m, leftDrag(x+10, y+1))
	m = send(t, m, pointerMove(x+12, y+1))
	if m.sel.active || m.press != nil || len(*copied) != 0 {
		t.Fatalf("the selection outlived a release in another window (copied %q)", *copied)
	}
}

// TestABoxOfBlankCellsCopiesNothing: an empty copy would clear the clipboard.
func TestABoxOfBlankCellsCopiesNothing(t *testing.T) {
	m, copied := newSelectingShell(t)

	// Inside the Not Ready column, which is empty in this fixture.
	x, y := testui.FindCell(t, m.View(), "Not Ready")
	m = send(t, m, leftClick(x+2, y+3))
	m = send(t, m, leftDrag(x+12, y+5))
	if !m.sel.active {
		t.Fatal("fixture: the drag did not start a selection")
	}
	m = send(t, m, leftRelease(x+12, y+5))
	if len(*copied) != 0 || strings.Contains(m.View(), "clipboard") {
		t.Fatalf("a blank box was copied: %q", *copied)
	}
}

// TestABoxEdgeInsideAWideGlyphTakesTheWholeGlyph: a glyph two cells wide under
// an edge of the box is selected whole. Cut in half it was dropped on one side
// and kept on the other, so the row moved a cell and the copy lost the glyph.
func TestABoxEdgeInsideAWideGlyphTakesTheWholeGlyph(t *testing.T) {
	t.Parallel()

	// The wide glyphs sit on cells 2-3 and 7-8.
	const line = "ab\x1b[31m✅cd\x1b[0m 日 tail"
	plain := testui.AnsiEscapePattern.ReplaceAllString(line, "")

	for left := 0; left <= 8; left++ {
		for right := left; right <= 12; right++ {
			sel := textSelection{from: screenCell{x: left}, to: screenCell{x: right}, screen: line}
			if got := testui.AnsiEscapePattern.ReplaceAllString(sel.view(), ""); got != plain {
				t.Errorf("box %d-%d moved the row: %q, want %q", left, right, got, plain)
			}
		}
	}

	for _, tc := range []struct {
		left, right int
		want        string
	}{
		{left: 0, right: 2, want: "ab✅"},
		{left: 3, right: 4, want: "✅c"},
		{left: 3, right: 7, want: "✅cd 日"},
		{left: 4, right: 5, want: "cd"},
	} {
		sel := textSelection{from: screenCell{x: tc.left}, to: screenCell{x: tc.right}, screen: line}
		if got := sel.text(); got != tc.want {
			t.Errorf("box %d-%d copied %q, want %q", tc.left, tc.right, got, tc.want)
		}
	}
}

// TestSelectingWorksOverTheHelpOverlay: the box stands over whatever is on
// screen, so the help text can be copied too.
func TestSelectingWorksOverTheHelpOverlay(t *testing.T) {
	m, copied := newSelectingShell(t)
	m = pressKey(t, m, "?")

	x, y := testui.FindCell(t, m.View(), "Mode switching:")
	m = send(t, m, leftClick(x, y))
	m = send(t, m, leftDrag(x+len("Mode switching:")-1, y))
	m = send(t, m, leftRelease(x+len("Mode switching:")-1, y))
	if len(*copied) != 1 || (*copied)[0] != "Mode switching:" {
		t.Fatalf("copied %q, want the help line", *copied)
	}
	if !m.showHelp {
		t.Fatal("selecting text closed the help overlay")
	}
}
