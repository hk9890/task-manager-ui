package detail

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	maxCommentBodyLines        = 18
	maxLogLikeCommentBodyLines = 28
)

func renderComments(comments []domain.IssueComment, width int) []string {
	if len(comments) == 0 {
		return []string{"(no comments)"}
	}

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

	out := make([]string, 0, len(ordered)*6)
	for i, comment := range ordered {
		author := emptyFallback(comment.Author, "unknown")
		timestamp := formatTime(comment.CreatedAt)
		commentHeader := fmt.Sprintf("[%d/%d] %s · %s", i+1, len(ordered), author, timestamp)
		out = append(out, styles.TruncateString(commentHeader, width))

		body, logLike := renderCommentBody(comment.Body, width)
		if logLike {
			out = append(out, styles.TruncateString("  ├─ output", width))
			for _, line := range body {
				if line == "" {
					out = append(out, styles.TruncateString("  │", width))
					continue
				}
				out = append(out, styles.TruncateString("  │ "+line, width))
			}
			out = append(out, styles.TruncateString("  └─", width))
		} else {
			for _, line := range body {
				if line == "" {
					out = append(out, "")
					continue
				}
				out = append(out, styles.TruncateString("  "+line, width))
			}
		}
		if i < len(ordered)-1 {
			out = append(out, styles.TruncateString(strings.Repeat("─", max(8, width)), width))
		}
	}

	return out
}

func renderCommentBody(body string, width int) ([]string, bool) {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\t", "    ")

	logLike := isLogLikeComment(normalized)
	if logLike {
		lines := renderMultiline(normalized, "(empty comment)", max(1, width-4))
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
	kept = append(kept, styles.TruncateString(fmt.Sprintf("… (+%d lines elided)", elided), width))
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

func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return "unknown time"
	}
	return ts.Format(timeLayout)
}
