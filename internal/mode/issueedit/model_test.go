package issueedit

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uiissueedit "github.com/hk9890/task-manager-ui/internal/ui/issueedit"
)

var (
	keyTab      = testui.Press("tab")
	keyShiftTab = testui.Press("shift+tab")
	keyUp       = testui.Press("up")
	keyDown     = testui.Press("down")
	keyLeft     = testui.Press("left")
	keyRight    = testui.Press("right")
	keyEnter    = testui.Press("enter")
	keySave     = testui.Press("ctrl+s")
)

var (
	statuses = []string{"open", "in_progress", "closed"}
	types    = []string{"task", "bug", "feature"}
)

func issueValues() Values {
	return Values{Title: "Fix the header", Status: "open", Type: "bug", Priority: 1, Assignee: "hans", Labels: "ui, docs", Text: "first line\nsecond line"}
}

func open(t *testing.T, opts Options) *Model {
	t.Helper()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	m := NewModel(keys)
	m.SetSize(80, 24)
	opts.IssueID = "tm-1"
	opts.Statuses, opts.Types = statuses, types
	m.Open(opts)
	return m
}

// openOn opens the editor and moves the focus to field.
func openOn(t *testing.T, opts Options, field uiissueedit.Field) *Model {
	t.Helper()

	m := open(t, opts)
	m.setFocus(field)
	return m
}

func openEdit(t *testing.T) *Model {
	t.Helper()
	return open(t, Options{Kind: KindEdit, Values: issueValues()})
}

// press sends keys and pastes the screen must consume and returns the
// message the last one asks the shell with, or nil.
func press(t *testing.T, m *Model, keys ...tea.Msg) tea.Msg {
	t.Helper()

	var msg tea.Msg
	for _, key := range keys {
		var consumed bool
		var cmd tea.Cmd
		switch key := key.(type) {
		case tea.KeyPressMsg:
			consumed, cmd = m.HandleKey(key)
		case tea.PasteMsg:
			consumed, cmd = m.HandlePaste(key)
		default:
			t.Fatalf("press takes a key or a paste, not %T", key)
		}
		if !consumed {
			t.Fatalf("%v was not consumed", key)
		}
		msg = nil
		if cmd != nil {
			msg = cmd()
		}
	}
	return msg
}

func plainView(m *Model) string {
	return testui.AnsiEscapePattern.ReplaceAllString(m.View("v1", "legend"), "")
}

func TestFocusWalksEveryFieldOfEachKindAndWraps(t *testing.T) {
	t.Parallel()

	f := struct{ title, status, reason, typ, priority, assignee, labels, text, save, cancel uiissueedit.Field }{
		uiissueedit.FieldTitle, uiissueedit.FieldStatus, uiissueedit.FieldReason, uiissueedit.FieldType, uiissueedit.FieldPriority,
		uiissueedit.FieldAssignee, uiissueedit.FieldLabels, uiissueedit.FieldText, uiissueedit.FieldSave, uiissueedit.FieldCancel,
	}
	closedBefore := issueValues()
	closedBefore.Status = domain.StatusClosed

	tests := []struct {
		name string
		opts Options
		want []uiissueedit.Field
	}{
		{"create", Options{Kind: KindCreate}, []uiissueedit.Field{f.title, f.typ, f.priority, f.assignee, f.labels, f.text, f.save, f.cancel}},
		{"edit", Options{Kind: KindEdit, Values: issueValues()}, []uiissueedit.Field{f.title, f.status, f.typ, f.priority, f.assignee, f.labels, f.text, f.save, f.cancel}},
		{"close", Options{Kind: KindEdit, Values: issueValues(), Close: true}, []uiissueedit.Field{f.reason, f.typ, f.priority, f.assignee, f.labels, f.text, f.save, f.cancel, f.title, f.status}},
		{"an issue closed before", Options{Kind: KindEdit, Values: closedBefore}, []uiissueedit.Field{f.title, f.status, f.typ, f.priority, f.assignee, f.labels, f.text, f.save, f.cancel}},
		{"comment add", Options{Kind: KindCommentAdd}, []uiissueedit.Field{f.text, f.save, f.cancel}},
		{"comment edit", Options{Kind: KindCommentEdit, Values: Values{Text: "a comment"}}, []uiissueedit.Field{f.text, f.save, f.cancel}},
	}
	for _, tt := range tests {
		m := open(t, tt.opts)

		var forward []uiissueedit.Field
		for range tt.want {
			forward = append(forward, m.focus)
			press(t, m, keyTab)
		}
		if !slices.Equal(forward, tt.want) {
			t.Errorf("%s: tab walks %v, want %v", tt.name, forward, tt.want)
		}
		if m.focus != tt.want[0] {
			t.Errorf("%s: tab from the last field lands on %d, want the first", tt.name, m.focus)
		}

		for idx := len(tt.want) - 1; idx >= 0; idx-- {
			press(t, m, keyShiftTab)
			if m.focus != tt.want[idx] {
				t.Errorf("%s: shift+tab lands on %d, want %d", tt.name, m.focus, tt.want[idx])
			}
		}
	}
}

