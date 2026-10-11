package helpscreen

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const sampleLegend = "down/up scroll • esc back • pgup/pgdown page • home/end bounds"

// sampleSections has its widest key in the second section, so a key column
// measured per section would put the descriptions out of line.
func sampleSections() []Section {
	return []Section{
		{Title: "Moving and opening", Entries: []Entry{
			{Key: "up / down", Desc: "the row above / below"},
			{Key: "enter", Desc: "open the selected issue in Detail"},
			{Key: "esc", Desc: "back to where the surface was opened from; hide a toast"},
		}},
		{Title: "Tabs", Entries: []Entry{
			{Key: "tab/ctrl+pgdown", Desc: "next tab: Board, Docs"},
			{Key: "shift+tab/ctrl+pgup", Desc: "previous tab"},
		}},
		{Title: "Mouse", Entries: []Entry{
			{Key: "click", Desc: "select a row, switch to a tab, press a menu-bar button or focus a pane"},
			{Key: "drag", Desc: "select a box of text; letting go sends it to the terminal clipboard"},
		}},
	}
}

func sampleState(width, height int) State {
	return State{
		Sections: sampleSections(),
		Version:  "v1.2.3",
		Legend:   sampleLegend,
		Width:    width,
		Height:   height,
	}
}

func plainLines(view string) []string {
	return strings.Split(textutil.StripANSI(view), "\n")
}

