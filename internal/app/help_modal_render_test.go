package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
)

// TestHelpModal_RenderedFrameHasClosingBottomBorder pins znri.5's visual
// rendering claim: when the help modal is open the rendered View must
// include a closing bottom border (`╰`) at the SAME column as the modal's
// opening top border (`╭`). If only the top corner appears, the modal box
// is unbounded and the underlying board renders through the lower portion.
//
// Tested at terminal widths that surfaced the bug during exploration:
// 120x34 (the documented default) and 160x40.
//
// SKIPPED: pins beads-workbench-znri.5. Remove the t.Skip below when the
// help-modal rendering is fixed so the test activates as a regression guard.
func TestHelpModal_RenderedFrameHasClosingBottomBorder(t *testing.T) {
	t.Skip("pins beads-workbench-znri.5 (help modal missing bottom border); remove skip when fixed")
	for _, size := range []struct {
		name          string
		width, height int
	}{
		{name: "120x34", width: 120, height: 34},
		{name: "160x40", width: 160, height: 40},
	} {
		t.Run(size.name, func(t *testing.T) {
			gw := newTestRepository()
			gw.seedReady("bw-1", "Ready", "task", 1)
			services, err := NewServices(gw, config.Default(), t.TempDir())
			if err != nil {
				t.Fatalf("NewServices: %v", err)
			}
			m := mustNewModel(t, services)
			m = applyMessages(t, m, runBatch(m.Init()))
			m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: size.width, Height: size.height}})
			m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}})

			view := m.View()
			lines := strings.Split(view, "\n")

			topLine, topCol := findRunePosition(lines, '╭', "Keyboard Help")
			if topLine < 0 {
				t.Fatalf("expected modal top border line containing 'Keyboard Help'; view:\n%s", view)
			}

			// Look for a closing `╰` at the same column on a later line.
			found := false
			for i := topLine + 1; i < len(lines); i++ {
				if runeAtColumn(lines[i], topCol) == '╰' {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("modal top border at line %d col %d has no matching ╰ closing border at the same column; rendered view:\n%s",
					topLine, topCol, view)
			}
		})
	}
}

// findRunePosition scans lines for the first line whose content contains
// mustContain (when non-empty) and includes the given rune; returns
// (lineIndex, columnIndex) of that rune. Returns (-1, -1) when not found.
// Column index is rune-position, not byte-position.
func findRunePosition(lines []string, want rune, mustContain string) (int, int) {
	for i, line := range lines {
		if mustContain != "" && !strings.Contains(line, mustContain) {
			continue
		}
		col := 0
		for _, r := range line {
			if r == want {
				return i, col
			}
			col++
		}
	}
	return -1, -1
}

// runeAtColumn returns the rune at column col in line, or 0 if the line is
// shorter than col runes.
func runeAtColumn(line string, col int) rune {
	i := 0
	for _, r := range line {
		if i == col {
			return r
		}
		i++
	}
	return 0
}
