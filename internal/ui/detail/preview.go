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
	// read, so the header shows the row the operator selected; with no ID the
	// pane is skeleton rows alone.
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

// PreviewPageRows is the lines a page key scrolls the pane: its rows less the
// two the scroll marks may take.
func PreviewPageRows(state PreviewState) int {
	return max(1, state.Height-2-2)
}

func previewLines(state PreviewState, width, height int) []string {
	if state.Empty {
		return []string{lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(textutil.TruncateString("Nothing selected", width))}
	}

	summary := state.Detail.Summary
	if state.Skeleton && summary.ID == "" {
		return renderProseContentSkeleton(width, height, state.SkeletonPhase)
	}
	lines := []string{
		contentHeaderMetaRow(summary, width),
		textutil.TruncateString(displaytext.OneLine(emptyFallback(summary.Title, "(untitled)")), width),
		contentHeaderRule(width),
	}

	switch {
	case strings.TrimSpace(state.Error) != "":
		message := styles.Glyphs.ToastWarn + " load failed: " + displaytext.OneLine(state.Error)
		lines = append(lines, textutil.WrapAtSpaces(message, width)...)
	case state.Skeleton:
		lines = append(lines, renderProseContentSkeleton(width, max(1, height-len(lines)), state.SkeletonPhase)...)
	default:
		lines = append(lines, renderMarkdownMultiline(state.Detail.Description, "(no description)", width)...)
		if strings.TrimSpace(state.Detail.Notes) != "" {
			lines = append(lines, "", "Notes")
			lines = append(lines, renderMarkdownMultiline(state.Detail.Notes, "(no notes)", width)...)
		}
	}
	return lines
}