func TestReasonRowFollowsTheStatus(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	if strings.Contains(plainView(m), "Reason") {
		t.Fatal("an open issue draws the reason row")
	}
	rowsOpen := m.text.Height()

	press(t, m, keyTab, keyLeft)
	if got := m.values().Status; got != domain.StatusClosed {
		t.Fatalf("left from the first status gives %q, want it to wrap to closed", got)
	}
	if !strings.Contains(plainView(m), "Reason") {
		t.Error("a closed status draws no reason row")
	}
	if got := m.text.Height(); got != rowsOpen-1 {
		t.Errorf("the text has %d rows with the reason row, want %d", got, rowsOpen-1)
	}

	press(t, m, keyTab)
	if m.focus != uiissueedit.FieldReason {
		t.Fatalf("tab from the status lands on %d, want the reason", m.focus)
	}
	press(t, m, testui.Runes("done"))
	if got := m.values().Reason; got != "done" {
		t.Errorf("the reason holds %q, want done", got)
	}

	press(t, m, keyShiftTab, keyRight)
	if got := m.values().Status; got != "open" {
		t.Fatalf("right from closed gives %q, want it to wrap to open", got)
	}
	if strings.Contains(plainView(m), "Reason") {
		t.Error("the reason row stays after the status left closed")
	}
}

func TestCloseOpensOnTheReasonAndKeepsTheOldStatusAsBaseline(t *testing.T) {
	t.Parallel()

	m := open(t, Options{Kind: KindEdit, Values: issueValues(), Close: true})
	if m.focus != uiissueedit.FieldReason {
		t.Errorf("the focus is on %d, want the reason", m.focus)
	}
	submit := press(t, m, testui.Runes("done"), keySave).(SubmitMsg)
	if submit.Before.Status != "open" || submit.After.Status != domain.StatusClosed || submit.After.Reason != "done" {
		t.Errorf("submit carries status %q -> %q with reason %q", submit.Before.Status, submit.After.Status, submit.After.Reason)
	}
}

// The reason of an issue closed before is drawn and takes neither the focus
// nor a click: the store takes a reason only when it closes.
func TestReasonOfAnIssueClosedBeforeIsReadOnly(t *testing.T) {
	t.Parallel()

	values := issueValues()
	values.Status, values.Reason = domain.StatusClosed, "shipped"
	m := open(t, Options{Kind: KindEdit, Values: values})

	view := m.View("v1", "legend")
	if !strings.Contains(testui.AnsiEscapePattern.ReplaceAllString(view, ""), "shipped") {
		t.Fatal("the reason of the closed issue is not drawn")
	}
	x, y := testui.FindCell(t, view, "shipped")
	m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y})
	if m.focus != uiissueedit.FieldTitle {
		t.Errorf("a click on the read-only reason moved the focus to %d", m.focus)
	}
}

