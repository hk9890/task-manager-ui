// Package configscreen renders the configuration screen: the settings the
// operator changes from inside the app, each written to the config file as it
// changes.
//
// Like the store picker it renders instead of the shell rather than inside
// it, in the chrome the help screen has (styles.Screen).
package configscreen

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	title = "Configuration"

	// labelWidth is the column the values start at, after the selection
	// gutter.
	labelWidth = 14

	sectionTitle = "Appearance"
)

// Row is one setting: its name and the value it has now.
type Row struct {
	Label string
	Value string
}

// State is the full configuration-screen renderer input.
type State struct {
	// Path is the config file a change is written to. Empty says there is
	// none, and the screen then says that a change cannot be kept.
	Path string
	// Rows are the settings of the Appearance section, in the order drawn.
	Rows        []Row
	SelectedRow int
	Version     string
	// Help is the key legend on the last line, in place of the shell footer.
	// It arrives styled, from styles.KeyLegend at styles.ScreenTextWidth.
	Help   string
	Width  int
	Height int
}

// Render draws the configuration screen.
func Render(state State) string {
	// The subtitle names the file a change is written to. A path too long for
	// the line loses its front: the file name says more than the directories
	// above it.
	const before, after = "written to ", " as it changes"
	target := before + textutil.TruncateStringFront(state.Path, max(styles.ScreenTextWidth(state.Width)-lipgloss.Width(before+after), 1)) + after
	if state.Path == "" {
		target = "no config file: a change is not kept"
	}

	body := []string{"", styles.SectionHeading(sectionTitle, state.Width)}
	for idx, row := range state.Rows {
		body = append(body, renderRow(row, idx == state.SelectedRow, state.Width))
	}

	// A screen too short for all of it loses its first lines, as far as it
	// takes to draw the selected row: styles.Screen cuts from the end.
	selectedLine := len(body) - len(state.Rows) + state.SelectedRow
	if rows := styles.ScreenBodyRows(state.Height); rows > 0 && selectedLine >= rows {
		body = body[selectedLine-rows+1:]
	}

	return styles.Screen(styles.ScreenConfig{
		Title:    title,
		Version:  state.Version,
		Subtitle: target,
		Body:     body,
		Legend:   state.Help,
		Width:    state.Width,
		Height:   state.Height,
	})
}

// renderRow draws one setting: the selection gutter, the label padded to the
// value column, and the value between the two step markers, which say that
// the row is stepped rather than typed and are drawn as the value is.
func renderRow(row Row, selected bool, width int) string {
	_, prefix := styles.SelectionPrefix(selected, true)
	label := lipgloss.NewStyle().Foreground(styles.SettingLabelColor)
	value := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)

	line := prefix +
		label.Render(textutil.PadToWidth(row.Label, labelWidth)) +
		value.Render(styles.Glyphs.StepPrev+" "+row.Value+" "+styles.Glyphs.StepNext)
	return styles.RowHighlight(textutil.TruncateString(line, width), width, selected, false)
}
