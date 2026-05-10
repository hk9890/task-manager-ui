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

func TestCommandRunnerRunUsesDefaultAndRequestOverrides(t *testing.T) {
	t.Parallel()

	execStub := &stubExecutor{
		result: ExecResult{Stdout: []byte("ok")},
	}

	runner := NewCommandRunner(RunnerConfig{
		Command:  "bd-custom",
		WorkDir:  "/default/workdir",
		Env:      []string{"A=1", "B=2"},
		Executor: execStub,
	})

	out, err := runner.Run(context.Background(), CommandRequest{
		Operation: "list issues",
		Args:      []string{"ready", "--json"},
		WorkDir:   "/request/workdir",
		Env:       []string{"C=3"},
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

	if execStub.workDir != "/request/workdir" {
		t.Fatalf("unexpected work dir: %q", execStub.workDir)
	}

	if !reflect.DeepEqual(execStub.env, []string{"A=1", "B=2", "C=3"}) {
		t.Fatalf("unexpected env: %#v", execStub.env)
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

func TestCommandRunnerRunNilReceiver(t *testing.T) {
	t.Parallel()

	var runner *CommandRunner
	_, err := runner.Run(context.Background(), CommandRequest{Operation: "op"})
	assertGatewayErrorCode(t, err, domain.ErrorCodeUnknown)
}

func TestCommandRunnerRunSerializesConcurrentExecutorCalls(t *testing.T) {
	t.Parallel()

	execStub := &concurrencyGuardExecutor{}
	runner := NewCommandRunner(RunnerConfig{Executor: execStub})

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := runner.Run(context.Background(), CommandRequest{Operation: "list issues", Args: []string{"list", "--json"}})
			if err != nil {
				t.Errorf("Run returned error: %v", err)
			}
		}()
	}

	wg.Wait()

	if execStub.maxConcurrent > 1 {
		t.Fatalf("expected serialized executor calls, max concurrent=%d", execStub.maxConcurrent)
	}
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

type stubExecutor struct {
	command string
	args    []string
	workDir string
	env     []string

	result ExecResult
	err    error
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
