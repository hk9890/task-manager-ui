package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// marginSurfaces is every screen the app draws, each reached from the mouse
// shell the way an operator reaches it.
var marginSurfaces = map[string]func(*testing.T, Model) Model{
	"board":        func(_ *testing.T, m Model) Model { return m },
	"docs":         func(t *testing.T, m Model) Model { return pressKey(t, m, "tab") },
	"store search": func(t *testing.T, m Model) Model { return pressKey(t, m, "alt+f") },
	"detail":       func(t *testing.T, m Model) Model { return pressKey(t, m, "enter") },
	"store picker": func(t *testing.T, m Model) Model { return pressKey(t, m, "alt+s") },
	"config":       func(t *testing.T, m Model) Model { return pressKey(t, m, "alt+c") },
	"help":         func(t *testing.T, m Model) Model { return pressKey(t, m, "alt+h") },
	"startup error": func(_ *testing.T, m Model) Model {
		m.fatalErrTitle = "no task-manager store here"
		m.fatalErrBody = strings.Repeat("No task-manager store resolved for this directory. ", 8)
		return m
	},
	"toast": func(_ *testing.T, m Model) Model {
		m.showToast(strings.Repeat("a toast wider than the terminal ", 8), toaster.StyleInfo)
		return m
	},
	// Stepped by hand: an open dialog schedules a repeating cursor tick.
	"modal": func(_ *testing.T, m Model) Model {
		next, _ := m.Update(testKey("delete"))
		return next.(Model)
	},
}

// fullScreenSurfaces are the ones that draw on the first and the last row of
// the screen: the rest centre a box or stand over one of these.
var fullScreenSurfaces = []string{"board", "docs", "store search", "detail", "store picker", "config"}

// terminalRows is a frame as the terminal shows it, without colour: one string
// a row, each padded to the terminal's width.
func terminalRows(t *testing.T, name, view string, width, height int) []string {
	t.Helper()

	rows := strings.Split(testui.AnsiEscapePattern.ReplaceAllString(view, ""), "\n")
	if len(rows) != height {
		t.Fatalf("%s: the frame is %d rows in a terminal of %d:\n%s", name, len(rows), height, view)
	}
	for idx, row := range rows {
		if got := lipgloss.Width(row); got > width {
			t.Fatalf("%s: row %d is %d cells wide in a terminal of %d:\n%s", name, idx, got, width, row)
		}
		rows[idx] = row + strings.Repeat(" ", width-lipgloss.Width(row))
	}
	return rows
}

func blank(cells string) bool {
	return strings.TrimSpace(cells) == ""
}

// assertSideMargin holds the first two and the last two columns of every row
// blank.
func assertSideMargin(t *testing.T, name string, rows []string, width int) {
	t.Helper()

	for idx, row := range rows {
		if left, right := ansi.Cut(row, 0, 2), ansi.Cut(row, width-2, width); !blank(left) || !blank(right) {
			t.Errorf("%s: row %d draws %q and %q in the side margin:\n%s", name, idx, left, right, row)
		}
	}
}

func TestEverySurfaceIsDrawnInsideTheMargin(t *testing.T) {
	const width, height = 120, 30

	for name, reach := range marginSurfaces {
		m := reach(t, send(t, newMouseShell(t), tea.WindowSizeMsg{Width: width, Height: height}))
		rows := terminalRows(t, name, m.View(), width, height)

		assertSideMargin(t, name, rows, width)
		if !blank(rows[0]) || !blank(rows[height-1]) {
			t.Errorf("%s: the first row is %q and the last %q, want both blank", name, rows[0], rows[height-1])
		}
		if blank(strings.Join(rows, "")) {
			t.Errorf("%s: the frame is blank", name)
		}
	}

	// A surface that fills the screen starts on the first cell inside the
	// margin and ends on the last row inside it, so the margin is two columns
	// and one row and not more.
	for _, name := range fullScreenSurfaces {
		m := marginSurfaces[name](t, send(t, newMouseShell(t), tea.WindowSizeMsg{Width: width, Height: height}))
		rows := terminalRows(t, name, m.View(), width, height)
		if blank(rows[1]) || blank(rows[height-2]) {
			t.Errorf("%s: the first row of the screen is %q and the last %q, want both drawn", name, rows[1], rows[height-2])
		}
	}

	m := send(t, newMouseShell(t), tea.WindowSizeMsg{Width: width, Height: height})
	rows := terminalRows(t, "board", m.View(), width, height)
	if rule := strings.TrimRight(rows[2], " "); rule != "   "+strings.Repeat("─", width-5) {
		t.Errorf("the rule under the menu bar does not run from the bar's first label to the margin: %q", rule)
	}
	if !strings.HasPrefix(rows[1], "   stores ") || !strings.HasSuffix(rows[1], "dev  ") {
		t.Errorf("the menu bar does not run from margin to margin: %q", rows[1])
	}
}

