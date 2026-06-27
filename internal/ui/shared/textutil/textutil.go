// Package textutil holds small, generic terminal-text helpers shared across the
// ui/* renderers (clamping, ANSI stripping, width padding). These were
// previously copy-pasted per package.
package textutil

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/styles"
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
		return styles.TruncateString(value, width)
	}
	return value + strings.Repeat(" ", width-renderedWidth)
}
