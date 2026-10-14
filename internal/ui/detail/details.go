package detail

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/scroll"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/markdown"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/renderhelpers"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	defaultDetailWidth  = 80
	defaultDetailHeight = 24
	timeLayout          = "2006-01-02 15:04"

	// Kept for compatibility with existing tests/callers.
	InspectorTwoColumnMinWidth   = 110
	InspectorThreeColumnMinWidth = 140

	// minDetailWidth is the narrowest terminal the panes are drawn at.
	minDetailWidth = 30

	detailColumnGap   = 2
	metadataRailWidth = 32
	leftRailMinWidth  = 24
	leftRailMaxWidth  = 44
)

var readOnlyMarkdown = markdown.NewRenderer()

// State is the issue detail renderer input.
type State struct {
	SelectionID            string
	TargetID               string
	Detail                 domain.IssueDetail
	BrowserItems           []domain.IssueReference
	BrowserSelectedIssueID string // cursor row (movable by ↑/↓); marked with the app-wide selection gutter
	ButtonKeys             ButtonKeys
	Loading                bool
	Skeleton               bool // when true, Content pane renders skeleton rows instead of description body
	SkeletonPhase          int  // color-cycle index for skeleton pulse; see loading.SkeletonPhase
	Error                  string
	Width                  int
	Height                 int
	Compact                bool
	FocusPane              FocusPane
	// CommentIndex is the selected comment, counted in OrderedComments. It is
	// drawn, with its buttons, only while FocusPane is FocusPaneComments.
	CommentIndex             int
	ContentScrollOffset      int
	DependenciesScrollOffset int
	// Hover is the cell under the pointer, as HitTest reported it; nil when the
	// pointer is elsewhere. Its reference row draws the hover band, and its
	// button is lit.
	Hover *Hit
}

// FocusPane identifies which detail pane is visually focused.
type FocusPane int

const (
	FocusPaneContent FocusPane = iota
	FocusPaneDependencies
	// FocusPaneComments is the comments of the Content pane: a stop of its own
	// after Content, with a cursor on one comment.
	FocusPaneComments

	// Backward-compatible alias.
	FocusPaneBrowser = FocusPaneDependencies
)

// ScrollOffsets describes max scroll bounds for each pane that scrolls. The
// Metadata pane does not.
type ScrollOffsets struct {
	Dependencies int
	Content      int

	// Inner heights are the visible content rows of each pane — total pane
	// height minus its two border rows — at the width and height the bounds
	// were computed for. They are returned alongside the bounds so a controller
	// clamping scroll never has to re-derive the responsive split; that
	// arithmetic lives here, next to the code that draws the panes.
	DependenciesInnerHeight int
	ContentInnerHeight      int
}

// ButtonKeys is the key each button of the Content pane shows.
type ButtonKeys struct {
	EditIssue     string
	AddComment    string
	EditComment   string
	DeleteComment string
}

