package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// TruncateString truncates a string to fit maxWidth with a Unicode ellipsis (…).
// The ellipsis glyph has rendered width 1, so more of the original content is
// preserved compared with a three-dot ASCII tail.
func TruncateString(s string, maxWidth int) string {
	if maxWidth < 1 {
		return ""
	}

	if lipgloss.Width(s) <= maxWidth {
		return s
	}

	return ansi.Truncate(s, maxWidth, "…")
}

// WrapLines word-wraps s to maxWidth and returns the resulting lines.
// Falls back to hard-wrap for tokens longer than maxWidth.
func WrapLines(s string, maxWidth int) []string {
	if maxWidth < 1 {
		return []string{""}
	}
	if lipgloss.Width(s) <= maxWidth {
		return []string{s}
	}
	return strings.Split(ansi.Wrap(s, maxWidth, " -"), "\n")
}
