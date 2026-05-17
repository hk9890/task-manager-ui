package board

// render_regression_test.go — regression harness for the "doubled column-top
// borders" and "frame-stacking" bug classes (beads-workbench-o7tk).
//
// These tests operate on the board model directly (not via app.Model, since
// internal/app imports internal/mode/board and a reverse import would be
// circular).  The app-level sizeKnown guard is tested separately in
// internal/app/render_regression_test.go.
//
// This harness catches the frame-stacking class: if the board rendering
// machinery were changed so that column headers accumulated across calls, the
// countColumnTopBorders helper would catch it immediately.

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

// countColumnTopBorders counts occurrences of the box-drawing top-left corner
// character (╭) in the rendered view.  A correct full-board render at a wide
// enough terminal has exactly 4 occurrences — one per column header.  A count
// of 8 or higher indicates frame stacking (doubled column headers).
func countColumnTopBorders(view string) int {
	return strings.Count(view, "╭")
}

// assertColumnTopCount fails if the view does not contain exactly want
// top-left corner characters.  It produces a human-readable diagnostic
// showing which lines contain corners.
func assertColumnTopCount(t *testing.T, label, view string, want int) {
	t.Helper()
	got := countColumnTopBorders(view)
	if got == want {
		return
	}
	var report strings.Builder
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "╭") {
			report.WriteString("\n  line ")
			report.WriteString(itoa(i))
			report.WriteString(": ")
			report.WriteString(line)
		}
	}
	t.Errorf("%s: expected %d column-top corners (╭), got %d — frame stacking detected%s",
		label, want, got, report.String())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// newRegressionBoard builds a board model with all 4 columns populated.
func newRegressionBoard(t *testing.T) *Model {
	t.Helper()
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{
			{ID: "reg-1", Title: "Ready issue", Status: "open", Priority: 1},
		},
		Blocked: []domain.BlockedIssueView{
			{Issue: domain.IssueSummary{ID: "reg-2", Title: "Blocked", Status: "blocked", Priority: 2}},
		},
	}
	gateway.QueryResponse = []domain.IssueSummary{
		{ID: "reg-3", Title: "In Progress", Status: "in_progress", Priority: 1},
	}
	gateway.CountIssuesResponse = domain.IssueCountResult{
		Total: 10,
		Groups: []domain.IssueStatusCount{
			{Status: "closed", Count: 10},
		},
	}
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	return NewModel(gateway, slog.Default(), keys)
}

// feedAllColumnResults feeds all four gateway result messages to the model,
// simulating a completed board load.
func feedAllColumnResults(m *Model) {
	m.pendingResults = 4
	_ = m.Update(readyExplainLoadedMsg{result: domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{
			{ID: "reg-1", Title: "Ready issue", Status: "open", Priority: 1},
		},
		Blocked: []domain.BlockedIssueView{
			{Issue: domain.IssueSummary{ID: "reg-2", Title: "Blocked", Status: "blocked", Priority: 2}},
		},
	}})
	_ = m.Update(inProgressLoadedMsg{issues: []domain.IssueSummary{
		{ID: "reg-3", Title: "In Progress", Status: "in_progress", Priority: 1},
	}})
	_ = m.Update(closedLoadedMsg{issues: []domain.IssueSummary{
		{ID: "reg-4", Title: "Done", Status: "closed", Priority: 3},
	}})
	_ = m.Update(closedCountLoadedMsg{count: 1})
}

// TestBoardRenderColumnTopBordersAfterWindowSizeMsg — Scenario B
// (no-data variant): after SetSize, the empty loading state renders exactly
// 4 column-top borders.
//
// If sizeKnown were absent at this layer and an additional default-size frame
// were composited, this count would jump to 8.
func TestBoardRenderColumnTopBordersAfterWindowSizeMsg(t *testing.T) {
	t.Parallel()

	m := newRegressionBoard(t)
	// Width=180 ensures all 4 columns are visible.
	m.SetSize(180, 30)

	view := m.View(0)
	assertColumnTopCount(t, "after SetSize(180,30) no data", view, 4)
}

// TestBoardRenderColumnTopBordersAfterData — Scenario C (data variant):
// after SetSize and a full data load, the board renders exactly 4
// column-top borders.  A count of 8 would indicate doubled headers.
func TestBoardRenderColumnTopBordersAfterData(t *testing.T) {
	t.Parallel()

	m := newRegressionBoard(t)
	m.SetSize(180, 30)
	feedAllColumnResults(m)

	view := m.View(0)
	assertColumnTopCount(t, "after SetSize(180,30) + data", view, 4)
}

// TestBoardRenderColumnTopBordersSmallToLargeResize — Scenario D:
// resize from a small to a large terminal.  Both captures must show exactly
// 4 column-top borders.
//
// The pre-fix code would produce a doubled header when the post-resize frame
// tried to overwrite the smaller frame.
func TestBoardRenderColumnTopBordersSmallToLargeResize(t *testing.T) {
	t.Parallel()

	m := newRegressionBoard(t)

	// Small terminal: not all 4 columns visible (width=80 shows ~3 columns).
	m.SetSize(80, 20)
	feedAllColumnResults(m)
	viewSmall := m.View(0)
	// At 80 wide we may only see 3 columns — don't assert count here.
	_ = viewSmall

	// Wide terminal: all 4 columns visible.
	m.SetSize(200, 60)
	// Feed fresh data at the new size.
	feedAllColumnResults(m)
	viewLarge := m.View(0)
	assertColumnTopCount(t, "after resize to 200x60 + fresh data", viewLarge, 4)
}

// TestBoardRenderColumnTopBordersPresizeDataResize — Scenario E:
// window size → data → resize → more data → resize.  Assert exactly 4
// column-top borders after EVERY View() capture.  Never 8.
func TestBoardRenderColumnTopBordersPresizeDataResize(t *testing.T) {
	t.Parallel()

	m := newRegressionBoard(t)

	// Step 1: set size before any data.
	m.SetSize(180, 30)
	// Board model renders 4 loading column headers.
	assertColumnTopCount(t, "step 1: SetSize before data (180x30)", m.View(0), 4)

	// Step 2: load data.
	feedAllColumnResults(m)
	assertColumnTopCount(t, "step 2: after data at 180x30", m.View(0), 4)

	// Step 3: resize.
	m.SetSize(200, 60)
	assertColumnTopCount(t, "step 3: after resize to 200x60", m.View(0), 4)

	// Step 4: load fresh data after resize.
	feedAllColumnResults(m)
	assertColumnTopCount(t, "step 4: after data at 200x60", m.View(0), 4)

	// Step 5: resize again to a different wide size (must be ≥180 for all 4
	// columns to fit; at 160 the renderer shows only 3).
	m.SetSize(180, 40)
	feedAllColumnResults(m)
	assertColumnTopCount(t, "step 5: after data at 180x40", m.View(0), 4)
}
