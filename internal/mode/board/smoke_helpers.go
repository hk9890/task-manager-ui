package board

import (
	"github.com/hk9890/beads-workbench/internal/domain"
)

// FeedTestData drives all four gateway result messages into m, simulating a
// completed board load with minimal data. It is the exported equivalent of
// feedAllColumnResults from render_regression_test.go and is used by the
// bwb-smoke render check to populate all 4 columns without a live gateway.
//
// This function is intended for use by the bwb-smoke binary and integration
// tests only; it bypasses the normal async dispatch path.
func FeedTestData(m *Model) {
	m.pendingResults = 4
	_ = m.Update(readyExplainLoadedMsg{result: domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{
			{ID: "smoke-1", Title: "Ready issue", Status: "open", Priority: 1},
		},
		Blocked: []domain.BlockedIssueView{
			{Issue: domain.IssueSummary{ID: "smoke-2", Title: "Blocked", Status: "blocked", Priority: 2}},
		},
	}})
	_ = m.Update(inProgressLoadedMsg{issues: []domain.IssueSummary{
		{ID: "smoke-3", Title: "In Progress", Status: "in_progress", Priority: 1},
	}})
	_ = m.Update(closedLoadedMsg{issues: []domain.IssueSummary{
		{ID: "smoke-4", Title: "Done", Status: "closed", Priority: 3},
	}})
	_ = m.Update(closedCountLoadedMsg{count: 1})
}
