// Package datasets provides read-only multi-DB test adapters for parity tests
// that compare workbench data layer output against bd CLI output on real datasets.
//
// # Dual-fixture policy
//
// Use Fixture for tests that need exact ID assertions (bwf-1, bwf-2, bwf-3 etc.);
// use ScaleFixture for tests that need realistic edge cases (cap engagement on the
// Done column, cardinality thresholds, sort tie-breaks, search corpus depth).
//
// Four dataset sources are supported:
//   - Fixture: minimal anchor (3 issues) — exact-ID assertions, always available
//   - ScaleFixture: realistic scale (~590 issues) — edge-case tests; opt-in via BWB_SCALE_FIXTURE=1
//   - ThisRepo: this repository's own .beads/ (read-only; gated behind BWB_PARITY_THIS_REPO=1)
//   - External: an arbitrary external repo at BWB_PARITY_EXTERNAL_PATH (read-only;
//     skipped when the env var is unset or the path lacks .beads/)
//
// Usage:
//
//	ds := datasets.Fixture(t)
//	gw := datasets.NewGateway(t, ds)
//	out, err := datasets.BdList(t, ds, "--status", "open")
package datasets

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	beads "github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/testing/e2e/embeddedfixture"
)

const (
	// EnvParityThisRepo gates ThisRepo. Must be set to "1" to enable.
	EnvParityThisRepo = "BWB_PARITY_THIS_REPO"
	// EnvParityExternalPath points External at an arbitrary repo root that
	// contains a .beads/ directory. Unset or empty → External skips cleanly.
	EnvParityExternalPath = "BWB_PARITY_EXTERNAL_PATH"
	// EnvScaleFixture gates ScaleFixture. Must be set to "1" to enable.
	// Seeding the scale fixture (~590 issues via bd subprocesses) takes several
	// minutes, so it is opt-in for integration speed.
	EnvScaleFixture = "BWB_SCALE_FIXTURE"
)

// Dataset describes one beads database available to parity tests.
type Dataset struct {
	// Name is a human-readable identifier used in test names and error messages.
	Name string
	// Path is the absolute path to the directory containing .beads/.
	Path string
	// ReadOnly indicates that no write operations are permitted on this dataset.
	// When true, NewGateway prepends --readonly to every bd argv, and the Bd*
	// helpers also pass --readonly.
	ReadOnly bool
}

// WritableTempFixture returns a Dataset backed by a fresh empty beads database
// created in a per-test temporary directory. The database has no issues; use it
// for write-contract tests that need an isolated, mutable repository.
//
// The bd binary must be on PATH; the test is skipped otherwise.
// The database is initialised with BD_NON_INTERACTIVE=1 bd init and is
// automatically cleaned up when the test ends via t.TempDir().
func WritableTempFixture(t *testing.T) Dataset {
	t.Helper()

	skipUnlessBdOnPath(t)

	dir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", "init")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("WritableTempFixture: bd init failed in %q: %v\n%s", dir, err, out)
	}

	return Dataset{
		Name:     "writable-temp",
		Path:     dir,
		ReadOnly: false,
	}
}

// Fixture returns a Dataset backed by a fresh, writable copy of the embedded
// fixture repository. The copy is seeded once per process via
// embeddedfixture.SharedFixtureRepoPath and cleaned up automatically by t.
//
// This dataset is always available (no env gate required).
func Fixture(t *testing.T) Dataset {
	t.Helper()

	repoPath := embeddedfixture.SharedFixtureRepoPath(t)
	return Dataset{
		Name:     "fixture",
		Path:     repoPath,
		ReadOnly: false,
	}
}

// ScaleFixture returns a Dataset backed by a fresh, writable copy of the scale
// embedded fixture repository (~590 issues, edge-case-designed). The copy is
// seeded once per process via embeddedfixture.SharedScaleFixtureRepoPath and
// cleaned up automatically by t.
//
// The test is skipped unless BWB_SCALE_FIXTURE=1 is set. Seeding ~590 issues
// via bd subprocesses takes several minutes, so this dataset is opt-in for
// integration speed.
//
// Use ScaleFixture for tests that need realistic edge cases: cap engagement on
// the Done column (>50 closed issues), cardinality warnings (>500 active),
// sort tie-breaks, and search corpus depth. Use Fixture for exact-ID assertions.
func ScaleFixture(t *testing.T) Dataset {
	t.Helper()

	repoPath := embeddedfixture.SharedScaleFixtureRepoPath(t)
	return Dataset{
		Name:     "scale-fixture",
		Path:     repoPath,
		ReadOnly: false,
	}
}