func TestStepsWrapAndAValueOutsideTheCatalogComesFirst(t *testing.T) {
	t.Parallel()

	values := issueValues()
	values.Type = "legacy"
	m := openOn(t, Options{Kind: KindEdit, Values: values}, uiissueedit.FieldType)

	if got := m.values().Type; got != "legacy" {
		t.Fatalf("the type opens on %q, want the value of the issue", got)
	}
	press(t, m, keyLeft)
	if got := m.values().Type; got != "feature" {
		t.Errorf("left from the first type gives %q, want the last", got)
	}
	press(t, m, keyRight, keyRight)
	if got := m.values().Type; got != "task" {
		t.Errorf("two steps right give %q, want task", got)
	}

	press(t, m, keyDown)
	if m.focus != uiissueedit.FieldPriority {
		t.Fatalf("down from the type lands on %d, want the priority", m.focus)
	}
	press(t, m, keyLeft, keyLeft)
	if got := m.values().Priority; got != 4 {
		t.Errorf("two steps left from P1 give P%d, want P4", got)
	}

	press(t, m, testui.Press("alt+left"))
	if got := m.values().Priority; got != 4 {
		t.Errorf("alt+left stepped the priority to P%d", got)
	}
}

func TestCreateStartsOnTheFirstTypeAndTheGivenPriority(t *testing.T) {
	t.Parallel()

	m := open(t, Options{Kind: KindCreate, Values: Values{Priority: 2}})
	if got := m.values(); got.Type != "task" || got.Priority != 2 {
		t.Errorf("a new issue starts on type %q and P%d, want task and P2", got.Type, got.Priority)
	}
	if view := plainView(m); strings.Contains(view, "Status") {
		t.Errorf("a new issue draws a status row:\n%s", view)
	}
}

func TestUpAndDownMoveFieldsOnARowAndTheCursorInTheText(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	press(t, m, keyDown, keyDown, keyEnter)
	if m.focus != uiissueedit.FieldPriority {
		t.Fatalf("down, down, enter from the title land on %d, want the priority", m.focus)
	}
	press(t, m, keyUp)
	if m.focus != uiissueedit.FieldType {
		t.Fatalf("up from the priority lands on %d, want the type", m.focus)
	}

	m = openOn(t, Options{Kind: KindEdit, Values: issueValues()}, uiissueedit.FieldText)
	press(t, m, keyDown, testui.Runes("X"), keyUp, testui.Press("home"), testui.Runes("Y"))
	if m.focus != uiissueedit.FieldText {
		t.Fatalf("an arrow inside the text left it for %d", m.focus)
	}
	if got := m.values().Text; got != "Yfirst line\nXsecond line" {
		t.Errorf("the text is %q; the arrows did not move its cursor", got)
	}

	press(t, m, keyEnter)
	if got := m.values().Text; got != "Y\nfirst line\nXsecond line" {
		t.Errorf("enter in the text gives %q, want a new line", got)
	}
}

// TestShiftEnterAndShiftBackspaceActAsTheKeysWithoutShift: a terminal with
// the kitty keyboard protocol reports the shift of enter and backspace, which
// others hide. The shifted key does what the plain one does.
func TestShiftEnterAndShiftBackspaceActAsTheKeysWithoutShift(t *testing.T) {
	t.Parallel()

	m := open(t, Options{Kind: KindCommentAdd})
	press(t, m, testui.Runes("ab"), testui.Press("shift+enter"), testui.Runes("c"))
	if got := m.values().Text; got != "ab\nc" {
		t.Errorf("shift+enter in the text gives %q, want a line break", got)
	}
	press(t, m, testui.Press("shift+backspace"))
	if got := m.values().Text; got != "ab\n" {
		t.Errorf("shift+backspace in the text gives %q, want the last rune deleted", got)
	}

	m = openOn(t, Options{Kind: KindEdit, Values: issueValues()}, uiissueedit.FieldTitle)
	press(t, m, testui.Press("shift+backspace"))
	if got := m.values().Title; got != "Fix the heade" {
		t.Errorf("shift+backspace in the title gives %q, want the last rune deleted", got)
	}
}

