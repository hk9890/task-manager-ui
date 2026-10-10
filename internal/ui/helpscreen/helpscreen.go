// Package helpscreen renders the help screen: every key the app answers to,
// grouped by what the keys act on.
//
// Like the store picker it renders instead of the shell rather than inside
// it, in the chrome the configuration screen has (styles.Screen).
package helpscreen

import (
	"github.com/charmbracelet/lipgloss"

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
)

// Entry is one line of the screen: a key as the legend spells it, and what it
// does.
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
	// Offset is the first line of the sections that is drawn. Render draws
	// from MaxOffset when it is past it.
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
	return styles.Screen(styles.ScreenConfig{
		Title:    title,
		Version:  state.Version,
		Subtitle: subtitle,
		Body:     lines[textutil.Clamp(state.Offset, 0, maxOffset(len(lines), state.Height)):],
		Legend:   state.Legend,
		Width:    state.Width,
		Height:   state.Height,
	})
}

// MaxOffset is the offset that draws the last line of sections on the last
// body row of a screen of height: the furthest the screen scrolls.
func MaxOffset(sections []Section, height int) int {
	return maxOffset(len(sectionLines(sections, 0)), height)
}

func maxOffset(lines, height int) int {
	return max(lines-styles.ScreenBodyRows(height), 0)
}

// sectionLines draws every section: a blank line between two, the heading
// with its rule, then a line for each key. One key column is shared by every
// section, so the descriptions line up down the whole screen and it reads as
// one table.
func sectionLines(sections []Section, width int) []string {
	keyWidth := 0
	for _, section := range sections {
		for _, entry := range section.Entries {
			keyWidth = max(keyWidth, lipgloss.Width(entry.Key))
		}
	}
	key := lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor)
	desc := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)

	var lines []string
	for idx, section := range sections {
		if idx > 0 {
			lines = append(lines, "")
		}
		// The heading starts under the screen's title and its rule stops a
		// cell short of the edge.
		lines = append(lines, " "+styles.SectionHeading(section.Title, width-2))
		for _, entry := range section.Entries {
			lines = append(lines, keyIndent+key.Render(textutil.PadToWidth(entry.Key, keyWidth))+keyGap+desc.Render(entry.Desc))
		}
	}
	return lines
}
