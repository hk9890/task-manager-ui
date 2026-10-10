package board

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
)

// handleMouse is the wheel, the pointer and the left button. The wheel moves
// the selection of the column under the pointer, taking the focus there. One
// click selects an issue, taking the focus to its column, and a second opens
// it, as Enter does.
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
		return m.wheel(hit.Column, -1)
	case mode.MouseWheelDown:
		return m.wheel(hit.Column, 1)
	case mode.MouseClick:
		return m.click(hit, msg)
	}
	return nil
}

// wheel focuses column and moves its selection one row. The column is drawn
// under the pointer, so the focus change leaves the columns where they are and
// the next notch lands on the same one.
func (m *Model) wheel(column, delta int) tea.Cmd {
	previous := m.selectedIssueID()
	m.focusedColumn = column
	m.queryHome = -1
	m.moveRow(delta)
	if m.selectedIssueID() == previous {
		return nil
	}
	return tea.Batch(m.selectionChangedCmd(), m.maybeLoadMoreClosed())
}

func (m *Model) click(hit uiboard.Hit, msg mode.MouseMsg) tea.Cmd {
	target := ""
	if hit.Row >= 0 {
		target = m.columns[hit.Column].shown[hit.Row].ID
	}
	previous := m.selectedIssueID()
	if m.clicks.Double(target, previous, msg) {
		return mode.RequestOpenDetailCmd(mode.Board, m.currentSelection())
	}
	if target == "" {
		return nil
	}

	m.focusedColumn = hit.Column
	m.queryHome = -1
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
		Query:         m.query.Text(),
		Placeholder:   queryPlaceholder,
		Columns:       uiColumns,
		FocusedColumn: m.focusedColumn,
		ColumnStart:   m.columnStart,
		Width:         m.width,
		Height:        m.height,
		SkeletonPhase: skeletonPhase,
		Now:           m.now(),
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