// TestToastAndModalSitInsideTheMargin: both are placed on the screen, so
// neither is centred on the terminal nor reaches into its last row.
func TestToastAndModalSitInsideTheMargin(t *testing.T) {
	const width, height = 120, 30
	base := send(t, newMouseShell(t), tea.WindowSizeMsg{Width: width, Height: height})

	toast := marginSurfaces["toast"](t, base)
	if !strings.Contains(toast.View(), "a toast wider than the terminal") {
		t.Fatalf("fixture: no toast on screen:\n%s", toast.View())
	}
	modal := marginSurfaces["modal"](t, base)
	if !modal.showActionModal {
		t.Fatal("fixture: the close dialog did not open")
	}

	for name, m := range map[string]Model{"toast": toast, "modal": modal} {
		over := terminalRows(t, name, m.View(), width, height)
		under := terminalRows(t, name, base.View(), width, height)

		// The cells the overlay changed are the box it drew.
		left, right, top, bottom := width, -1, height, -1
		for y := range over {
			for x := 0; x < width; x++ {
				if ansi.Cut(over[y], x, x+1) != ansi.Cut(under[y], x, x+1) {
					left, right, top, bottom = min(left, x), max(right, x), min(top, y), max(bottom, y)
				}
			}
		}
		if left < 2 || right > width-3 || top < 1 || bottom > height-2 {
			t.Errorf("%s: the box covers columns %d-%d and rows %d-%d, outside the screen", name, left, right, top, bottom)
		}
		// Centred on the screen is centred on the terminal: the margin is the
		// same on both sides.
		if spare := (left - 2) - (width - 3 - right); spare < -1 || spare > 1 {
			t.Errorf("%s: the box covers columns %d-%d, not centred between the margins", name, left, right)
		}
	}
}

func TestATerminalUnderAHundredColumnsHasNoMargin(t *testing.T) {
	const width, height = 99, 30

	for _, name := range fullScreenSurfaces {
		m := marginSurfaces[name](t, send(t, newMouseShell(t), tea.WindowSizeMsg{Width: width, Height: height}))
		rows := terminalRows(t, name, m.View(), width, height)
		if blank(rows[0]) || blank(rows[height-1]) {
			t.Errorf("%s: the first row is %q and the last %q, want both drawn", name, rows[0], rows[height-1])
		}
	}

	m := send(t, newMouseShell(t), tea.WindowSizeMsg{Width: width, Height: height})
	rows := terminalRows(t, "board", m.View(), width, height)
	if !strings.HasPrefix(rows[headerMenuRow], " stores ") {
		t.Errorf("the menu bar does not start on the terminal's first cell: %q", rows[headerMenuRow])
	}
	if rule := rows[1]; rule != " "+strings.Repeat("─", width-1) {
		t.Errorf("the rule does not run from the bar's first label to the terminal's edge: %q", rule)
	}

	// A click lands where it is drawn there too.
	x, y := testui.FindCell(t, m.View(), " Docs ")
	if m = send(t, m, leftClick(x, y)); y != headerTabsRow || m.active != mode.Docs {
		t.Errorf("a click on the Docs tab, drawn on row %d, left the shell on %q", y, m.active)
	}
}

func TestATerminalUnderTwentyFourRowsKeepsTheSideMarginAlone(t *testing.T) {
	const width, height = 120, 23

	for _, name := range fullScreenSurfaces {
		m := marginSurfaces[name](t, send(t, newMouseShell(t), tea.WindowSizeMsg{Width: width, Height: height}))
		rows := terminalRows(t, name, m.View(), width, height)

		assertSideMargin(t, name, rows, width)
		if blank(rows[0]) || blank(rows[height-1]) {
			t.Errorf("%s: the first row is %q and the last %q, want both drawn", name, rows[0], rows[height-1])
		}
	}

	m := send(t, newMouseShell(t), tea.WindowSizeMsg{Width: width, Height: height})
	x, y := testui.FindCell(t, m.View(), " Docs ")
	if m = send(t, m, leftClick(x, y)); y != headerTabsRow || x != 2+headerTabsEnd()-len(" Docs ") || m.active != mode.Docs {
		t.Errorf("a click on the Docs tab, drawn at column %d of row %d, left the shell on %q", x, y, m.active)
	}
}

