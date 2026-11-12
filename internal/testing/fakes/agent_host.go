package fakes

import (
	"context"
	"sync"

	"github.com/hk9890/task-manager-ui/internal/agent"
)

// AgentHostCall captures one call to the agent host: the method, its
// argument (the root for List, the session for Resume, the agent's Ref for
// the rest), the Spec of a Start and the force of a Close.
type AgentHostCall struct {
	Method string
	Arg    string
	Spec   agent.Spec
	Force  bool
}

// FakeAgentHost is a deterministic seam for agent.Host. Start returns
// Started, and adds it to Agents so a later List sees it; with StartErr set
// it returns Started with the error and adds nothing, as a start that failed
// after the agent opened does.
type FakeAgentHost struct {
	mu sync.Mutex

	Agents    []agent.Agent
	ListErr   error
	Started   agent.Agent
	StartErr  error
	FocusErr  error
	Resumed   agent.Agent
	ResumeErr error
	WaitErr   error
	Said      string
	ReadErr   error
	CloseErr  error
	calls     []AgentHostCall
}

var _ agent.Host = (*FakeAgentHost)(nil)

func (f *FakeAgentHost) record(call AgentHostCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

// Start records the call and returns Started, with StartErr when it is set.
func (f *FakeAgentHost) Start(_ context.Context, spec agent.Spec) (agent.Agent, error) {
	f.record(AgentHostCall{Method: "Start", Arg: spec.Root, Spec: spec})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.StartErr != nil {
		return f.Started, f.StartErr
	}
	f.Agents = append(f.Agents, f.Started)
	return f.Started, nil
}

// List records the call and returns a copy of Agents, or ListErr.
func (f *FakeAgentHost) List(_ context.Context, root string) ([]agent.Agent, error) {
	f.record(AgentHostCall{Method: "List", Arg: root})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ListErr != nil {
		return nil, f.ListErr
	}
	return append([]agent.Agent(nil), f.Agents...), nil
}

// Focus records the call and returns FocusErr.
func (f *FakeAgentHost) Focus(_ context.Context, a agent.Agent) error {
	f.record(AgentHostCall{Method: "Focus", Arg: a.Ref})
	return f.FocusErr
}

// Resume records the call and returns Resumed, or ResumeErr.
func (f *FakeAgentHost) Resume(_ context.Context, _, session string) (agent.Agent, error) {
	f.record(AgentHostCall{Method: "Resume", Arg: session})
	if f.ResumeErr != nil {
		return agent.Agent{}, f.ResumeErr
	}
	return f.Resumed, nil
}

// Wait records the call and returns WaitErr.
func (f *FakeAgentHost) Wait(_ context.Context, a agent.Agent) error {
	f.record(AgentHostCall{Method: "Wait", Arg: a.Ref})
	return f.WaitErr
}

// Read records the call and returns Said, or ReadErr.
func (f *FakeAgentHost) Read(_ context.Context, a agent.Agent) (string, error) {
	f.record(AgentHostCall{Method: "Read", Arg: a.Ref})
	return f.Said, f.ReadErr
}

// Close records the call and returns CloseErr.
func (f *FakeAgentHost) Close(_ context.Context, a agent.Agent, force bool) error {
	f.record(AgentHostCall{Method: "Close", Arg: a.Ref, Force: force})
	return f.CloseErr
}

// Calls returns a snapshot of the calls recorded so far, in order.
func (f *FakeAgentHost) Calls() []AgentHostCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]AgentHostCall(nil), f.calls...)
}
