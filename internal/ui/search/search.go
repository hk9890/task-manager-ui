package search

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
	"github.com/hk9890/beads-workbench/internal/ui/shared/issuerow"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const (
	defaultSearchWidth        = 100
	defaultSearchHeight       = 24
	searchColumnGap           = 2
	searchWideMinWidth        = 110
	searchRailMinWidthWide    = 38
	searchRailMaxWidthWide    = 52
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
	Loading bool
	Error   string

	Query  string
	Focus  FocusPane
	Typing bool

	Results        []domain.IssueSummary
	SelectedID     string
	SelectedDetail domain.IssueDetail
	DetailLoading  bool

	MetadataSelectedField uidetails.MetadataFieldKey

	Width  int
	Height int
}

// Render renders the standalone search view.
func Render(state State) string {
	if state.Loading {
		return loading.View(loading.State{Scope: loading.ScopeSearch})
	}

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
	queryHeight := 3
	resultsHeight := max(6, height-queryHeight)

	queryContent := renderQueryContent(state.Query, state.Focus == FocusQuery)
	queryBox := styles.FormSection(styles.FormSectionConfig{
		Width:              railWidth,
		Height:             queryHeight,
		TopLeft:            "Search",
		TopRight:           queryStatusHint(state),
		Content:            []string{styles.TruncateString(queryContent, railWidth-2)},
		Focused:            state.Focus == FocusQuery,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	resultsBox := styles.FormSection(styles.FormSectionConfig{
		Width:              railWidth,
		Height:             resultsHeight,
		TopLeft:            "Results",
		TopRight:           resultCountTitle(state.Results),
		Content:            renderResultsContent(state, railWidth-2),
		Focused:            state.Focus == FocusResults,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	left := lipgloss.JoinVertical(lipgloss.Left, queryBox, resultsBox)
	contentBox := uidetails.RenderContentPane(selectedDetail, contentWidth, height, state.Focus == FocusContent, 0)
	metadataBox := uidetails.RenderMetadataPane(selectedDetail, metadataWidth, height, state.Focus == FocusMetadata, 0, state.MetadataSelectedField)

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
	queryHeight := 3
	resultsHeight := max(6, height-queryHeight)
	contentHeight, metadataHeight := splitNarrowRightHeights(height)

	queryContent := renderQueryContent(state.Query, state.Focus == FocusQuery)
	queryBox := styles.FormSection(styles.FormSectionConfig{
		Width:              leftWidth,
		Height:             queryHeight,
		TopLeft:            "Search",
		TopRight:           queryStatusHint(state),
		Content:            []string{styles.TruncateString(queryContent, leftWidth-2)},
		Focused:            state.Focus == FocusQuery,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	resultsBox := styles.FormSection(styles.FormSectionConfig{
		Width:              leftWidth,
		Height:             resultsHeight,
		TopLeft:            "Results",
		TopRight:           resultCountTitle(state.Results),
		Content:            renderResultsContent(state, leftWidth-2),
		Focused:            state.Focus == FocusResults,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	left := lipgloss.JoinVertical(lipgloss.Left, queryBox, resultsBox)
	contentBox := uidetails.RenderContentPane(selectedDetail, rightWidth, contentHeight, state.Focus == FocusContent, 0)
	metadataBox := uidetails.RenderMetadataPane(selectedDetail, rightWidth, metadataHeight, state.Focus == FocusMetadata, 0, state.MetadataSelectedField)
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
		Description: loading.View(loading.State{Scope: loading.ScopeDetail, Target: strings.TrimSpace(summary.ID)}),
	}
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

func renderQueryContent(query string, focused bool) string {
	value := strings.TrimSpace(query)
	if value == "" {
		if focused {
			return "│"
		}
		return "Type to search issues…"
	}
	if focused {
		return value + "│"
	}
	return value
}

func queryStatusHint(state State) string {
	if strings.TrimSpace(state.Error) != "" {
		return "error"
	}
	if state.Typing {
		return "typing"
	}
	if strings.TrimSpace(state.Query) == "" {
		return "text"
	}
	return "live"
}

func resultCountTitle(results []domain.IssueSummary) string {
	if len(results) == 0 {
		return "0"
	}
	return fmt.Sprintf("%d", len(results))
}

func renderResultsContent(state State, width int) []string {
	if strings.TrimSpace(state.Error) != "" {
		return []string{
			styles.TruncateString("Search failed", width),
			styles.TruncateString(state.Error, width),
		}
	}

	if state.Typing && len(state.Results) == 0 {
		return []string{
			styles.TruncateString("Searching…", width),
			"",
			styles.TruncateString("Keep typing to refine the query.", width),
		}
	}

	if len(state.Results) == 0 {
		return []string{
			styles.TruncateString("No results found.", width),
			"",
			styles.TruncateString("Try a different text query.", width),
		}
	}

	lines := make([]string, 0, len(state.Results))
	for _, issue := range state.Results {
		lines = append(lines, issuerow.RenderCompact(issuerow.RenderConfig{
			Issue:    issue,
			Selected: issue.ID == state.SelectedID,
			Width:    width,
			Styled:   true,
		}))
	}

	return lines
}

func splitWideWidths(total int) (rail, content, metadata int) {
	available := total - (searchColumnGap * 2)
	if available < 3 {
		available = 3
	}

	metadata = searchMetadataWidth
	rail = clamp((available*35)/100, searchRailMinWidthWide, searchRailMaxWidthWide)
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

		content = available - rail - metadata
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