// TestTheMouseLandsOnWhatIsDrawnInsideTheMargin finds each thing in the frame
// the terminal shows and sends the event to that cell: the shell must take the
// margin off before it asks what is drawn there.
func TestTheMouseLandsOnWhatIsDrawnInsideTheMargin(t *testing.T) {
	base := newMouseShell(t)
	view := base.View()

	tabX, tabY := testui.FindCell(t, view, " Docs ")
	if tabY != 1+headerTabsRow || tabX != 2+headerTabsEnd()-len(" Docs ") {
		t.Fatalf("fixture: the Docs tab is drawn at column %d of row %d, not inside a margin", tabX, tabY)
	}
	// Both padding cells of the tab, and the cell on either side of it.
	for _, x := range []int{tabX, tabX + len(" Docs ") - 1} {
		if m := send(t, base, pointerMove(x, tabY)); m.hoverTab != mode.Docs {
			t.Errorf("the pointer on column %d of the Docs tab lights %q", x, m.hoverTab)
		}
		if m := send(t, base, leftClick(x, tabY)); m.active != mode.Docs {
			t.Errorf("a click on column %d of the Docs tab left the shell on %q", x, m.active)
		}
	}
	for _, cell := range [][2]int{{tabX - 1, tabY}, {tabX + len(" Docs "), tabY}, {tabX, tabY - 1}, {tabX, tabY + 1}} {
		if m := send(t, base, leftClick(cell[0], cell[1])); m.active != mode.Board || m.hoverTab != "" {
			t.Errorf("a click beside the Docs tab, on column %d of row %d, reached it (surface %q, hover %q)", cell[0], cell[1], m.active, m.hoverTab)
		}
	}

	helpX, barY := testui.FindCell(t, view, "help alt+h")
	if barY != 1+headerMenuRow {
		t.Fatalf("fixture: the menu bar is drawn on row %d, not under a blank row", barY)
	}
	// The button is a space wider than its words on each side.
	for _, x := range []int{helpX - 1, helpX, helpX + len("help alt+h")} {
		if m := send(t, base, pointerMove(x, barY)); litButton(m) != "help" {
			t.Errorf("the pointer on column %d of the help button lights %q", x, litButton(m))
		}
		if m := send(t, base, leftClick(x, barY)); !m.showHelp {
			t.Errorf("a click on column %d of the help button did not open help", x)
		}
	}
	for _, cell := range [][2]int{{helpX - 2, barY}, {helpX + len("help alt+h") + 1, barY}, {helpX, barY - 1}, {helpX, barY + 1}} {
		if m := send(t, base, leftClick(cell[0], cell[1])); m.showHelp || litButton(m) != "" {
			t.Errorf("a click beside the help button, on column %d of row %d, reached it", cell[0], cell[1])
		}
	}
	// The first button's leading space is the first cell of the screen.
	if m := send(t, base, leftClick(2, barY)); m.active != mode.StorePicker {
		t.Errorf("a click on the first cell inside the margin left the shell on %q, want the store picker", m.active)
	}

	rowX, rowY := testui.FindCell(t, view, "progress-second")
	hovered := send(t, base, pointerMove(rowX, rowY))
	if testui.RowBand(t, hovered.View(), "progress-second") == "" || testui.RowBand(t, hovered.View(), "progress-first") != "" {
		t.Error("the pointer on a row does not light that row and it alone")
	}
	if m := send(t, base, leftClick(rowX, rowY)); firstSelectionID(m, mode.Board) != "tm-2" {
		t.Errorf("a click on the row of tm-2 selected %q", firstSelectionID(m, mode.Board))
	}
	if m := send(t, base, wheel(rowX, rowY, tea.MouseButtonWheelDown)); firstSelectionID(m, mode.Board) != "tm-2" {
		t.Errorf("a wheel notch over the column of tm-4 and tm-2 selected %q, want tm-2", firstSelectionID(m, mode.Board))
	}
}

// TestTheMarginTakesNoMouseEvent: nothing is drawn there, so nothing is under
// the pointer, and a surface that had it lets it go.
func TestTheMarginTakesNoMouseEvent(t *testing.T) {
	base := newMouseShell(t)
	rowX, rowY := testui.FindCell(t, base.View(), "progress-second")
	_, barY := testui.FindCell(t, base.View(), "help alt+h")

	margin := map[string][2]int{
		"left of a row":       {1, rowY},
		"right of a row":      {158, rowY},
		"left of the bar":     {1, barY},
		"right of the bar":    {159, barY},
		"above the bar":       {rowX, 0},
		"under the legend":    {rowX, 29},
		"the top left corner": {0, 0},
	}
	for name, cell := range margin {
		lit := send(t, base, pointerMove(rowX, rowY))
		if left := send(t, lit, pointerMove(cell[0], cell[1])); left.View() != base.View() {
			t.Errorf("%s: the row stayed lit after the pointer moved into the margin", name)
		}
		for _, event := range []tea.MouseMsg{leftClick(cell[0], cell[1]), leftClick(cell[0], cell[1]), wheel(cell[0], cell[1], tea.MouseButtonWheelDown)} {
			m := send(t, base, event)
			if m.active != mode.Board || firstSelectionID(m, mode.Board) != "tm-1" || litButton(m) != "" || m.hoverTab != "" {
				t.Errorf("%s: an event in the margin reached the shell (surface %q, selection %q, hover %q/%q)",
					name, m.active, firstSelectionID(m, mode.Board), m.hoverTab, litButton(m))
			}
		}
	}

	// Detail and the picker are inside the margin as well.
	for name, key := range map[string]string{"detail": "enter", "the picker": "alt+s"} {
		under := pressKey(t, base, key)
		for where, cell := range margin {
			if m := send(t, send(t, under, leftClick(cell[0], cell[1])), leftClick(cell[0], cell[1])); m.View() != under.View() {
				t.Errorf("%s: a click %s, in the margin, changed the screen", name, where)
			}
		}
	}
}

