package detail

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// Button identifies a button drawn inside a pane.
type Button int

const (
	ButtonNone Button = iota
	ButtonEdit
	ButtonAddComment
	ButtonEditComment
	ButtonDeleteComment
)

const (
	// buttonSeparator stands between two buttons on one line, as on the menu
	// bar.
	buttonSeparator = " · "
	// buttonTextFloor is the least cells a line keeps for its text. A line with
	// less room draws no button.
	buttonTextFloor = 12
)

// cells is the columns a button covers, from x0 up to x1 and not x1 itself.
// The zero value covers nothing.
type cells struct {
	x0, x1 int
}

func (c cells) covers(x int) bool {
	return x >= c.x0 && x < c.x1
}

// paneButton draws a button inside a pane as the menu bar draws one: the label
// with a space on each side, then the key that does the same. lit is the
// button under the pointer.
func paneButton(label, key string, lit bool) string {
	labelStyle := lipgloss.NewStyle().Foreground(styles.ShellActionColor)
	keyStyle := lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor)
	if lit {
		labelStyle = labelStyle.Background(styles.ShellHoverBgColor).Bold(true)
		keyStyle = keyStyle.Background(styles.ShellHoverBgColor)
	}
	return labelStyle.Render(" "+label+" ") + keyStyle.Render(key+" ")
}

// placeButtons puts buttons at the right end of line, which is width cells
// wide, and reports the cells of each. It draws none of them, and reports
// nil, when fewer than buttonTextFloor cells would stay for the text.
func placeButtons(line string, width int, buttons ...string) (string, []cells) {
	used := (len(buttons) - 1) * lipgloss.Width(buttonSeparator)
	for _, button := range buttons {
		used += lipgloss.Width(button)
	}
	room := width - used
	if len(buttons) == 0 || room < buttonTextFloor {
		return textutil.TruncateString(line, width), nil
	}

	separator := lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor).Render(buttonSeparator)
	var b strings.Builder
	// One cell stays blank between the text and the first button.
	b.WriteString(textutil.PadToWidth(textutil.TruncateString(line, room-1), room))
	placed := make([]cells, 0, len(buttons))
	x := room
	for idx, button := range buttons {
		if idx > 0 {
			b.WriteString(separator)
			x += lipgloss.Width(buttonSeparator)
		}
		b.WriteString(button)
		placed = append(placed, cells{x0: x, x1: x + lipgloss.Width(button)})
		x = placed[idx].x1
	}
	return b.String(), placed
}
