package detail

import "strings"

// Hit is the detail cell under a point: the pane, and what is drawn there.
// RefID is "" anywhere but on a row of the Dependencies pane, and Button is
// ButtonNone anywhere but on a button. On a comment, Pane is FocusPaneComments
// and Comment the index of that comment; read Comment only then. On the row
// of an agent in the Details pane, Pane is FocusPaneDetails and Agent the
// session of that agent.
type Hit struct {
	Pane    FocusPane
	RefID   string
	Button  Button
	Comment int
	Agent   string
}

// rect is a pane's box, borders included.
type rect struct {
	x, y, width, height int
}

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}

// panes is where Render puts each pane, at the state's size.
type panes struct {
	dependencies, content, details rect
}

func layoutPanes(width, height int) panes {
	if usesResponsiveDetailLayout(width) {
		contentHeight, bottomHeight := splitResponsiveLayoutHeights(height)
		dependenciesWidth, detailsWidth := splitResponsiveBottomWidths(width)
		return panes{
			content:      rect{0, 0, width, contentHeight},
			dependencies: rect{0, contentHeight, dependenciesWidth, bottomHeight},
			details:      rect{dependenciesWidth + detailColumnGap, contentHeight, detailsWidth, bottomHeight},
		}
	}

	leftWidth, contentWidth, detailsWidth := splitThreePaneWidths(width)
	return panes{
		dependencies: rect{0, 0, leftWidth, height},
		content:      rect{leftWidth + detailColumnGap, 0, contentWidth, height},
		details:      rect{leftWidth + contentWidth + 2*detailColumnGap, 0, detailsWidth, height},
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
// first cell of the frame. ok is false in the gaps between panes, on the
// Details pane off the row of an agent, and whenever Render draws something
// other than the panes.
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
		return hitContent(state, p.content, x, y), true
	case p.details.contains(x, y):
		return hitDetails(state, p.details, x, y)
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

// hitContent is what the Content pane draws at (x, y): a button of the ID
// line, the add button on the Comments heading, a comment, or the buttons
// of the selected comment.
func hitContent(state State, pane rect, x, y int) Hit {
	hit := Hit{Pane: FocusPaneContent}
	if state.Skeleton {
		return hit
	}

	innerHeight := max(1, pane.height-2)
	column := x - pane.x
	line := y - pane.y - 1
	if column == 0 || column == pane.width-1 || line < 0 || line >= innerHeight {
		return hit
	}
	contentPane := state.contentPane(pane.width, pane.height)
	content := renderContentPaneLines(contentPane, pane.width-2, innerHeight)
	// The window sliceWithOffset draws; its two indicator rows are no line.
	total := len(content.lines)
	offset := max(0, min(state.ContentScrollOffset, total-innerHeight))
	end := min(total, offset+innerHeight)
	index := offset + line
	if index >= end || (offset > 0 && line == 0) || (end < total && index == end-1) {
		return hit
	}

	column-- // the border
	if index == 0 {
		for _, placed := range content.idButtons {
			if placed.cells.covers(column) {
				hit.Button = placed.button
			}
		}
		return hit
	}
	if index == content.heading {
		if content.add.covers(column) {
			hit.Button = ButtonAddComment
		}
		return hit
	}
	comment := content.comment[index]
	if comment < 0 {
		return hit
	}
	hit.Pane = FocusPaneComments
	hit.Comment = comment
	firstLine := content.comment[index-1] != comment
	if !contentPane.CommentsFocused || comment != contentPane.CommentIndex || !firstLine {
		return hit
	}
	for idx, button := range []Button{ButtonEditComment, ButtonDeleteComment} {
		if idx < len(content.commentButtons) && content.commentButtons[idx].covers(column) {
			hit.Button = button
		}
	}
	return hit
}

// hitDetails is the agent whose row of the Details pane is at (x, y). ok is
// false anywhere else in the pane: no other line of it is clickable.
func hitDetails(state State, pane rect, x, y int) (hit Hit, ok bool) {
	column := x - pane.x
	line := y - pane.y - 1
	innerHeight := max(1, pane.height-2)
	if state.Skeleton || column == 0 || column == pane.width-1 || line < 0 || line >= innerHeight {
		return Hit{}, false
	}
	_, sessions := detailsPaneLines(state.detailsPane(pane.width, pane.height), pane.width)
	// The window sliceWithOffset draws from the top; its last row is the
	// "… (N more)" indicator when it clips.
	if line >= len(sessions) || (len(sessions) > innerHeight && line == innerHeight-1) {
		return Hit{}, false
	}
	if sessions[line] == "" {
		return Hit{}, false
	}
	return Hit{Pane: FocusPaneDetails, Agent: sessions[line]}, true
}
