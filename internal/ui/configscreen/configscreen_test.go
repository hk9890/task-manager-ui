package configscreen

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const sampleHelp = "down/up move • left/right change • esc back • ctrl+c quit"

func sampleState(width, height int) State {
	return State{
		Path: "/home/hans/.config/taskmgr-ui/config.yaml",
		Rows: []Row{
			{Label: "theme", Value: "catppuccin-mocha"},
			{Label: "glyphs", Value: "unicode"},
		},
		Version: "v1.2.3",
		Help:    sampleHelp,
		Width:   width,
		Height:  height,
	}
}

func TestRenderGoldens(t *testing.T) {
	t.Parallel()

	t.Run("theme_selected_w100", func(t *testing.T) {
		testui.AssertMatchesGoldenNormalized(t, []byte(Render(sampleState(100, 12))), "config_screen_theme_w100.golden")
	})

	t.Run("glyphs_selected_w60", func(t *testing.T) {
		state := sampleState(60, 12)
		state.SelectedRow = 1

		testui.AssertMatchesGoldenNormalized(t, []byte(Render(state)), "config_screen_glyphs_w60.golden")
	})

	// At this width the path line, the theme row and the legend are each
	// longer than their line and are cut.
	t.Run("narrow_w24", func(t *testing.T) {
		testui.AssertMatchesGoldenNormalized(t, []byte(Render(sampleState(24, 12))), "config_screen_narrow_w24.golden")
	})

	// With no config file the screen says that a change is not kept, rather
	// than naming an empty path.
	t.Run("no_config_file_w100", func(t *testing.T) {
		state := sampleState(100, 12)
		state.Path = ""

		testui.AssertMatchesGoldenNormalized(t, []byte(Render(state)), "config_screen_no_file_w100.golden")
	})
}

// The cursor and the step markers come from the applied glyph set. Not
// parallel: it applies another set.
func TestRenderDrawsTheAppliedGlyphSet(t *testing.T) {
	t.Cleanup(func() {
		if err := styles.Apply("catppuccin-mocha", "unicode"); err != nil {
			t.Fatalf("restore the initial styles: %v", err)
		}
	})

	unicode := Render(sampleState(100, 12))

	if err := styles.Apply("catppuccin-mocha", "ascii"); err != nil {
		t.Fatalf("Apply(ascii): %v", err)
	}
	testui.AssertMatchesGoldenNormalized(t, []byte(Render(sampleState(100, 12))), "config_screen_ascii_w100.golden")

	// The nerd set draws this screen with the markers of the unicode set, so
	// it has no golden of its own.
	if err := styles.Apply("catppuccin-mocha", "nerd"); err != nil {
		t.Fatalf("Apply(nerd): %v", err)
	}
	if nerd := Render(sampleState(100, 12)); nerd != unicode {
		t.Errorf("the nerd set drew another screen than the unicode set:\n%s\n--- unicode ---\n%s", nerd, unicode)
	}
}

// The screen is the terminal's size at any size: a wider line wraps, and the
// shell cuts a taller frame.
func TestRenderFitsTheTerminal(t *testing.T) {
	t.Parallel()

	for _, width := range []int{6, 12, 24, 60, 100, 200} {
		for _, height := range []int{5, 6, 12, 40} {
			lines := strings.Split(Render(sampleState(width, height)), "\n")
			if len(lines) != height {
				t.Errorf("%dx%d: drew %d lines", width, height, len(lines))
			}
			for _, line := range lines {
				if got := lipgloss.Width(line); got > width {
					t.Errorf("%dx%d: a line is %d cells wide: %q", width, height, got, line)
				}
			}
		}
	}
}

// A screen too short for the whole section still draws the selected row: a
// step changes a value the operator must see.
func TestRenderKeepsTheSelectedRowInAShortFrame(t *testing.T) {
	t.Parallel()

	for height := 6; height <= 10; height++ {
		for selected, label := range []string{"theme", "glyphs"} {
			state := sampleState(60, height)
			state.SelectedRow = selected
			view := Render(state)

			if !strings.Contains(view, label) {
				t.Errorf("height %d: the selected row %q is not drawn:\n%s", height, label, view)
			}
			if got := strings.Count(view, "\n") + 1; got != height {
				t.Errorf("height %d, row %q: drew %d lines", height, label, got)
			}
		}
	}
}

