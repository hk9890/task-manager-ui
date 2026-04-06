package search

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
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
	resultsHeight := maxInt(6, height-queryHeight-1)
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

	if strings.TrimSpace(state.Query) == "" {
		return []string{
			styles.TruncateString("Start typing to search issues.", width),
			"",
			styles.TruncateString("Type a word or phrase to search across issue content.", width),
			styles.TruncateString("Then use j/k for results, l for preview, enter for detail.", width),
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
		prefix := "  "
		if issue.ID == state.SelectedID {
			prefix = "› "
		}

		meta := strings.Join([]string{
			compactIssueType(issue.Type),
			compactPriority(issue.Priority),
			compactIssueState(issue.Status),
			compactIssueID(issue.ID, maxInt(7, width/5)),
		}, " ")
		base := prefix + meta + " "
		titleWidth := width - lipgloss.Width(base)
		if titleWidth < 8 {
			lines = append(lines, styles.TruncateString(prefix+meta, width))
			continue
		}
		lines = append(lines, base+styles.TruncateString(issue.Title, titleWidth))
	}

	return lines
}

func renderPreviewContent(state State, width int) []string {
	if strings.TrimSpace(state.Query) == "" {
		return []string{
			"Selected issue preview",
			"",
			styles.TruncateString("Results appear on the left and the current selection is previewed here.", width),
		}
	}

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

func compactIssueType(issueType string) string {
	switch normalizeToken(issueType) {
	case "bug":
		return "B"
	case "task":
		return "T"
	case "feature":
		return "F"
	case "epic":
		return "E"
	case "chore":
		return "C"
	case "docs":
		return "D"
	case "spike":
		return "S"
	default:
		return "?"
	}
}

func compactPriority(priority int) string {
	if priority < 0 {
		priority = 0
	}
	return fmt.Sprintf("P%d", priority)
}

func compactIssueState(status string) string {
	switch normalizeToken(status) {
	case "blocked":
		return "BLK"
	case "in_progress":
		return "IP"
	case "open":
		return "OPN"
	case "closed":
		return "CLS"
	case "ready":
		return "RDY"
	default:
		tok := strings.ToUpper(normalizeToken(status))
		if tok == "" {
			return "---"
		}
		runes := []rune(tok)
		if len(runes) > 3 {
			return string(runes[:3])
		}
		return tok
	}
}

func compactIssueID(id string, maxWidth int) string {
	trimmed := strings.TrimSpace(id)
	if lipgloss.Width(trimmed) <= maxWidth {
		return trimmed
	}
	const repoPrefix = "beads-workbench-"
	if strings.HasPrefix(trimmed, repoPrefix) {
		trimmed = strings.TrimPrefix(trimmed, repoPrefix)
		if lipgloss.Width(trimmed) <= maxWidth {
			return trimmed
		}
	}
	if maxWidth <= 1 {
		return styles.TruncateString(trimmed, maxWidth)
	}
	runes := []rune(trimmed)
	suffixWidth := maxWidth - 1
	if suffixWidth <= 0 || len(runes) <= suffixWidth {
		return trimmed
	}
	return "…" + string(runes[len(runes)-suffixWidth:])
}

func normalizeToken(raw string) string {
	tok := strings.TrimSpace(strings.ToLower(raw))
	tok = strings.ReplaceAll(tok, "-", "_")
	tok = strings.ReplaceAll(tok, " ", "_")
	return tok
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
