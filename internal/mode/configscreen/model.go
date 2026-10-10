// Package configscreen is the configuration-screen controller: the full-screen
// surface where the operator picks the theme and the glyph set.
//
// It is not a browse tab. Like the store picker it sits above all of them, so
// it is absent from mode.BrowseModes and renders instead of the shell rather
// than inside it (docs/DESIGN-GUIDE.md).
package configscreen

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	uiconfigscreen "github.com/hk9890/task-manager-ui/internal/ui/configscreen"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// ChangeMsg asks the shell for a theme and a glyph set, one of them stepped
// from what the screen shows. The screen emits it rather than changing either
// itself: the write to the config file, the styles every surface draws with
// and the config the app holds are the shell's, and the screen shows a new
// value only once the shell has made it so (SetValues).
type ChangeMsg struct {
	Theme  string
	Glyphs string
}

// The rows, in the order drawn.
const (
	rowTheme = iota
	rowGlyphs
	rowCount
)

// Model is the configuration-screen controller.
type Model struct {
	keys config.ResolvedKeyBindings

	width  int
	height int

	// path is the config file a change is written to, shown on the screen.
	path string

	theme  string
	glyphs string

	selectedRow int
}

// NewModel builds the configuration-screen controller.
func NewModel(keys config.ResolvedKeyBindings) *Model {
	return &Model{keys: keys}
}

// Open puts the cursor on the first row and takes what the screen shows: the
// config file and the two values in use.
func (m *Model) Open(path, theme, glyphs string) {
	m.path = displaytext.OneLine(path)
	m.selectedRow = rowTheme
	m.SetValues(theme, glyphs)
}

// SetValues takes the theme and the glyph set in use, after the shell changed
// one.
func (m *Model) SetValues(theme, glyphs string) {
	m.theme = theme
	m.glyphs = glyphs
}

// SetSize updates render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// HandleKey processes one key press and reports whether the screen consumed
// it. An unconsumed key falls through to the shell, which is how Escape, quit
// and help keep working while the screen is up.
//
// Row movement reads the board keybinding context, as the store picker does.
// Left, Right and Enter are the keys themselves: a value has no binding to
// step it, and alt+ chords stay with the shell.
func (m *Model) HandleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch {
	case m.keys.Match(config.BoardContext, config.BoardActionMoveUp, msg):
		m.selectedRow = max(m.selectedRow-1, 0)
		return true, nil
	case m.keys.Match(config.BoardContext, config.BoardActionMoveDown, msg):
		m.selectedRow = min(m.selectedRow+1, rowCount-1)
		return true, nil
	case msg.Alt:
		return false, nil
	case msg.Type == tea.KeyLeft:
		return true, m.step(-1)
	case msg.Type == tea.KeyRight, msg.Type == tea.KeyEnter:
		return true, m.step(1)
	}
	return false, nil
}

// step asks the shell for the value before or after the selected row's, and
// wraps at both ends of the list.
func (m *Model) step(delta int) tea.Cmd {
	change := ChangeMsg{Theme: m.theme, Glyphs: m.glyphs}
	if m.selectedRow == rowTheme {
		change.Theme = cycle(styles.Themes(), m.theme, delta)
	} else {
		change.Glyphs = cycle(styles.GlyphSets(), m.glyphs, delta)
	}
	return func() tea.Msg { return change }
}

func cycle(values []string, current string, delta int) string {
	return values[(slices.Index(values, current)+delta+len(values))%len(values)]
}

// View renders the screen full size.
func (m *Model) View(help string) string {
	return uiconfigscreen.Render(uiconfigscreen.State{
		Path: m.path,
		Rows: []uiconfigscreen.Row{
			rowTheme:  {Label: "theme", Value: m.theme},
			rowGlyphs: {Label: "glyphs", Value: m.glyphs},
		},
		SelectedRow: m.selectedRow,
		Help:        help,
		Width:       m.width,
		Height:      m.height,
	})
}
