package details

import (
	"reflect"
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
func (m *Model) HandleKey(msg tea.KeyMsg, maxWidth, viewportHeight int) bool {
	if viewportHeight <= 0 {
		return false
	}
	if m.cachedViewportHeight != 0 && m.cachedViewportHeight != viewportHeight {
		m.invalidateLineCountCache()
	}
	if m.Keys.IsZero() {
		resolved, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
		if err == nil {
			m.Keys = resolved
		}
	}

	total := m.lineCountForScroll(maxWidth)
	maxOffset := maxScrollOffset(total, viewportHeight)

	move := 0
	switch {
	case m.Keys.Match(config.DetailContext, config.DetailActionScrollUp, msg):
		move = -1
	case m.Keys.Match(config.DetailContext, config.DetailActionScrollDown, msg):
		move = 1
	case m.Keys.Match(config.DetailContext, config.DetailActionPageUp, msg):
		move = -max(1, viewportHeight-1)
	case m.Keys.Match(config.DetailContext, config.DetailActionPageDown, msg):
		move = max(1, viewportHeight-1)
	case m.Keys.Match(config.DetailContext, config.DetailActionHome, msg):
		m.ScrollOffset = 0
		return true
	case m.Keys.Match(config.DetailContext, config.DetailActionEnd, msg):
		m.ScrollOffset = maxOffset
		return true
	default:
		return false
	}

	if move == 0 {
		return false
	}

	next := m.ScrollOffset + move
	if next < 0 {
		next = 0
	}
	if next > maxOffset {
		next = maxOffset
	}
	m.ScrollOffset = next
	return true
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