func TestUpOnTheFirstLineAndDownOnTheLastLineLeaveTheText(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	for range len(m.order()) {
		if m.focus == uiissueedit.FieldText {
			break
		}
		press(t, m, keyDown)
	}
	if m.focus != uiissueedit.FieldText {
		t.Fatalf("down from the title never reached the text; the focus is on %d", m.focus)
	}

	press(t, m, keyDown)
	if m.focus != uiissueedit.FieldText || m.text.Line() != 1 {
		t.Fatalf("down on the first of two lines: focus %d, line %d, want the text and line 1", m.focus, m.text.Line())
	}
	press(t, m, keyDown)
	if m.focus != uiissueedit.FieldSave {
		t.Fatalf("down on the last line lands on %d, want Save", m.focus)
	}

	press(t, m, keyUp, keyUp)
	if m.focus != uiissueedit.FieldText || m.text.Line() != 0 {
		t.Fatalf("up from Save and up again: focus %d, line %d, want the text and line 0", m.focus, m.text.Line())
	}
	press(t, m, keyUp)
	if m.focus != uiissueedit.FieldLabels {
		t.Fatalf("up on the first line lands on %d, want the labels", m.focus)
	}
	press(t, m, keyUp, keyUp, keyUp, keyUp, keyUp)
	if m.focus != uiissueedit.FieldTitle {
		t.Fatalf("up from the labels five times lands on %d, want the title", m.focus)
	}
	if got := m.values().Text; got != m.baseline.Text {
		t.Errorf("the arrows changed the text to %q", got)
	}
}

func TestAWrappedLineOfTheTextIsLeftFromItsFirstAndLastRowOnly(t *testing.T) {
	t.Parallel()

	values := issueValues()
	values.Text = strings.Repeat("word ", 40)
	m := openOn(t, Options{Kind: KindEdit, Values: values}, uiissueedit.FieldText)

	press(t, m, keyDown)
	if m.focus != uiissueedit.FieldText {
		t.Fatalf("down on the first row of a wrapped line left the text for %d", m.focus)
	}
	press(t, m, keyUp)
	if m.focus != uiissueedit.FieldText {
		t.Fatalf("up on the second row of a wrapped line left the text for %d", m.focus)
	}
	press(t, m, keyUp)
	if m.focus != uiissueedit.FieldLabels {
		t.Fatalf("up on the first row lands on %d, want the labels", m.focus)
	}
}

func TestDownLeavesALastLineThatFillsTheWidthOfTheText(t *testing.T) {
	t.Parallel()

	// At the end of the line the cursor is in the empty row the widget counts
	// under it.
	keyEnd := testui.Press("end")
	for _, before := range []string{"", "short\n"} {
		for _, inLine := range [][]tea.Msg{nil, {keyEnd}} {
			values := issueValues()
			m := openOn(t, Options{Kind: KindEdit, Values: values}, uiissueedit.FieldText)
			values.Text = before + strings.Repeat("x", m.text.Width())
			m = openOn(t, Options{Kind: KindEdit, Values: values}, uiissueedit.FieldText)

			press(t, m, slices.Repeat([]tea.Msg{keyDown}, strings.Count(before, "\n"))...)
			press(t, m, inLine...)
			press(t, m, keyDown)
			if m.focus != uiissueedit.FieldSave {
				t.Errorf("after %q and %d keys in the line, down on the full last line lands on %d, want Save", before, len(inLine), m.focus)
			}
			if got := m.values().Text; got != values.Text {
				t.Errorf("after %q, down changed the text to %q", before, got)
			}
		}
	}
}

