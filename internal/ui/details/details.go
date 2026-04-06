package details

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const (
	defaultDetailWidth = 80
	timeLayout         = "2006-01-02 15:04"
)

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

	out := make([]string, 0, 64)
	out = append(out,
		styles.TruncateString(emptyFallback(detail.Summary.Title, "(untitled)"), width),
		styles.TruncateString(fmt.Sprintf("%s · %s · %s · %s", detail.Summary.ID, emptyFallback(detail.Summary.Status, "(unknown)"), emptyFallback(detail.Summary.Type, "(unknown)"), formatPriority(detail.Summary.Priority)), width),
		styles.TruncateString(fmt.Sprintf("Assignee: %s  Labels: %s", emptyFallback(detail.Summary.Assignee, "(unassigned)"), renderLabels(detail.Summary.Labels)), width),
	)

	out = append(out, "")
	out = append(out, "Description")
	out = append(out, renderMultiline(detail.Description, "(no description)", width)...)

	out = append(out, "")
	out = append(out, "Notes")
	out = append(out, renderMultiline(detail.Notes, "(no notes)", width)...)

	out = append(out, "")
	out = append(out, fmt.Sprintf("Comments (%d)", len(detail.Comments)))
	out = append(out, renderComments(detail.Comments, width)...)

	out = append(out, "")
	out = append(out, "Related Work")
	out = append(out, renderReferences("Blocked by", detail.BlockedBy, width)...)
	out = append(out, renderReferences("Blocks", detail.Blocks, width)...)
	out = append(out, renderReferences("Related", detail.Related, width)...)

	return strings.Join(out, "\n")
}

func renderCompact(detail domain.IssueDetail, width int) string {
	out := make([]string, 0, 24)
	out = append(out,
		styles.TruncateString(emptyFallback(detail.Summary.Title, "(untitled)"), width),
	)
	out = append(out,
		styles.TruncateString(fmt.Sprintf("%s · %s · %s · %s", detail.Summary.ID, emptyFallback(detail.Summary.Status, "(unknown)"), emptyFallback(detail.Summary.Type, "(unknown)"), formatPriority(detail.Summary.Priority)), width),
		styles.TruncateString(fmt.Sprintf("Assignee: %s  Labels: %s", emptyFallback(detail.Summary.Assignee, "(unassigned)"), renderLabels(detail.Summary.Labels)), width),
	)

	out = append(out, "")
	out = append(out, "Description")
	out = append(out, renderPreviewLines(detail.Description, "(no description)", width, 8)...)

	out = append(out, "")
	out = append(out, fmt.Sprintf("Comments: %d", len(detail.Comments)))
	if len(detail.BlockedBy) > 0 {
		out = append(out, styles.TruncateString(fmt.Sprintf("Blocked by: %s", summarizeReferenceTitles(detail.BlockedBy, width-12)), width))
	}
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

func summarizeReferences(detail domain.IssueDetail, width int) []string {
	line := fmt.Sprintf("Blocked by: %d  Blocks: %d  Related: %d", len(detail.BlockedBy), len(detail.Blocks), len(detail.Related))
	return []string{styles.TruncateString(line, width)}
}

func summarizeReferenceTitles(refs []domain.IssueReference, width int) string {
	if len(refs) == 0 {
		return "(none)"
	}
	titles := make([]string, 0, len(refs))
	for _, ref := range refs {
		title := strings.TrimSpace(ref.Title)
		if title == "" {
			title = strings.TrimSpace(ref.ID)
		}
		if title != "" {
			titles = append(titles, title)
		}
	}
	if len(titles) == 0 {
		return "(none)"
	}
	return styles.TruncateString(strings.Join(titles, ", "), width)
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
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return []string{fallback}
	}
	lines := strings.Split(trimmed, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			out = append(out, "")
			continue
		}
		out = append(out, styles.TruncateString(clean, width))
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
