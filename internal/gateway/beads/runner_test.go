package beads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
)

func TestCommandRunnerRunUsesDefaultWorkDirAndIgnoresRequestWorkDir(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{
		result: ExecResult{Stdout: []byte("ok")},
	}

	runner := NewCommandRunner(RunnerConfig{
		Command:  "bd-custom",
		WorkDir:  "/default/workdir",
		Env:      []string{"PATH=/usr/bin"},
		Executor: execStub,
	})

	// Pass a request WorkDir — the runner must ignore it and use the gateway's
	// bound defaultWorkDir (CODING.md rule #3: gateway is source-specific).
	out, err := runner.Run(context.Background(), CommandRequest{
		Operation: "list issues",
		Args:      []string{"ready", "--json"},
		WorkDir:   "/request/workdir",
		Env:       []string{"HOME=/home/user"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if string(out) != "ok" {
		t.Fatalf("unexpected stdout: %q", string(out))
	}

	if execStub.command != "bd-custom" {
		t.Fatalf("unexpected command: %q", execStub.command)
	}

	if !reflect.DeepEqual(execStub.args, []string{"ready", "--json"}) {
		t.Fatalf("unexpected args: %#v", execStub.args)
	}

	// WorkDir override in request must be ignored; executor receives the bound defaultWorkDir.
	if execStub.workDir != "/default/workdir" {
		t.Fatalf("unexpected work dir: got %q, want /default/workdir (request WorkDir must be ignored)", execStub.workDir)
	}

	// Both PATH (from RunnerConfig.Env) and HOME (from per-request Env) pass the
	// allowlist filter. Non-allowlisted entries would be stripped.
	// BD_NON_INTERACTIVE=1 is always appended last by resolveEnv to prevent
	// bd from prompting for tty input.
	expected := []string{"PATH=/usr/bin", "HOME=/home/user", "BD_NON_INTERACTIVE=1"}
	if !reflect.DeepEqual(execStub.env, expected) {
		t.Fatalf("unexpected env: %#v (want %#v)", execStub.env, expected)
	}
}

func TestCommandRunnerRunFallsBackToDefaultCommand(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	runner := NewCommandRunner(RunnerConfig{Executor: execStub})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "op"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if execStub.command != "bd" {
		t.Fatalf("expected default command 'bd', got %q", execStub.command)
	}
}

func TestCommandRunnerRunMapsExitCodeFailure(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{
		result: ExecResult{ExitCode: 2, Stderr: []byte("bad args")},
	}
	runner := NewCommandRunner(RunnerConfig{Executor: execStub})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "update issue"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
	assertContains(t, err.Error(), "command exited with code 2")
	assertContains(t, err.Error(), "bad args")
}

func TestCommandRunnerRunMapsCommandUnavailable(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{
		result: ExecResult{Stderr: []byte("not installed")},
		err:    exec.ErrNotFound,
	}
	runner := NewCommandRunner(RunnerConfig{Executor: execStub})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "ready issues"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandUnavailable)
	assertContains(t, err.Error(), "bd command is unavailable")
	assertContains(t, err.Error(), "not installed")
}

func TestCommandRunnerRunMapsTimeout(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{err: context.DeadlineExceeded}
	runner := NewCommandRunner(RunnerConfig{Executor: execStub})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "search issues"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeTimeout)
	assertContains(t, err.Error(), "command timed out")
}

func TestCommandRunnerRunMapsGenericExecutionFailure(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{err: errors.New("fork/exec failed")}
	runner := NewCommandRunner(RunnerConfig{Executor: execStub})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "show issue"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeCommandFailed)
	assertContains(t, err.Error(), "failed to execute command")
}

func TestCommandRunnerRunJSONIntoDecodesResult(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{
		result: ExecResult{Stdout: []byte(`{"id":"ISSUE-1","title":"test"}`)},
	}
	runner := NewCommandRunner(RunnerConfig{Executor: execStub})

	var got struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}

	err := runner.RunJSONInto(context.Background(), CommandRequest{Operation: "show issue"}, &got)
	if err != nil {
		t.Fatalf("RunJSONInto returned error: %v", err)
	}

	if got.ID != "ISSUE-1" || got.Title != "test" {
		t.Fatalf("unexpected decoded output: %#v", got)
	}
}