// ThisRepo returns a Dataset pointing at the beads-workbench repository's own
// .beads/ directory. The dataset is read-only to protect the live tracker.
//
// The test is skipped unless BWB_PARITY_THIS_REPO=1 is set. The bd binary must
// also be on PATH.
func ThisRepo(t *testing.T) Dataset {
	t.Helper()

	skipUnlessEnvSet(t, EnvParityThisRepo)
	skipUnlessBdOnPath(t)

	return Dataset{
		Name:     "this-repo",
		Path:     repoRoot(t),
		ReadOnly: true,
	}
}

// External returns a Dataset pointing at an arbitrary external repository
// whose root path is read from BWB_PARITY_EXTERNAL_PATH. The dataset is
// read-only. No project-specific default is configured: if the env var is
// unset, empty, or points at a directory without .beads/, the test skips.
//
// The bd binary must also be on PATH.
func External(t *testing.T) Dataset {
	t.Helper()

	skipUnlessBdOnPath(t)

	path := os.Getenv(EnvParityExternalPath)
	if path == "" {
		t.Skipf("datasets.External: %s is not set; skipping opt-in external dataset test", EnvParityExternalPath)
	}

	if _, err := os.Stat(filepath.Join(path, ".beads")); err != nil {
		t.Skipf("datasets.External: .beads/ not found at %q (%v); skipping", path, err)
	}

	return Dataset{
		Name:     "external",
		Path:     path,
		ReadOnly: true,
	}
}

// NewGateway constructs a BeadsGateway bound to ds.Path.
//
// When ds.ReadOnly is true the runner is configured with ReadOnly: true, which
// causes --readonly to be prepended to every bd argv. Any attempted write
// through the returned gateway will fail with ErrorCodeCommandFailed.
func NewGateway(t *testing.T, ds Dataset) beads.BeadsGateway {
	t.Helper()

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir:  ds.Path,
		ReadOnly: ds.ReadOnly,
	})

	return beads.NewCLIGateway(runner)
}

// BdCount runs "bd [--readonly] count <args> --json" from ds.Path and returns
// raw stdout. The caller is responsible for JSON decoding.
//
// --readonly is prepended automatically when ds.ReadOnly is true.
func BdCount(t *testing.T, ds Dataset, args ...string) ([]byte, error) {
	t.Helper()
	return runBd(t, ds, "count", args)
}

// BdList runs "bd [--readonly] list <args> --json" from ds.Path and returns
// raw stdout.
func BdList(t *testing.T, ds Dataset, args ...string) ([]byte, error) {
	t.Helper()
	return runBd(t, ds, "list", args)
}

// BdReady runs "bd [--readonly] ready <args> --json" from ds.Path and returns
// raw stdout.
func BdReady(t *testing.T, ds Dataset, args ...string) ([]byte, error) {
	t.Helper()
	return runBd(t, ds, "ready", args)
}

// BdBlocked runs "bd [--readonly] blocked <args> --json" from ds.Path and
// returns raw stdout.
func BdBlocked(t *testing.T, ds Dataset, args ...string) ([]byte, error) {
	t.Helper()
	return runBd(t, ds, "blocked", args)
}

// BdSearch runs "bd [--readonly] search <args> --json" from ds.Path and
// returns raw stdout.
func BdSearch(t *testing.T, ds Dataset, args ...string) ([]byte, error) {
	t.Helper()
	return runBd(t, ds, "search", args)
}

// runBd is the shared helper that assembles and executes a bd invocation.
// The argv structure is: [--readonly] <verb> [args...] --json
func runBd(t *testing.T, ds Dataset, verb string, extraArgs []string) ([]byte, error) {
	t.Helper()

	argv := make([]string, 0, len(extraArgs)+3)
	if ds.ReadOnly {
		argv = append(argv, "--readonly")
	}
	argv = append(argv, verb)
	argv = append(argv, extraArgs...)
	argv = append(argv, "--json")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", argv...)
	cmd.Dir = ds.Path
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")

	return cmd.Output()
}

// skipUnlessEnvSet skips the test if the named environment variable is not set
// to "1".
func skipUnlessEnvSet(t *testing.T, name string) {
	t.Helper()

	if os.Getenv(name) != "1" {
		t.Skipf("datasets: %s is not set to 1; skipping opt-in dataset test", name)
	}
}

// skipUnlessBdOnPath skips the test if bd is not on PATH.
func skipUnlessBdOnPath(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("datasets: bd not found on PATH; skipping")
	}
}

// repoRoot resolves the repository root by walking up from this source file's
// own location until a go.mod is found.
func repoRoot(t *testing.T) string {
	t.Helper()

	// Use the source file location of this package (frame 0 = repoRoot itself).
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("datasets.repoRoot: failed to resolve source path")
	}

	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("datasets.repoRoot: go.mod not found above %q", filepath.Dir(file))
		}
		dir = parent
	}
}
