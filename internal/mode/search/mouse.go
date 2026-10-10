package search

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
)

// handleMouse is the board's mouse on one column: the wheel moves the
// selection, one click selects a result and a second opens it.
func (m *Model) handleMouse(msg mode.MouseMsg) tea.Cmd {
	// A click or a wheel notch drops a held Enter, as a key does: it can move
	// the selection the Enter was pressed on. So does the pointer leaving, which
	// is also what the shell sends a surface it no longer draws.
	if msg.Kind != mode.MouseMove {
		m.heldOpen = false
	}
	moved, open := m.list.Mouse(msg, m.viewState(0))
	if open {
		return m.openDetail()
	}
	return m.selectionMovedCmd(moved)
}

// viewState is the results column as the renderer sees it. View and the hit
// test build the same value, so a click lands on the row that is drawn under
// it. The column draws no age markers, so the state carries no clock.
func (m *Model) viewState(skeletonPhase int) uiboard.State {
	return uiboard.State{
		Query:         m.query.Text(),
		Placeholder:   queryPlaceholder,
		Search:        true,
		Columns:       []uiboard.Column{m.uiColumn()},
		FocusedColumn: 0,
		Width:         m.width,
		Height:        m.height,
		SkeletonPhase: skeletonPhase,
	}
}
