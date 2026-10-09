package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestKeyLegendDropsTrailingHintsThatDoNotFit(t *testing.T) {
	hints := []KeyHint{{Key: "q", Desc: "quit"}, {Key: "?", Desc: "help"}, {Desc: "read only"}, {Key: "enter", Desc: "open"}}
	const all = "q quit · ? help · read only · enter open"

	if got := KeyLegend(hints, 0); got != all {
		t.Fatalf("width 0 draws every hint: got %q, want %q", got, all)
	}
	if got := KeyLegend(hints, lipgloss.Width(all)); got != all {
		t.Fatalf("a legend that fits exactly was cut: %q", got)
	}

	for width := lipgloss.Width("q quit"); width < lipgloss.Width(all); width++ {
		got := KeyLegend(hints, width)
		if lipgloss.Width(got) > width {
			t.Errorf("width %d: legend is %d cells wide: %q", width, lipgloss.Width(got), got)
		}
		// What is left is whole hints from the front, never a cut one.
		if !strings.HasPrefix(all, got) || (got != "" && !strings.HasPrefix(all[len(got):], legendSeparator)) {
			t.Errorf("width %d: legend %q is not a run of whole leading hints", width, got)
		}
	}

	if got := KeyLegend(hints, lipgloss.Width("q quit · ? help")); got != "q quit · ? help" {
		t.Errorf("expected the two hints that fit, got %q", got)
	}
	// The first hint is cut rather than dropped: a legend is never empty.
	if got := KeyLegend(hints, lipgloss.Width("q quit")-1); got != "q qu…" {
		t.Errorf("expected the first hint cut to the width, got %q", got)
	}
}

// TestKeyLegendStyledKeepsToTheWidth: the escapes of a styled legend are not
// cells, so the fit is the same with colour on.
func TestKeyLegendStyledKeepsToTheWidth(t *testing.T) {
	forceTrueColor(t)

	hints := []KeyHint{{Key: "q", Desc: "quit"}, {Key: "?", Desc: "help"}}
	got := KeyLegend(hints, 10)
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected a styled legend, got %q", got)
	}
	if lipgloss.Width(got) != lipgloss.Width("q quit") {
		t.Fatalf("expected only the first hint at width 10, got %q", got)
	}
}
