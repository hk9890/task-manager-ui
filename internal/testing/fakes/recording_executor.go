package fakes

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/hk9890/beads-workbench/internal/gateway/beads"
)

// RecordedCall captures one CommandExecutor.Run invocation.
type RecordedCall struct {
	Command string
	Args    []string
	WorkDir string
	EnvLen  int
	IsWrite bool
	At      time.Time
}

// argsRule holds a configured argv match and its canned response.
type argsRule struct {
	args   []string
	result beads.ExecResult
	err    error
}

// RecordingExecutor implements beads.CommandExecutor and records every Run call.
//
// Usage:
//
//	rec := fakes.NewRecordingExecutor()
//	rec.OnArgs([]string{"ping"}).Return(beads.ExecResult{Stdout: []byte("pong")}, nil)
//	runner := beads.NewCommandRunner(beads.RunnerConfig{Executor: rec})
//
// Calls() returns all recorded invocations in order; safe to call concurrently.
// If no argv rule matches the default result is used.
type RecordingExecutor struct {
	mu sync.RWMutex

	calls         []RecordedCall
	rules         []argsRule
	defaultResult beads.ExecResult
	defaultErr    error
}

var _ beads.CommandExecutor = (*RecordingExecutor)(nil)

// NewRecordingExecutor returns a RecordingExecutor with a default zero ExecResult.
func NewRecordingExecutor() *RecordingExecutor {
	return &RecordingExecutor{}
}

// SetDefault configures the result returned when no argv rule matches.
func (r *RecordingExecutor) SetDefault(result beads.ExecResult, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.defaultResult = result
	r.defaultErr = err
}

// argsRuleBuilder is a fluent builder returned by OnArgs.
type argsRuleBuilder struct {
	rec  *RecordingExecutor
	args []string
}

// OnArgs registers a response rule for an exact argv match.
// Call Return on the returned builder to complete the rule.
func (r *RecordingExecutor) OnArgs(args []string) *argsRuleBuilder {
	return &argsRuleBuilder{rec: r, args: append([]string(nil), args...)}
}

// Return completes an argv rule, associating result and err with the argv.
func (b *argsRuleBuilder) Return(result beads.ExecResult, err error) {
	b.rec.mu.Lock()
	defer b.rec.mu.Unlock()

	b.rec.rules = append(b.rec.rules, argsRule{
		args:   b.args,
		result: result,
		err:    err,
	})
}

// Run records the invocation and returns the configured response.
// IsWrite is inferred from CommandRequest.IsWrite via the runner's mutex
// contract, but the executor itself cannot observe that field — callers that
// need per-call IsWrite tracking should set it via RecordedCall.IsWrite
// explicitly after the fact, or use a wrapper. For now IsWrite is always false
// here because CommandExecutor.Run does not receive CommandRequest.
func (r *RecordingExecutor) Run(_ context.Context, command string, args []string, workDir string, env []string) (beads.ExecResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, RecordedCall{
		Command: command,
		Args:    append([]string(nil), args...),
		WorkDir: workDir,
		EnvLen:  len(env),
		At:      time.Now(),
	})

	argsCopy := append([]string(nil), args...)
	for _, rule := range r.rules {
		if reflect.DeepEqual(rule.args, argsCopy) {
			return rule.result, rule.err
		}
	}

	return r.defaultResult, r.defaultErr
}

// Calls returns a snapshot of all recorded invocations in call order.
// Safe to call concurrently with Run.
func (r *RecordingExecutor) Calls() []RecordedCall {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return append([]RecordedCall(nil), r.calls...)
}

// CallCount returns the number of recorded invocations. Safe to call concurrently.
func (r *RecordingExecutor) CallCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.calls)
}

// ResetCalls clears recorded invocations while keeping configured rules and defaults.
func (r *RecordingExecutor) ResetCalls() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = nil
}
