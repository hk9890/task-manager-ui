package search

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

func TestRenderResultsFirstSearchLayout(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Query:        "backend",
		AppliedQuery: "backend",
		Focus:        FocusResults,
		Results: []domain.IssueSummary{
			{ID: "tm-1", Title: "Backend search result", Status: "open", Type: "task", Priority: 1},
			{ID: "tm-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
		SelectedID: "tm-1",
		Width:      120,
		Height:     28,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	gutter, _ := styles.SelectionPrefix(true, false)

	for _, want := range []string{
		"Search",
		"Results",
		"Content",
		"Metadata",
		"backend",
		"shown",
		"exact",
		"│" + gutter + "T Backend search result",
		"│" + gutter + "  P1 OPN tm-1",
		"│  B Another result",
		"│    P0 IP tm-2",
		"Backend search result",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected %q in view:\n%s", want, plain)
		}
	}
}

func TestRenderShowsEmptyQueryResultsAndPreview(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Focus: FocusResults,
		Results: []domain.IssueSummary{
			{ID: "tm-1", Title: "Default all result", Status: "open", Type: "task", Priority: 1},
			{ID: "tm-2", Title: "Second default", Status: "in_progress", Type: "bug", Priority: 2},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, Completeness: domain.SearchResultCompletenessExact},
		SelectedID: "tm-1",
		Width:      100,
		Height:     24,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	gutter, _ := styles.SelectionPrefix(true, false)
	for _, want := range []string{"│" + gutter + "T Default all result", "│" + gutter + "  P1 OPN tm-1"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected default issue list row line %q, got:\n%s", want, plain)
		}
	}
	if !strings.Contains(plain, "Second default") {
		t.Fatalf("expected second issue row, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Default all result") {
		t.Fatalf("expected preview content for selected result, got:\n%s", plain)
	}
}

// TestRenderResultsRowsCompactTheIDToTheRowWidth pins the ID on a result's
// second line: it has the rest of that line, so it is whole where it fits and
// cut from the front where it does not.
func TestRenderResultsRowsCompactTheIDToTheRowWidth(t *testing.T) {
	t.Parallel()

	state := State{
		Focus: FocusResults,
		Results: []domain.IssueSummary{
			{ID: "task-manager-ui-ultra-wide-width-id", Title: "Result", Status: "open", Type: "task", Priority: 1},
		},
		SelectedID: "task-manager-ui-ultra-wide-width-id",
	}

	for _, tc := range []struct {
		width int
		want  string
	}{
		{width: 60, want: "P1 OPN task-manager-ui-ultra-wide-width-id"},
		{width: 24, want: "P1 OPN …ide-width-id"},
	} {
		lines := renderResultsContent(state, tc.width, resultsPaneHeight(defaultSearchHeight))
		if len(lines) != issuerow.Height {
			t.Fatalf("width %d: got %d lines, want %d", tc.width, len(lines), issuerow.Height)
		}
		plain := testui.AnsiEscapePattern.ReplaceAllString(lines[1], "")
		if strings.TrimSpace(strings.TrimPrefix(plain, styles.Glyphs.Cursor)) != tc.want {
			t.Fatalf("width %d: second line = %q, want the meta and the ID as %q", tc.width, plain, tc.want)
		}
		if got := lipgloss.Width(lines[1]); got > tc.width {
			t.Fatalf("width %d: second line is %d cells wide", tc.width, got)
		}
	}
}

func TestRenderResultsContentUsesSharedIssueRowRenderer(t *testing.T) {
	t.Parallel()

	issue := domain.IssueSummary{ID: "task-manager-ui-u5s", Title: "Shared renderer", Status: "open", Type: "task", Priority: 1}
	lines := renderResultsContent(State{Results: []domain.IssueSummary{issue}, SelectedID: issue.ID}, 60, resultsPaneHeight(defaultSearchHeight))
	want := issuerow.RenderCompact(issuerow.RenderConfig{Issue: issue, Selected: true, Width: 60, Styled: true})
	if len(want) != issuerow.Height {
		t.Fatalf("expected the shared renderer to draw %d lines, got %d", issuerow.Height, len(want))
	}
	if !slices.Equal(lines, want) {
		t.Fatalf("expected exactly one rendered row from the shared renderer\nwant: %q\ngot:  %q", want, lines)
	}
}

func TestRenderResultsUsesStyledSharedRowRenderer(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	view := Render(State{
		Focus: FocusResults,
		Results: []domain.IssueSummary{
			{ID: "task-manager-ui-u5s", Title: "Result", Status: "open", Type: "task", Priority: 0},
		},
		SelectedID: "task-manager-ui-u5s",
		Width:      120,
		Height:     24,
	})

	if !bytes.Contains([]byte(view), []byte("\x1b[")) {
		t.Fatalf("expected ANSI styling in search row output, got: %q", view)
	}
}

func TestRenderShowsErrorInResultsPane(t *testing.T) {
	t.Parallel()

	view := Render(State{Query: "bad", Error: "boom", Width: 100, Height: 24})
	if !strings.Contains(view, "Search failed.") || !strings.Contains(view, "boom") || !strings.Contains(view, "failed") {
		t.Fatalf("expected search error, got:\n%s", view)
	}
}

func TestRenderPreviewUsesSharedContentAndMetadataRendering(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Query: "markdown",
		Focus: FocusContent,
		Results: []domain.IssueSummary{
			{ID: "tm-50", Title: "Markdown preview", Status: "open", Type: "task", Priority: 1},
		},
		SelectedID: "tm-50",
		SelectedDetail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-50", Title: "Markdown preview", Status: "open", Type: "task", Priority: 1},
			Description: "# Header\n\n- item one\n- item two",
		},
		Width:  80,
		Height: 20,
	})

	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	for _, want := range []string{"Content", "Header", "Metadata", "Core"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected %q in compact markdown preview:\n%s", want, plain)
		}
	}
}

