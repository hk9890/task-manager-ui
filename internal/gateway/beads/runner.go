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

// envAllowlist is the fixed set of env var names passed to bd subprocesses.
// All BD_* and other ambient vars are stripped; only these vars (and any
// BWB_-prefixed var) survive. The gateway is bound to one project; env
// isolation prevents stray BD_DB_PATH or XDG_CONFIG_HOME values from
// redirecting bd to a different database.
var envAllowlist = []string{
	"PATH",
	"HOME",
	"USER",
	"LANG",
	"LC_ALL",
	"LC_CTYPE",
	"LC_MESSAGES",
	"TERM",
}

// filterEnvToAllowlist returns a filtered copy of environ containing only
// entries whose key is in envAllowlist or whose key has the prefix "BWB_".
func filterEnvToAllowlist(environ []string) []string {
	allowed := make(map[string]struct{}, len(envAllowlist))
	for _, k := range envAllowlist {
		allowed[k] = struct{}{}
	}

	out := make([]string, 0, len(envAllowlist))
	for _, entry := range environ {
		k, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, listed := allowed[k]; listed {
			out = append(out, entry)
			continue
		}
		if strings.HasPrefix(k, "BWB_") {
			out = append(out, entry)
		}
	}
	return out
}

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
	// ReadOnly, when true, prepends "--readonly" to every bd argv. This causes
	// bd to reject all write operations (create, update, close, comment add,
	// dep add, etc.) with a non-zero exit code, protecting external or shared
	// databases from accidental mutation during tests.
	ReadOnly bool
}

// bdSemCap is the maximum number of bd subprocesses allowed to execute
// concurrently. Empirical data: solo bd ≈ 0.28 s, 4-way contended calls
// measured 1.25 s median / 6.1 s p100 (2578 log samples). A board auto-refresh
// fans out 4 subprocess calls at once; capping at 2 forces contention down to
// pairs, producing the largest relative latency improvement with minimal
// throughput cost. Cap 3 would still queue one of the four burst callers but
// allow slightly higher throughput on heterogeneous bursts; 2 is the more
// conservative choice given the super-linear contention profile observed.
const bdSemCap = 2

// CommandRunner is a reusable execution layer for bd-backed gateway methods.
type CommandRunner struct {
	command        string
	defaultWorkDir string
	defaultEnv     []string
	executor       CommandExecutor
	logger         *slog.Logger
	readOnly       bool
	runMu          sync.RWMutex
	// sem is a buffered-channel semaphore that caps concurrent bd subprocess
	// executions to bdSemCap. A token is acquired immediately before calling
	// executor.Run and released on return (deferred). Context cancellation while
	// waiting for a token returns promptly without executing the subprocess.
	sem   chan struct{}
	cache *readCache
}

// NewCommandRunner creates a command runner for bd CLI interactions.
func NewCommandRunner(cfg RunnerConfig) *CommandRunner {
	command := cfg.Command
	if strings.TrimSpace(command) == "" {
		command = defaultBDCommand
	}

	rawEnv := cfg.Env
	if rawEnv == nil {
		rawEnv = os.Environ()
	}
	defaultEnv := filterEnvToAllowlist(rawEnv)

	executor := cfg.Executor
	if executor == nil {
		executor = osCommandExecutor{}
	}

	sem := make(chan struct{}, bdSemCap)
	for i := 0; i < bdSemCap; i++ {
		sem <- struct{}{}
	}

	cache := newReadCache(cfg.WorkDir)
	if err := cache.bootstrap(); err != nil && cfg.Logger != nil {
		cfg.Logger.Warn("failed to bootstrap cache token file; cache may be disabled",
			"path", cache.tokenPath(),
			"error", err.Error(),
		)
	}

	return &CommandRunner{
		command:        command,
		defaultWorkDir: cfg.WorkDir,
		defaultEnv:     append([]string(nil), defaultEnv...),
		executor:       executor,
		logger:         cfg.Logger,
		readOnly:       cfg.ReadOnly,
		sem:            sem,
		cache:          cache,
	}
}

// Run executes one command and returns stdout on success.
//
// For read requests (IsWrite == false), the result is served from the in-process
// read cache when the .beads/last-touched mtime is unchanged. Cache hits skip
// runMu and the semaphore entirely.
//
// For write requests (IsWrite == true), the cache is invalidated before exec so
// that the next read re-execs bd. External writes are caught by the
// .beads/last-touched mtime advancing on the next token check.
func (r *CommandRunner) Run(ctx context.Context, req CommandRequest) ([]byte, error) {
	if r == nil {
		return nil, newGatewayError(domain.ErrorCodeUnknown, req.Operation, "command runner is not configured", nil)
	}

	// Resolve argv early so it is consistent between the cache key and the
	// executor invocation.
	argv := r.resolveArgs(req.Args)

	// ---- read cache fast path ------------------------------------------------
	// Check the cache before acquiring any lock. Hits return immediately without
	// touching runMu or the semaphore. A successful hit is logged so operators
	// can compute hit/miss ratios from the same log stream as "bd command
	// finished" misses (see docs/MONITORING.md).
	if !req.IsWrite {
		if cached, hit := r.cache.get(argv); hit {
			r.logCacheHit(req, argv)
			return cached, nil
		}
	}
	// ---- end fast path -------------------------------------------------------

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
		// Invalidate the read cache while holding the write lock. This ensures
		// that the next read, which must hold at least an RLock, cannot see stale
		// data from before this write. Belt-and-suspenders: external writes are
		// also caught by the .beads/last-touched mtime advancing.
		r.cache.invalidate()
	} else {
		r.runMu.RLock()
		defer r.runMu.RUnlock()
	}

	if !req.IsWrite {
		// Sample the token BEFORE exec. If an external write lands during the
		// exec, the token we stored will lag behind the current mtime on the
		// next read, causing a cache miss — the conservative correct direction.
		token, tokenOK := r.cache.currentToken()

		stdout, execErr := r.execOnce(ctx, req, argv)
		if execErr != nil {
			return nil, execErr
		}
		// Only cache successful reads when the cache is enabled (workDir set).
		if tokenOK {
			r.cache.set(argv, token, stdout)
		}
		return stdout, nil
	}

	// Write path: exec directly (cache was already invalidated above).
	return r.execOnce(ctx, req, argv)
}