// The widget does not move the cursor down out of a line that fills the
// width, unless from its end. Down reaches the line below all the same, and
// leaves the text from the last line only.
func TestDownOnALineThatFillsTheWidthReachesTheLineBelow(t *testing.T) {
	t.Parallel()

	values := issueValues()
	width := openOn(t, Options{Kind: KindEdit, Values: values}, uiissueedit.FieldText).text.Width()
	tests := []struct {
		name string
		full string
		rows int
	}{
		{"one word", strings.Repeat("x", width), 1},
		{"a space in the last cell", strings.Repeat("x", width-1) + " ", 1},
		{"two rows", strings.Repeat("x", 2*width), 2},
	}
	for _, tt := range tests {
		values.Text = tt.full + "\nbelow"
		m := openOn(t, Options{Kind: KindEdit, Values: values}, uiissueedit.FieldText)

		press(t, m, keyRight, keyRight)
		press(t, m, slices.Repeat([]tea.Msg{keyDown}, tt.rows)...)
		if m.focus != uiissueedit.FieldText || m.text.Line() != 1 || m.text.LineInfo().ColumnOffset != 2 {
			t.Errorf("%s: %d times down from the full line: focus %d, line %d, column %d; want the text, line 1 and column 2",
				tt.name, tt.rows, m.focus, m.text.Line(), m.text.LineInfo().ColumnOffset)
		}
		press(t, m, keyDown)
		if m.focus != uiissueedit.FieldSave {
			t.Errorf("%s: down on the last line lands on %d, want Save", tt.name, m.focus)
		}
		if got := m.values().Text; got != values.Text {
			t.Errorf("%s: down changed the text to %q", tt.name, got)
		}
	}
}

func TestUpOnTheFirstLineOfACommentStaysInTheText(t *testing.T) {
	t.Parallel()

	m := open(t, Options{Kind: KindCommentAdd})
	press(t, m, keyUp)
	if m.focus != uiissueedit.FieldText {
		t.Fatalf("up in an empty comment lands on %d, want the text", m.focus)
	}
	press(t, m, keyDown)
	if m.focus != uiissueedit.FieldSave {
		t.Fatalf("down on the last line of a comment lands on %d, want Save", m.focus)
	}
}

func TestTextOpensOnItsFirstCharacter(t *testing.T) {
	t.Parallel()

	values := issueValues()
	values.Text = strings.Repeat("a line\n", 60)
	m := open(t, Options{Kind: KindEdit, Values: values})

	if view := plainView(m); strings.Count(view, "a line") != m.text.Height() {
		t.Fatalf("the text does not fill its %d rows from the first line:\n%s", m.text.Height(), view)
	}
	if m.text.Line() != 0 {
		t.Errorf("the cursor opens on line %d, want the first", m.text.Line())
	}
	if got := m.values().Text; got != m.baseline.Text {
		t.Error("opening the text changed it")
	}
}

func TestPageKeysAndTheWheelMoveTheTextCursor(t *testing.T) {
	t.Parallel()

	values := issueValues()
	values.Text = strings.Repeat("a line\n", 60)
	m := openOn(t, Options{Kind: KindEdit, Values: values}, uiissueedit.FieldText)
	page := m.text.Height()

	press(t, m, testui.Press("pgdown"))
	if got := m.text.Line(); got != page {
		t.Errorf("pgdown moved the cursor to line %d, want %d", got, page)
	}
	press(t, m, testui.Press("pgup"))
	if got := m.text.Line(); got != 0 {
		t.Errorf("pgup moved the cursor to line %d, want 0", got)
	}

	// The wheel moves the cursor also while another field has the focus, and
	// leaves the focus where it is.
	press(t, m, keyTab)
	x, y := testui.FindCell(t, m.View("v1", "legend"), "Description")
	m.HandleMouse(mode.MouseMsg{Kind: mode.MouseWheelDown, X: x, Y: y + 1})
	if got := m.text.Line(); got != wheelLines {
		t.Errorf("a wheel notch moved the cursor to line %d, want %d", got, wheelLines)
	}
	if m.focus != uiissueedit.FieldSave {
		t.Errorf("the wheel moved the focus to %d", m.focus)
	}
}

func TestAltChordsTypeNothing(t *testing.T) {
	t.Parallel()

	for _, focus := range []uiissueedit.Field{uiissueedit.FieldTitle, uiissueedit.FieldText} {
		m := openOn(t, Options{Kind: KindEdit, Values: issueValues()}, focus)
		before := m.values()
		for _, letter := range "audclbf" {
			press(t, m, testui.Press("alt+"+string(letter)))
		}
		if got := m.values(); got != before {
			t.Errorf("focus %d: an alt+letter chord changed the values to %+v", focus, got)
		}
	}
}

