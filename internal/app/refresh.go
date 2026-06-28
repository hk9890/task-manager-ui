package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
)

func (m *Model) maybeAutoRefreshActiveSurfaceCmd() tea.Cmd {
	return m.maybeAutoRefreshActiveSurfaceCmdWithPolicy(false)
}

func (m *Model) maybeAutoRefreshActiveSurfaceCmdOnFocusRegain() tea.Cmd {
	return m.maybeAutoRefreshActiveSurfaceCmdWithPolicy(true)
}

func (m *Model) maybeAutoRefreshActiveSurfaceCmdWithPolicy(force bool) tea.Cmd {
	if m.showHelp || m.showActionModal {
		return nil
	}
	if m.focusKnown && !m.terminalFocused {
		return nil
	}
	if !force && !m.shouldRefreshSurface(m.active) {
		return nil
	}
	return m.refreshActiveSurfaceCmd()
}

func (m *Model) refreshActiveSurfaceCmd() tea.Cmd {
	switch m.active {
	case mode.Board:
		if m.boardIsLoading() {
			return nil
		}
		m.markSurfaceRefreshed(mode.Board)
		return m.board.AutoRefresh()
	case mode.Search:
		if m.searchIsLoading() {
			return nil
		}
		m.markSurfaceRefreshed(mode.Search)
		return m.search.AutoRefresh()
	case mode.Detail:
		if m.detail.Loading {
			return nil
		}
		selection := m.currentSelection()
		if selection == nil || selection.Issue.ID == "" {
			return nil
		}
		m.detail.SelectionID = selection.Issue.ID
		m.detail.SelectBrowserIssue(selection.Issue.ID)
		m.detail.Loading = true
		m.detail.Error = ""
		m.detail.TargetID = selection.Issue.ID
		m.markSurfaceRefreshed(mode.Detail)
		return loadDetailCmd(m.services, selection.Issue.ID)
	default:
		return nil
	}
}

func (m *Model) markBrowseSurfacesDirty() {
	m.markSurfaceDirty(mode.Board, mode.Search)
}

func (m *Model) markSurfaceDirty(surfaces ...mode.ID) {
	if m.refreshStateBySurface == nil {
		m.refreshStateBySurface = make(map[mode.ID]surfaceRefreshState)
	}
	for _, surface := range surfaces {
		state := m.refreshStateBySurface[surface]
		state.dirty = true
		m.refreshStateBySurface[surface] = state
	}
}

func (m *Model) markSurfaceRefreshed(surface mode.ID) {
	if m.refreshStateBySurface == nil {
		m.refreshStateBySurface = make(map[mode.ID]surfaceRefreshState)
	}
	state := m.refreshStateBySurface[surface]
	state.dirty = false
	state.lastRefresh = modelNow()
	m.refreshStateBySurface[surface] = state
}

func (m *Model) shouldRefreshSurface(surface mode.ID) bool {
	state, ok := m.refreshStateBySurface[surface]
	if !ok {
		return true
	}
	if state.dirty {
		return true
	}
	if state.lastRefresh.IsZero() {
		return true
	}
	return modelNow().Sub(state.lastRefresh) >= refreshTickInterval
}
