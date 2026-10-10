package search

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/detail"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
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

	Results  []domain.IssueSummary
	Metadata domain.SearchResultMetadata
	// ScrollOffset is the index of the first result the pane draws. Render and
	// HitTest both read it, so the row under a click is the row drawn there.
	ScrollOffset   int
	SelectedID     string
	SelectedDetail domain.IssueDetail
	DetailLoading  bool

	// IncludeClosed reports the active search scope. It is rendered on the
	// results header because it is a persistent mode rather than transient
	// status: without it a thin result set reads as "nothing matched" when it
	// actually means "nothing open matched".
	IncludeClosed bool

	MetadataSelectedField detail.MetadataFieldKey
	QuickActions          detail.QuickActionLabels

	Width         int
	Height        int
	SkeletonPhase int // color-cycle index for skeleton row pulse; see loading.SkeletonPhase

	// Hover is the cell under the pointer, as HitTest reported it; nil when the
	// pointer is elsewhere. Its result row draws the hover band.
	Hover *Hit
}

// Render renders the standalone search view.
func Render(state State) string {
	width := state.Width
	if width <= 0 {
		width = defaultSearchWidth
	}
	height := frameHeight(state)

	selectedDetail := selectedDetailForRender(state)

	if width >= searchWideMinWidth {
		return renderWideLayout(state, selectedDetail, width, height)
	}

	return renderNarrowLayout(state, selectedDetail, width, height)
}

