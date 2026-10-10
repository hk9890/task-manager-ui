package rowlist

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
)

// defaultCapacity is the row window used before the first tea.WindowSizeMsg
// sets a real height.
const defaultCapacity = 20

// List is the selection, the scroll window and the pointer of a one-column
// surface. Every method takes the uiboard.State the surface draws, so the
// stored offset and a click are computed against the rows View draws.
type List struct {
	SelectedRow  int
	ScrollOffset int

	pointer *mode.Pointer
	clicks  mode.ClickTracker
}

// Clamp pulls the selection and the window back inside the rows of state. A
// surface calls it after its rows or its size changed.
func (l *List) Clamp(state uiboard.State) {
	column := state.Columns[0]
	if len(column.Rows) == 0 {
		l.SelectedRow = 0
		l.ScrollOffset = 0
		return
	}
	l.SelectedRow = min(max(l.SelectedRow, 0), len(column.Rows)-1)
	// Pull the window back inside the list first, as board's clampScrollOffsets
	// does. EnsureVisible only slides far enough to reveal the selected row, so
	// on its own a list that shrank under a scrolled offset keeps the offset and
	// draws its last rows with the ones above unreachable until the operator
	// moves up. MaxOffset counts the lines the renderer draws, as capacity does.
	content := capacity(state.Height)
	column.SelectedRow = l.SelectedRow
	column.ScrollOffset = min(l.ScrollOffset, uiboard.MaxOffset(column, content, state.Now))
	l.ScrollOffset = uiboard.EnsureVisible(column, content, state.Now)
}

// Move moves the selection by delta rows, clamped at both ends, and reports
// whether it landed on another row.
func (l *List) Move(delta int, state uiboard.State) (moved bool) {
	if len(state.Columns[0].Rows) == 0 {
		l.SelectedRow = 0
		return false
	}

	previous := l.SelectedRow
	l.SelectedRow += delta
	l.Clamp(state)
	return l.SelectedRow != previous
}

// MoveKey runs msg when it is one of the six move keys of the board context:
// the column is a board column, so the surfaces must not drift apart. handled
// is false for every other key.
func (l *List) MoveKey(keys config.ResolvedKeyBindings, msg tea.KeyMsg, state uiboard.State) (moved, handled bool) {
	rows := len(state.Columns[0].Rows)
	page := pageRows(state.Height)

	var delta int
	switch {
	case keys.Match(config.BoardContext, config.BoardActionMoveUp, msg):
		delta = -1
	case keys.Match(config.BoardContext, config.BoardActionMoveDown, msg):
		delta = 1
	case keys.Match(config.BoardContext, config.BoardActionPageUp, msg):
		delta = -page
	case keys.Match(config.BoardContext, config.BoardActionPageDown, msg):
		delta = page
	case keys.Match(config.BoardContext, config.BoardActionMoveHome, msg):
		delta = -rows
	case keys.Match(config.BoardContext, config.BoardActionMoveEnd, msg):
		delta = rows
	default:
		return false, false
	}
	return l.Move(delta, state), true
}

// Mouse is the board's mouse on one column: the wheel moves the selection, one
// click selects a row and a second opens it. open says the surface opens its
// current selection, and then nothing moved.
func (l *List) Mouse(msg mode.MouseMsg, state uiboard.State) (moved, open bool) {
	l.pointer = msg.Pointer()
	if l.pointer == nil {
		return false, false
	}

	hit, ok := uiboard.HitTest(state, msg.X, msg.Y)
	if !ok {
		return false, false
	}

	switch msg.Kind {
	case mode.MouseWheelUp:
		return l.Move(-1, state), false
	case mode.MouseWheelDown:
		return l.Move(1, state), false
	case mode.MouseClick:
		rows := state.Columns[0].Rows
		target := ""
		if hit.Row >= 0 {
			target = rows[hit.Row].ID
		}
		if l.clicks.Double(target, l.SelectedID(rows), msg) {
			return false, true
		}
		if target == "" {
			return false, false
		}
		return l.Move(hit.Row-l.SelectedRow, state), false
	}
	return false, false
}

// Hover is the row under the pointer, or nil.
func (l *List) Hover(state uiboard.State) *uiboard.Hit {
	if l.pointer == nil {
		return nil
	}
	hit, ok := uiboard.HitTest(state, l.pointer.X, l.pointer.Y)
	if !ok || hit.Row < 0 {
		return nil
	}
	return &hit
}

// Selection is the selected row of rows, or nil when there is none.
func (l *List) Selection(rows []domain.IssueSummary) *mode.Selection {
	if len(rows) == 0 {
		return nil
	}
	row := l.SelectedRow
	if row < 0 || row >= len(rows) {
		row = 0
	}
	return &mode.Selection{Issue: rows[row]}
}

// SelectedID is the ID of the selected row of rows, or "" when there is none.
func (l *List) SelectedID(rows []domain.IssueSummary) string {
	if selection := l.Selection(rows); selection != nil {
		return selection.Issue.ID
	}
	return ""
}

// capacity returns the number of content rows a column holds in a frame of
// height lines. It mirrors the board's section capacity; which of those rows
// are issues is uiboard.EnsureVisible's business.
func capacity(height int) int {
	if height == 0 {
		return defaultCapacity
	}
	return uiboard.ContentRows(height)
}

// pageRows is the number of rows a page key moves the selection by: the rows
// the column shows at that height.
func pageRows(height int) int {
	return max(1, capacity(height)/issuerow.Height)
}
