// Package issueedit is the issue-editor controller: the full-screen surface
// where the operator writes an issue or a comment.
//
// It is not a browse tab. Like the configuration screen it sits above all of
// them, so it is absent from mode.BrowseModes and renders instead of the shell
// rather than inside it (docs/DESIGN-GUIDE.md). It writes nothing: a save
// emits SubmitMsg and the shell builds the write.
package issueedit

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	uiissueedit "github.com/hk9890/task-manager-ui/internal/ui/issueedit"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// wheelLines is how far one wheel notch moves the cursor of the text.
const wheelLines = 3

var priorities = []string{"P0", "P1", "P2", "P3", "P4"}

// Kind is what the screen writes.
type Kind int

const (
	KindCreate Kind = iota
	KindEdit
	KindCommentAdd
	KindCommentEdit
)

// Values is what the fields hold.
type Values struct {
	Title    string
	Status   string
	Type     string
	Priority int
	Assignee string
	// Labels is the labels as typed, comma separated.
	Labels string
	Reason string
	Text   string
}

// Options is what the screen opens with.
type Options struct {
	Kind     Kind
	IssueID  string
	Subtitle string
	// Values is what the fields start with.
	Values   Values
	Statuses []string
	Types    []string
	// Close steps the status to closed and puts the focus on the reason.
	Close bool
}

// SubmitMsg asks the shell to write. Before is what the screen opened with, as
// the widgets hold it, so a field the operator did not touch is no change.
type SubmitMsg struct{ Before, After Values }

// CancelMsg is the Cancel button.
type CancelMsg struct{}

// Model is the issue-editor controller.
type Model struct {
	keys config.ResolvedKeyBindings

	width  int
	height int

	kind     Kind
	issueID  string
	subtitle string

	title    textinput.Model
	assignee textinput.Model
	labels   textinput.Model
	reason   textinput.Model
	text     textarea.Model

	statuses []string
	types    []string
	status   int
	typ      int
	priority int

	baseline Values
	focus    uiissueedit.Field
	errText  string
	saving   bool
}

// NewModel builds the issue-editor controller.
//
// The widgets exist from here on: the shell sizes the screen before it opens
// it the first time.
func NewModel(keys config.ResolvedKeyBindings) *Model {
	m := &Model{keys: keys}
	m.Open(Options{})
	return m
}

// Open takes what the screen shows and puts the focus on its first field. The
// widgets are built here, so they draw in the theme applied now.
func (m *Model) Open(opts Options) {
	m.kind = opts.Kind
	m.issueID = opts.IssueID
	m.subtitle = opts.Subtitle
	m.errText = ""
	m.saving = false

	m.title = newInput(opts.Values.Title, "")
	m.assignee = newInput(opts.Values.Assignee, "")
	m.labels = newInput(opts.Values.Labels, "comma separated")
	m.reason = newInput(opts.Values.Reason, "")
	m.text = newText(opts.Values.Text)

	m.statuses, m.status = options(opts.Statuses, opts.Values.Status)
	m.types, m.typ = options(opts.Types, opts.Values.Type)
	m.priority = textutil.Clamp(opts.Values.Priority, 0, len(priorities)-1)

	// The widgets normalise a text: a TAB becomes spaces, CRLF becomes LF. The
	// baseline is read back from them, so that is no change.
	m.baseline = m.values()

	focus := uiissueedit.FieldTitle
	switch {
	case m.comment():
		focus = uiissueedit.FieldText
	case opts.Close:
		m.statuses, m.status = options(m.statuses, domain.StatusClosed)
		focus = uiissueedit.FieldReason
	}
	m.setFocus(focus)
	m.resize()

	// The text opens on its first character. The widget moves its window to
	// the cursor only in Update, so the move is a key.
	m.moveText(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl}, 1)
}

// options is the values a stepped field steps through and the index of value
// in them. A value the catalog does not have comes first.
func options(catalog []string, value string) ([]string, int) {
	if idx := slices.Index(catalog, value); idx >= 0 || value == "" {
		return catalog, max(idx, 0)
	}
	return append([]string{value}, catalog...), 0
}

