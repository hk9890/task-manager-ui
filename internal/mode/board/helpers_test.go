package board

import (
	"context"

	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// cannedDashboard answers every Dashboard call with one prepared response and
// leaves the rest of the interface to the embedded nil, which panics if a test
// reaches for a method this package's tests do not drive.
//
// It carries no recording of its own: fakes.ErrorInjectingRepository wraps it
// and records the options each call received. Before Call carried arguments,
// this package held four separate stubs to do that — two of them the same type,
// written 1169 lines apart.
type cannedDashboard struct {
	repository.Repository
	resp repository.DashboardData
}

func (c *cannedDashboard) Dashboard(_ context.Context, _ repository.DashboardOptions) (repository.DashboardData, error) {
	return c.resp, nil
}

// dashboardStub is the board test package's single repository double: a canned
// Dashboard response plus the shared call recorder.
type dashboardStub struct {
	*fakes.ErrorInjectingRepository
}

// newDashboardStub returns a stub answering every Dashboard call with resp. Pass
// the zero value for tests that only care about the options they sent.
func newDashboardStub(resp repository.DashboardData) *dashboardStub {
	return &dashboardStub{ErrorInjectingRepository: fakes.NewErrorInjecting(&cannedDashboard{resp: resp})}
}

// capturedOpts returns the DashboardOptions of every recorded Dashboard call.
func (d *dashboardStub) capturedOpts() []repository.DashboardOptions {
	calls := d.CallsFor(fakes.MethodDashboard)
	out := make([]repository.DashboardOptions, 0, len(calls))
	for _, c := range calls {
		if opts, ok := c.Args.(repository.DashboardOptions); ok {
			out = append(out, opts)
		}
	}
	return out
}

// dashboardCallCount is the number of Dashboard calls recorded so far.
func (d *dashboardStub) dashboardCallCount() int {
	return len(d.CallsFor(fakes.MethodDashboard))
}

var _ repository.Repository = (*dashboardStub)(nil)