// Render renders standalone issue detail metadata and content.
func Render(state State) string {
	selected := displaytext.OneLine(strings.TrimSpace(state.SelectionID))
	if selected == "" {
		return "No selected issue.\nSelect an issue on a browse tab first."
	}

	if state.Loading && strings.TrimSpace(state.Detail.Summary.ID) == "" {
		// Cold start: no prior detail is available yet.  Render a skeleton
		// placeholder so the layout stays stable without a full-screen takeover.
		target := selected
		if strings.TrimSpace(state.TargetID) != "" {
			target = state.TargetID
		}
		height := state.Height
		if height <= 0 {
			height = defaultDetailHeight
		}
		return renderColdStartSkeleton(target, state.Width, height, state.SkeletonPhase)
	}
	// Refresh (same or different previously-loaded issue): fall through to the
	// normal detail layout so stale content stays visible while the spinner
	// in the header signals the in-flight request.

	width := state.Width
	if width <= 0 {
		width = defaultDetailWidth
	}

	if strings.TrimSpace(state.Error) != "" {
		// The error is the store's own text and can be longer than the view.
		message := fmt.Sprintf("Failed to load details for %s.\nError: %s", selected, displaytext.OneLine(state.Error))
		return strings.Join(textutil.WrapAtSpaces(message, width), "\n")
	}

	detail := state.Detail
	if strings.TrimSpace(detail.Summary.ID) == "" {
		return fmt.Sprintf("No detail loaded for %s yet.", selected)
	}

	height := state.Height
	if height <= 0 {
		height = defaultDetailHeight
	}

	if width < minDetailWidth {
		return "Terminal too narrow"
	}

	if state.Compact {
		return renderCompact(detail, width)
	}

	var rendered string
	if usesResponsiveDetailLayout(width) {
		rendered = renderResponsiveLayout(detail, state, width, height)
	} else {
		rendered = renderThreePane(detail, state, width, height)
	}

	// Refresh with stale content: tint the pane block so the user sees local
	// motion while waiting for the new detail to arrive. The tint cycles with
	// the skeleton phase so refresh matches the established animation language.
	if state.Loading && strings.TrimSpace(state.Detail.Summary.ID) != "" {
		n := len(styles.SkeletonShades)
		idx := state.SkeletonPhase % n
		if idx < 0 {
			idx = 0
		}
		rendered = lipgloss.NewStyle().Faint(true).Foreground(styles.SkeletonShades[idx]).Render(rendered)
	}

	return rendered
}

// MaxScrollOffsets returns deterministic scroll bounds for pane interactions.
func MaxScrollOffsets(state State) ScrollOffsets {
	width := state.Width
	if width <= 0 {
		width = defaultDetailWidth
	}
	height := state.Height
	if height <= 0 {
		height = defaultDetailHeight
	}

	if usesResponsiveDetailLayout(width) {
		contentHeight, bottomHeight := splitResponsiveLayoutHeights(height)
		dependenciesWidth, _ := splitResponsiveBottomWidths(width)

		contentInnerHeight := max(1, contentHeight-2)
		bottomInnerHeight := max(1, bottomHeight-2)
		deps := renderDependenciesPaneLines(state.Detail, state.BrowserItems, "", dependenciesWidth-2, state.Skeleton, state.SkeletonPhase)
		content := renderContentPaneLines(state.contentPane(width, contentHeight), width-2, contentInnerHeight).lines

		return ScrollOffsets{
			Dependencies:            max(0, len(deps)-bottomInnerHeight),
			Content:                 max(0, len(content)-contentInnerHeight),
			DependenciesInnerHeight: bottomInnerHeight,
			ContentInnerHeight:      contentInnerHeight,
		}
	}

	leftWidth, contentWidth, _ := splitThreePaneWidths(width)
	innerHeight := max(1, height-2)

	deps := renderDependenciesPaneLines(state.Detail, state.BrowserItems, "", leftWidth-2, state.Skeleton, state.SkeletonPhase)
	content := renderContentPaneLines(state.contentPane(contentWidth, height), contentWidth-2, innerHeight).lines

	return ScrollOffsets{
		Dependencies:            max(0, len(deps)-innerHeight),
		Content:                 max(0, len(content)-innerHeight),
		DependenciesInnerHeight: innerHeight,
		ContentInnerHeight:      innerHeight,
	}
}

func usesResponsiveDetailLayout(width int) bool {
	return width < InspectorTwoColumnMinWidth
}

