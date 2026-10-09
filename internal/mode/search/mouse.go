package search

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	uisearch "github.com/hk9890/task-manager-ui/internal/ui/search"
)

// pointer is the cell the mouse is over, in the tab's own coordinates. The
// hover is derived from it on every draw, as the board's is.
type pointer struct {
	x, y int
}

// handleMouse is the wheel, the pointer and the left button. The wheel over
// the results moves the selection. A click focuses the pane it lands on; on a
// result it also selects it, and a second click opens it.
func (m *Model) handleMouse(msg mode.MouseMsg) tea.Cmd {
	if msg.Kind == mode.MouseLeave {
		m.pointer = nil
		return nil
	}
	m.pointer = &pointer{x: msg.X, y: msg.Y}

	hit, ok := uisearch.HitTest(m.viewState(0), msg.X, msg.Y)
	if !ok {
		return nil
	}

	switch msg.Kind {
	case mode.MouseWheelUp:
		if hit.Pane == uisearch.FocusResults {
			return m.selectRow(m.selectedRow - 1)
		}
	case mode.MouseWheelDown:
		if hit.Pane == uisearch.FocusResults {
			return m.selectRow(m.selectedRow + 1)
		}
	case mode.MouseClick:
		return m.click(hit, msg)
	}
	return nil
}

func (m *Model) click(hit uisearch.Hit, msg mode.MouseMsg) tea.Cmd {
	target := ""
	if hit.Row >= 0 {
		target = m.page.Results[hit.Row].Issue.ID
	}
	if m.clicks.Double(target, msg) {
		return mode.RequestActionCmd(mode.Search, mode.ActionOpenDetail)
	}

	// Without results only the query box holds focus, as cycleFocus has it.
	if hit.Pane != uisearch.FocusQuery && !m.hasResults() {
		return nil
	}
	m.focus = hit.Pane
	if hit.Pane == uisearch.FocusMetadata {
		m.ensureMetadataSelection()
	}
	if target == "" {
		return nil
	}
	return m.selectRow(hit.Row)
}

// selectRow moves the result selection to row, as the move keys do.
func (m *Model) selectRow(row int) tea.Cmd {
	if !m.moveSelection(row - m.selectedRow) {
		return nil
	}
	m.selectedDetailLoading = true
	m.selectedDetail = domain.IssueDetail{}
	return m.selectionChangedCmd()
}

// hover is the result under the pointer, or nil.
func (m *Model) hover(state uisearch.State) *uisearch.Hit {
	if m.pointer == nil {
		return nil
	}
	hit, ok := uisearch.HitTest(state, m.pointer.x, m.pointer.y)
	if !ok || hit.Row < 0 {
		return nil
	}
	return &hit
}