// The terminal's own paste arrives as one message. The widgets' paste key,
// which reads the system clipboard, is unbound.
func TestPasteIsTypedAndCtrlVDoesNothing(t *testing.T) {
	t.Parallel()

	paste := testui.Paste("pasted text")

	m := open(t, Options{Kind: KindCreate})
	press(t, m, paste)
	if got := m.values().Title; got != "pasted text" {
		t.Errorf("a paste into the title gives %q", got)
	}
	if msg := press(t, m, testui.Press("ctrl+v")); msg != nil {
		t.Errorf("ctrl+v in the title produced %T", msg)
	}

	m = open(t, Options{Kind: KindCommentAdd})
	lines := strings.Repeat("pasted\n", 40) + "the end"
	press(t, m, testui.Paste(lines))
	if got := m.values().Text; got != lines {
		t.Errorf("a paste of lines into the text gives %q", got)
	}
	if view := plainView(m); !strings.Contains(view, "the end") {
		t.Errorf("the text does not show its cursor line after a paste of more lines than it has rows:\n%s", view)
	}
	if msg := press(t, m, testui.Press("ctrl+v")); msg != nil {
		t.Errorf("ctrl+v in the text produced %T", msg)
	}
	if got := m.values().Text; got != lines {
		t.Errorf("ctrl+v changed the text to %q", got)
	}
}

// A paste is typed as the widget types it: the text makes a TAB 4 spaces and
// a CR and an LF a line break each, an input makes each of them a space.
func TestPasteKeepsTheTextHandlingOfTheWidgets(t *testing.T) {
	t.Parallel()

	paste := testui.Paste("one\ttwo\r\nthree")

	m := open(t, Options{Kind: KindCreate})
	press(t, m, paste)
	if got, want := m.values().Title, "one two  three"; got != want {
		t.Errorf("a paste into the title gives %q, want %q", got, want)
	}

	m = open(t, Options{Kind: KindCommentAdd})
	press(t, m, paste)
	if got, want := m.values().Text, "one    two\n\nthree"; got != want {
		t.Errorf("a paste into the text gives %q, want %q", got, want)
	}
}

func TestPasteIntoASteppedFieldOrDuringASaveChangesNothing(t *testing.T) {
	t.Parallel()

	m := openOn(t, Options{Kind: KindEdit, Values: issueValues()}, uiissueedit.FieldType)
	press(t, m, testui.Paste("closed"))
	if got := m.values(); got != issueValues() {
		t.Errorf("a paste on the type changed the values to %+v", got)
	}

	m = openEdit(t)
	m.SetSaving(true)
	press(t, m, testui.Paste("x"))
	if got := m.values(); got != issueValues() {
		t.Errorf("a paste during a save changed the values to %+v", got)
	}
}

// The screen draws no selection, so no key of the text starts one: the key
// after it would replace text the operator cannot see.
func TestNoKeyStartsASelectionInTheText(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"shift+right", "shift+left", "shift+up", "shift+down", "ctrl+shift+right", "alt+shift+left", "ctrl+g", "ctrl+shift+c"} {
		m := openOn(t, Options{Kind: KindEdit, Values: issueValues()}, uiissueedit.FieldText)
		press(t, m, keyRight, testui.Press(name))
		if m.text.HasSelection() {
			t.Errorf("%s started a selection", name)
		}
		press(t, m, testui.Runes("X"))
		if got, want := m.values().Text, "fXirst line\nsecond line"; got != want {
			t.Errorf("%s, then X gives %q, want %q", name, got, want)
		}
	}
}

