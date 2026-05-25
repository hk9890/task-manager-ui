//go:build integration

package parity_test

// scale_parity_integration_test.go — cap-engagement parity assertions for the
// Done column.
//
// After iwvm, opts.ClosedLimit is a caller-controlled parameter so ANY fixture
// can exercise the cap path by passing a limit smaller than its closed count.
// The tests below use a small seeded fixture (~3 closed issues) with ClosedLimit=2,
// making them runnable on every mise run test:integration without BWB_SCALE_FIXTURE.
//
// Tests that genuinely require scale-fixture properties (cardinality warnings,
// sort tie-breaks across hundreds of issues) remain gated behind
// datasets.ScaleFixture / BWB_SCALE_FIXTURE=1.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/testing/datasets"
)

// seedCapTestRepo creates an isolated bd repo in dir with nClosed closed issues
// and returns a beads.Repository for it.
func seedCapTestRepo(t *testing.T, nClosed int) (repository.Repository, int) {
	t.Helper()

	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping cap-engagement test")
	}

	dir := filepath.Join(t.TempDir(), "cap-test-repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("seedCapTestRepo: mkdir %q: %v", dir, err)
	}

	runBD := func(args ...string) {
		t.Helper()
		cmd := exec.Command("bd", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bd %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = dir
	gitCmd.Env = os.Environ()
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in %q: %v\n%s", dir, err, out)
	}

	runBD("init", "--non-interactive", "--skip-hooks", "--skip-agents", "--prefix", "cap")

	for i := 0; i < nClosed; i++ {
		id := fmt.Sprintf("cap-%02d", i+1)
		runBD("create", "--id", id, "--title", fmt.Sprintf("Closed issue %02d", i+1))
		runBD("close", id, "--reason", "cap test fixture")
	}

	ds := datasets.Dataset{Name: "cap-test", Path: dir, ReadOnly: false}
	return datasets.NewRepository(t, ds), nClosed
}

// bdCountClosed runs bd count --by-status --json in dir and returns the closed count.
func bdCountClosed(t *testing.T, dir string) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", "count", "--by-status", "--json")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bdCountClosed: %v", err)
	}

	var result struct {
		Groups []struct {
			Count int    `json:"count"`
			Group string `json:"group"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("bdCountClosed: unmarshal: %v", err)
	}

	for _, g := range result.Groups {
		if g.Group == "closed" {
			return g.Count
		}
	}
	return 0
}

// TestScaleParity_DoneColumnCapEngagement exercises the ssom regression class:
// when closed issues exceed ClosedLimit, the Done column must report
// TotalIsExact=false and Done.Total must equal the real DB count (not the cap).
//
// This test uses a small seeded fixture (3 closed issues, ClosedLimit=2) so it
// runs on every mise run test:integration without BWB_SCALE_FIXTURE.
//
// regression class: ssom (Done-column cap badge)
func TestScaleParity_DoneColumnCapEngagement(t *testing.T) {
	const nClosed = 3
	const capLimit = 2

	repo, _ := seedCapTestRepo(t, nClosed)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	data, err := repo.Dashboard(ctx, repository.DashboardOptions{ClosedLimit: capLimit})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}

	cols := runDashboardFetch(t, repo, capLimit)

	// Done.Total must equal the real DB count (not the cap).
	t.Run("DoneTotalEqualsRealClosedCount", func(t *testing.T) {
		if data.ClosedTotal != nClosed {
			t.Errorf("ssom: ClosedTotal=%d; want %d (real count); cap must not leak into Total",
				data.ClosedTotal, nClosed)
		}
		if cols.Done.Total != nClosed {
			t.Errorf("ssom: Done.Total=%d; want %d (real count)", cols.Done.Total, nClosed)
		}
	})

	// Done.TotalIsExact must be false when capped ("N of M" badge signal).
	t.Run("DoneTotalIsExactFalseWhenCapped", func(t *testing.T) {
		if cols.Done.TotalIsExact {
			t.Errorf("ssom: Done.TotalIsExact=true when %d closed > cap %d; expected false",
				nClosed, capLimit)
		}
	})

	// Done.Issues row count must not exceed the cap.
	t.Run("DoneRowsRespectCap", func(t *testing.T) {
		if len(data.Closed) > capLimit {
			t.Errorf("ssom: len(Closed)=%d; want <=%d (cap must be respected)",
				len(data.Closed), capLimit)
		}
		if len(cols.Done.Issues) > capLimit {
			t.Errorf("ssom: Done.Issues len=%d; want <=%d", len(cols.Done.Issues), capLimit)
		}
	})

	t.Logf("cap engagement OK: nClosed=%d cap=%d Closed=%d ClosedTotal=%d TotalIsExact=%v",
		nClosed, capLimit, len(data.Closed), data.ClosedTotal, cols.Done.TotalIsExact)
}

// TestScaleParity_CapEngagement_VsBdCount verifies that the Done.Total count
// reported by the bwb data path under cap is identical to the source-of-truth
// count from `bd count --by-status` for the seeded fixture.
//
// regression class: ssom
func TestScaleParity_CapEngagement_VsBdCount(t *testing.T) {
	const nClosed = 3
	const capLimit = 2

	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping cap-engagement test")
	}

	dir := filepath.Join(t.TempDir(), "cap-vsbdcount-repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runBD := func(args ...string) {
		t.Helper()
		cmd := exec.Command("bd", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bd %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = dir
	gitCmd.Env = os.Environ()
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	runBD("init", "--non-interactive", "--skip-hooks", "--skip-agents", "--prefix", "vbc")

	for i := 0; i < nClosed; i++ {
		id := fmt.Sprintf("vbc-%02d", i+1)
		runBD("create", "--id", id, "--title", fmt.Sprintf("Closed %02d", i+1))
		runBD("close", id, "--reason", "vbc fixture")
	}

	ds := datasets.Dataset{Name: "vbc-test", Path: dir, ReadOnly: false}
	repo := datasets.NewRepository(t, ds)

	cols := runDashboardFetch(t, repo, capLimit)
	bdClosed := bdCountClosed(t, dir)

	if cols.Done.Total != bdClosed {
		t.Errorf("ssom cap parity: Done.Total=%d; bd count=%d; delta=%d",
			cols.Done.Total, bdClosed, cols.Done.Total-bdClosed)
	}

	t.Logf("cap vs bd count OK: Done.Total=%d bdClosed=%d", cols.Done.Total, bdClosed)
}
