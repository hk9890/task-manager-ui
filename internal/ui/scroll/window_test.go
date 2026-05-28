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