func newInput(value, placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	style := textinput.StyleState{
		Text:        lipgloss.NewStyle().Foreground(styles.TextPrimaryColor),
		Placeholder: lipgloss.NewStyle().Foreground(styles.TextMutedColor),
	}
	input.SetStyles(textinput.Styles{Focused: style, Blurred: style, Cursor: textinput.CursorStyle{Color: lipgloss.NoColor{}}})
	// The terminal's own paste arrives as a paste message and is typed. The
	// widget's paste reads the system clipboard, which the screen does not
	// touch.
	input.KeyMap.Paste = key.NewBinding()
	input.KeyMap.DeleteCharacterBackward = withShift(input.KeyMap.DeleteCharacterBackward, "backspace")
	input.SetValue(value)
	return input
}

func newText(value string) textarea.Model {
	text := textarea.New()
	text.Prompt = ""
	text.ShowLineNumbers = false
	// The widget stops a newline at MaxHeight lines.
	text.MaxHeight = 0
	style := textarea.StyleState{
		Text:        lipgloss.NewStyle().Foreground(styles.TextPrimaryColor),
		CursorLine:  lipgloss.NewStyle().Foreground(styles.TextPrimaryColor),
		Placeholder: lipgloss.NewStyle().Foreground(styles.TextMutedColor),
	}
	text.SetStyles(textarea.Styles{Focused: style, Blurred: style, Cursor: textarea.CursorStyle{Color: lipgloss.NoColor{}}})
	// alt+u, alt+l and alt+c are shell habits, and ctrl+t and ctrl+v do
	// nothing here. The screen draws no selection, so no key starts one.
	text.KeyMap.Paste = key.NewBinding()
	text.KeyMap.UppercaseWordForward = key.NewBinding()
	text.KeyMap.LowercaseWordForward = key.NewBinding()
	text.KeyMap.CapitalizeWordForward = key.NewBinding()
	text.KeyMap.TransposeCharacterBackward = key.NewBinding()
	text.KeyMap.SelectCharacterForward = key.NewBinding()
	text.KeyMap.SelectCharacterBackward = key.NewBinding()
	text.KeyMap.SelectWordForward = key.NewBinding()
	text.KeyMap.SelectWordBackward = key.NewBinding()
	text.KeyMap.SelectLineUp = key.NewBinding()
	text.KeyMap.SelectLineDown = key.NewBinding()
	text.KeyMap.SelectAll = key.NewBinding()
	text.KeyMap.CopySelection = key.NewBinding()
	text.KeyMap.InsertNewline = withShift(text.KeyMap.InsertNewline, "enter")
	text.KeyMap.DeleteCharacterBackward = withShift(text.KeyMap.DeleteCharacterBackward, "backspace")
	// The widget makes a line break of a CR and of an LF each, so a CRLF is
	// one LF before the widget gets it.
	text.SetValue(strings.ReplaceAll(value, "\r\n", "\n"))
	return text
}

// withShift adds shift+name to binding. A terminal with the kitty keyboard
// protocol reports the shift of enter and backspace, which others hide, and
// the shifted key does what the plain one does.
func withShift(binding key.Binding, name string) key.Binding {
	binding.SetKeys(append(binding.Keys(), "shift+"+name)...)
	return binding
}

// SetSize updates render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.resize()
}

// SetSaving says whether a save is in flight. Until it ends the screen takes
// no key but quit and help, so the result finds the typed values on screen.
func (m *Model) SetSaving(saving bool) {
	m.saving = saving
}

// SetError shows the refusal of a save above the form, "" for none.
func (m *Model) SetError(text string) {
	m.errText = text
	m.resize()
}

// resize gives every widget the room the screen has for it now. The text
// takes the rows the error and the rows leave.
func (m *Model) resize() {
	for _, input := range []*textinput.Model{&m.title, &m.assignee, &m.labels, &m.reason} {
		input.SetWidth(uiissueedit.InputWidth(m.width))
	}
	m.text.SetWidth(uiissueedit.TextWidth(m.width))
	m.text.SetHeight(uiissueedit.TextRows(m.viewState("", "")))
	m.moveText(nil, 1)
}

func (m *Model) comment() bool {
	return m.kind == KindCommentAdd || m.kind == KindCommentEdit
}

func (m *Model) statusValue() string {
	if len(m.statuses) == 0 {
		return ""
	}
	return m.statuses[m.status]
}

