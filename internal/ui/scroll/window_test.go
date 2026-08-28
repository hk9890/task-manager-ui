package scroll

import "testing"

func TestEnsureVisible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		offset int
		sel    int
		window int
		want   int
	}{
		// sel is already visible — offset unchanged
		{name: "sel inside window stays", offset: 5, sel: 7, window: 5, want: 5},
		{name: "sel at window top stays", offset: 5, sel: 5, window: 5, want: 5},
		{name: "sel at window bottom stays", offset: 5, sel: 9, window: 5, want: 5},

		// sel above current offset — slide window up to sel
		{name: "sel above window slides up", offset: 10, sel: 3, window: 5, want: 3},
		{name: "sel at 0 above offset slides to 0", offset: 5, sel: 0, window: 5, want: 0},

		// sel below window bottom — slide window down so sel is last visible
		{name: "sel below window slides down", offset: 5, sel: 12, window: 5, want: 8},
		{name: "sel exactly at window bottom plus 1", offset: 0, sel: 5, window: 5, want: 1},

		// edge: window = 1
		{name: "window 1 sel matches offset", offset: 3, sel: 3, window: 1, want: 3},
		{name: "window 1 sel above slides up", offset: 3, sel: 1, window: 1, want: 1},
		{name: "window 1 sel below slides down", offset: 3, sel: 5, window: 1, want: 5},

		// edge: window <= 0 treated as 1
		{name: "window 0 treated as 1", offset: 0, sel: 3, window: 0, want: 3},
		{name: "window negative treated as 1", offset: 0, sel: 2, window: -5, want: 2},

		// edge: sel = 0
		{name: "sel 0 offset 0 window 5", offset: 0, sel: 0, window: 5, want: 0},
		{name: "sel 0 above offset 3", offset: 3, sel: 0, window: 5, want: 0},

		// edge: large list, sel at end
		{name: "sel at end of large list", offset: 0, sel: 99, window: 10, want: 90},

		// edge: negative inputs clamped
		{name: "negative offset clamped to 0", offset: -5, sel: 2, window: 5, want: 0},
		{name: "negative sel clamped to 0", offset: 5, sel: -1, window: 5, want: 0},

		// window >= list size scenarios (offset stays 0)
		{name: "window >= total items", offset: 0, sel: 4, window: 100, want: 0},

		// sel exactly at offset+window
		{name: "sel exactly at offset+window boundary", offset: 5, sel: 10, window: 5, want: 6},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := EnsureVisible(tc.offset, tc.sel, tc.window)
			if got != tc.want {
				t.Errorf("EnsureVisible(%d, %d, %d) = %d, want %d",
					tc.offset, tc.sel, tc.window, got, tc.want)
			}
		})
	}
}

func TestEnsureVisibleClipped(t *testing.T) {
	t.Parallel()

	// A pane of `total` lines showing `window` rows replaces its first row with
	// "… (N earlier)" once scrolled, and its last row with "… (N more)" while
	// content remains below. contentRows reports the rows that survive that, so
	// each case asserts the selection lands on one of them.
	contentRows := func(offset, window, total int) (first, last int) {
		first, last = offset, offset+window-1
		if offset > 0 {
			first++
		}
		if offset+window < total {
			last--
		}
		return first, last
	}

	cases := []struct {
		name                       string
		offset, sel, window, total int
		want                       int
	}{
		{"pane fits, no clipping", 0, 3, 10, 5, 0},
		{"already on a content row", 5, 8, 10, 37, 5},
		{"scrolling down clears the bottom indicator", 0, 9, 10, 37, 1},
		{"scrolling up clears the top indicator", 10, 4, 10, 37, 3},
		{"selection at the very top pins offset 0", 5, 0, 10, 37, 0},
		{"selection at the very end pins max offset", 0, 36, 10, 37, 27},
		{"window of one falls back to EnsureVisible", 0, 5, 1, 37, 5},
		{"window of two falls back to EnsureVisible", 0, 5, 2, 37, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := EnsureVisibleClipped(tc.offset, tc.sel, tc.window, tc.total)
			if got != tc.want {
				t.Fatalf("EnsureVisibleClipped(%d, %d, %d, %d) = %d, want %d",
					tc.offset, tc.sel, tc.window, tc.total, got, tc.want)
			}
		})
	}

	// The property that matters: for any starting offset and any selection, the
	// returned offset must put the selection on a row that renders content.
	t.Run("selection always lands on a rendered row", func(t *testing.T) {
		t.Parallel()
		const window, total = 10, 37
		for offset := 0; offset <= total-window; offset++ {
			for sel := 0; sel < total; sel++ {
				got := EnsureVisibleClipped(offset, sel, window, total)
				if got < 0 || got > total-window {
					t.Fatalf("offset %d sel %d: returned %d, outside [0, %d]", offset, sel, got, total-window)
				}
				first, last := contentRows(got, window, total)
				if sel < first || sel > last {
					t.Fatalf("offset %d sel %d: returned %d, but content rows are [%d, %d] — the selection would render under a scroll indicator",
						offset, sel, got, first, last)
				}
			}
		}
	})
}
