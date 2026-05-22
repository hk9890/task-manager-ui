package beads

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTmpBeadsDir creates a temporary directory with a .beads/last-touched file
// and returns the workDir path. The caller owns cleanup via t.Cleanup.
func newTmpBeadsDir(t *testing.T) string {
	t.Helper()

	workDir := t.TempDir()
	beadsDir := filepath.Join(workDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .beads: %v", err)
	}
	touchFile := filepath.Join(beadsDir, "last-touched")
	if err := os.WriteFile(touchFile, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile last-touched: %v", err)
	}
	return workDir
}

// advanceToken writes to .beads/last-touched to advance its mtime. It sleeps
// briefly to ensure the mtime change is detectable even on filesystems with
// 1-second mtime resolution.
func advanceToken(t *testing.T, workDir string) {
	t.Helper()

	touchFile := filepath.Join(workDir, ".beads", "last-touched")
	// Write new content to ensure the mtime changes.
	if err := os.WriteFile(touchFile, []byte(time.Now().String()), 0o644); err != nil {
		t.Fatalf("WriteFile last-touched: %v", err)
	}

	// Force a mtime that is definitely different from the current one by nudging
	// it 1 second into the future. This guarantees a detectable change even on
	// filesystems whose mtime resolution is 1 second (e.g. some Linux tmpfs
	// mounts under test).
	fi, err := os.Stat(touchFile)
	if err != nil {
		t.Fatalf("Stat last-touched: %v", err)
	}
	future := fi.ModTime().Add(time.Second)
	if err := os.Chtimes(touchFile, future, future); err != nil {
		t.Fatalf("Chtimes last-touched: %v", err)
	}
}

// countingExecutor wraps a delegate executor and counts invocations.
type countingExecutor struct {
	mu    sync.Mutex
	count int
	base  CommandExecutor
}

func newCountingExecutor(base CommandExecutor) *countingExecutor {
	return &countingExecutor{base: base}
}

func (e *countingExecutor) Run(ctx context.Context, command string, args []string, workDir string, env []string) (ExecResult, error) {
	e.mu.Lock()
	e.count++
	e.mu.Unlock()
	return e.base.Run(ctx, command, args, workDir, env)
}

func (e *countingExecutor) Count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.count
}

// TestCacheHitSkipsExec verifies that a repeat read with an unchanged
// .beads/last-touched token returns from cache without spawning bd.
func TestCacheHitSkipsExec(t *testing.T) {
	t.Parallel()

	workDir := newTmpBeadsDir(t)

	stub := &stubExecutor{result: ExecResult{Stdout: []byte(`{"id":"bd-1"}`)}}
	counter := newCountingExecutor(stub)

	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  workDir,
		Executor: counter,
	})

	req := CommandRequest{
		Operation: "show issue",
		Args:      []string{"show", "bd-1", "--json"},
	}

	// First call: cache miss — must exec.
	out1, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}

	// Second call with same argv and unchanged token: must return from cache.
	out2, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}

	if string(out1) != string(out2) {
		t.Fatalf("cached output differs: got %q, want %q", out2, out1)
	}

	if got := counter.Count(); got != 1 {
		t.Fatalf("expected exactly 1 executor call (cache hit on second), got %d", got)
	}
}

// TestCacheInvalidatedByTokenAdvance verifies that when .beads/last-touched
// mtime advances between two reads, the cache is invalidated and bd is
// re-executed.
func TestCacheInvalidatedByTokenAdvance(t *testing.T) {
	t.Parallel()

	workDir := newTmpBeadsDir(t)

	stub := &stubExecutor{result: ExecResult{Stdout: []byte(`{"id":"bd-1"}`)}}
	counter := newCountingExecutor(stub)

	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  workDir,
		Executor: counter,
	})

	req := CommandRequest{
		Operation: "show issue",
		Args:      []string{"show", "bd-1", "--json"},
	}

	// First call: cache miss.
	if _, err := runner.Run(context.Background(), req); err != nil {
		t.Fatalf("first Run error: %v", err)
	}

	// Advance the token (simulate an external write).
	advanceToken(t, workDir)

	// Second call: token mismatch — must re-exec.
	if _, err := runner.Run(context.Background(), req); err != nil {
		t.Fatalf("second Run error: %v", err)
	}

	if got := counter.Count(); got != 2 {
		t.Fatalf("expected 2 executor calls (token advanced), got %d", got)
	}
}

