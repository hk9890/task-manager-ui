package detail

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/ui/detail"
)

// wheelLines is how far one wheel notch scrolls a pane of text. A list moves
// one row a notch; prose read a line at a time is slower than the keys.
const wheelLines = 3

// HandleMouse is the wheel, the pointer and the left button. The wheel scrolls
// the Content pane and moves the cursor of the Dependencies pane, as the
// scroll keys do in each. A click focuses the pane it lands on; on a
// reference row it also puts the cursor there, and a second click opens that
// issue: the command returned carries the OpenRelatedIssueMsg. A click on a
// comment puts the comment cursor there, and one on a button sends the request
// the button's key sends.
func (m *Model) HandleMouse(msg mode.MouseMsg, maxWidth, viewportHeight int) tea.Cmd {
	// Nothing is drawn without a viewport, so there is no cell to be over.
	if viewportHeight <= 0 {
		m.pointer = nil
		return nil
	}
	m.pointer = msg.Pointer()
	// A move only places the pointer: every draw reads the hover from it.
	if m.pointer == nil || msg.Kind == mode.MouseMove {
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
	m.ContentScrollOffset = applyScrollAction(m.ContentScrollOffset, bounds.Content, "", direction*wheelLines)
}

// buttonActions is the request each pane button sends: the one its key sends.
var buttonActions = map[detail.Button]mode.Action{
	detail.ButtonEdit:          mode.ActionEditIssue,
	detail.ButtonAddComment:    mode.ActionAddComment,
	detail.ButtonEditComment:   mode.ActionEditComment,
	detail.ButtonDeleteComment: mode.ActionDeleteComment,
}

// commentClick is what the click tracker holds after a click on a comment.
const commentClick = "comment"

func (m *Model) click(hit detail.Hit, msg mode.MouseMsg) tea.Cmd {
	if action, ok := buttonActions[hit.Button]; ok {
		// A click that selects a comment draws its buttons, and one of them
		// can stand under that click. The second click of a double click in
		// place means the comment, as it means the row in a list.
		if m.clicks.Double("", commentClick, msg) {
			return nil
		}
		return mode.RequestActionCmd(mode.Detail, action)
	}
	if hit.Pane == detail.FocusPaneComments {
		m.clicks.Double(commentClick, "", msg)
		m.FocusPane = hit.Pane
		m.commentIndex = hit.Comment
		return nil
	}
	if m.clicks.Double(hit.RefID, m.browserSelectedIssueID(), msg) {
		if ref, ok := m.selectedRelatedIssue(); ok {
			return openRelatedIssueCmd(ref)
		}
		return nil
	}

	m.FocusPane = hit.Pane
	if hit.RefID != "" {
		m.selectBrowserIssue(hit.RefID)
	}
	return nil
}

// hover is the reference row or the button under the pointer, or nil.
func (m *Model) hover(state detail.State) *detail.Hit {
	if m.pointer == nil {
		return nil
	}
	hit, ok := detail.HitTest(state, m.pointer.X, m.pointer.Y)
	if !ok || (hit.RefID == "" && hit.Button == detail.ButtonNone) {
		return nil
	}
	return &hit
}
