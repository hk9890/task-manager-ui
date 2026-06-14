package issuerow

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
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

// TestRenderCompactSkeletonSegmentStructure verifies that the plain-text skeleton
// row has the expected segment shape: three "X" metadata runs (type/priority/
// state) followed by two SkeletonGlyph bars (id + title).
func TestRenderCompactSkeletonSegmentStructure(t *testing.T) {
	row := RenderCompactSkeleton(SkeletonOpts{Width: 80, Seed: 0, Styled: false})
	// Strip ANSI escapes for the structural check (Styled:false so none expected).
	plain := testui.AnsiEscapePattern.ReplaceAllString(row, "")

	metaRuns := regexp.MustCompile(SkeletonMetaGlyph+"+").FindAllString(plain, -1)
	if len(metaRuns) != 3 {
		t.Errorf("expected 3 %s metadata runs, got %d in %q", SkeletonMetaGlyph, len(metaRuns), plain)
	}
	barRuns := regexp.MustCompile(SkeletonGlyph+"+").FindAllString(plain, -1)
	if len(barRuns) != 2 {
		t.Errorf("expected 2 %s bar runs (id + title), got %d in %q", SkeletonGlyph, len(barRuns), plain)
	}
}

// TestRenderCompactSkeletonStyledSegmentStructure verifies the same structure when Styled:true.
func TestRenderCompactSkeletonStyledSegmentStructure(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	row := RenderCompactSkeleton(SkeletonOpts{Width: 80, Seed: 0, Styled: true})
	plain := testui.AnsiEscapePattern.ReplaceAllString(row, "")
	if got := len(regexp.MustCompile(SkeletonMetaGlyph+"+").FindAllString(plain, -1)); got != 3 {
		t.Errorf("expected 3 %s metadata runs in styled row, got %d in %q", SkeletonMetaGlyph, got, plain)
	}
	if got := len(regexp.MustCompile(SkeletonGlyph+"+").FindAllString(plain, -1)); got != 2 {
		t.Errorf("expected 2 %s bar runs in styled row, got %d in %q", SkeletonGlyph, got, plain)
	}
}

// TestRenderCompactSkeletonSixDistinctTitleFills verifies that six successive
// Seed values produce six visibly different title bar widths.
func TestRenderCompactSkeletonSixDistinctTitleFills(t *testing.T) {
	re := regexp.MustCompile(SkeletonGlyph + "+")
	seen := make(map[int]bool)
	for seed := 0; seed < 6; seed++ {
		row := RenderCompactSkeleton(SkeletonOpts{Width: 80, Seed: seed, Styled: false})
		plain := testui.AnsiEscapePattern.ReplaceAllString(row, "")
		runs := re.FindAllString(plain, -1)
		if len(runs) == 0 {
			t.Fatalf("seed %d: no %s runs found in %q", seed, SkeletonGlyph, plain)
		}
		// The last bar run is the title segment.
		seen[len([]rune(runs[len(runs)-1]))] = true
	}
	if len(seen) != 6 {
		t.Errorf("expected 6 distinct title bar widths across seeds 0-5, got %d: %v", len(seen), seen)
	}
}

// TestSkeletonGlyphConstants verifies the exported placeholder glyph constants.
func TestSkeletonGlyphConstants(t *testing.T) {
	if SkeletonGlyph != "░" {
		t.Errorf("SkeletonGlyph = %q; want %q (U+2591 LIGHT SHADE)", SkeletonGlyph, "░")
	}
	if SkeletonMetaGlyph != "X" {
		t.Errorf("SkeletonMetaGlyph = %q; want %q", SkeletonMetaGlyph, "X")
	}
}

// TestRenderCompactDimFalseIsUnchanged is a regression guard: with Dim==false the
// output must be byte-identical to the baseline (no Dim field set at all).
func TestRenderCompactDimFalseIsUnchanged(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	cfg := RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "beads-workbench-u5s",
			Title:    "Regression guard row",
			Type:     "task",
			Status:   "open",
			Priority: 2,
		},
		Selected: false,
		Width:    72,
		Styled:   true,
	}

	// Baseline: zero-value Dim and Phase (Dim==false is the default).
	baseline := RenderCompact(cfg)

	// Explicit Dim==false must be byte-identical.
	cfg.Dim = false
	cfg.Phase = 1
	got := RenderCompact(cfg)

	if got != baseline {
		t.Fatalf("Dim==false output differs from baseline:\nbaseline: %q\ngot:      %q", baseline, got)
	}
}

