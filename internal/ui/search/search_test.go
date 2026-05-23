package search

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hk9890/beads-workbench/internal/domain"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
	"github.com/hk9890/beads-workbench/internal/ui/shared/issuerow"
)

func assertGoldenNormalized(t *testing.T, output []byte, name string) {
	t.Helper()

	if os.Getenv("BWB_UPDATE_GOLDEN") == "1" {
		path := filepath.Join("testdata", name)
		if err := os.WriteFile(path, output, 0o600); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
	}

	testui.AssertMatchesGoldenNormalized(t, output, name)
}

func TestRenderResultsFirstSearchLayout(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Query:        "gateway",
		AppliedQuery: "gateway",
		Focus:        FocusResults,
		Results: []domain.IssueSummary{
			{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1},
			{ID: "bw-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
		SelectedID: "bw-1",
		Width:      120,
		Height:     28,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	for _, want := range []string{
		"Search",
		"Results",
		"Content",
		"Metadata",
		"gateway",
		"shown",
		"exact",
		"› T P1 OPN bw-1 Gateway search result",
		"B P0 IP bw-2 Another result",
		"Gateway search result",
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
			{ID: "bw-1", Title: "Default all result", Status: "open", Type: "task", Priority: 1},
			{ID: "bw-2", Title: "Second default", Status: "in_progress", Type: "bug", Priority: 2},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, Completeness: domain.SearchResultCompletenessExact},
		SelectedID: "bw-1",
		Width:      100,
		Height:     24,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "› T P1 OPN bw-1 Default all result") {
		t.Fatalf("expected default issue list row, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Second default") {
		t.Fatalf("expected second issue row, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Default all result") {
		t.Fatalf("expected preview content for selected result, got:\n%s", plain)
	}
}

func TestRenderResultsRowsApplySharedIDWidthCap(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Focus: FocusResults,
		Results: []domain.IssueSummary{
			{ID: "beads-workbench-ultra-wide-width-id", Title: "Result", Status: "open", Type: "task", Priority: 1},
		},
		SelectedID: "beads-workbench-ultra-wide-width-id",
		Width:      220,
		Height:     24,
	})

	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "…") || !strings.Contains(plain, "width-id") {
		t.Fatalf("expected capped compact issue id suffix in search results, got:\n%s", plain)
	}
}

func TestRenderResultsContentUsesSharedIssueRowRenderer(t *testing.T) {
	t.Parallel()

	issue := domain.IssueSummary{ID: "beads-workbench-u5s", Title: "Shared renderer", Status: "open", Type: "task", Priority: 1}
	lines := renderResultsContent(State{Results: []domain.IssueSummary{issue}, SelectedID: issue.ID}, 60)
	if len(lines) != 1 {
		t.Fatalf("expected exactly one rendered row, got %d", len(lines))
	}

	want := issuerow.RenderCompact(issuerow.RenderConfig{Issue: issue, Selected: true, Width: 60, Styled: true})
	if lines[0] != want {
		t.Fatalf("expected results row to use shared renderer\nwant: %q\ngot:  %q", want, lines[0])
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
			{ID: "beads-workbench-u5s", Title: "Result", Status: "open", Type: "task", Priority: 0},
		},
		SelectedID: "beads-workbench-u5s",
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
			{ID: "bw-50", Title: "Markdown preview", Status: "open", Type: "task", Priority: 1},
		},
		SelectedID: "bw-50",
		SelectedDetail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-50", Title: "Markdown preview", Status: "open", Type: "task", Priority: 1},
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
			Query:        "gateway",
			AppliedQuery: "gateway",
			Focus:        FocusResults,
			Results: []domain.IssueSummary{
				{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "bw-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:       domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
			SelectedID:     "bw-1",
			SelectedDetail: domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}}, Description: "Search preview description"},
			Width:          120,
			Height:         28,
		})

		assertGoldenNormalized(t, []byte(view), "search_results_preview_w120.golden")
	})

	t.Run("results_loading_stub_w120", func(t *testing.T) {
		view := Render(State{
			Query:        "gateway",
			AppliedQuery: "gateway",
			Focus:        FocusResults,
			Reloading:    true,
			Results: []domain.IssueSummary{
				{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "bw-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:      domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessMaybeMore, Notice: "Results may be incomplete because the backend limit may have capped additional matches."},
			SelectedID:    "bw-1",
			DetailLoading: true,
			Width:         120,
			Height:        28,
		})

		assertGoldenNormalized(t, []byte(view), "search_results_loading_stub_w120.golden")
	})

	t.Run("no_search_yet_w120", func(t *testing.T) {
		view := Render(State{
			Focus:   FocusQuery,
			Results: nil,
			Width:   120,
			Height:  28,
		})

		assertGoldenNormalized(t, []byte(view), "search_no_search_yet_w120.golden")
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

		assertGoldenNormalized(t, []byte(view), "search_no_matches_w120.golden")
	})

	t.Run("results_narrow_w80", func(t *testing.T) {
		view := Render(State{
			Query:        "gateway",
			AppliedQuery: "gateway",
			Focus:        FocusResults,
			Results: []domain.IssueSummary{
				{ID: "beads-workbench-yze.4.2", Title: "Implement create update close and comment actions in the app", Status: "open", Type: "task", Priority: 1},
				{ID: "beads-workbench-yze.4.3", Title: "Implement launcher framework with issue-context interpolation", Status: "in_progress", Type: "task", Priority: 1},
			},
			Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
			SelectedID: "beads-workbench-yze.4.2",
			Width:      80,
			Height:     24,
		})

		assertGoldenNormalized(t, []byte(view), "search_results_narrow_w80.golden")
	})

	t.Run("results_boundary_w110", func(t *testing.T) {
		view := Render(State{
			Query:        "gateway",
			AppliedQuery: "gateway",
			Focus:        FocusResults,
			Results: []domain.IssueSummary{
				{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "bw-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:       domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
			SelectedID:     "bw-1",
			SelectedDetail: domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}}, Description: "Search preview description"},
			Width:          110,
			Height:         28,
		})

		assertGoldenNormalized(t, []byte(view), "search_results_boundary_w110.golden")
	})

	t.Run("default_all_results_w120", func(t *testing.T) {
		view := Render(State{
			Focus: FocusResults,
			Results: []domain.IssueSummary{
				{ID: "bw-1", Title: "Default all result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "bw-2", Title: "Second default", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:       domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessExact},
			SelectedID:     "bw-1",
			SelectedDetail: domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Default all result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}}, Description: "Default preview description"},
			Width:          120,
			Height:         28,
		})

		assertGoldenNormalized(t, []byte(view), "search_default_all_results_w120.golden")
	})

	// Stale-draft state: Query != AppliedQuery with prior results still visible.
	// Reproduces the "zqx99 typed but 25 gateway rows still shown" scenario from
	// ticket beads-workbench-2ev4.4.
	t.Run("stale_draft_w120", func(t *testing.T) {
		view := Render(State{
			Query:        "zqx99",
			AppliedQuery: "gateway",
			Focus:        FocusQuery,
			Results: []domain.IssueSummary{
				{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "bw-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
			},
			Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessMaybeMore},
			SelectedID: "bw-1",
			Width:      120,
			Height:     28,
		})

		assertGoldenNormalized(t, []byte(view), "search_results_stale_draft_w120.golden")
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
			{ID: "bw-1", Title: "Stale Result One", Status: "open", Type: "task", Priority: 1},
			{ID: "bw-2", Title: "Stale Result Two", Status: "in_progress", Type: "bug", Priority: 2},
		},
		SelectedID: "bw-1",
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
			{ID: "bw-1", Title: "Idle Result", Status: "open", Type: "task", Priority: 1},
		},
		SelectedID: "bw-1",
		// SelectedDetail must match SelectedID; otherwise the detail pane renders
		// a loading skeleton (correct behaviour — detail has not yet been fetched).
		SelectedDetail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-1", Title: "Idle Result", Status: "open", Type: "task", Priority: 1},
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
			{ID: "bw-5", Title: "Stale Search Result", Status: "open", Type: "task", Priority: 1},
		},
		SelectedID:    "bw-5",
		SkeletonPhase: phase,
		Width:         120,
		Height:        28,
	})

	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "Stale Search Result") {
		t.Fatalf("stale result title not visible (ANSI-stripped), got:\n%s", plain)
	}

	// SkeletonShades[2] dark = "#7F7F7F" → RGB(127,127,127) → ANSI 38;2;127;127;127
	const wantANSI = "38;2;127;127;127"
	if !strings.Contains(view, wantANSI) {
		t.Fatalf("expected dim ANSI sequence %q in refresh result row, got:\n%s", wantANSI, view)
	}
}

