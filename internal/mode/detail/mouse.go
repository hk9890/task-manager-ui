package detail

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/ui/detail"
)

// wheelLines is how far one wheel notch scrolls a pane of text. A list moves
// one row a notch; prose read a line at a time is slower than the keys.
const wheelLines = 3

// HandleMouse is the wheel, the pointer and the left button. The wheel scrolls
// the text pane it is over and moves the cursor of the Dependencies pane, as
// the scroll keys do in each. A click focuses the pane it lands on; on a
// reference row it also puts the cursor there, and a second click opens that
// issue: the command returned carries the OpenRelatedIssueMsg.
func (m *Model) HandleMouse(msg mode.MouseMsg, maxWidth, viewportHeight int) tea.Cmd {
	// Nothing is drawn without a viewport, so there is no cell to be over.
	if viewportHeight <= 0 {
		m.pointer = nil
		return nil
	}
	m.pointer = msg.Pointer()
	if m.pointer == nil {
		return nil
	}

	hit, ok := detail.HitTest(m.viewState(maxWidth, viewportHeight, 0), msg.X, msg.Y)
	if !ok {
		return nil
	}

	switch msg.Kind {
	case mode.MouseWheelUp:
		m.wheel(hit.Pane, -1, maxWidth, viewportHeight)
	case mode.MouseWheelDown:
		m.wheel(hit.Pane, 1, maxWidth, viewportHeight)
	case mode.MouseClick:
		return m.click(hit, msg)
	}
	return nil
}

func (m *Model) wheel(pane detail.FocusPane, direction, maxWidth, viewportHeight int) {
	if pane == detail.FocusPaneDependencies {
		m.moveRelatedSelection(direction, maxWidth, viewportHeight)
		return
	}

	bounds := m.paneGeometry(maxWidth, viewportHeight)
	if pane == detail.FocusPaneMetadata {
		m.MetadataScrollOffset = applyScrollAction(m.MetadataScrollOffset, bounds.Metadata, "", direction*wheelLines)
		return
	}
	m.ContentScrollOffset = applyScrollAction(m.ContentScrollOffset, bounds.Content, "", direction*wheelLines)
}

func (m *Model) click(hit detail.Hit, msg mode.MouseMsg) tea.Cmd {
	if m.clicks.Double(hit.RefID, m.browserSelectedIssueID(), msg) {
		if ref, ok := m.selectedRelatedIssue(); ok {
			return openRelatedIssueCmd(ref)
		}
		return nil
	}

	m.FocusPane = hit.Pane
	if hit.Pane == detail.FocusPaneMetadata {
		m.ensureMetadataSelection()
	}
	if hit.RefID != "" {
		m.selectBrowserIssue(hit.RefID)
	}
	return nil
}

// hover is the reference row under the pointer, or nil.
func (m *Model) hover(state detail.State) *detail.Hit {
	if m.pointer == nil {
		return nil
	}
	hit, ok := detail.HitTest(state, m.pointer.X, m.pointer.Y)
	if !ok || hit.RefID == "" {
		return nil
	}
	return &hit
}
