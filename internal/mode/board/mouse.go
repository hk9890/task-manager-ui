package board

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
)

// handleMouse is the wheel, the pointer and the left button. The wheel moves
// the selection of the focused column, as the arrow keys do. One click selects
// an issue, taking the focus to its column, and a second opens it, as Enter
// does.
func (m *Model) handleMouse(msg mode.MouseMsg) tea.Cmd {
	m.pointer = msg.Pointer()
	if m.pointer == nil {
		return nil
	}

	hit, ok := uiboard.HitTest(m.viewState(0), msg.X, msg.Y)
	if !ok {
		return nil
	}

	switch msg.Kind {
	case mode.MouseWheelUp:
		return m.wheel(-1)
	case mode.MouseWheelDown:
		return m.wheel(1)
	case mode.MouseClick:
		return m.click(hit, msg)
	}
	return nil
}

// wheel moves the selection of the focused column, whichever column the
// pointer is over. Taking the focus to the column under the pointer would
// re-centre a board too narrow for all its columns, and the next notch would
// land on a different column than the first.
func (m *Model) wheel(delta int) tea.Cmd {
	previous := m.selectedIssueID()
	m.moveRow(delta)
	if m.selectedIssueID() == previous {
		return nil
	}
	return tea.Batch(m.selectionChangedCmd(), m.maybeLoadMoreClosed())
}

func (m *Model) click(hit uiboard.Hit, msg mode.MouseMsg) tea.Cmd {
	target := ""
	if hit.Row >= 0 {
		target = m.columns[hit.Column].issues[hit.Row].ID
	}
	previous := m.selectedIssueID()
	if m.clicks.Double(target, previous, msg) {
		return mode.RequestActionCmd(mode.Board, mode.ActionOpenDetail)
	}
	if target == "" {
		return nil
	}

	m.focusedColumn = hit.Column
	m.selectedRow[hit.Column] = hit.Row
	m.moveRow(0)
	if m.selectedIssueID() == previous {
		return nil
	}
	return tea.Batch(m.selectionChangedCmd(), m.maybeLoadMoreClosed())
}

func (m *Model) selectedIssueID() string {
	if selection := m.currentSelection(); selection != nil {
		return selection.Issue.ID
	}
	return ""
}

// viewState is the board as the renderer sees it. View and the hit test build
// the same value, so a click lands on the row that is drawn under it.
func (m *Model) viewState(skeletonPhase int) uiboard.State {
	uiColumns := make([]uiboard.Column, 0, len(m.columns))
	for colIdx := range m.columns {
		selectedRow := -1
		if colIdx == m.focusedColumn {
			selectedRow = m.selectedRow[colIdx]
		}
		uiColumns = append(uiColumns, m.uiColumn(colIdx, selectedRow))
	}

	return uiboard.State{
		DashboardTitle: dashboardTitle,
		Columns:        uiColumns,
		FocusedColumn:  m.focusedColumn,
		Width:          m.width,
		Height:         m.height,
		SkeletonPhase:  skeletonPhase,
		Now:            m.now(),
	}
}

// hover is the issue under the pointer, or nil.
func (m *Model) hover(state uiboard.State) *uiboard.Hit {
	if m.pointer == nil {
		return nil
	}
	hit, ok := uiboard.HitTest(state, m.pointer.X, m.pointer.Y)
	if !ok || hit.Row < 0 {
		return nil
	}
	return &hit
}
