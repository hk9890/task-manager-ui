// Package configscreen renders the configuration screen: the settings the
// operator changes from inside the app, each written to the config file as it
// changes.
//
// Like the store picker it renders instead of the shell rather than inside
// it, so it draws its own help line where the shell footer would be.
package configscreen

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	// helpLines is the row the help hint occupies below the section box.
	helpLines = 1

	// labelWidth is the column the values start at, after the selection
	// gutter.
	labelWidth = 10

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
	// Help is the key legend drawn below the box, in place of the shell
	// footer. It arrives styled (styles.KeyLegend).
	Help   string
	Width  int
	Height int
}

// Render draws the configuration screen.
func Render(state State) string {
	innerWidth := max(state.Width-2, 1)
	_, gutter := styles.SelectionPrefix(false, false)
	textWidth := max(innerWidth-lipgloss.Width(gutter), 0)

	// A path too long for the line loses its front: the file name says more
	// than the directories above it.
	const before, after = "written to ", " as it changes"
	target := before + textutil.TruncateStringFront(state.Path, max(textWidth-lipgloss.Width(before+after), 1)) + after
	if state.Path == "" {
		target = "no config file: a change is not kept"
	}

	content := []string{
		gutter + lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(textutil.TruncateString(target, textWidth)),
		"",
		gutter + renderHeading(sectionTitle, textWidth-lipgloss.Width(gutter)),
	}
	for idx, row := range state.Rows {
		content = append(content, renderRow(row, idx == state.SelectedRow, innerWidth))
	}

	// A frame too short for all of it loses its first lines, as far as it
	// takes to draw the selected row: FormSection cuts from the end.
	selectedLine := len(content) - len(state.Rows) + state.SelectedRow
	if inner := state.Height - helpLines - 2; inner > 0 && selectedLine >= inner {
		content = content[selectedLine-inner+1:]
	}

	// FormSection does not cut a title: a frame too narrow for it, the corners
	// and the dashes around it goes without.
	title := "Configuration"
	if state.Width < lipgloss.Width(title)+6 {
		title = ""
	}

	box := styles.FormSection(styles.FormSectionConfig{
		Content:            content,
		Width:              state.Width,
		Height:             state.Height - helpLines,
		TopLeft:            title,
		Focused:            true,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	return lipgloss.JoinVertical(lipgloss.Left, box, textutil.TruncateString(state.Help, state.Width))
}

// renderHeading draws a section title with a rule to the edge, so a section
// reads as a block and not as one more line.
func renderHeading(title string, width int) string {
	title = textutil.TruncateString(title, width)
	heading := lipgloss.NewStyle().Foreground(styles.SectionHeadingColor).Bold(true).Render(title)
	return heading + " " + styles.Rule(width-lipgloss.Width(title)-1)
}

// renderRow draws one setting: the selection gutter, the label padded to the
// value column, and the value between the two step markers, which say that
// the row is stepped rather than typed.
func renderRow(row Row, selected bool, innerWidth int) string {
	_, prefix := styles.SelectionPrefix(selected, true)
	label := lipgloss.NewStyle().Foreground(styles.SettingLabelColor)
	value := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)
	marker := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	line := prefix +
		label.Render(textutil.PadToWidth(row.Label, labelWidth)) +
		marker.Render(styles.Glyphs.StepPrev) + " " + value.Render(row.Value) + " " + marker.Render(styles.Glyphs.StepNext)
	return styles.RowHighlight(textutil.TruncateString(line, innerWidth), innerWidth, selected, false)
}