func TestSaveKeySubmitsFromAnyFieldWithBaselineAndValues(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	press(t, m, testui.Runes("!"))
	for range m.order() {
		msg := press(t, m, keySave)
		submit, ok := msg.(SubmitMsg)
		if !ok {
			t.Fatalf("the save key on field %d produced %T, want a SubmitMsg", m.focus, msg)
		}
		if submit.Before != issueValues() {
			t.Errorf("Before = %+v, want what the screen opened with", submit.Before)
		}
		want := issueValues()
		want.Title = "Fix the header!"
		if submit.After != want {
			t.Errorf("After = %+v, want %+v", submit.After, want)
		}
		m.SetSaving(false)
		press(t, m, keyTab)
	}
}

func TestSecondSaveBeforeTheResultSubmitsNothing(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	if _, ok := press(t, m, keySave).(SubmitMsg); !ok {
		t.Fatal("the save key produced no SubmitMsg")
	}
	if msg := press(t, m, keySave); msg != nil {
		t.Errorf("a second save key produced %T, want nothing", msg)
	}
	x, y := testui.FindCell(t, m.View("v1", "legend"), "Saving…")
	if cmd := m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y}); cmd != nil {
		t.Errorf("a click during the save produced %T, want nothing", cmd())
	}
}

func TestButtonsPressOnEnterAndOnAClick(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	m.setFocus(uiissueedit.FieldSave)
	if _, ok := press(t, m, keyEnter).(SubmitMsg); !ok {
		t.Error("enter on Save does not submit")
	}
	m.SetSaving(false)
	press(t, m, keyRight)
	if m.focus != uiissueedit.FieldCancel {
		t.Fatalf("right from Save lands on %d, want Cancel", m.focus)
	}
	if _, ok := press(t, m, keyEnter).(CancelMsg); !ok {
		t.Error("enter on Cancel does not cancel")
	}
	press(t, m, keyLeft, keyUp)
	if m.focus != uiissueedit.FieldText {
		t.Errorf("left, up from Cancel land on %d, want the text", m.focus)
	}

	view := m.View("v1", "legend")
	click := func(text string) tea.Msg {
		x, y := testui.FindCell(t, view, text)
		cmd := m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y})
		if cmd == nil {
			return nil
		}
		return cmd()
	}
	if _, ok := click("Save ctrl+s").(SubmitMsg); !ok {
		t.Error("a click on Save does not submit")
	}
	m.SetSaving(false)
	if _, ok := click("Cancel esc").(CancelMsg); !ok {
		t.Error("a click on Cancel does not cancel")
	}
	if msg := click("Assignee"); msg != nil || m.focus != uiissueedit.FieldAssignee {
		t.Errorf("a click on the assignee row gives %T and the focus on %d", msg, m.focus)
	}
	press(t, m, testui.Runes("x"))
	if got := m.values().Assignee; got != "hansx" {
		t.Errorf("the clicked field holds %q after a key, want hansx", got)
	}
}

// The widgets hold a TAB as spaces and CRLF as LF. The baseline is what they
// hold, so a text nobody touched is no change.
func TestStoredTextTheWidgetsNormaliseIsNoChange(t *testing.T) {
	t.Parallel()

	values := issueValues()
	values.Text = "one\ttab\r\nnext line"
	m := open(t, Options{Kind: KindEdit, Values: values})

	submit := press(t, m, keySave).(SubmitMsg)
	if submit.Before != submit.After {
		t.Errorf("an untouched issue submits a change:\nbefore %+v\nafter  %+v", submit.Before, submit.After)
	}
	if want := "one    tab\nnext line"; submit.After.Text != want {
		t.Errorf("the text is %q, want %q: a TAB is 4 spaces and a CRLF one line break", submit.After.Text, want)
	}
}

// Runes that reach the app in one message are typed, also when they spell the
// name of a key the screen or the shell acts on.
func TestRunesThatArriveTogetherAreTextAlsoWhenTheySpellAKey(t *testing.T) {
	t.Parallel()

	words := []string{"esc", "tab", "up", "down", "left", "right", "enter", "pgup", "delete", "backspace", "home"}
	for _, focus := range []uiissueedit.Field{uiissueedit.FieldAssignee, uiissueedit.FieldText} {
		for _, word := range words {
			m := openOn(t, Options{Kind: KindEdit, Values: Values{Status: "open", Type: "task"}}, focus)
			press(t, m, testui.Runes(word))

			got := m.values().Assignee
			if focus == uiissueedit.FieldText {
				got = m.values().Text
			}
			if got != word || m.focus != focus {
				t.Errorf("focus %d: the runes %q gave the value %q and the focus %d", focus, word, got, m.focus)
			}
		}
	}
}

