package detail

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/markdown"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	maxCommentBodyLines        = 18
	maxLogLikeCommentBodyLines = 28
)

// OrderedComments is comments in the order the Content pane draws them: the
// newest first, and the one without a time last.
func OrderedComments(comments []domain.IssueComment) []domain.IssueComment {
	ordered := append([]domain.IssueComment(nil), comments...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i].CreatedAt
		right := ordered[j].CreatedAt
		if left.Equal(right) {
			return ordered[i].ID > ordered[j].ID
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.After(right)
	})
	return ordered
}

// renderComments draws the comments, and says for each line which comment it
// belongs to: -1 for the three lines between two comments. Every comment line
// starts with the selection gutter, so the line count does not follow the
// cursor. The comment at index selected, -1 for none, has the bar on each of
// its lines and buttons at the right end of its header line; placed is their
// cells.
func renderComments(comments []domain.IssueComment, width, selected int, buttons ...string) (lines []string, owner []int, placed []cells) {
	ordered := OrderedComments(comments)
	gutterPlain, _ := styles.SelectionPrefix(false, false)
	gutterWidth := lipgloss.Width(gutterPlain)
	textWidth := max(1, width-gutterWidth)
	bold := lipgloss.NewStyle().Bold(true)

	for i, comment := range ordered {
		if i > 0 {
			lines = append(lines, "", contentHeaderRule(width), "")
			owner = append(owner, -1, -1, -1)
		}
		_, gutter := styles.SelectionPrefix(i == selected, true)
		add := func(line string) {
			lines = append(lines, gutter+textutil.TruncateString(line, textWidth))
			owner = append(owner, i)
		}

		author := displaytext.OneLine(emptyFallback(comment.Author, "unknown"))
		header := bold.Render(textutil.TruncateString(fmt.Sprintf("[%d/%d] %s · %s", i+1, len(ordered), author, FormatTime(comment.CreatedAt)), textWidth))
		if i == selected {
			header, placed = placeButtons(header, textWidth, buttons...)
			for idx := range placed {
				placed[idx].x0 += gutterWidth
				placed[idx].x1 += gutterWidth
			}
		}
		add(header)

		body, logLike := renderCommentBody(comment.Body, width)
		if !logLike {
			for _, line := range body {
				add(line)
			}
			continue
		}
		add("├─ output")
		for _, line := range body {
			add(strings.TrimRight("│ "+line, " "))
		}
		add("└─")
	}

	return lines, owner, placed
}

func renderCommentBody(body string, width int) ([]string, bool) {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\t", "    ")

	logLike := isLogLikeComment(normalized)
	if logLike {
		lines := strings.Split(markdown.RenderPlain(normalized, max(1, width-4)), "\n")
		return applyCommentElision(lines, maxLogLikeCommentBodyLines, max(1, width-4)), true
	}

	lines := renderMarkdownMultiline(normalized, "(empty comment)", max(1, width-2))
	return applyCommentElision(lines, maxCommentBodyLines, max(1, width-2)), false
}

func applyCommentElision(lines []string, maxLines, width int) []string {
	if maxLines <= 0 || len(lines) <= maxLines {
		return lines
	}
	kept := append([]string(nil), lines[:maxLines-1]...)
	elided := len(lines) - (maxLines - 1)
	kept = append(kept, textutil.TruncateString(fmt.Sprintf("… (+%d lines elided)", elided), width))
	return kept
}

func isLogLikeComment(body string) bool {
	if strings.TrimSpace(body) == "" {
		return false
	}
	if strings.Contains(body, "```") {
		return true
	}

	lines := strings.Split(body, "\n")
	longLines := 0
	tabLines := 0
	indicatorLines := 0

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if raw != "" && strings.Contains(raw, "\t") {
			tabLines++
		}
		if len([]rune(raw)) >= 100 {
			longLines++
		}
		if line == "" {
			continue
		}
		if hasLogIndicator(line) {
			indicatorLines++
		}
	}

	return tabLines >= 2 || longLines >= 3 || indicatorLines >= 3
}

func hasLogIndicator(line string) bool {
	indicators := []string{
		"$ ",
		"> ",
		"--- PASS:",
		"--- FAIL:",
		"PASS",
		"FAIL",
		"ok  ",
		"panic:",
		"Error:",
		"[error]",
		"[warn]",
		"stdout",
		"stderr",
	}

	for _, marker := range indicators {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

// FormatTime is a time of an issue or a comment as Detail writes it.
func FormatTime(ts time.Time) string {
	if ts.IsZero() {
		return "unknown time"
	}
	return ts.Format(timeLayout)
}
