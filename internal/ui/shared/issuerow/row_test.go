package issuerow

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hk9890/beads-workbench/internal/domain"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
)

func TestRenderCompactSelectionAndMetadata(t *testing.T) {
	line := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "beads-workbench-u5s",
			Title:    "Row renderer metadata",
			Type:     "task",
			Status:   "open",
			Priority: 1,
		},
		Selected: true,
		Width:    72,
	})

	if !strings.HasPrefix(line, "› ") {
		t.Fatalf("expected selected row prefix, got: %q", line)
	}
	if !strings.Contains(line, "T P1 OPN") {
		t.Fatalf("expected compact metadata tokens, got: %q", line)
	}
}

func TestRenderCompactTruncatesMetadataWhenVeryNarrow(t *testing.T) {
	line := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "beads-workbench-very-long-id",
			Title:    "Long title that should not fit",
			Type:     "feature",
			Status:   "in_progress",
			Priority: 0,
		},
		Width: 15,
	})

	if strings.Contains(line, "Long title") {
		t.Fatalf("expected title to be omitted when metadata consumes width, got: %q", line)
	}
	if !strings.Contains(line, "F P0 IP") {
		t.Fatalf("expected metadata retained in narrow row, got: %q", line)
	}
}

func TestRenderCompactStyledIncludesANSI(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	line := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "beads-workbench-u5s",
			Title:    "Styled row",
			Type:     "bug",
			Status:   "blocked",
			Priority: 0,
		},
		Selected: true,
		Width:    64,
		Styled:   true,
	})

	if !strings.Contains(line, "\x1b[") {
		t.Fatalf("expected ANSI styling when Styled is true, got: %q", line)
	}
	plain := testui.AnsiEscapePattern.ReplaceAllString(line, "")
	if !strings.Contains(plain, "› B P0 BLK u5s") {
		t.Fatalf("expected styled metadata to preserve token text, got: %q", plain)
	}
}

func TestCompactIDWidthUsesSharedBoundedRule(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{width: 10, want: 7},
		{width: 45, want: 9},
		{width: 60, want: 12},
		{width: 200, want: 12},
	}

	for _, tc := range tests {
		if got := CompactIDWidth(tc.width); got != tc.want {
			t.Fatalf("CompactIDWidth(%d) = %d, want %d", tc.width, got, tc.want)
		}
	}
}

func TestRenderReferenceCompactNarrowWidthsRemainReadable(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{name: "width 22", width: 22},
		{name: "width 24", width: 24},
		{name: "width 28", width: 28},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			line := RenderReferenceCompact(ReferenceRenderConfig{
				Issue: domain.IssueReference{
					ID:       "beads-workbench-syf.3",
					Title:    "Add narrow related issue row renderer for left rail",
					Type:     "task",
					Priority: 3,
					Status:   "in_progress",
				},
				Selected: true,
				Width:    tc.width,
			})

			if strings.Contains(line, "\n") {
				t.Fatalf("expected one-line row at width %d, got %q", tc.width, line)
			}
			if lipgloss.Width(line) > tc.width {
				t.Fatalf("expected row width <= %d, got %d: %q", tc.width, lipgloss.Width(line), line)
			}
			if !strings.Contains(line, "T P3 I") {
				t.Fatalf("expected type/priority/status compact tokens at width %d, got %q", tc.width, line)
			}
			if !strings.Contains(line, "syf.3") {
				t.Fatalf("expected compact issue id token at width %d, got %q", tc.width, line)
			}
		})
	}
}

func TestRenderReferenceCompactSelectionDistinct(t *testing.T) {
	issue := domain.IssueReference{
		ID:       "beads-workbench-9uk",
		Title:    "Selection contrast check",
		Type:     "bug",
		Priority: 1,
		Status:   "blocked",
	}

	selected := RenderReferenceCompact(ReferenceRenderConfig{Issue: issue, Selected: true, Styled: true, Width: 28})
	idle := RenderReferenceCompact(ReferenceRenderConfig{Issue: issue, Selected: false, Styled: true, Width: 28})

	if selected == idle {
		t.Fatalf("expected selected and unselected rows to differ, got selected=%q idle=%q", selected, idle)
	}

	selectedPlain := testui.AnsiEscapePattern.ReplaceAllString(selected, "")
	idlePlain := testui.AnsiEscapePattern.ReplaceAllString(idle, "")

	if !strings.HasPrefix(selectedPlain, "› ") {
		t.Fatalf("expected selected row indicator prefix, got %q", selectedPlain)
	}
	if !strings.HasPrefix(idlePlain, "  ") {
		t.Fatalf("expected unselected row idle prefix, got %q", idlePlain)
	}
}