// TestWriteInvalidatesCache verifies that an IsWrite=true request clears the
// cache so the next read re-execs bd.
func TestWriteInvalidatesCache(t *testing.T) {
	t.Parallel()

	workDir := newTmpBeadsDir(t)

	stub := &stubExecutor{result: ExecResult{Stdout: []byte(`{"id":"bd-1"}`)}}
	counter := newCountingExecutor(stub)

	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  workDir,
		Executor: counter,
	})

	readReq := CommandRequest{
		Operation: "show issue",
		Args:      []string{"show", "bd-1", "--json"},
	}
	writeReq := CommandRequest{
		Operation: "update issue",
		Args:      []string{"update", "bd-1"},
		IsWrite:   true,
	}

	// First read: cache miss, exec.
	if _, err := runner.Run(context.Background(), readReq); err != nil {
		t.Fatalf("first read error: %v", err)
	}
	if got := counter.Count(); got != 1 {
		t.Fatalf("after first read: expected 1 exec, got %d", got)
	}

	// Write: must invalidate the cache (exec count becomes 2).
	if _, err := runner.Run(context.Background(), writeReq); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if got := counter.Count(); got != 2 {
		t.Fatalf("after write: expected 2 execs, got %d", got)
	}

	// Second read of same argv: cache was invalidated — must re-exec.
	if _, err := runner.Run(context.Background(), readReq); err != nil {
		t.Fatalf("second read error: %v", err)
	}
	if got := counter.Count(); got != 3 {
		t.Fatalf("after second read (post-write): expected 3 execs, got %d", got)
	}
}

// TestSingleflightCollapsesIdenticalConcurrentArgv verifies that N concurrent
// goroutines issuing the same argv collapse to a single executor call via
// singleflight.
func TestSingleflightCollapsesIdenticalConcurrentArgv(t *testing.T) {
	t.Parallel()

	workDir := newTmpBeadsDir(t)

	// gate controls when the executor unblocks.
	gate := make(chan struct{})
	var execCount atomic.Int64

	blockingExec := &gatedExecutor{
		gate:   gate,
		result: ExecResult{Stdout: []byte(`["item"]`)},
		onExec: func() { execCount.Add(1) },
	}

	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  workDir,
		Executor: blockingExec,
	})

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([][]byte, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i], errs[i] = runner.Run(context.Background(), CommandRequest{
				Operation: "list issues",
				Args:      []string{"list", "--json"},
			})
		}()
	}

	// Give all goroutines time to pile up inside singleflight.
	time.Sleep(50 * time.Millisecond)
	// Unblock the single exec.
	close(gate)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d returned error: %v", i, err)
		}
	}

	if got := execCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 executor call (singleflight), got %d", got)
	}
}

// TestFailureNotCached verifies that a failed exec (non-zero exit code) is not
// stored in the cache; the next call re-execs.
func TestFailureNotCached(t *testing.T) {
	t.Parallel()

	workDir := newTmpBeadsDir(t)

	failResult := ExecResult{ExitCode: 1, Stderr: []byte("something went wrong")}
	stub := &stubExecutor{result: failResult}
	counter := newCountingExecutor(stub)

	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  workDir,
		Executor: counter,
	})

	req := CommandRequest{
		Operation: "show issue",
		Args:      []string{"show", "bd-missing", "--json"},
	}

	// First call: fails.
	if _, err := runner.Run(context.Background(), req); err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}

	// Reconfigure the executor to succeed on the second call.
	stub.result = ExecResult{Stdout: []byte(`{"id":"bd-missing"}`)}
	stub.err = nil

	// Second call: must re-exec (failure was not cached).
	if _, err := runner.Run(context.Background(), req); err != nil {
		t.Fatalf("second Run error: %v", err)
	}

	if got := counter.Count(); got != 2 {
		t.Fatalf("expected 2 executor calls (failure not cached), got %d", got)
	}
}

