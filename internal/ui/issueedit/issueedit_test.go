package issueedit

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const sampleHelp = "tab/shift+tab field • ctrl+s save • esc cancel • left/right change"

// refusal is an error longer than the four lines the screen gives one.
var refusal = strings.Repeat("the store refused the write because the body has no Context section. ", 6)

func createRows() []Row {
	return []Row{
		{Field: FieldTitle, Label: "Title", Value: "Fix the header"},
		{Field: FieldType, Label: "Type", Value: "task", Stepped: true},
		{Field: FieldPriority, Label: "Priority", Value: "P2", Stepped: true},
		{Field: FieldAssignee, Label: "Assignee", Value: "hans"},
		{Field: FieldLabels, Label: "Labels", Value: "ui, docs"},
	}
}

func editRows(reason *Row) []Row {
	rows := []Row{
		{Field: FieldTitle, Label: "Title", Value: "Fix the header"},
		{Field: FieldStatus, Label: "Status", Value: "open", Stepped: true},
	}
	if reason != nil {
		rows[1].Value = "closed"
		rows = append(rows, *reason)
	}
	return append(rows, createRows()[1:]...)
}

// sampleState is a screen with its text area filled to the rows it has.
func sampleState(title string, rows []Row, width, height int) State {
	state := State{
		Title:     title,
		Subtitle:  "Fix the header",
		Rows:      rows,
		TextLabel: "Description",
		SaveKey:   "ctrl+s",
		CancelKey: "esc",
		Version:   "v1.2.3",
		Help:      sampleHelp,
		Width:     width,
		Height:    height,
	}
	return withText(state)
}

func withText(state State) State {
	state.Text = nil
	for row := range TextRows(state) {
		state.Text = append(state.Text, fmt.Sprintf("text line %d", row+1))
	}
	return state
}

func closedWithError() State {
	state := sampleState("Edit issue tm-1", editRows(&Row{Field: FieldReason, Label: "Reason", Value: "done"}), 80, 24)
	state.Error = refusal
	return withText(state)
}

func commentState() State {
	state := sampleState("New comment on tm-1", nil, 80, 24)
	state.TextLabel = "Comment"
	state.Focus = FieldText
	return withText(state)
}

func plain(view string) string {
	return testui.AnsiEscapePattern.ReplaceAllString(view, "")
}

func TestRenderGoldens(t *testing.T) {
	t.Parallel()

	t.Run("create_w80", func(t *testing.T) {
		state := sampleState("New issue", createRows(), 80, 24)
		state.Subtitle = "a new issue"
		testui.AssertMatchesGoldenNormalized(t, []byte(Render(state)), "issue_edit_create_w80.golden")
	})

	t.Run("edit_w80", func(t *testing.T) {
		state := sampleState("Edit issue tm-1", editRows(nil), 80, 24)
		state.Focus = FieldStatus
		testui.AssertMatchesGoldenNormalized(t, []byte(Render(state)), "issue_edit_edit_w80.golden")
	})

	t.Run("edit_closed_error_w80", func(t *testing.T) {
		state := closedWithError()
		state.Focus = FieldReason
		testui.AssertMatchesGoldenNormalized(t, []byte(Render(state)), "issue_edit_closed_error_w80.golden")
	})

	t.Run("comment_w80", func(t *testing.T) {
		testui.AssertMatchesGoldenNormalized(t, []byte(Render(commentState())), "issue_edit_comment_w80.golden")
	})
}

// At 80x24 nothing scrolls: every field and both buttons are drawn whatever
// has the focus, also with the reason row and an error of four lines.
func TestEveryFieldIsDrawnAt80x24(t *testing.T) {
	t.Parallel()

	states := map[string]State{
		"create":            sampleState("New issue", createRows(), 80, 24),
		"edit":              sampleState("Edit issue tm-1", editRows(nil), 80, 24),
		"closed with error": closedWithError(),
		"comment":           commentState(),
	}
	for name, state := range states {
		for _, focus := range []Field{FieldTitle, FieldStatus, FieldReason, FieldType, FieldPriority, FieldAssignee, FieldLabels, FieldText, FieldSave, FieldCancel} {
			state.Focus = focus
			view := plain(Render(state))

			if lines := strings.Count(view, "\n") + 1; lines != 24 {
				t.Fatalf("%s, focus %d: the screen is %d lines, want 24", name, focus, lines)
			}
			want := []string{state.TextLabel, "text line 1", fmt.Sprintf("text line %d", len(state.Text)), "Save ctrl+s", "Cancel esc"}
			for _, row := range state.Rows {
				want = append(want, row.Label)
			}
			for _, text := range want {
				if !strings.Contains(view, text) {
					t.Errorf("%s, focus %d: %q is not drawn:\n%s", name, focus, text, view)
				}
			}
		}
	}

	if rows := TextRows(closedWithError()); rows != 3 {
		t.Errorf("the text has %d rows under the reason row and a four-line error, want 3", rows)
	}
}