// execOnce acquires the semaphore, runs the subprocess, logs the result, and
// returns stdout. It is called from Run after locking. argv must already be
// resolved via resolveArgs.
func (r *CommandRunner) execOnce(ctx context.Context, req CommandRequest, argv []string) ([]byte, error) {
	// Acquire a semaphore slot before executing. This limits the number of
	// concurrent bd subprocesses to bdSemCap regardless of how many callers hold
	// the RWMutex read lock simultaneously. Context cancellation or timeout while
	// waiting returns promptly without running the subprocess.
	select {
	case <-r.sem:
		// slot acquired
	case <-ctx.Done():
		return nil, normalizeExecutionError(ctx, req.Operation, nil, ctx.Err())
	}
	defer func() { r.sem <- struct{}{} }()

	startedAt := time.Now()
	result, err := r.executor.Run(ctx, r.command, argv, r.resolveWorkDir(req.WorkDir), r.resolveEnv(req.Env))
	r.logExecution(req, argv, result, err, time.Since(startedAt))
	if err != nil {
		return nil, normalizeExecutionError(ctx, req.Operation, result.Stderr, err)
	}

	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(string(result.Stderr))
		// TODO(beads-workbench-db0z.6): bd 1.0.4 does not expose a dedicated exit
		// code or stable JSON signal for the "no database" condition — all CLI
		// failures return exit code 1. The substring match below is therefore a
		// best-effort detection pinned to the current wording. If bd adds a
		// dedicated exit code or structured error field in a future release, prefer
		// that over this substring check. The pinning test
		// TestMissingBDDatabaseDetectionSubstringPin in runner_test.go will fail
		// loudly if the wording changes, giving a clear signal to revisit.
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
//
// NDJSON (newline-delimited JSON, i.e. multiple top-level JSON objects) is
// intentionally unsupported. The second Decode call below detects any trailing
// JSON content — including an NDJSON second record — and returns
// ErrorCodeDecodeFailed. All bd commands that this gateway calls are expected
// to emit exactly one JSON object on stdout.
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
	env = append(env, filterEnvToAllowlist(extra)...)
	// Force BD_NON_INTERACTIVE=1 last so it always wins over caller-supplied
	// values that survived the allowlist. bwb is a programmatic caller and must
	// never let a child bd process prompt for tty input (every gateway call
	// would hang). See the embedded-fixture integration tests in internal/app.
	env = append(env, "BD_NON_INTERACTIVE=1")
	return env
}

// resolveWorkDir always returns the gateway's bound defaultWorkDir.
// CommandRequest.WorkDir is intentionally ignored: a gateway instance is bound
// to exactly one beads project and must not be redirected by per-request
// values (see CODING.md rule #3 — gateway is source-specific).
func (r *CommandRunner) resolveWorkDir(_ string) string {
	return r.defaultWorkDir
}

// resolveArgs returns the effective argv for a command invocation. When the
// runner is configured with ReadOnly == true, "--readonly" is prepended to
// every argv so bd rejects write operations at the CLI layer. This protects
// shared or external databases from accidental mutation during parity tests.
func (r *CommandRunner) resolveArgs(args []string) []string {
	if !r.readOnly {
		return args
	}
	resolved := make([]string, 0, len(args)+1)
	resolved = append(resolved, "--readonly")
	resolved = append(resolved, args...)
	return resolved
}

// logCacheHit records a per-call trace for a cache hit. Mirrors the operation
// + argv fields of "bd command finished" so hit-rate analysis can be done with
// a single grep against the same log file: "cache hit" lines are hits,
// "bd command finished" lines with IsWrite=false are misses.
func (r *CommandRunner) logCacheHit(req CommandRequest, resolvedArgs []string) {
	if r.logger == nil {
		return
	}
	argv := append([]string{r.command}, resolvedArgs...)
	r.logger.Info("bd command cache hit",
		"operation", req.Operation,
		"argv", argv,
	)
}

func (r *CommandRunner) logExecution(req CommandRequest, resolvedArgs []string, result ExecResult, err error, duration time.Duration) {
	if r.logger == nil {
		return
	}

	exitCode := result.ExitCode
	if err != nil && exitCode == 0 {
		exitCode = -1
	}
	argv := append([]string{r.command}, resolvedArgs...)
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
	// Non-zero exit codes (including the -1 sentinel set above for execution
	// errors) are real failures. Emit them at WARN so they surface above the
	// INFO success stream and through stderr mirroring, rather than hiding
	// among routine traces.
	if exitCode != 0 {
		r.logger.Warn("bd command finished", attrs...)
		return
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
