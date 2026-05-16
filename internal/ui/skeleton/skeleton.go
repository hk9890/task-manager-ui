// Package skeleton provides a shared skeleton-row renderer used by per-surface
// non-blocking loading states (board, search, detail). It renders placeholder
// rows composed of ░ glyphs whose visible width (as measured by lipgloss.Width)
// matches the requested terminal width exactly.
package skeleton

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

// SkeletonGlyph is the canonical placeholder character used in skeleton rows.
// Downstream PTY assertions (0x36.5) reference this constant so no caller
// hard-codes the literal rune.
const SkeletonGlyph = "░"

// skeletonStyle is the muted foreground style applied to every skeleton block,
// matching the subtle dim styling used in internal/ui/loading/loading.go.
var skeletonStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)

// SkeletonRow returns a styled placeholder row whose visible width
// (lipgloss.Width) equals width. The row is composed of slots distinct blocks
// of ░ glyphs separated by single-space gaps, approximating a typical issue
// row layout:
//   - slot 0 is ~8 chars wide (id-like)
//   - remaining slots share the leftover width approximately evenly
//
// Returns an empty string when slots <= 0 or width <= 0.
func SkeletonRow(width int, slots int) string {
	if width <= 0 || slots <= 0 {
		return ""
	}

	// Number of gap characters between slots.
	gaps := slots - 1

	// Total width available for glyph content after gaps are reserved.
	contentWidth := width - gaps
	if contentWidth <= 0 {
		// Not enough room for even one glyph per slot; fill the whole width
		// with a single block.
		return skeletonStyle.Render(strings.Repeat(SkeletonGlyph, width))
	}

	// Determine per-slot widths.
	widths := make([]int, slots)
	if slots == 1 {
		widths[0] = contentWidth
	} else {
		// First slot is id-like (~8 chars) but capped at half of contentWidth
		// so there is always meaningful space for the remaining slots.
		firstSlotWidth := 8
		if firstSlotWidth > contentWidth/2 {
			firstSlotWidth = contentWidth / 2
		}
		if firstSlotWidth < 1 {
			firstSlotWidth = 1
		}
		widths[0] = firstSlotWidth

		remaining := contentWidth - firstSlotWidth
		tail := slots - 1
		base := remaining / tail
		extra := remaining % tail // distribute remainder across early slots

		for i := 1; i < slots; i++ {
			w := base
			if (i - 1) < extra {
				w++
			}
			if w < 1 {
				w = 1
			}
			widths[i] = w
		}
	}

	// Build each slot as a styled block of SkeletonGlyph runes, then join
	// with single-space gaps.  Gaps are unstyled so they remain transparent
	// and don't alter lipgloss width accounting.
	parts := make([]string, slots)
	for i, w := range widths {
		parts[i] = skeletonStyle.Render(strings.Repeat(SkeletonGlyph, w))
	}

	return strings.Join(parts, " ")
}
