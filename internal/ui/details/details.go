package details

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
	"github.com/hk9890/beads-workbench/internal/ui/shared/issuerow"
	"github.com/hk9890/beads-workbench/internal/ui/shared/markdown"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const (
	defaultDetailWidth  = 80
	defaultDetailHeight = 24
	timeLayout          = "2006-01-02 15:04"

	// Kept for compatibility with existing tests/callers.
	InspectorTwoColumnMinWidth   = 110
	InspectorThreeColumnMinWidth = 140

	detailColumnGap   = 2
	metadataRailWidth = 34
	leftRailMinWidth  = 24
	leftRailMaxWidth  = 44
)

var readOnlyMarkdown = markdown.NewRenderer()

// State is the issue detail renderer input.
type State struct {
	SelectionID              string
	TargetID                 string
	Detail                   domain.IssueDetail
	BrowserItems             []domain.IssueReference
	BrowserSelectedIssueID   string
	Loading                  bool
	Error                    string
	Width                    int
	Height                   int
	Compact                  bool
	FocusPane                FocusPane
	MetadataSelectedField    MetadataFieldKey
	ContentScrollOffset      int
	DependenciesScrollOffset int
	MetadataScrollOffset     int
}

// FocusPane identifies which detail pane is visually focused.
type FocusPane int

const (
	FocusPaneContent FocusPane = iota
	FocusPaneDependencies
	FocusPaneMetadata

	// Backward-compatible alias.
	FocusPaneBrowser = FocusPaneDependencies
)

// ScrollOffsets describes max scroll bounds for each independent pane.
type ScrollOffsets struct {
	Dependencies int
	Content      int
	Metadata     int
}

// relationshipGroup is shared relation rendering input used by multiple rails.
type relationshipGroup struct {
	Label string
	Refs  []domain.IssueReference
}

// Render renders standalone issue detail metadata and content.
func Render(state State) string {
	selected := strings.TrimSpace(state.SelectionID)
	if selected == "" {
		return "No selected issue.\nSelect an issue in board/search first."
	}

	if state.Loading {
		target := selected
		if strings.TrimSpace(state.TargetID) != "" {
			target = state.TargetID
		}
		return loading.View(loading.State{Scope: loading.ScopeDetail, Target: target})
	}

	if strings.TrimSpace(state.Error) != "" {
		return fmt.Sprintf("Failed to load details for %s.\nError: %s", selected, state.Error)
	}

	detail := state.Detail
	if strings.TrimSpace(detail.Summary.ID) == "" {
		return fmt.Sprintf("No detail loaded for %s yet.", selected)
	}

	width := state.Width
	if width <= 0 {
		width = defaultDetailWidth
	}
	height := state.Height
	if height <= 0 {
		height = defaultDetailHeight
	}

	if state.Compact {
		return renderCompact(detail, width)
	}

	return renderThreePane(detail, state, width, height)
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

	leftWidth, contentWidth, metadataWidth := splitThreePaneWidths(width)
	innerHeight := max(1, height-2)

	deps := renderDependenciesPaneLines(state.Detail, state.BrowserItems, "", leftWidth-2)
	content := renderContentPaneLines(state.Detail, contentWidth-2, innerHeight)
	metadata := renderMetadataPaneLines(state.Detail, metadataWidth-2, MetadataFieldNone, false)

	return ScrollOffsets{
		Dependencies: max(0, len(deps)-innerHeight),
		Content:      max(0, len(content)-innerHeight),
		Metadata:     max(0, len(metadata)-innerHeight),
	}
}

