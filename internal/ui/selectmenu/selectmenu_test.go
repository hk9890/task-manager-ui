package selectmenu

import (
	"testing"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

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

	state := State{
		Title:   "Sent 12 characters to the clipboard",
		Hint:    "shift+drag if not copied",
		Entries: []Entry{{Label: "Quote"}, {Label: "Search", Disabled: true}, {Label: "Close", Key: "esc"}},
		Hover:   -1,
	}
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
