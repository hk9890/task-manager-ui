package docs

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
)

// handleMouse is the board's mouse on one column: the wheel moves the
// selection, one click selects a doc and a second opens it.
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
		return m.moveRow(-1)
	case mode.MouseWheelDown:
		return m.moveRow(1)
	case mode.MouseClick:
		target := ""
		if hit.Row >= 0 {
			target = m.issues[hit.Row].ID
		}
		if m.clicks.Double(target, m.selectedIssueID(), msg) {
			return mode.RequestActionCmd(mode.Docs, mode.ActionOpenDetail)
		}
		if target == "" {
			return nil
		}
		return m.moveRow(hit.Row - m.selectedRow)
	}
	return nil
}

func (m *Model) selectedIssueID() string {
	if selection := m.currentSelection(); selection != nil {
		return selection.Issue.ID
	}
	return ""
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
	hit, ok := uiboard.HitTest(state, m.pointer.X, m.pointer.Y)
	if !ok || hit.Row < 0 {
		return nil
	}
	return &hit
}
