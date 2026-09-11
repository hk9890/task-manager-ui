package nostore

import (
	"context"
	"errors"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// Every operation fails with the code a missing store produces. A read that
// returned empty data instead would render a plausible empty board; a write that
// returned success would lose what the operator typed.
func TestEveryOperationReportsNoStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := New()

	calls := map[string]func() error{
		"Dashboard": func() error { _, err := r.Dashboard(ctx, repository.DashboardOptions{}); return err },
		"Issue":     func() error { _, err := r.Issue(ctx, "x"); return err },
		"Search":    func() error { _, err := r.Search(ctx, domain.SearchIssuesQuery{}); return err },
		"CreateIssue": func() error {
			_, err := r.CreateIssue(ctx, domain.CreateIssueInput{Title: "t"})
			return err
		},
		"UpdateIssue": func() error { return r.UpdateIssue(ctx, "x", domain.UpdateIssueInput{}) },
		"CloseIssue":  func() error { return r.CloseIssue(ctx, "x", domain.CloseIssueInput{}) },
		"AddComment":  func() error { return r.AddComment(ctx, "x", domain.AddCommentInput{}) },
		"HealthCheck": func() error { return r.HealthCheck(ctx) },
		"Catalogs":    func() error { _, err := r.Catalogs(ctx); return err },
	}

	for name, call := range calls {
		var repoErr domain.RepositoryError
		if err := call(); !errors.As(err, &repoErr) || repoErr.Code != domain.ErrorCodeNoDatabaseFound {
			t.Errorf("%s: got %v, want a RepositoryError with code %q", name, err, domain.ErrorCodeNoDatabaseFound)
		}
	}
}