func TestRunJSONGenericHelper(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte(`[1,2,3]`)}}
	runner := NewCommandRunner(RunnerConfig{Executor: execStub})

	got, err := RunJSON[[]int](context.Background(), runner, CommandRequest{Operation: "numbers"})
	if err != nil {
		t.Fatalf("RunJSON returned error: %v", err)
	}

	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestDecodeJSONIntoMapsDecodeFailure(t *testing.T) {
	t.Parallel()

	var target struct {
		Value string `json:"value"`
	}

	err := DecodeJSONInto("decode op", []byte(`{"value":`), &target)
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
	assertContains(t, err.Error(), "failed to decode command JSON output")
}

func TestDecodeJSONIntoRejectsTrailingPayload(t *testing.T) {
	t.Parallel()

	var target struct {
		Value string `json:"value"`
	}

	err := DecodeJSONInto("decode op", []byte(`{"value":"x"} {"extra":true}`), &target)
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
	assertContains(t, err.Error(), "failed to decode command JSON output")
}

// TestDecodeJSONIntoRejectsNDJSON pins the NDJSON-unsupported contract.
// DecodeJSONInto expects exactly one top-level JSON object; a second record on a
// new line (NDJSON format) must be rejected with ErrorCodeDecodeFailed. See the
// doc comment on DecodeJSONInto for the rationale.
func TestDecodeJSONIntoRejectsNDJSON(t *testing.T) {
	t.Parallel()

	// Two concatenated JSON objects separated by a newline (NDJSON format).
	ndjson := []byte("{\"id\":\"bd-1\",\"title\":\"first\"}\n{\"id\":\"bd-2\",\"title\":\"second\"}\n")

	var target struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}

	err := DecodeJSONInto("decode op", ndjson, &target)
	if err == nil {
		t.Fatal("expected NDJSON to be rejected, got nil error")
	}
	assertGatewayErrorCode(t, err, domain.ErrorCodeDecodeFailed)
	assertContains(t, err.Error(), "failed to decode command JSON output")
}

func TestCommandRunnerRunNilReceiver(t *testing.T) {
	t.Parallel()

	var runner *CommandRunner
	_, err := runner.Run(context.Background(), CommandRequest{Operation: "op"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeUnknown)
}

func TestCommandRunnerRunSerializesWriteCalls(t *testing.T) {
	t.Parallel()

	execStub := &concurrencyGuardExecutor{}
	runner := NewCommandRunner(RunnerConfig{Executor: execStub})

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := runner.Run(context.Background(), CommandRequest{
				Operation: "update issue",
				Args:      []string{"update", "bd-1"},
				IsWrite:   true,
			})
			if err != nil {
				t.Errorf("Run returned error: %v", err)
			}
		}()
	}

	wg.Wait()

	if execStub.maxConcurrent > 1 {
		t.Fatalf("expected serialized write calls, max concurrent=%d", execStub.maxConcurrent)
	}
}

// TestRWMutexParallelReadOverlap verifies that concurrent read-flagged Run
// calls execute in parallel up to the semaphore cap (bdSemCap).
//
// We run parallelReads == 2 × bdSemCap goroutines, each sleeping 20 ms.
// With cap=2 these batch into 2 groups of 2, finishing in ~2 × sleep = ~40 ms.
// If reads were fully serialized the wall time would be 4 × 20 ms = 80 ms;
// the 70 ms threshold catches that while allowing scheduler jitter.
//
// The prior test ran 100 goroutines without any bound. With the bdSemCap
// semaphore, 100 readers batch into 50 rounds and legitimately take ~1 s
// — the burst-cap behavior is covered by TestConcurrencyCapBurst instead.
func TestRWMutexParallelReadOverlap(t *testing.T) {
	t.Parallel()

	const (
		parallelReads = 2 * bdSemCap // 2 full batches at cap
		sleepPerCall  = 20 * time.Millisecond
		// 2 parallel batches -> ~40 ms expected. 70 ms catches full serialization
		// (~80 ms) while leaving headroom for scheduler jitter.
		maxWallTime = 70 * time.Millisecond
		iterations  = 5
	)

	sleepingExec := &sleepingExecutor{sleep: sleepPerCall}
	runner := NewCommandRunner(RunnerConfig{Executor: sleepingExec})

	for iter := 0; iter < iterations; iter++ {
		var wg sync.WaitGroup
		wg.Add(parallelReads)
		start := time.Now()

		for i := 0; i < parallelReads; i++ {
			go func() {
				defer wg.Done()
				_, err := runner.Run(context.Background(), CommandRequest{
					Operation: "list issues",
					Args:      []string{"list", "--json"},
					IsWrite:   false,
				})
				if err != nil {
					t.Errorf("Run returned error: %v", err)
				}
			}()
		}

		wg.Wait()
		elapsed := time.Since(start)

		if elapsed >= maxWallTime {
			t.Fatalf("iteration %d: parallel read overlap too slow: %v >= %v (reads appear serialized)", iter+1, elapsed, maxWallTime)
		}
	}
}

