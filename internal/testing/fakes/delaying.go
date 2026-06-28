package fakes

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

// DelayingRepository wraps a repository.Repository and blocks every call to a
// single chosen method (MethodDashboard or MethodSearch) until Release or
// ReleaseAll is called. All other methods delegate immediately.
//
// The blocking mechanism uses a channel gate: each in-flight gated call waits
// for a value on the release channel before proceeding. Release unblocks exactly
// one in-flight call; ReleaseAll unblocks all current and future calls by closing
// the channel. Use InFlight to observe how many gated calls are currently blocked.
//
// Designed for controller-async contract tests that need to verify guard
// behaviour under realistic async conditions (the board model's doneLoadInFlight
// guard via Dashboard, the search controller's in-flight guard via Search).
type DelayingRepository struct {
	inner repository.Repository
	gated Method // the single method that blocks

	mu       sync.Mutex
	release  chan struct{} // current gate; each value unblocks one call
	released bool          // true once ReleaseAll has been called (close)
	inFlight atomic.Int64  // count of gated calls currently blocked
}

// NewDelayingRepository wraps inner and gates the given method (MethodDashboard
// or MethodSearch). Calls to that method block until Release()/ReleaseAll().
func NewDelayingRepository(inner repository.Repository, gated Method) *DelayingRepository {
	return &DelayingRepository{
		inner:   inner,
		gated:   gated,
		release: make(chan struct{}, 64), // buffer so Release() never blocks the test goroutine
	}
}

// NewDelayingDashboardRepository wraps inner and gates Dashboard calls.
func NewDelayingDashboardRepository(inner repository.Repository) *DelayingRepository {
	return NewDelayingRepository(inner, MethodDashboard)
}

// NewDelayingSearchRepository wraps inner and gates Search calls.
func NewDelayingSearchRepository(inner repository.Repository) *DelayingRepository {
	return NewDelayingRepository(inner, MethodSearch)
}

// Release unblocks exactly one in-flight gated call (or permits one future gated
// call to pass through immediately).
func (d *DelayingRepository) Release() {
	d.mu.Lock()
	if d.released {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	d.release <- struct{}{}
}

// ReleaseAll unblocks all current and future gated calls by closing the gate
// channel. After ReleaseAll, any subsequent gated call returns immediately.
func (d *DelayingRepository) ReleaseAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.released {
		d.released = true
		close(d.release)
	}
}

// InFlight returns the number of gated calls currently blocked inside the gate.
func (d *DelayingRepository) InFlight() int {
	return int(d.inFlight.Load())
}

// wait blocks until Release/ReleaseAll when m is the gated method; otherwise it
// returns immediately. It reports ctx cancellation so callers can abort.
func (d *DelayingRepository) wait(ctx context.Context, m Method) error {
	if m != d.gated {
		return nil
	}
	d.inFlight.Add(1)
	defer d.inFlight.Add(-1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.release:
		// Value sent by Release() or channel closed by ReleaseAll(); either way proceed.
		return nil
	}
}

// Dashboard implements repository.Repository; it blocks when MethodDashboard is gated.
func (d *DelayingRepository) Dashboard(ctx context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	if err := d.wait(ctx, MethodDashboard); err != nil {
		return repository.DashboardData{}, err
	}
	return d.inner.Dashboard(ctx, opts)
}

// Search implements repository.Repository; it blocks when MethodSearch is gated.
func (d *DelayingRepository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	if err := d.wait(ctx, MethodSearch); err != nil {
		return domain.SearchResultPage{}, err
	}
	return d.inner.Search(ctx, query)
}

// Remaining methods delegate to the inner repository.

func (d *DelayingRepository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	return d.inner.Issue(ctx, id)
}

func (d *DelayingRepository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return d.inner.CreateIssue(ctx, input)
}

func (d *DelayingRepository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	return d.inner.UpdateIssue(ctx, id, input)
}

func (d *DelayingRepository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	return d.inner.CloseIssue(ctx, id, input)
}

func (d *DelayingRepository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	return d.inner.AddComment(ctx, id, input)
}

func (d *DelayingRepository) HealthCheck(ctx context.Context) error {
	return d.inner.HealthCheck(ctx)
}

func (d *DelayingRepository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	return d.inner.Catalogs(ctx)
}

var _ repository.Repository = (*DelayingRepository)(nil)