// ---------------------------------------------------------------------------
// Stale-draft indicator tests (beads-workbench-2ev4.4)
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
		AppliedQuery: "gateway",
		Focus:        FocusQuery,
		Results: []domain.IssueSummary{
			{ID: "bw-1", Title: "Gateway result", Status: "open", Type: "task", Priority: 1},
			{ID: "bw-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 2, Completeness: domain.SearchResultCompletenessMaybeMore},
		SelectedID: "bw-1",
		Width:      120,
		Height:     28,
	})
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

	// The stale banner must appear — the prefix fits within the pane width.
	if !strings.Contains(plain, "Results below are stale") {
		t.Fatalf("expected stale-draft banner in results pane, got:\n%s", plain)
	}
	// Prior results must still be visible (not erased).
	if !strings.Contains(plain, "Gateway result") {
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
		AppliedQuery: "gateway",
		Results: []domain.IssueSummary{
			{ID: "bw-1", Title: "Gateway result", Status: "open", Type: "task", Priority: 1},
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
		Query:        "gateway",
		AppliedQuery: "gateway",
		Focus:        FocusResults,
		Results: []domain.IssueSummary{
			{ID: "bw-1", Title: "Gateway result", Status: "open", Type: "task", Priority: 1},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 1, Completeness: domain.SearchResultCompletenessExact},
		SelectedID: "bw-1",
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
		AppliedQuery: "gateway",
		Loading:      true,
		Reloading:    true, // in-flight: hasDraftChanges is true but isInlineReload is also true
		Results: []domain.IssueSummary{
			{ID: "bw-1", Title: "Gateway result", Status: "open", Type: "task", Priority: 1},
		},
		Metadata:   domain.SearchResultMetadata{ReturnedCount: 1, Completeness: domain.SearchResultCompletenessMaybeMore},
		SelectedID: "bw-1",
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
// is cleared (empty) but prior results remain (Query="" != AppliedQuery="gateway"),
// the banner text includes "Press Enter to clear" rather than an empty quoted draft.
// The test uses renderResultsBanner directly at a wide width to avoid pane truncation.
func TestRenderStaleDraftEmptyDraftShowsClearHint(t *testing.T) {
	t.Parallel()

	state := State{
		Query:        "",
		AppliedQuery: "gateway",
		Results: []domain.IssueSummary{
			{ID: "bw-1", Title: "Gateway result", Status: "open", Type: "task", Priority: 1},
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