func renderResponsiveLayout(detail domain.IssueDetail, state State, width, height int) string {
	contentHeight, bottomHeight := splitResponsiveLayoutHeights(height)
	dependenciesWidth, metadataWidth := splitResponsiveBottomWidths(width)

	contentBox := RenderContentPane(state.contentPane(width, contentHeight))
	dependenciesBox := renderDependenciesPane(detail, state, dependenciesWidth, bottomHeight)
	metadataBox := RenderMetadataPane(MetadataPaneState{
		Detail:   detail,
		Width:    metadataWidth,
		Height:   bottomHeight,
		Skeleton: state.Skeleton,
	})

	contentLines := strings.Split(contentBox, "\n")
	dependencyLines := strings.Split(dependenciesBox, "\n")
	metadataLines := strings.Split(metadataBox, "\n")

	bottomLines := make([]string, 0, bottomHeight)
	for i := 0; i < bottomHeight; i++ {
		dependenciesLine := ""
		if i < len(dependencyLines) {
			dependenciesLine = dependencyLines[i]
		}
		metadataLine := ""
		if i < len(metadataLines) {
			metadataLine = metadataLines[i]
		}
		bottomLines = append(bottomLines,
			textutil.PadToWidth(dependenciesLine, dependenciesWidth)+strings.Repeat(" ", detailColumnGap)+textutil.PadToWidth(metadataLine, metadataWidth),
		)
	}

	return strings.Join(append(contentLines, bottomLines...), "\n")
}

