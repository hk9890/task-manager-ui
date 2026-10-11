package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestKeyLegendDropsTrailingHintsThatDoNotFit(t *testing.T) {
	hints := []KeyHint{{Key: "q", Desc: "quit"}, {Key: "?", Desc: "help"}, {Desc: "read only"}, {Key: "enter", Desc: "open"}}
	const all = "q quit • ? help • read only • enter open"

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
		// What is left is whole hints from the front, never a cut one, and the
		// mark of the drop where it fits.
		kept := strings.TrimSuffix(got, legendCut)
		if !strings.HasPrefix(all, kept) || !strings.HasPrefix(all[len(kept):], legendSeparator) {
			t.Errorf("width %d: legend %q is not a run of whole leading hints", width, got)
		}
		if marked := kept != got; marked != (lipgloss.Width(kept)+lipgloss.Width(legendCut) <= width) {
			t.Errorf("width %d: legend %q: the mark is drawn exactly where it fits", width, got)
		}
	}

	if got := KeyLegend(hints, lipgloss.Width("q quit • ? help")); got != "q quit • ? help" {
		t.Errorf("expected the two hints that fit and no room for the mark, got %q", got)
	}
	if got := KeyLegend(hints, lipgloss.Width("q quit • ? help …")); got != "q quit • ? help …" {
		t.Errorf("expected the two hints that fit and the mark, got %q", got)
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
	if lipgloss.Width(got) != lipgloss.Width("q quit …") {
		t.Fatalf("expected the first hint and the mark at width 10, got %q", got)
	}
}

// TestKeyLegendMutesTheKeyAndNotWhatItDoes: the key is the muted run and the
// description the bright one, the separator is the rule's colour, and the
// mark of a drop is muted as a key is.
func TestKeyLegendMutesTheKeyAndNotWhatItDoes(t *testing.T) {
	forceTrueColor(t)

	run := func(role lipgloss.Color, text string) string {
		return lipgloss.NewStyle().Foreground(role).Render(text)
	}
	if ShellFooterHelpColor == TextPrimaryColor || ShellRuleColor == ShellFooterHelpColor {
		t.Fatal("fixture: the legend's roles share a colour")
	}

	hints := []KeyHint{{Key: "q", Desc: "quit"}, {Key: "?", Desc: "help"}, {Key: "enter", Desc: "open"}}
	want := run(ShellFooterHelpColor, "q") + " " + run(TextPrimaryColor, "quit") +
		run(ShellRuleColor, " • ") +
		run(ShellFooterHelpColor, "?") + " " + run(TextPrimaryColor, "help") +
		run(ShellFooterHelpColor, " …")
	if got := KeyLegend(hints, lipgloss.Width("q quit • ? help …")); got != want {
		t.Errorf("legend:\n got %q\nwant %q", got, want)
	}
}
