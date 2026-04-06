package search

import (
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

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
