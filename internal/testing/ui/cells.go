package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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

// ForceTrueColor renders with full colour for the rest of the test. The suite
// otherwise renders without any, which hides what a background says. The
// profile is process-wide, so a test that calls this must not be parallel.
func ForceTrueColor(tb testing.TB) {
	tb.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	tb.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

var backgroundPattern = regexp.MustCompile(`\x1b\[[0-9;]*48;2;(\d+;\d+;\d+)`)

// RowBand returns the background colour drawn immediately before text on its
// row, as "r;g;b", or "" when that cell has none. It is how a test tells the
// selected row's band from the hover's and from a plain row. It needs
// ForceTrueColor, and the text must appear exactly once.
func RowBand(tb testing.TB, view, text string) string {
	tb.Helper()

	_, y := FindCell(tb, view, text)
	line := strings.Split(view, "\n")[y]
	before, _, _ := strings.Cut(line, text)

	// The band is whatever background is still on where the text starts: the
	// last one set, unless a reset came after it.
	set := backgroundPattern.FindAllStringSubmatchIndex(before, -1)
	if len(set) == 0 {
		return ""
	}
	last := set[len(set)-1]
	if strings.Contains(before[last[1]:], "\x1b[0m") {
		return ""
	}
	return before[last[2]:last[3]]
}
