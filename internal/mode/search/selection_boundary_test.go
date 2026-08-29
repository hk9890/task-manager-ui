package search

import (
	"context"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
)

// resultsPage builds a page of n results with predictable IDs.
func resultsPage(n int) domain.SearchResultPage {
	results := make([]domain.SearchResult, n)
	for i := range results {
		id := "row-" + string(rune('a'+i))
		results[i] = domain.SearchResult{Issue: domain.IssueSummary{ID: id, Title: "Row " + id, Status: "open"}}
	}

	return domain.SearchResultPage{Results: results}
}

func modelWithResults(t *testing.T, n int) *Model {
	t.Helper()

	m := NewModel(context.Background(), memoryrepo.New(), nil)
	m.SetSize(160, 40)
	_ = m.Update(searchLoadedMsg{page: resultsPage(n)})

	return m
}

// TestNormalizeSelectionClampsTheRowExactlyPastTheEnd pins the upper clamp at
// its own boundary. Every larger overshoot is caught by the same branch, so the
// exactly-one-past case is the only input that distinguishes >= from >: at that
// row currentSelection returns nil, which blanks the content and metadata panes
// and sends a nil selection to the shell while the results list still renders.
func TestNormalizeSelectionClampsTheRowExactlyPastTheEnd(t *testing.T) {
	t.Parallel()

	const n = 3
	cases := []struct {
		name     string
		selected int
		want     int
	}{
		{"exactly one past the end", n, n - 1},
		{"far past the end", n + 7, n - 1},
		{"last row is left alone", n - 1, n - 1},
		{"negative row clamps to the first", -2, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := modelWithResults(t, n)
			m.selectedRow = tc.selected
			m.normalizeSelection()

			if m.selectedRow != tc.want {
				t.Fatalf("selectedRow = %d, want %d", m.selectedRow, tc.want)
			}
			if m.currentSelection() == nil {
				t.Fatalf("currentSelection is nil at row %d: the pane would render no selection", m.selectedRow)
			}
		})
	}
}

// TestMoveSelectionDownOffTheLastRowKeepsASelection drives the same boundary
// through the movement path the operator actually takes.
func TestMoveSelectionDownOffTheLastRowKeepsASelection(t *testing.T) {
	t.Parallel()

	const n = 3
	m := modelWithResults(t, n)
	m.selectedRow = n - 1

	m.moveSelection(1)

	if m.selectedRow != n-1 {
		t.Fatalf("selectedRow = %d after moving down off the last row, want %d", m.selectedRow, n-1)
	}
	if got := m.selectedIssueID(); got == "" {
		t.Fatal("selectedIssueID is empty after moving down off the last row")
	}
}

// TestSelectionSurvivesAPageExactlyOneShorter covers the other way the row can
// land exactly at len(Results): a re-search returning one fewer row than the
// current index.
func TestSelectionSurvivesAPageExactlyOneShorter(t *testing.T) {
	t.Parallel()

	m := modelWithResults(t, 4)
	m.selectedRow = 3

	_ = m.Update(searchLoadedMsg{page: resultsPage(3)})

	if m.selectedRow != 2 {
		t.Fatalf("selectedRow = %d after the page shrank by one, want 2", m.selectedRow)
	}
	if m.currentSelection() == nil {
		t.Fatal("currentSelection is nil after the page shrank by one")
	}
}

// TestResultCountPrefersTheBackendCountAndFallsBackToTheRows pins the exported
// accessor the shell header reads. No test called it, so a backend or metadata
// change leaving ReturnedCount at 0 would report "no results" against a pane
// that visibly lists rows.
func TestResultCountPrefersTheBackendCountAndFallsBackToTheRows(t *testing.T) {
	t.Parallel()

	t.Run("metadata count is used when present", func(t *testing.T) {
		t.Parallel()

		m := modelWithResults(t, 3)
		page := resultsPage(3)
		// A backend may report a count that is not the row count of this page;
		// the metadata is the authority when it is set.
		page.Metadata.ReturnedCount = 2
		_ = m.Update(searchLoadedMsg{page: page})

		if got := m.ResultCount(); got != 2 {
			t.Fatalf("ResultCount = %d, want the metadata count 2", got)
		}
	})

	t.Run("row count is used when metadata is absent", func(t *testing.T) {
		t.Parallel()

		m := modelWithResults(t, 3)

		if got := m.ResultCount(); got != 3 {
			t.Fatalf("ResultCount = %d, want the row count 3", got)
		}
	})

	t.Run("empty page counts zero", func(t *testing.T) {
		t.Parallel()

		m := modelWithResults(t, 0)

		if got := m.ResultCount(); got != 0 {
			t.Fatalf("ResultCount = %d, want 0", got)
		}
	})
}
