//go:build integration

package bd

import (
	"context"
	"os/exec"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
)

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
	assertRepositoryErrorCode(t, err, domain.ErrorCodeNoDatabaseFound)

	// Pin the exact substring that the detection logic in runner.go depends on.
	// If bd renames this message in a future release this assertion will fail,
	// signalling that runner.go's substring detection must be revisited.
	const pinnedSubstring = "no beads database found"
	assertContains(t, err.Error(), pinnedSubstring)
}
