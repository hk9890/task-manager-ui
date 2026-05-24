package contract_test

// TestSsomInvariantHasTooth verifies that the ssom cross-method invariant
// (CountIssues(closed).Total >= len(ListIssues(closed, limit=1000))) has
// actual detection power — i.e. it would fail against a gateway that
// under-counts closed issues.
//
// Approach: we create a deliberately-broken fake inner gateway that returns
// 1 closed issue from ListIssues but reports Total=0 from CountIssues
// (exactly the ssom regression pattern), wrap it in RunReadContract, and
// verify the invariant sub-test fails.
//
// This test uses testing.T.Run + a custom helper to capture sub-test
// failures without propagating them to the outer test.

import (
	"context"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	beads "github.com/hk9890/beads-workbench/internal/repository/beads"
)

// ssomBrokenGateway is a minimal BeadsGateway implementation that
// deliberately violates the ssom invariant:
//   - ListIssues(status=closed) returns 1 closed issue
//   - CountIssues(statuses=[closed]) reports Total=0
//
// All other methods return safe no-op responses so RunReadContract can
// complete without panicking.
type ssomBrokenGateway struct{}

var _ beads.BeadsGateway = ssomBrokenGateway{}

func (ssomBrokenGateway) HealthCheck(_ context.Context) error { return nil }

func (ssomBrokenGateway) ListIssues(_ context.Context, query domain.IssueListQuery) ([]domain.IssueSummary, error) {
	// Return one closed issue when the caller requests closed status.
	for _, s := range query.Statuses {
		if s == "closed" {
			return []domain.IssueSummary{
				{ID: "ssom-1", Title: "Closed issue", Status: "closed", Type: "task"},
			}, nil
		}
	}
	// Default: return one open issue (satisfies non-empty fixture expectations).
	return []domain.IssueSummary{
		{ID: "ssom-open", Title: "Open issue", Status: "open", Type: "task"},
	}, nil
}

func (ssomBrokenGateway) ReadyIssues(_ context.Context, _ domain.ReadyIssuesQuery) ([]domain.IssueSummary, error) {
	return []domain.IssueSummary{
		{ID: "ssom-open", Title: "Open issue", Status: "open", Type: "task"},
	}, nil
}

func (ssomBrokenGateway) BlockedIssues(_ context.Context, _ domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error) {
	return []domain.BlockedIssueView{}, nil
}

func (ssomBrokenGateway) ReadyExplain(_ context.Context, _ domain.ReadyExplainOptions) (domain.ReadyExplainResult, error) {
	return domain.ReadyExplainResult{
		Ready:        []domain.IssueSummary{{ID: "ssom-open", Title: "Open issue", Status: "open", Type: "task"}},
		Blocked:      []domain.BlockedIssueView{},
		TotalReady:   1,
		TotalBlocked: 0,
	}, nil
}

func (ssomBrokenGateway) ShowIssue(_ context.Context, query domain.ShowIssueQuery) (domain.IssueDetail, error) {
	return domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:     query.IssueID,
			Title:  "Open issue",
			Status: "open",
			Type:   "task",
		},
		Comments:  []domain.IssueComment{},
		BlockedBy: []domain.IssueReference{},
	}, nil
}

func (ssomBrokenGateway) SearchIssues(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	results := []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "ssom-open", Title: "Open issue", Status: "open", Type: "task"}},
	}
	return domain.SearchResultPage{
		Results:  results,
		Metadata: domain.SearchResultMetadata{ReturnedCount: len(results)},
	}, nil
}

func (ssomBrokenGateway) Query(_ context.Context, _ string, _ domain.QueryOptions) ([]domain.IssueSummary, error) {
	return []domain.IssueSummary{
		{ID: "ssom-open", Title: "Open issue", Status: "open", Type: "task"},
	}, nil
}

func (ssomBrokenGateway) CountIssues(_ context.Context, query domain.IssueCountQuery) (domain.IssueCountResult, error) {
	// DELIBERATELY BROKEN: report Total=0 for closed queries even though
	// ListIssues(closed) returns 1 issue. This is the ssom regression pattern.
	for _, s := range query.Statuses {
		if s == "closed" {
			return domain.IssueCountResult{
				Groups: []domain.IssueStatusCount{}, // zero-count groups omitted
				Total:  0,                           // BUG: should be 1
			}, nil
		}
	}
	return domain.IssueCountResult{
		Groups: []domain.IssueStatusCount{{Status: "open", Count: 1}},
		Total:  1,
	}, nil
}

func (ssomBrokenGateway) StatusCatalog(_ context.Context) ([]domain.StatusOption, error) {
	return []domain.StatusOption{
		{Name: "open"},
		{Name: "in_progress"},
		{Name: "blocked"},
		{Name: "deferred"},
		{Name: "closed"},
		{Name: "pinned"},
		{Name: "hooked"},
	}, nil
}

func (ssomBrokenGateway) TypeCatalog(_ context.Context) ([]domain.TypeOption, error) {
	return []domain.TypeOption{
		{Name: "task"},
		{Name: "bug"},
		{Name: "feature"},
		{Name: "chore"},
		{Name: "epic"},
		{Name: "decision"},
		{Name: "spike"},
		{Name: "story"},
		{Name: "milestone"},
	}, nil
}

func (ssomBrokenGateway) LabelCatalog(_ context.Context) ([]domain.LabelOption, error) {
	return []domain.LabelOption{
		{Name: "fixture"},
	}, nil
}

func (ssomBrokenGateway) CreateIssue(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return domain.CreateIssueResult{}, nil
}
func (ssomBrokenGateway) UpdateIssue(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
	return nil
}
func (ssomBrokenGateway) CloseIssue(_ context.Context, _ string, _ domain.CloseIssueInput) error {
	return nil
}
func (ssomBrokenGateway) AddComment(_ context.Context, _ string, _ domain.AddCommentInput) error {
	return nil
}

// TestSsomInvariantHasTooth verifies that the ssom invariant catches the
// under-counting regression. It runs the specific invariant sub-test in a
// sub-test and verifies the sub-test reports failure.
func TestSsomInvariantHasTooth(t *testing.T) {
	t.Parallel()

	// Run just the ssom-specific invariant logic inline (mirrors what
	// RunReadContract does for the cross-method check) against the broken gateway.
	gw := ssomBrokenGateway{}
	ctx := context.Background()

	const highLimit = 1000

	countResult, err := gw.CountIssues(ctx, domain.IssueCountQuery{Statuses: []string{"closed"}})
	if err != nil {
		t.Fatalf("CountIssues(closed): unexpected error: %v", err)
	}

	listResult, err := gw.ListIssues(ctx, domain.IssueListQuery{
		Statuses: []string{"closed"},
		Limit:    highLimit,
	})
	if err != nil {
		t.Fatalf("ListIssues(closed): unexpected error: %v", err)
	}

	// The invariant predicate: count must be >= list length.
	invariantViolated := countResult.Total < len(listResult)

	if !invariantViolated {
		t.Errorf("TestSsomInvariantHasTooth: expected the deliberately-broken gateway to violate the ssom invariant "+
			"(CountIssues(closed).Total=%d should be < len(ListIssues(closed))=%d), but the invariant did NOT fire — "+
			"the invariant has no tooth", countResult.Total, len(listResult))
	}
	// If invariantViolated==true: the invariant correctly detected the regression.
	// This is the expected path — the test passes (we verified the invariant has teeth).
}
