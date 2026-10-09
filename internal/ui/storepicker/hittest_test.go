package storepicker

import (
	"fmt"
	"strings"
	"testing"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func hitTestState(height int) State {
	rows := []Row{{Action: "create-local"}, {Action: "create-central"}}
	for idx := 0; idx < 12; idx++ {
		rows = append(rows, Row{Name: fmt.Sprintf("store-%02d", idx), ProjectPath: fmt.Sprintf("/p/%02d", idx), Health: "ok", Usable: true})
	}
	return State{Rows: rows, SelectedRow: -1, Width: 100, Height: height}
}

// TestHitTestFindsEveryRowWhereRenderDrewIt asks the rendered frame where each
// row is: the action rows, a scrolled window, and the rows under a pinned
// error line. A row is rowLines lines and each of them is that row: the name
// line and the path line under it.
func TestHitTestFindsEveryRowWhereRenderDrewIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		height int
		mutate func(*State)
		drawn  []int
	}{
		{name: "everything fits", height: 31, drawn: []int{0, 1, 2, 13}},
		{name: "everything fits with a line to spare", height: 32, drawn: []int{0, 1, 2, 13}},
		{name: "scrolled", height: 9, mutate: func(s *State) { s.ScrollOffset = 5 }, drawn: []int{5, 6, 7}},
		{name: "scrolled with a line to spare", height: 10, mutate: func(s *State) { s.ScrollOffset = 5 }, drawn: []int{5, 6, 7}},
		{name: "pinned error", height: 32, mutate: func(s *State) { s.Error = "registry unreadable" }, drawn: []int{0, 2, 13}},
		{name: "pinned error with a line to spare", height: 33, mutate: func(s *State) { s.Error = "registry unreadable" }, drawn: []int{0, 2, 13}},
		{name: "scrolled under a pinned error", height: 10, mutate: func(s *State) { s.ScrollOffset = 5; s.Error = "registry unreadable" }, drawn: []int{5, 6, 7}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := hitTestState(tc.height)
			if tc.mutate != nil {
				tc.mutate(&state)
			}
			view := Render(state)

			x, y := 0, 0
			for _, want := range tc.drawn {
				text := state.Rows[want].Action
				if text == "" {
					text = state.Rows[want].Name
				}
				x, y = testui.FindCell(t, view, text)
				if row, ok := HitTest(state, x, y); !ok || row != want {
					t.Errorf("HitTest at %q (%d,%d) = %d, %v; want row %d", text, x, y, row, ok, want)
				}

				// The second line: a store's path, and nothing on an action row.
				if path := state.Rows[want].ProjectPath; path != "" {
					if _, pathY := testui.FindCell(t, view, path); pathY != y+1 {
						t.Fatalf("row %d: the path is on line %d, want the line under the name (%d)", want, pathY, y+1)
					}
				}
				if row, ok := HitTest(state, x, y+1); !ok || row != want {
					t.Errorf("HitTest on the second line of %q (%d,%d) = %d, %v; want row %d", text, x, y+1, row, ok, want)
				}
			}

			// The window ends with the last drawn row: the line under it is the
			// spare line or the border, and the next row is not on screen.
			last := tc.drawn[len(tc.drawn)-1]
			if row, ok := HitTest(state, x, y+rowLines); ok {
				t.Errorf("HitTest under the last drawn row = row %d, want no hit", row)
			}
			if last+1 < len(state.Rows) && strings.Contains(view, state.Rows[last+1].Name) {
				t.Errorf("row %d is drawn past the window:\n%s", last+1, view)
			}
		})
	}
}

// TestHitTestStaysInsideAFrameTooShortForOneRow covers the height at which the
// box has one content line: the row's name is drawn, its path is not, and the
// line where the path would be is the bottom border.
func TestHitTestStaysInsideAFrameTooShortForOneRow(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		height int
		err    string
	}{
		{height: 4},
		{height: 5, err: "registry unreadable"},
	} {
		state := hitTestState(tc.height)
		state.Error = tc.err
		state.Help = "help-line"
		view := Render(state)

		x, y := testui.FindCell(t, view, "create-local")
		if row, ok := HitTest(state, x, y); !ok || row != 0 {
			t.Errorf("height %d: HitTest on the one drawn line = %d, %v; want row 0", tc.height, row, ok)
		}
		if _, borderY := testui.FindCell(t, view, "╰"); borderY != y+1 {
			t.Fatalf("height %d: the bottom border is on line %d, want %d:\n%s", tc.height, borderY, y+1, view)
		}
		if row, ok := HitTest(state, x, y+1); ok {
			t.Errorf("height %d: HitTest on the bottom border = row %d, want no hit", tc.height, row)
		}
	}
}

func TestHitTestReportsNothingOffTheRows(t *testing.T) {
	t.Parallel()

	state := hitTestState(40)
	state.Error = "registry unreadable"
	state.Help = "help-line"
	view := Render(state)

	for _, text := range []string{"Task Stores", "registry unreadable", "help-line"} {
		x, y := testui.FindCell(t, view, text)
		if row, ok := HitTest(state, x, y); ok {
			t.Errorf("HitTest on %q = row %d, want no hit", text, row)
		}
	}

	// Below the last row, and on the side border of either line of a row.
	x, y := testui.FindCell(t, view, "store-11")
	for _, cell := range [][2]int{{x, y + rowLines}, {0, y}, {99, y}, {0, y + 1}, {99, y + 1}} {
		if row, ok := HitTest(state, cell[0], cell[1]); ok {
			t.Errorf("HitTest at %v = row %d, want no hit", cell, row)
		}
	}

	// A cold listing draws skeleton rows, which are not rows.
	if row, ok := HitTest(State{Loading: true, Width: 100, Height: 24}, 5, 3); ok {
		t.Errorf("HitTest on a skeleton row = row %d, want no hit", row)
	}
}

func TestRenderBandsTheSelectedAndTheHoveredRow(t *testing.T) {
	testui.ForceTrueColor(t)

	state := hitTestState(24)
	state.SelectedRow = 3
	hover := 6
	state.Hover = &hover
	view := Render(state)

	selected, hovered := testui.RowBand(t, view, "store-01"), testui.RowBand(t, view, "store-04")
	if selected == "" || hovered == "" || selected == hovered {
		t.Fatalf("bands: selected %q, hovered %q; want two different backgrounds", selected, hovered)
	}
	// The band covers the row, so the path line carries it too.
	if path := testui.RowBand(t, view, "/p/01"); path != selected {
		t.Fatalf("the selected row's path line carries %q, want the selection band %q", path, selected)
	}
	if path := testui.RowBand(t, view, "/p/04"); path != hovered {
		t.Fatalf("the hovered row's path line carries %q, want the hover band %q", path, hovered)
	}
	if plain := testui.RowBand(t, view, "store-02"); plain != "" {
		t.Fatalf("a plain row carries the band %q", plain)
	}
	// An action row is a row too.
	hover = 0
	if band := testui.RowBand(t, Render(state), "create-local"); band != hovered {
		t.Fatalf("a hovered action row carries %q, want the hover band %q", band, hovered)
	}
}
