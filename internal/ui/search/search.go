package search

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
	"github.com/hk9890/beads-workbench/internal/ui/shared/issuerow"
	"github.com/hk9890/beads-workbench/internal/ui/shared/renderhelpers"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const (
	defaultSearchWidth  = 100
	defaultSearchHeight = 24
	searchColumnGap     = 2
	minLeftPaneWidth    = 34
	minRightPaneWidth   = 26
)

// FocusPane identifies which search sub-pane is active.
type FocusPane int

const (
	FocusQuery FocusPane = iota
	FocusResults
	FocusPreview
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

	leftWidth, rightWidth := splitWidths(width)
	queryHeight := 3
	resultsHeight := renderhelpers.MaxInt(6, height-queryHeight-1)
	previewHeight := height

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

	previewBox := styles.FormSection(styles.FormSectionConfig{
		Width:              rightWidth,
		Height:             previewHeight,
		TopLeft:            "Preview",
		Content:            renderPreviewContent(state, rightWidth-2),
		Focused:            state.Focus == FocusPreview,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	left := lipgloss.JoinVertical(lipgloss.Left, queryBox, resultsBox)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", searchColumnGap), previewBox)
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

func renderPreviewContent(state State, width int) []string {
	if len(state.Results) == 0 || strings.TrimSpace(state.SelectedID) == "" {
		return []string{
			styles.TruncateString("No selected result.", width),
		}
	}

	selectionID := state.SelectedID
	if strings.TrimSpace(state.SelectedDetail.Summary.ID) == "" {
		for _, issue := range state.Results {
			if issue.ID == state.SelectedID {
				return strings.Split(uidetails.Render(uidetails.State{
					SelectionID: selectionID,
					Detail:      domain.IssueDetail{Summary: issue},
					Width:       width,
					Compact:     true,
				}), "\n")
			}
		}
	}

	return strings.Split(uidetails.Render(uidetails.State{
		SelectionID: selectionID,
		Detail:      state.SelectedDetail,
		Width:       width,
		Compact:     true,
	}), "\n")
}

func splitWidths(total int) (left, right int) {
	available := total - searchColumnGap
	if available < minLeftPaneWidth+minRightPaneWidth {
		available = minLeftPaneWidth + minRightPaneWidth
	}
	left = (available * 45) / 100
	right = available - left
	if left < minLeftPaneWidth {
		left = minLeftPaneWidth
		right = available - left
	}
	if right < minRightPaneWidth {
		right = minRightPaneWidth
		left = available - right
	}
	return left, right
}
