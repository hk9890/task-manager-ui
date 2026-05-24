package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/logging"
	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

// --- resolveAndValidateCWD tests ---

func TestResolveAndValidateCWDAcceptsReadableDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	got, err := resolveAndValidateCWD(dir, "")
	if err != nil {
		t.Fatalf("expected no error for readable dir, got: %v", err)
	}
	if got != dir {
		t.Fatalf("expected %q, got %q", dir, got)
	}
}

func TestResolveAndValidateCWDRejectsNonExistentPath(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := resolveAndValidateCWD(dir, "")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected 'does not exist' error, got: %v", err)
	}
}

func TestResolveAndValidateCWDRejectsFile(t *testing.T) {
	t.Parallel()

	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := resolveAndValidateCWD(f, "")
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected 'not a directory' error, got: %v", err)
	}
}

func TestResolveAndValidateCWDRejectsInaccessibleDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		// TODO(beads-workbench-2rfx): Windows ACLs do not honour Unix permission
		// bits set via os.Chmod(0o000), so locked dirs remain accessible.
		t.Skip("Unix permission bits have no effect on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	t.Parallel()

	parent := t.TempDir()
	locked := filepath.Join(parent, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	_, err := resolveAndValidateCWD(locked, "")
	if err == nil {
		t.Fatal("expected error for inaccessible directory, got nil")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected 'not accessible' in error message, got: %v", err)
	}
}

// TestRunCWDInaccessibleExitsWithCode1 verifies the full run() path returns exit
// code 1 with a clear message when --cwd points to an unreadable directory.
func TestRunCWDInaccessibleExitsWithCode1(t *testing.T) {
	if runtime.GOOS == "windows" {
		// TODO(beads-workbench-2rfx): Windows ACLs do not honour Unix permission
		// bits set via os.Chmod(0o000), so locked dirs remain accessible.
		t.Skip("Unix permission bits have no effect on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	t.Parallel()

	parent := t.TempDir()
	locked := filepath.Join(parent, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	var stderr bytes.Buffer
	code := run(
		[]string{"--cwd", locked},
		&bytes.Buffer{},
		&stderr,
		func(config.LoadOptions) (config.Result, error) { panic("should not reach load") },
		func(config.Model, startupOptions) error { panic("should not reach start") },
	)

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	msg := stderr.String()
	if !strings.Contains(msg, "failed to resolve --cwd") {
		t.Fatalf("expected 'failed to resolve --cwd' in stderr, got: %q", msg)
	}
}

// TestRunDebugWithHelpExitsZeroNoDebugLines asserts that --debug is silently
// dropped in the non-interactive --help path: exit code is 0 and no
// [bwb-debug] lines appear on stderr.  This encodes the decided contract:
// the debug flag is not honoured before the logger is initialised, so
// non-interactive flag paths (--help, --version) never emit debug output.
func TestRunDebugWithHelpExitsZeroNoDebugLines(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	code := run(
		[]string{"--debug", "--help"},
		&bytes.Buffer{},
		&stderr,
		func(config.LoadOptions) (config.Result, error) { panic("should not reach load") },
		func(config.Model, startupOptions) error { panic("should not reach start") },
	)

	if code != 0 {
		t.Fatalf("expected exit code 0 for --debug --help, got %d (stderr: %q)", code, stderr.String())
	}
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.Contains(line, "[bwb-debug]") {
			t.Fatalf("unexpected [bwb-debug] line on stderr for --debug --help: %q", line)
		}
	}
}

// TestResolveAndValidateCWDRejectsExecuteOnlyDir covers the EACCES path
// triggered by os.Open when the directory has execute permission but no read
// permission (mode 0o111).  This is distinct from the 0o000 case: os.Stat
// succeeds (directory exists and is a dir), but the Open probe returns EACCES.
// On Windows, Unix permission bits have no effect so this scenario cannot be
// reproduced with os.Chmod.
func TestResolveAndValidateCWDRejectsExecuteOnlyDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		// TODO: find a Windows-native way to test inaccessible-directory rejection.
		t.Skip("Unix execute-only permission bits have no effect on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	t.Parallel()

	parent := t.TempDir()
	execOnly := filepath.Join(parent, "exec-only")
	if err := os.Mkdir(execOnly, 0o111); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(execOnly, 0o755) })

	_, err := resolveAndValidateCWD(execOnly, "")
	if err == nil {
		t.Fatal("expected error for execute-only directory (EACCES on open), got nil")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("expected 'not accessible' in error message, got: %v", err)
	}
}

// TestRunCWDExecuteOnlyExitsWithCode1 verifies the full run() path returns exit
// code 1 with a clear message when --cwd points to an execute-only directory
// (mode 0o111).  This exercises the EACCES path through the os.Open probe in
// resolveAndValidateCWD where os.Stat succeeds but Open fails.
// On Windows, Unix permission bits (os.Chmod) have no effect, so execute-only
// directories are fully accessible and this scenario cannot be reproduced.
func TestRunCWDExecuteOnlyExitsWithCode1(t *testing.T) {
	if runtime.GOOS == "windows" {
		// TODO: find a Windows-native way to test inaccessible-directory rejection.
		t.Skip("Unix execute-only permission bits have no effect on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	t.Parallel()

	parent := t.TempDir()
	execOnly := filepath.Join(parent, "exec-only")
	if err := os.Mkdir(execOnly, 0o111); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(execOnly, 0o755) })

	var stderr bytes.Buffer
	code := run(
		[]string{"--cwd", execOnly},
		&bytes.Buffer{},
		&stderr,
		func(config.LoadOptions) (config.Result, error) { panic("should not reach load") },
		func(config.Model, startupOptions) error { panic("should not reach start") },
	)

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "failed to resolve --cwd") {
		t.Fatalf("expected 'failed to resolve --cwd' in stderr, got: %q", stderr.String())
	}
}