func TestErrorIsCutToFourLinesWithTheMark(t *testing.T) {
	t.Parallel()

	lines := errorLines(closedWithError())
	if len(lines) != maxErrorLines {
		t.Fatalf("the error takes %d lines, want %d", len(lines), maxErrorLines)
	}
	for _, line := range lines {
		if width := lipgloss.Width(line); width > 79 {
			t.Errorf("an error line is %d cells wide on a screen of 80", width)
		}
	}
	if last := plain(lines[maxErrorLines-1]); !strings.HasSuffix(last, "…") {
		t.Errorf("the last error line does not end in the cut mark: %q", last)
	}
}

// A screen too short for the form draws the field that has the focus.
func TestShortScreenDrawsTheFocusedField(t *testing.T) {
	t.Parallel()

	state := sampleState("Edit issue tm-1", editRows(&Row{Field: FieldReason, Label: "Reason", Value: "done"}), 80, 14)
	drawn := map[Field]string{
		FieldTitle: "Title", FieldStatus: "Status", FieldReason: "Reason", FieldType: "Type",
		FieldPriority: "Priority", FieldAssignee: "Assignee", FieldLabels: "Labels",
		FieldText: "text line 3", FieldSave: "Save ctrl+s", FieldCancel: "Cancel esc",
	}
	for focus, text := range drawn {
		state.Focus = focus
		view := plain(Render(state))
		if !strings.Contains(view, text) {
			t.Errorf("focus %d: %q is not drawn:\n%s", focus, text, view)
		}
		if lines := strings.Count(view, "\n") + 1; lines != 14 {
			t.Errorf("focus %d: the screen is %d lines, want 14", focus, lines)
		}
	}
}

// A refusal does not take the rows of the focused field: on a short screen it
// gets fewer lines than maxErrorLines.
func TestShortScreenWithAnErrorDrawsTheFocusedField(t *testing.T) {
	t.Parallel()

	drawn := map[Field]string{
		FieldTitle: "Title", FieldStatus: "Status", FieldReason: "Reason", FieldType: "Type",
		FieldPriority: "Priority", FieldAssignee: "Assignee", FieldLabels: "Labels",
		FieldText: "text line 3", FieldSave: "Save ctrl+s", FieldCancel: "Cancel esc",
	}
	for height := 6; height <= 14; height++ {
		state := sampleState("Edit issue tm-1", editRows(&Row{Field: FieldReason, Label: "Reason", Value: "done"}), 80, height)
		state.Error = refusal
		state = withText(state)
		for focus, text := range drawn {
			state.Focus = focus
			view := plain(Render(state))
			if !strings.Contains(view, text) {
				t.Errorf("height %d, focus %d: %q is not drawn:\n%s", height, focus, text, view)
			}
			if lines := strings.Count(view, "\n") + 1; lines != height {
				t.Errorf("height %d, focus %d: the screen is %d lines", height, focus, lines)
			}
		}
		if height >= 7 && !strings.Contains(plain(Render(state)), "the store refused") {
			t.Errorf("height %d: the error is not drawn", height)
		}
	}
}

// At height 10 the body has 5 rows: the error takes one, so the text keeps its
// heading and all of minTextRows.
func TestErrorLeavesTheTextItsRowsOnAShortScreen(t *testing.T) {
	t.Parallel()

	state := sampleState("New comment on tm-1", nil, 80, 10)
	state.Error = refusal
	state.Focus = FieldText
	state = withText(state)
	view := plain(Render(state))
	for _, want := range []string{"the store refused", "Description", "text line 1", "text line 2", "text line 3"} {
		if !strings.Contains(view, want) {
			t.Errorf("%q is not drawn:\n%s", want, view)
		}
	}
}

