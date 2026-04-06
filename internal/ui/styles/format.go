package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TruncateString truncates a string to fit maxWidth with ellipsis.
func TruncateString(s string, maxWidth int) string {
	if maxWidth < 1 {
		return ""
	}

	if lipgloss.Width(s) <= maxWidth {
		return s
	}

	if maxWidth <= 3 {
		return strings.Repeat(".", maxWidth)
	}

	result := ""
	for _, r := range s {
		next := result + string(r)
		if lipgloss.Width(next) > maxWidth-3 {
			break
		}
		result = next
	}

	return result + "..."
}

// FormatCommentIndicator renders `<n>💬` when count > 0.
func FormatCommentIndicator(count int) string {
	if count <= 0 {
		return ""
	}

	return fmt.Sprintf("%d\U0001F4AC", count)
}
