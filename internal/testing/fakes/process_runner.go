package fakes

import (
	"context"
	"sync"

	"github.com/hk9890/beads-workbench/internal/launcher"
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
	Calls []ProcessRunCall
}

var _ launcher.ProcessRunner = (*FakeProcessRunner)(nil)

// Run records process launch intent and returns a configured result.
func (f *FakeProcessRunner) Run(_ context.Context, command string, args []string, dir string, env []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, ProcessRunCall{
		Command: command,
		Args:    append([]string(nil), args...),
		Dir:     dir,
		Env:     append([]string(nil), env...),
	})

	return f.Err
}

// ResetCalls clears recorded invocations.
func (f *FakeProcessRunner) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = nil
}