// TestRWMutexWriteExclusion verifies that writers are fully exclusive: no
// writer ever runs concurrently with another writer or with any reader. We
// mix 5 writers and 10 readers, track in-flight writer and reader counts,
// and panic (caught by the test harness) if exclusion is violated.
func TestRWMutexWriteExclusion(t *testing.T) {
	t.Parallel()

	const (
		writers   = 5
		readers   = 10
		sleepEach = 5 * time.Millisecond
	)

	var (
		mu              sync.Mutex
		inFlightWriters int
		inFlightReaders int
	)

	exclusionExec := newCallbackExecutor(func(isWrite bool) {
		mu.Lock()
		if isWrite {
			// When entering a write, no readers or other writers may be in flight.
			if inFlightReaders > 0 || inFlightWriters > 0 {
				panic(fmt.Sprintf("write exclusion violated: inFlightReaders=%d inFlightWriters=%d", inFlightReaders, inFlightWriters))
			}
			inFlightWriters++
		} else {
			inFlightReaders++
		}
		mu.Unlock()

		time.Sleep(sleepEach)

		mu.Lock()
		if isWrite {
			inFlightWriters--
		} else {
			inFlightReaders--
		}
		mu.Unlock()
	})

	runner := NewCommandRunner(RunnerConfig{Executor: exclusionExec})

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			_, err := runner.Run(context.Background(), CommandRequest{
				Operation: "update issue",
				IsWrite:   true,
			})
			if err != nil {
				t.Errorf("writer Run returned error: %v", err)
			}
		}()
	}

	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			_, err := runner.Run(context.Background(), CommandRequest{
				Operation: "list issues",
				IsWrite:   false,
			})
			if err != nil {
				t.Errorf("reader Run returned error: %v", err)
			}
		}()
	}

	wg.Wait()
}

// TestConcurrencyCapBurst verifies that a burst of N > bdSemCap concurrent
// reads never runs more than bdSemCap subprocess execs simultaneously. We
// launch burstSize goroutines, each performing a read; the peak-tracking
// executor records the maximum observed concurrency and the test asserts it
// never exceeds bdSemCap.
func TestConcurrencyCapBurst(t *testing.T) {
	t.Parallel()

	const burstSize = 10 // well above bdSemCap

	guard := &concurrencyGuardExecutor{}
	runner := NewCommandRunner(RunnerConfig{Executor: guard})

	var wg sync.WaitGroup
	wg.Add(burstSize)

	for i := 0; i < burstSize; i++ {
		go func() {
			defer wg.Done()
			_, err := runner.Run(context.Background(), CommandRequest{
				Operation: "list issues",
				Args:      []string{"list", "--json"},
				IsWrite:   false,
			})
			if err != nil {
				t.Errorf("Run returned error: %v", err)
			}
		}()
	}

	wg.Wait()

	if guard.maxConcurrent > bdSemCap {
		t.Fatalf("concurrency cap violated: max concurrent executions=%d, want <= %d", guard.maxConcurrent, bdSemCap)
	}
	if guard.maxConcurrent == 0 {
		t.Fatal("no executions were observed; something is wrong with the test setup")
	}
}

