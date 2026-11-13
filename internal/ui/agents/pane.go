package agents

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/ui/scroll"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/markdown"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/renderhelpers"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	paneTitle = "Agent"
	// paneKeyWidth is the column the field names of the pane take.
	paneKeyWidth = 9
	// panePad is the blank cell on each side of the pane's text.
	panePad = 1

	messageHeading = "Last message"
)

// Pane is what the Agent pane shows of the selected agent beside the fields
// of its row: the last thing it said, or how far the read of it is.
type Pane struct {
	// Message is the last thing the agent said, as markdown.
	Message string
	// Reading is true while the read of Message is in flight and no older
	// message of the agent is known.
	Reading bool
	// Error is why the read failed.
	Error        string
	ScrollOffset int
}

// RenderPane draws the Agent pane of row, width cells wide and height lines
// high: the agent's title and status, what it does, its issue, label and
// session, then its last message. A nil row draws the pane empty.
func RenderPane(pane Pane, row *Row, width, height int) string {
	inner := max(1, height-2)
	text := max(1, width-2-2*panePad)
	view, _ := scroll.Window(paneLines(pane, row, text), pane.ScrollOffset, inner, text)
	pad := strings.Repeat(" ", panePad)
	for i, line := range view {
		view[i] = pad + line
	}
	return styles.FormSection(styles.FormSectionConfig{
		Content: view,
		Width:   width,
		Height:  height,
		TopLeft: paneTitle,
	})
}

// PaneMaxOffset is the largest scroll offset of the pane that leaves no blank
// line at its bottom: Render's state, with the pane at the width the split
// gives it.
func PaneMaxOffset(state State) int {
	_, width, split := styles.SplitWidths(max(state.Width, 6))
	if !split {
		return 0
	}
	inner := InnerHeight(state.Height)
	lines := paneLines(state.Pane, selectedRow(state), max(1, width-2-2*panePad))
	return max(0, len(lines)-inner)
}

func paneLines(pane Pane, row *Row, width int) []string {
	if row == nil {
		return []string{muted(textutil.TruncateString("No agent selected", width))}
	}
	a := row.Agent
	// The activity gets lines of its own under the title, wrapped, not cut.
	titleOnly := a
	titleOnly.Activity = ""
	lines := []string{agentLine(titleOnly, width, nil)}
	if activity := strings.TrimSpace(displaytext.OneLine(a.Activity)); activity != "" {
		for _, line := range textutil.WrapAtSpaces(activity, width) {
			lines = append(lines, muted(line))
		}
	}
	lines = append(lines, styles.ContentRule(width))
	lines = append(lines, issueLines(row, width)...)
	if a.Title() != a.Label {
		lines = append(lines, field("Label", displaytext.OneLine(a.Label), width))
	}
	if a.Session != "" {
		lines = append(lines, field("Session", displaytext.OneLine(a.Session), width))
	}
	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(styles.TextMutedColor).Render(messageHeading))
	return append(lines, messageLines(pane, a.Status, width)...)
}

// issueLines is the issue the agent is linked to as Detail's Content pane
// heads it: its type, priority, status and ID, then its title under them.
func issueLines(row *Row, width int) []string {
	if row.Issue == nil {
		return []string{field("Issue", muted("none"), width)}
	}
	issue := row.Issue
	meta := strings.Join([]string{
		renderhelpers.CompactIssueTypeStyled(issue.Type),
		renderhelpers.CompactPriorityStyled(issue.Priority),
		renderhelpers.CompactIssueStateStyled(issue.Status),
		styles.IssueIDMutedStyle.Render(displaytext.OneLine(issue.ID)),
	}, " ")
	title := textutil.TruncateString(displaytext.OneLine(issue.Title), max(0, width-paneKeyWidth))
	return []string{
		field("Issue", meta, width),
		strings.Repeat(" ", paneKeyWidth) + lipgloss.NewStyle().Bold(true).Foreground(styles.TextPrimaryColor).Render(title),
	}
}

func field(name, value string, width int) string {
	key := muted(name + strings.Repeat(" ", max(1, paneKeyWidth-lipgloss.Width(name))))
	return key + textutil.TruncateString(value, max(0, width-paneKeyWidth))
}

func messageLines(pane Pane, status agent.Status, width int) []string {
	switch {
	case status == agent.Gone:
		return wrapMuted("The host no longer runs this agent. Resume it to read what it said.", width)
	case pane.Error != "":
		message := styles.Glyphs.ToastWarn + " read failed: " + displaytext.OneLine(pane.Error)
		var lines []string
		for _, line := range textutil.WrapAtSpaces(message, width) {
			lines = append(lines, lipgloss.NewStyle().Foreground(styles.ErrorTextColor).Render(line))
		}
		return lines
	case pane.Reading:
		return []string{muted("reading…")}
	case strings.TrimSpace(pane.Message) == "":
		return []string{muted("(nothing yet)")}
	}
	var lines []string
	for _, line := range strings.Split(markdown.NewRenderer().RenderMarkdown(pane.Message, width), "\n") {
		lines = append(lines, textutil.TruncateString(strings.TrimRight(line, " \t"), width))
	}
	return lines
}

func wrapMuted(text string, width int) []string {
	lines := textutil.WrapAtSpaces(text, width)
	for i, line := range lines {
		lines[i] = muted(line)
	}
	return lines
}
