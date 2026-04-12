package details

import (
	"reflect"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
)

const defaultLineCountWidth = 80

// Model is the shell-owned standalone detail presentation state.
type Model struct {
	SelectionID string
	TargetID    string
	Detail      domain.IssueDetail
	Loading     bool
	Error       string
	Keys        config.ResolvedKeyBindings
	FocusPane   uidetails.FocusPane

	SelectedRelatedIndex int

	ScrollOffset int

	cachedLineCount      int
	cachedLineCountWidth int
	cachedViewportHeight int
	cachedSelectionID    string
	cachedTargetID       string
	cachedDetail         domain.IssueDetail
	cachedLoading        bool
	cachedError          string
}

// OpenRelatedIssueIntent requests shell-level navigation to another issue from
// dedicated detail mode.
type OpenRelatedIssueIntent struct {
	IssueID string
}

// View renders the detail surface for pane and dedicated detail mode.
func (m *Model) View(maxWidth, viewportHeight int, compact bool) string {
	content := uidetails.Render(uidetails.State{
		SelectionID: m.SelectionID,
		TargetID:    m.TargetID,
		Detail:      m.Detail,
		Loading:     m.Loading,
		Error:       m.Error,
		Width:       maxWidth,
		Compact:     compact,
		FocusPane:   m.focusPane(),
	})

	if compact || viewportHeight <= 0 {
		if compact {
			m.invalidateLineCountCache()
		}
		return content
	}

	lines := strings.Split(content, "\n")
	m.setLineCountCache(len(lines), maxWidth, viewportHeight)
	maxOffset := maxScrollOffset(len(lines), viewportHeight)
	offset := m.ScrollOffset
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	start := offset
	end := start + viewportHeight
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[start:end], "\n")
}

// ClampScroll keeps scroll offset inside current content bounds.
func (m *Model) ClampScroll(maxWidth, viewportHeight int) {
	if m.cachedViewportHeight != 0 && m.cachedViewportHeight != viewportHeight {
		m.invalidateLineCountCache()
	}
	total := m.lineCountForScroll(maxWidth)
	maxOffset := maxScrollOffset(total, viewportHeight)
	if m.ScrollOffset < 0 {
		m.ScrollOffset = 0
	}
	if m.ScrollOffset > maxOffset {
		m.ScrollOffset = maxOffset
	}
}

// HandleKey updates detail-mode scroll state and reports whether it consumed the key.
func (m *Model) HandleKey(msg tea.KeyMsg, maxWidth, viewportHeight int) (bool, *OpenRelatedIssueIntent) {
	if viewportHeight <= 0 {
		return false, nil
	}
	if m.cachedViewportHeight != 0 && m.cachedViewportHeight != viewportHeight {
		m.invalidateLineCountCache()
	}
	m.normalizeRelatedSelection()
	if m.Keys.IsZero() {
		resolved, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
		if err == nil {
			m.Keys = resolved
		}
	}

	switch msg.Type {
	case tea.KeyLeft:
		m.moveFocusLeft()
		return true, nil
	case tea.KeyRight:
		m.moveFocusRight()
		return true, nil
	}

	if msg.Type == tea.KeyEnter && m.focusPane() == uidetails.FocusPaneRelated {
		if ref, ok := m.selectedRelatedIssue(); ok {
			return true, &OpenRelatedIssueIntent{IssueID: ref.ID}
		}
		return true, nil
	}

	total := m.lineCountForScroll(maxWidth)
	maxOffset := maxScrollOffset(total, viewportHeight)

	move := 0
	action := ""
	switch {
	case m.Keys.Match(config.DetailContext, config.DetailActionScrollUp, msg):
		action = config.DetailActionScrollUp
		move = -1
	case m.Keys.Match(config.DetailContext, config.DetailActionScrollDown, msg):
		action = config.DetailActionScrollDown
		move = 1
	case m.Keys.Match(config.DetailContext, config.DetailActionPageUp, msg):
		action = config.DetailActionPageUp
		move = -max(1, viewportHeight-1)
	case m.Keys.Match(config.DetailContext, config.DetailActionPageDown, msg):
		action = config.DetailActionPageDown
		move = max(1, viewportHeight-1)
	case m.Keys.Match(config.DetailContext, config.DetailActionHome, msg):
		action = config.DetailActionHome
		if m.focusPane() != uidetails.FocusPaneContent {
			return true, nil
		}
		m.ScrollOffset = 0
		return true, nil
	case m.Keys.Match(config.DetailContext, config.DetailActionEnd, msg):
		action = config.DetailActionEnd
		if m.focusPane() != uidetails.FocusPaneContent {
			return true, nil
		}
		m.ScrollOffset = maxOffset
		return true, nil
	default:
		return false, nil
	}

	if m.focusPane() == uidetails.FocusPaneRelated {
		switch action {
		case config.DetailActionScrollUp:
			m.moveRelatedSelection(-1)
			return true, nil
		case config.DetailActionScrollDown:
			m.moveRelatedSelection(1)
			return true, nil
		default:
			return true, nil
		}
	}

	if m.focusPane() == uidetails.FocusPaneMetadata {
		return true, nil
	}

	if move == 0 {
		return false, nil
	}

	next := m.ScrollOffset + move
	if next < 0 {
		next = 0
	}
	if next > maxOffset {
		next = maxOffset
	}
	m.ScrollOffset = next
	return true, nil
}

