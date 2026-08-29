package board

// The wide-layout breakpoint, asserted at its own width. The goldens sample 80,
// 120 and 180 and so never render 150 itself, which left the comparison free to
// move by a column undetected.
//
// Direct assertions rather than goldens at 149/150: two more snapshots would
// need regenerating on every unrelated visual edit, and the property that
// matters here is the column arithmetic, not the pixels.

import "testing"

// TestMaxVisibleColumnsSwitchesToWideAtTheThreshold pins the breakpoint from
// both sides. The widths are literal: deriving them from wideLayoutThreshold
// would make the test move with the constant and assert nothing about its
// value.
func TestMaxVisibleColumnsSwitchesToWideAtTheThreshold(t *testing.T) {
	t.Parallel()

	if wideLayoutThreshold != 150 {
		t.Fatalf("wideLayoutThreshold = %d, but this test's widths are written for 150", wideLayoutThreshold)
	}

	// At 149 the narrow minimum (32) applies: (149+2)/(32+2) = 4 columns.
	// At 150 the wide minimum (40) applies: (150+2)/(40+2) = 3 columns.
	// The switch makes the board show *fewer, wider* columns once there is room
	// for them to be readable.
	cases := []struct {
		width int
		want  int
	}{
		{149, 4},
		{150, 3},
		{151, 3},
	}

	for _, tc := range cases {
		if got := maxVisibleColumns(tc.width); got != tc.want {
			t.Errorf("maxVisibleColumns(%d) = %d, want %d", tc.width, got, tc.want)
		}
	}
}

// The threshold is about column width, so assert the minimum each side yields
// rather than only the count — a count can coincide across both branches at
// some widths, but the readable minimum cannot.
func TestWideLayoutRaisesTheReadableColumnMinimum(t *testing.T) {
	t.Parallel()

	if minReadableColumn >= minReadableWideCol {
		t.Fatalf("the wide minimum (%d) must exceed the narrow one (%d), or the threshold changes nothing",
			minReadableWideCol, minReadableColumn)
	}

	// One below the threshold the board packs in as many narrow columns as fit;
	// at the threshold the same width must yield strictly fewer.
	narrow := maxVisibleColumns(149)
	wide := maxVisibleColumns(150)
	if wide >= narrow {
		t.Errorf("crossing the threshold did not reduce the column count: 149 -> %d, 150 -> %d", narrow, wide)
	}
}
