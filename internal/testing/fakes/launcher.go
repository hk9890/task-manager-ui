package fakes

import (
	"context"
	"sync"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/launcher"
)

// LauncherCall captures one launcher invocation.
type LauncherCall struct {
	Action string
	Issue  domain.IssueDetail
}

// FakeLauncher is a deterministic launcher.Service test seam.
type FakeLauncher struct {
	mu sync.Mutex

	Err   error
	calls []LauncherCall
}

var _ launcher.Service = (*FakeLauncher)(nil)

// Launch records the requested action and returns a configured error, if any.
func (f *FakeLauncher) Launch(_ context.Context, action string, issue domain.IssueDetail) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, LauncherCall{Action: action, Issue: issue})
	return f.Err
}

// Calls returns a snapshot of the calls recorded so far, in order. It takes
// the same lock the recording path does, so a test reading it while the
// component under test is still running cannot race.
func (f *FakeLauncher) Calls() []LauncherCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]LauncherCall, len(f.calls))
	copy(out, f.calls)
	return out
}
