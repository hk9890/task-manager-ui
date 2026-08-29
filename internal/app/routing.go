package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
)

// browseTab pairs a browse mode with its controller.
type browseTab struct {
	ID  mode.ID
	Tab mode.Browse
}

// browseTabs returns the browse controllers in BrowseModes order. Registering a
// tab is what wires it into forwarding, sizing, loading state and auto-refresh
// at once; before the mode.Browse interface existed the shell hand-wrote each of
// those once per tab, at eight sites in all.
//
// A tab the shell has not constructed yet is skipped rather than dereferenced.
func (m *Model) browseTabs() []browseTab {
	out := make([]browseTab, 0, len(mode.BrowseModes))
	for _, id := range mode.BrowseModes {
		if tab := m.browseController(id); tab != nil {
			out = append(out, browseTab{ID: id, Tab: tab})
		}
	}
	return out
}

// browseController returns the controller for id, or nil when there is none. The
// typed-nil check matters: a nil *board.Model stored in a mode.Browse is a
// non-nil interface, which would turn a skipped tab into a panic.
func (m *Model) browseController(id mode.ID) mode.Browse {
	switch id {
	case mode.Board:
		if m.board == nil {
			return nil
		}
		return m.board
	case mode.Docs:
		if m.docs == nil {
			return nil
		}
		return m.docs
	case mode.Search:
		if m.search == nil {
			return nil
		}
		return m.search
	}
	return nil
}

func (m *Model) forwardModeMessages(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(mode.BrowseModes))
	for _, entry := range m.browseTabs() {
		if !m.shouldForwardTo(entry.ID, msg) {
			continue
		}
		cmds = append(cmds, entry.Tab.Update(msg))
	}
	return batchCmds(cmds...)
}

// shouldForwardTo reports whether msg goes to the tab with this id. A key
// belongs to the active tab alone; everything else reaches all of them.
func (m Model) shouldForwardTo(id mode.ID, msg tea.Msg) bool {
	if _, isKey := msg.(tea.KeyMsg); isKey {
		return m.active == id
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
// Cycling also drops any drill-in: the tab strip returns the operator to a
// browse row, and that row is the selection every shell action then uses.
func (m *Model) applyModeCycle(target mode.ID) {
	m.clearDrillSelection()
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
