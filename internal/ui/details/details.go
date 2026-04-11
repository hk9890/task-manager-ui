package details

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
	"github.com/hk9890/beads-workbench/internal/ui/shared/markdown"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const (
	defaultDetailWidth = 80
	timeLayout         = "2006-01-02 15:04"

	// InspectorTwoColumnMinWidth is the detail width breakpoint where the
	// inspector layout switches from one column to content+metadata rail.
	InspectorTwoColumnMinWidth = 110

	detailColumnGap       = 2
	metadataRailMinWidth  = 28
	contentColumnMinWidth = 50
)

var readOnlyMarkdown = markdown.NewRenderer()

// State is the issue detail renderer input.
type State struct {
	SelectionID string
	TargetID    string
	Detail      domain.IssueDetail
	Loading     bool
	Error       string
	Width       int
	Compact     bool
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

	if state.Compact {
		return renderCompact(detail, width)
	}

	if width >= InspectorTwoColumnMinWidth {
		return renderTwoColumn(detail, width)
	}

	return renderSingleColumn(detail, width)
}

func renderSingleColumn(detail domain.IssueDetail, width int) string {
	out := make([]string, 0, 64)
	out = append(out,
		styles.TruncateString(emptyFallback(detail.Summary.Title, "(untitled)"), width),
		styles.TruncateString(detail.Summary.ID, width),
	)

	metadata := renderMetadataRail(detail, width)
	if len(metadata) > 0 {
		out = append(out, "")
		out = append(out, "Metadata")
		out = append(out, metadata...)
	}

	out = append(out, renderContentSections(detail, width)...)

	return strings.Join(out, "\n")
}

func renderTwoColumn(detail domain.IssueDetail, width int) string {
	contentWidth, metadataWidth := splitInspectorWidths(width)

	left := make([]string, 0, 64)
	left = append(left,
		styles.TruncateString(emptyFallback(detail.Summary.Title, "(untitled)"), contentWidth),
		styles.TruncateString(detail.Summary.ID, contentWidth),
	)
	left = append(left, renderContentSections(detail, contentWidth)...)

	right := []string{"Metadata"}
	metadata := renderMetadataRail(detail, metadataWidth)
	if len(metadata) == 0 {
		right = append(right, "(none)")
	} else {
		right = append(right, metadata...)
	}

	maxLines := len(left)
	if len(right) > maxLines {
		maxLines = len(right)
	}

	merged := make([]string, 0, maxLines)
	for i := 0; i < maxLines; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		merged = append(merged, padToWidth(l, contentWidth)+strings.Repeat(" ", detailColumnGap)+styles.TruncateString(r, metadataWidth))
	}

	return strings.Join(merged, "\n")
}

func renderContentSections(detail domain.IssueDetail, width int) []string {
	out := make([]string, 0, 56)
	out = append(out, "")
	out = append(out, "Description")
	out = append(out, renderMarkdownMultiline(detail.Description, "(no description)", width)...)

	out = append(out, "")
	out = append(out, "Notes")
	out = append(out, renderMarkdownMultiline(detail.Notes, "(no notes)", width)...)

	out = append(out, "")
	out = append(out, fmt.Sprintf("Comments (%d)", len(detail.Comments)))
	out = append(out, renderComments(detail.Comments, width)...)

	out = append(out, "")
	out = append(out, "Related Work")
	out = append(out, renderReferences("Blocked by", detail.BlockedBy, width)...)
	out = append(out, renderReferences("Blocks", detail.Blocks, width)...)
	out = append(out, renderReferences("Related", detail.Related, width)...)

	return out
}

func splitInspectorWidths(total int) (content, metadata int) {
	available := total - detailColumnGap
	metadata = max(metadataRailMinWidth, (available*32)/100)
	if available-metadata < contentColumnMinWidth {
		metadata = available - contentColumnMinWidth
	}
	if metadata < metadataRailMinWidth {
		metadata = metadataRailMinWidth
	}
	content = available - metadata
	if content < 1 {
		content = 1
	}
	if metadata < 1 {
		metadata = 1
	}
	return content, metadata
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
		styles.TruncateString(detail.Summary.ID, width),
	)
	metadata := renderMetadataRail(detail, width)
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

func renderPreviewLines(text, fallback string, width, maxLines int) []string {
	lines := renderMultiline(text, fallback, width)
	if len(lines) <= maxLines {
		return lines
	}

	trimmed := append([]string(nil), lines[:maxLines-1]...)
	trimmed = append(trimmed, styles.TruncateString("…", width))
	return trimmed
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

func renderLabels(labels []string) string {
	if len(labels) == 0 {
		return "(none)"
	}
	trimmed := make([]string, 0, len(labels))
	for _, label := range labels {
		if text := strings.TrimSpace(label); text != "" {
			trimmed = append(trimmed, text)
		}
	}
	if len(trimmed) == 0 {
		return "(none)"
	}
	sort.Strings(trimmed)
	return strings.Join(trimmed, ", ")
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
		for _, line := range renderMultiline(comment.Body, "(empty comment)", width-2) {
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

func renderReferences(label string, refs []domain.IssueReference, width int) []string {
	if len(refs) == 0 {
		return []string{label + ": (none)"}
	}

	ordered := append([]domain.IssueReference(nil), refs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})

	out := []string{fmt.Sprintf("%s (%d)", label, len(ordered))}
	for _, ref := range ordered {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			id = "(unknown-id)"
		}
		title := emptyFallback(ref.Title, "(no title)")
		out = append(out, styles.TruncateString(fmt.Sprintf("%s · %s", id, title), width))
	}

	return out
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
