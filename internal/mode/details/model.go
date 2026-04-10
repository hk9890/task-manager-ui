package details

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
)

// Model is the shell-owned standalone detail presentation state.
type Model struct {
	SelectionID string
	TargetID    string
	Detail      domain.IssueDetail
	Loading     bool
	Error       string

	ScrollOffset int
}

// View renders the detail surface for pane and dedicated detail mode.
func (m Model) View(maxWidth, viewportHeight int, compact bool) string {
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
		return content
	}

	lines := strings.Split(content, "\n")
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
func (m *Model) ClampScroll(viewportHeight int) {
	total := len(strings.Split(uidetails.Render(uidetails.State{
		SelectionID: m.SelectionID,
		TargetID:    m.TargetID,
		Detail:      m.Detail,
		Loading:     m.Loading,
		Error:       m.Error,
		Compact:     false,
	}), "\n"))
	maxOffset := maxScrollOffset(total, viewportHeight)
	if m.ScrollOffset < 0 {
		m.ScrollOffset = 0
	}
	if m.ScrollOffset > maxOffset {
		m.ScrollOffset = maxOffset
	}
}

// HandleKey updates detail-mode scroll state and reports whether it consumed the key.
func (m *Model) HandleKey(msg tea.KeyMsg, viewportHeight int) bool {
	if viewportHeight <= 0 {
		return false
	}

	total := len(strings.Split(uidetails.Render(uidetails.State{
		SelectionID: m.SelectionID,
		TargetID:    m.TargetID,
		Detail:      m.Detail,
		Loading:     m.Loading,
		Error:       m.Error,
		Compact:     false,
	}), "\n"))
	maxOffset := maxScrollOffset(total, viewportHeight)

	move := 0
	switch msg.Type {
	case tea.KeyUp:
		move = -1
	case tea.KeyDown:
		move = 1
	case tea.KeyPgUp:
		move = -maxInt(1, viewportHeight-1)
	case tea.KeyPgDown:
		move = maxInt(1, viewportHeight-1)
	case tea.KeyHome:
		m.ScrollOffset = 0
		return true
	case tea.KeyEnd:
		m.ScrollOffset = maxOffset
		return true
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "j":
			move = 1
		case "k":
			move = -1
		default:
			return false
		}
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
