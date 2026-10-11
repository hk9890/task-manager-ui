package issueedit

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uiissueedit "github.com/hk9890/task-manager-ui/internal/ui/issueedit"
)

var (
	keyTab      = tea.KeyMsg{Type: tea.KeyTab}
	keyShiftTab = tea.KeyMsg{Type: tea.KeyShiftTab}
	keyUp       = tea.KeyMsg{Type: tea.KeyUp}
	keyDown     = tea.KeyMsg{Type: tea.KeyDown}
	keyLeft     = tea.KeyMsg{Type: tea.KeyLeft}
	keyRight    = tea.KeyMsg{Type: tea.KeyRight}
	keyEnter    = tea.KeyMsg{Type: tea.KeyEnter}
	keySave     = tea.KeyMsg{Type: tea.KeyCtrlS}
)

func typed(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

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

func openEdit(t *testing.T) *Model {
	t.Helper()
	return open(t, Options{Kind: KindEdit, Values: issueValues()})
}

// press sends keys the screen must consume and returns the message the last
// one asks the shell with, or nil.
func press(t *testing.T, m *Model, keys ...tea.KeyMsg) tea.Msg {
	t.Helper()

	var msg tea.Msg
	for _, key := range keys {
		consumed, cmd := m.HandleKey(key)
		if !consumed {
			t.Fatalf("%q was not consumed", key.String())
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
	closedBefore.Status = closed

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

func TestFocusOpensOnTheFieldAsked(t *testing.T) {
	t.Parallel()

	if m := open(t, Options{Kind: KindEdit, Values: issueValues(), Focus: uiissueedit.FieldPriority}); m.focus != uiissueedit.FieldPriority {
		t.Errorf("the editor opened on field %d, want the priority", m.focus)
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
	if got := m.values().Status; got != closed {
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
	press(t, m, typed("done"))
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
	submit := press(t, m, typed("done"), keySave).(SubmitMsg)
	if submit.Before.Status != "open" || submit.After.Status != closed || submit.After.Reason != "done" {
		t.Errorf("submit carries status %q -> %q with reason %q", submit.Before.Status, submit.After.Status, submit.After.Reason)
	}
}

// The reason of an issue closed before is drawn and takes neither the focus
// nor a click: the store takes a reason only when it closes.
func TestReasonOfAnIssueClosedBeforeIsReadOnly(t *testing.T) {
	t.Parallel()

	values := issueValues()
	values.Status, values.Reason = closed, "shipped"
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
	m := open(t, Options{Kind: KindEdit, Values: values, Focus: uiissueedit.FieldType})

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

	press(t, m, tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
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

	m = open(t, Options{Kind: KindEdit, Values: issueValues(), Focus: uiissueedit.FieldText})
	press(t, m, keyDown, typed("X"), keyUp, keyUp, keyUp, tea.KeyMsg{Type: tea.KeyHome}, typed("Y"))
	if m.focus != uiissueedit.FieldText {
		t.Fatalf("an arrow left the text for %d", m.focus)
	}
	if got := m.values().Text; got != "Yfirst line\nXsecond line" {
		t.Errorf("the text is %q; the arrows did not move its cursor", got)
	}

	press(t, m, keyEnter)
	if got := m.values().Text; got != "Y\nfirst line\nXsecond line" {
		t.Errorf("enter in the text gives %q, want a new line", got)
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
	m := open(t, Options{Kind: KindEdit, Values: values, Focus: uiissueedit.FieldText})
	page := m.text.Height()

	press(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if got := m.text.Line(); got != page {
		t.Errorf("pgdown moved the cursor to line %d, want %d", got, page)
	}
	press(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
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
		m := open(t, Options{Kind: KindEdit, Values: issueValues(), Focus: focus})
		before := m.values()
		for _, letter := range "audclbf" {
			press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{letter}, Alt: true})
		}
		if got := m.values(); got != before {
			t.Errorf("focus %d: an alt+letter chord changed the values to %+v", focus, got)
		}
	}
}

// The terminal's own paste arrives as one key message. The widgets' paste
// key, which reads the system clipboard, is unbound.
func TestPasteIsTypedAndCtrlVDoesNothing(t *testing.T) {
	t.Parallel()

	paste := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pasted text"), Paste: true}

	m := open(t, Options{Kind: KindCreate})
	press(t, m, paste)
	if got := m.values().Title; got != "pasted text" {
		t.Errorf("a paste into the title gives %q", got)
	}
	if msg := press(t, m, tea.KeyMsg{Type: tea.KeyCtrlV}); msg != nil {
		t.Errorf("ctrl+v in the title produced %T", msg)
	}

	m = open(t, Options{Kind: KindCommentAdd})
	lines := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("pasted\n", 40) + "the end"), Paste: true}
	press(t, m, lines)
	if got := m.values().Text; got != string(lines.Runes) {
		t.Errorf("a paste of lines into the text gives %q", got)
	}
	if view := plainView(m); !strings.Contains(view, "the end") {
		t.Errorf("the text does not show its cursor line after a paste of more lines than it has rows:\n%s", view)
	}
	if msg := press(t, m, tea.KeyMsg{Type: tea.KeyCtrlV}); msg != nil {
		t.Errorf("ctrl+v in the text produced %T", msg)
	}
	if got := m.values().Text; got != string(lines.Runes) {
		t.Errorf("ctrl+v changed the text to %q", got)
	}
}

func TestSaveKeySubmitsFromAnyFieldWithBaselineAndValues(t *testing.T) {
	t.Parallel()

	m := openEdit(t)
	press(t, m, typed("!"))
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
	press(t, m, typed("x"))
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
			m := open(t, Options{Kind: KindEdit, Values: Values{Status: "open", Type: "task"}, Focus: focus})
			press(t, m, typed(word))

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
	for name, key := range map[string]tea.KeyMsg{
		"quit":   {Type: tea.KeyCtrlC},
		"help":   {Type: tea.KeyRunes, Runes: []rune("h"), Alt: true},
		"escape": {Type: tea.KeyEsc},
	} {
		if consumed, cmd := m.HandleKey(key); consumed || cmd != nil {
			t.Errorf("%s was consumed; the shell owns it", name)
		}
	}
	for name, key := range map[string]tea.KeyMsg{
		"a shell action": {Type: tea.KeyRunes, Runes: []rune("n"), Alt: true},
		"delete":         {Type: tea.KeyDelete},
		"a tab key":      {Type: tea.KeyCtrlPgDown},
		"f5":             {Type: tea.KeyF5},
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

	for _, key := range []tea.KeyMsg{{Type: tea.KeyEsc}, keySave, keyTab, typed("x"), keyEnter, keyLeft} {
		if consumed, cmd := m.HandleKey(key); !consumed || cmd != nil {
			t.Errorf("%q during a save: consumed %v, command %v", key.String(), consumed, cmd != nil)
		}
	}
	if m.values() != before || m.focus != uiissueedit.FieldTitle {
		t.Error("a key during a save changed the screen")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyCtrlC}, {Type: tea.KeyRunes, Runes: []rune("h"), Alt: true}} {
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
