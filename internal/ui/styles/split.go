package styles

// The list-and-preview split of a tab: a list of rows on the left and a pane
// on the right that shows the selected row.
const (
	// SplitMinWidth is the narrowest screen that draws both columns. A
	// narrower screen draws the list alone.
	SplitMinWidth = 100
	// SplitGap is the blank cells between the two columns, the gap of the
	// board columns.
	SplitGap = 2

	splitListMin = 40
	splitListMax = 56
)

// SplitWidths is the width of the list and of the preview on a screen width
// cells wide: a third of it for the list, held between splitListMin and
// splitListMax, and the rest less SplitGap for the preview. split is false
// below SplitMinWidth, and the list then takes the whole width.
func SplitWidths(width int) (list, preview int, split bool) {
	if width < SplitMinWidth {
		return width, 0, false
	}
	list = min(max(width/3, splitListMin), splitListMax)
	return list, width - list - SplitGap, true
}
