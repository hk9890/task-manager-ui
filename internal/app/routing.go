package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
)

func (m *Model) forwardModeMessages(msg tea.Msg) tea.Cmd {
	boardCmd := m.forwardBoardMessage(msg)
	searchCmd := m.forwardSearchMessage(msg)
	return batchCmds(boardCmd, searchCmd)
}

func (m *Model) forwardBoardMessage(msg tea.Msg) tea.Cmd {
	if m.board == nil || !m.shouldForwardToBoard(msg) {
		return nil
	}
	return m.board.Update(msg)
}

func (m *Model) forwardSearchMessage(msg tea.Msg) tea.Cmd {
	if m.search == nil || !m.shouldForwardToSearch(msg) {
		return nil
	}
	return m.search.Update(msg)
}

func (m Model) shouldForwardToBoard(msg tea.Msg) bool {
	if _, isKey := msg.(tea.KeyMsg); isKey {
		return m.active == mode.Board
	}
	return true
}

func (m Model) shouldForwardToSearch(msg tea.Msg) bool {
	if _, isKey := msg.(tea.KeyMsg); isKey {
		return m.active == mode.Search
	}
	return true
}

func (m Model) shouldCaptureKeyForOverlay(msg tea.Msg) bool {
	if !m.showHelp && !m.showActionModal {
		return false
	}
	_, isKey := msg.(tea.KeyMsg)
	return isKey
}

// applyModeCycle switches the active mode to target while preserving the
// invariant that lastBrowse is always a browse mode (Board or Search) —
// currentSelection() and the Escape handler rely on it. Entering a browse mode
// sets lastBrowse to it; entering Detail captures the browse mode we came from
// (mirroring the explicit Detail handler) and otherwise leaves lastBrowse
// untouched. The previous code did `lastBrowse = active` unconditionally, so
// cycling into Detail (e.g. prevMode(Board) == Detail) set lastBrowse = Detail,
// which made currentSelection() return nil (blank/stuck Detail view) and turned
// Escape (active = lastBrowse) into a no-op.
func (m *Model) applyModeCycle(target mode.ID) {
	switch target {
	case mode.Board, mode.Search:
		m.active = target
		m.lastBrowse = target
	case mode.Detail:
		if m.active == mode.Board || m.active == mode.Search {
			m.lastBrowse = m.active
		}
		m.active = mode.Detail
	default:
		m.active = target
	}
}

func nextMode(current mode.ID, lastBrowse mode.ID) mode.ID {
	switch current {
	case mode.Board:
		return mode.Search
	case mode.Search:
		return mode.Board
	case mode.Detail:
		if lastBrowse == mode.Search {
			return mode.Board
		}
		return mode.Search
	default:
		return mode.Board
	}
}

func prevMode(current mode.ID, _ mode.ID) mode.ID {
	switch current {
	case mode.Board:
		return mode.Detail
	case mode.Search:
		return mode.Board
	case mode.Detail:
		return mode.Search
	default:
		return mode.Search
	}
}