func renderWideLayout(state State, selectedDetail domain.IssueDetail, width, height int) string {
	railWidth, contentWidth, metadataWidth := splitWideWidths(width)
	queryHeight := searchQueryHeight
	resultsHeight := resultsPaneHeight(height)

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
		TopLeft:            resultsTitle,
		TopRight:           resultCountTitle(state, railWidth, height),
		Content:            renderResultsContent(state, railWidth-2, height),
		Focused:            state.Focus == FocusResults,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	left := lipgloss.JoinVertical(lipgloss.Left, queryBox, resultsBox)
	detailSkeleton := isDetailLoadingSkeleton(state)
	contentBox := detail.RenderContentPane(detail.ContentPaneState{
		Detail:        selectedDetail,
		Width:         contentWidth,
		Height:        height,
		Focused:       state.Focus == FocusContent,
		Skeleton:      detailSkeleton,
		SkeletonPhase: state.SkeletonPhase,
	})
	metadataBox := detail.RenderMetadataPane(detail.MetadataPaneState{
		Detail:        selectedDetail,
		Width:         metadataWidth,
		Height:        height,
		Focused:       state.Focus == FocusMetadata,
		SelectedField: state.MetadataSelectedField,
		QuickActions:  state.QuickActions,
		Skeleton:      detailSkeleton,
	})

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
	resultsHeight := resultsPaneHeight(height)
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
		TopLeft:            resultsTitle,
		TopRight:           resultCountTitle(state, leftWidth, height),
		Content:            renderResultsContent(state, leftWidth-2, height),
		Focused:            state.Focus == FocusResults,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	left := lipgloss.JoinVertical(lipgloss.Left, queryBox, resultsBox)
	detailSkeleton := isDetailLoadingSkeleton(state)
	contentBox := detail.RenderContentPane(detail.ContentPaneState{
		Detail:        selectedDetail,
		Width:         rightWidth,
		Height:        contentHeight,
		Focused:       state.Focus == FocusContent,
		Skeleton:      detailSkeleton,
		SkeletonPhase: state.SkeletonPhase,
	})
	metadataBox := detail.RenderMetadataPane(detail.MetadataPaneState{
		Detail:        selectedDetail,
		Width:         rightWidth,
		Height:        metadataHeight,
		Focused:       state.Focus == FocusMetadata,
		SelectedField: state.MetadataSelectedField,
		QuickActions:  state.QuickActions,
		Skeleton:      detailSkeleton,
	})
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
		textutil.TruncateString(renderQueryInputLine(state.Query, state.Focus == FocusQuery), width),
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

const resultsTitle = "Results"

// resultCountTitle is the Results header: the loaded count, whether the
// backend had more, and the scope. It leads with `N of` when the pane draws
// only N of the loaded results, and with `N/` when the rail is too narrow for
// that beside the title. The scope stays whole: a rail too narrow for the
// short form too loses the word for whether the backend had more.
func resultCountTitle(state State, paneWidth, height int) string {
	scope := searchScopeLabel(state)
	tail := scope
	if badge := strings.TrimSpace(resultCompletenessBadge(state)); badge != "" {
		tail = badge + " · " + scope
	}

	loaded := displayedResultCount(state)
	drawn := drawnResultCount(state, height)
	if drawn >= len(state.Results) {
		return fmt.Sprintf("%d %s", loaded, tail)
	}
	// What styles.FormSection leaves beside the left title: the corners, the
	// spaces around both titles and one rule between them.
	room := paneWidth - lipgloss.Width(resultsTitle) - 9
	title := fmt.Sprintf("%d of %d %s", drawn, loaded, tail)
	if lipgloss.Width(title) > room {
		title = fmt.Sprintf("%d/%d %s", drawn, loaded, tail)
	}
	if lipgloss.Width(title) > room {
		title = fmt.Sprintf("%d/%d · %s", drawn, loaded, scope)
	}
	return textutil.TruncateString(title, room)
}

// frameHeight is the height Render draws the state at.
func frameHeight(state State) int {
	if state.Height <= 0 {
		return defaultSearchHeight
	}
	return state.Height
}

// resultsPaneHeight is the height of the results box, borders included, in a
// frame of the given height.
func resultsPaneHeight(height int) int {
	return max(6, height-searchQueryHeight)
}

// resultsPaneLines is the lines inside the results box that are on screen in
// a frame of the given height. The box keeps a floor of lines, and the shell
// cuts a frame taller than its workspace from the bottom.
func resultsPaneLines(height int) int {
	return max(0, min(resultsPaneHeight(height)-2, height-searchQueryHeight-1))
}

// bannerLines is what the banner takes from the rows: itself and the blank
// line under it.
const bannerLines = 2

// bannerShown reports whether the pane draws the banner: there is one, and
// the pane has the lines of a whole result under it. In a shorter pane the
// banner gives way to the selected result.
func bannerShown(state State, height int) bool {
	return len(renderResultsBanner(state, 0)) > 0 && resultsPaneLines(height)-bannerLines >= issuerow.Height
}

// resultLines is the lines on screen for the rows in a frame of the given
// height: the inside of the results box, less the banner while that is up.
func resultLines(state State, height int) int {
	lines := resultsPaneLines(height)
	if bannerShown(state, height) {
		lines -= bannerLines
	}
	return lines
}

// RowCapacity returns how many whole results Render(state) has lines for. The
// controller takes its scroll window from it, so the window is the rows drawn,
// with the banner up or not.
func RowCapacity(state State) int {
	return max(1, resultLines(state, frameHeight(state))/issuerow.Height)
}

// firstResult is the scroll offset held inside the result list.
func firstResult(state State) int {
	return textutil.Clamp(state.ScrollOffset, 0, max(0, len(state.Results)-1))
}

// drawnResultCount is the number of results the pane draws whole.
func drawnResultCount(state State, height int) int {
	return min(resultLines(state, height)/issuerow.Height, len(state.Results)-firstResult(state))
}

// searchScopeLabel names the active scope for the results header.
func searchScopeLabel(state State) string {
	if state.IncludeClosed {
		return "all"
	}
	return "open"
}

func renderResultsContent(state State, width, height int) []string {
	body := renderResultsBody(state, width, height)
	if !bannerShown(state, height) {
		return body
	}
	return append(append(renderResultsBanner(state, width), ""), body...)
}

func renderResultsBanner(state State, width int) []string {
	if strings.TrimSpace(state.Error) != "" && len(state.Results) > 0 {
		return []string{textutil.TruncateString(state.Error, width)}
	}
	// Show a stale-results hint when the typed draft differs from the last
	// applied query and a search is not already in flight (in-flight case has
	// its own "reload" affordance in the query-box badge).
	if hasDraftChanges(state) && !isInlineReload(state) && len(state.Results) > 0 {
		draft := strings.TrimSpace(state.Query)
		if draft == "" {
			return []string{textutil.TruncateString("Results below are from a previous query. Press Enter to clear.", width)}
		}
		return []string{textutil.TruncateString(fmt.Sprintf("Results below are stale. Press Enter to search for %q.", draft), width)}
	}
	return nil
}

func renderResultsBody(state State, width, height int) []string {
	if strings.TrimSpace(state.Error) != "" && len(state.Results) == 0 {
		lines := []string{"Search failed."}
		lines = append(lines, textutil.WrapLines(state.Error, width)...)
		lines = append(lines, "")
		lines = append(lines, textutil.WrapLines("Edit the query, then press Enter to retry.", width)...)
		return lines
	}

	// Cold-start: loading with no prior results — render skeleton placeholder rows.
	if state.Loading && len(state.Results) == 0 {
		return renderSkeletonRows(width, 3, state.SkeletonPhase)
	}

	if len(state.Results) == 0 {
		return renderEmptyResultsBody(state, width)
	}

	return renderResultRows(state, width, height)
}

func renderEmptyResultsBody(state State, width int) []string {
	if strings.TrimSpace(state.AppliedQuery) == "" {
		lines := []string{"No search has run yet.", ""}
		lines = append(lines, textutil.WrapLines("Type query text, then press Enter to search.", width)...)
		return lines
	}

	lines := textutil.WrapLines(fmt.Sprintf("No matches for %q.", strings.TrimSpace(state.AppliedQuery)), width)
	lines = append(lines, "")
	lines = append(lines, textutil.WrapLines("Try broader terms or clear the query, then press Enter.", width)...)
	return lines
}

func renderResultRows(state State, width, height int) []string {
	// Dim rows when a refresh is in flight (stale data visible, new data pending).
	dim := state.Loading && len(state.Results) > 0
	first := firstResult(state)
	// A search loads far more results than the pane has lines for. The last
	// row drawn may have a line for its title only.
	fitting := (resultLines(state, height) + issuerow.Height - 1) / issuerow.Height
	end := min(len(state.Results), first+fitting)
	lines := make([]string, 0, issuerow.Height*(end-first))
	hover := -1
	if state.Hover != nil && state.Hover.Pane == FocusResults {
		hover = state.Hover.Row
	}
	for idx := first; idx < end; idx++ {
		issue := state.Results[idx]
		lines = append(lines, issuerow.RenderCompact(issuerow.RenderConfig{
			Issue:    issue,
			Selected: issue.ID == state.SelectedID,
			Hovered:  idx == hover,
			Width:    width,
			Styled:   true,
			Dim:      dim,
			Phase:    state.SkeletonPhase,
		})...)
	}

	return lines
}

// renderSkeletonRows returns n skeleton placeholder rows for the cold-start
// loading state. Each row uses RenderCompactSkeleton shaped like a real issue row.
//
// phase is what makes the rows pulse: it indexes styles.SkeletonShades and
// advances every 4 spinner frames (docs/DESIGN-GUIDE.md, Loading feedback).
// Omitting it pinned every row to shade 0, so a stalled search
// looked exactly like a fast one.
func renderSkeletonRows(width, n, phase int) []string {
	lines := make([]string, 0, issuerow.Height*n)
	for i := 0; i < n; i++ {
		lines = append(lines, issuerow.RenderCompactSkeleton(issuerow.SkeletonOpts{
			Width:  width,
			Seed:   i,
			Phase:  phase,
			Styled: true,
		})...)
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
	rail = textutil.Clamp((available*searchRailPercentWide)/100, searchRailMinWidthWide, searchRailMaxWidthWide)
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
