package search

// Hit is the search cell under a point: the pane, and the result drawn there.
// Row is -1 anywhere but on a result row.
type Hit struct {
	Pane FocusPane
	Row  int
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
	query, results, content, metadata rect
}

func layoutPanes(state State) panes {
	width := state.Width
	if width <= 0 {
		width = defaultSearchWidth
	}
	height := state.Height
	if height <= 0 {
		height = defaultSearchHeight
	}
	resultsHeight := max(6, height-searchQueryHeight)

	if width >= searchWideMinWidth {
		rail, content, metadata := splitWideWidths(width)
		contentX := rail + searchColumnGap
		return panes{
			query:    rect{0, 0, rail, searchQueryHeight},
			results:  rect{0, searchQueryHeight, rail, resultsHeight},
			content:  rect{contentX, 0, content, height},
			metadata: rect{contentX + content + searchColumnGap, 0, metadata, height},
		}
	}

	left, right := splitNarrowWidths(width)
	contentHeight, metadataHeight := splitNarrowRightHeights(height)
	rightX := left + searchColumnGap
	return panes{
		query:    rect{0, 0, left, searchQueryHeight},
		results:  rect{0, searchQueryHeight, left, resultsHeight},
		content:  rect{rightX, 0, right, contentHeight},
		metadata: rect{rightX, contentHeight, right, metadataHeight},
	}
}

// HitTest reports what Render(state) draws at cell (x, y), with (0, 0) the
// first cell of the frame. ok is false in the gaps between panes.
func HitTest(state State, x, y int) (hit Hit, ok bool) {
	p := layoutPanes(state)
	switch {
	case p.query.contains(x, y):
		return Hit{Pane: FocusQuery, Row: -1}, true
	case p.content.contains(x, y):
		return Hit{Pane: FocusContent, Row: -1}, true
	case p.metadata.contains(x, y):
		return Hit{Pane: FocusMetadata, Row: -1}, true
	case !p.results.contains(x, y):
		return Hit{}, false
	}

	hit = Hit{Pane: FocusResults, Row: -1}
	line := y - p.results.y - 1
	onBorder := x == p.results.x || x == p.results.x+p.results.width-1
	if onBorder || line < 0 || line >= p.results.height-2 {
		return hit, true
	}
	// The banner and the blank line under it sit above the rows.
	if banner := renderResultsBanner(state, p.results.width-2); len(banner) > 0 {
		line -= len(banner) + 1
	}
	if line >= 0 && line < len(state.Results) {
		hit.Row = line
	}
	return hit, true
}
