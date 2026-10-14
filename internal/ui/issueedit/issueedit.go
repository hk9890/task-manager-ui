package issueedit

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	// labelWidth is the column the values start at, after the selection
	// gutter.
	labelWidth = 10

	gutterWidth = 2

	// minTextRows is the least the text area keeps on a short screen.
	minTextRows = 3

	// maxErrorLines is the most lines the refusal of a save takes. A short
	// screen gives it fewer (errorLines).
	maxErrorLines = 4

	buttonGap = 2
)

// Field is one stop of the focus: a row, the text or a button.
type Field int

const (
	FieldTitle Field = iota
	FieldStatus
	FieldReason
	FieldType
	FieldPriority
	FieldAssignee
	FieldLabels
	FieldText
	FieldSave
	FieldCancel
)

// Row is one one-line field.
type Row struct {
	Field Field
	Label string
	// Value is the value as drawn: a text input with its cursor, or the
	// stepped value.
	Value string
	// Stepped draws the value between Glyphs.StepPrev and Glyphs.StepNext.
	Stepped bool
	// ReadOnly draws the value muted and takes no focus: the reason of an
	// issue that was closed before.
	ReadOnly bool
}

// State is the full issue-editor renderer input.
type State struct {
	Title string
	// Subtitle names the issue the screen writes to.
	Subtitle string
	// Rows are the one-line fields above the text; a comment has none.
	Rows      []Row
	TextLabel string
	// Text is the text area as drawn, TextRows(state) lines of it.
	Text  []string
	Focus Field
	// Error is the refusal of the last save, "" for none.
	Error     string
	Saving    bool
	SaveKey   string
	CancelKey string
	Version   string
	// Help is the key legend on the last line, in place of the shell footer.
	// It arrives styled, from styles.KeyLegend at styles.ScreenTextWidth.
	Help   string
	Width  int
	Height int
}

// TextWidth is the cells a line of the text area has on a screen of width.
func TextWidth(width int) int {
	return max(width-gutterWidth-1, 1)
}

// InputWidth is the cells a one-line input has for its text. The widget draws
// one cell more, for the cursor after the last character.
func InputWidth(width int) int {
	return max(width-gutterWidth-labelWidth-1, 1)
}

// TextRows is the rows of the text area: what the body has left under the
// error and the other form lines, and never fewer than minTextRows.
func TextRows(state State) int {
	return max(styles.ScreenBodyRows(state.Height)-len(errorLines(state))-fixedFormLines(state), minTextRows)
}

// fixedFormLines is the form lines that are not the text: the blank line
// above the rows, the rows and the blank line under them, the heading of the
// text, the blank line under it and the buttons.
func fixedFormLines(state State) int {
	if len(state.Rows) == 0 {
		return 4
	}
	return len(state.Rows) + 5
}

// errorLines is the refusal of the last save, wrapped, one cell in. It takes
// maxErrorLines lines at most, and on a short screen only what leaves the
// form a heading and minTextRows lines, or one line, so the focused field is
// still drawn. A longer text ends in the cut mark.
func errorLines(state State) []string {
	body := styles.ScreenBodyRows(state.Height)
	limit := min(maxErrorLines, max(body-minTextRows-1, 1), body-1)
	if state.Error == "" || limit <= 0 {
		return nil
	}
	width := max(state.Width-2, 1)
	lines := textutil.WrapAtSpaces(displaytext.Lines(state.Error), width)
	if len(lines) > limit {
		lines = lines[:limit]
		lines[limit-1] = ansi.Truncate(lines[limit-1], width-1, "") + "…"
	}
	style := lipgloss.NewStyle().Foreground(styles.ErrorTextColor)
	for idx, line := range lines {
		lines[idx] = " " + style.Render(line)
	}
	return lines
}

// noField is the field of a form line a click does nothing on.
const noField Field = -1