// --- runWithLogger plumbing (no-op newLogger stub) ---

func noopLogger(logging.Options) *logging.Manager { return nil }

// --- stderr suppression around interactive runtime ---

// TestStartInteractiveSuppressesStderrDuringRun verifies that startInteractive
// calls SetStderrSuppressed(true) before starting the interactive program and
// SetStderrSuppressed(false) after it returns. This is the integration-level
// guard for the alt-screen corruption bug (o7tk root cause 2).
//
// The test uses a probe-logger seam: a *logging.Manager constructed against a
// bytes.Buffer stderr, and a stubbed "start" function that fires a slog.Warn
// via the manager's logger and records whether stderr was clear at that point.
func TestStartInteractiveSuppressesStderrDuringRun(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	logMgr := logging.New(logging.Options{
		StateDir:     stateDir,
		Stderr:       &stderr,
		SessionID:    "integ-supp-test",
		ProjectRoot:  t.TempDir(),
		BuildVersion: "dev",
	})
	t.Cleanup(func() { _ = logMgr.Close() })

	var stderrDuringRun string
	var stderrAfterRun string

	stubStart := func(cfg config.Model, opts startupOptions) error {
		// This simulates the interactive window: fire a warn and capture stderr.
		stderr.Reset()
		opts.logManager.Logger().Warn("runtime-warn-during-interactive")
		stderrDuringRun = stderr.String()
		return nil
	}

	// Wire runWithLogger with a newLogger that returns our probe manager.
	var stderr2 bytes.Buffer
	code := runWithLogger(
		[]string{"--cwd", t.TempDir()},
		&bytes.Buffer{},
		&stderr2,
		func(config.LoadOptions) (config.Result, error) {
			return config.Result{Config: config.Model{}, Path: "(none)"}, nil
		},
		stubStart,
		func(logging.Options) *logging.Manager { return logMgr },
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr2: %q)", code, stderr2.String())
	}

	// During interactive run, warn must NOT appear on stderr.
	if strings.Contains(stderrDuringRun, "runtime-warn-during-interactive") {
		t.Fatalf("regression: warn reached stderr during interactive window — alt-screen would be corrupted; stderr during run: %q", stderrDuringRun)
	}

	// After run, suppression must be lifted — a new warn must reach stderr.
	stderr.Reset()
	logMgr.Logger().Warn("post-run-warn")
	stderrAfterRun = stderr.String()
	if !strings.Contains(stderrAfterRun, "warn: post-run-warn") {
		t.Fatalf("expected warn to reach stderr after interactive run exits, got %q", stderrAfterRun)
	}
}

