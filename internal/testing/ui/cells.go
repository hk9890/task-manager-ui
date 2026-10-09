package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// FindCell returns the terminal cell where text starts in a rendered view,
// with (0, 0) the view's first cell. It is how a hit-test test asks "where was
// this drawn" of the renderer itself instead of repeating its arithmetic. The
// text must appear exactly once.
func FindCell(tb testing.TB, view, text string) (x, y int) {
	tb.Helper()

	found := false
	for row, line := range strings.Split(AnsiEscapePattern.ReplaceAllString(view, ""), "\n") {
		idx := strings.Index(line, text)
		if idx < 0 {
			continue
		}
		if found || strings.Count(line, text) > 1 {
			tb.Fatalf("FindCell: %q appears more than once in view:\n%s", text, view)
		}
		found = true
		x, y = lipgloss.Width(line[:idx]), row
	}
	if !found {
		tb.Fatalf("FindCell: %q not found in view:\n%s", text, view)
	}
	return x, y
}