// form is the lines of the form and the field each one belongs to. Render and
// HitTest both read it, so a click lands on the field drawn under it. The
// button line belongs to FieldSave; buttonAt tells the two buttons apart.
func form(state State) (lines []string, fields []Field) {
	add := func(line string, field Field) {
		lines = append(lines, line)
		fields = append(fields, field)
	}

	add("", noField)
	for _, row := range state.Rows {
		field := row.Field
		if row.ReadOnly {
			field = noField
		}
		add(renderRow(row, state.Focus == row.Field, state.Width), field)
	}
	if len(state.Rows) > 0 {
		add("", noField)
	}

	_, idle := styles.SelectionPrefix(false, true)
	add(idle+styles.SectionHeading(state.TextLabel, state.Width-gutterWidth), FieldText)
	_, gutter := styles.SelectionPrefix(state.Focus == FieldText, true)
	for _, line := range state.Text {
		add(gutter+line, FieldText)
	}

	add("", noField)
	add(renderButtons(state), FieldSave)
	return lines, fields
}

// window is the first form line drawn when rows of the body are left for the
// form: as far down as it takes to draw the last line of the focused field.
func window(state State, fields []Field, rows int) int {
	if rows >= len(fields) {
		return 0
	}
	focus := state.Focus
	if focus == FieldCancel {
		focus = FieldSave
	}
	last := 0
	for idx, field := range fields {
		if field == focus {
			last = idx
		}
	}
	return max(last-max(rows, 0)+1, 0)
}

// Render draws the issue editor.
func Render(state State) string {
	body := errorLines(state)
	lines, fields := form(state)
	body = append(body, lines[window(state, fields, styles.ScreenBodyRows(state.Height)-len(body)):]...)

	return styles.Screen(styles.ScreenConfig{
		Title:    displaytext.OneLine(state.Title),
		Version:  state.Version,
		Subtitle: displaytext.OneLine(state.Subtitle),
		Body:     body,
		Legend:   state.Help,
		Width:    state.Width,
		Height:   state.Height,
	})
}

// renderRow draws one field: the selection gutter, the label padded to the
// value column, and the value. A stepped value stands between the two step
// markers, which say that the row is stepped rather than typed. A value the
// screen draws itself is cut to the cells a text input has; an input cuts its
// own.
func renderRow(row Row, focused bool, width int) string {
	cells := InputWidth(width) + 1
	_, prefix := styles.SelectionPrefix(focused, true)
	label := lipgloss.NewStyle().Foreground(styles.SettingLabelColor).Render(textutil.PadToWidth(row.Label, labelWidth))

	value := row.Value
	switch {
	case row.ReadOnly:
		value = lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(textutil.TruncateString(displaytext.OneLine(value), cells))
	case row.Stepped:
		markers := lipgloss.Width(styles.Glyphs.StepPrev + "  " + styles.Glyphs.StepNext)
		value = lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).
			Render(styles.Glyphs.StepPrev + " " + textutil.TruncateString(displaytext.OneLine(value), cells-markers) + " " + styles.Glyphs.StepNext)
	}
	return prefix + label + value
}

// buttons is the two buttons as drawn, each with the key that does the same.
func buttons(state State) (save, cancel string) {
	saveStyle, cancelStyle := styles.PrimaryButtonStyle, styles.SecondaryButtonStyle
	switch state.Focus {
	case FieldSave:
		saveStyle = styles.PrimaryButtonFocusedStyle
	case FieldCancel:
		cancelStyle = styles.SecondaryButtonFocusedStyle
	}
	return saveStyle.Render("Save " + state.SaveKey), cancelStyle.Render("Cancel " + state.CancelKey)
}

func renderButtons(state State) string {
	indent := strings.Repeat(" ", gutterWidth)
	if state.Saving {
		return indent + lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Saving…")
	}
	save, cancel := buttons(state)
	return indent + save + strings.Repeat(" ", buttonGap) + cancel
}
