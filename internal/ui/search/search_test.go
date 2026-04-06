package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/testing/ui"
)

func assertGoldenNormalized(t *testing.T, output []byte, name string) {
	t.Helper()

	if os.Getenv("BWB_UPDATE_GOLDEN") == "1" {
		path := filepath.Join("testdata", name)
		if err := os.WriteFile(path, output, 0o600); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
	}

	ui.AssertMatchesGoldenNormalized(t, output, name)
}

func TestRenderResultsFirstSearchLayout(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Query: "gateway",
		Focus: FocusResults,
		Results: []domain.IssueSummary{
			{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1},
			{ID: "bw-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
		},
		SelectedID: "bw-1",
		Width:      120,
		Height:     28,
	})

	for _, want := range []string{
		"Search",
		"Results",
		"Preview",
		"gateway",
		"live",
		"› T P1 OPN bw-1 Gateway search result",
		"B P0 IP bw-2 Another result",
		"Gateway search result",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q in view:\n%s", want, view)
		}
	}
}

func TestRenderShowsEmptyQueryHelp(t *testing.T) {
	t.Parallel()

	view := Render(State{Width: 100, Height: 24})
	if !strings.Contains(view, "Start typing to search issues.") {
		t.Fatalf("expected empty-query help, got:\n%s", view)
	}
	if !strings.Contains(view, "│") {
		t.Fatalf("expected caret-like empty query field, got:\n%s", view)
	}
	if !strings.Contains(view, "Selected issue preview") {
		t.Fatalf("expected preview empty state, got:\n%s", view)
	}
}

func TestRenderShowsErrorInResultsPane(t *testing.T) {
	t.Parallel()

	view := Render(State{Query: "bad", Error: "boom", Width: 100, Height: 24})
	if !strings.Contains(view, "Search failed") || !strings.Contains(view, "boom") {
		t.Fatalf("expected search error, got:\n%s", view)
	}
}

func TestRenderGoldens(t *testing.T) {
	t.Parallel()

	t.Run("results_with_preview_w120", func(t *testing.T) {
		view := Render(State{
			Query: "gateway",
			Focus: FocusResults,
			Results: []domain.IssueSummary{
				{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "bw-2", Title: "Another result", Status: "in_progress", Type: "bug", Priority: 0},
			},
			SelectedID:     "bw-1",
			SelectedDetail: domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Gateway search result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}}, Description: "Search preview description"},
			Width:          120,
			Height:         28,
		})

		assertGoldenNormalized(t, []byte(view), "search_results_preview_w120.golden")
	})

	t.Run("empty_results_w120", func(t *testing.T) {
		view := Render(State{
			Query:   "nomatch",
			Focus:   FocusQuery,
			Results: nil,
			Width:   120,
			Height:  28,
		})

		assertGoldenNormalized(t, []byte(view), "search_empty_results_w120.golden")
	})

	t.Run("results_narrow_w80", func(t *testing.T) {
		view := Render(State{
			Query: "gateway",
			Focus: FocusResults,
			Results: []domain.IssueSummary{
				{ID: "beads-workbench-yze.4.2", Title: "Implement create update close and comment actions in the app", Status: "open", Type: "task", Priority: 1},
				{ID: "beads-workbench-yze.4.3", Title: "Implement launcher framework with issue-context interpolation", Status: "in_progress", Type: "task", Priority: 1},
			},
			SelectedID: "beads-workbench-yze.4.2",
			Width:      80,
			Height:     24,
		})

		assertGoldenNormalized(t, []byte(view), "search_results_narrow_w80.golden")
	})
}
