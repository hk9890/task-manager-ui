package styles

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestRowPrefixMarksTheHoveredRowWithoutReplacingTheSelection pins the three
// gutters a row can draw. The hover chevron takes the selection's two cells and
// its glyph, in its own colour; a row that is both draws the selection's.
func TestRowPrefixMarksTheHoveredRowWithoutReplacingTheSelection(t *testing.T) {
	forceTrueColor(t)

	_, selected := SelectionPrefix(true, true)
	_, idle := SelectionPrefix(false, true)

	plain, hovered := RowPrefix(false, true, true)
	if plain != "› " || lipgloss.Width(hovered) != 2 {
		t.Fatalf("hover gutter = plain %q, rendered width %d; want the 2-cell chevron", plain, lipgloss.Width(hovered))
	}
	if hovered == selected || hovered == idle {
		t.Fatalf("hover gutter %q must differ from the selected %q and the idle %q gutters", hovered, selected, idle)
	}

	if _, both := RowPrefix(true, true, true); both != selected {
		t.Fatalf("a selected row under the pointer drew %q, want the selection gutter %q", both, selected)
	}
	if _, neither := RowPrefix(false, false, true); neither != idle {
		t.Fatalf("an idle row drew %q, want the idle gutter %q", neither, idle)
	}
}
