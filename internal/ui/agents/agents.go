package agents

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/board"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
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
	// Query is the filter text as typed, and Placeholder what the query line
	// says while it is empty. Rows holds the agents the query matched, of
	// Loaded agents in all.
	Query       string
	Placeholder string
	Loaded      int
	// Pane is what the Agent pane shows of the selected row. It is drawn
	// only where styles.SplitWidths splits the screen.
	Pane Pane
}

const (
	// headLines is the rule and the query line above the list box.
	headLines = 2
	// queryIndent puts the prompt under the first menu-bar button, as on
	// the board.
	queryIndent = 1
	// RowLines is the lines of one row: the agent, then its details.
	RowLines = 2
	// gapLines is the blank line between the two groups.
	gapLines = 1

	linkedTitle   = "Linked"
	unlinkedTitle = "Not linked"
	emptyText     = "No agents"
	noMatchText   = "No agent matches the filter"
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
// head and the box's two borders leave.
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

// Render draws the head, the list box under it and, on a screen wide enough,
// the Agent pane beside the list.
func Render(state State) string {
	width := max(state.Width, 6)
	listWidth, paneWidth, split := styles.SplitWidths(width)
	head := styles.Rule(width) + "\n" + strings.Repeat(" ", queryIndent) +
		board.RenderQueryLine(state.Query, state.Placeholder, width-queryIndent)
	list := renderList(state, listWidth)
	if !split {
		return head + "\n" + list
	}
	pane := RenderPane(state.Pane, selectedRow(state), paneWidth, InnerHeight(state.Height)+2)
	return head + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, list, strings.Repeat(" ", styles.SplitGap), pane)
}

func renderList(state State, width int) string {
	inner := width - 2
	window := InnerHeight(state.Height)

	var content []string
	topRight := ""
	switch {
	case state.Query != "" && state.Loaded > 0:
		topRight = fmt.Sprintf("%d of %d", len(state.Rows), state.Loaded)
	case len(state.Rows) > 0:
		topRight = fmt.Sprintf("%d", len(state.Rows))
	}
	if len(state.Rows) == 0 {
		title, why := emptyText, state.Empty
		if state.Query != "" && state.Empty == "" {
			title, why = noMatchText, ""
		}
		content = append(content, " "+lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Render(textutil.TruncateString(title, inner-1)))
		for _, line := range textutil.WrapAtSpaces(displaytext.OneLine(why), inner-2) {
			content = append(content, " "+muted(line))
		}
	} else {
		lines := layout(state.Rows)
		offset := clampOffset(state.ScrollOffset, len(lines), window)
		for _, l := range lines[offset:min(len(lines), offset+window)] {
			content = append(content, renderLine(state, l, inner))
		}
	}

	return styles.FormSection(styles.FormSectionConfig{
		Content:  content,
		Width:    width,
		Height:   window + 2,
		TopLeft:  "Agents",
		TopRight: topRight,
	})
}

func selectedRow(state State) *Row {
	if state.SelectedRow < 0 || state.SelectedRow >= len(state.Rows) {
		return nil
	}
	return &state.Rows[state.SelectedRow]
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
	match := strings.Fields(state.Query)
	if l.first {
		text = agentLine(row.Agent, room, match)
	} else {
		text = issuerow.MarkMatches(textutil.TruncateString(detailText(row), room), match, mutedStyle())
	}
	return styles.RowHighlight(gutter+text, width, selected, l.row == state.Hover)
}

// agentLine is the name of a, its status in the colour of the status, and
// what it does, cut to room cells. The name is cut first, so the status the
// tab sorts by stays while there is room for it and one cell of the name.
func agentLine(a agent.Agent, room int, match []string) string {
	status := "  " + displaytext.OneLine(agent.StatusWord(a.Status))
	nameRoom := room
	if room > lipgloss.Width(status) {
		nameRoom = room - lipgloss.Width(status)
	}
	name := textutil.TruncateString(displaytext.OneLine(a.Title()), nameRoom)
	out := issuerow.MarkMatches(name, match, lipgloss.NewStyle().Bold(true).Foreground(styles.TextPrimaryColor))
	left := room - lipgloss.Width(name)
	if left < lipgloss.Width(status) {
		return out
	}
	out += statusStyle(a.Status).Render(status)
	left -= lipgloss.Width(status)
	if activity := strings.TrimSpace(displaytext.OneLine(a.Activity)); activity != "" && left > 4 {
		out += issuerow.MarkMatches(textutil.TruncateString(" · "+activity, left), match, mutedStyle())
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
	return mutedStyle().Render(text)
}

func mutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextMutedColor)
}

// HitTest is the row drawn at cell (x, y) of Render's output: ok is false on
// the head, a border, a heading, a gap, below the last row and on the pane.
func HitTest(state State, x, y int) (row int, ok bool) {
	width, _, _ := styles.SplitWidths(max(state.Width, 6))
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

// OnPane reports whether cell (x, y) of Render's output is on the Agent pane,
// borders included. It is false while the screen is not split.
func OnPane(state State, x, y int) bool {
	list, _, split := styles.SplitWidths(max(state.Width, 6))
	return split && x >= list+styles.SplitGap && y >= headLines
}