// TestStartInteractiveNoLogManagerDoesNotPanic verifies that startInteractive
// works correctly when logManager is nil (no logger configured), which is a
// supported path via the noopLogger stub.
func TestStartInteractiveNoLogManagerDoesNotPanic(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("startInteractive panicked with nil logManager: %v", r)
		}
	}()

	stubStart := func(cfg config.Model, opts startupOptions) error {
		// logManager is nil — SetStderrSuppressed must not be called on nil.
		if opts.logManager != nil {
			t.Fatal("expected nil logManager in this test path")
		}
		return nil
	}

	code := runWithLogger(
		[]string{"--cwd", t.TempDir()},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(config.LoadOptions) (config.Result, error) {
			return config.Result{Config: config.Model{}, Path: "(none)"}, nil
		},
		stubStart,
		noopLogger,
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

// --- --repo / --repo-file flag tests ---

// TestParseCLIRepoFlags exercises flag parsing and validation for --repo and
// --repo-file. parseCLI is the correct test seam: it takes []string and returns
// (cliOptions, exitCode, ok), so table-driven tests can be written directly
// without spawning a subprocess or wiring a full run().
func TestParseCLIRepoFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		args           []string
		wantOK         bool
		wantCode       int
		wantRepo       string
		wantFile       string
		stderrContains []string
	}{
		{
			name:     "defaults: repo=beads, repoFile empty",
			args:     []string{},
			wantOK:   true,
			wantRepo: "beads",
			wantFile: "",
		},
		{
			name:     "--repo beads alone is valid",
			args:     []string{"--repo", "beads"},
			wantOK:   true,
			wantRepo: "beads",
			wantFile: "",
		},
		{
			name:     "--repo memory with --repo-file is valid",
			args:     []string{"--repo", "memory", "--repo-file", "x.jsonl"},
			wantOK:   true,
			wantRepo: "memory",
			wantFile: "x.jsonl",
		},
		{
			name:           "--repo memory without --repo-file errors exit 2",
			args:           []string{"--repo", "memory"},
			wantOK:         false,
			wantCode:       2,
			stderrContains: []string{"memory", "--repo-file"},
		},
		{
			name:           "--repo bogus errors exit 2",
			args:           []string{"--repo", "bogus"},
			wantOK:         false,
			wantCode:       2,
			stderrContains: []string{"beads or memory", "bogus"},
		},
		{
			name:     "--repo-file alone (beads mode) is accepted",
			args:     []string{"--repo-file", "data.jsonl"},
			wantOK:   true,
			wantRepo: "beads",
			wantFile: "data.jsonl",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			opts, code, ok := parseCLI(tc.args, &stderr)

			if ok != tc.wantOK {
				t.Fatalf("ok: got %v, want %v (stderr=%q)", ok, tc.wantOK, stderr.String())
			}
			if !tc.wantOK {
				if code != tc.wantCode {
					t.Fatalf("exit code: got %d, want %d", code, tc.wantCode)
				}
				for _, want := range tc.stderrContains {
					if !strings.Contains(stderr.String(), want) {
						t.Errorf("stderr: want %q in %q", want, stderr.String())
					}
				}
				return
			}

			if opts.repo != tc.wantRepo {
				t.Errorf("opts.repo: got %q, want %q", opts.repo, tc.wantRepo)
			}
			if opts.repoFile != tc.wantFile {
				t.Errorf("opts.repoFile: got %q, want %q", opts.repoFile, tc.wantFile)
			}
		})
	}
}

