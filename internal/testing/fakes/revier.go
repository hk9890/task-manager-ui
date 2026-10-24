package fakes

import (
	"context"
	"sync"

	"github.com/hk9890/task-manager-ui/internal/revier"
)

// RevierCall captures one call to the revier client: the method and its
// argument (the root, the project or the address).
type RevierCall struct {
	Method string
	Arg    string
}

// FakeRevierClient is a deterministic seam for the revier client.
type FakeRevierClient struct {
	mu sync.Mutex

	ProjectName string
	ProjectErr  error
	AgentList   []revier.Agent
	AgentsErr   error
	FocusErr    error
	calls       []RevierCall
}

var _ revier.Client = (*FakeRevierClient)(nil)

func (f *FakeRevierClient) record(method, arg string) {
	f.calls = append(f.calls, RevierCall{Method: method, Arg: arg})
}

// Project records the call and returns ProjectName, or ProjectErr.
func (f *FakeRevierClient) Project(_ context.Context, root string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Project", root)
	if f.ProjectErr != nil {
		return "", f.ProjectErr
	}
	return f.ProjectName, nil
}

// Agents records the call and returns a copy of AgentList, or AgentsErr.
func (f *FakeRevierClient) Agents(_ context.Context, project string) ([]revier.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Agents", project)
	if f.AgentsErr != nil {
		return nil, f.AgentsErr
	}
	return append([]revier.Agent(nil), f.AgentList...), nil
}

// Focus records the call and returns FocusErr.
func (f *FakeRevierClient) Focus(_ context.Context, addr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("Focus", addr)
	return f.FocusErr
}

// Calls returns a snapshot of the calls recorded so far, in order.
func (f *FakeRevierClient) Calls() []RevierCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RevierCall(nil), f.calls...)
}