func TestQuitHelpAndEscapeFallThroughAndEveryOtherKeyIsConsumed(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	for name, key := range map[string]tea.KeyPressMsg{
		"quit":   testui.Press("ctrl+c"),
		"help":   testui.Press("alt+h"),
		"escape": testui.Press("esc"),
	} {
		if consumed, cmd := m.HandleKey(key); consumed || cmd != nil {
			t.Errorf("%s was consumed; the shell owns it", name)
		}
	}
	for name, key := range map[string]tea.KeyPressMsg{
		"a shell action": testui.Press("alt+n"),
		"delete":         testui.Press("delete"),
		"a tab key":      testui.Press("ctrl+pgdown"),
		"f5":             testui.Press("f5"),
	} {
		if consumed, cmd := m.HandleKey(key); !consumed || cmd != nil {
			t.Errorf("%s: consumed %v, command %v; nothing below the screen is drawn", name, consumed, cmd != nil)
		}
	}
}

func TestSavingTakesEveryKeyButQuitAndHelp(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	m.SetSaving(true)
	before := m.values()

	for _, key := range []tea.KeyPressMsg{testui.Press("esc"), keySave, keyTab, testui.Runes("x"), keyEnter, keyLeft} {
		if consumed, cmd := m.HandleKey(key); !consumed || cmd != nil {
			t.Errorf("%q during a save: consumed %v, command %v", key.String(), consumed, cmd != nil)
		}
	}
	if m.values() != before || m.focus != uiissueedit.FieldTitle {
		t.Error("a key during a save changed the screen")
	}
	for _, key := range []tea.KeyPressMsg{testui.Press("ctrl+c"), testui.Press("alt+h")} {
		if consumed, _ := m.HandleKey(key); consumed {
			t.Errorf("%q was consumed during a save", key.String())
		}
	}

	view := m.View("v1", "legend")
	x, y := testui.FindCell(t, view, "Assignee")
	if cmd := m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y}); cmd != nil || m.focus != uiissueedit.FieldTitle {
		t.Error("a click during a save moved the focus")
	}
	if !strings.Contains(view, "Saving…") {
		t.Errorf("a save is not shown:\n%s", view)
	}
}

func TestErrorTakesRowsFromTheTextAndGivesThemBack(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	rows := m.text.Height()

	m.SetError("the store refused the write")
	if !strings.Contains(plainView(m), "the store refused the write") {
		t.Error("the refusal is not drawn")
	}
	if got := m.text.Height(); got != rows-1 {
		t.Errorf("the text has %d rows under a one-line error, want %d", got, rows-1)
	}
	if lines := strings.Count(m.View("v1", "legend"), "\n") + 1; lines != 24 {
		t.Errorf("the screen is %d lines with an error, want 24", lines)
	}

	m.SetError("")
	if got := m.text.Height(); got != rows {
		t.Errorf("the text has %d rows after the error went, want %d", got, rows)
	}
}

func TestTitlesNameTheKindAndTheIssue(t *testing.T) {
	t.Parallel()

	for kind, want := range map[Kind][]string{
		KindCreate:      {"New issue", "Description"},
		KindEdit:        {"Edit issue tm-1", "Description"},
		KindCommentAdd:  {"New comment on tm-1", "Comment"},
		KindCommentEdit: {"Edit comment on tm-1", "Comment"},
	} {
		view := plainView(open(t, Options{Kind: kind, Subtitle: "the subtitle"}))
		for _, text := range append(want, "the subtitle") {
			if !strings.Contains(view, text) {
				t.Errorf("kind %d: %q is not drawn:\n%s", kind, text, view)
			}
		}
	}
}