// TestDragCopiesTheTextUnderTheBoxInsideTheMargin drags over one title as the
// terminal shows it: the box is drawn on those cells and the copy is that text.
func TestDragCopiesTheTextUnderTheBoxInsideTheMargin(t *testing.T) {
	const title = "progress-second"
	m, copied := newSelectingShell(t)

	// Select the row with the keys first, so the press of the drag, which is a
	// click on it, moves nothing.
	m = send(t, send(t, m, tea.KeyMsg{Type: tea.KeyRight}), tea.KeyMsg{Type: tea.KeyDown})
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("fixture: the keys selected %q, want tm-2", got)
	}
	x, y := testui.FindCell(t, m.View(), title)
	m = send(t, m, leftClick(x, y))
	frozen := m.View()
	if againX, againY := testui.FindCell(t, frozen, title); againX != x || againY != y {
		t.Fatalf("fixture: the press moved the row from %d,%d to %d,%d", x, y, againX, againY)
	}

	m = send(t, m, leftDrag(x+len(title)-1, y))
	line := strings.Split(m.View(), "\n")[y]
	before, box, found := strings.Cut(line, "\x1b[0;7m")
	if !found || lipgloss.Width(before) != x || !strings.HasPrefix(box, title+"\x1b[0m") {
		t.Fatalf("the box is not drawn over the title at column %d:\n%q", x, line)
	}
	if got, want := testui.AnsiEscapePattern.ReplaceAllString(m.View(), ""), testui.AnsiEscapePattern.ReplaceAllString(frozen, ""); got != want {
		t.Fatalf("drawing the box moved text on the screen:\n%s", got)
	}

	m = send(t, m, leftRelease(x+len(title)-1, y))
	if len(*copied) != 1 || (*copied)[0] != title {
		t.Fatalf("copied %q, want %q", *copied, title)
	}

	// A drag that runs on into the margin stops at the edge of the screen:
	// the margin stays blank and the copy is the line to its end.
	// On a shell of its own: a second press on the cell would open the row.
	m, copied = newSelectingShell(t)
	m = send(t, send(t, m, tea.KeyMsg{Type: tea.KeyRight}), tea.KeyMsg{Type: tea.KeyDown})
	m = send(t, m, leftClick(x, y))
	m = send(t, m, leftDrag(159, 29))
	rows := terminalRows(t, "drag into the margin", m.View(), 160, 30)
	assertSideMargin(t, "drag into the margin", rows, 160)
	boxed := 0
	for _, line := range strings.Split(m.View(), "\n") {
		before, box, found := strings.Cut(line, "\x1b[0;7m")
		if !found {
			continue
		}
		boxed++
		box, _, _ = strings.Cut(box, "\x1b[0m")
		if end := lipgloss.Width(before) + lipgloss.Width(box); end != 158 {
			t.Fatalf("the box ends at column %d, want the screen's edge at 158:\n%q", end, line)
		}
	}
	if boxed != 29-y {
		t.Fatalf("the box covers %d rows, want the %d from the title's row to the last row of the screen", boxed, 29-y)
	}
	m = send(t, m, leftRelease(159, 29))
	if len(*copied) != 1 || !strings.HasPrefix((*copied)[0], title) {
		t.Fatalf("copied %q, want the text from the title to the corner of the screen", *copied)
	}
	wantLines := 29 - y
	if got := strings.Count((*copied)[0], "\n") + 1; got != wantLines {
		t.Errorf("the copy is %d lines, want the %d from the title's row to the last row of the screen", got, wantLines)
	}

	// And one that begins in the margin begins at the screen's first cell.
	m, copied = newSelectingShell(t)
	storeX, barY := testui.FindCell(t, m.View(), "stores alt+s")
	m = send(t, m, leftClick(0, 0))
	m = send(t, m, leftDrag(storeX+len("stores alt+s")-1, barY))
	send(t, m, leftRelease(storeX+len("stores alt+s")-1, barY))
	if len(*copied) != 1 || (*copied)[0] != " stores alt+s" {
		t.Fatalf("copied %q, want the bar from its first cell to the end of the store button", *copied)
	}
}
