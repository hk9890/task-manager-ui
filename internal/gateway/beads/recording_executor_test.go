package beads

import (
	"context"
	"reflect"
	"sync"
)

// testRecordedCall captures one Run invocation.
type testRecordedCall struct {
	Args []string
}

// testArgsRule holds a configured argv match and its canned response.
type testArgsRule struct {
	args   []string
	result ExecResult
	err    error
}

// testRecordingExecutor implements CommandExecutor, records every Run call,
// and returns canned responses keyed by exact argv match.
//
// This is the package-internal analogue of fakes.RecordingExecutor. It exists
// here (instead of importing fakes) because internal test packages cannot import
// packages that themselves import the package under test (import cycle).
type testRecordingExecutor struct {
	mu sync.RWMutex

	calls         []testRecordedCall
	rules         []testArgsRule
	defaultResult ExecResult
	defaultErr    error
}

var _ CommandExecutor = (*testRecordingExecutor)(nil)

func newTestRecordingExecutor() *testRecordingExecutor {
	return &testRecordingExecutor{}
}

// testArgsRuleBuilder is a fluent builder returned by OnArgs.
type testArgsRuleBuilder struct {
	rec  *testRecordingExecutor
	args []string
}

// OnArgs registers a response rule for an exact argv match.
func (r *testRecordingExecutor) OnArgs(args []string) *testArgsRuleBuilder {
	return &testArgsRuleBuilder{rec: r, args: append([]string(nil), args...)}
}

// Return completes an argv rule.
func (b *testArgsRuleBuilder) Return(result ExecResult, err error) {
	b.rec.mu.Lock()
	defer b.rec.mu.Unlock()

	b.rec.rules = append(b.rec.rules, testArgsRule{
		args:   b.args,
		result: result,
		err:    err,
	})
}

// Run records the invocation and returns the configured response.
func (r *testRecordingExecutor) Run(_ context.Context, _ string, args []string, _ string, _ []string) (ExecResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, testRecordedCall{Args: append([]string(nil), args...)})

	argsCopy := append([]string(nil), args...)
	for _, rule := range r.rules {
		if reflect.DeepEqual(rule.args, argsCopy) {
			return rule.result, rule.err
		}
	}

	return r.defaultResult, r.defaultErr
}

// CallCount returns the number of recorded invocations.
func (r *testRecordingExecutor) CallCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.calls)
}

// Calls returns a snapshot of all recorded invocations.
func (r *testRecordingExecutor) Calls() []testRecordedCall {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return append([]testRecordedCall(nil), r.calls...)
}