func (m *Model) typeValue() string {
	if len(m.types) == 0 {
		return ""
	}
	return m.types[m.typ]
}

func (m *Model) values() Values {
	return Values{
		Title:    m.title.Value(),
		Status:   m.statusValue(),
		Type:     m.typeValue(),
		Priority: m.priority,
		Assignee: m.assignee.Value(),
		Labels:   m.labels.Value(),
		Reason:   m.reason.Value(),
		Text:     m.text.Value(),
	}
}

// rows is the one-line fields in the order drawn. The reason stands under the
// status while that is closed, and is read-only on an issue closed before:
// the store takes a reason only when it closes.
func (m *Model) rows() []uiissueedit.Row {
	if m.comment() {
		return nil
	}
	rows := []uiissueedit.Row{{Field: uiissueedit.FieldTitle, Label: "Title", Value: m.title.View()}}
	if m.kind == KindEdit {
		rows = append(rows, uiissueedit.Row{Field: uiissueedit.FieldStatus, Label: "Status", Value: m.statusValue(), Stepped: true})
		switch {
		case m.statusValue() != domain.StatusClosed:
		case m.baseline.Status == domain.StatusClosed:
			rows = append(rows, uiissueedit.Row{Field: uiissueedit.FieldReason, Label: "Reason", Value: m.reason.Value(), ReadOnly: true})
		default:
			rows = append(rows, uiissueedit.Row{Field: uiissueedit.FieldReason, Label: "Reason", Value: m.reason.View()})
		}
	}
	return append(rows,
		uiissueedit.Row{Field: uiissueedit.FieldType, Label: "Type", Value: m.typeValue(), Stepped: true},
		uiissueedit.Row{Field: uiissueedit.FieldPriority, Label: "Priority", Value: priorities[m.priority], Stepped: true},
		uiissueedit.Row{Field: uiissueedit.FieldAssignee, Label: "Assignee", Value: m.assignee.View()},
		uiissueedit.Row{Field: uiissueedit.FieldLabels, Label: "Labels", Value: m.labels.View()},
	)
}

// order is the fields the focus walks, in order.
func (m *Model) order() []uiissueedit.Field {
	var fields []uiissueedit.Field
	for _, row := range m.rows() {
		if !row.ReadOnly {
			fields = append(fields, row.Field)
		}
	}
	return append(fields, uiissueedit.FieldText, uiissueedit.FieldSave, uiissueedit.FieldCancel)
}

// input is the text input of field, or nil for a field that is none.
func (m *Model) input(field uiissueedit.Field) *textinput.Model {
	switch field {
	case uiissueedit.FieldTitle:
		return &m.title
	case uiissueedit.FieldReason:
		return &m.reason
	case uiissueedit.FieldAssignee:
		return &m.assignee
	case uiissueedit.FieldLabels:
		return &m.labels
	}
	return nil
}

// setFocus moves the focus to field. A widget takes keys and draws its cursor
// only while it is focused.
func (m *Model) setFocus(field uiissueedit.Field) {
	for _, input := range []*textinput.Model{&m.title, &m.assignee, &m.labels, &m.reason} {
		input.Blur()
	}
	m.text.Blur()

	m.focus = field
	if input := m.input(field); input != nil {
		input.Focus()
	}
	if field == uiissueedit.FieldText {
		m.text.Focus()
	}
}

// moveFocus steps the focus by delta fields and wraps at both ends.
func (m *Model) moveFocus(delta int) {
	order := m.order()
	m.setFocus(order[(slices.Index(order, m.focus)+delta+len(order))%len(order)])
}

// moveText sends the text a key, times over, also while another field has the
// focus: the widget takes a key only while focused.
//
// The widget moves its window to the cursor at the end of Update, against the
// lines its last View drew. A pasted text adds lines no View has drawn, so the
// lines are drawn and the window is moved once more.
func (m *Model) moveText(msg tea.Msg, times int) {
	focused := m.text.Focused()
	m.text.Focus()
	for range times {
		m.text, _ = m.text.Update(msg)
	}
	m.text.View()
	m.text, _ = m.text.Update(nil)
	if !focused {
		m.text.Blur()
	}
}

// textCursorOnFirstLine counts the lines as drawn: a wrapped line of the text
// is several.
func (m *Model) textCursorOnFirstLine() bool {
	return m.text.Line() == 0 && m.text.LineInfo().RowOffset == 0
}