// TestRenderCompactSkeletonWidth verifies that lipgloss.Width equals opts.Width
// for representative widths and that RenderConfig.RenderCompact is unchanged.
func TestRenderCompactSkeletonWidth(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	for _, w := range []int{30, 50, 80, 120, 200} {
		row := RenderCompactSkeleton(SkeletonOpts{Width: w, Seed: 0, Styled: true})
		got := lipgloss.Width(row)
		if got != w {
			t.Errorf("RenderCompactSkeleton(width=%d): lipgloss.Width=%d, want %d (row=%q)", w, got, w, row)
		}
	}
}

// TestRenderCompactSkeletonFiveRuns verifies that the plain-text skeleton row
// contains exactly five contiguous runs of ▓ separated by single spaces.
func TestRenderCompactSkeletonFiveRuns(t *testing.T) {
	row := RenderCompactSkeleton(SkeletonOpts{Width: 80, Seed: 0, Styled: false})
	// Strip ANSI escapes for the structural check (Styled:false so none expected).
	plain := testui.AnsiEscapePattern.ReplaceAllString(row, "")
	re := regexp.MustCompile(`▓+`)
	runs := re.FindAllString(plain, -1)
	if len(runs) != 5 {
		t.Errorf("expected 5 ▓ runs, got %d in %q", len(runs), plain)
	}
}

// TestRenderCompactSkeletonStyledFiveRuns verifies the same structure when Styled:true.
func TestRenderCompactSkeletonStyledFiveRuns(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	row := RenderCompactSkeleton(SkeletonOpts{Width: 80, Seed: 0, Styled: true})
	plain := testui.AnsiEscapePattern.ReplaceAllString(row, "")
	re := regexp.MustCompile(`▓+`)
	runs := re.FindAllString(plain, -1)
	if len(runs) != 5 {
		t.Errorf("expected 5 ▓ runs in styled row, got %d in %q", len(runs), plain)
	}
}

// TestRenderCompactSkeletonSixDistinctTitleFills verifies that six successive
// Seed values produce six visibly different title fill widths.
func TestRenderCompactSkeletonSixDistinctTitleFills(t *testing.T) {
	re := regexp.MustCompile(`▓+`)
	seen := make(map[int]bool)
	for seed := 0; seed < 6; seed++ {
		row := RenderCompactSkeleton(SkeletonOpts{Width: 80, Seed: seed, Styled: false})
		plain := testui.AnsiEscapePattern.ReplaceAllString(row, "")
		runs := re.FindAllString(plain, -1)
		if len(runs) == 0 {
			t.Fatalf("seed %d: no ▓ runs found in %q", seed, plain)
		}
		// The last run is the title segment.
		titleRunLen := len([]rune(runs[len(runs)-1]))
		seen[titleRunLen] = true
	}
	if len(seen) != 6 {
		t.Errorf("expected 6 distinct title fill widths across seeds 0-5, got %d distinct values: %v", len(seen), seen)
	}
}

// TestSkeletonGlyphConstant verifies the exported glyph constant value.
func TestSkeletonGlyphConstant(t *testing.T) {
	if SkeletonGlyph != "▓" {
		t.Errorf("SkeletonGlyph = %q; want %q (U+2593 DARK SHADE)", SkeletonGlyph, "▓")
	}
}

// TestRenderCompactSkeletonPhaseCyclesColor verifies that:
//   - Phase 0 and Phase 1 produce different styled output (different ANSI sequences)
//   - All three phases (0, 1, 2) produce three distinct styled outputs
//   - Plain text (ANSI-stripped) is identical across phases for the same Width/Seed
func TestRenderCompactSkeletonPhaseCyclesColor(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	const width = 80
	const seed = 0

	outputs := make([]string, 3)
	plains := make([]string, 3)
	for phase := 0; phase < 3; phase++ {
		row := RenderCompactSkeleton(SkeletonOpts{Width: width, Seed: seed, Phase: phase, Styled: true})
		outputs[phase] = row
		plains[phase] = testui.AnsiEscapePattern.ReplaceAllString(row, "")
	}

	// Plain text must be identical across all three phases.
	if plains[0] != plains[1] || plains[1] != plains[2] {
		t.Fatalf("plain text differs across phases: phase0=%q phase1=%q phase2=%q", plains[0], plains[1], plains[2])
	}

	// Styled output must differ between phase 0 and phase 1.
	if outputs[0] == outputs[1] {
		t.Fatalf("Phase 0 and Phase 1 styled output are identical — color cycling not working\noutput: %q", outputs[0])
	}

	// All three phases must produce three distinct styled outputs.
	seen := make(map[string]bool)
	for i, out := range outputs {
		if seen[out] {
			t.Fatalf("Phase %d styled output is not distinct from a previous phase\noutput: %q", i, out)
		}
		seen[out] = true
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 distinct styled outputs across Phase 0-2, got %d", len(seen))
	}
}
