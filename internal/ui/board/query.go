package board

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// renderQueryLine is the line a surface's filter text is typed on: the prompt,
// the text, then a cursor block. An empty query shows the placeholder, dim,
// with the cursor on its first cell, so the line says what a key press does.
//
// A text too long for width is cut from the front: the end is where the next
// key lands. The line is never wider than width.
func renderQueryLine(query, placeholder string, width int) string {
	accent := lipgloss.NewStyle().Foreground(styles.QueryAccentColor)
	cursor := accent.Reverse(true)
	prompt := styles.Glyphs.Prompt + " "

	if query == "" {
		muted := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		hint := []rune(textutil.TruncateString(placeholder, width-lipgloss.Width(prompt)))
		if len(hint) == 0 {
			return textutil.TruncateString(accent.Render(prompt), width)
		}
		return accent.Render(prompt) + cursor.Render(string(hint[:1])) + muted.Render(string(hint[1:]))
	}

	// One cell is the cursor's.
	room := width - lipgloss.Width(prompt) - 1
	text := textutil.TruncateStringFront(query, room)
	line := accent.Render(prompt) +
		lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Render(text) +
		cursor.Render(" ")
	return textutil.TruncateString(line, width)
}
