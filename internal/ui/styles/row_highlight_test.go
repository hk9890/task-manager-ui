package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// bandOf is the background sequence RowHighlight opens a band with.
func bandOf(color lipgloss.AdaptiveColor) string {
	on, _, _ := strings.Cut(lipgloss.NewStyle().Background(color).Render("x"), "x")
	return on
}

// TestRowHighlightBandsTheWholeRow pins the band: it spans the row's width,
// survives the reset that ends every styled run inside the row, and is the
// selection's on a row that is both selected and under the pointer.
func TestRowHighlightBandsTheWholeRow(t *testing.T) {
	forceTrueColor(t)

	row := lipgloss.NewStyle().Foreground(TextMutedColor).Render("ab") + " " +
		lipgloss.NewStyle().Bold(true).Render("cd")

	selected, hover := bandOf(RowSelectedBgColor), bandOf(RowHoverBgColor)
	if selected == "" || hover == "" || selected == hover {
		t.Fatalf("the two bands must be two colours, got %q and %q", selected, hover)
	}

	cases := []struct {
		name              string
		selected, hovered bool
		want              string
	}{
		{name: "selected", selected: true, want: selected},
		{name: "hovered", hovered: true, want: hover},
		{name: "selected and hovered", selected: true, hovered: true, want: selected},
	}
	for _, tc := range cases {
		got := RowHighlight(row, 12, tc.selected, tc.hovered)
		if lipgloss.Width(got) != 12 {
			t.Errorf("%s: band is %d cells wide, want the row width 12", tc.name, lipgloss.Width(got))
		}
		if !strings.HasPrefix(got, tc.want) || !strings.HasSuffix(got, sgrReset) {
			t.Errorf("%s: band must open with %q and close with a reset, got %q", tc.name, tc.want, got)
		}
		// Every run inside the row ends in a reset; the band is back on after
		// each one, so no cell of the row is left bare.
		inner := strings.TrimSuffix(got, sgrReset)
		if strings.Count(inner, sgrReset) != strings.Count(inner, sgrReset+tc.want) {
			t.Errorf("%s: a reset inside the row is not followed by the band: %q", tc.name, got)
		}
	}

	if got := RowHighlight(row, 12, false, false); got != row {
		t.Errorf("a plain row changed: %q", got)
	}
	if got := RowHighlight(row, 3, true, false); lipgloss.Width(got) != lipgloss.Width(row) {
		t.Errorf("a row wider than the width was cut or padded: %q", got)
	}
}

// TestHoverRolesStayDistinctOnEveryTerminal: a hover role says something only
// while it differs from the role next to it. The light values of the two bands
// were one 256-colour palette entry, and the hovered tab's light value was the
// inactive tab's, so on those terminals the pointer marked nothing.
func TestHoverRolesStayDistinctOnEveryTerminal(t *testing.T) {
	previousProfile, previousDark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
		lipgloss.SetHasDarkBackground(previousDark)
	})

	for name, profile := range map[string]termenv.Profile{
		"true colour": termenv.TrueColor, "256 colours": termenv.ANSI256, "16 colours": termenv.ANSI,
	} {
		for _, dark := range []bool{true, false} {
			lipgloss.SetColorProfile(profile)
			lipgloss.SetHasDarkBackground(dark)

			if bandOf(RowSelectedBgColor) == bandOf(RowHoverBgColor) {
				t.Errorf("%s, dark %v: the hover band is the selection band %q", name, dark, bandOf(RowHoverBgColor))
			}
			// Sixteen colours hold one light grey, so the two tab roles cannot
			// differ there on a light background.
			tabsAlike := sameStyle(lipgloss.NewStyle().Foreground(ShellTabHoverColor), lipgloss.NewStyle().Foreground(ShellTabInactiveColor))
			if profile != termenv.ANSI && tabsAlike {
				t.Errorf("%s, dark %v: a hovered tab is drawn as an inactive one", name, dark)
			}
		}
	}
}

// TestRowHighlightIsNothingWithoutColour: on a terminal with no colour the
// band cannot be drawn, and the selection's gutter chevron is what is left.
func TestRowHighlightIsNothingWithoutColour(t *testing.T) {
	if got := RowHighlight("row", 10, true, true); got != "row" {
		t.Fatalf("a colourless terminal changed the row: %q", got)
	}
}