func TestRenderGoldens(t *testing.T) {
	t.Parallel()

	t.Run("results_with_preview_w120", func(t *testing.T) {
		view := Render(State{
			Query:        "backend",
			AppliedQuery: "backend",
			Focus:        FocusResults,
			Results: []domain.IssueSummary{
				{ID: "tm-1", Title: "Backend search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "tm-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:       domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
			SelectedID:     "tm-1",
			SelectedDetail: domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Backend search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}}, Description: "Search preview description"},
			Width:          120,
			Height:         28,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "search_results_preview_w120.golden")
	})

	t.Run("results_loading_stub_w120", func(t *testing.T) {
		view := Render(State{
			Query:        "backend",
			AppliedQuery: "backend",
			Focus:        FocusResults,
			Reloading:    true,
			Results: []domain.IssueSummary{
				{ID: "tm-1", Title: "Backend search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "tm-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:      domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessMaybeMore, Notice: "Results may be incomplete because the backend limit may have capped additional matches."},
			SelectedID:    "tm-1",
			DetailLoading: true,
			Width:         120,
			Height:        28,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "search_results_loading_stub_w120.golden")
	})

	t.Run("no_search_yet_w120", func(t *testing.T) {
		view := Render(State{
			Focus:   FocusQuery,
			Results: nil,
			Width:   120,
			Height:  28,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "search_no_search_yet_w120.golden")
	})

	t.Run("no_matches_w120", func(t *testing.T) {
		view := Render(State{
			Query:        "nomatch",
			AppliedQuery: "nomatch",
			Focus:        FocusQuery,
			Results:      nil,
			Metadata:     domain.SearchResultMetadata{ReturnedCount: 0, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
			Width:        120,
			Height:       28,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "search_no_matches_w120.golden")
	})

	t.Run("results_narrow_w80", func(t *testing.T) {
		view := Render(State{
			Query:        "backend",
			AppliedQuery: "backend",
			Focus:        FocusResults,
			Results: []domain.IssueSummary{
				{ID: "task-manager-ui-yze.4.2", Title: "Implement create update close and comment actions in the app", Status: "open", Type: "task", Priority: 1},
				{ID: "task-manager-ui-yze.4.3", Title: "Implement launcher framework with issue-context interpolation", Status: "in_progress", Type: "task", Priority: 1},
			},
			Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
			SelectedID: "task-manager-ui-yze.4.2",
			Width:      80,
			Height:     24,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "search_results_narrow_w80.golden")
	})

	t.Run("results_boundary_w110", func(t *testing.T) {
		view := Render(State{
			Query:        "backend",
			AppliedQuery: "backend",
			Focus:        FocusResults,
			Results: []domain.IssueSummary{
				{ID: "tm-1", Title: "Backend search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "tm-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:       domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
			SelectedID:     "tm-1",
			SelectedDetail: domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Backend search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}}, Description: "Search preview description"},
			Width:          110,
			Height:         28,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "search_results_boundary_w110.golden")
	})

	t.Run("default_all_results_w120", func(t *testing.T) {
		view := Render(State{
			Focus: FocusResults,
			Results: []domain.IssueSummary{
				{ID: "tm-1", Title: "Default all result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "tm-2", Title: "Second default", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:       domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
			SelectedID:     "tm-1",
			SelectedDetail: domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Default all result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}}, Description: "Default preview description"},
			Width:          120,
			Height:         28,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "search_default_all_results_w120.golden")
	})

	// Stale-draft state: Query != AppliedQuery with prior results still visible.
	// Reproduces the "zqx99 typed but 25 repository rows still shown" scenario from
	// the stale-draft indicator scenario.
	t.Run("stale_draft_w120", func(t *testing.T) {
		view := Render(State{
			Query:        "zqx99",
			AppliedQuery: "backend",
			Focus:        FocusQuery,
			Results: []domain.IssueSummary{
				{ID: "tm-1", Title: "Backend search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "tm-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessMaybeMore},
			SelectedID: "tm-1",
			Width:      120,
			Height:     28,
		})

		testui.AssertMatchesGoldenNormalized(t, []byte(view), "search_results_stale_draft_w120.golden")
	})
}

// TestRenderColdStartLoadingShowsSkeletonAndInput verifies that when Loading is
// true and there are no prior results (cold start), the search input is still
// visible and the result area shows skeleton placeholder rows instead of a
// full-screen loading takeover.
func TestRenderColdStartLoadingShowsSkeletonAndInput(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Loading: true,
		Results: nil,
		Query:   "test",
		Width:   120,
		Height:  28,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	// Search input must be visible — no full-screen takeover.
	if !strings.Contains(plain, "Search") {
		t.Fatalf("expected search input box to be visible in cold-start loading state, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Results") {
		t.Fatalf("expected results box to be visible in cold-start loading state, got:\n%s", plain)
	}

	// Skeleton glyph must appear in the result area.
	if !strings.Contains(view, issuerow.SkeletonGlyph) {
		t.Fatalf("expected skeleton glyph %q in cold-start loading state, got:\n%s", issuerow.SkeletonGlyph, view)
	}

	// Row count, not merely presence: DESIGN-GUIDE requires the wait state to
	// fill the surface it stands in for, and "at least one glyph" is satisfied
	// by a single line as much as by a full rail.
	if got, want := countSkeletonLines(plain), coldStartSkeletonRows*issuerow.Height; got != want {
		t.Fatalf("cold start drew %d skeleton lines, want %d rows of %d:\n%s", got, coldStartSkeletonRows, issuerow.Height, plain)
	}
}

// coldStartSkeletonRows is the number of placeholder rows the cold-start rail
// draws. Written as a literal rather than read from the renderer, so a change
// to the constant fails this test instead of moving with it.
const coldStartSkeletonRows = 3

// countSkeletonLines counts rendered lines carrying the skeleton glyph. A
// skeleton row carries it on both of its lines: the title bar and the ID bar.
func countSkeletonLines(plain string) int {
	n := 0
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, issuerow.SkeletonGlyph) {
			n++
		}
	}

	return n
}

// TestRenderRefreshKeepsStaleResults verifies that when Loading is true and
// there are existing results (refresh / reloading state), the stale result
// rows remain visible and skeleton rows are NOT substituted.
func TestRenderRefreshKeepsStaleResults(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Loading:   true,
		Reloading: true,
		Results: []domain.IssueSummary{
			{ID: "tm-1", Title: "Stale Result One", Status: "open", Type: "task", Priority: 1},
			{ID: "tm-2", Title: "Stale Result Two", Status: "in_progress", Type: "bug", Priority: 2},
		},
		SelectedID: "tm-1",
		Width:      120,
		Height:     28,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	// Stale result titles must remain visible.
	if !strings.Contains(plain, "Stale Result One") {
		t.Fatalf("expected stale results to stay visible during refresh, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Stale Result Two") {
		t.Fatalf("expected all stale results to stay visible during refresh, got:\n%s", plain)
	}
}

// TestRenderIdleStateUnchanged is a regression test verifying that when results
// are loaded AND the selected detail is loaded, the view renders normally with
// no skeleton rows.
func TestRenderIdleStateUnchanged(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Loading: false,
		Results: []domain.IssueSummary{
			{ID: "tm-1", Title: "Idle Result", Status: "open", Type: "task", Priority: 1},
		},
		SelectedID: "tm-1",
		// SelectedDetail must match SelectedID; otherwise the detail pane renders
		// a loading skeleton (correct behaviour — detail has not yet been fetched).
		SelectedDetail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-1", Title: "Idle Result", Status: "open", Type: "task", Priority: 1},
			Description: "Idle result description",
		},
		Width:  120,
		Height: 28,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	if !strings.Contains(plain, "Idle Result") {
		t.Fatalf("expected idle state to show results normally, got:\n%s", plain)
	}
	if strings.Contains(view, issuerow.SkeletonGlyph) {
		t.Fatalf("expected no skeleton glyph in fully-loaded idle state, got:\n%s", view)
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests for layout-math functions
// ---------------------------------------------------------------------------

func TestSplitWideWidths(t *testing.T) {
	t.Parallel()

	// For very small totals, the rail/meta floors deliberately overshoot
	// `available`; outputs are pinned to observed behavior.
	tests := []struct {
		name        string
		total       int
		wantRail    int
		wantContent int
		wantMeta    int
	}{
		{name: "zero total", total: 0, wantRail: 12, wantContent: 1, wantMeta: 12},
		{name: "total=1", total: 1, wantRail: 12, wantContent: 1, wantMeta: 12},
		{name: "total=30 (very narrow)", total: 30, wantRail: 12, wantContent: 2, wantMeta: 12},
		{name: "total=60 (adjustment fires)", total: 60, wantRail: 20, wantContent: 20, wantMeta: 16},
		{name: "total=100 (no adjustment)", total: 100, wantRail: 40, wantContent: 22, wantMeta: 34},
		{name: "total=120 (typical)", total: 120, wantRail: 40, wantContent: 42, wantMeta: 34},
		{name: "total=220 (large)", total: 220, wantRail: 64, wantContent: 118, wantMeta: 34},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rail, content, metadata := splitWideWidths(tc.total)
			if rail != tc.wantRail {
				t.Errorf("total=%d: rail=%d, want %d", tc.total, rail, tc.wantRail)
			}
			if content != tc.wantContent {
				t.Errorf("total=%d: content=%d, want %d", tc.total, content, tc.wantContent)
			}
			if metadata != tc.wantMeta {
				t.Errorf("total=%d: metadata=%d, want %d", tc.total, metadata, tc.wantMeta)
			}
		})
	}
}

func TestSplitNarrowWidths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		total     int
		wantLeft  int
		wantRight int
	}{
		{name: "zero total", total: 0, wantLeft: 34, wantRight: 26},
		{name: "total=1", total: 1, wantLeft: 34, wantRight: 26},
		{name: "below minimum sum", total: searchRailMinWidthNarrow + searchRightMinWidthNarrow, wantLeft: 34, wantRight: 26},
		{name: "exact min plus gap", total: searchRailMinWidthNarrow + searchRightMinWidthNarrow + searchColumnGap, wantLeft: 34, wantRight: 26},
		{name: "total=80 (typical narrow)", total: 80, wantLeft: 35, wantRight: 43},
		{name: "total=200 (large)", total: 200, wantLeft: 89, wantRight: 109},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			left, right := splitNarrowWidths(tc.total)
			if left != tc.wantLeft {
				t.Errorf("total=%d: left=%d, want %d", tc.total, left, tc.wantLeft)
			}
			if right != tc.wantRight {
				t.Errorf("total=%d: right=%d, want %d", tc.total, right, tc.wantRight)
			}
		})
	}
}

func TestSplitNarrowRightHeights(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		total        int
		wantContent  int
		wantMetadata int
	}{
		{name: "total=0", total: 0, wantContent: 1, wantMetadata: 1},
		{name: "total=1", total: 1, wantContent: 1, wantMetadata: 1},
		{name: "total=2", total: 2, wantContent: 1, wantMetadata: 1},
		{name: "total=3 (both min branches fire)", total: 3, wantContent: 1, wantMetadata: 6},
		{name: "total=9 (both min branches fire)", total: 9, wantContent: 3, wantMetadata: 6},
		{name: "total=12 (metadata min branch)", total: 12, wantContent: 6, wantMetadata: 6},
		{name: "total=20 (natural split)", total: 20, wantContent: 12, wantMetadata: 8},
		{name: "total=24 (typical)", total: 24, wantContent: 14, wantMetadata: 10},
		{name: "total=60 (large)", total: 60, wantContent: 36, wantMetadata: 24},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			content, metadata := splitNarrowRightHeights(tc.total)
			if content != tc.wantContent {
				t.Errorf("total=%d: content=%d, want %d", tc.total, content, tc.wantContent)
			}
			if metadata != tc.wantMetadata {
				t.Errorf("total=%d: metadata=%d, want %d", tc.total, metadata, tc.wantMetadata)
			}
		})
	}
}