// textDown moves the text cursor one drawn line down. On the last drawn line
// it moves nothing and reports false.
//
// The widget counts an empty row under a line that fills the width. The cursor
// is in that row only at the end of the line. From anywhere else in the line
// the widget does not move the cursor past the row, so the line below is
// reached from the end of the line.
func (m *Model) textDown(msg tea.KeyPressMsg) bool {
	line := m.text.Line()
	lastLine := line == m.text.LineCount()-1
	before := m.text.LineInfo()
	if lastLine && before.RowOffset+1 >= before.Height {
		return false
	}

	m.moveText(msg, 1)
	if m.text.Line() != line || m.text.LineInfo().RowOffset != before.RowOffset {
		return true
	}
	if lastLine {
		return false
	}

	m.text.CursorEnd()
	m.moveText(msg, 1)
	m.text.SetCursorColumn(min(before.ColumnOffset, m.text.LineInfo().Width-1))
	return true
}

// HandleKey processes one key press and reports whether the screen consumed
// it. An unconsumed key falls through to the shell: quit, help and Escape.
//
// The keys that move between the fields are the keys themselves, as on the
// configuration screen: the screen takes typed text, so a binding on a key
// that prints would leave no way out of a field.
func (m *Model) HandleKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	// Runes that arrive in one message are text, as a paste is, also when they
	// spell the name of a key: a key is matched by its name, and "esc" typed
	// over a slow link would leave the screen.
	if utf8.RuneCountInString(msg.Text) > 1 {
		return m.HandlePaste(tea.PasteMsg{Content: msg.Text})
	}

	shell := func(action string) bool { return m.keys.Match(config.ShellContext, action, msg) }
	switch {
	case shell(config.ShellActionQuit), shell(config.ShellActionHelp):
		return false, nil
	case m.saving:
		return true, nil
	case shell(config.ShellActionEscape):
		return false, nil
	case shell(config.ShellActionSaveEdit):
		return true, m.submit()
	}

	switch msg.String() {
	case "tab":
		m.moveFocus(1)
		return true, nil
	case "shift+tab":
		m.moveFocus(-1)
		return true, nil
	}

	// A widget types the runes of an alt+ chord too, and those are shell keys.
	typed := !msg.Mod.Contains(tea.ModAlt) || !unicode.IsPrint(msg.Code)

	switch {
	case m.focus == uiissueedit.FieldText:
		switch {
		case msg.String() == "pgup":
			m.moveText(tea.KeyPressMsg{Code: tea.KeyUp}, m.text.Height())
		case msg.String() == "pgdown":
			m.moveText(tea.KeyPressMsg{Code: tea.KeyDown}, m.text.Height())
		case msg.String() == "up" && m.textCursorOnFirstLine():
			// A comment has no field above its text.
			if !m.comment() {
				m.moveFocus(-1)
			}
		case msg.String() == "down":
			if !m.textDown(msg) {
				m.setFocus(uiissueedit.FieldSave)
			}
		case typed:
			m.moveText(msg, 1)
		}
	case m.focus == uiissueedit.FieldSave || m.focus == uiissueedit.FieldCancel:
		return true, m.buttonKey(msg)
	case m.input(m.focus) != nil:
		switch {
		case msg.String() == "up":
			m.moveFocus(-1)
		case msg.String() == "down", msg.String() == "enter":
			m.moveFocus(1)
		case typed:
			input := m.input(m.focus)
			*input, _ = input.Update(msg)
		}
	default:
		switch msg.String() {
		case "left":
			m.step(-1)
		case "right":
			m.step(1)
		case "up":
			m.moveFocus(-1)
		case "down", "enter":
			m.moveFocus(1)
		}
	}
	return true, nil
}

// HandlePaste types pasted text into the focused text or input, as the
// widget types it, with each CRLF as one LF, as newText holds the opening
// value: the widgets break a line at a CR and at an LF each. The screen
// consumes every paste: one into another field, or during a save, does
// nothing.
func (m *Model) HandlePaste(msg tea.PasteMsg) (bool, tea.Cmd) {
	msg.Content = strings.ReplaceAll(msg.Content, "\r\n", "\n")
	switch {
	case m.saving:
	case m.focus == uiissueedit.FieldText:
		m.moveText(msg, 1)
	case m.input(m.focus) != nil:
		input := m.input(m.focus)
		*input, _ = input.Update(msg)
	}
	return true, nil
}

