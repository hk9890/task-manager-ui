//go:build unix

package config

// The regular-file precondition on the config path. Its sibling directory guard
// is pinned by TestLoad_DirectoryAtConfigPathReturnsError, but this half could
// be deleted with the whole repository green — and the half-covered stat block
// reads as if the validation were fully tested.
//
// Unix-only: the test needs a path that exists, is not a directory, and is not
// a regular file. The project ships Linux and macOS binaries only
// (.goreleaser.yaml), so a Unix build constraint costs no supported platform.

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// irregularConfigPath returns a path holding a Unix domain socket.
//
// A socket rather than a FIFO, deliberately. Both exercise the guard, but with
// the guard removed os.ReadFile on a FIFO blocks until a writer appears, so the
// test would hang for the full go-test timeout instead of failing — the very
// startup hang this guard exists to prevent, reproduced inside the suite. A
// socket makes the same read fail immediately, so a regression here is a fast
// red rather than a ten-minute stall.
func irregularConfigPath(t *testing.T) string {
	t.Helper()

	// Socket paths are limited to ~104 bytes, so keep the directory short
	// rather than nesting under the test name.
	path := filepath.Join(t.TempDir(), "cfg.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("cannot create a unix socket at %q: %v", path, err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	return path
}

// TestLoadRejectsANonRegularConfigFile asserts startup fails with a message
// naming the problem, rather than falling through to a read that cannot
// succeed.
func TestLoadRejectsANonRegularConfigFile(t *testing.T) {
	t.Parallel()

	path := irregularConfigPath(t)

	_, err := LoadWithOptions(LoadOptions{Path: path})

	if err == nil {
		t.Fatal("a socket at the config path was accepted as configuration")
	}
	if !strings.Contains(err.Error(), "is not a regular file") {
		t.Fatalf("error = %v, want it to name the irregular file", err)
	}
}

// The guard must not reject an ordinary file, or every start fails.
func TestLoadAcceptsARegularConfigFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("editor:\n  command: vi\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadWithOptions(LoadOptions{Path: path}); err != nil {
		t.Fatalf("a regular config file was rejected: %v", err)
	}
}
