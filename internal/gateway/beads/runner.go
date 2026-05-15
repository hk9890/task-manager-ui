package beads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
)

const defaultBDCommand = "bd"

// CommandRequest describes one CLI invocation.
type CommandRequest struct {
	// Operation is a stable logical name used in gateway errors.
	Operation string
	Args      []string
	WorkDir   string
	Env       []string
	// IsWrite marks the request as a mutating bd command (create, update, close,
	// comment add, dep add, etc.). Write requests acquire an exclusive lock so
	// they never overlap with other writers or in-flight readers. Read requests
	// (IsWrite == false, the default) acquire a shared lock and run concurrently.
	IsWrite bool
}

// ExecResult captures subprocess output and exit status.
type ExecResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// CommandExecutor runs a single subprocess invocation.
//
// Returning a non-zero ExitCode with nil error represents a process that ran
// but failed (for example: CLI validation error, not found, etc).
type CommandExecutor interface {
	Run(ctx context.Context, command string, args []string, workDir string, env []string) (ExecResult, error)
}

// RunnerConfig configures a bd CLI runner.
type RunnerConfig struct {
	Command  string
	WorkDir  string
	Env      []string
	Executor CommandExecutor
	Logger   *slog.Logger
}

// CommandRunner is a reusable execution layer for bd-backed gateway methods.
type CommandRunner struct {
	command        string
	defaultWorkDir string
	defaultEnv     []string
	executor       CommandExecutor
	logger         *slog.Logger
	runMu          sync.RWMutex
}

// NewCommandRunner creates a command runner for bd CLI interactions.
func NewCommandRunner(cfg RunnerConfig) *CommandRunner {
	command := cfg.Command
	if strings.TrimSpace(command) == "" {
		command = defaultBDCommand
	}

	defaultEnv := cfg.Env
	if defaultEnv == nil {
		defaultEnv = os.Environ()
	}

	executor := cfg.Executor
	if executor == nil {
		executor = osCommandExecutor{}
	}

	return &CommandRunner{
		command:        command,
		defaultWorkDir: cfg.WorkDir,
		defaultEnv:     append([]string(nil), defaultEnv...),
		executor:       executor,
		logger:         cfg.Logger,
	}
}

// Run executes one command and returns stdout on success.
func (r *CommandRunner) Run(ctx context.Context, req CommandRequest) ([]byte, error) {
	if r == nil {
		return nil, newGatewayError(domain.ErrorCodeUnknown, req.Operation, "command runner is not configured", nil)
	}

	// Locking contract:
	//   - Read requests (IsWrite == false): acquire a shared RLock so multiple
	//     concurrent read calls (e.g. dashboard section loads via tea.Batch) run
	//     in parallel. Empirical sampling against bd 1.0.4 + embedded Dolt (~25
	//     concurrent reads, 0 failures) and the parallel soak test added in task
	//     beads-workbench-5b1k (100 goroutines × 5 iterations, well under N×sleep
	//     budget) confirm read-side concurrency is safe.
	//   - Write requests (IsWrite == true): acquire an exclusive Lock so writers
	//     never overlap with readers or other writers. The write-exclusion soak
	//     test in runner_test.go (TestRWMutexWriteExclusion) enforces this.
	if req.IsWrite {
		r.runMu.Lock()
		defer r.runMu.Unlock()
	} else {
		r.runMu.RLock()
		defer r.runMu.RUnlock()
	}
	startedAt := time.Now()
	result, err := r.executor.Run(ctx, r.command, req.Args, r.resolveWorkDir(req.WorkDir), r.resolveEnv(req.Env))
	r.logExecution(req, result, err, time.Since(startedAt))
	if err != nil {
		return nil, normalizeExecutionError(ctx, req.Operation, result.Stderr, err)
	}

	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(string(result.Stderr))
		if strings.Contains(stderr, "no beads database found") {
			return nil, newGatewayError(domain.ErrorCodeNoDatabaseFound, req.Operation, stderr, nil)
		}
		message := fmt.Sprintf("command exited with code %d", result.ExitCode)
		if stderr != "" {
			message = fmt.Sprintf("%s: %s", message, stderr)
		}

		return nil, newGatewayError(domain.ErrorCodeCommandFailed, req.Operation, message, nil)
	}

	return result.Stdout, nil
}

