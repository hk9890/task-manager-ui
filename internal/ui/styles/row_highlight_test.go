package styles

import (
	"strconv"
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

// terminalProfiles are the colour profiles a band can be drawn on.
var terminalProfiles = map[string]termenv.Profile{
	"true colour": termenv.TrueColor, "256 colours": termenv.ANSI256, "16 colours": termenv.ANSI,
}

// applyOnEveryTerminal applies every theme on every colour profile in turn and
// calls check after each. The hover roles are chosen by the profile, so the
// profile is set before the theme is applied.
func applyOnEveryTerminal(t *testing.T, check func(name, theme string, profile termenv.Profile)) {
	t.Helper()
	// Cleanups run last first: the profile is back before the roles are
	// applied again, so they are the roles of the start profile.
	restoreInitialStyles(t)
	previousProfile := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	for name, profile := range terminalProfiles {
		for _, theme := range Themes() {
			lipgloss.SetColorProfile(profile)
			if err := Apply(theme, initialGlyphs); err != nil {
				t.Fatalf("Apply(%q): %v", theme, err)
			}
			check(name, theme, profile)
		}
	}
}

// TestHoverIsOneSurfaceBelowTheSelectionWhereTheTerminalCanDrawIt: in true
// colour the row or the button under the pointer is lit a step below the
// selected row, and both are lighter than the terminal's background. A
// 256-colour terminal draws those two surfaces as one entry, so the hover is a
// grey of the ramp there, and the 16 colours have no entry for it.
func TestHoverIsOneSurfaceBelowTheSelectionWhereTheTerminalCanDrawIt(t *testing.T) {
	applyOnEveryTerminal(t, func(name, theme string, profile termenv.Profile) {
		flavor := flavors[theme]
		if RowHoverBgColor != ShellHoverBgColor {
			t.Errorf("%s, %s: the hover is %q on a row and %q on the header", name, theme, RowHoverBgColor, ShellHoverBgColor)
		}
		switch profile {
		case termenv.TrueColor:
			if surface0 := lipgloss.Color(flavor.Surface0().Hex); RowHoverBgColor != surface0 {
				t.Errorf("%s, %s: the hover is %q, want surface0 %q", name, theme, RowHoverBgColor, surface0)
			}
		case termenv.ANSI256:
			if entry, err := strconv.Atoi(string(RowHoverBgColor)); err != nil || entry < 232 || entry > 255 {
				t.Errorf("%s, %s: the hover is %q, want an entry of the grey ramp", name, theme, RowHoverBgColor)
			}
		case termenv.ANSI:
			if RowHoverBgColor != "" {
				t.Errorf("%s, %s: the hover is %q, want no band", name, theme, RowHoverBgColor)
			}
		}
		if surface1 := lipgloss.Color(flavor.Surface1().Hex); RowSelectedBgColor != surface1 {
			t.Errorf("%s, %s: the selection is %q, want surface1 %q", name, theme, RowSelectedBgColor, surface1)
		}
	})
}

// TestHoverRolesStayDistinctOnEveryTerminal: a hover role says something only
// while it differs from the role next to it. Surface0 and Surface1 of the dark
// themes are one 256-colour palette entry, the mantle is the entry of the
// background, and the hovered tab's light value was the inactive tab's, so on
// those terminals the pointer marked nothing.
func TestHoverRolesStayDistinctOnEveryTerminal(t *testing.T) {
	applyOnEveryTerminal(t, func(name, theme string, profile termenv.Profile) {
		hover := bandOf(RowHoverBgColor)
		if bandOf(RowSelectedBgColor) == hover {
			t.Errorf("%s, %s: the hover band is the selection band %q", name, theme, hover)
		}
		// The app paints no background, so a row at rest stands on the
		// terminal's, which the user of a theme sets to its base. The 16
		// colours draw no hover band to compare.
		if profile != termenv.ANSI && (hover == "" || hover == bandOf(lipgloss.Color(flavors[theme].Base().Hex))) {
			t.Errorf("%s, %s: the hover band %q is the background of a row at rest", name, theme, hover)
		}

		hoveredTab := lipgloss.NewStyle().Foreground(ShellTabHoverColor).Background(ShellHoverBgColor)
		if sameStyle(hoveredTab, lipgloss.NewStyle().Foreground(ShellTabInactiveColor)) {
			t.Errorf("%s, %s: a hovered tab is drawn as an inactive one", name, theme)
		}
		if sameStyle(hoveredTab, lipgloss.NewStyle().Foreground(ShellTabActiveTextColor).Background(ShellTabActiveBgColor)) {
			t.Errorf("%s, %s: a hovered tab is drawn as the active one", name, theme)
		}
		if sameStyle(lipgloss.NewStyle().Foreground(ShellTabHoverColor), lipgloss.NewStyle().Foreground(ShellTabInactiveColor)) {
			t.Errorf("%s, %s: a hovered tab has the text colour of an inactive one", name, theme)
		}

		// A hovered label is also bold, which is all the 16 colours show.
		button := lipgloss.NewStyle().Foreground(ShellActionColor)
		if sameStyle(button.Background(ShellHoverBgColor).Bold(true), button) {
			t.Errorf("%s, %s: a hovered button is drawn as one at rest", name, theme)
		}
		if profile != termenv.ANSI && sameStyle(button.Background(ShellHoverBgColor), button) {
			t.Errorf("%s, %s: a hovered button has the background of one at rest", name, theme)
		}
	})
}

// TestTheKeyOfAHoveredButtonIsReadableOnEveryTerminal: the key is the dim run
// of a button. On Surface0 a 256-colour terminal drew it in the palette entry
// next to its background.
func TestTheKeyOfAHoveredButtonIsReadableOnEveryTerminal(t *testing.T) {
	// entry is the palette entry or the colour of the one sequence a style
	// opens with, without the digits that say foreground or background. A
	// style that opens with no sequence has no entry.
	entry := func(style lipgloss.Style) string {
		on, _, _ := strings.Cut(style.Render("x"), "x")
		on = strings.TrimSuffix(strings.TrimPrefix(on, "\x1b["), "m")
		if rest, found := strings.CutPrefix(on, "38;"); found {
			return rest
		}
		if rest, found := strings.CutPrefix(on, "48;"); found {
			return rest
		}
		if on == "" {
			return ""
		}
		// The 16 colours: 30-37 and 90-97 in front, 40-47 and 100-107 behind.
		code, err := strconv.Atoi(on)
		if err != nil {
			t.Fatalf("no colour in %q", on)
		}
		if code >= 40 && code < 50 || code >= 100 {
			code -= 10
		}
		return strconv.Itoa(code)
	}

	applyOnEveryTerminal(t, func(name, theme string, _ termenv.Profile) {
		key := entry(lipgloss.NewStyle().Foreground(ShellFooterHelpColor))
		under := entry(lipgloss.NewStyle().Background(ShellHoverBgColor))
		if key == "" || key == under {
			t.Errorf("%s, %s: the key of a hovered button is drawn in %q on %q", name, theme, key, under)
		}
	})
}

// TestApplyOnEveryTerminalLeavesTheRolesOfTheStartProfile: the helper ends on
// a palette profile two times in three, and a later test reads the hover roles
// without applying a theme.
func TestApplyOnEveryTerminalLeavesTheRolesOfTheStartProfile(t *testing.T) {
	forceTrueColor(t)
	restoreInitialStyles(t)
	if err := Apply(initialTheme, initialGlyphs); err != nil {
		t.Fatal(err)
	}
	before := RowHoverBgColor

	for range 6 {
		t.Run("", func(t *testing.T) {
			applyOnEveryTerminal(t, func(string, string, termenv.Profile) {})
		})
		if RowHoverBgColor != before || ShellHoverBgColor != before {
			t.Fatalf("the hover is %q on a row and %q on the header after the helper, want %q as before it", RowHoverBgColor, ShellHoverBgColor, before)
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