func (m *Model) focusPane() uidetails.FocusPane {
	switch m.FocusPane {
	case uidetails.FocusPaneRelated, uidetails.FocusPaneContent, uidetails.FocusPaneMetadata:
		return m.FocusPane
	default:
		return uidetails.FocusPaneContent
	}
}

func (m *Model) moveFocusLeft() {
	switch m.focusPane() {
	case uidetails.FocusPaneMetadata:
		m.FocusPane = uidetails.FocusPaneContent
	case uidetails.FocusPaneContent:
		m.FocusPane = uidetails.FocusPaneRelated
	}
}

func (m *Model) moveFocusRight() {
	switch m.focusPane() {
	case uidetails.FocusPaneRelated:
		m.FocusPane = uidetails.FocusPaneContent
	case uidetails.FocusPaneContent:
		m.FocusPane = uidetails.FocusPaneMetadata
	}
}

func (m *Model) moveRelatedSelection(delta int) {
	refs := m.relatedIssues()
	if len(refs) == 0 {
		m.SelectedRelatedIndex = -1
		return
	}
	m.normalizeRelatedSelection()
	next := m.SelectedRelatedIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= len(refs) {
		next = len(refs) - 1
	}
	m.SelectedRelatedIndex = next
}

func (m *Model) selectedRelatedIssue() (domain.IssueReference, bool) {
	refs := m.relatedIssues()
	if len(refs) == 0 {
		return domain.IssueReference{}, false
	}
	m.normalizeRelatedSelection()
	if m.SelectedRelatedIndex < 0 || m.SelectedRelatedIndex >= len(refs) {
		return domain.IssueReference{}, false
	}
	return refs[m.SelectedRelatedIndex], true
}

func (m *Model) normalizeRelatedSelection() {
	refs := m.relatedIssues()
	if len(refs) == 0 {
		m.SelectedRelatedIndex = -1
		return
	}
	if m.SelectedRelatedIndex < 0 {
		m.SelectedRelatedIndex = 0
	}
	if m.SelectedRelatedIndex >= len(refs) {
		m.SelectedRelatedIndex = len(refs) - 1
	}
}

func (m *Model) relatedIssues() []domain.IssueReference {
	out := make([]domain.IssueReference, 0, len(m.Detail.BlockedBy)+len(m.Detail.Blocks)+len(m.Detail.Related))
	out = append(out, orderedReferencesByID(m.Detail.BlockedBy)...)
	out = append(out, orderedReferencesByID(m.Detail.Blocks)...)
	out = append(out, orderedReferencesByID(m.Detail.Related)...)
	return out
}

func orderedReferencesByID(refs []domain.IssueReference) []domain.IssueReference {
	ordered := append([]domain.IssueReference(nil), refs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

func maxScrollOffset(totalLines, viewportHeight int) int {
	if viewportHeight <= 0 || totalLines <= viewportHeight {
		return 0
	}
	return totalLines - viewportHeight
}

func (m *Model) lineCountForScroll(width int) int {
	if m.cachedLineCount > 0 && m.cacheMatchesCurrentState(width) {
		return m.cachedLineCount
	}

	if width <= 0 {
		width = defaultLineCountWidth
	}

	content := uidetails.Render(uidetails.State{
		SelectionID: m.SelectionID,
		TargetID:    m.TargetID,
		Detail:      m.Detail,
		Loading:     m.Loading,
		Error:       m.Error,
		Width:       width,
		Compact:     false,
	})
	total := len(strings.Split(content, "\n"))
	m.setLineCountCache(total, width, m.cachedViewportHeight)
	return total
}

func (m *Model) cacheMatchesCurrentState(width int) bool {
	if width <= 0 {
		width = defaultLineCountWidth
	}

	return m.SelectionID == m.cachedSelectionID &&
		m.TargetID == m.cachedTargetID &&
		m.Loading == m.cachedLoading &&
		m.Error == m.cachedError &&
		width == m.cachedLineCountWidth &&
		reflect.DeepEqual(m.Detail, m.cachedDetail)
}

func (m *Model) setLineCountCache(total, width, viewportHeight int) {
	m.cachedLineCount = total
	m.cachedLineCountWidth = width
	m.cachedViewportHeight = viewportHeight
	m.cachedSelectionID = m.SelectionID
	m.cachedTargetID = m.TargetID
	m.cachedDetail = m.Detail
	m.cachedLoading = m.Loading
	m.cachedError = m.Error
}

func (m *Model) invalidateLineCountCache() {
	m.cachedLineCount = 0
	m.cachedLineCountWidth = 0
	m.cachedViewportHeight = 0
}
