package skeleton_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/beads-workbench/internal/ui/skeleton"
)

// TestSkeletonGlyphIdentity verifies the canonical glyph constant.
func TestSkeletonGlyphIdentity(t *testing.T) {
	if skeleton.SkeletonGlyph != "░" {
		t.Errorf("SkeletonGlyph = %q; want %q (U+2591)", skeleton.SkeletonGlyph, "░")
	}
}

// TestSkeletonRowWidth verifies that lipgloss.Width(SkeletonRow(w, s)) == w for
// several representative width/slot combinations.
func TestSkeletonRowWidth(t *testing.T) {
	cases := []struct {
		width int
		slots int
	}{
		{width: 40, slots: 2},
		{width: 80, slots: 3},
		{width: 20, slots: 1},
		{width: 60, slots: 4},
		{width: 10, slots: 2},
		{width: 1, slots: 1},
	}

	for _, tc := range cases {
		row := skeleton.SkeletonRow(tc.width, tc.slots)
		got := lipgloss.Width(row)
		if got != tc.width {
			t.Errorf("SkeletonRow(%d, %d): lipgloss.Width = %d; want %d (row=%q)",
				tc.width, tc.slots, got, tc.width, row)
		}
	}
}

// TestSkeletonRowContainsGlyph verifies that non-empty rows include the
// SkeletonGlyph rune.
func TestSkeletonRowContainsGlyph(t *testing.T) {
	cases := []struct {
		width int
		slots int
	}{
		{40, 2},
		{80, 3},
		{20, 1},
	}

	for _, tc := range cases {
		row := skeleton.SkeletonRow(tc.width, tc.slots)
		if !strings.Contains(row, skeleton.SkeletonGlyph) {
			t.Errorf("SkeletonRow(%d, %d) does not contain SkeletonGlyph %q; got %q",
				tc.width, tc.slots, skeleton.SkeletonGlyph, row)
		}
	}
}

// TestSkeletonRowEdgeCases verifies that invalid inputs return an empty string.
func TestSkeletonRowEdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		width int
		slots int
	}{
		{name: "zero width", width: 0, slots: 2},
		{name: "negative width", width: -5, slots: 2},
		{name: "zero slots", width: 40, slots: 0},
		{name: "negative slots", width: 40, slots: -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := skeleton.SkeletonRow(tc.width, tc.slots)
			if row != "" {
				t.Errorf("SkeletonRow(%d, %d) = %q; want empty string",
					tc.width, tc.slots, row)
			}
		})
	}
}

// TestSkeletonRowSlotSeparation verifies that multi-slot rows contain at least
// one gap character between slots (i.e. the rendered string has a plain space
// that is not part of the glyph blocks).
func TestSkeletonRowSlotSeparation(t *testing.T) {
	row := skeleton.SkeletonRow(40, 2)
	if !strings.Contains(row, " ") {
		t.Errorf("SkeletonRow(40, 2) expected space-separated slots but found no space in %q", row)
	}
}
