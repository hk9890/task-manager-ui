package search

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
	"github.com/hk9890/beads-workbench/internal/ui/shared/issuerow"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const (
	defaultSearchWidth        = 100
	defaultSearchHeight       = 24
	searchColumnGap           = 2
	searchQueryHeight         = 3
	searchWideMinWidth        = 110
	searchRailMinWidthWide    = 40
	searchRailMaxWidthWide    = 120
	searchRailPercentWide     = 30
	searchMetadataWidth       = 34
	searchContentMinWidthWide = 20
	searchRailMinWidthNarrow  = 34
	searchRightMinWidthNarrow = 26
)

// FocusPane identifies which search sub-pane is active.
type FocusPane int

const (
	FocusQuery FocusPane = iota
	FocusResults
	FocusContent
	FocusMetadata

	// Backward-compatible alias.
	FocusPreview = FocusContent
)

// State is the UI renderer input for search mode.
type State struct {
	Loading   bool
	Reloading bool
	Error     string

	Query        string
	AppliedQuery string
	Focus        FocusPane
	Typing       bool

	Results        []domain.IssueSummary
	Metadata       domain.SearchResultMetadata
	SelectedID     string
	SelectedDetail domain.IssueDetail
	DetailLoading  bool

	MetadataSelectedField uidetails.MetadataFieldKey
	QuickActions          uidetails.QuickActionLabels

	Width         int
	Height        int
	SkeletonPhase int // color-cycle index for skeleton row pulse; see loading.SkeletonPhase
}

// Render renders the standalone search view.
func Render(state State) string {
	width := state.Width
	if width <= 0 {
		width = defaultSearchWidth
	}
	height := state.Height
	if height <= 0 {
		height = defaultSearchHeight
	}

	selectedDetail := selectedDetailForRender(state)

	if width >= searchWideMinWidth {
		return renderWideLayout(state, selectedDetail, width, height)
	}

	return renderNarrowLayout(state, selectedDetail, width, height)
}

