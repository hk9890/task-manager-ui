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

// plainLines strips the escapes from each line of a rendered row.
func plainLines(lines []string) []string {
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = testui.AnsiEscapePattern.ReplaceAllString(line, "")
	}
	return plain
}

func TestRenderCompactSelectionAndMetadata(t *testing.T) {
	lines := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "task-manager-ui-u5s",
			Title:    "Row renderer metadata",
			Type:     "task",
			Status:   "open",
			Priority: 1,
		},
		Selected: true,
		Width:    72,
	})

	if len(lines) != Height {
		t.Fatalf("expected %d lines, got %d: %q", Height, len(lines), lines)
	}
	if lines[0] != "▌ T Row renderer metadata" {
		t.Fatalf("expected the selection bar, the type and the title on the first line, got: %q", lines[0])
	}
	if lines[1] != "▌   P1 OPN task-manager-ui-u5s" {
		t.Fatalf("expected the selection bar and the metadata under the title, got: %q", lines[1])
	}
}

func TestRenderCompactKeepsMetadataWhenVeryNarrow(t *testing.T) {
	issue := domain.IssueSummary{
		ID:       "task-manager-ui-very-long-id",
		Title:    "Long title that should not fit",
		Type:     "feature",
		Status:   "in_progress",
		Priority: 0,
	}

	tests := []struct {
		width int
		want  []string
	}{
		// The ID keeps what is left of the second line.
		{width: 15, want: []string{"  F Long title…", "    P0 IP …g-id"}},
		// No cell is left for the ID: the row drops it and keeps the tokens.
		{width: 10, want: []string{"  F Long …", "    P0 IP"}},
	}

	for _, tc := range tests {
		lines := RenderCompact(RenderConfig{Issue: issue, Width: tc.width})

		if len(lines) != len(tc.want) || lines[0] != tc.want[0] || lines[1] != tc.want[1] {
			t.Errorf("width %d: got %q, want %q", tc.width, lines, tc.want)
		}
		for i, line := range lines {
			if lipgloss.Width(line) > tc.width {
				t.Errorf("width %d: line %d is %d cells wide: %q", tc.width, i, lipgloss.Width(line), line)
			}
		}
	}
}

func TestRenderCompactStyledIncludesANSI(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	lines := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "task-manager-ui-u5s",
			Title:    "Styled row",
			Type:     "bug",
			Status:   "blocked",
			Priority: 0,
		},
		Selected: true,
		Width:    64,
		Styled:   true,
	})

	for i, line := range lines {
		if !strings.Contains(line, "\x1b[") {
			t.Fatalf("expected ANSI styling on line %d when Styled is true, got: %q", i, line)
		}
	}
	plain := plainLines(lines)
	if !strings.HasPrefix(plain[0], "▌ B Styled row") || !strings.HasPrefix(plain[1], "▌   P0 BLK task-manager-ui-u5s") {
		t.Fatalf("expected styled metadata to preserve token text, got: %q", plain)
	}
}

// TestRenderCompactStyledSelectedRowFillsItsWidth: the band and the selection
// bar are what join the two lines into one row, so a styled selected row is
// Width cells on both lines and carries the bar on both — under every glyph
// set, because an icon that measured two cells would push one line past the
// other.
func TestRenderCompactStyledSelectedRowFillsItsWidth(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	// Apply writes package variables, so this test is not parallel.
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
		if err := styles.Apply("catppuccin-mocha", "unicode"); err != nil {
			t.Fatalf("restore the initial styles: %v", err)
		}
	})

	issues := []domain.IssueSummary{
		{ID: "task-manager-ui-very-long-id", Title: strings.Repeat("A long title ", 12), Type: "feature", Status: "in_progress", Priority: 0},
		{ID: "TM-1", Title: "Short", Type: "not-a-type", Status: "not-a-status", Priority: 3},
	}

	for _, set := range styles.GlyphSets() {
		if err := styles.Apply("catppuccin-mocha", set); err != nil {
			t.Fatalf("Apply(%q): %v", set, err)
		}
		cursor := styles.Glyphs.Cursor + " "

		for _, issue := range issues {
			for width := 2; width <= 100; width++ {
				lines := RenderCompact(RenderConfig{Issue: issue, Selected: true, Styled: true, Width: width})
				if len(lines) != Height {
					t.Fatalf("%s, width %d: expected %d lines, got %d", set, width, Height, len(lines))
				}
				for i, line := range plainLines(lines) {
					if got := lipgloss.Width(line); got != width {
						t.Errorf("%s, %s, width %d: line %d is %d cells wide: %q", set, issue.ID, width, i, got, line)
					}
					if !strings.HasPrefix(line, cursor) {
						t.Errorf("%s, %s, width %d: line %d does not carry the selection bar: %q", set, issue.ID, width, i, line)
					}
				}
			}
		}
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
					ID:       "task-manager-ui-syf.3",
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
		ID:       "task-manager-ui-9uk",
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

	if !strings.HasPrefix(selectedPlain, "▌ ") {
		t.Fatalf("expected selected row indicator prefix, got %q", selectedPlain)
	}
	if !strings.HasPrefix(idlePlain, "  ") {
		t.Fatalf("expected unselected row idle prefix, got %q", idlePlain)
	}
}

