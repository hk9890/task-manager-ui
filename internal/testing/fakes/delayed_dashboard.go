package fakes

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
)

// DelayedDashboardRepository wraps a repository.Repository and blocks every
// call to Dashboard until Release (or ReleaseAll) is called. All other methods
// are delegated immediately.
//
// The blocking mechanism uses a channel gate: each in-flight Dashboard call
// waits for a value to be sent on the release channel before proceeding.
// Release unblocks exactly one in-flight call; ReleaseAll unblocks all current
// and future calls by closing the channel.
//
// Use InFlight to observe how many Dashboard calls are currently blocked.
//
// Designed for controller-async contract tests that need to verify guard
// behaviour under realistic async conditions (e.g. board model's
// doneLoadInFlight guard). Compare with the search-package-local
// DelayedFakeRepository, which delays Search calls only.
type DelayedDashboardRepository struct {
	inner repository.Repository

	mu       sync.Mutex
	release  chan struct{} // current gate; each value unblocks one call
	released bool         // true once ReleaseAll has been called (close)
	inFlight atomic.Int64 // count of Dashboard calls currently blocked
}

// NewDelayedDashboardRepository creates a DelayedDashboardRepository wrapping
// inner. Calls to Dashboard block until Release() or ReleaseAll() is invoked.
func NewDelayedDashboardRepository(inner repository.Repository) *DelayedDashboardRepository {
	return &DelayedDashboardRepository{
		inner:   inner,
		release: make(chan struct{}, 64), // buffer so Release() never blocks the test goroutine
	}
}

// Release unblocks exactly one in-flight Dashboard call (or permits one future
// Dashboard call to pass through immediately).
func (d *DelayedDashboardRepository) Release() {
	d.mu.Lock()
	if d.released {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	d.release <- struct{}{}
}

// ReleaseAll unblocks all current and future Dashboard calls by closing the
// gate channel. After ReleaseAll, any subsequent Dashboard call returns
// immediately.
func (d *DelayedDashboardRepository) ReleaseAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.released {
		d.released = true
		close(d.release)
	}
}

// InFlight returns the number of Dashboard calls currently blocked inside the
// gate.
func (d *DelayedDashboardRepository) InFlight() int {
	return int(d.inFlight.Load())
}

// Dashboard implements repository.Repository. It blocks until Release or
// ReleaseAll is called, then delegates to the inner repository.
func (d *DelayedDashboardRepository) Dashboard(ctx context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	d.inFlight.Add(1)
	defer d.inFlight.Add(-1)

	// Block until released, or context cancels.
	// When the channel is closed (ReleaseAll), the receive completes
	// immediately — that is the intended pass-through behaviour.
	select {
	case <-ctx.Done():
		return repository.DashboardData{}, ctx.Err()
	case <-d.release:
		// Either a value sent by Release() or the channel was closed by ReleaseAll();
		// in both cases we proceed to the inner call.
	}

	return d.inner.Dashboard(ctx, opts)
}

// Delegate all non-Dashboard methods to the inner repository.

func (d *DelayedDashboardRepository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	return d.inner.Issue(ctx, id)
}

func (d *DelayedDashboardRepository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return d.inner.Search(ctx, query)
}

func (d *DelayedDashboardRepository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return d.inner.CreateIssue(ctx, input)
}

func (d *DelayedDashboardRepository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	return d.inner.UpdateIssue(ctx, id, input)
}

func (d *DelayedDashboardRepository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	return d.inner.CloseIssue(ctx, id, input)
}

func (d *DelayedDashboardRepository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	return d.inner.AddComment(ctx, id, input)
}

func (d *DelayedDashboardRepository) HealthCheck(ctx context.Context) error {
	return d.inner.HealthCheck(ctx)
}

func (d *DelayedDashboardRepository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	return d.inner.Catalogs(ctx)
}

var _ repository.Repository = (*DelayedDashboardRepository)(nil)
