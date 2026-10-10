package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const selectionIdlePrefix = "  "

const ruleGlyph = "─"

// Rule is a thin line across width: under the menu bar, and above the query
// line of a list.
func Rule(width int) string {
	return lipgloss.NewStyle().Foreground(ShellRuleColor).Render(strings.Repeat(ruleGlyph, max(0, width)))
}

// SelectionPrefix returns the shared 2-character selection gutter prefix.
// The plain variant is unstyled and should be used for width math/truncation.
// The rendered variant applies app-wide selection styling when requested.
func SelectionPrefix(selected, styled bool) (plain string, rendered string) {
	if !selected {
		return selectionIdlePrefix, selectionIdlePrefix
	}

	plain = Glyphs.Cursor + " "
	if styled {
		return plain, SelectionIndicatorStyle.Render(Glyphs.Cursor) + " "
	}

	return plain, plain
}

// sgrReset is what Lip Gloss ends every styled run with.
const sgrReset = "\x1b[0m"

// RowHighlight lays the selection's or the hover's background under a rendered
// row and pads it to width, so the band spans the row instead of ending at its
// last character. Two rows can be lit at once — the one the keys act on and
// the one the pointer is over — so the hover is a step quieter, and the
// selection wins on a row that is both.
//
// The row arrives as separately styled runs, each ending in a reset that would
// cut the band. The background is put back after every reset rather than
// threaded through each run's style, so a renderer stays unaware of it.
func RowHighlight(row string, width int, selected, hovered bool) string {
	color := RowHoverBgColor
	switch {
	case selected:
		color = RowSelectedBgColor
	case !hovered:
		return row
	}

	// Lip Gloss writes the sequence for the active colour profile, and nothing
	// at all on a terminal without colour.
	on, _, _ := strings.Cut(lipgloss.NewStyle().Background(color).Render("x"), "x")
	if on == "" {
		return row
	}
	if gap := width - lipgloss.Width(row); gap > 0 {
		row += strings.Repeat(" ", gap)
	}
	return on + strings.ReplaceAll(row, sgrReset, sgrReset+on) + sgrReset
}