// A value the screen draws itself ends in the cut mark where the row ends.
func TestReadOnlyAndSteppedValuesAreCutWithTheMark(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a long close reason ", 8)
	state := sampleState("Edit issue tm-1", editRows(&Row{Field: FieldReason, Label: "Reason", Value: long, ReadOnly: true}), 80, 24)
	state.Rows[1].Value = strings.Repeat("status", 20)
	for _, line := range strings.Split(plain(Render(state)), "\n") {
		trimmed := strings.TrimRight(line, " ")
		switch {
		case strings.Contains(line, "Reason"):
			if !strings.HasSuffix(trimmed, "…") || lipgloss.Width(trimmed) != 80 {
				t.Errorf("the reason row is not cut at the last cell with the mark: %q", trimmed)
			}
		case strings.Contains(line, "Status"):
			if !strings.HasSuffix(trimmed, "… "+styles.Glyphs.StepNext) || lipgloss.Width(trimmed) != 80 {
				t.Errorf("the status row is not cut with the mark before the step marker: %q", trimmed)
			}
		}
	}
}

func TestHitTestReportsTheFieldDrawnAtACell(t *testing.T) {
	t.Parallel()

	short := sampleState("Edit issue tm-1", editRows(nil), 80, 14)
	short.Focus = FieldSave

	for name, state := range map[string]State{
		"edit":              sampleState("Edit issue tm-1", editRows(nil), 80, 24),
		"closed with error": closedWithError(),
		"short, on Save":    short,
	} {
		view := Render(state)
		drawn := plain(view)
		targets := map[string]Field{
			"Save ctrl+s": FieldSave, "Cancel esc": FieldCancel,
			state.TextLabel: FieldText, fmt.Sprintf("text line %d", len(state.Text)): FieldText,
		}
		for _, row := range state.Rows {
			targets[row.Label] = row.Field
		}
		for text, field := range targets {
			if !strings.Contains(drawn, text) {
				continue
			}
			x, y := testui.FindCell(t, view, text)
			hit, ok := HitTest(state, x, y)
			if !ok || hit.Field != field {
				t.Errorf("%s: %q at (%d,%d) reports field %d (ok %v), want %d", name, text, x, y, hit.Field, ok, field)
			}
		}
	}
}

func TestHitTestReportsNothingWhereNothingIsPressed(t *testing.T) {
	t.Parallel()

	state := closedWithError()
	state.Rows[2].ReadOnly = true
	view := Render(state)

	for _, text := range []string{"Edit issue tm-1", "because…", "Reason", "tab/shift+tab"} {
		x, y := testui.FindCell(t, view, text)
		if hit, ok := HitTest(state, x, y); ok {
			t.Errorf("%q at (%d,%d) reports field %d, want nothing", text, x, y, hit.Field)
		}
	}

	x, y := testui.FindCell(t, view, "Save ctrl+s")
	if _, ok := HitTest(state, x-3, y); ok {
		t.Error("the cell in front of the Save button reports a button")
	}

	state.Saving = true
	if _, ok := HitTest(state, x, y); ok {
		t.Error("the button line reports a button during a save")
	}
	if drawn := plain(Render(state)); !strings.Contains(drawn, "Saving…") || strings.Contains(drawn, "Cancel esc") {
		t.Errorf("a save draws the buttons, want Saving…:\n%s", drawn)
	}
}

// styleOn is the escape sequence style opens a run with.
func styleOn(style lipgloss.Style) string {
	on, _, _ := strings.Cut(style.Render("x"), "x")
	return on
}

// Not parallel: it renders in true colour.
func TestErrorAndFocusedButtonAreDrawnInTheirRoles(t *testing.T) {
	testui.ForceTrueColor(t)

	state := closedWithError()
	state.Focus = FieldSave
	lines := strings.Split(Render(state), "\n")

	errorOn := styleOn(lipgloss.NewStyle().Foreground(styles.ErrorTextColor))
	if line := lines[styles.ScreenBodyTop]; !strings.Contains(line, errorOn+"the store refused") {
		t.Errorf("the error is not drawn in ErrorTextColor: %q", line)
	}

	buttons := lines[len(lines)-2]
	if !strings.Contains(buttons, styles.PrimaryButtonFocusedStyle.Render("Save ctrl+s")) {
		t.Errorf("the focused Save button is not drawn in its focus style: %q", buttons)
	}
	if !strings.Contains(buttons, styles.SecondaryButtonStyle.Render("Cancel esc")) {
		t.Errorf("the Cancel button without the focus is not drawn at rest: %q", buttons)
	}
}
