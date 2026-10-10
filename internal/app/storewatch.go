package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// watchStore subscribes to the changes other processes make to the store. It
// returns nil when the backend reports none, when the watch cannot start, or
// under --no-auto-refresh; the refresh tick is then the only trigger.
func (m *Model) watchStore(repo repository.Repository) <-chan struct{} {
	if m.runtime.DisableAutoRefresh {
		return nil
	}
	watcher, ok := repo.(repository.ChangeWatcher)
	if !ok {
		return nil
	}
	changes, err := watcher.WatchChanges(m.ctx)
	if err != nil {
		m.logger().Warn("store change watch not started; the refresh tick is the only trigger", "error", err.Error())
		return nil
	}
	return changes
}

// waitForStoreChangeCmd waits for the store to change or its watch to end. It
// re-arms only from the storeChangedMsg handler, and it is scoped: the watch of
// a store that was switched away ends with that store's context, and the stale
// epoch drops the message instead of ending the new store's chain.
func (m Model) waitForStoreChangeCmd() tea.Cmd {
	changes := m.storeChanges
	if changes == nil {
		return nil
	}
	return m.scoped(m.awaitStoreChange(changes))
}

// storeWatched reports whether a watch signals the changes of the active store,
// the writes of this process included. The watch then owns every reload that
// follows a write: a handler that reloaded for its own write would read the
// store a second time when the signal for that write arrives.
func (m Model) storeWatched() bool {
	return m.storeChanges != nil
}

// refreshAfterStoreChangeCmd reloads the active surface when the store changed
// after that surface's latest load started, or when a write of this process
// left it dirty. Update runs it after every message, so a change that arrives
// during a load, under an overlay or while another surface is active is applied
// as soon as the surface can take it. The terminal focus does not gate it: a
// board in view while an agent writes in another pane is the case the watch
// exists for.
func (m *Model) refreshAfterStoreChangeCmd() tea.Cmd {
	if m.overlayOpen() {
		return nil
	}
	owed := m.behindStore(m.active) || m.refreshStateBySurface[m.active].dirty
	if !owed || m.surfaceLoading(m.active) {
		return nil
	}
	cmd := m.refreshActiveSurfaceCmd()
	if cmd == nil {
		// The surface cannot take the change yet, such as a search whose query
		// is being typed. The change stays owed and the next message asks again.
		return nil
	}
	// Recorded here and not only by trackSurfaceLoads, which sees no load start
	// when this load follows the previous one within a single update.
	state := m.refreshStateBySurface[m.active]
	state.loadedAtChange = m.storeChangeSeq
	m.refreshStateBySurface[m.active] = state
	return cmd
}

// behindStore reports whether the store changed after the latest load of
// surface started. A surface the shell does not track is never behind.
func (m *Model) behindStore(surface mode.ID) bool {
	state, tracked := m.refreshStateBySurface[surface]
	return tracked && state.loadedAtChange != m.storeChangeSeq
}

// trackSurfaceLoads records, for each surface whose load started in this
// update, the store change it started after, and that the load reads what a
// write of this process left the surface dirty for. Every path that starts a
// load is covered, the reload keys the modes handle themselves included.
func (m *Model) trackSurfaceLoads() {
	for surface, state := range m.refreshStateBySurface {
		loading := m.surfaceLoading(surface)
		if loading && !state.loading {
			state.loadedAtChange = m.storeChangeSeq
			state.dirty = false
		}
		state.loading = loading
		m.refreshStateBySurface[surface] = state
	}
}

func (m *Model) surfaceLoading(surface mode.ID) bool {
	if surface == mode.Detail {
		return m.detail.IsLoading()
	}
	tab := m.browseController(surface)
	return tab != nil && tab.IsLoading()
}
