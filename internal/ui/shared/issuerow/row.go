package issuerow

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/ui/shared/renderhelpers"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const (
	minTitleWidth      = 8
	minCompactIDWidth  = 7
	maxCompactIDWidth  = 12
	selectedPrefixText = "› "
	idlePrefixText     = "  "
)

// RenderConfig configures compact issue row rendering.
type RenderConfig struct {
	// Issue uses domain.IssueSummary directly because compact rows need only
	// canonical summary fields (id/title/type/status/priority). This keeps board
	// and search on one data shape and removes adapter-only row structs.
	Issue    domain.IssueSummary
	Selected bool
	Width    int
	Styled   bool
}

// RenderCompact renders one compact issue row with shared metadata semantics.
func RenderCompact(config RenderConfig) string {
	prefixPlain := idlePrefixText
	prefixStyled := idlePrefixText
	if config.Selected {
		prefixPlain = selectedPrefixText
		if config.Styled {
			prefixStyled = styles.SelectionIndicatorStyle.Render("›") + " "
		} else {
			prefixStyled = selectedPrefixText
		}
	}

	title := strings.TrimSpace(config.Issue.Title)
	if title == "" {
		title = "(untitled)"
	}

	idWidth := CompactIDWidth(config.Width)
	metaPlain := strings.Join([]string{
		renderhelpers.CompactIssueType(config.Issue.Type),
		renderhelpers.CompactPriority(config.Issue.Priority),
		renderhelpers.CompactIssueState(config.Issue.Status),
		renderhelpers.CompactIssueID(config.Issue.ID, idWidth),
	}, " ")
	metaStyled := metaPlain
	if config.Styled {
		metaStyled = strings.Join([]string{
			renderhelpers.CompactIssueTypeStyled(config.Issue.Type),
			renderhelpers.CompactPriorityStyled(config.Issue.Priority),
			renderhelpers.CompactIssueStateStyled(config.Issue.Status),
			renderhelpers.CompactIssueIDMuted(config.Issue.ID, idWidth),
		}, " ")
	}

	titlePrefix := prefixPlain + metaPlain + " "
	titleWidth := config.Width - lipgloss.Width(titlePrefix)
	if titleWidth < minTitleWidth {
		return styles.TruncateString(prefixPlain+metaPlain, config.Width)
	}

	return prefixStyled + metaStyled + " " + styles.TruncateString(title, titleWidth)
}

// CompactIDWidth returns the shared max width for compact issue IDs.
func CompactIDWidth(width int) int {
	return min(maxCompactIDWidth, max(minCompactIDWidth, width/5))
}
