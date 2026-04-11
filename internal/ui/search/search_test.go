package search

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
	"github.com/hk9890/beads-workbench/internal/ui/shared/issuerow"
	"github.com/muesli/termenv"
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
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

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
	if !strings.Contains(plain, "…de-width-id") {
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
	if !strings.Contains(view, "Search failed") || !strings.Contains(view, "boom") {
		t.Fatalf("expected search error, got:\n%s", view)
	}
}

func TestRenderPreviewUsesCompactMarkdownDetailRendering(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Query: "markdown",
		Focus: FocusPreview,
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
	for _, want := range []string{"Preview", "Header", "Metadata", "Core", "Counts"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected %q in compact markdown preview:\n%s", want, plain)
		}
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

	t.Run("default_all_results_w120", func(t *testing.T) {
		view := Render(State{
			Focus: FocusResults,
			Results: []domain.IssueSummary{
				{ID: "bw-1", Title: "Default all result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}},
				{ID: "bw-2", Title: "Second default", Status: "in_progress", Type: "bug", Priority: 0},
			},
			SelectedID:     "bw-1",
			SelectedDetail: domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Default all result", Status: "open", Type: "task", Priority: 1, Assignee: "hans", Labels: []string{"ui"}}, Description: "Default preview description"},
			Width:          120,
			Height:         28,
		})

		assertGoldenNormalized(t, []byte(view), "search_default_all_results_w120.golden")
	})
}