// The selected row carries the selection band across the whole row, and the
// other row none. Not parallel: the colour profile is process-wide.
func TestSelectedRowCarriesTheBand(t *testing.T) {
	testui.ForceTrueColor(t)

	state := sampleState(60, 12)
	state.SelectedRow = 1
	view := Render(state)

	if band := testui.RowBand(t, view, "glyphs"); band == "" {
		t.Errorf("the selected row has no band:\n%s", view)
	}
	if band := testui.RowBand(t, view, "theme"); band != "" {
		t.Errorf("the row that is not selected carries the band %q:\n%s", band, view)
	}
}

// The screen stands in the chrome the help screen has: the title with the
// version at the right edge, the rule one cell in, the file line as the
// subtitle, a rule from edge to edge, and the legend on the last line, one
// cell in. The section heading runs its rule to the edge.
func TestRenderDrawsTheScreenChrome(t *testing.T) {
	t.Parallel()

	lines := strings.Split(testui.AnsiEscapePattern.ReplaceAllString(Render(sampleState(80, 12)), ""), "\n")

	want := []string{
		" Configuration" + strings.Repeat(" ", 80-len(" Configuration")-len("v1.2.3")) + "v1.2.3",
		" " + strings.Repeat("─", 79),
		" written to /home/hans/.config/taskmgr-ui/config.yaml as it changes",
		strings.Repeat("─", 80),
		"",
		"Appearance " + strings.Repeat("─", 80-len("Appearance ")),
	}
	for row, line := range want {
		if strings.TrimRight(lines[row], " ") != line {
			t.Errorf("row %d:\n got %q\nwant %q", row, lines[row], line)
		}
	}
	if want := " " + sampleHelp; lines[len(lines)-1] != want {
		t.Errorf("last line %q, want the legend one cell in", lines[len(lines)-1])
	}
}

// A value starts labelWidth cells after the selection gutter, on every row.
func TestRenderStartsEveryValueInOneColumn(t *testing.T) {
	t.Parallel()

	view := Render(sampleState(80, 12))
	for _, value := range []string{"‹ catppuccin-mocha ›", "‹ unicode ›"} {
		if x, _ := testui.FindCell(t, view, value); x != 2+14 {
			t.Errorf("%q starts on column %d, want %d", value, x, 2+14)
		}
	}
}

var sgrPattern = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// sgrBefore is the SGR parameters in force where text first starts on the
// line of view it is drawn on.
func sgrBefore(t *testing.T, view, text string) string {
	t.Helper()

	_, y := testui.FindCell(t, view, text)
	before, _, _ := strings.Cut(strings.Split(view, "\n")[y], text)
	codes := sgrPattern.FindAllStringSubmatch(before, -1)
	if len(codes) == 0 {
		return ""
	}
	return codes[len(codes)-1][1]
}

// foreground is the SGR parameter Lip Gloss writes for a colour role.
func foreground(role lipgloss.Color) string {
	return sgrPattern.FindStringSubmatch(lipgloss.NewStyle().Foreground(role).Render("x"))[1]
}

// The colours are the roles': a text-only check passes on a screen drawn in
// one colour. Not parallel: the colour profile is process-wide.
func TestRenderColoursTheChromeAndTheHeading(t *testing.T) {
	testui.ForceTrueColor(t)

	view := Render(sampleState(100, 12))
	for text, want := range map[string]string{
		"Configuration": "1;" + foreground(styles.TextPrimaryColor),
		"v1.2.3":        foreground(styles.ShellFooterHelpColor),
		"written to":    foreground(styles.ScreenSubtitleColor),
		// The step markers are part of the value: one run in its colour.
		"‹ unicode ›": foreground(styles.TextPrimaryColor),
		"Appearance":  "1;" + foreground(styles.SectionHeadingColor),
	} {
		if got := sgrBefore(t, view, text); got != want {
			t.Errorf("%q is drawn with %q, want %q", text, got, want)
		}
	}

	if foreground(styles.ScreenSubtitleColor) == foreground(styles.TextMutedColor) {
		t.Fatal("fixture: the subtitle role has the muted colour")
	}

	_, y := testui.FindCell(t, view, "Appearance")
	_, rule, _ := strings.Cut(strings.Split(view, "\n")[y], "Appearance")
	if codes := sgrPattern.FindAllStringSubmatch(rule, -1); len(codes) < 2 || codes[1][1] != foreground(styles.ShellRuleColor) {
		t.Errorf("the rule after the heading is not in the rule colour %q: %q", foreground(styles.ShellRuleColor), rule)
	}
}
