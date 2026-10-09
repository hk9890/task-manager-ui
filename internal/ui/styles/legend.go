package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
)

// KeyHint is one entry of a key legend: a key and what it does. A hint with no
// key is a notice, drawn as its text alone.
type KeyHint struct {
	Key  string
	Desc string
}

const legendSeparator = " · "

// KeyLegend draws the key legend of a surface on one line: "key what · key
// what". A hint that does not fit in width is dropped with every hint after
// it, so the order of hints is their priority; the first hint is cut instead,
// so a legend is never empty. width <= 0 draws them all.
func KeyLegend(hints []KeyHint, width int) string {
	key := lipgloss.NewStyle().Foreground(TextSecondaryColor)
	desc := lipgloss.NewStyle().Foreground(ShellFooterHelpColor)

	var b strings.Builder
	used := 0
	for _, hint := range hints {
		plain := strings.TrimSpace(hint.Key + " " + hint.Desc)
		cells := lipgloss.Width(plain)
		if used > 0 {
			cells += lipgloss.Width(legendSeparator)
		}
		if width > 0 && used+cells > width {
			if used == 0 {
				return desc.Render(textutil.TruncateString(plain, width))
			}
			break
		}
		if used > 0 {
			b.WriteString(desc.Render(legendSeparator))
		}
		if hint.Key != "" {
			b.WriteString(key.Render(hint.Key))
			b.WriteString(" ")
		}
		b.WriteString(desc.Render(hint.Desc))
		used += cells
	}
	return b.String()
}