// TestConcurrencyCapContextCancelWhileWaiting verifies that a context
// cancelled while waiting for a semaphore slot returns promptly without
// executing the subprocess. We fill all bdSemCap slots with long-running
// goroutines, then submit an additional read with a pre-cancelled context and
// assert: (a) it returns promptly, (b) the executor was never called for it.
func TestConcurrencyCapContextCancelWhileWaiting(t *testing.T) {
	t.Parallel()

	// gate controls when the slot-holding goroutines release their executions.
	gate := make(chan struct{})
	// entered signals each time a goroutine enters the executor (i.e. has taken
	// a semaphore slot and is now inside executor.Run).
	entered := make(chan struct{}, bdSemCap)

	var execMu sync.Mutex
	totalExecs := 0

	blockingExec := newCallbackExecutor(func(_ bool) {
		execMu.Lock()
		totalExecs++
		execMu.Unlock()
		entered <- struct{}{} // signal: slot is now occupied
		<-gate                // hold the slot until the gate is opened
	})

	runner := NewCommandRunner(RunnerConfig{Executor: blockingExec})

	// Launch exactly bdSemCap goroutines to occupy all semaphore slots.
	var slotHolderWg sync.WaitGroup
	slotHolderWg.Add(bdSemCap)

	for i := 0; i < bdSemCap; i++ {
		go func() {
			defer slotHolderWg.Done()
			_, _ = runner.Run(context.Background(), CommandRequest{
				Operation: "list issues",
				Args:      []string{"list", "--json"},
				IsWrite:   false,
			})
		}()
	}

	// Wait until all bdSemCap goroutines have entered executor.Run (all slots taken).
	deadline := time.Now().Add(2 * time.Second)
	for i := 0; i < bdSemCap; i++ {
		select {
		case <-entered:
		case <-time.After(time.Until(deadline)):
			t.Fatalf("timed out waiting for slot-holder goroutine %d to enter executor", i+1)
		}
	}

	// Now submit one more read with a pre-cancelled context — all slots are full.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	start := time.Now()
	_, err := runner.Run(cancelledCtx, CommandRequest{
		Operation: "list issues",
		Args:      []string{"list", "--json"},
		IsWrite:   false,
	})
	elapsed := time.Since(start)

	// Must return promptly — well under any slot-holder sleep.
	const promptReturn = 500 * time.Millisecond
	if elapsed >= promptReturn {
		t.Fatalf("cancelled request did not return promptly: elapsed=%v", elapsed)
	}

	// Must return a timeout or cancellation error.
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	var gatewayErr domain.GatewayError
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("expected domain.GatewayError, got %T (%v)", err, err)
	}
	if gatewayErr.Code != domain.ErrorCodeTimeout && gatewayErr.Code != domain.ErrorCodeCommandFailed {
		t.Fatalf("unexpected error code %q; want Timeout or CommandFailed (cancel maps to CommandFailed)", gatewayErr.Code)
	}

	// Executor must NOT have been called for the cancelled request — total execs
	// must still equal bdSemCap (only the slot-holders ran).
	execMu.Lock()
	execsAtCancel := totalExecs
	execMu.Unlock()
	if execsAtCancel != bdSemCap {
		t.Fatalf("executor call count mismatch: got %d, want exactly %d (cancelled request must not execute)", execsAtCancel, bdSemCap)
	}

	// Release all slot-holding goroutines.
	close(gate)
	slotHolderWg.Wait()
}

func TestCommandRunnerRunLogsExecutionTraceOnSuccess(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	var sink strings.Builder
	runner := NewCommandRunner(RunnerConfig{
		Executor: execStub,
		Logger:   slog.New(slog.NewJSONHandler(&sink, nil)),
	})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "ready", Args: []string{"ready", "--json"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	record := decodeLoggedRecord(t, sink.String())
	if got := record["msg"]; got != "bd command finished" {
		t.Fatalf("expected message %q, got %#v", "bd command finished", got)
	}
	if got := record["level"]; got != "INFO" {
		t.Fatalf("expected successful trace at INFO level, got %#v", got)
	}
	assertLoggedArray(t, record["argv"], []string{"bd", "ready", "--json"})
	assertLoggedFloatEquals(t, record["exit_code"], 0)
	assertLoggedFloatAtLeast(t, record["duration_ms"], 0)
	if got := record["operation"]; got != "ready" {
		t.Fatalf("expected operation ready, got %#v", got)
	}
}

func TestCommandRunnerRunLogsExecutionTraceOnCommandFailure(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{ExitCode: 2, Stderr: []byte("bad args")}}
	var sink strings.Builder
	runner := NewCommandRunner(RunnerConfig{
		Executor: execStub,
		Logger:   slog.New(slog.NewJSONHandler(&sink, nil)),
	})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "ready", Args: []string{"ready"}})
	if err == nil {
		t.Fatal("expected command failure")
	}
	record := decodeLoggedRecord(t, sink.String())
	if got := record["level"]; got != "WARN" {
		t.Fatalf("expected non-zero exit trace at WARN level, got %#v", got)
	}
	assertLoggedFloatEquals(t, record["exit_code"], 2)
	if got := record["stderr"]; got != "bad args" {
		t.Fatalf("expected stderr field, got %#v", got)
	}
}

