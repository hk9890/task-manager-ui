package styles

import "testing"

func TestSplitWidthsGivesTheListAThirdHeldBetweenItsBounds(t *testing.T) {
	for _, tc := range []struct {
		width, list, preview int
		split                bool
	}{
		{SplitMinWidth - 1, SplitMinWidth - 1, 0, false},
		{SplitMinWidth, splitListMin, SplitMinWidth - splitListMin - SplitGap, true},
		{150, 50, 98, true},
		{240, splitListMax, 240 - splitListMax - SplitGap, true},
	} {
		list, preview, split := SplitWidths(tc.width)
		if list != tc.list || preview != tc.preview || split != tc.split {
			t.Errorf("SplitWidths(%d) = %d, %d, %v; want %d, %d, %v",
				tc.width, list, preview, split, tc.list, tc.preview, tc.split)
		}
	}
}
