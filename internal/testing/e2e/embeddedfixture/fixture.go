package embeddedfixture

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Spec is the fixture seed specification consumed by the setup script.
type Spec struct {
	Prefix string           `json:"prefix"`
	Issues []IssueSpec      `json:"issues"`
	Deps   []DependencySpec `json:"dependencies"`
}

// IssueSpec describes one deterministic issue seed.
type IssueSpec struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Priority    int      `json:"priority"`
	Status      string   `json:"status"`
	Assignee    string   `json:"assignee"`
	Labels      []string `json:"labels"`
	Comments    []string `json:"comments"`
}

// DependencySpec states that BlockerID blocks BlockedID.
type DependencySpec struct {
	BlockerID string `json:"blocker_id"`
	BlockedID string `json:"blocked_id"`
}

// Paths returns package-relative paths to fixture assets.
func Paths(tb testing.TB) (scriptPath, seedPath string) {
	tb.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("failed to resolve embeddedfixture paths")
	}

	baseDir := filepath.Dir(file)
	return filepath.Join(baseDir, "setup.sh"), filepath.Join(baseDir, "seed.json")
}

// TempRepoPath returns a recommended temp directory path for fixture creation.
func TempRepoPath(tb testing.TB) string {
	tb.Helper()

	name := strings.ToLower(strings.ReplaceAll(tb.Name(), "/", "-"))
	return filepath.Join(tb.TempDir(), "embedded-beads-fixture-"+name)
}

// Seed creates and seeds an embedded-mode beads fixture repository.
//
// The fixture setup is delegated to setup.sh so integration and smoke tests can
// use the exact same reproducible data source.
func Seed(tb testing.TB, repoPath string) {
	tb.Helper()

	scriptPath, seedPath := Paths(tb)

	cmd := exec.Command("sh", scriptPath, repoPath, seedPath)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("failed to seed embedded fixture repo %q: %v\n%s", repoPath, err, out)
	}

	if !strings.Contains(string(out), "fixture-ready:") {
		tb.Fatalf("unexpected fixture setup output: %s", out)
	}
}

// sharedCache holds the process-level shared fixture cache state.
var sharedCache struct {
	once    sync.Once
	repoDir string
	initErr error
}

// seedSharedCache seeds the fixture into a process-local cache directory and
// verifies it with a bd sanity check, without going through testing.TB.Fatalf,
// so errors can be captured cleanly inside sync.Once.
func seedSharedCache(scriptPath, seedPath, cacheDir string) error {
	cmd := exec.Command("sh", scriptPath, cacheDir, seedPath)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("seed script failed: %w\n%s", err, out)
	}
	if !strings.Contains(string(out), "fixture-ready:") {
		return fmt.Errorf("unexpected fixture setup output: %s", out)
	}

	// Sanity check: run bd ready against the freshly seeded cache dir to confirm
	// the fixture is usable before any per-test copies are made.
	readyCmd := exec.Command("bd", "ready", "--json")
	readyCmd.Dir = cacheDir
	readyCmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	if out, err := readyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sanity check 'bd ready' failed in cache repo %q: %w\n%s", cacheDir, err, out)
	}
	return nil
}

// SharedFixtureRepoPath seeds the bd fixture once per process (sync.Once-guarded)
// in a process-local cache directory and returns a fresh per-test copy of the
// seeded repository containing both .git/ and .beads/.
//
// The first call is slow (~10s) because it forks bd to populate the fixture.
// Subsequent calls are fast (<100ms) because they copy the pre-seeded cache
// directory rather than re-running the setup script.
//
// The returned path is a tb.TempDir()-backed directory that is automatically
// cleaned up when the test ends.
func SharedFixtureRepoPath(tb testing.TB) string {
	tb.Helper()

	sharedCache.once.Do(func() {
		cacheDir := filepath.Join(os.TempDir(), "bwb-embedded-fixture-cache")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			sharedCache.initErr = fmt.Errorf("create fixture cache dir %q: %w", cacheDir, err)
			return
		}

		scriptPath, seedPath := Paths(tb)
		if err := seedSharedCache(scriptPath, seedPath, cacheDir); err != nil {
			sharedCache.initErr = fmt.Errorf("seed shared fixture cache in %q: %w", cacheDir, err)
			return
		}
		sharedCache.repoDir = cacheDir
	})

	if sharedCache.initErr != nil {
		tb.Fatalf("SharedFixtureRepoPath: %v", sharedCache.initErr)
	}

	// Create a fresh per-test copy by copying .git/ and .beads/ into a new tempdir.
	destDir := filepath.Join(tb.TempDir(), "shared-embedded-fixture")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		tb.Fatalf("SharedFixtureRepoPath: create dest dir %q: %v", destDir, err)
	}

	for _, subdir := range []string{".git", ".beads"} {
		src := filepath.Join(sharedCache.repoDir, subdir)
		dst := filepath.Join(destDir, subdir)
		cmd := exec.Command("cp", "-a", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			tb.Fatalf("SharedFixtureRepoPath: copy %q to %q: %v\n%s", src, dst, err, out)
		}
	}

	return destDir
}

// ReadSeedSpec loads the deterministic fixture seed specification.
func ReadSeedSpec(tb testing.TB) Spec {
	tb.Helper()

	_, seedPath := Paths(tb)
	bts, err := os.ReadFile(seedPath)
	if err != nil {
		tb.Fatalf("failed to read fixture seed %q: %v", seedPath, err)
	}

	var spec Spec
	if err := json.Unmarshal(bts, &spec); err != nil {
		tb.Fatalf("failed to decode fixture seed %q: %v", seedPath, err)
	}

	if spec.Prefix == "" {
		tb.Fatalf("invalid fixture seed %q: missing prefix", seedPath)
	}

	return spec
}
