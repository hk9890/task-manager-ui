//go:build integration

package embeddedfixture

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// TestSharedFixtureRepoPathRoundTrip verifies that:
//   - Two sub-tests each receive a distinct, usable fixture repository.
//   - Both repos contain .beads/ data (config.yaml as canary).
//   - Both repos successfully respond to a bd query.
//   - The second call is at least 10x faster than the first (perf guard).
func TestSharedFixtureRepoPathRoundTrip(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}

	var (
		path1    atomic.Value
		path2    atomic.Value
		elapsed1 atomic.Int64
		elapsed2 atomic.Int64
	)

	t.Run("first", func(t *testing.T) {
		start := time.Now()
		p := SharedFixtureRepoPath(t)
		dur := time.Since(start)

		path1.Store(p)
		elapsed1.Store(int64(dur))
		t.Logf("first call: %v -> %s", dur, p)

		assertFixtureUsable(t, p)
	})

	t.Run("second", func(t *testing.T) {
		start := time.Now()
		p := SharedFixtureRepoPath(t)
		dur := time.Since(start)

		path2.Store(p)
		elapsed2.Store(int64(dur))
		t.Logf("second call: %v -> %s", dur, p)

		assertFixtureUsable(t, p)
	})

	// Cross-sub-test assertions: paths must be distinct.
	p1 := path1.Load().(string)
	p2 := path2.Load().(string)

	if p1 == "" {
		t.Fatal("first sub-test returned empty path")
	}
	if p2 == "" {
		t.Fatal("second sub-test returned empty path")
	}
	if p1 == p2 {
		t.Fatalf("expected distinct paths; both returned %q", p1)
	}

	// Perf guard: second call must be at least 10x faster than the first.
	dur1 := time.Duration(elapsed1.Load())
	dur2 := time.Duration(elapsed2.Load())

	if dur1 < 100*time.Millisecond {
		t.Logf("warning: first call unexpectedly fast (%v); perf guard may be unreliable (cache already warm?)", dur1)
	} else {
		speedup := float64(dur1) / float64(dur2)
		if speedup < 10 {
			t.Errorf("expected second call to be >=10x faster than first; first=%v second=%v speedup=%.1fx", dur1, dur2, speedup)
		} else {
			t.Logf("perf guard passed: %.1fx speedup (first=%v second=%v)", speedup, dur1, dur2)
		}
	}
}

// assertFixtureUsable verifies that the given repo path contains a usable beads
// fixture: a .beads directory with its config, the embedded data store, and a
// .git directory; and that bd ready responds successfully.
func assertFixtureUsable(t *testing.T, repoPath string) {
	t.Helper()

	// .beads/config.yaml must exist (canary for the fixture data directory).
	configPath := filepath.Join(repoPath, ".beads", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("expected .beads/config.yaml in %q: %v", repoPath, err)
	}

	// .beads/embeddeddolt must exist (the bd embedded data store).
	doltPath := filepath.Join(repoPath, ".beads", "embeddeddolt")
	if _, err := os.Stat(doltPath); err != nil {
		t.Errorf("expected .beads/embeddeddolt/ in %q: %v", repoPath, err)
	}

	// .git must exist.
	gitPath := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		t.Errorf("expected .git/ in %q: %v", repoPath, err)
	}

	// bd ready --json must succeed (end-to-end usability check).
	cmd := exec.Command("bd", "ready", "--json")
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("bd ready --json failed in %q: %v\n%s", repoPath, err, out)
	}
}