// TestRenderCompactSkeletonWidth verifies that the skeleton is Height lines,
// each opts.Width cells wide, down to the widths where it degrades to bars.
func TestRenderCompactSkeletonWidth(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	for _, w := range []int{1, 4, 5, 11, 12, 30, 50, 80, 120, 200} {
		for seed := 0; seed < 6; seed++ {
			lines := RenderCompactSkeleton(SkeletonOpts{Width: w, Seed: seed, Styled: true})
			if len(lines) != Height {
				t.Fatalf("RenderCompactSkeleton(width=%d): %d lines, want %d", w, len(lines), Height)
			}
			for i, line := range lines {
				if got := lipgloss.Width(line); got != w {
					t.Errorf("RenderCompactSkeleton(width=%d, seed=%d): line %d lipgloss.Width=%d, want %d (line=%q)", w, seed, i, got, w, line)
				}
			}
		}
	}

	// No width draws nothing, and still takes the row's height.
	if lines := RenderCompactSkeleton(SkeletonOpts{Width: 0, Styled: true}); len(lines) != Height || strings.Join(lines, "") != "" {
		t.Errorf("RenderCompactSkeleton(width=0) = %q, want %d empty lines", lines, Height)
	}
}

// skeletonRuns counts the runs of glyph on each line of a skeleton row.
func skeletonRuns(lines []string, glyph string) []int {
	re := regexp.MustCompile(glyph + "+")
	runs := make([]int, len(lines))
	for i, line := range plainLines(lines) {
		runs[i] = len(re.FindAllString(line, -1))
	}
	return runs
}

// TestRenderCompactSkeletonSegmentStructure verifies that the skeleton row has
// the shape of a real one: an "X" type slot and a title bar on the first line,
// then "X" priority and state slots and an ID bar on the second.
func TestRenderCompactSkeletonSegmentStructure(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	for _, styled := range []bool{false, true} {
		lines := RenderCompactSkeleton(SkeletonOpts{Width: 80, Seed: 0, Styled: styled})

		if got := skeletonRuns(lines, SkeletonMetaGlyph); len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Errorf("styled %v: expected %s metadata runs [1 2] (type, then priority and state), got %v in %q",
				styled, SkeletonMetaGlyph, got, plainLines(lines))
		}
		if got := skeletonRuns(lines, SkeletonGlyph); len(got) != 2 || got[0] != 1 || got[1] != 1 {
			t.Errorf("styled %v: expected %s bar runs [1 1] (title, then id), got %v in %q",
				styled, SkeletonGlyph, got, plainLines(lines))
		}
	}
}