// TestRenderCompactDimAppliesSkeletonShadesForeground verifies that when
// Dim==true && Selected==false the output contains the SkeletonShades[phase]
// ANSI sequence as a foreground color code.
func TestRenderCompactDimAppliesSkeletonShadesForeground(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	for phase := 0; phase < len(styles.SkeletonShades); phase++ {
		phase := phase
		t.Run("phase"+string(rune('0'+phase)), func(t *testing.T) {
			dimmed := RenderCompact(RenderConfig{
				Issue: domain.IssueSummary{
					ID:       "beads-workbench-dim1",
					Title:    "Dim foreground test",
					Type:     "task",
					Status:   "open",
					Priority: 1,
				},
				Selected: false,
				Width:    72,
				Styled:   true,
				Dim:      true,
				Phase:    phase,
			})

			if !strings.Contains(dimmed, "\x1b[") {
				t.Fatalf("phase %d: expected ANSI in dimmed row, got: %q", phase, dimmed)
			}

			// The plain text must still contain the issue content.
			plain := testui.AnsiEscapePattern.ReplaceAllString(dimmed, "")
			if !strings.Contains(plain, "Dim foreground test") {
				t.Fatalf("phase %d: expected title in plain text, got: %q", phase, plain)
			}

			// Assert the specific SkeletonShades[phase] ANSI foreground sequence is present.
			// Render a sentinel string with the expected color and extract the escape prefix
			// (everything before the sentinel character) to check it appears in dimmed output.
			sentinel := "\x00"
			rendered := lipgloss.NewStyle().Foreground(skeletonColor(phase)).Render(sentinel)
			ansiPrefix := strings.SplitN(rendered, sentinel, 2)[0]
			if !strings.Contains(dimmed, ansiPrefix) {
				t.Fatalf("phase %d: expected SkeletonShades[%d] ANSI sequence %q in dimmed row, got: %q",
					phase, phase, ansiPrefix, dimmed)
			}

			// Verify the row with Dim differs from the same row without Dim.
			undimmed := RenderCompact(RenderConfig{
				Issue: domain.IssueSummary{
					ID:       "beads-workbench-dim1",
					Title:    "Dim foreground test",
					Type:     "task",
					Status:   "open",
					Priority: 1,
				},
				Selected: false,
				Width:    72,
				Styled:   true,
				Dim:      false,
			})
			if dimmed == undimmed {
				t.Fatalf("phase %d: Dim==true output identical to Dim==false — dim not applied", phase)
			}
		})
	}
}

// TestRenderCompactDimSelectedPreservesSelectionAndDimsForeground verifies the
// selection-conflict rule: when Selected==true && Dim==true the selection
// indicator is preserved in the output AND a dim foreground ANSI code is
// present in the content portion.
func TestRenderCompactDimSelectedPreservesSelectionAndDimsForeground(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	dimmedSelected := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "beads-workbench-sc1",
			Title:    "Selection conflict test",
			Type:     "bug",
			Status:   "blocked",
			Priority: 0,
		},
		Selected: true,
		Width:    72,
		Styled:   true,
		Dim:      true,
		Phase:    0,
	})

	// The plain text must still contain the selection prefix.
	plain := testui.AnsiEscapePattern.ReplaceAllString(dimmedSelected, "")
	if !strings.HasPrefix(plain, "› ") {
		t.Fatalf("expected selection prefix '› ' in dimmed+selected row, got: %q", plain)
	}

	// The styled output must contain ANSI (both selection indicator and dim shade).
	if !strings.Contains(dimmedSelected, "\x1b[") {
		t.Fatalf("expected ANSI in dimmed+selected row, got: %q", dimmedSelected)
	}

	// The dimmed+selected row must differ from the selected-but-not-dimmed row.
	selectedOnly := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "beads-workbench-sc1",
			Title:    "Selection conflict test",
			Type:     "bug",
			Status:   "blocked",
			Priority: 0,
		},
		Selected: true,
		Width:    72,
		Styled:   true,
		Dim:      false,
	})
	if dimmedSelected == selectedOnly {
		t.Fatalf("expected dimmed+selected to differ from selected-only\ngot: %q", dimmedSelected)
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