// TestDefaultRepoFilePath verifies the derived cache path is deterministic and
// has the expected shape.
func TestDefaultRepoFilePath(t *testing.T) {
	t.Parallel()

	path1 := defaultRepoFilePath("/home/user/projects/myproject")
	path2 := defaultRepoFilePath("/home/user/projects/myproject")
	if path1 != path2 {
		t.Fatalf("defaultRepoFilePath not deterministic: %q != %q", path1, path2)
	}

	other := defaultRepoFilePath("/home/user/projects/other")
	if path1 == other {
		t.Fatalf("different roots produced the same hash path: %q", path1)
	}

	if !strings.HasSuffix(path1, "repo.jsonl") {
		t.Errorf("expected path to end with repo.jsonl, got %q", path1)
	}
	if !strings.Contains(path1, "bwb") {
		t.Errorf("expected path to contain 'bwb', got %q", path1)
	}
}

// TestRunRepoMemoryLoadsFromFile exercises the full run() path with --repo=memory
// and a valid fixture file produced by filestorage.Save, using a stub start that
// asserts the startupOptions have the correct repoFlag and repoFile set.
func TestRunRepoMemoryLoadsFromFile(t *testing.T) {
	t.Parallel()

	// Build a tiny in-memory repo and save it to a temp file.
	r := memory.New()
	r.Seed(memory.Issue{
		ID:     "fixture-1",
		Title:  "Fixture issue",
		Status: "open",
		Type:   "task",
	})

	dir := t.TempDir()
	repoFile := filepath.Join(dir, "repo.jsonl")
	if err := filestorage.Save(r, repoFile); err != nil {
		t.Fatalf("filestorage.Save: %v", err)
	}

	var seenOpts startupOptions
	started := false

	var stderr bytes.Buffer
	code := runWithLogger(
		[]string{"--repo", "memory", "--repo-file", repoFile, "--cwd", t.TempDir()},
		&bytes.Buffer{},
		&stderr,
		func(config.LoadOptions) (config.Result, error) {
			return config.Result{Config: config.Model{}, Path: "(none)"}, nil
		},
		func(cfg config.Model, opts startupOptions) error {
			started = true
			seenOpts = opts
			return nil
		},
		noopLogger,
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, stderr.String())
	}
	if !started {
		t.Fatal("expected start to be called")
	}
	if seenOpts.repoFlag != "memory" {
		t.Errorf("repoFlag: got %q, want %q", seenOpts.repoFlag, "memory")
	}
	if seenOpts.repoFile != repoFile {
		t.Errorf("repoFile: got %q, want %q", seenOpts.repoFile, repoFile)
	}
}

// TestRunRepoMemoryWithoutFileExitsCode2 verifies that --repo=memory without
// --repo-file exits with code 2 and a clear message.
func TestRunRepoMemoryWithoutFileExitsCode2(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	code := run(
		[]string{"--repo", "memory"},
		&bytes.Buffer{},
		&stderr,
		func(config.LoadOptions) (config.Result, error) { panic("should not reach load") },
		func(config.Model, startupOptions) error { panic("should not reach start") },
	)

	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	msg := stderr.String()
	if !strings.Contains(msg, "memory") || !strings.Contains(msg, "--repo-file") {
		t.Fatalf("expected error message referencing 'memory' and '--repo-file', got: %q", msg)
	}
}

// TestRunRepoBeadsPassesDefaultRepoFilePath verifies that --repo=beads derives
// a non-empty default repo-file path and passes it into startupOptions.
func TestRunRepoBeadsPassesDefaultRepoFilePath(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	var seenOpts startupOptions

	code := runWithLogger(
		[]string{"--cwd", projectDir},
		&bytes.Buffer{},
		&bytes.Buffer{},
		func(config.LoadOptions) (config.Result, error) {
			return config.Result{Config: config.Model{}, Path: "(none)"}, nil
		},
		func(cfg config.Model, opts startupOptions) error {
			seenOpts = opts
			return nil
		},
		noopLogger,
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if seenOpts.repoFlag != "beads" {
		t.Errorf("repoFlag: got %q, want %q", seenOpts.repoFlag, "beads")
	}
	if seenOpts.repoFile == "" {
		t.Error("expected non-empty default repoFile for beads mode")
	}
	if !strings.HasSuffix(seenOpts.repoFile, "repo.jsonl") {
		t.Errorf("expected repoFile to end with repo.jsonl, got %q", seenOpts.repoFile)
	}
}