// RunJSONInto executes a command and decodes JSON stdout into target.
func (r *CommandRunner) RunJSONInto(ctx context.Context, req CommandRequest, target any) error {
	stdout, err := r.Run(ctx, req)
	if err != nil {
		return err
	}

	return DecodeJSONInto(req.Operation, stdout, target)
}

// RunJSON executes a command and decodes JSON stdout into a typed result.
func RunJSON[T any](ctx context.Context, r *CommandRunner, req CommandRequest) (T, error) {
	var value T

	if err := r.RunJSONInto(ctx, req, &value); err != nil {
		return value, err
	}

	return value, nil
}

// DecodeJSONInto decodes JSON output into target and normalizes decode errors.
func DecodeJSONInto(operation string, stdout []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(stdout))

	if err := decoder.Decode(target); err != nil {
		return newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	if err := decoder.Decode(new(struct{})); err != io.EOF {
		if err == nil {
			return newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", errors.New("extra trailing JSON content"))
		}

		return newGatewayError(domain.ErrorCodeDecodeFailed, operation, "failed to decode command JSON output", err)
	}

	return nil
}

func (r *CommandRunner) resolveEnv(extra []string) []string {
	env := append([]string(nil), r.defaultEnv...)
	env = append(env, extra...)
	return env
}

func (r *CommandRunner) resolveWorkDir(override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}

	return r.defaultWorkDir
}

func (r *CommandRunner) logExecution(req CommandRequest, result ExecResult, err error, duration time.Duration) {
	if r.logger == nil {
		return
	}

	exitCode := result.ExitCode
	if err != nil && exitCode == 0 {
		exitCode = -1
	}
	argv := append([]string{r.command}, req.Args...)
	attrs := []any{
		"operation", req.Operation,
		"argv", argv,
		"exit_code", exitCode,
		"duration_ms", duration.Milliseconds(),
	}
	if trimmedStderr := strings.TrimSpace(string(result.Stderr)); trimmedStderr != "" {
		attrs = append(attrs, "stderr", trimmedStderr)
	}
	if err != nil {
		attrs = append(attrs, "error", err.Error())
	}
	r.logger.Info("bd command finished", attrs...)
}

func normalizeExecutionError(ctx context.Context, operation string, stderr []byte, err error) error {
	trimmedStderr := strings.TrimSpace(string(stderr))

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		message := "command timed out"
		if trimmedStderr != "" {
			message = fmt.Sprintf("%s: %s", message, trimmedStderr)
		}

		return newGatewayError(domain.ErrorCodeTimeout, operation, message, err)
	}

	if errors.Is(err, exec.ErrNotFound) {
		message := "bd command is unavailable"
		if trimmedStderr != "" {
			message = fmt.Sprintf("%s: %s", message, trimmedStderr)
		}

		return newGatewayError(domain.ErrorCodeCommandUnavailable, operation, message, err)
	}

	message := "failed to execute command"
	if trimmedStderr != "" {
		message = fmt.Sprintf("%s: %s", message, trimmedStderr)
	}

	return newGatewayError(domain.ErrorCodeCommandFailed, operation, message, err)
}

func newGatewayError(code domain.ErrorCode, operation, message string, cause error) error {
	return domain.GatewayError{
		Code:      code,
		Operation: operation,
		Message:   message,
		Cause:     cause,
	}
}

type osCommandExecutor struct{}

func (osCommandExecutor) Run(ctx context.Context, command string, args []string, workDir string, env []string) (ExecResult, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workDir
	cmd.Env = env

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := ExecResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}

	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}

	return result, err
}
