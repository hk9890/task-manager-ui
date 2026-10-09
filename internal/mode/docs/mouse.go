package docs

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
)

// pointer is the cell the mouse is over, in the tab's own coordinates. The
// hover is derived from it on every draw, as the board's is.
type pointer struct {
	x, y int
}

// handleMouse is the board's mouse on one column: the wheel moves the
// selection, one click selects a doc and a second opens it.
func (m *Model) handleMouse(msg mode.MouseMsg) tea.Cmd {
	if msg.Kind == mode.MouseLeave {
		m.pointer = nil
		return nil
	}
	m.pointer = &pointer{x: msg.X, y: msg.Y}

	hit, ok := uiboard.HitTest(m.viewState(0), msg.X, msg.Y)
	if !ok {
		return nil
	}

	switch msg.Kind {
	case mode.MouseWheelUp:
		return m.moveRow(-1)
	case mode.MouseWheelDown:
		return m.moveRow(1)
	case mode.MouseClick:
		target := ""
		if hit.Row >= 0 {
			target = m.issues[hit.Row].ID
		}
		if m.clicks.Double(target, msg) {
			return mode.RequestActionCmd(mode.Docs, mode.ActionOpenDetail)
		}
		if target == "" {
			return nil
		}
		return m.moveRow(hit.Row - m.selectedRow)
	}
	return nil
}

// viewState is the docs column as the renderer sees it. View and the hit test
// build the same value, so a click lands on the row that is drawn under it.
//
// No dashboard title: with a single column the tab chip and the column header
// already name the surface, so the board's title line would only repeat them.
func (m *Model) viewState(skeletonPhase int) uiboard.State {
	return uiboard.State{
		Columns:       []uiboard.Column{m.uiColumn()},
		FocusedColumn: 0,
		Width:         m.width,
		Height:        m.height,
		SkeletonPhase: skeletonPhase,
		Now:           m.now(),
	}
}

// hover is the doc under the pointer, or nil.
func (m *Model) hover(state uiboard.State) *uiboard.Hit {
	if m.pointer == nil {
		return nil
	}
	hit, ok := uiboard.HitTest(state, m.pointer.x, m.pointer.y)
	if !ok || hit.Row < 0 {
		return nil
	}
	return &hit
}
