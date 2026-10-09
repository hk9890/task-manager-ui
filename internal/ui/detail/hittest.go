package detail

import "strings"

// Hit is the detail cell under a point: the pane, and the reference row drawn
// there. RefID is "" anywhere but on a row of the Dependencies pane.
type Hit struct {
	Pane  FocusPane
	RefID string
}

// rect is a pane's box, borders included.
type rect struct {
	x, y, width, height int
}

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}

// panes is where Render puts each pane at the state's size.
type panes struct {
	dependencies, content, metadata rect
}

func layoutPanes(width, height int) panes {
	if usesResponsiveDetailLayout(width) {
		contentHeight, bottomHeight := splitResponsiveLayoutHeights(height)
		dependenciesWidth, metadataWidth := splitResponsiveBottomWidths(width)
		return panes{
			content:      rect{0, 0, width, contentHeight},
			dependencies: rect{0, contentHeight, dependenciesWidth, bottomHeight},
			metadata:     rect{dependenciesWidth + detailColumnGap, contentHeight, metadataWidth, bottomHeight},
		}
	}

	leftWidth, contentWidth, metadataWidth := splitThreePaneWidths(width)
	contentX := leftWidth + detailColumnGap
	return panes{
		dependencies: rect{0, 0, leftWidth, height},
		content:      rect{contentX, 0, contentWidth, height},
		metadata:     rect{contentX + contentWidth + detailColumnGap, 0, metadataWidth, height},
	}
}

// drawsPanes reports whether Render(state) draws the three panes, rather than
// one of its placeholder texts or the compact form.
func drawsPanes(state State) bool {
	loaded := strings.TrimSpace(state.Detail.Summary.ID) != ""
	return strings.TrimSpace(state.SelectionID) != "" &&
		loaded &&
		strings.TrimSpace(state.Error) == "" &&
		state.Width >= minDetailWidth &&
		!state.Compact
}

// HitTest reports what Render(state) draws at cell (x, y), with (0, 0) the
// first cell of the frame. ok is false in the gaps between panes, and whenever
// Render draws something other than the panes.
func HitTest(state State, x, y int) (hit Hit, ok bool) {
	if state.Width <= 0 {
		state.Width = defaultDetailWidth
	}
	if state.Height <= 0 {
		state.Height = defaultDetailHeight
	}
	if !drawsPanes(state) {
		return Hit{}, false
	}

	p := layoutPanes(state.Width, state.Height)
	switch {
	case p.content.contains(x, y):
		return Hit{Pane: FocusPaneContent}, true
	case p.metadata.contains(x, y):
		return Hit{Pane: FocusPaneMetadata}, true
	case !p.dependencies.contains(x, y):
		return Hit{}, false
	}

	hit = Hit{Pane: FocusPaneDependencies}
	pane := p.dependencies
	innerHeight := max(1, pane.height-2)
	line := y - pane.y - 1
	onBorder := x == pane.x || x == pane.x+pane.width-1
	if state.Skeleton || onBorder || line < 0 || line >= innerHeight {
		return hit, true
	}

	// The window sliceWithOffset draws, whose first and last rows turn into
	// the "… (N earlier)" and "… (N more)" indicators when it clips.
	refs := dependencyLineRefs(dependencyGroups(state.Detail, state.BrowserItems))
	offset := max(0, min(state.DependenciesScrollOffset, len(refs)-innerHeight))
	end := min(len(refs), offset+innerHeight)
	if offset+line >= end {
		return hit, true
	}
	if (offset > 0 && line == 0) || (end < len(refs) && offset+line == end-1) {
		return hit, true
	}
	hit.RefID = refs[offset+line]
	return hit, true
}
