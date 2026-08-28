package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
)

func (m *Model) forwardModeMessages(msg tea.Msg) tea.Cmd {
	boardCmd := m.forwardBoardMessage(msg)
	docsCmd := m.forwardDocsMessage(msg)
	searchCmd := m.forwardSearchMessage(msg)
	return batchCmds(boardCmd, docsCmd, searchCmd)
}

func (m *Model) forwardBoardMessage(msg tea.Msg) tea.Cmd {
	if m.board == nil || !m.shouldForwardToBoard(msg) {
		return nil
	}
	return m.board.Update(msg)
}

func (m *Model) forwardDocsMessage(msg tea.Msg) tea.Cmd {
	if m.docs == nil || !m.shouldForwardToDocs(msg) {
		return nil
	}
	return m.docs.Update(msg)
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

func (m Model) shouldForwardToDocs(msg tea.Msg) bool {
	if _, isKey := msg.(tea.KeyMsg); isKey {
		return m.active == mode.Docs
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
	switch {
	case mode.IsBrowse(target):
		m.active = target
		m.lastBrowse = target
	case target == mode.Detail:
		if mode.IsBrowse(m.active) {
			m.lastBrowse = m.active
		}
		m.active = mode.Detail
	default:
		m.active = target
	}
}

// nextMode and prevMode cycle the header tab strip — Board, Docs, Search — in
// mode.BrowseModes order. Detail is not a tab: it is a drill-in, so cycling
// from it steps off the tab lastBrowse points at rather than staying in Detail.
func nextMode(current mode.ID, lastBrowse mode.ID) mode.ID {
	return cycleBrowse(current, lastBrowse, 1)
}

func prevMode(current mode.ID, lastBrowse mode.ID) mode.ID {
	return cycleBrowse(current, lastBrowse, -1)
}

func cycleBrowse(current mode.ID, lastBrowse mode.ID, delta int) mode.ID {
	tabs := mode.BrowseModes

	from := current
	if !mode.IsBrowse(from) {
		from = lastBrowse
	}
	if !mode.IsBrowse(from) {
		from = mode.Board
	}

	idx := 0
	for i, tab := range tabs {
		if tab == from {
			idx = i
			break
		}
	}

	next := (idx + delta) % len(tabs)
	if next < 0 {
		next += len(tabs)
	}
	return tabs[next]
}
