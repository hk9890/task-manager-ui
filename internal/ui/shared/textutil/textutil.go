// Package textutil holds the ANSI-aware terminal text measurement and cutting
// helpers shared across the ui/* renderers: clamping, ANSI stripping, width
// padding, truncation and wrapping.
//
// All five live here rather than being split with internal/ui/styles, which
// owns colour roles and shell chrome. The split forced DESIGN-GUIDE.md to spell
// out which package held which operation, and made textutil import styles to
// finish its own work.
package textutil

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Clamp returns value bounded to the inclusive [low, high] range.
func Clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripANSI removes SGR (color/style) escape sequences from s.
func StripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// PadToWidth right-pads value with spaces to the given rendered width, or
// truncates it (preserving styling) when it is already at least that wide.
func PadToWidth(value string, width int) string {
	renderedWidth := lipgloss.Width(value)
	if renderedWidth >= width {
		return TruncateString(value, width)
	}
	return value + strings.Repeat(" ", width-renderedWidth)
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
