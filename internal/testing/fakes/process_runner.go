package fakes

import (
	"context"
	"sync"

	"github.com/hk9890/task-manager-ui/internal/launcher"
)

// ProcessRunCall captures one external process run request.
type ProcessRunCall struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
}

// FakeProcessRunner is a deterministic seam for launcher process execution.
type FakeProcessRunner struct {
	mu sync.Mutex

	Err   error
	calls []ProcessRunCall
}

var _ launcher.ProcessRunner = (*FakeProcessRunner)(nil)

// Run records process launch intent and returns a configured result.
func (f *FakeProcessRunner) Run(_ context.Context, command string, args []string, dir string, env []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, ProcessRunCall{
		Command: command,
		Args:    append([]string(nil), args...),
		Dir:     dir,
		Env:     append([]string(nil), env...),
	})

	return f.Err
}

// Calls returns a snapshot of the calls recorded so far, in order. It takes
// the same lock the recording path does, so a test reading it while the
// component under test is still running cannot race.
func (f *FakeProcessRunner) Calls() []ProcessRunCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ProcessRunCall, len(f.calls))
	copy(out, f.calls)
	return out
}
