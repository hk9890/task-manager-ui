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
