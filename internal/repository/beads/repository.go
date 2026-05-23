// Package beads provides a [repository.Repository] implementation that
// translates the 9-method Repository interface into calls on the existing
// [gateway/beads.BeadsGateway] (CLIGateway). It contains no business logic:
// every bd-specific quirk (arg encoding, workarounds, retry) stays in
// CLIGateway where it is already tested.
//
// # Fan-out methods
//
// Dashboard and Catalogs use [errgroup.WithContext] so their constituent calls
// run in parallel. The first error cancels all remaining in-flight goroutines;
// no partial result is returned (per the Repository contract in hhho.1).
//
// # Closed-issue limit
//
// Dashboard fetches the recently-closed list with a fixed cap of
// [defaultClosedLimit] (50). This matches the floor from board.closedLimit()
// in internal/mode/board, which is max(50, sectionItemCapacity()). The
// dynamic terminal-height-aware portion is handled at the board layer; the
// repository always uses the floor so it is safe to cache the dashboard result
// without knowing the current terminal dimensions.
package beads

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/hk9890/beads-workbench/internal/domain"
	gateway "github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/repository"
)

// defaultClosedLimit is the cap sent to the "status=closed" Query call inside
// Dashboard. It matches the minimum of board.closedLimit() (max(50, capacity))
// so the repository always returns at least 50 recently-closed issues.
const defaultClosedLimit = 50

// Repository wraps a BeadsGateway and implements repository.Repository.
// Construct with [New]; do not create a zero value directly.
type Repository struct {
	gw gateway.BeadsGateway
}

// Compile-time interface assertion.
var _ repository.Repository = (*Repository)(nil)

// New returns a Repository backed by the given gateway.
// gw must be non-nil; passing nil will panic at the first method call.
func New(gw gateway.BeadsGateway) *Repository {
	return &Repository{gw: gw}
}

// Dashboard fans out five gateway calls in parallel and assembles
// [repository.DashboardData]. Any single failure cancels the remaining calls
// and causes Dashboard to return that error; no partial result is returned.
func (r *Repository) Dashboard(ctx context.Context) (repository.DashboardData, error) {
	g, gCtx := errgroup.WithContext(ctx)

	var readyExplain domain.ReadyExplainResult
	var inProgress []domain.IssueSummary
	var closed []domain.IssueSummary
	var closedCount domain.IssueCountResult
	var blocked []domain.IssueSummary

	g.Go(func() error {
		var err error
		readyExplain, err = r.gw.ReadyExplain(gCtx, domain.ReadyExplainOptions{Limit: 0})
		return err
	})

	g.Go(func() error {
		var err error
		inProgress, err = r.gw.Query(gCtx, "status=in_progress", domain.QueryOptions{Limit: 0})
		return err
	})

	g.Go(func() error {
		var err error
		closed, err = r.gw.Query(gCtx, "status=closed", domain.QueryOptions{
			IncludeClosed: true,
			SortBy:        domain.SortFieldClosedAt,
			SortOrder:     domain.SortDirectionDescending,
			Limit:         defaultClosedLimit,
		})
		return err
	})

	g.Go(func() error {
		var err error
		closedCount, err = r.gw.CountIssues(gCtx, domain.IssueCountQuery{
			Statuses: []string{"closed"},
		})
		return err
	})

	g.Go(func() error {
		var err error
		blocked, err = r.gw.Query(gCtx, "status=blocked", domain.QueryOptions{Limit: 0})
		return err
	})

	if err := g.Wait(); err != nil {
		return repository.DashboardData{}, err
	}

	return repository.DashboardData{
		ReadyExplain: readyExplain,
		InProgress:   inProgress,
		Closed:       closed,
		ClosedTotal:  closedCount.Total,
		Blocked:      blocked,
	}, nil
}

// Issue returns full detail for the issue identified by id.
// An unknown id returns a *domain.GatewayError with Code ==
// domain.ErrorCodeCommandFailed (bd's behavior for unknown identifiers).
func (r *Repository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	return r.gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: id})
}

// Search delegates directly to the gateway's SearchIssues method.
func (r *Repository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return r.gw.SearchIssues(ctx, query)
}

// CreateIssue is a 1:1 pass-through to the gateway.
func (r *Repository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return r.gw.CreateIssue(ctx, input)
}

// UpdateIssue is a 1:1 pass-through to the gateway.
func (r *Repository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	return r.gw.UpdateIssue(ctx, id, input)
}

// CloseIssue is a 1:1 pass-through to the gateway.
func (r *Repository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	return r.gw.CloseIssue(ctx, id, input)
}

// AddComment is a 1:1 pass-through to the gateway.
func (r *Repository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	return r.gw.AddComment(ctx, id, input)
}

// HealthCheck delegates directly to the gateway.
func (r *Repository) HealthCheck(ctx context.Context) error {
	return r.gw.HealthCheck(ctx)
}

// Catalogs fans out three gateway calls in parallel and assembles a
// [repository.Catalogs] value. Any single failure cancels the remaining
// calls and causes Catalogs to return that error; no partial result is
// returned.
func (r *Repository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	g, gCtx := errgroup.WithContext(ctx)

	var statuses []domain.StatusOption
	var types []domain.TypeOption
	var labels []domain.LabelOption

	g.Go(func() error {
		var err error
		statuses, err = r.gw.StatusCatalog(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		types, err = r.gw.TypeCatalog(gCtx)
		return err
	})

	g.Go(func() error {
		var err error
		labels, err = r.gw.LabelCatalog(gCtx)
		return err
	})

	if err := g.Wait(); err != nil {
		return repository.Catalogs{}, err
	}

	return repository.Catalogs{
		Statuses: statuses,
		Types:    types,
		Labels:   labels,
	}, nil
}
