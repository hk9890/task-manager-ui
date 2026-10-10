package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// bandOf is the background sequence RowHighlight opens a band with.
func bandOf(color lipgloss.Color) string {
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

// TestHoverIsOneSurfaceBelowTheSelectionInEveryTheme: the row or the button
// under the pointer is lit a step below the selected row, and both are lighter
// than the terminal's background. In true colour the two bands are two colours
// in every theme.
func TestHoverIsOneSurfaceBelowTheSelectionInEveryTheme(t *testing.T) {
	forceTrueColor(t)
	restoreInitialStyles(t)

	for _, theme := range Themes() {
		if err := Apply(theme, initialGlyphs); err != nil {
			t.Fatalf("Apply(%q): %v", theme, err)
		}

		flavor := flavors[theme]
		surface0, surface1 := lipgloss.Color(flavor.Surface0().Hex), lipgloss.Color(flavor.Surface1().Hex)
		if RowHoverBgColor != surface0 || ShellHoverBgColor != surface0 {
			t.Errorf("%s: the hover is %q on a row and %q on the header, want surface0 %q", theme, RowHoverBgColor, ShellHoverBgColor, surface0)
		}
		if RowSelectedBgColor != surface1 {
			t.Errorf("%s: the selection is %q, want surface1 %q", theme, RowSelectedBgColor, surface1)
		}
		if bandOf(RowSelectedBgColor) == bandOf(RowHoverBgColor) {
			t.Errorf("%s: the hover band is the selection band %q", theme, bandOf(RowHoverBgColor))
		}
	}
}

// TestHoveredTabStaysDistinctOnEveryTerminal: a hover role says something only
// while it differs from the role next to it. The hovered tab's light value was
// the inactive tab's, so on a terminal without true colour the pointer marked
// nothing.
//
// The two row bands are not held to this. The hover is the surface next to the
// selection's, and a 256- or a 16-colour terminal can draw the two as one
// palette entry: the selection's gutter bar tells the rows apart there.
func TestHoveredTabStaysDistinctOnEveryTerminal(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })
	restoreInitialStyles(t)

	for name, profile := range map[string]termenv.Profile{
		"true colour": termenv.TrueColor, "256 colours": termenv.ANSI256, "16 colours": termenv.ANSI,
	} {
		for _, theme := range Themes() {
			lipgloss.SetColorProfile(profile)
			if err := Apply(theme, initialGlyphs); err != nil {
				t.Fatalf("Apply(%q): %v", theme, err)
			}

			tabsAlike := sameStyle(lipgloss.NewStyle().Foreground(ShellTabHoverColor), lipgloss.NewStyle().Foreground(ShellTabInactiveColor))
			if tabsAlike {
				t.Errorf("%s, %s: a hovered tab is drawn as an inactive one", name, theme)
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