func TestRenderGoldens(t *testing.T) {
	t.Parallel()

	t.Run("wide_w200", func(t *testing.T) {
		testui.AssertMatchesGoldenNormalized(t, []byte(Render(sampleState(200, 16))), "help_screen_w200.golden")
	})

	t.Run("normal_w120", func(t *testing.T) {
		testui.AssertMatchesGoldenNormalized(t, []byte(Render(sampleState(120, 16))), "help_screen_w120.golden")
	})

	// At this width the long descriptions are wrapped and the legend is cut
	// with a mark. The wrapped lines are more than the body holds, so its last
	// row counts the ones below.
	t.Run("narrow_w60", func(t *testing.T) {
		testui.AssertMatchesGoldenNormalized(t, []byte(Render(sampleState(60, 16))), "help_screen_narrow_w60.golden")
	})

	// A screen too short for the sections draws them from the offset, between
	// the chrome that stays, with an indicator on its first and its last row.
	t.Run("scrolled_w120", func(t *testing.T) {
		state := sampleState(120, 8)
		state.Offset = 4

		testui.AssertMatchesGoldenNormalized(t, []byte(Render(state)), "help_screen_scrolled_w120.golden")
	})
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

// The first line is the title with the version at the right edge. The rule
// under it, the subtitle and the legend on the last line start under the
// title, one cell in; the rule under the subtitle runs from edge to edge.
func TestRenderDrawsTheScreenChrome(t *testing.T) {
	t.Parallel()

	lines := plainLines(Render(sampleState(80, 20)))

	want := []string{
		" Keyboard Help" + strings.Repeat(" ", 80-len(" Keyboard Help")-len("v1.2.3")) + "v1.2.3",
		" " + strings.Repeat("─", 79),
		" every key taskmgr-ui answers to",
		strings.Repeat("─", 80),
	}
	for row, line := range want {
		if lines[row] != line {
			t.Errorf("row %d:\n got %q\nwant %q", row, lines[row], line)
		}
	}
	if want := " " + sampleLegend; lines[len(lines)-1] != want {
		t.Errorf("last line %q, want the legend one cell in", lines[len(lines)-1])
	}
}

// A description wider than its column goes on in that column on the next
// lines. It is broken at spaces alone, so no word is cut and a compound word
// stays whole, and no line reaches the screen's last cell.
func TestRenderWrapsALongDescriptionInItsColumn(t *testing.T) {
	t.Parallel()

	const descColumn = 3 + len("shift+tab/ctrl+pgup") + 2
	for width := descColumn + 1 + wrapMinWidth; width <= 100; width++ {
		lines := plainLines(Render(sampleState(width, 60)))
		body := lines[4 : len(lines)-1]

		wrapped := 0
		for _, section := range sampleSections() {
			for _, entry := range section.Entries {
				first := slices.IndexFunc(body, func(line string) bool {
					return strings.HasPrefix(line, "   "+textutil.PadToWidth(entry.Key, len("shift+tab/ctrl+pgup"))+"  ")
				})
				if first < 0 {
					t.Fatalf("width %d: no line opens with the key %q", width, entry.Key)
				}
				// The entry's lines end at the next key, heading or blank line.
				parts := []string{body[first][descColumn:]}
				for _, line := range body[first+1:] {
					if !strings.HasPrefix(line, strings.Repeat(" ", descColumn)) {
						break
					}
					parts = append(parts, line[descColumn:])
				}
				if got := strings.Join(parts, " "); got != entry.Desc {
					t.Errorf("width %d: the lines of %q read %q, want %q", width, entry.Key, got, entry.Desc)
				}
				for _, part := range parts {
					if part != strings.TrimSpace(part) || strings.HasSuffix(part, "-") {
						t.Errorf("width %d: a line of %q is %q, not broken at a space", width, entry.Key, part)
					}
					if got := descColumn + lipgloss.Width(part); got > width-1 {
						t.Errorf("width %d: a line of %q ends at column %d, on or past the last cell", width, entry.Key, got)
					}
				}
				wrapped += len(parts) - 1
			}
		}
		if width == descColumn+1+wrapMinWidth && wrapped == 0 {
			t.Fatal("fixture: no description is wrapped at the narrowest width that wraps")
		}
	}
}

// A screen too narrow for the key column and a few words of description keeps
// an entry on one line, which ends at the screen's last cell with no mark in
// it.
func TestRenderCutsALongLineWithNoMarkOnAScreenTooNarrowToWrap(t *testing.T) {
	t.Parallel()

	const width = 3 + len("shift+tab/ctrl+pgup") + 2 + wrapMinWidth
	lines := plainLines(Render(sampleState(width, 20)))
	full := plainLines(Render(sampleState(200, 20)))

	cut := 0
	for row := 4; row < len(lines)-1; row++ {
		// A heading is drawn to the width, so only an entry is cut.
		if lipgloss.Width(full[row]) <= width || strings.Contains(full[row], "─") {
			continue
		}
		cut++
		if want := string([]rune(full[row])[:width]); lines[row] != want {
			t.Errorf("row %d:\n got %q\nwant %q", row, lines[row], want)
		}
	}
	if cut == 0 {
		t.Fatal("fixture: no entry is wider than the screen")
	}
}

// A body that does not hold every line says so on its first and its last row,
// in the words a clipped detail pane uses and with its count: the lines beyond
// the window, without the one the indicator is drawn on. Every offset to MaxOffset
// draws another line, and no line is out of reach, wrapped ones included.
func TestRenderMarksAClippedBodyAndReachesEveryLine(t *testing.T) {
	t.Parallel()

	indicator := regexp.MustCompile(`^ … \((\d+) (earlier|more)\)$`)
	for _, size := range [][2]int{{120, 8}, {120, 10}, {60, 8}, {60, 12}, {44, 9}, {30, 8}} {
		width, height := size[0], size[1]
		state := sampleState(width, height)
		all := plainLines(strings.Join(sectionLines(state.Sections, width), "\n"))
		rows := styles.ScreenBodyRows(height)
		limit := MaxOffset(state.Sections, width, height)
		if limit != len(all)-rows || limit < 2 {
			t.Fatalf("%dx%d: MaxOffset is %d for %d lines on %d rows", width, height, limit, len(all), rows)
		}

		seen := 0
		for state.Offset = 0; state.Offset <= limit; state.Offset++ {
			lines := plainLines(Render(state))
			body := lines[4 : len(lines)-1]

			// earlier and more are the lines not drawn on each side: the ones
			// an indicator counts and the one it stands on.
			earlier, more := 0, 0
			if match := indicator.FindStringSubmatch(body[0]); match != nil && match[2] == "earlier" {
				earlier, _ = strconv.Atoi(match[1])
				if earlier != state.Offset {
					t.Errorf("%dx%d at %d: the first row counts %d earlier, want the lines above the window", width, height, state.Offset, earlier)
				}
				earlier++
				body = body[1:]
			}
			if match := indicator.FindStringSubmatch(body[len(body)-1]); match != nil && match[2] == "more" {
				more, _ = strconv.Atoi(match[1])
				if more != len(all)-rows-state.Offset {
					t.Errorf("%dx%d at %d: the last row counts %d more, want the lines below the window", width, height, state.Offset, more)
				}
				more++
				body = body[:len(body)-1]
			}
			if (earlier > 0) != (state.Offset > 0) || (more > 0) != (state.Offset < limit) {
				t.Fatalf("%dx%d at %d: %d earlier and %d more", width, height, state.Offset, earlier, more)
			}
			if earlier+len(body)+more != len(all) {
				t.Errorf("%dx%d at %d: %d earlier, %d drawn and %d more are not the %d lines", width, height, state.Offset, earlier, len(body), more, len(all))
			}
			for idx, line := range body {
				if want := ansi.Truncate(all[earlier+idx], width, ""); line != want {
					t.Fatalf("%dx%d at %d: row %d is %q, want line %d, %q", width, height, state.Offset, idx, line, earlier+idx, want)
				}
			}
			// A step skips no line and draws one more: the last step draws
			// two, the line its indicator stood on too.
			last := earlier + len(body)
			if earlier > seen || state.Offset > 0 && last <= seen {
				t.Errorf("%dx%d at %d: lines %d to %d are drawn, after the offset before drew to %d", width, height, state.Offset, earlier, last, seen)
			}
			seen = last
		}
		if seen != len(all) {
			t.Errorf("%dx%d: scrolled to line %d of %d", width, height, seen, len(all))
		}
	}
}

// A body of fewer than three rows has no row to spare for an indicator: it
// draws lines alone, so each can still be reached.
func TestRenderDrawsNoIndicatorOnABodyOfTwoRows(t *testing.T) {
	t.Parallel()

	state := sampleState(120, 7)
	state.Offset = 3
	lines := plainLines(Render(state))
	all := plainLines(strings.Join(sectionLines(state.Sections, 120), "\n"))
	if lines[4] != all[3] || lines[5] != all[4] {
		t.Errorf("the body is %q and %q, want lines 3 and 4 of the sections", lines[4], lines[5])
	}
}

// A section opens with its title one cell in and a rule that stops one cell
// short of the edge, and a blank line parts two sections.
func TestRenderDrawsEachSectionUnderAHeadingWithARule(t *testing.T) {
	t.Parallel()

	lines := plainLines(Render(sampleState(80, 20)))

	for row, title := range map[int]string{4: "Moving and opening", 9: "Tabs", 13: "Mouse"} {
		want := " " + title + " " + strings.Repeat("─", 80-lipgloss.Width(title)-3)
		if lines[row] != want {
			t.Errorf("heading on row %d:\n got %q\nwant %q", row, lines[row], want)
		}
		if row > 4 && lines[row-1] != "" {
			t.Errorf("row %d above the heading %q is not blank: %q", row-1, title, lines[row-1])
		}
	}
}

// Every key stands three cells in, in one column that is as wide as the widest
// key of any section, and every description starts two cells after it.
func TestRenderAlignsTheKeysOfAllSectionsInOneColumn(t *testing.T) {
	t.Parallel()

	const keyWidth = len("shift+tab/ctrl+pgup")
	view := Render(sampleState(160, 20))

	for _, section := range sampleSections() {
		for _, entry := range section.Entries {
			x, y := testui.FindCell(t, view, entry.Desc)
			if x != 3+keyWidth+2 {
				t.Errorf("%q starts on column %d, want %d", entry.Desc, x, 3+keyWidth+2)
			}
			if want := "   " + textutil.PadToWidth(entry.Key, keyWidth) + "  " + entry.Desc; plainLines(view)[y] != want {
				t.Errorf("entry line:\n got %q\nwant %q", plainLines(view)[y], want)
			}
		}
	}
}

// An offset past the end draws the last line of the sections on the last body
// row, so a screen that grew or a list that shrank leaves no empty rows.
func TestRenderClampsTheOffset(t *testing.T) {
	t.Parallel()

	state := sampleState(120, 8)
	limit := MaxOffset(state.Sections, state.Width, state.Height)
	if limit != 9 {
		t.Fatalf("fixture: MaxOffset is %d, want 12 lines less 3 body rows", limit)
	}

	state.Offset = limit
	end := Render(state)
	if lines := plainLines(end); !strings.Contains(lines[len(lines)-2], "select a box of text") {
		t.Errorf("the last body row at MaxOffset is not the last entry:\n%s", end)
	}

	state.Offset = limit + 50
	if Render(state) != end {
		t.Error("an offset past MaxOffset drew another screen than MaxOffset")
	}
	state.Offset = -3
	if Render(state) != Render(sampleState(120, 8)) {
		t.Error("an offset below zero drew another screen than the top")
	}
	if got := MaxOffset(state.Sections, state.Width, 40); got != 0 {
		t.Errorf("MaxOffset is %d on a screen that holds every line", got)
	}
}

var sgrPattern = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// lineOf is the line of view that text is drawn on.
func lineOf(t *testing.T, view, text string) string {
	t.Helper()

	_, y := testui.FindCell(t, view, text)
	return strings.Split(view, "\n")[y]
}

// sgrBefore is the SGR parameters in force where text first starts on line.
func sgrBefore(line, text string) string {
	before, _, _ := strings.Cut(line, text)
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
func TestRenderColoursTheTitleTheHeadingsTheRulesAndTheKeys(t *testing.T) {
	testui.ForceTrueColor(t)

	view := Render(sampleState(120, 20))
	rule := foreground(styles.ShellRuleColor)
	muted := foreground(styles.ShellFooterHelpColor)
	heading := foreground(styles.SectionHeadingColor)
	text := foreground(styles.TextPrimaryColor)
	if rule == muted || rule == heading || muted == heading || muted == text {
		t.Fatalf("fixture: the roles share a colour: rule %q, muted %q, heading %q, text %q", rule, muted, heading, text)
	}

	if got := sgrBefore(lineOf(t, view, "Keyboard Help"), "Keyboard Help"); got != "1;"+text {
		t.Errorf("the title is drawn with %q, want bold in the text colour %q", got, text)
	}
	if got := sgrBefore(lineOf(t, view, "v1.2.3"), "v1.2.3"); got != muted {
		t.Errorf("the version is drawn with %q, want the muted colour %q", got, muted)
	}
	for _, row := range []int{1, 3} {
		if got := sgrBefore(strings.Split(view, "\n")[row], "─"); got != rule {
			t.Errorf("the rule on row %d is drawn with %q, want the rule colour %q", row, got, rule)
		}
	}
	if got, want := sgrBefore(lineOf(t, view, subtitle), subtitle), foreground(styles.ScreenSubtitleColor); got != want || want == muted {
		t.Errorf("the subtitle is drawn with %q, want the subtitle colour %q, which is not the muted %q", got, want, muted)
	}

	for _, section := range sampleSections() {
		line := lineOf(t, view, section.Title)
		if got := sgrBefore(line, section.Title); got != "1;"+heading {
			t.Errorf("the heading %q is drawn with %q, want bold in the heading colour %q", section.Title, got, heading)
		}
		if got := sgrBefore(line, "─"); got != rule {
			t.Errorf("the rule after %q is drawn with %q, want the rule colour %q", section.Title, got, rule)
		}

		for _, entry := range section.Entries {
			line := lineOf(t, view, entry.Desc)
			if got := sgrBefore(line, entry.Key); got != muted {
				t.Errorf("the key %q is drawn with %q, want the muted colour %q", entry.Key, got, muted)
			}
			if got := sgrBefore(line, entry.Desc); got != text {
				t.Errorf("the description %q is drawn with %q, want the text colour %q", entry.Desc, got, text)
			}
		}
	}
}

// A wrapped line of a description is in the text colour as its first line is,
// and an indicator carries no colour, as it does in a detail pane. Not
// parallel: the colour profile is process-wide.
func TestRenderColoursAWrappedLineAndLeavesTheIndicatorPlain(t *testing.T) {
	testui.ForceTrueColor(t)

	state := sampleState(60, 9)
	state.Offset = 2
	view := Render(state)
	text := foreground(styles.TextPrimaryColor)

	line := lineOf(t, view, "opened from; hide a toast")
	if !strings.HasPrefix(textutil.StripANSI(line), strings.Repeat(" ", 3+len("shift+tab/ctrl+pgup")+2)+"opened") {
		t.Fatalf("fixture: %q is not a wrapped line of a description", textutil.StripANSI(line))
	}
	if got := sgrBefore(line, "opened from; hide a toast"); got != text {
		t.Errorf("the wrapped line is drawn with %q, want the text colour %q", got, text)
	}
	for _, mark := range []string{"… (2 earlier)", "more)"} {
		if line := lineOf(t, view, mark); strings.Contains(line, "\x1b") {
			t.Errorf("the indicator %q carries a colour: %q", mark, line)
		}
	}
}
