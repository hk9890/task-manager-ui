package board

import (
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// FeedTestData drives a single dashboard result message into m, simulating a
// completed board load with minimal data. It is the exported equivalent of
// feedDashboard from render_regression_test.go and is used by the taskmgr-ui-smoke
// render check to populate all 4 columns without a live repository.
//
// This function is intended for use by the taskmgr-ui-smoke binary and integration
// tests only; it bypasses the normal async dispatch path.
func FeedTestData(m *Model) {
	_ = m.Update(dashboardLoadedMsg{data: repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{
				{ID: "smoke-1", Title: "Ready issue", Status: "open", Priority: 1},
			},
			Blocked: []domain.BlockedIssueView{
				{Issue: domain.IssueSummary{ID: "smoke-2", Title: "Blocked", Status: "blocked", Priority: 2}},
			},
		},
		InProgress: []domain.IssueSummary{
			{ID: "smoke-3", Title: "In Progress", Status: "in_progress", Priority: 1},
		},
		Closed: []domain.IssueSummary{
			{ID: "smoke-4", Title: "Done", Status: "closed", Priority: 3},
		},
		ClosedTotal: 1,
	}})
}
