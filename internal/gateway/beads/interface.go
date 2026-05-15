package beads

import (
	"context"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// BeadsGateway is the source-specific beads gateway used by the Beads Workbench UI.
// A gateway instance is bound to one beads source/project.
type BeadsGateway interface {
	// HealthCheck verifies that the bd CLI is reachable. Returns an error with
	// ErrorCodeCommandUnavailable if bd is not installed or not in PATH.
	HealthCheck(ctx context.Context) error

	ListIssues(ctx context.Context, query domain.IssueListQuery) ([]domain.IssueSummary, error)
	ReadyIssues(ctx context.Context, query domain.ReadyIssuesQuery) ([]domain.IssueSummary, error)
	BlockedIssues(ctx context.Context, query domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error)
	ShowIssue(ctx context.Context, query domain.ShowIssueQuery) (domain.IssueDetail, error)
	SearchIssues(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error)

	CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error)
	UpdateIssue(ctx context.Context, issueID string, input domain.UpdateIssueInput) error
	CloseIssue(ctx context.Context, issueID string, input domain.CloseIssueInput) error
	AddComment(ctx context.Context, issueID string, input domain.AddCommentInput) error

	StatusCatalog(ctx context.Context) ([]domain.StatusOption, error)
	TypeCatalog(ctx context.Context) ([]domain.TypeOption, error)
	LabelCatalog(ctx context.Context) ([]domain.LabelOption, error)
}
