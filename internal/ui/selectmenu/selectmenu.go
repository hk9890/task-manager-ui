// Package selectmenu draws the menu a finished drag selection opens beside
// the release cell: what the release sent to the clipboard, and what else can
// be done with the selected text.
package selectmenu

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// Entry is one line of the menu. A disabled entry is drawn muted and takes no
// cursor and no click.
type Entry struct {
	Label    string
	Key      string
	Disabled bool
}

// State is what Render and HitTest both read. Width is the screen's: the menu
// is cut to it. Cursor and Hover are entry indexes; Hover is -1 when the
// pointer is on no entry.
type State struct {
	Title   string
	Hint    string
	Width   int
	Entries []Entry
	Cursor  int
	Hover   int
}

// headerRows is the top border, the title and the hint: the first entry is
// drawn on the row after them.
const headerRows = 3

// gutterCells is the selection gutter in front of an entry.
const gutterCells = 2

// innerWidth is the width between the side borders: what the text asks for,
// and no more than the screen leaves.
func innerWidth(s State) int {
	width := max(lipgloss.Width(s.Title), lipgloss.Width(s.Hint)) + 2
	for _, entry := range s.Entries {
		width = max(width, gutterCells+lipgloss.Width(entry.Label)+2+lipgloss.Width(entry.Key)+1)
	}
	return min(width, max(s.Width-2, 0))
}

// Size is the width and the height Render draws, borders included.
func Size(s State) (width, height int) {
	return innerWidth(s) + 2, headerRows + len(s.Entries) + 1
}

// Render draws the menu in a rounded border.
func Render(s State) string {
	inner := innerWidth(s)
	title := lipgloss.NewStyle().Bold(true).Foreground(styles.OverlayTitleColor).Render(textutil.TruncateString(s.Title, inner-1))
	hint := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(textutil.TruncateString(s.Hint, inner-1))
	lines := []string{
		textutil.PadToWidth(" "+title, inner),
		textutil.PadToWidth(" "+hint, inner),
	}
	for i, entry := range s.Entries {
		lines = append(lines, renderEntry(entry, inner, i == s.Cursor && !entry.Disabled, i == s.Hover && !entry.Disabled))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Render(strings.Join(lines, "\n"))
}

func renderEntry(entry Entry, width int, selected, hovered bool) string {
	_, gutter := styles.SelectionPrefix(selected, true)
	labelStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)
	if entry.Disabled {
		labelStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	}
	text := textutil.TruncateString(entry.Label, width-gutterCells-lipgloss.Width(entry.Key)-2)
	label := labelStyle.Render(text)
	key := lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor).Render(entry.Key)
	gap := width - gutterCells - lipgloss.Width(text) - lipgloss.Width(entry.Key) - 1
	row := textutil.PadToWidth(gutter+label+strings.Repeat(" ", max(gap, 1))+key+" ", width)
	return styles.RowHighlight(row, width, selected, hovered)
}

// HitTest reports the entry drawn at the cell x, y, counted from the menu's
// top left, and whether the cell is on the menu at all. entry is -1 on the
// border, the title and the hint.
func HitTest(s State, x, y int) (entry int, inside bool) {
	width, height := Size(s)
	if x < 0 || y < 0 || x >= width || y >= height {
		return -1, false
	}
	row := y - headerRows
	if x == 0 || x == width-1 || row < 0 || row >= len(s.Entries) {
		return -1, true
	}
	return row, true
}

// Place puts a menu of the given size beside the cell x, y of a screen:
// below and right of it, and above or left of it where it would leave the
// screen there. A screen smaller than the menu gets it at its top left.
func Place(x, y, width, height, screenWidth, screenHeight int) (left, top int) {
	left, top = x+1, y+1
	if left+width > screenWidth {
		left = x - width
	}
	if top+height > screenHeight {
		top = y - height
	}
	return textutil.Clamp(left, 0, max(screenWidth-width, 0)), textutil.Clamp(top, 0, max(screenHeight-height, 0))
}