// TestRefreshSearchCarriesDimPhaseStyle verifies that when search is in the
// refresh state (Loading=true, existing results present), the rendered output
// contains the SkeletonShades[phase] ANSI color sequence.
func TestRefreshSearchCarriesDimPhaseStyle(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	const phase = 2 // pick a non-zero phase for a distinct shade

	view := Render(State{
		Loading:   true,
		Reloading: true,
		Results: []domain.IssueSummary{
			{ID: "tm-5", Title: "Stale Search Result", Status: "open", Type: "task", Priority: 1},
		},
		SelectedID:    "tm-5",
		SkeletonPhase: phase,
		Width:         120,
		Height:        28,
	})

	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "Stale Search Result") {
		t.Fatalf("stale result title not visible (ANSI-stripped), got:\n%s", plain)
	}

	// Render a sentinel in the expected shade and take the escape before it.
	const sentinel = "\x00"
	wantANSI, _, _ := strings.Cut(lipgloss.NewStyle().Foreground(styles.SkeletonShades[phase]).Render(sentinel), sentinel)
	if wantANSI == "" {
		t.Fatal("expected the skeleton shade to render an ANSI sequence")
	}
	// Both lines of the result row carry the tint; the selection bar marks them.
	dimmed := 0
	for _, line := range strings.Split(view, "\n") {
		if !strings.Contains(line, styles.Glyphs.Cursor) {
			continue
		}
		dimmed++
		if !strings.Contains(line, wantANSI) {
			t.Fatalf("expected dim ANSI sequence %q in refresh result row line, got:\n%s", wantANSI, line)
		}
	}
	if dimmed != issuerow.Height {
		t.Fatalf("expected %d dimmed result lines, got %d:\n%s", issuerow.Height, dimmed, view)
	}
}