func renderWideLayout(state State, selectedDetail domain.IssueDetail, width, height int) string {
	railWidth, contentWidth, metadataWidth := splitWideWidths(width)
	queryHeight := searchQueryHeight
	resultsHeight := max(6, height-queryHeight)

	queryContent := renderQueryContent(state, railWidth-2)
	queryBox := styles.FormSection(styles.FormSectionConfig{
		Width:              railWidth,
		Height:             queryHeight,
		TopLeft:            "Search",
		TopRight:           queryStatusBadge(state),
		Content:            queryContent,
		Focused:            state.Focus == FocusQuery,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	resultsBox := styles.FormSection(styles.FormSectionConfig{
		Width:              railWidth,
		Height:             resultsHeight,
		TopLeft:            "Results",
		TopRight:           resultCountTitle(state),
		Content:            renderResultsContent(state, railWidth-2),
		Focused:            state.Focus == FocusResults,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	left := lipgloss.JoinVertical(lipgloss.Left, queryBox, resultsBox)
	detailSkeleton := isDetailLoadingSkeleton(state)
	contentBox := uidetails.RenderContentPane(selectedDetail, contentWidth, height, state.Focus == FocusContent, 0, detailSkeleton, state.SkeletonPhase)
	metadataBox := uidetails.RenderMetadataPane(selectedDetail, metadataWidth, height, state.Focus == FocusMetadata, 0, state.MetadataSelectedField, state.QuickActions)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		left,
		strings.Repeat(" ", searchColumnGap),
		contentBox,
		strings.Repeat(" ", searchColumnGap),
		metadataBox,
	)
}

func renderNarrowLayout(state State, selectedDetail domain.IssueDetail, width, height int) string {
	leftWidth, rightWidth := splitNarrowWidths(width)
	queryHeight := searchQueryHeight
	resultsHeight := max(6, height-queryHeight)
	contentHeight, metadataHeight := splitNarrowRightHeights(height)

	queryContent := renderQueryContent(state, leftWidth-2)
	queryBox := styles.FormSection(styles.FormSectionConfig{
		Width:              leftWidth,
		Height:             queryHeight,
		TopLeft:            "Search",
		TopRight:           queryStatusBadge(state),
		Content:            queryContent,
		Focused:            state.Focus == FocusQuery,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	resultsBox := styles.FormSection(styles.FormSectionConfig{
		Width:              leftWidth,
		Height:             resultsHeight,
		TopLeft:            "Results",
		TopRight:           resultCountTitle(state),
		Content:            renderResultsContent(state, leftWidth-2),
		Focused:            state.Focus == FocusResults,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	left := lipgloss.JoinVertical(lipgloss.Left, queryBox, resultsBox)
	detailSkeleton := isDetailLoadingSkeleton(state)
	contentBox := uidetails.RenderContentPane(selectedDetail, rightWidth, contentHeight, state.Focus == FocusContent, 0, detailSkeleton, state.SkeletonPhase)
	metadataBox := uidetails.RenderMetadataPane(selectedDetail, rightWidth, metadataHeight, state.Focus == FocusMetadata, 0, state.MetadataSelectedField, state.QuickActions)
	right := lipgloss.JoinVertical(lipgloss.Left, contentBox, metadataBox)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", searchColumnGap), right)
}

func selectedDetailForRender(state State) domain.IssueDetail {
	summary, ok := selectedSummary(state.Results, state.SelectedID)
	if !ok {
		return domain.IssueDetail{
			Summary: domain.IssueSummary{
				Title:    "No selected result.",
				ID:       "(none)",
				Status:   "(none)",
				Type:     "",
				Priority: -1,
			},
			Description: "Select a result in the search rail to preview issue content.",
		}
	}

	if state.DetailLoading || strings.TrimSpace(state.SelectedDetail.Summary.ID) != strings.TrimSpace(state.SelectedID) {
		return detailLoadingStub(summary)
	}

	if strings.TrimSpace(state.SelectedDetail.Summary.ID) == "" {
		return detailLoadingStub(summary)
	}

	return state.SelectedDetail
}

func detailLoadingStub(summary domain.IssueSummary) domain.IssueDetail {
	return domain.IssueDetail{
		Summary:     summary,
		Description: "",
	}
}

// isDetailLoadingSkeleton reports whether the search detail preview pane should
// render skeleton rows.  True when the preview detail is a loading stub (the
// repository response has not yet arrived for the selected result).
func isDetailLoadingSkeleton(state State) bool {
	_, ok := selectedSummary(state.Results, state.SelectedID)
	if !ok {
		return false
	}
	return state.DetailLoading || strings.TrimSpace(state.SelectedDetail.Summary.ID) != strings.TrimSpace(state.SelectedID)
}

func selectedSummary(results []domain.IssueSummary, selectedID string) (domain.IssueSummary, bool) {
	selectedID = strings.TrimSpace(selectedID)
	if selectedID == "" {
		return domain.IssueSummary{}, false
	}
	for _, issue := range results {
		if strings.TrimSpace(issue.ID) == selectedID {
			return issue, true
		}
	}
	return domain.IssueSummary{}, false
}

func renderQueryContent(state State, width int) []string {
	return []string{
		styles.TruncateString(renderQueryInputLine(state.Query, state.Focus == FocusQuery), width),
	}
}

func renderQueryInputLine(query string, focused bool) string {
	value := strings.TrimSpace(query)
	if value == "" {
		if focused {
			return "│"
		}
		return "Type query, then press Enter."
	}
	if focused {
		return value + "│"
	}
	return value
}

func queryStatusBadge(state State) string {
	if isInlineReload(state) {
		return "reload"
	}
	if strings.TrimSpace(state.Error) != "" {
		return "failed"
	}
	if hasDraftChanges(state) {
		return "draft"
	}
	if hasSearchContext(state) {
		return "shown"
	}
	return "idle"
}

func resultCountTitle(state State) string {
	count := displayedResultCount(state)
	badge := strings.TrimSpace(resultCompletenessBadge(state))
	var parts []string
	if badge != "" {
		parts = append(parts, badge)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d", count)
	}
	return fmt.Sprintf("%d %s", count, strings.Join(parts, " · "))
}

func renderResultsContent(state State, width int) []string {
	banner := renderResultsBanner(state, width)
	body := renderResultsBody(state, width)
	if len(banner) == 0 {
		return body
	}
	if len(body) == 0 {
		return banner
	}
	return append(append(banner, ""), body...)
}

func renderResultsBanner(state State, width int) []string {
	if strings.TrimSpace(state.Error) != "" && len(state.Results) > 0 {
		return []string{styles.TruncateString(state.Error, width)}
	}
	// Show a stale-results hint when the typed draft differs from the last
	// applied query and a search is not already in flight (in-flight case has
	// its own "reload" affordance in the query-box badge).
	if hasDraftChanges(state) && !isInlineReload(state) && len(state.Results) > 0 {
		draft := strings.TrimSpace(state.Query)
		if draft == "" {
			return []string{styles.TruncateString("Results below are from a previous query. Press Enter to clear.", width)}
		}
		return []string{styles.TruncateString(fmt.Sprintf("Results below are stale. Press Enter to search for %q.", draft), width)}
	}
	return nil
}

func renderResultsBody(state State, width int) []string {
	if strings.TrimSpace(state.Error) != "" && len(state.Results) == 0 {
		lines := []string{"Search failed."}
		lines = append(lines, styles.WrapLines(state.Error, width)...)
		lines = append(lines, "")
		lines = append(lines, styles.WrapLines("Edit the query, then press Enter to retry.", width)...)
		return lines
	}

	// Cold-start: loading with no prior results — render skeleton placeholder rows.
	if state.Loading && len(state.Results) == 0 {
		return renderSkeletonRows(width, 6)
	}

	if len(state.Results) == 0 {
		return renderEmptyResultsBody(state, width)
	}

	return renderResultRows(state, width)
}

func renderEmptyResultsBody(state State, width int) []string {
	if strings.TrimSpace(state.AppliedQuery) == "" {
		lines := []string{"No search has run yet.", ""}
		lines = append(lines, styles.WrapLines("Type query text, then press Enter to search.", width)...)
		return lines
	}

	lines := styles.WrapLines(fmt.Sprintf("No matches for %q.", strings.TrimSpace(state.AppliedQuery)), width)
	lines = append(lines, "")
	lines = append(lines, styles.WrapLines("Try broader terms or clear the query, then press Enter.", width)...)
	return lines
}

func renderResultRows(state State, width int) []string {
	// Dim rows when a refresh is in flight (stale data visible, new data pending).
	dim := state.Loading && len(state.Results) > 0
	lines := make([]string, 0, len(state.Results))
	for _, issue := range state.Results {
		lines = append(lines, issuerow.RenderCompact(issuerow.RenderConfig{
			Issue:    issue,
			Selected: issue.ID == state.SelectedID,
			Width:    width,
			Styled:   true,
			Dim:      dim,
			Phase:    state.SkeletonPhase,
		}))
	}

	return lines
}

// renderSkeletonRows returns n skeleton placeholder rows for the cold-start
// loading state. Each row uses RenderCompactSkeleton shaped like a real issue row.
func renderSkeletonRows(width, n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = issuerow.RenderCompactSkeleton(issuerow.SkeletonOpts{
			Width:  width,
			Seed:   i,
			Styled: true,
		})
	}
	return lines
}

func displayedResultCount(state State) int {
	if state.Metadata.ReturnedCount > 0 {
		return state.Metadata.ReturnedCount
	}
	return len(state.Results)
}

func resultCompletenessBadge(state State) string {
	switch state.Metadata.Completeness {
	case domain.SearchResultCompletenessExact:
		return "exact"
	case domain.SearchResultCompletenessMaybeMore:
		return "capped"
	case domain.SearchResultCompletenessPartial:
		return "partial"
	default:
		return ""
	}
}

func isInlineReload(state State) bool {
	return state.Reloading || (state.Loading && len(state.Results) > 0)
}

func hasDraftChanges(state State) bool {
	return strings.TrimSpace(state.Query) != strings.TrimSpace(state.AppliedQuery)
}

func hasSearchContext(state State) bool {
	return strings.TrimSpace(state.AppliedQuery) != "" || len(state.Results) > 0
}

func splitWideWidths(total int) (rail, content, metadata int) {
	available := total - (searchColumnGap * 2)
	if available < 3 {
		available = 3
	}

	metadata = searchMetadataWidth
	rail = clamp((available*searchRailPercentWide)/100, searchRailMinWidthWide, searchRailMaxWidthWide)
	content = available - rail - metadata

	if content < searchContentMinWidthWide {
		need := searchContentMinWidthWide - content
		reduceRail := min(need, max(0, rail-24))
		rail -= reduceRail
		need -= reduceRail

		reduceMetadata := min(need, max(0, metadata-20))
		metadata -= reduceMetadata
		need -= reduceMetadata

		if need > 0 {
			rail = max(12, rail-need/2)
			metadata = max(12, metadata-(need-need/2))
		}
	}

	if rail < 1 {
		rail = 1
	}
	if metadata < 1 {
		metadata = 1
	}
	content = available - rail - metadata
	if content < 1 {
		content = 1
	}

	return rail, content, metadata
}

func splitNarrowWidths(total int) (left, right int) {
	available := total - searchColumnGap
	if available < searchRailMinWidthNarrow+searchRightMinWidthNarrow {
		available = searchRailMinWidthNarrow + searchRightMinWidthNarrow
	}
	left = (available * 45) / 100
	right = available - left
	if left < searchRailMinWidthNarrow {
		left = searchRailMinWidthNarrow
		right = available - left
	}
	if right < searchRightMinWidthNarrow {
		right = searchRightMinWidthNarrow
		left = available - right
	}
	return left, right
}

func splitNarrowRightHeights(total int) (content, metadata int) {
	if total <= 2 {
		return 1, 1
	}
	content = (total * 3) / 5
	metadata = total - content
	if content < 6 {
		content = 6
		metadata = total - content
	}
	if metadata < 6 {
		metadata = 6
		content = total - metadata
	}
	if content < 1 {
		content = 1
	}
	if metadata < 1 {
		metadata = 1
	}
	return content, metadata
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
