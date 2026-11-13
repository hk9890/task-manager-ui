package docs

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
	uidetail "github.com/hk9890/task-manager-ui/internal/ui/detail"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// previewWheelLines is how far a wheel notch scrolls the preview, as a notch
// scrolls help.
const previewWheelLines = 3

// docLoadedMsg carries the result of the read of one doc for the preview.
// asOf is the UpdatedAt of the row when the read started.
type docLoadedMsg struct {
	id     string
	asOf   time.Time
	detail domain.IssueDetail
	err    error
}

// docRead is a doc read for the preview, and the UpdatedAt its row had when
// the read started. The row, not the doc, is the clock: the list and the read
// may stamp the same change differently.
type docRead struct {
	detail domain.IssueDetail
	asOf   time.Time
}

// preview is the Preview pane: the docs read for it and where it is scrolled.
type preview struct {
	// read holds each doc read for the preview, by ID. A doc whose row has a
	// later UpdatedAt than its entry is read again.
	read map[string]docRead
	// failed is why the read of a doc failed, by ID, until it is read again.
	failed map[string]string
	// pending is the ID of each doc a read is in flight for.
	pending map[string]bool

	// offset is the scroll offset of the doc in shownID.
	offset  int
	shownID string
}

// Previewing reports whether the screen is wide enough to draw the preview,
// and the page keys so scroll it rather than page the list.
func (m *Model) Previewing() bool {
	_, _, ok := m.split()
	return ok
}

// split reports the width of the list and of the preview, and whether the
// screen is wide enough for both.
func (m *Model) split() (list, previewWidth int, ok bool) {
	return styles.SplitWidths(m.width)
}

// previewCmd reads the selected doc when the preview has no current copy of
// it. The read runs at every width, so the preview is ready when the screen
// grows wide enough to draw it.
func (m *Model) previewCmd() tea.Cmd {
	selection := m.currentSelection()
	if selection == nil {
		return nil
	}
	id := selection.Issue.ID
	if id != m.preview.shownID {
		m.preview.shownID = id
		m.preview.offset = 0
	}
	if m.preview.pending[id] || m.hasCurrent(selection.Issue) {
		return nil
	}
	if m.preview.pending == nil {
		m.preview.pending = map[string]bool{}
	}
	m.preview.pending[id] = true
	return loadDocCmd(m.ctx, m.repo, id, selection.Issue.UpdatedAt)
}

// hasCurrent reports whether the preview holds a read of row at least as new
// as the row.
func (m *Model) hasCurrent(row domain.IssueSummary) bool {
	if _, failed := m.preview.failed[row.ID]; failed {
		return true
	}
	doc, ok := m.preview.read[row.ID]
	return ok && !doc.asOf.Before(row.UpdatedAt)
}

// applyDoc stores a finished read. A doc that is no longer selected keeps its
// read for when it is selected again.
func (m *Model) applyDoc(msg docLoadedMsg) tea.Cmd {
	delete(m.preview.pending, msg.id)
	if msg.err != nil {
		if m.preview.failed == nil {
			m.preview.failed = map[string]string{}
		}
		m.preview.failed[msg.id] = msg.err.Error()
		return nil
	}
	if m.preview.read == nil {
		m.preview.read = map[string]docRead{}
	}
	delete(m.preview.failed, msg.id)
	m.preview.read[msg.id] = docRead{detail: msg.detail, asOf: msg.asOf}
	m.preview.offset = min(m.preview.offset, uidetail.PreviewMaxOffset(m.previewState(0)))
	// The row may have changed again while the read was in flight.
	return m.previewCmd()
}

// forgetFailures drops the failed reads, so a reload tries them again.
func (m *Model) forgetFailures() {
	m.preview.failed = nil
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
	if selection == nil {
		state.Empty = true
		return state
	}
	row := selection.Issue
	if reason, failed := m.preview.failed[row.ID]; failed {
		state.Detail.Summary = row
		state.Error = reason
		return state
	}
	if doc, ok := m.preview.read[row.ID]; ok {
		state.Detail = doc.detail
		return state
	}
	state.Detail.Summary = row
	state.Skeleton = true
	return state
}

// scrollPreview moves the preview by delta lines, held inside the doc.
func (m *Model) scrollPreview(delta int) {
	maxOffset := uidetail.PreviewMaxOffset(m.previewState(0))
	m.preview.offset = min(max(m.preview.offset+delta, 0), maxOffset)
}

// previewPage is the lines a page key scrolls the preview: the rows of the
// pane less the two the scroll marks may take.
func (m *Model) previewPage() int {
	return max(1, uiboard.ColumnHeight(m.height)-2-2)
}

// previewKey scrolls the preview on a page key of the detail context while
// the preview is drawn. handled is false for every other key and at every
// width that draws the list alone, where the page keys page the list.
func (m *Model) previewKey(msg tea.KeyPressMsg) (handled bool) {
	if _, _, ok := m.split(); !ok || m.currentSelection() == nil {
		return false
	}
	switch {
	case m.keys.Match(config.DetailContext, config.DetailActionPageDown, msg):
		m.scrollPreview(m.previewPage())
	case m.keys.Match(config.DetailContext, config.DetailActionPageUp, msg):
		m.scrollPreview(-m.previewPage())
	default:
		return false
	}
	return true
}

// overPreview reports whether msg is over the preview: right of the list and
// below the list head.
func (m *Model) overPreview(msg mode.MouseMsg) bool {
	list, _, ok := m.split()
	return ok && msg.X >= list && msg.Y >= m.height-uiboard.ColumnHeight(m.height)
}

func loadDocCmd(ctx context.Context, repo repository.Repository, id string, asOf time.Time) tea.Cmd {
	return func() tea.Msg {
		detail, err := repo.Issue(ctx, id)
		return docLoadedMsg{id: id, asOf: asOf, detail: detail, err: err}
	}
}
