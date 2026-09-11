// Package nostore is the repository the app holds while no store is open.
//
// Startup with nothing to resolve used to exit. It now starts on the store
// picker, and the shell still needs a repository to build its services and
// surfaces around. This one answers every call with ErrorCodeNoDatabaseFound —
// the code a store that is not there already produces — so anything that does
// reach it fails loudly instead of reading or writing into nothing.
package nostore

import (
	"context"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// Repository is a repository.Repository with no store behind it.
type Repository struct{}

var _ repository.Repository = Repository{}

// New returns a Repository.
func New() Repository { return Repository{} }

func noStore(op string) error {
	return domain.RepositoryError{
		Code:      domain.ErrorCodeNoDatabaseFound,
		Operation: op,
		Message:   "no task-manager store is open",
	}
}

func (Repository) Dashboard(context.Context, repository.DashboardOptions) (repository.DashboardData, error) {
	return repository.DashboardData{}, noStore("dashboard")
}

func (Repository) Issue(context.Context, string) (domain.IssueDetail, error) {
	return domain.IssueDetail{}, noStore("issue")
}

func (Repository) Search(context.Context, domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return domain.SearchResultPage{}, noStore("search")
}

func (Repository) CreateIssue(context.Context, domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return domain.CreateIssueResult{}, noStore("create issue")
}

func (Repository) UpdateIssue(context.Context, string, domain.UpdateIssueInput) error {
	return noStore("update issue")
}

func (Repository) CloseIssue(context.Context, string, domain.CloseIssueInput) error {
	return noStore("close issue")
}

func (Repository) AddComment(context.Context, string, domain.AddCommentInput) error {
	return noStore("add comment")
}

func (Repository) HealthCheck(context.Context) error {
	return noStore("health check")
}

func (Repository) Catalogs(context.Context) (repository.Catalogs, error) {
	return repository.Catalogs{}, noStore("catalogs")
}
