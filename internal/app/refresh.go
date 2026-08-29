package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/mode/detail"
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
	if m.active == mode.Detail {
		if m.detail.IsLoading() {
			return nil
		}
		return m.reloadDetailCmd()
	}

	tab := m.browseController(m.active)
	if tab == nil || tab.IsLoading() {
		return nil
	}
	m.markSurfaceRefreshed(m.active)
	return tab.AutoRefresh()
}

// reloadDetailCmd issues a detail load for the current selection and marks the
// surface refreshed. It is the path the explicit reload key takes.
//
// The reload key used to set detail.Loading and then call
// ensureDetailForCurrentSelectionCmd, whose first guard returns nil for a
// target that is already loading — so the key issued no load at all and left
// the header claiming "Loading: detail" for the rest of the session.
func (m *Model) reloadDetailCmd() tea.Cmd {
	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		return nil
	}
	m.detail.BeginLoad(selection.Issue.ID, detail.BeginLoadOptions{})
	m.markSurfaceRefreshed(mode.Detail)
	return loadDetailCmd(m.ctx, m.services, selection.Issue.ID)
}

func (m *Model) markBrowseSurfacesDirty() {
	m.markSurfaceDirty(mode.BrowseModes...)
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
