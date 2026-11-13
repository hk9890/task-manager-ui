package docs

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/mode"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
)

// handleMouse is the board's mouse on one column: the wheel moves the
// selection, one click selects a doc and a second opens it. The wheel over the
// preview scrolls the preview, and a click there does nothing.
func (m *Model) handleMouse(msg mode.MouseMsg) tea.Cmd {
	if uiboard.OverAside(m.viewState(0), msg.X, msg.Y) {
		m.list.Mouse(mode.MouseMsg{Kind: mode.MouseLeave}, m.viewState(0))
		switch msg.Kind {
		case mode.MouseWheelUp:
			m.scrollPreview(-previewWheelLines)
		case mode.MouseWheelDown:
			m.scrollPreview(previewWheelLines)
		}
		return nil
	}
	moved, open := m.list.Mouse(msg, m.viewState(0))
	if open {
		return mode.RequestOpenDetailCmd(mode.Docs, m.currentSelection())
	}
	return m.selectionMovedCmd(moved)
}

// viewState is the docs column as the renderer sees it. View and the hit test
// build the same value, so a click lands on the row that is drawn under it.
func (m *Model) viewState(skeletonPhase int) uiboard.State {
	_, previewWidth, _ := m.split()
	return uiboard.State{
		Query:         m.query.Text(),
		Placeholder:   queryPlaceholder,
		Columns:       []uiboard.Column{m.uiColumn()},
		FocusedColumn: 0,
		Width:         m.width,
		Height:        m.height,
		SkeletonPhase: skeletonPhase,
		AsideWidth:    previewWidth,
	}
}
