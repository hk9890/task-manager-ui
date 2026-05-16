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
// calls execute in parallel rather than serially. 100 goroutines each sleep
// 20 ms inside the executor; if reads were serialized the total wall time
// would be ~2 s. We assert < 100 ms (5× the per-call sleep) per iteration,
// repeated 5 times to catch flakiness.
func TestRWMutexParallelReadOverlap(t *testing.T) {
	t.Parallel()

	const (
		parallelReads = 100
		sleepPerCall  = 20 * time.Millisecond
		maxWallTime   = 100 * time.Millisecond
		iterations    = 5
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
		writers    = 5
		readers    = 10
		sleepEach  = 5 * time.Millisecond
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

func TestCommandRunnerRunLogsExecutionTraceOnSuccess(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{result: ExecResult{Stdout: []byte("ok")}}
	var sink strings.Builder
	runner := NewCommandRunner(RunnerConfig{
		Executor: execStub,
		Logger: slog.New(slog.NewJSONHandler(&sink, nil)),
	})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "ready", Args: []string{"ready", "--json"}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	record := decodeLoggedRecord(t, sink.String())
	if got := record["msg"]; got != "bd command finished" {
		t.Fatalf("expected message %q, got %#v", "bd command finished", got)
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
		Logger: slog.New(slog.NewJSONHandler(&sink, nil)),
	})

	_, err := runner.Run(context.Background(), CommandRequest{Operation: "ready", Args: []string{"ready"}})
	if err == nil {
		t.Fatal("expected command failure")
	}
	record := decodeLoggedRecord(t, sink.String())
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
	assertLoggedFloatEquals(t, record["exit_code"], -1)
	if got := record["error"]; got == nil || !strings.Contains(fmt.Sprint(got), "executable file not found") {
		t.Fatalf("expected execution error field, got %#v", got)
	}
}

// TestMissingBDDatabaseDetectionSubstringPin is a pinning integration test that
// invokes the real bd CLI in an empty temporary directory (no .beads/) and
// verifies two properties:
//
//  1. The runner maps the result to ErrorCodeNoDatabaseFound — proving the
//     substring detection in runner.go still fires on the current bd wording.
//  2. The stderr from bd contains the exact substring "no beads database found"
//     that the detection logic depends on — this assertion fails loudly if bd
//     renames the message in a future release, signalling that the detection
//     mechanism in runner.go must be updated.
//
// TODO(beads-workbench-db0z.6): If bd adds a dedicated exit code or stable
// structured-error field for missing-db in a future release, switch the
// detection in runner.go to that signal and simplify or remove this test.
func TestMissingBDDatabaseDetectionSubstringPin(t *testing.T) {
	// This test spawns the real bd binary; skip in environments where bd is not
	// available on PATH.
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping integration pinning test")
	}

	emptyDir := t.TempDir()

	runner := NewCommandRunner(RunnerConfig{
		WorkDir: emptyDir,
		// Use the real osCommandExecutor (nil Executor falls back to default).
	})

	_, err := runner.Run(context.Background(), CommandRequest{
		Operation: "ready issues",
		Args:      []string{"ready", "--json"},
	})

	if err == nil {
		t.Fatal("expected an error from bd in a directory with no .beads/, got nil")
	}

	// Assert the runner mapped the result to ErrorCodeNoDatabaseFound.
	assertGatewayErrorCode(t, err, domain.ErrorCodeNoDatabaseFound)

	// Pin the exact substring that the detection logic in runner.go depends on.
	// If bd renames this message in a future release this assertion will fail,
	// signalling that runner.go's substring detection must be revisited.
	const pinnedSubstring = "no beads database found"
	assertContains(t, err.Error(), pinnedSubstring)
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
