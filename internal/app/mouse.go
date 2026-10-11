package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/ui/helpscreen"
)

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
// shell, then the header and the active surface.
//
// An open overlay holds the screen as it holds the keyboard, so nothing below
// it sees the event — the help screen scrolls under the wheel and a dialog
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

	// Nothing is drawn on a cell of the margin, so the pointer there is on
	// nothing. The picker and help take the wheel without a hit test, and
	// would move under it.
	if msg.X < 0 || msg.X >= m.width || msg.Y < 0 || msg.Y >= m.height {
		m.clearHeaderHover()
		return m, m.mouseToSurface(leave)
	}

	if m.overlayOpen() {
		m.clearHeaderHover()
		cmd := m.mouseToSurface(leave)
		// A notch moves no further than a page, or a body of a few rows
		// would skip lines.
		notch := min(helpWheelLines, helpscreen.PageRows(m.height))
		switch {
		case !m.showHelp:
		case kind == mode.MouseWheelUp:
			m.scrollHelpBy(-notch)
		case kind == mode.MouseWheelDown:
			m.scrollHelpBy(notch)
		}
		return m, cmd
	}

	if m.active == mode.StorePicker {
		m.clearHeaderHover()
		return m, m.storePicker.Update(event)
	}
	// The configuration screen does nothing on the mouse, and the header is
	// not drawn while it is up.
	if m.active == mode.Config {
		m.clearHeaderHover()
		return m, nil
	}

	headerHeight := lipgloss.Height(m.renderHeader())
	_, workspaceHeight := m.workspaceSize()
	switch {
	case event.Y < headerHeight:
		cmd := m.mouseToSurface(leave)
		return m, batchCmds(cmd, m.mouseOnHeader(event))
	case event.Y >= headerHeight+workspaceHeight:
		m.clearHeaderHover()
		return m, m.mouseToSurface(leave)
	}

	m.clearHeaderHover()
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
	case mode.Config:
		return nil
	case mode.Detail:
		// One measure for both: each viewport getter renders the header and
		// the footer to take theirs, and the pointer asks on every cell.
		width, height := m.workspaceSize()
		return m.detail.HandleMouse(event, width, height)
	}
	// Board is the shell's home tab, so an unknown active mode draws it
	// (renderBody) and the mouse follows what is drawn.
	if tab := m.browseController(m.active); tab != nil {
		return m.scoped(tab.Update(event))
	}
	return m.scoped(m.board.Update(event))
}

// clearHeaderHover unlights the tab or the button the pointer was on.
func (m *Model) clearHeaderHover() {
	m.hoverTab = ""
	m.barPointer = nil
}

// mouseOnHeader lights the tab or the button under the pointer, and on a click
// switches to the tab or runs the button's action. The menu bar and the tab
// line answer: the rule between them draws nothing to press.
func (m *Model) mouseOnHeader(event mode.MouseMsg) tea.Cmd {
	m.clearHeaderHover()
	switch event.Y {
	case headerMenuRow:
		return m.mouseOnMenuBar(event)
	case headerTabsRow:
		return m.mouseOnTabs(event)
	}
	return nil
}

func (m *Model) mouseOnTabs(event mode.MouseMsg) tea.Cmd {
	if tab, ok := m.tabAt(event.X); ok {
		m.hoverTab = tab
		if event.Kind != mode.MouseClick || (tab == m.active) {
			return nil
		}
		return m.switchToTab(tab)
	}
	return nil
}

func (m *Model) mouseOnMenuBar(event mode.MouseMsg) tea.Cmd {
	m.barPointer = &event.X
	if cell, ok := m.buttonAt(event.X); ok && event.Kind == mode.MouseClick {
		return cell.action.run(m)
	}
	return nil
}

// buttonAt is the button the menu bar draws at column x.
func (m Model) buttonAt(x int) (barCell, bool) {
	for _, cell := range m.barCells() {
		if cell.covers(x) {
			return cell, true
		}
	}
	return barCell{}, false
}

// tabAt is the browse tab the tab line draws at column x.
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
