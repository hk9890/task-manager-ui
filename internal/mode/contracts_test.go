package mode

import (
	"testing"
	"time"
)

func TestClickTrackerTellsADoubleClickFromTwoSingleOnes(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	// selected is the row the surface holds selected as the click arrives: a
	// click on a row selects it, so the next click sees that row.
	type click struct {
		target   string
		selected string
		x, y     int
		ms       int
		want     bool
	}
	cases := []struct {
		name   string
		clicks []click
	}{
		{name: "two fast clicks on one row", clicks: []click{{"a", "", 5, 5, 0, false}, {"a", "a", 9, 5, 200, true}}},
		{name: "two slow clicks", clicks: []click{{"a", "", 5, 5, 0, false}, {"a", "a", 5, 5, 400, false}}},
		{name: "two rows", clicks: []click{{"a", "", 5, 5, 0, false}, {"b", "a", 5, 6, 100, false}}},
		{name: "a double click is consumed", clicks: []click{{"a", "", 5, 5, 0, false}, {"a", "a", 5, 5, 100, true}, {"a", "a", 5, 5, 200, false}, {"a", "a", 5, 5, 300, true}}},
		// The first click moved the row away: the same cell now holds another
		// row, or none, and the operator still means the row they selected.
		{name: "same cell, another row under it", clicks: []click{{"a", "", 5, 5, 0, false}, {"b", "a", 5, 5, 100, true}}},
		{name: "same cell, no row under it", clicks: []click{{"a", "", 5, 5, 0, false}, {"", "a", 5, 5, 100, true}}},
		{name: "the first click selected nothing", clicks: []click{{"", "", 5, 5, 0, false}, {"", "", 5, 5, 100, false}, {"a", "", 5, 5, 200, false}}},
		// A wheel notch or a key moved the selection off the clicked row:
		// opening the selection would open a row nobody clicked.
		{name: "the selection moved between the clicks", clicks: []click{{"a", "", 5, 5, 0, false}, {"a", "b", 5, 5, 100, false}}},
	}

	for _, tc := range cases {
		var tracker ClickTracker
		for idx, c := range tc.clicks {
			msg := MouseMsg{Kind: MouseClick, X: c.x, Y: c.y, At: start.Add(time.Duration(c.ms) * time.Millisecond)}
			if got := tracker.Double(c.target, c.selected, msg); got != c.want {
				t.Errorf("%s: click %d on %q = %v, want %v", tc.name, idx, c.target, got, c.want)
			}
		}
	}
}

func TestMouseMsgPointerIsItsCellUntilItLeaves(t *testing.T) {
	t.Parallel()

	if got := (MouseMsg{Kind: MouseMove, X: 4, Y: 7}).Pointer(); got == nil || *got != (Pointer{X: 4, Y: 7}) {
		t.Fatalf("a move left the pointer at %v, want cell (4,7)", got)
	}
	if got := (MouseMsg{Kind: MouseLeave, X: 4, Y: 7}).Pointer(); got != nil {
		t.Fatalf("a leave left the pointer at %v, want none", got)
	}
}