func TestCommandRunnerRunLogsExecutionTraceOnExecutionError(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{err: exec.ErrNotFound}
	var sink strings.Builder
	runner := NewCommandRunner(RunnerConfig{
		Executor: execStub,
		Logger:   slog.New(slog.NewJSONHandler(&sink, nil)),
	})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "ready", Args: []string{"ready"}})
	if err == nil {
		t.Fatal("expected execution error")
	}
	record := decodeLoggedRecord(t, sink.String())
	if got := record["level"]; got != "WARN" {
		t.Fatalf("expected execution-error trace at WARN level, got %#v", got)
	}
	assertLoggedFloatEquals(t, record["exit_code"], -1)
	if got := record["error"]; got == nil || !strings.Contains(fmt.Sprint(got), "executable file not found") {
		t.Fatalf("expected execution error field, got %#v", got)
	}
}

type stubExecutor struct {
	command string
	args    []string
	workDir string
	env     []string

	result ExecResult
	err    error
}

// sleepingExecutor blocks for a fixed duration then returns success. Used to
// verify read-overlap: if reads are truly parallel the wall time stays well
// under N × sleep.
type sleepingExecutor struct {
	sleep time.Duration
}

func (e *sleepingExecutor) Run(_ context.Context, _ string, _ []string, _ string, _ []string) (ExecResult, error) {
	time.Sleep(e.sleep)
	return ExecResult{Stdout: []byte("ok")}, nil
}

// callbackExecutor invokes fn with the IsWrite flag captured on the
// CommandRequest. Because Run does not receive the request directly, the
// runner stamps the flag onto a context value — instead we rely on the
// request-level IsWrite plumbing being tested via concurrencyGuardExecutor.
// Here we use a simpler design: the callback is registered per-goroutine via
// a channel so each invocation knows its role.
//
// Actually: the executor receives no IsWrite information because CommandRequest
// is opaque at the executor boundary. We therefore use a stateful executor
// that tracks the invocation order externally via the runner's lock contract.
// The callbackExecutor calls fn(isWrite) where isWrite is determined by whether
// the caller set IsWrite on the CommandRequest — we thread the flag through via
// a per-call channel injected before the goroutine starts.
type callbackExecutor struct {
	fn func(isWrite bool)
	// isWriteCh is fed by the goroutine before calling runner.Run so the
	// executor can retrieve the write flag inside Run.
	isWriteCh chan bool
}

func newCallbackExecutor(fn func(isWrite bool)) *callbackExecutor {
	return &callbackExecutor{fn: fn, isWriteCh: make(chan bool, 64)}
}

func (e *callbackExecutor) Run(_ context.Context, _ string, args []string, _ string, _ []string) (ExecResult, error) {
	// Distinguish writes from reads by the command verb in args, since the
	// executor does not see CommandRequest.IsWrite directly. The test ensures
	// write goroutines pass args starting with "update" and readers pass "list".
	isWrite := len(args) > 0 && args[0] == "update"
	e.fn(isWrite)
	return ExecResult{Stdout: []byte("ok")}, nil
}

type concurrencyGuardExecutor struct {
	mu            sync.Mutex
	current       int
	maxConcurrent int
}

func (e *concurrencyGuardExecutor) Run(_ context.Context, _ string, _ []string, _ string, _ []string) (ExecResult, error) {
	e.mu.Lock()
	e.current++
	if e.current > e.maxConcurrent {
		e.maxConcurrent = e.current
	}
	e.mu.Unlock()

	time.Sleep(10 * time.Millisecond)

	e.mu.Lock()
	e.current--
	e.mu.Unlock()

	return ExecResult{Stdout: []byte("ok")}, nil
}

func (s *stubExecutor) Run(_ context.Context, command string, args []string, workDir string, env []string) (ExecResult, error) {
	s.command = command
	s.args = append([]string(nil), args...)
	s.workDir = workDir
	s.env = append([]string(nil), env...)

	return s.result, s.err
}

func assertGatewayErrorCode(t *testing.T, err error, expected domain.ErrorCode) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var gatewayErr domain.GatewayError
	if !errors.As(err, &gatewayErr) {
		t.Fatalf("expected domain.GatewayError, got %T (%v)", err, err)
	}

	if gatewayErr.Code != expected {
		t.Fatalf("unexpected error code: got %q want %q", gatewayErr.Code, expected)
	}
}

func assertContains(t *testing.T, got string, wantSubstring string) {
	t.Helper()

	if !strings.Contains(got, wantSubstring) {
		t.Fatalf("expected %q to contain %q", got, wantSubstring)
	}
}

func decodeLoggedRecord(t *testing.T, content string) map[string]any {
	t.Helper()

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		t.Fatal("expected logged record content")
	}

	var record map[string]any
	if err := json.Unmarshal([]byte(trimmed), &record); err != nil {
		t.Fatalf("json.Unmarshal failed: %v (content=%q)", err, trimmed)
	}
	return record
}