func (m *Model) buttonKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "left", "right":
		if m.focus == uiissueedit.FieldSave {
			m.setFocus(uiissueedit.FieldCancel)
		} else {
			m.setFocus(uiissueedit.FieldSave)
		}
	case "up":
		m.moveFocus(-1)
	case "down":
		m.moveFocus(1)
	case "enter":
		return m.press(m.focus)
	}
	return nil
}

// step moves the focused stepped field by delta values and wraps at both
// ends. The reason row comes and goes with the status, so the text is sized
// again.
func (m *Model) step(delta int) {
	wrap := func(index, count int) int { return (index + delta + count) % max(count, 1) }
	switch m.focus {
	case uiissueedit.FieldStatus:
		m.status = wrap(m.status, len(m.statuses))
		m.resize()
	case uiissueedit.FieldType:
		m.typ = wrap(m.typ, len(m.types))
	case uiissueedit.FieldPriority:
		m.priority = wrap(m.priority, len(priorities))
	}
}

// press is Enter on a button, and a click on it.
func (m *Model) press(button uiissueedit.Field) tea.Cmd {
	if button == uiissueedit.FieldCancel {
		return func() tea.Msg { return CancelMsg{} }
	}
	return m.submit()
}

// submit asks the shell to write. The save is in flight from here, not from
// the moment the shell hears of it: a second save key in between must not
// start a second write.
func (m *Model) submit() tea.Cmd {
	m.saving = true
	msg := SubmitMsg{Before: m.baseline, After: m.values()}
	return func() tea.Msg { return msg }
}

// HandleMouse does what a key does: a click on a field gives it the focus, a
// click on a button presses it, and the wheel over the text moves its cursor.
// During a save it does nothing, as a key does nothing then, and a move of the
// pointer does nothing at any time: the screen lights nothing under it.
func (m *Model) HandleMouse(msg mode.MouseMsg) tea.Cmd {
	if m.saving || msg.Kind == mode.MouseMove {
		return nil
	}
	hit, ok := uiissueedit.HitTest(m.viewState("", ""), msg.X, msg.Y)
	if !ok {
		return nil
	}
	switch msg.Kind {
	case mode.MouseClick:
		if hit.Field == uiissueedit.FieldSave || hit.Field == uiissueedit.FieldCancel {
			return m.press(hit.Field)
		}
		m.setFocus(hit.Field)
	case mode.MouseWheelUp:
		if hit.Field == uiissueedit.FieldText {
			m.moveText(tea.KeyPressMsg{Code: tea.KeyUp}, wheelLines)
		}
	case mode.MouseWheelDown:
		if hit.Field == uiissueedit.FieldText {
			m.moveText(tea.KeyPressMsg{Code: tea.KeyDown}, wheelLines)
		}
	}
	return nil
}

func (m *Model) heading() (title, textLabel string) {
	switch m.kind {
	case KindCreate:
		return "New issue", "Description"
	case KindCommentAdd:
		return fmt.Sprintf("New comment on %s", m.issueID), "Comment"
	case KindCommentEdit:
		return fmt.Sprintf("Edit comment on %s", m.issueID), "Comment"
	}
	return fmt.Sprintf("Edit issue %s", m.issueID), "Description"
}

// viewState is the one state Render and HitTest both read.
func (m *Model) viewState(version, help string) uiissueedit.State {
	title, textLabel := m.heading()
	return uiissueedit.State{
		Title:     title,
		Subtitle:  m.subtitle,
		Rows:      m.rows(),
		TextLabel: textLabel,
		Text:      strings.Split(m.text.View(), "\n"),
		Focus:     m.focus,
		Error:     m.errText,
		Saving:    m.saving,
		SaveKey:   m.keys.DisplayPrimary(config.ShellContext, config.ShellActionSaveEdit),
		CancelKey: m.keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape),
		Version:   version,
		Help:      help,
		Width:     m.width,
		Height:    m.height,
	}
}

// View renders the screen full size.
func (m *Model) View(version, help string) string {
	return uiissueedit.Render(m.viewState(version, help))
}