func renderThreePane(detail domain.IssueDetail, state State, width, height int) string {
	leftWidth, contentWidth, metadataWidth := splitThreePaneWidths(width)
	innerHeight := max(1, height-2)

	deps := renderDependenciesPaneLines(detail, state.BrowserItems, state.BrowserSelectedIssueID, leftWidth-2)
	depView, _ := sliceWithOffset(deps, state.DependenciesScrollOffset, innerHeight, leftWidth-2)
	leftBox := styles.FormSection(styles.FormSectionConfig{
		Width:              leftWidth,
		Height:             height,
		TopLeft:            "Dependencies",
		TopRight:           fmt.Sprintf("%d", countDependencyReferences(detail)),
		Content:            depView,
		Focused:            state.FocusPane == FocusPaneDependencies,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	content := renderContentPaneLines(detail, contentWidth-2, innerHeight)
	contentView, _ := sliceWithOffset(content, state.ContentScrollOffset, innerHeight, contentWidth-2)
	contentBox := styles.FormSection(styles.FormSectionConfig{
		Width:              contentWidth,
		Height:             height,
		TopLeft:            "Content",
		TopRight:           fmt.Sprintf("%d comments", len(detail.Comments)),
		Content:            contentView,
		Focused:            state.FocusPane == FocusPaneContent,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	selectedField := MetadataFieldNone
	if state.FocusPane == FocusPaneMetadata {
		selectedField = state.MetadataSelectedField
	}
	metadata := renderMetadataPaneLines(detail, metadataWidth-2, selectedField, state.FocusPane == FocusPaneMetadata)
	metaView, _ := sliceWithOffset(metadata, state.MetadataScrollOffset, innerHeight, metadataWidth-2)
	metaBox := styles.FormSection(styles.FormSectionConfig{
		Width:              metadataWidth,
		Height:             height,
		TopLeft:            "Metadata",
		Content:            metaView,
		Focused:            state.FocusPane == FocusPaneMetadata,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
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
			padToWidth(leftLine, leftWidth)+strings.Repeat(" ", detailColumnGap)+
				padToWidth(contentLine, contentWidth)+strings.Repeat(" ", detailColumnGap)+
				padToWidth(metaLine, metadataWidth),
		)
	}

	return strings.Join(out, "\n")
}

func splitThreePaneWidths(total int) (left, content, metadata int) {
	available := total - (detailColumnGap * 2)
	if available < 3 {
		available = 3
	}

	metadata = metadataRailWidth
	left = clamp(available/4, leftRailMinWidth, leftRailMaxWidth)
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

		content = available - left - metadata
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

func renderDependenciesPaneLines(detail domain.IssueDetail, browserItems []domain.IssueReference, selectedIssueID string, width int) []string {
	return renderRelationshipGroups(dependencyGroups(detail, browserItems), selectedIssueID, width)
}

func dependencyGroups(detail domain.IssueDetail, browserItems []domain.IssueReference) []relationshipGroup {
	groups := []relationshipGroup{
		{Label: "Blocked by", Refs: detail.BlockedBy},
		{Label: "Blocks", Refs: detail.Blocks},
		{Label: "Related", Refs: detail.Related},
	}
	if len(browserItems) > 0 && strings.TrimSpace(detail.ParentGroupBrowser.Parent.ID) != "" {
		groups = append(groups, relationshipGroup{Label: "Structure", Refs: browserItems})
	}
	return groups
}

func renderRelationshipGroups(groups []relationshipGroup, selectedIssueID string, width int) []string {
	out := make([]string, 0, 32)
	selectedIssueID = strings.TrimSpace(selectedIssueID)
	selectedMatched := false
	for _, group := range groups {
		ordered := orderedReferences(group.Refs)
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, styles.TruncateString(fmt.Sprintf("%s (%d)", group.Label, len(ordered)), width))
		if len(ordered) == 0 {
			out = append(out, styles.TruncateString("(none)", width))
			continue
		}
		for _, ref := range ordered {
			selected := !selectedMatched && selectedIssueID != "" && ref.ID == selectedIssueID
			if selected {
				selectedMatched = true
			}
			out = append(out, renderReferenceRow(ref, width, selected))
		}
	}
	if len(out) == 0 {
		return []string{"(none)"}
	}
	return out
}

func countDependencyReferences(detail domain.IssueDetail) int {
	return len(detail.BlockedBy) + len(detail.Blocks) + len(detail.Related)
}

func renderContentPaneLines(detail domain.IssueDetail, width, availableHeight int) []string {
	upper := make([]string, 0, 48)
	upper = append(upper, styles.TruncateString(emptyFallback(detail.Summary.Title, "(untitled)"), width))
	summary := fmt.Sprintf("%s · %s · %s", emptyFallback(detail.Summary.ID, "(unknown)"), emptyFallback(detail.Summary.Status, "(unknown)"), formatPriority(detail.Summary.Priority))
	upper = append(upper, styles.TruncateString(summary, width))

	upper = append(upper, "")
	upper = append(upper, "Description")
	upper = append(upper, renderMarkdownMultiline(detail.Description, "(no description)", width)...)

	upper = append(upper, "")
	upper = append(upper, "Notes")
	upper = append(upper, renderMarkdownMultiline(detail.Notes, "(no notes)", width)...)

	commentsSection := make([]string, 0, 16)
	commentsSection = append(commentsSection, "")
	commentsSection = append(commentsSection, fmt.Sprintf("Comments (%d)", len(detail.Comments)))
	commentsSection = append(commentsSection, renderComments(detail.Comments, width)...)

	spacer := 0
	totalLines := len(upper) + len(commentsSection)
	if availableHeight > totalLines {
		spacer = availableHeight - totalLines
	}

	out := make([]string, 0, totalLines+spacer)
	out = append(out, upper...)
	for i := 0; i < spacer; i++ {
		out = append(out, "")
	}
	out = append(out, commentsSection...)

	return out
}

func renderMetadataPaneLines(detail domain.IssueDetail, width int, selectedField MetadataFieldKey, focused bool) []string {
	out := make([]string, 0, 48)
	if focused {
		out = append(out, "› Editable: Status, Priority")
		out = append(out, "")
	}
	out = append(out, renderMetadataRail(detail, width, selectedField)...)
	out = append(out, "")
	out = append(out,
		"Quick actions",
		"e Edit issue",
		"u Update issue",
		"c Add comment",
		"x Close issue",
		"r Reload detail",
	)
	return out
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
		window[0] = styles.TruncateString(fmt.Sprintf("… (%d earlier)", offset), width)
	}
	if end < len(lines) && len(window) > 0 {
		window[len(window)-1] = styles.TruncateString(fmt.Sprintf("… (%d more)", len(lines)-end), width)
	}

	for len(window) < height {
		window = append(window, "")
	}

	return window, offset
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func padToWidth(value string, width int) string {
	renderedWidth := lipgloss.Width(value)
	if renderedWidth >= width {
		return styles.TruncateString(value, width)
	}
	return value + strings.Repeat(" ", width-renderedWidth)
}

func renderCompact(detail domain.IssueDetail, width int) string {
	out := make([]string, 0, 28)
	out = append(out,
		styles.TruncateString(emptyFallback(detail.Summary.Title, "(untitled)"), width),
		styles.TruncateString(fmt.Sprintf("%s · %s", detail.Summary.ID, emptyFallback(detail.Summary.Status, "(unknown)")), width),
	)
	metadata := renderMetadataRail(detail, width, MetadataFieldNone)
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
	trimmed = append(trimmed, styles.TruncateString("…", width))
	return trimmed
}

func summarizeReferences(detail domain.IssueDetail, width int) []string {
	line := fmt.Sprintf("Blocked by: %d  Blocks: %d  Related: %d", len(detail.BlockedBy), len(detail.Blocks), len(detail.Related))
	return []string{styles.TruncateString(line, width)}
}

func renderMultiline(text, fallback string, width int) []string {
	trimmed := strings.Trim(text, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return []string{fallback}
	}
	lines := strings.Split(trimmed, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		cleaned := strings.TrimRight(line, " \t")
		out = append(out, styles.TruncateString(cleaned, width))
	}
	if len(out) == 0 {
		return []string{fallback}
	}
	return out
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
		out = append(out, styles.TruncateString(strings.TrimRight(line, " \t"), width))
	}
	if len(out) == 0 {
		return []string{fallback}
	}

	return out
}

func renderComments(comments []domain.IssueComment, width int) []string {
	if len(comments) == 0 {
		return []string{"(no comments)"}
	}

	ordered := append([]domain.IssueComment(nil), comments...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})

	out := make([]string, 0, len(ordered)*3)
	for i, comment := range ordered {
		author := emptyFallback(comment.Author, "unknown")
		timestamp := formatTime(comment.CreatedAt)
		out = append(out, styles.TruncateString(fmt.Sprintf("%s · %s", author, timestamp), width))
		for _, line := range renderMarkdownMultiline(comment.Body, "(empty comment)", width-2) {
			if line == "" {
				out = append(out, "")
				continue
			}
			out = append(out, styles.TruncateString("  "+line, width))
		}
		if i < len(ordered)-1 {
			out = append(out, "")
		}
	}

	return out
}

func renderReferenceRow(ref domain.IssueReference, width int, selected bool) string {
	return issuerow.RenderReferenceCompact(issuerow.ReferenceRenderConfig{
		Issue:    ref,
		Selected: selected,
		Width:    width,
		Styled:   true,
	})
}

func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return "unknown time"
	}
	return ts.Format(timeLayout)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
