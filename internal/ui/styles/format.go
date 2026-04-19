package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	selectionSelectedPrefix = "› "
	selectionIdlePrefix     = "  "
)

// SelectionPrefix returns the shared 2-character selection gutter prefix.
// The plain variant is unstyled and should be used for width math/truncation.
// The rendered variant applies app-wide selection styling when requested.
func SelectionPrefix(selected, styled bool) (plain string, rendered string) {
	if !selected {
		return selectionIdlePrefix, selectionIdlePrefix
	}

	if styled {
		return selectionSelectedPrefix, SelectionIndicatorStyle.Render("›") + " "
	}

	return selectionSelectedPrefix, selectionSelectedPrefix
}

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
