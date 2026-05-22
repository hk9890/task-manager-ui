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

// scaleSeedCompleteMarker is the sentinel file written inside the cache
// directory after a successful full-scale-seed run.  Its presence means
// every issue in scale-seed.json has been committed to the cache.
const scaleSeedCompleteMarker = ".bwb-scale-seed-complete"

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

// sharedScaleCache holds the process-level shared scale fixture cache state.
// Seeding scale-seed.json (~590 issues) takes several minutes.
// This cache is opt-in: set BWB_SCALE_FIXTURE=1 to enable scale fixture seeding.
var sharedScaleCache struct {
	once    sync.Once
	repoDir string
	initErr error
}

// ScaleSeedPath returns the absolute path to scale-seed.json.
func ScaleSeedPath(tb testing.TB) string {
	tb.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("failed to resolve embeddedfixture paths via runtime.Caller")
	}
	return filepath.Join(filepath.Dir(file), "scale-seed.json")
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

// seedScaleSharedCache seeds the scale fixture into cacheDir, protected by a
// cross-process exclusive file lock.  Multiple test-binary processes can call
// this concurrently: all but the winner block on the lock; when the winner
// finishes and writes scaleSeedCompleteMarker the waiters check the marker and
// skip re-seeding.
//
// The lock file is cacheDir + ".lock" — a sibling of the cache dir itself.
func seedScaleSharedCache(scriptPath, seedPath, cacheDir string) error {
	lockPath := cacheDir + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file %q: %w", lockPath, err)
	}
	defer func() { _ = lockFile.Close() }()

	// Acquire an exclusive lock — blocks until any concurrent seeder finishes.
	if err := acquireFileLock(lockFile); err != nil {
		return fmt.Errorf("acquire scale fixture lock %q: %w", lockPath, err)
	}
	defer func() { _ = releaseFileLock(lockFile) }()

	// Under the lock: if a previous process already completed the seed, skip.
	markerPath := filepath.Join(cacheDir, scaleSeedCompleteMarker)
	if _, err := os.Stat(markerPath); err == nil {
		// Marker present — cache is complete.
		return nil
	}

	// Seed the cache.
	if err := seedSharedCache(scriptPath, seedPath, cacheDir); err != nil {
		return err
	}

	// Write the completion marker so subsequent processes skip seeding.
	if err := os.WriteFile(markerPath, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("write scale seed complete marker %q: %w", markerPath, err)
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
		// The cache is seeded once per process (sync.Once) and is not designed
		// for cross-process sharing — unlike the scale cache, it has no lock or
		// completion marker. A fixed path let a broken or half-seeded cache
		// from a prior run, or a concurrent test-binary process, poison every
		// subsequent seed: setup.sh skips `bd init` when .beads already exists,
		// so a stale .beads fails later with "issue_prefix config is missing".
		// A process-unique dir keeps each test binary isolated.
		cacheDir := filepath.Join(os.TempDir(), fmt.Sprintf("bwb-embedded-fixture-cache-%d", os.Getpid()))
		if err := os.RemoveAll(cacheDir); err != nil {
			sharedCache.initErr = fmt.Errorf("clean fixture cache dir %q: %w", cacheDir, err)
			return
		}
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

// SharedScaleFixtureRepoPath seeds the scale bd fixture (scale-seed.json, ~590
// issues) once per process (sync.Once-guarded) in a process-local cache
// directory and returns a fresh per-test copy of the seeded repository.
//
// Seeding ~590 issues via bd subprocesses takes several minutes. This function
// is therefore opt-in: the test is skipped unless BWB_SCALE_FIXTURE=1 is set.
//
// The returned path is a tb.TempDir()-backed directory that is automatically
// cleaned up when the test ends.
//
// The bd and git binaries must be on PATH.
func SharedScaleFixtureRepoPath(tb testing.TB) string {
	tb.Helper()

	if os.Getenv("BWB_SCALE_FIXTURE") != "1" {
		tb.Skip("scale fixture: set BWB_SCALE_FIXTURE=1 to enable (seeding ~590 issues takes several minutes)")
	}

	sharedScaleCache.once.Do(func() {
		cacheDir := filepath.Join(os.TempDir(), "bwb-scale-fixture-cache")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			sharedScaleCache.initErr = fmt.Errorf("create scale fixture cache dir %q: %w", cacheDir, err)
			return
		}

		scriptPath, _ := Paths(tb)
		seedPath := ScaleSeedPath(tb)
		// seedScaleSharedCache uses a cross-process file lock to prevent
		// concurrent seeding from multiple test-binary processes, and
		// skips re-seeding when the completion marker is already present.
		if err := seedScaleSharedCache(scriptPath, seedPath, cacheDir); err != nil {
			sharedScaleCache.initErr = fmt.Errorf("seed shared scale fixture cache in %q: %w", cacheDir, err)
			return
		}
		sharedScaleCache.repoDir = cacheDir
	})

	if sharedScaleCache.initErr != nil {
		tb.Fatalf("SharedScaleFixtureRepoPath: %v", sharedScaleCache.initErr)
	}

	// Create a fresh per-test copy by copying .git/ and .beads/ into a new tempdir.
	destDir := filepath.Join(tb.TempDir(), "shared-scale-fixture")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		tb.Fatalf("SharedScaleFixtureRepoPath: create dest dir %q: %v", destDir, err)
	}

	for _, subdir := range []string{".git", ".beads"} {
		src := filepath.Join(sharedScaleCache.repoDir, subdir)
		dst := filepath.Join(destDir, subdir)
		cmd := exec.Command("cp", "-a", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			tb.Fatalf("SharedScaleFixtureRepoPath: copy %q to %q: %v\n%s", src, dst, err, out)
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