// ---------------------------------------------------------------------------
// Stale-draft indicator tests
// ---------------------------------------------------------------------------

// TestRenderStaleDraftShowsBanner verifies that when the typed draft query
// differs from the last applied query (and no search is in flight), the
// Results pane shows a stale-results banner. The banner is the sole affordance
// for the stale-draft state; there is no "stale" badge in the Results title
// and result rows are not dimmed.
func TestRenderStaleDraftShowsBanner(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Query:        "zqx99",
		AppliedQuery: "backend",
		Focus:        FocusQuery,
		Results: []domain.IssueSummary{
			{ID: "tm-1", Title: "Repository result", Status: "open", Type: "task", Priority: 1},
			{ID: "tm-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, Completeness: domain.SearchResultCompletenessMaybeMore},
		SelectedID: "tm-1",
		Width:      120,
		Height:     28,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	// The stale banner must appear — the prefix fits within the pane width.
	if !strings.Contains(plain, "Results below are stale") {
		t.Fatalf("expected stale-draft banner in results pane, got:\n%s", plain)
	}
	// Prior results must still be visible (not erased).
	if !strings.Contains(plain, "Repository result") {
		t.Fatalf("expected prior results still visible, got:\n%s", plain)
	}
}

// TestRenderStaleDraftBannerTextUntruncated verifies the raw banner text
// (before pane-width truncation) includes the full search instruction and
// the quoted draft query, using renderResultsBanner directly at a wide width.
func TestRenderStaleDraftBannerTextUntruncated(t *testing.T) {
	t.Parallel()

	state := State{
		Query:        "zqx99",
		AppliedQuery: "backend",
		Results: []domain.IssueSummary{
			{ID: "tm-1", Title: "Repository result", Status: "open", Type: "task", Priority: 1},
		},
	}
	banner := renderResultsBanner(state, 200) // wide enough to avoid truncation
	if len(banner) != 1 {
		t.Fatalf("expected exactly one banner line, got %d: %v", len(banner), banner)
	}
	want := `Results below are stale. Press Enter to search for "zqx99".`
	if banner[0] != want {
		t.Fatalf("unexpected banner text\nwant: %q\ngot:  %q", want, banner[0])
	}
}

// TestRenderStaleDraftAbsentWhenApplied verifies that once the search is
// applied (Query == AppliedQuery), neither the stale banner nor the "stale"
// badge appear.
func TestRenderStaleDraftAbsentWhenApplied(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Query:        "backend",
		AppliedQuery: "backend",
		Focus:        FocusResults,
		Results: []domain.IssueSummary{
			{ID: "tm-1", Title: "Repository result", Status: "open", Type: "task", Priority: 1},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 1, Completeness: domain.SearchResultCompletenessExact},
		SelectedID: "tm-1",
		Width:      120,
		Height:     28,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	if strings.Contains(plain, "stale") {
		t.Fatalf("expected no stale indicator when applied==draft, got:\n%s", plain)
	}
	if strings.Contains(plain, "Press Enter to search") {
		t.Fatalf("expected no stale banner when applied==draft, got:\n%s", plain)
	}
}

// TestRenderStaleDraftAbsentWhenSearchInFlight verifies that while a search
// is in flight (Loading=true, prior results visible), the stale-draft banner
// is NOT shown — the "reload" query-box badge already communicates that state.
func TestRenderStaleDraftAbsentWhenSearchInFlight(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Query:        "zqx99",
		AppliedQuery: "backend",
		Loading:      true,
		Reloading:    true, // in-flight: hasDraftChanges is true but isInlineReload is also true
		Results: []domain.IssueSummary{
			{ID: "tm-1", Title: "Repository result", Status: "open", Type: "task", Priority: 1},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 1, Completeness: domain.SearchResultCompletenessMaybeMore},
		SelectedID: "tm-1",
		Width:      120,
		Height:     28,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	// No stale banner: the reload state is handled by the query-badge "reload" affordance.
	if strings.Contains(plain, "Press Enter to search") {
		t.Fatalf("expected no stale banner while search is in flight (reload badge covers it), got:\n%s", plain)
	}
	// "stale" badge must not appear either.
	if strings.Contains(plain, "stale") {
		t.Fatalf("expected no 'stale' badge while search is in flight, got:\n%s", plain)
	}
}

// TestRenderStaleDraftEmptyDraftShowsClearHint verifies that when the draft
// is cleared (empty) but prior results remain (Query="" != AppliedQuery="backend"),
// the banner text includes "Press Enter to clear" rather than an empty quoted draft.
// The test uses renderResultsBanner directly at a wide width to avoid pane truncation.
func TestRenderStaleDraftEmptyDraftShowsClearHint(t *testing.T) {
	t.Parallel()

	state := State{
		Query:        "",
		AppliedQuery: "backend",
		Results: []domain.IssueSummary{
			{ID: "tm-1", Title: "Repository result", Status: "open", Type: "task", Priority: 1},
		},
	}
	banner := renderResultsBanner(state, 200) // wide enough to avoid truncation
	if len(banner) != 1 {
		t.Fatalf("expected exactly one banner line, got %d: %v", len(banner), banner)
	}
	if !strings.Contains(banner[0], "Press Enter to clear") {
		t.Fatalf("expected clear hint in banner when draft is empty, got: %q", banner[0])
	}
}
