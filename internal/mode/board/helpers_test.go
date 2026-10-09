package board

import (
	"context"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// selectedGutter is the gutter the renderer draws down both lines of the
// selected issue.
var selectedGutter, _ = styles.SelectionPrefix(true, false)

// issueCapacity is the number of whole issues a section holds at the current
// height when no divider and no error row takes a line from them.
func issueCapacity(m *Model) int {
	return m.sectionItemCapacity() / issuerow.Height
}

// assertSelectionDrawn fails unless the board draws both lines of the selected
// issue, each behind the selection gutter: the title, and the ID directly
// under it. An offset that leaves either line outside the window fails it.
func assertSelectionDrawn(t *testing.T, m *Model) {
	t.Helper()

	selection := m.currentSelection()
	if selection == nil {
		t.Fatal("assertSelectionDrawn: the board has no selection")
	}
	plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	lines := strings.Split(plain, "\n")
	for idx := 0; idx+1 < len(lines); idx++ {
		if strings.Contains(lines[idx], selectedGutter) && strings.Contains(lines[idx], selection.Issue.Title) &&
			strings.Contains(lines[idx+1], selectedGutter) && strings.Contains(lines[idx+1], selection.Issue.ID) {
			return
		}
	}
	t.Fatalf("the selected issue %s is not drawn with both of its lines:\n%s", selection.Issue.ID, plain)
}

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
