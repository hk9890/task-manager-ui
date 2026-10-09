package storepicker

import (
	"fmt"
	"testing"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func hitTestState(height int) State {
	rows := []Row{{Action: "create-local"}, {Action: "create-central"}}
	for idx := 0; idx < 12; idx++ {
		rows = append(rows, Row{Name: fmt.Sprintf("store-%02d", idx), ProjectPath: "/p", Health: "ok", Usable: true})
	}
	return State{Rows: rows, SelectedRow: -1, Width: 100, Height: height}
}

// TestHitTestFindsEveryRowWhereRenderDrewIt asks the rendered frame where each
// row is: the action rows, a scrolled window, and the rows under a pinned
// error line.
func TestHitTestFindsEveryRowWhereRenderDrewIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		height int
		mutate func(*State)
		drawn  []int
	}{
		{name: "everything fits", height: 24, drawn: []int{0, 1, 2, 13}},
		{name: "scrolled", height: 9, mutate: func(s *State) { s.ScrollOffset = 5 }, drawn: []int{5, 6, 10}},
		{name: "pinned error", height: 24, mutate: func(s *State) { s.Error = "registry unreadable" }, drawn: []int{0, 2, 13}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := hitTestState(tc.height)
			if tc.mutate != nil {
				tc.mutate(&state)
			}
			view := Render(state)

			for _, want := range tc.drawn {
				text := state.Rows[want].Action
				if text == "" {
					text = state.Rows[want].Name
				}
				x, y := testui.FindCell(t, view, text)
				if row, ok := HitTest(state, x, y); !ok || row != want {
					t.Errorf("HitTest at %q (%d,%d) = %d, %v; want row %d", text, x, y, row, ok, want)
				}
			}
		})
	}
}

func TestHitTestReportsNothingOffTheRows(t *testing.T) {
	t.Parallel()

	state := hitTestState(24)
	state.Error = "registry unreadable"
	state.Help = "help-line"
	view := Render(state)

	for _, text := range []string{"Task Stores", "registry unreadable", "help-line"} {
		x, y := testui.FindCell(t, view, text)
		if row, ok := HitTest(state, x, y); ok {
			t.Errorf("HitTest on %q = row %d, want no hit", text, row)
		}
	}

	// Below the last row, and on the side border of a row.
	x, y := testui.FindCell(t, view, "store-11")
	for _, cell := range [][2]int{{x, y + 1}, {0, y}, {99, y}} {
		if row, ok := HitTest(state, cell[0], cell[1]); ok {
			t.Errorf("HitTest at %v = row %d, want no hit", cell, row)
		}
	}

	// A cold listing draws skeleton rows, which are not rows.
	if row, ok := HitTest(State{Loading: true, Width: 100, Height: 24}, 5, 3); ok {
		t.Errorf("HitTest on a skeleton row = row %d, want no hit", row)
	}
}

func TestRenderMarksTheHoveredRow(t *testing.T) {
	t.Parallel()

	state := hitTestState(24)
	_, y := testui.FindCell(t, Render(state), "store-04")
	hover := 6
	state.Hover = &hover
	if _, hoverY := testui.FindCell(t, Render(state), "›"); hoverY != y {
		t.Fatalf("hover chevron drawn on row %d, want row %d", hoverY, y)
	}
}
