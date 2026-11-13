package docs

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
	uidetail "github.com/hk9890/task-manager-ui/internal/ui/detail"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// previewWheelLines is how far a wheel notch scrolls the preview, as a notch
// scrolls the Content pane of Detail.
const previewWheelLines = 3

// preview is the Preview pane: the doc the shell last read and where it is
// scrolled. The shell reads the selected issue for Detail on every selection
// change, and hands each read here (ShowDoc, ShowDocFailure), so the tab reads
// nothing itself.
type preview struct {
	doc domain.IssueDetail
	// failedID is the doc whose last read failed, and failure why.
	failedID string
	failure  string

	// offset is the scroll offset of the doc in shownID.
	offset  int
	shownID string
}

// ShowDoc is the read of an issue the shell finished. The pane draws it while
// that issue is the selected doc.
func (m *Model) ShowDoc(doc domain.IssueDetail) {
	m.preview.doc = doc
	if m.preview.failedID == doc.Summary.ID {
		m.preview.failedID, m.preview.failure = "", ""
	}
	m.clampPreview()
}

// ShowDocFailure is a read of the issue id that failed. The pane draws the
// error while that issue is the selected doc, until a read of it lands.
func (m *Model) ShowDocFailure(id, failure string) {
	m.preview.failedID, m.preview.failure = id, failure
	m.clampPreview()
}

// Previewing reports whether the pane is drawn with a doc in it, and the page
// keys so scroll it rather than page the list.
func (m *Model) Previewing() bool {
	_, _, ok := m.split()
	return ok && m.currentSelection() != nil
}

// split reports the width of the list and of the preview, and whether the
// screen is wide enough for both.
func (m *Model) split() (list, previewWidth int, ok bool) {
	return styles.SplitWidths(m.width)
}

// followSelection opens a doc that was not on the pane before at its top.
func (m *Model) followSelection() {
	id := m.selectedIssueID()
	if id != m.preview.shownID {
		m.preview.shownID = id
		m.preview.offset = 0
	}
}

// previewState is the Preview pane as the renderer sees it.
func (m *Model) previewState(skeletonPhase int) uidetail.PreviewState {
	_, width, _ := m.split()
	state := uidetail.PreviewState{
		Width:         width,
		Height:        uiboard.ColumnHeight(m.height),
		ScrollOffset:  m.preview.offset,
		SkeletonPhase: skeletonPhase,
	}
	selection := m.currentSelection()
	switch {
	case selection == nil && m.loading:
		state.Skeleton = true
	case selection == nil:
		state.Empty = true
	case m.preview.failedID == selection.Issue.ID:
		state.Detail.Summary = selection.Issue
		state.Error = m.preview.failure
	case m.preview.doc.Summary.ID == selection.Issue.ID:
		state.Detail = m.preview.doc
	default:
		state.Detail.Summary = selection.Issue
		state.Skeleton = true
	}
	return state
}

// scrollPreview moves the preview by delta lines, held inside the doc.
func (m *Model) scrollPreview(delta int) {
	m.preview.offset += delta
	m.clampPreview()
}

// clampPreview holds the offset inside the doc as the pane draws it now. A
// screen too narrow for the pane leaves the offset as it is.
func (m *Model) clampPreview() {
	if _, _, ok := m.split(); !ok {
		return
	}
	m.preview.offset = textutil.Clamp(m.preview.offset, 0, uidetail.PreviewMaxOffset(m.previewState(0)))
}

// previewKey scrolls the preview on a page key of the board context while the
// preview is drawn: the docs tab reads every key from that context, so the
// reserved-key check of the configuration covers this one too. handled is
// false for every other key and at every width that draws the list alone,
// where the page keys page the list.
func (m *Model) previewKey(msg tea.KeyPressMsg) (handled bool) {
	if !m.Previewing() {
		return false
	}
	page := uidetail.PreviewPageRows(m.previewState(0))
	switch {
	case m.keys.Match(config.BoardContext, config.BoardActionPageDown, msg):
		m.scrollPreview(page)
	case m.keys.Match(config.BoardContext, config.BoardActionPageUp, msg):
		m.scrollPreview(-page)
	default:
		return false
	}
	return true
}
