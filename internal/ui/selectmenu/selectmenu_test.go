package selectmenu

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

func sampleState(width int) State {
	return State{
		Title:   "Sent 12 characters to the clipboard",
		Hint:    "shift+drag if not copied",
		Width:   width,
		Entries: []Entry{{Label: "Quote"}, {Label: "Search", Disabled: true}, {Label: "Close", Key: "esc"}},
		Hover:   -1,
	}
}

func TestPlaceKeepsTheMenuOnTheScreen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		x, y         int
		wantX, wantY int
	}{
		{name: "room below and right", x: 10, y: 5, wantX: 11, wantY: 6},
		{name: "right edge puts it left of the cell", x: 75, y: 5, wantX: 55, wantY: 6},
		{name: "bottom edge puts it above the cell", x: 10, y: 22, wantX: 11, wantY: 14},
		{name: "a corner does both", x: 79, y: 23, wantX: 59, wantY: 15},
		{name: "near the top left with no room above", x: 2, y: 3, wantX: 3, wantY: 4},
	}
	for _, tc := range cases {
		left, top := Place(tc.x, tc.y, 20, 8, 80, 24)
		if left != tc.wantX || top != tc.wantY {
			t.Errorf("%s: placed at %d,%d, want %d,%d", tc.name, left, top, tc.wantX, tc.wantY)
		}
	}
	if left, top := Place(5, 5, 20, 8, 10, 4); left != 0 || top != 0 {
		t.Errorf("a screen smaller than the menu placed it at %d,%d, want 0,0", left, top)
	}
}

func TestHitTestReportsTheEntryRenderDrewThere(t *testing.T) {
	t.Parallel()

	state := sampleState(80)
	view := Render(state)
	for i, label := range []string{"Quote", "Search", "Close"} {
		x, y := testui.FindCell(t, view, label)
		if entry, inside := HitTest(state, x, y); !inside || entry != i {
			t.Errorf("%s drawn at %d,%d: HitTest reports entry %d, inside %v", label, x, y, entry, inside)
		}
	}
	x, y := testui.FindCell(t, view, "Sent")
	if entry, inside := HitTest(state, x, y); !inside || entry != -1 {
		t.Errorf("the title is entry %d, inside %v: want no entry, on the menu", entry, inside)
	}
	width, height := Size(state)
	if _, inside := HitTest(state, width, height-1); inside {
		t.Error("the cell right of the menu is on it")
	}
}

// TestRenderIsCutToTheScreen: a screen narrower than the menu's text gets a
// menu no wider than itself, every line of it.
func TestRenderIsCutToTheScreen(t *testing.T) {
	t.Parallel()

	state := sampleState(24)
	width, _ := Size(state)
	if width != 24 {
		t.Fatalf("Size reports %d columns on a 24-column screen", width)
	}
	for i, line := range strings.Split(Render(state), "\n") {
		if got := lipgloss.Width(line); got != width {
			t.Errorf("line %d is %d wide, want %d: %q", i, got, width, line)
		}
	}
}

// background is the escape sequence Lip Gloss starts a band of a colour role
// with.
func background(role color.Color) string {
	on, _, _ := strings.Cut(lipgloss.NewStyle().Background(role).Render("x"), "x")
	return on
}

// TestRenderBandsTheCursorAndTheHoverAndMutesADisabledEntry: the cursor
// entry has the selection band, the hovered one the quieter hover band, and a
// disabled entry is drawn muted and lit by neither. Not parallel: the colour
// profile is process-wide.
func TestRenderBandsTheCursorAndTheHoverAndMutesADisabledEntry(t *testing.T) {
	testui.ForceTrueColor(t)

	selected, hover := background(styles.RowSelectedBgColor), background(styles.RowHoverBgColor)
	state := sampleState(80)
	line := func(text string) string {
		view := Render(state)
		_, y := testui.FindCell(t, view, text)
		return strings.Split(view, "\n")[y]
	}

	state.Cursor, state.Hover = 0, 2
	if quote := line("Quote"); !strings.Contains(quote, selected) {
		t.Errorf("the cursor entry has no selection band: %q", quote)
	}
	if closeLine := line("Close"); !strings.Contains(closeLine, hover) || strings.Contains(closeLine, selected) {
		t.Errorf("the hovered entry has no hover band, or the selection's: %q", closeLine)
	}

	state.Hover = 1
	search := line("Search")
	if !strings.Contains(search, lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Search")) {
		t.Errorf("the disabled entry is not muted: %q", search)
	}
	if strings.Contains(search, hover) || strings.Contains(search, selected) {
		t.Errorf("the pointer lit a disabled entry: %q", search)
	}
}
