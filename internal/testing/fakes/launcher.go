package fakes

import (
	"context"
	"sync"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/launcher"
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
	Calls []LauncherCall
}

var _ launcher.Service = (*FakeLauncher)(nil)

// Launch records the requested action and returns a configured error, if any.
func (f *FakeLauncher) Launch(_ context.Context, action string, issue domain.IssueDetail) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, LauncherCall{Action: action, Issue: issue})
	return f.Err
}
