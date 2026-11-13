// Package agents renders the Agents tab: the agents of the store's project in
// two groups, those linked to an issue and those that are not, each agent a
// row of two lines. HitTest reads the same layout, so a click lands on the row
// drawn under it.
package agents

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// Row is one agent of the list. Issue is the issue it is linked to, nil for
// an agent that is linked to none.
type Row struct {
	Agent agent.Agent
	Issue *domain.IssueSummary
}

// State is what Render draws.
type State struct {
	// Rows are the linked agents first, then the others: Render opens a
	// group where Issue changes from set to nil.
	Rows         []Row
	SelectedRow  int
	ScrollOffset int
	Width        int
	Height       int
	// Empty is why there is no agent, drawn under "No agents" when Rows is
	// empty; "" draws "No agents" alone.
	Empty string
	// Hover is the row under the pointer, -1 for none.
	Hover int
}

const (
	// headLines is the rule above the list box.
	headLines = 1
	// RowLines is the lines of one row: the agent, then its details.
	RowLines = 2
	// gapLines is the blank line between the two groups.
	gapLines = 1

	linkedTitle   = "Linked"
	unlinkedTitle = "Not linked"
	emptyText     = "No agents"
)

// line is one line of the list: a group heading, a gap, or a line of a row.
type line struct {
	row     int // -1 for a heading or a gap
	heading string
	first   bool // the first line of row
}

// layout is every line of the list, in order.
func layout(rows []Row) []line {
	var lines []line
	linked := 0
	for linked < len(rows) && rows[linked].Issue != nil {
		linked++
	}
	group := func(title string, from, to int) {
		if from == to {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, line{row: -1})
		}
		lines = append(lines, line{row: -1, heading: fmt.Sprintf("%s (%d)", title, to-from)})
		for i := from; i < to; i++ {
			lines = append(lines, line{row: i, first: true}, line{row: i})
		}
	}
	group(linkedTitle, 0, linked)
	group(unlinkedTitle, linked, len(rows))
	return lines
}

// InnerHeight is the lines of rows a frame of height lines shows: what the
// rule and the box's two borders leave.
func InnerHeight(height int) int {
	return max(1, height-headLines-2)
}

// EnsureVisible is the scroll offset, in lines, that shows the whole of the
// selected row, the heading above it when that fits, and starts no later
// than it must. offset is the offset the list had.
func EnsureVisible(rows []Row, selected, offset, height int) int {
	lines := layout(rows)
	window := InnerHeight(height)
	start := -1
	for i, l := range lines {
		if l.row == selected && l.first {
			start = i
			break
		}
	}
	if start < 0 {
		return 0
	}
	top := start
	if start > 0 && lines[start-1].heading != "" {
		top = start - 1
	}
	switch {
	case top < offset:
		offset = top
	case start+RowLines > offset+window:
		offset = start + RowLines - window
	}
	return clampOffset(offset, len(lines), window)
}

func clampOffset(offset, total, window int) int {
	return max(0, min(offset, total-window))
}

// Render draws the rule and the list box under it.
func Render(state State) string {
	width := max(state.Width, 6)
	inner := width - 2
	window := InnerHeight(state.Height)

	var content []string
	topRight := ""
	if len(state.Rows) == 0 {
		content = append(content, " "+lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Render(emptyText))
		if state.Empty != "" {
			content = append(content, " "+muted(textutil.TruncateString(displaytext.OneLine(state.Empty), inner-1)))
		}
	} else {
		topRight = fmt.Sprintf("%d", len(state.Rows))
		lines := layout(state.Rows)
		offset := clampOffset(state.ScrollOffset, len(lines), window)
		for _, l := range lines[offset:min(len(lines), offset+window)] {
			content = append(content, renderLine(state, l, inner))
		}
	}

	return styles.Rule(width) + "\n" + styles.FormSection(styles.FormSectionConfig{
		Content:  content,
		Width:    width,
		Height:   window + 2,
		TopLeft:  "Agents",
		TopRight: topRight,
	})
}

func renderLine(state State, l line, width int) string {
	if l.row < 0 {
		if l.heading == "" {
			return ""
		}
		return " " + lipgloss.NewStyle().Bold(true).Foreground(styles.TextMutedColor).Render(textutil.TruncateString(l.heading, width-1))
	}
	row := state.Rows[l.row]
	selected := l.row == state.SelectedRow
	plain, gutter := styles.SelectionPrefix(selected, true)
	room := width - lipgloss.Width(plain)
	var text string
	if l.first {
		text = agentLine(row.Agent, room)
	} else {
		text = muted(textutil.TruncateString(detailText(row), room))
	}
	return styles.RowHighlight(gutter+text, width, selected, l.row == state.Hover)
}

// agentLine is the name of a, its status in the colour of the status, and
// what it does, cut to room cells. The name is cut first, so the status the
// tab sorts by stays while there is room for it and one cell of the name.
func agentLine(a agent.Agent, room int) string {
	status := "  " + displaytext.OneLine(agent.StatusWord(a.Status))
	nameRoom := room
	if room > lipgloss.Width(status) {
		nameRoom = room - lipgloss.Width(status)
	}
	name := textutil.TruncateString(displaytext.OneLine(a.Title()), nameRoom)
	out := lipgloss.NewStyle().Bold(true).Foreground(styles.TextPrimaryColor).Render(name)
	left := room - lipgloss.Width(name)
	if left < lipgloss.Width(status) {
		return out
	}
	out += statusStyle(a.Status).Render(status)
	left -= lipgloss.Width(status)
	if activity := strings.TrimSpace(displaytext.OneLine(a.Activity)); activity != "" && left > 4 {
		out += muted(textutil.TruncateString(" · "+activity, left))
	}
	return out
}

// statusStyle draws a gone agent muted: it is no live status.
func statusStyle(status agent.Status) lipgloss.Style {
	if status == agent.Gone {
		return lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	}
	return styles.AgentStatusStyle(status)
}

// detailText is the second line of a row: where the agent is, and the issue
// it is linked to.
func detailText(row Row) string {
	parts := make([]string, 0, 2)
	// Without a name the first line already shows the label.
	if row.Agent.Title() != row.Agent.Label {
		parts = append(parts, displaytext.OneLine(row.Agent.Label))
	}
	if row.Issue != nil {
		parts = append(parts, displaytext.OneLine(row.Issue.ID)+"  "+displaytext.OneLine(row.Issue.Title))
	}
	return strings.Join(parts, " · ")
}

func muted(text string) string {
	return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(text)
}

// HitTest is the row drawn at cell (x, y) of Render's output: ok is false on
// the rule, a border, a heading, a gap and below the last row.
func HitTest(state State, x, y int) (row int, ok bool) {
	width := max(state.Width, 6)
	window := InnerHeight(state.Height)
	content := y - headLines - 1
	if x <= 0 || x >= width-1 || content < 0 || content >= window {
		return 0, false
	}
	lines := layout(state.Rows)
	at := clampOffset(state.ScrollOffset, len(lines), window) + content
	if at >= len(lines) || lines[at].row < 0 {
		return 0, false
	}
	return lines[at].row, true
}