func renderDependenciesPane(detail domain.IssueDetail, state State, width, height int) string {
	innerHeight := max(1, height-2)
	dependencies := renderRelationshipGroups(dependencyGroups(detail, state.BrowserItems), state.marks(), width-2, state.Skeleton, state.SkeletonPhase)
	dependenciesView, _ := sliceWithOffset(dependencies, state.DependenciesScrollOffset, innerHeight, width-2)
	totalRefs := countDependencyReferences(detail)
	var topRight string
	switch {
	case state.Skeleton:
		topRight = issuerow.SkeletonGlyph
	case len(dependenciesView) < len(dependencies):
		topRight = fmt.Sprintf("%d of %d", len(dependenciesView), len(dependencies))
	default:
		topRight = fmt.Sprintf("%d", totalRefs)
	}
	return styles.FormSection(styles.FormSectionConfig{
		Width:              width,
		Height:             height,
		TopLeft:            "Dependencies",
		TopRight:           topRight,
		Content:            dependenciesView,
		Focused:            state.FocusPane == FocusPaneDependencies,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

func splitResponsiveLayoutHeights(total int) (content, bottom int) {
	if total <= 0 {
		total = defaultDetailHeight
	}
	if total <= 6 {
		content = max(3, total-3)
		bottom = max(3, total-content)
		if content+bottom > total {
			content = max(1, total-bottom)
		}
		return content, total - content
	}

	content = max(8, (total*3)/5)
	bottom = total - content
	if bottom < 6 {
		shift := 6 - bottom
		content = max(3, content-shift)
		bottom = total - content
	}
	return content, bottom
}

func splitResponsiveBottomWidths(total int) (dependencies, metadata int) {
	available := total - detailColumnGap
	if available < 2 {
		available = 2
	}

	metadata = min(metadataRailWidth, max(20, available/2))
	if metadata > available-20 {
		metadata = max(1, available-20)
	}
	dependencies = available - metadata
	if dependencies < 1 {
		dependencies = 1
		metadata = available - dependencies
	}

	return dependencies, metadata
}

func renderThreePane(detail domain.IssueDetail, state State, width, height int) string {
	leftWidth, contentWidth, metadataWidth := splitThreePaneWidths(width)

	depGroups := dependencyGroups(detail, state.BrowserItems)
	deps := renderRelationshipGroups(depGroups, state.marks(), leftWidth-2, state.Skeleton, state.SkeletonPhase)
	innerHeight := max(1, height-2)
	depView, _ := sliceWithOffset(deps, state.DependenciesScrollOffset, innerHeight, leftWidth-2)
	totalDepRefs := countDependencyReferences(detail)
	var depTopRight string
	switch {
	case state.Skeleton:
		depTopRight = issuerow.SkeletonGlyph
	case len(depView) < len(deps):
		depTopRight = fmt.Sprintf("%d of %d", len(depView), len(deps))
	default:
		depTopRight = fmt.Sprintf("%d", totalDepRefs)
	}
	leftBox := styles.FormSection(styles.FormSectionConfig{
		Width:              leftWidth,
		Height:             height,
		TopLeft:            "Dependencies",
		TopRight:           depTopRight,
		Content:            depView,
		Focused:            state.FocusPane == FocusPaneDependencies,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	contentBox := RenderContentPane(state.contentPane(contentWidth, height))

	metaBox := RenderMetadataPane(MetadataPaneState{
		Detail:   detail,
		Width:    metadataWidth,
		Height:   height,
		Skeleton: state.Skeleton,
	})

	leftLines := strings.Split(leftBox, "\n")
	contentLines := strings.Split(contentBox, "\n")
	metaLines := strings.Split(metaBox, "\n")

	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		leftLine := ""
		if i < len(leftLines) {
			leftLine = leftLines[i]
		}
		contentLine := ""
		if i < len(contentLines) {
			contentLine = contentLines[i]
		}
		metaLine := ""
		if i < len(metaLines) {
			metaLine = metaLines[i]
		}
		out = append(out,
			textutil.PadToWidth(leftLine, leftWidth)+strings.Repeat(" ", detailColumnGap)+
				textutil.PadToWidth(contentLine, contentWidth)+strings.Repeat(" ", detailColumnGap)+
				textutil.PadToWidth(metaLine, metadataWidth),
		)
	}

	return strings.Join(out, "\n")
}

// ContentPaneState is the state RenderContentPane draws from.
type ContentPaneState struct {
	Detail       domain.IssueDetail
	Width        int
	Height       int
	Focused      bool
	ScrollOffset int
	// Skeleton renders ▓-filled placeholder rows instead of the real
	// description, bypassing markdown rendering.
	Skeleton bool
	// SkeletonPhase is the colour-cycle index for the skeleton pulse; see
	// loading.SkeletonPhase.
	SkeletonPhase int
	ButtonKeys    ButtonKeys
	// CommentsFocused draws the comment at CommentIndex as selected, with its
	// buttons.
	CommentsFocused bool
	CommentIndex    int
	// Hover is the button under the pointer.
	Hover Button
}

// contentPane is the Content pane of state at the size the layout gives it.
// Render, HitTest and the scroll bounds build the same value, so a click lands
// on what is drawn under it.
func (state State) contentPane(width, height int) ContentPaneState {
	pane := ContentPaneState{
		Detail:          state.Detail,
		Width:           width,
		Height:          height,
		Focused:         state.FocusPane == FocusPaneContent || state.FocusPane == FocusPaneComments,
		ScrollOffset:    state.ContentScrollOffset,
		Skeleton:        state.Skeleton,
		SkeletonPhase:   state.SkeletonPhase,
		ButtonKeys:      state.ButtonKeys,
		CommentsFocused: state.FocusPane == FocusPaneComments,
		CommentIndex:    state.CommentIndex,
	}
	if state.Hover != nil {
		pane.Hover = state.Hover.Button
	}
	return pane
}

// RenderContentPane renders the shared detail Content pane section.
func RenderContentPane(state ContentPaneState) string {
	width, height := state.Width, state.Height
	if width <= 0 {
		width = defaultDetailWidth
	}
	if height <= 0 {
		height = defaultDetailHeight
	}

	innerHeight := max(1, height-2)
	content := renderContentPaneLines(state, width-2, innerHeight)
	contentView, _ := sliceWithOffset(content.lines, state.ScrollOffset, innerHeight, width-2)
	topRight := fmt.Sprintf("%d comments", len(state.Detail.Comments))
	if state.Skeleton {
		topRight = issuerow.SkeletonGlyph + issuerow.SkeletonGlyph + " comments"
	}
	return styles.FormSection(styles.FormSectionConfig{
		Width:              width,
		Height:             height,
		TopLeft:            "Content",
		TopRight:           topRight,
		Content:            contentView,
		Focused:            state.Focused,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

// MetadataPaneState is the state RenderMetadataPane draws from.
type MetadataPaneState struct {
	Detail domain.IssueDetail
	Width  int
	Height int
	// Skeleton renders the Counts group as placeholder glyphs instead of real
	// numeric values, matching the Content pane's loading-state treatment.
	Skeleton bool
}

// RenderMetadataPane renders the shared detail Metadata pane section. The pane
// only shows: it takes no focus and does not scroll, so a pane too short for
// its lines ends on the "more" indicator.
func RenderMetadataPane(state MetadataPaneState) string {
	width, height := state.Width, state.Height
	if width <= 0 {
		width = defaultDetailWidth
	}
	if height <= 0 {
		height = defaultDetailHeight
	}

	metadata := renderMetadataRail(state.Detail, width-2, state.Skeleton)
	metaView, _ := sliceWithOffset(metadata, 0, max(1, height-2), width-2)
	return styles.FormSection(styles.FormSectionConfig{
		Width:   width,
		Height:  height,
		TopLeft: "Metadata",
		Content: metaView,
	})
}

func splitThreePaneWidths(total int) (left, content, metadata int) {
	available := total - (detailColumnGap * 2)
	if available < 3 {
		available = 3
	}

	metadata = metadataRailWidth
	left = textutil.Clamp(available/4, leftRailMinWidth, leftRailMaxWidth)
	content = available - left - metadata

	const minContent = 20
	if content < minContent {
		need := minContent - content
		reduceLeft := min(need, max(0, left-14))
		left -= reduceLeft
		need -= reduceLeft

		reduceMetadata := min(need, max(0, metadata-14))
		metadata -= reduceMetadata
		need -= reduceMetadata

		if need > 0 {
			left = max(8, left-need/2)
			metadata = max(8, metadata-(need-need/2))
		}
	}

	if left < 1 {
		left = 1
	}
	if metadata < 1 {
		metadata = 1
	}
	content = available - left - metadata
	if content < 1 {
		content = 1
	}

	return
}

// Compatibility helpers retained for existing tests.
func splitInspectorWidths(total int) (content, metadata int) {
	left, center, right := splitThreePaneWidths(total)
	_ = left
	return center, right
}

// Compatibility helper retained for existing tests.
func splitThreeColumnWidths(total int) (left, content, metadata int) {
	return splitThreePaneWidths(total)
}

// contentHeaderMetaRow renders the compact, dashboard-styled metadata row shown at the
// top of the Content pane: type, priority, and status tokens (each in its board color,
// reusing the shared renderhelpers) followed by the full issue ID (muted). It mirrors
// the compact issue rows used on the board so the detail header reads consistently.
// When the styled row would exceed width, it falls back to a width-bounded plain row.
func contentHeaderMetaRow(summary domain.IssueSummary, width int) string {
	id := displaytext.OneLine(emptyFallback(summary.ID, "(unknown)"))
	meta := strings.Join([]string{
		renderhelpers.CompactIssueTypeStyled(summary.Type),
		renderhelpers.CompactPriorityStyled(summary.Priority),
		renderhelpers.CompactIssueStateStyled(summary.Status),
		styles.IssueIDMutedStyle.Render(id),
	}, " ")
	if width > 0 && lipgloss.Width(meta) > width {
		plain := strings.Join([]string{
			renderhelpers.CompactIssueType(summary.Type),
			renderhelpers.CompactPriority(summary.Priority),
			renderhelpers.CompactIssueState(summary.Status),
			id,
		}, " ")
		return textutil.TruncateString(plain, width)
	}
	return meta
}

// contentHeaderRule renders the thin full-width horizontal divider that separates the
// Content pane header (meta row + title) from the description body, in the border colour.
// The style is built on each call: a package variable would keep the colour of the theme
// the styles package starts on, whatever styles.Apply assigned after it.
func contentHeaderRule(width int) string {
	if width < 1 {
		return ""
	}
	return lipgloss.NewStyle().Foreground(styles.BorderDefaultColor).Render(strings.Repeat("─", width))
}

// contentLines is the lines of the Content pane before the scroll window, and
// what the mouse can press on them.
type contentLines struct {
	lines []string
	// comment is the index of the comment each line belongs to, -1 for none.
	comment []int
	// heading is the line of the Comments heading, -1 in skeleton state.
	heading int
	// edit is the cells of the edit button on the first line, the ID line.
	// add is the cells of the add button on the heading line. commentButtons
	// is the cells of edit and delete on the first line of the selected
	// comment. Each is empty when the button is not drawn.
	edit           cells
	add            cells
	commentButtons []cells
}

func renderContentPaneLines(pane ContentPaneState, width, availableHeight int) contentLines {
	detail := pane.Detail
	keys := pane.ButtonKeys.withDefaults()
	upper := make([]string, 0, 48)

	// Header: dashboard-styled meta row (type · priority · status · muted id), then the
	// title, then a thin rule that visually separates the header from the body below.
	// The ID is not cut to make room for the button: its key still edits.
	idLine := contentHeaderMetaRow(detail.Summary, width)
	edit := paneButton("edit", keys.EditIssue, pane.Hover == ButtonEdit)
	var editCells []cells
	if !pane.Skeleton && lipgloss.Width(idLine)+1+lipgloss.Width(edit) <= width {
		idLine, editCells = placeButtons(idLine, width, edit)
	}
	upper = append(upper, idLine)
	upper = append(upper, textutil.TruncateString(displaytext.OneLine(emptyFallback(detail.Summary.Title, "(untitled)")), width))
	upper = append(upper, contentHeaderRule(width))
	upper = append(upper, "Description")

	if pane.Skeleton {
		// Skeleton mode: render prose-style placeholder lines instead of the
		// description body, bypassing markdown rendering so ANSI sequences
		// are not corrupted.  A short heading bar followed by paragraph blocks
		// of varying-width ░ lines with blank-line gaps reads as a document
		// body rather than a list of board rows.
		n := max(1, availableHeight-len(upper))
		upper = append(upper, renderProseContentSkeleton(width, n, pane.SkeletonPhase)...)
		return contentLines{lines: upper, comment: noComment(len(upper)), heading: -1}
	}

	upper = append(upper, renderMarkdownMultiline(detail.Description, "(no description)", width)...)

	if strings.TrimSpace(detail.Notes) != "" {
		upper = append(upper, "")
		upper = append(upper, "Notes")
		upper = append(upper, renderMarkdownMultiline(detail.Notes, "(no notes)", width)...)
	}

	heading := "Comments (0)"
	if len(detail.Comments) > 0 {
		heading = fmt.Sprintf("Comments (%d · newest first)", len(detail.Comments))
	}
	heading, addCells := placeButtons(heading, width, paneButton("add", keys.AddComment, pane.Hover == ButtonAddComment))

	selected := -1
	if pane.CommentsFocused {
		selected = pane.CommentIndex
	}
	comments, owner, commentButtons := renderComments(detail.Comments, width, selected,
		paneButton("edit", keys.EditComment, pane.Hover == ButtonEditComment),
		paneButton("delete", keys.DeleteComment, pane.Hover == ButtonDeleteComment),
	)

	// The comments stand at the bottom of the pane: a body shorter than the
	// pane is followed by blank lines.
	spacer := max(0, availableHeight-len(upper)-2-len(comments))
	out := contentLines{heading: len(upper) + spacer + 1, commentButtons: commentButtons}
	if len(editCells) > 0 {
		out.edit = editCells[0]
	}
	if len(addCells) > 0 {
		out.add = addCells[0]
	}
	out.lines = append(out.lines, upper...)
	out.lines = append(out.lines, make([]string, spacer+1)...)
	out.lines = append(out.lines, heading)
	out.comment = noComment(len(out.lines))
	out.lines = append(out.lines, comments...)
	out.comment = append(out.comment, owner...)
	return out
}

// noComment marks count lines as lines of no comment.
func noComment(count int) []int {
	owner := make([]int, count)
	for idx := range owner {
		owner[idx] = -1
	}
	return owner
}

// CommentLineRange is the first and the last line the Content pane draws the
// comment at index on, counted before the scroll window.
func CommentLineRange(state State, index int) (first, last int) {
	if state.Width <= 0 {
		state.Width = defaultDetailWidth
	}
	if state.Height <= 0 {
		state.Height = defaultDetailHeight
	}
	pane := layoutPanes(state.Width, state.Height).content
	content := renderContentPaneLines(state.contentPane(pane.width, pane.height), pane.width-2, max(1, pane.height-2))
	first = slices.Index(content.comment, index)
	last = first
	for last+1 < len(content.comment) && content.comment[last+1] == index {
		last++
	}
	return first, last
}

func (k ButtonKeys) withDefaults() ButtonKeys {
	if strings.TrimSpace(k.EditIssue) == "" {
		k.EditIssue = "e"
	}
	if strings.TrimSpace(k.AddComment) == "" {
		k.AddComment = "a"
	}
	if strings.TrimSpace(k.EditComment) == "" {
		k.EditComment = "enter"
	}
	if strings.TrimSpace(k.DeleteComment) == "" {
		k.DeleteComment = "d"
	}
	return k
}

func sliceWithOffset(lines []string, offset, height, width int) ([]string, int) {
	if height <= 0 {
		return nil, 0
	}
	if len(lines) == 0 {
		lines = []string{""}
	}

	maxOffset := max(0, len(lines)-height)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	start := offset
	end := min(len(lines), start+height)
	window := append([]string(nil), lines[start:end]...)

	if offset > 0 && len(window) > 0 {
		window[0] = textutil.TruncateString(scroll.Earlier(offset), width)
	}
	if end < len(lines) && len(window) > 0 {
		window[len(window)-1] = textutil.TruncateString(scroll.More(len(lines)-end), width)
	}

	for len(window) < height {
		window = append(window, "")
	}

	return window, offset
}

func renderCompact(detail domain.IssueDetail, width int) string {
	out := make([]string, 0, 28)
	out = append(out,
		textutil.TruncateString(displaytext.OneLine(emptyFallback(detail.Summary.Title, "(untitled)")), width),
		textutil.TruncateString(displaytext.OneLine(fmt.Sprintf("%s · %s", detail.Summary.ID, emptyFallback(detail.Summary.Status, "(unknown)"))), width),
	)
	metadata := renderMetadataRail(detail, width, false)
	if len(metadata) > 0 {
		out = append(out, "")
		out = append(out, "Metadata")
		out = append(out, metadata...)
	}

	out = append(out, "")
	out = append(out, "Description")
	out = append(out, renderMarkdownPreviewLines(detail.Description, "(no description)", width, 8)...)

	out = append(out, "")
	out = append(out, fmt.Sprintf("Comments: %d", len(detail.Comments)))
	out = append(out, summarizeReferences(detail, width)...)

	return strings.Join(out, "\n")
}

func renderMarkdownPreviewLines(text, fallback string, width, maxLines int) []string {
	lines := renderMarkdownMultiline(text, fallback, width)
	if len(lines) <= maxLines {
		return lines
	}

	trimmed := append([]string(nil), lines[:maxLines-1]...)
	trimmed = append(trimmed, textutil.TruncateString("…", width))
	return trimmed
}

func summarizeReferences(detail domain.IssueDetail, width int) []string {
	line := fmt.Sprintf("Blocked by: %d  Blocks: %d  Related: %d  Children: %d", len(detail.BlockedBy), len(detail.Blocks), len(detail.Related), len(detail.Children))
	return []string{textutil.TruncateString(line, width)}
}

func renderMarkdownMultiline(text, fallback string, width int) []string {
	r := readOnlyMarkdown
	r.EmptyFallback = fallback
	rendered := r.RenderReadOnly(text, width)

	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) == 0 {
		return []string{fallback}
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, textutil.TruncateString(strings.TrimRight(line, " \t"), width))
	}
	if len(out) == 0 {
		return []string{fallback}
	}

	return out
}

func formatPriority(priority int) string {
	if priority < 0 {
		return "(unknown)"
	}
	return fmt.Sprintf("P%d", priority)
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
