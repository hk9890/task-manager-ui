package mode

import (
	"testing"
	"time"
)

func TestClickTrackerTellsADoubleClickFromTwoSingleOnes(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	type click struct {
		target string
		x, y   int
		ms     int
		want   bool
	}
	cases := []struct {
		name   string
		clicks []click
	}{
		{name: "two fast clicks on one row", clicks: []click{{"a", 5, 5, 0, false}, {"a", 9, 5, 200, true}}},
		{name: "two slow clicks", clicks: []click{{"a", 5, 5, 0, false}, {"a", 5, 5, 400, false}}},
		{name: "two rows", clicks: []click{{"a", 5, 5, 0, false}, {"b", 5, 6, 100, false}}},
		{name: "a double click is consumed", clicks: []click{{"a", 5, 5, 0, false}, {"a", 5, 5, 100, true}, {"a", 5, 5, 200, false}, {"a", 5, 5, 300, true}}},
		// The first click moved the row away: the same cell now holds another
		// row, or none, and the operator still means the row they selected.
		{name: "same cell, another row under it", clicks: []click{{"a", 5, 5, 0, false}, {"b", 5, 5, 100, true}}},
		{name: "same cell, no row under it", clicks: []click{{"a", 5, 5, 0, false}, {"", 5, 5, 100, true}}},
		{name: "the first click selected nothing", clicks: []click{{"", 5, 5, 0, false}, {"", 5, 5, 100, false}, {"a", 5, 5, 200, false}}},
	}

	for _, tc := range cases {
		var tracker ClickTracker
		for idx, c := range tc.clicks {
			msg := MouseMsg{Kind: MouseClick, X: c.x, Y: c.y, At: start.Add(time.Duration(c.ms) * time.Millisecond)}
			if got := tracker.Double(c.target, msg); got != c.want {
				t.Errorf("%s: click %d on %q = %v, want %v", tc.name, idx, c.target, got, c.want)
			}
		}
	}
}