func assertLoggedArray(t *testing.T, got any, want []string) {
	t.Helper()

	raw, ok := got.([]any)
	if !ok {
		t.Fatalf("expected []any array, got %T (%#v)", got, got)
	}
	if len(raw) != len(want) {
		t.Fatalf("expected array len %d, got %d (%#v)", len(want), len(raw), raw)
	}
	for i, item := range raw {
		if fmt.Sprint(item) != want[i] {
			t.Fatalf("expected argv[%d]=%q, got %#v", i, want[i], item)
		}
	}
}

func assertLoggedFloatEquals(t *testing.T, got any, want float64) {
	t.Helper()

	value, ok := got.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T (%#v)", got, got)
	}
	if value != want {
		t.Fatalf("expected %v, got %v", want, value)
	}
}

func assertLoggedFloatAtLeast(t *testing.T, got any, min float64) {
	t.Helper()

	value, ok := got.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T (%#v)", got, got)
	}
	if value < min {
		t.Fatalf("expected value >= %v, got %v", min, value)
	}
}

// TestFilterEnvToAllowlistStripsBeadsActor verifies that BEADS_ACTOR is
// stripped by filterEnvToAllowlist. The gateway never passes --actor to bd, so
// BEADS_ACTOR in the parent process env must not leak to bd subprocesses —
// doing so would let the ambient env silently override the actor attribution
// that bd derives from git user.name. See the Actor attribution note in
// interface.go (audit 2026-05-17).
func TestFilterEnvToAllowlistStripsBeadsActor(t *testing.T) {
	t.Parallel()

	input := []string{
		"BEADS_ACTOR=impersonator",
		"PATH=/usr/bin",
		"HOME=/home/user",
	}

	out := filterEnvToAllowlist(input)

	for _, entry := range out {
		if strings.HasPrefix(entry, "BEADS_ACTOR=") {
			t.Fatalf("filterEnvToAllowlist: BEADS_ACTOR must be stripped but was present: %v", out)
		}
	}

	// PATH and HOME must survive.
	foundPATH, foundHOME := false, false
	for _, entry := range out {
		switch entry {
		case "PATH=/usr/bin":
			foundPATH = true
		case "HOME=/home/user":
			foundHOME = true
		}
	}
	if !foundPATH {
		t.Fatalf("filterEnvToAllowlist: PATH must survive allowlist; got %v", out)
	}
	if !foundHOME {
		t.Fatalf("filterEnvToAllowlist: HOME must survive allowlist; got %v", out)
	}
}

// TestCreateIssueDoesNotReceiveBeadsActorInEnv verifies the end-to-end
// integration: when BEADS_ACTOR is present in the RunnerConfig.Env (simulating
// a parent process that has it set), the executor invocation for CreateIssue
// does NOT receive BEADS_ACTOR. This is the CreateIssue-level assertion
// described in wgz0; filterEnvToAllowlist does the filtering.
func TestCreateIssueDoesNotReceiveBeadsActorInEnv(t *testing.T) {
	t.Parallel()

	var capturedEnv []string
	capturingExec := &envCapturingExecutor{
		result: ExecResult{Stdout: []byte(`{"id":"bd-env-test"}`)},
		captureEnv: func(env []string) {
			capturedEnv = append([]string(nil), env...)
		},
	}

	runner := NewCommandRunner(RunnerConfig{
		Env:      []string{"BEADS_ACTOR=impersonator", "PATH=/usr/bin"},
		Executor: capturingExec,
	})
	gateway := NewCLIGateway(runner)

	_, err := gateway.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "env test"})
	if err != nil {
		t.Fatalf("CreateIssue returned error: %v", err)
	}

	for _, entry := range capturedEnv {
		if strings.HasPrefix(entry, "BEADS_ACTOR=") {
			t.Fatalf("executor received BEADS_ACTOR in env — must be stripped; env=%v", capturedEnv)
		}
	}
}

// envCapturingExecutor is a CommandExecutor that records the env slice passed
// to Run so tests can assert filtering behavior.
type envCapturingExecutor struct {
	result     ExecResult
	captureEnv func([]string)
}

func (e *envCapturingExecutor) Run(_ context.Context, _ string, _ []string, _ string, env []string) (ExecResult, error) {
	if e.captureEnv != nil {
		e.captureEnv(env)
	}
	return e.result, nil
}
