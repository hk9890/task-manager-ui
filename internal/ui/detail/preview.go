package detail

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// previewTitle is the title in the top border of the Preview pane.
const previewTitle = "Preview"

// PreviewState is the state RenderPreview draws from: the text of one issue,
// read only. It has no button and no comment; Detail has those.
type PreviewState struct {
	// Detail is the issue to show. While Skeleton is set only its Summary is
	// read, so the header shows the row the operator selected.
	Detail       domain.IssueDetail
	Width        int
	Height       int
	ScrollOffset int
	// Skeleton draws placeholder rows in place of the body while it is read.
	Skeleton      bool
	SkeletonPhase int
	// Error is why the issue could not be read. It stands in place of the body.
	Error string
	// Empty says no issue is selected. The pane then says so and nothing else.
	Empty bool
}

// RenderPreview draws the Preview pane: the type, priority, status and ID of
// the issue, its title, a rule, and its description and notes as markdown.
// Lines below the pane are reached with ScrollOffset, and the window marks the
// lines it does not show, as the Content pane does.
func RenderPreview(state PreviewState) string {
	innerWidth, innerHeight := state.Width-2, max(1, state.Height-2)
	view, _ := sliceWithOffset(previewLines(state, innerWidth, innerHeight), state.ScrollOffset, innerHeight, innerWidth)
	return styles.FormSection(styles.FormSectionConfig{
		Width:   state.Width,
		Height:  state.Height,
		TopLeft: previewTitle,
		Content: view,
	})
}

// PreviewMaxOffset is the largest ScrollOffset that leaves no blank row at the
// bottom of the pane.
func PreviewMaxOffset(state PreviewState) int {
	innerHeight := max(1, state.Height-2)
	return max(0, len(previewLines(state, state.Width-2, innerHeight))-innerHeight)
}

func previewLines(state PreviewState, width, height int) []string {
	muted := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	if state.Empty {
		return []string{muted.Render(textutil.TruncateString("Nothing selected", width))}
	}

	summary := state.Detail.Summary
	lines := []string{
		contentHeaderMetaRow(summary, width),
		lipgloss.NewStyle().Bold(true).Render(textutil.TruncateString(displaytext.OneLine(emptyFallback(summary.Title, "(untitled)")), width)),
		contentHeaderRule(width),
	}

	switch {
	case strings.TrimSpace(state.Error) != "":
		message := styles.Glyphs.ToastWarn + " load failed: " + displaytext.OneLine(state.Error)
		for _, line := range textutil.WrapAtSpaces(message, width) {
			lines = append(lines, lipgloss.NewStyle().Foreground(styles.ErrorTextColor).Render(line))
		}
	case state.Skeleton:
		lines = append(lines, renderProseContentSkeleton(width, max(1, height-len(lines)), state.SkeletonPhase)...)
	default:
		lines = append(lines, renderMarkdownMultiline(state.Detail.Description, "(no description)", width)...)
		if strings.TrimSpace(state.Detail.Notes) != "" {
			lines = append(lines, "", muted.Bold(true).Render("Notes"))
			lines = append(lines, renderMarkdownMultiline(state.Detail.Notes, "(no notes)", width)...)
		}
	}
	return lines
}