// TestRenderCompactSkeletonSixDistinctTitleFills verifies that six successive
// Seed values produce six visibly different title bar widths.
func TestRenderCompactSkeletonSixDistinctTitleFills(t *testing.T) {
	re := regexp.MustCompile(SkeletonGlyph + "+")
	seen := make(map[int]bool)
	for seed := 0; seed < 6; seed++ {
		lines := RenderCompactSkeleton(SkeletonOpts{Width: 80, Seed: seed, Styled: false})
		// The title bar is the bar of the first line.
		runs := re.FindAllString(lines[0], -1)
		if len(runs) != 1 {
			t.Fatalf("seed %d: expected one %s title bar in %q", seed, SkeletonGlyph, lines[0])
		}
		seen[len([]rune(runs[0]))] = true
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
			ID:       "task-manager-ui-u5s",
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
	baseline := strings.Join(RenderCompact(cfg), "\n")

	// Explicit Dim==false must be byte-identical.
	cfg.Dim = false
	cfg.Phase = 1
	got := strings.Join(RenderCompact(cfg), "\n")

	if got != baseline {
		t.Fatalf("Dim==false output differs from baseline:\nbaseline: %q\ngot:      %q", baseline, got)
	}
}

// TestRenderCompactDimAppliesSkeletonShadesForeground verifies that when
// Dim==true && Selected==false both lines carry the SkeletonShades[phase]
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
					ID:       "task-manager-ui-dim1",
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

			// The plain text must still contain the issue content.
			plain := plainLines(dimmed)
			if !strings.Contains(plain[0], "Dim foreground test") || !strings.Contains(plain[1], "task-manager-ui-dim1") {
				t.Fatalf("phase %d: expected title and id in plain text, got: %q", phase, plain)
			}

			// Assert the specific SkeletonShades[phase] ANSI foreground sequence is present.
			// Render a sentinel string with the expected color and extract the escape prefix
			// (everything before the sentinel character) to check it appears in dimmed output.
			sentinel := "\x00"
			rendered := lipgloss.NewStyle().Foreground(skeletonColor(phase)).Render(sentinel)
			ansiPrefix := strings.SplitN(rendered, sentinel, 2)[0]
			for i, line := range dimmed {
				if !strings.Contains(line, ansiPrefix) {
					t.Fatalf("phase %d: expected SkeletonShades[%d] ANSI sequence %q on line %d, got: %q",
						phase, phase, ansiPrefix, i, line)
				}
			}

			// Verify the row with Dim differs from the same row without Dim.
			undimmed := RenderCompact(RenderConfig{
				Issue: domain.IssueSummary{
					ID:       "task-manager-ui-dim1",
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
			for i := range dimmed {
				if dimmed[i] == undimmed[i] {
					t.Fatalf("phase %d: line %d of Dim==true is identical to Dim==false — dim not applied", phase, i)
				}
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
			ID:       "task-manager-ui-sc1",
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

	// The selected-but-not-dimmed row, to compare against.
	selectedOnly := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "task-manager-ui-sc1",
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

	_, styledPrefix := styles.SelectionPrefix(true, true)
	for i, line := range dimmedSelected {
		// The plain text must still contain the selection prefix.
		if plain := testui.AnsiEscapePattern.ReplaceAllString(line, ""); !strings.HasPrefix(plain, "▌ ") {
			t.Fatalf("expected selection prefix '▌ ' on line %d of a dimmed+selected row, got: %q", i, plain)
		}
		// The selection bar keeps its own colour: the tint is on the content only.
		if !strings.Contains(line, strings.TrimSuffix(styledPrefix, " ")) {
			t.Fatalf("expected the selection bar in its own style on line %d, got: %q", i, line)
		}
		// The dimmed+selected row must differ from the selected-but-not-dimmed row.
		if line == selectedOnly[i] {
			t.Fatalf("expected line %d of dimmed+selected to differ from selected-only\ngot: %q", i, line)
		}
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
		row := strings.Join(RenderCompactSkeleton(SkeletonOpts{Width: width, Seed: seed, Phase: phase, Styled: true}), "\n")
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

// TestRenderCompactGivesTheTitleEveryCellAfterTheType pins the title slot at
// its own boundary. This primitive serves both the board columns and the
// search results, so a one-off in the slot cuts a title that fits — or lets a
// row run past its width — on both surfaces at once.
//
// The title has the first line to itself after the gutter, the type and a
// space, so the slot is the same whether the row is styled or not.
func TestRenderCompactGivesTheTitleEveryCellAfterTheType(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	// Uppercase ID and lowercase title, so a lowercase rune is the title.
	issue := domain.IssueSummary{ID: "TM-1", Type: "task", Priority: 1, Status: "open"}
	const chrome = len("  T ")

	for _, styled := range []bool{false, true} {
		titleLine := func(width int, title string) string {
			row := issue
			row.Title = title
			return plainLines(RenderCompact(RenderConfig{Issue: row, Width: width, Styled: styled}))[0]
		}

		for width := chrome + 1; width <= 60; width++ {
			slot := width - chrome

			// A title of exactly the slot renders whole, and one rune longer
			// is truncated. A slot that is off by one fails one of the two.
			whole := strings.Repeat("z", slot)
			if got := titleLine(width, whole); got != "  T "+whole {
				t.Errorf("styled %v, width %d: a %d-rune title was not rendered whole: %q", styled, width, slot, got)
			}
			got := titleLine(width, whole+"z")
			if strings.Contains(got, whole+"z") || lipgloss.Width(got) != width || !strings.HasSuffix(got, "…") {
				t.Errorf("styled %v, width %d: a %d-rune title must be cut to the row width with an ellipsis: %q",
					styled, width, slot+1, got)
			}
		}

		// With no cell left for a title, the line is the type and nothing of
		// the title, and it stays inside the width.
		if got := titleLine(chrome, "zzzz"); strings.Contains(got, "z") || lipgloss.Width(got) > chrome {
			t.Errorf("styled %v, width %d: expected no title and no overflow, got %q", styled, chrome, got)
		}
	}
}
