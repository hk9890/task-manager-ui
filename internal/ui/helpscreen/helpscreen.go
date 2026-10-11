// Package helpscreen renders the help screen: every key the app answers to,
// grouped by what the keys act on.
//
// Like the store picker it renders instead of the shell rather than inside
// it, in the chrome the configuration screen has (styles.Screen).
package helpscreen

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/scroll"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	title    = "Keyboard Help"
	subtitle = "every key taskmgr-ui answers to"

	// keyIndent is the cells in front of a key, and keyGap the cells between
	// the key column and the descriptions.
	keyIndent = "   "
	keyGap    = "  "
	// wrapMinWidth is the narrowest description column a description is
	// wrapped in. A narrower one draws a word or two a line, so the entry
	// stays one line and is cut at the screen's edge.
	wrapMinWidth = 16
	// clipMinRows is the fewest body rows that give two to the indicators: a
	// shorter body would have no row left for a line of the sections.
	clipMinRows = 3
)

// Entry is a key as the legend spells it, and what it does: one line of the
// screen, and more where the description is wrapped.
type Entry struct {
	Key  string
	Desc string
}

// Section is the keys that act on one thing.
type Section struct {
	Title   string
	Entries []Entry
}

// State is the full help-screen renderer input.
type State struct {
	Sections []Section
	// Offset is the first line of the sections in the body. Render draws from
	// MaxOffset when it is past it.
	Offset  int
	Version string
	// Legend is the key legend on the last line. It arrives styled, from
	// styles.KeyLegend at styles.ScreenTextWidth.
	Legend string
	Width  int
	Height int
}

// Render draws the help screen.
func Render(state State) string {
	lines := sectionLines(state.Sections, state.Width)
	rows := styles.ScreenBodyRows(state.Height)
	return styles.Screen(styles.ScreenConfig{
		Title:    title,
		Version:  state.Version,
		Subtitle: subtitle,
		Body:     window(lines, textutil.Clamp(state.Offset, 0, max(len(lines)-rows, 0)), rows),
		Legend:   state.Legend,
		Width:    state.Width,
		Height:   state.Height,
	})
}

// MaxOffset is the offset that draws the last line of sections on the last
// body row of a screen of width and height: the furthest the screen scrolls.
// A description is wrapped to the width, so the width decides how many lines
// there are.
func MaxOffset(sections []Section, width, height int) int {
	return max(len(sectionLines(sections, width))-styles.ScreenBodyRows(height), 0)
}

// PageRows is how far a page key scrolls a screen of height: the body rows
// less the two an indicator can stand on, so a page shows no line twice and
// skips none.
func PageRows(height int) int {
	return max(styles.ScreenBodyRows(height)-2, 1)
}

// window is the rows of lines a body of that many rows draws from offset. A
// body that does not start at the first line gives its first row to the
// indicator of the lines above, and one that does not end at the last line its
// last row to the indicator of the lines below. Each counts the lines beyond
// the window, as the indicators of a detail pane do, and not the line it
// stands on. Every offset from 0 to MaxOffset draws another line, and each
// line is drawn at one of them.
func window(lines []string, offset, rows int) []string {
	end := min(offset+rows, len(lines))
	shown := slices.Clone(lines[offset:end])
	if rows < clipMinRows {
		return shown
	}
	if offset > 0 {
		shown[0] = " " + scroll.Earlier(offset)
	}
	if end < len(lines) {
		shown[len(shown)-1] = " " + scroll.More(len(lines)-end)
	}
	return shown
}

// sectionLines draws every section: a blank line between two, the heading
// with its rule, then the lines of each key. One key column is shared by every
// section, so the descriptions line up down the whole screen and it reads as
// one table. A description wider than its column goes on at the column's first
// cell on the next line, broken at a space, and stops a cell short of the edge
// as the rule of a heading does.
func sectionLines(sections []Section, width int) []string {
	keyWidth := 0
	for _, section := range sections {
		for _, entry := range section.Entries {
			keyWidth = max(keyWidth, lipgloss.Width(entry.Key))
		}
	}
	key := lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor)
	desc := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)
	descColumn := lipgloss.Width(keyIndent) + keyWidth + lipgloss.Width(keyGap)
	descWidth := width - descColumn - 1

	var lines []string
	for idx, section := range sections {
		if idx > 0 {
			lines = append(lines, "")
		}
		// The heading starts under the screen's title and its rule stops a
		// cell short of the edge.
		lines = append(lines, " "+styles.SectionHeading(section.Title, width-2))
		for _, entry := range section.Entries {
			wrapped := []string{entry.Desc}
			if descWidth >= wrapMinWidth {
				wrapped = textutil.WrapAtSpaces(entry.Desc, descWidth)
			}
			lines = append(lines, keyIndent+key.Render(textutil.PadToWidth(entry.Key, keyWidth))+keyGap+desc.Render(wrapped[0]))
			for _, more := range wrapped[1:] {
				lines = append(lines, strings.Repeat(" ", descColumn)+desc.Render(more))
			}
		}
	}
	return lines
}
