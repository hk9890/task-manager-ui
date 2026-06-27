package search

// render_regression_test.go — regression harness for pane-stacking bugs in
// the search view (task-manager-ui-o7tk, Bug A class applied to search).
//
// The search layout has 4 bordered panes:
//   - Query box (left rail, top)
//   - Results box (left rail, bottom)
//   - Content pane (right, top in wide / stacked in narrow)
//   - Metadata pane (right, bottom)
//
// Each pane is framed with a rounded border that starts with ╭ at the top-left
// corner.  A correct render has exactly 4 occurrences of ╭.  A count of 8 or
// higher indicates pane stacking (doubled pane headers, the same class of bug
// as the board "doubled column headers" regression).
//
// These tests operate on the search model directly (not via app.Model, since
// internal/app imports internal/mode/search and a reverse import would be
// circular).

import (
	"context"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
)

// countPaneTopBorders counts occurrences of the box-drawing top-left corner
// character (╭) in the rendered view.  The search view has 4 bordered panes
// → expected count = 4.  A count of 8 or higher indicates pane stacking.
func countPaneTopBorders(view string) int {
	return strings.Count(view, "╭")
}

// assertPaneTopCount fails if the view does not contain exactly want top-left
// corner characters.  It prints which lines contain corners for diagnostics.
func assertPaneTopCount(t *testing.T, label, view string, want int) {
	t.Helper()
	got := countPaneTopBorders(view)
	if got == want {
		return
	}
	var report strings.Builder
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "╭") {
			report.WriteString("\n  line ")
			report.WriteString(searchItoa(i))
			report.WriteString(": ")
			report.WriteString(line)
		}
	}
	t.Errorf("%s: expected %d pane-top corners (╭), got %d — pane stacking detected%s",
		label, want, got, report.String())
}

func searchItoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// newRegressionSearch builds a search model with a memory repository seeded
// for search results.
func newRegressionSearch(t *testing.T) *Model {
	t.Helper()
	repo := memoryrepo.New()
	repo.Seed(memoryrepo.Issue{ID: "s-1", Title: "Match one", Status: "open", Priority: 1})
	repo.Seed(memoryrepo.Issue{ID: "s-2", Title: "Match two", Status: "in_progress", Priority: 2})
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	return NewModel(context.Background(), repo, nil, keys)
}

// feedSearchResults delivers a searchLoadedMsg to the model, simulating a
// completed search load.
func feedSearchResults(m *Model) {
	_ = m.Update(searchLoadedMsg{
		appliedQuery: "test",
		page: domain.SearchResultPage{
			Results: []domain.SearchResult{
				{Issue: domain.IssueSummary{ID: "s-1", Title: "Match one", Status: "open", Priority: 1}},
				{Issue: domain.IssueSummary{ID: "s-2", Title: "Match two", Status: "in_progress", Priority: 2}},
			},
		},
	})
}

// TestSearchRenderPaneTopBordersAfterSetSizeNoData — Scenario B (no-data):
// after SetSize at a wide terminal, the empty loading state renders exactly
// 4 pane-top borders.
func TestSearchRenderPaneTopBordersAfterSetSizeNoData(t *testing.T) {
	t.Parallel()

	m := newRegressionSearch(t)
	// Width >= searchWideMinWidth (110) to use wide layout with all 4 panes.
	m.SetSize(160, 40)

	assertPaneTopCount(t, "wide layout no data (160x40)", m.View(0), 4)
}

// TestSearchRenderPaneTopBordersAfterData — Scenario C (data variant):
// after SetSize and a completed search load, the view renders exactly 4
// pane-top borders.
func TestSearchRenderPaneTopBordersAfterData(t *testing.T) {
	t.Parallel()

	m := newRegressionSearch(t)
	m.SetSize(160, 40)
	feedSearchResults(m)

	assertPaneTopCount(t, "wide layout + data (160x40)", m.View(0), 4)
}

// TestSearchRenderPaneTopBordersSmallToLargeResize — Scenario D:
// resize from narrow to wide.  Assert exactly 4 pane-top borders after both
// captures.  Narrow layout also produces 4 pane borders (query + results on
// left; content + metadata stacked on the right).
func TestSearchRenderPaneTopBordersSmallToLargeResize(t *testing.T) {
	t.Parallel()

	m := newRegressionSearch(t)

	// Narrow terminal.
	m.SetSize(80, 30)
	feedSearchResults(m)
	viewNarrow := m.View(0)
	assertPaneTopCount(t, "narrow layout + data (80x30)", viewNarrow, 4)

	// Wide terminal.
	m.SetSize(200, 60)
	feedSearchResults(m)
	viewWide := m.View(0)
	assertPaneTopCount(t, "wide layout + data after resize (200x60)", viewWide, 4)
}

// TestSearchRenderPaneTopBordersPresizeDataResize — Scenario E:
// window size → data → resize → more data → resize.  Assert exactly 4
// pane-top borders after EVERY capture.  Never 8.
func TestSearchRenderPaneTopBordersPresizeDataResize(t *testing.T) {
	t.Parallel()

	m := newRegressionSearch(t)

	// Step 1: size before any data.
	m.SetSize(160, 40)
	assertPaneTopCount(t, "step 1: SetSize before data (160x40)", m.View(0), 4)

	// Step 2: load data.
	feedSearchResults(m)
	assertPaneTopCount(t, "step 2: after data at 160x40", m.View(0), 4)

	// Step 3: resize.
	m.SetSize(200, 60)
	assertPaneTopCount(t, "step 3: after resize to 200x60", m.View(0), 4)

	// Step 4: fresh data after resize.
	feedSearchResults(m)
	assertPaneTopCount(t, "step 4: after data at 200x60", m.View(0), 4)

	// Step 5: resize to narrow.
	m.SetSize(80, 30)
	feedSearchResults(m)
	assertPaneTopCount(t, "step 5: after data at narrow 80x30", m.View(0), 4)
}
