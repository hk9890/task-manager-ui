package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/mode"
)

// helpWheelLines is how far one wheel notch scrolls the help overlay.
const helpWheelLines = 3

// mouseKind maps a Bubble Tea mouse event onto the few the surfaces act on:
// the wheel, the pointer moving, and a press of the left button. A drag is a
// move, and a release or another button is nothing.
func mouseKind(msg tea.MouseMsg) (mode.MouseKind, bool) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		return mode.MouseWheelUp, true
	case msg.Button == tea.MouseButtonWheelDown:
		return mode.MouseWheelDown, true
	case msg.Action == tea.MouseActionMotion:
		return mode.MouseMove, true
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		return mode.MouseClick, true
	}
	return 0, false
}

// handleMouse routes one mouse event to whatever is drawn under it, in the
// order the keyboard is routed: an overlay first, then a surface above the
// shell, then the tab strip and the active surface.
//
// An open overlay holds the screen as it holds the keyboard, so nothing below
// it sees the event — the help overlay scrolls under the wheel and a dialog
// ignores the mouse. Each surface gets the event in its own coordinates.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	kind, ok := mouseKind(msg)
	if !ok {
		return m, nil
	}
	event := mode.MouseMsg{Kind: kind, X: msg.X, Y: msg.Y, At: modelNow()}
	leave := mode.MouseMsg{Kind: mode.MouseLeave, At: event.At}
	m.leaveHiddenSurfaces(leave)

	// A click or a wheel notch cancels a dialog still loading, as a key does:
	// it can move the selection the dialog was asked for.
	if kind != mode.MouseMove {
		m.pendingDialog = pendingDialogGuard{}
	}

	if m.showHelp || m.showActionModal {
		m.hoverTab = ""
		cmd := m.mouseToSurface(leave)
		switch {
		case !m.showHelp:
		case kind == mode.MouseWheelUp:
			m.help = m.help.Scroll(-helpWheelLines)
		case kind == mode.MouseWheelDown:
			m.help = m.help.Scroll(helpWheelLines)
		}
		return m, cmd
	}

	if m.active == mode.StorePicker {
		m.hoverTab = ""
		return m, m.storePicker.Update(event)
	}

	headerHeight := lipgloss.Height(m.renderHeader())
	_, workspaceHeight := m.workspaceSize()
	switch {
	case event.Y < headerHeight:
		cmd := m.mouseToSurface(leave)
		return m, batchCmds(cmd, m.mouseOnHeader(event))
	case event.Y >= headerHeight+workspaceHeight:
		m.hoverTab = ""
		return m, m.mouseToSurface(leave)
	}

	m.hoverTab = ""
	event.Y -= headerHeight
	return m, m.mouseToSurface(event)
}

// leaveHiddenSurfaces tells every surface the shell is not drawing that the
// pointer left it. A surface keeps its pointer cell while a key takes the
// shell to another one, and would mark a row under a cell the pointer is long
// gone from when it is drawn again.
func (m *Model) leaveHiddenSurfaces(leave mode.MouseMsg) {
	if m.active != mode.StorePicker {
		m.storePicker.Update(leave)
	}
	if m.active != mode.Detail {
		m.detail.HandleMouse(leave, 0, 0)
	}
	for _, entry := range m.browseTabs() {
		if entry.ID != m.active {
			entry.Tab.Update(leave)
		}
	}
}

// mouseToSurface hands event to the surface the shell is drawing.
func (m *Model) mouseToSurface(event mode.MouseMsg) tea.Cmd {
	switch m.active {
	case mode.StorePicker:
		return m.storePicker.Update(event)
	case mode.Detail:
		intent := m.detail.HandleMouse(event, m.detailViewportWidth(), m.detailViewportHeight())
		if intent == nil {
			return nil
		}
		return m.drillInto(*intent)
	}
	// Board is the shell's home tab, so an unknown active mode draws it
	// (renderBody) and the mouse follows what is drawn.
	if tab := m.browseController(m.active); tab != nil {
		return m.scoped(tab.Update(event))
	}
	return m.scoped(m.board.Update(event))
}

// mouseOnHeader lights the tab under the pointer and switches to it on a
// click.
func (m *Model) mouseOnHeader(event mode.MouseMsg) tea.Cmd {
	tab, ok := m.tabAt(event.X)
	if !ok {
		m.hoverTab = ""
		return nil
	}
	m.hoverTab = tab
	if event.Kind != mode.MouseClick || (tab == m.active) {
		return nil
	}
	return m.switchToTab(tab)
}

// tabAt is the browse tab the header strip draws at column x.
func (m Model) tabAt(x int) (mode.ID, bool) {
	left := headerTabsStart()
	for _, id := range mode.BrowseModes {
		width := lipgloss.Width(tabLabels[id]) + 2*headerTabPadding
		if x >= left && x < left+width {
			return id, true
		}
		left += width + headerTabGap
	}
	return "", false
}