// TestWriteNotCached verifies that write requests are never stored in the cache.
func TestWriteNotCached(t *testing.T) {
	t.Parallel()

	workDir := newTmpBeadsDir(t)

	stub := &stubExecutor{result: ExecResult{Stdout: []byte(`{"id":"bd-1"}`)}}
	counter := newCountingExecutor(stub)

	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  workDir,
		Executor: counter,
	})

	writeReq := CommandRequest{
		Operation: "create issue",
		Args:      []string{"create", "--title", "test"},
		IsWrite:   true,
	}

	// Two consecutive writes with same argv must both exec.
	if _, err := runner.Run(context.Background(), writeReq); err != nil {
		t.Fatalf("first write error: %v", err)
	}
	if _, err := runner.Run(context.Background(), writeReq); err != nil {
		t.Fatalf("second write error: %v", err)
	}

	if got := counter.Count(); got != 2 {
		t.Fatalf("expected 2 executor calls (writes not cached), got %d", got)
	}
}

// TestCacheNoCacheWhenWorkDirEmpty verifies that the cache and singleflight are
// both inactive when WorkDir is empty, so all callers get independent executor
// invocations.
func TestCacheNoCacheWhenWorkDirEmpty(t *testing.T) {
	t.Parallel()

	stub := &stubExecutor{result: ExecResult{Stdout: []byte(`["item"]`)}}
	counter := newCountingExecutor(stub)

	runner := NewCommandRunner(RunnerConfig{
		// WorkDir intentionally empty — cache and singleflight disabled.
		Executor: counter,
	})

	req := CommandRequest{
		Operation: "list issues",
		Args:      []string{"list", "--json"},
	}

	const calls = 3
	for i := 0; i < calls; i++ {
		if _, err := runner.Run(context.Background(), req); err != nil {
			t.Fatalf("call %d error: %v", i+1, err)
		}
	}

	if got := counter.Count(); got != calls {
		t.Fatalf("expected %d executor calls (no caching without WorkDir), got %d", calls, got)
	}
}

// TestCacheDataRaceFree runs a heavy concurrent mix of reads and writes
// targeting the cache with a configured WorkDir. The -race detector will catch
// any unsynchronized access. This test is deliberately not timing-sensitive.
func TestCacheDataRaceFree(t *testing.T) {
	t.Parallel()

	workDir := newTmpBeadsDir(t)

	stub := &stubExecutor{result: ExecResult{Stdout: []byte(`[]`)}}
	runner := NewCommandRunner(RunnerConfig{
		WorkDir:  workDir,
		Executor: stub,
	})

	const (
		readers = 20
		writers = 5
	)

	var wg sync.WaitGroup
	wg.Add(readers + writers)

	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			_, _ = runner.Run(context.Background(), CommandRequest{
				Operation: "list",
				Args:      []string{"list", "--json"},
				IsWrite:   false,
			})
		}()
	}

	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			_, _ = runner.Run(context.Background(), CommandRequest{
				Operation: "update",
				Args:      []string{"update", "bd-1"},
				IsWrite:   true,
			})
		}()
	}

	wg.Wait()
}

// gatedExecutor blocks in Run until its gate channel is closed, then returns
// the configured result. An optional onExec callback fires before blocking.
type gatedExecutor struct {
	gate   chan struct{}
	result ExecResult
	err    error
	onExec func()
}

func (e *gatedExecutor) Run(_ context.Context, _ string, _ []string, _ string, _ []string) (ExecResult, error) {
	if e.onExec != nil {
		e.onExec()
	}
	<-e.gate
	return e.result, e.err
}
