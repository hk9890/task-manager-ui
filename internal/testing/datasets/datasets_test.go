//go:build integration

package datasets_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/testing/datasets"
)

// TestFixtureDataset verifies that Fixture returns a usable Dataset and
// NewGateway produces a gateway whose ListIssues succeeds.
func TestFixtureDataset(t *testing.T) {
	t.Parallel()

	ds := datasets.Fixture(t)

	if ds.Name == "" {
		t.Fatal("Fixture returned empty Name")
	}
	if ds.Path == "" {
		t.Fatal("Fixture returned empty Path")
	}
	if ds.ReadOnly {
		t.Fatal("Fixture dataset must be writable (ReadOnly == false)")
	}

	gw := datasets.NewGateway(t, ds)

	issues, err := gw.ListIssues(context.Background(), domain.IssueListQuery{})
	if err != nil {
		t.Fatalf("ListIssues against fixture failed: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("ListIssues returned no issues against fixture; expected seeded data")
	}
}

// TestThisRepoSkipsWhenEnvNotSet verifies that ThisRepo skips cleanly when the
// env gate is absent.
func TestThisRepoSkipsWhenEnvNotSet(t *testing.T) {
	if os.Getenv(datasets.EnvParityThisRepo) != "" {
		t.Skipf("%s is set; this test verifies the skip path when it is unset", datasets.EnvParityThisRepo)
	}

	var subT *testing.T
	t.Run("gate-off", func(s *testing.T) {
		subT = s
		datasets.ThisRepo(s) // expected to call s.Skip() and abort this goroutine
		// Unreachable when skip works correctly.
	})

	if !subT.Skipped() {
		t.Fatalf("datasets.ThisRepo did not skip when %s was unset", datasets.EnvParityThisRepo)
	}
}

// TestExternalSkipsWhenEnvNotSet verifies that External skips cleanly when
// BWB_PARITY_EXTERNAL_PATH is absent.
func TestExternalSkipsWhenEnvNotSet(t *testing.T) {
	if os.Getenv(datasets.EnvParityExternalPath) != "" {
		t.Skipf("%s is set; this test verifies the skip path when it is unset", datasets.EnvParityExternalPath)
	}

	var subT *testing.T
	t.Run("gate-off", func(s *testing.T) {
		subT = s
		datasets.External(s) // expected to call s.Skip() and abort this goroutine
		// Unreachable when skip works correctly.
	})

	if !subT.Skipped() {
		t.Fatalf("datasets.External did not skip when %s was unset", datasets.EnvParityExternalPath)
	}
}

// TestReadOnlyGatewayBlocksWrites verifies that a gateway built from a
// ReadOnly dataset rejects write operations. This is the negative test.
//
// We use a writable fixture copy and wrap it as ReadOnly to simulate the
// external-dataset scenario. This protects the actual fixture from corruption.
func TestReadOnlyGatewayBlocksWrites(t *testing.T) {
	t.Parallel()

	// Build a writable fixture, then declare it read-only for this test.
	writableDS := datasets.Fixture(t)

	readOnlyDS := datasets.Dataset{
		Name:     "fixture-readonly",
		Path:     writableDS.Path,
		ReadOnly: true,
	}

	gw := datasets.NewGateway(t, readOnlyDS)

	// Attempting to create an issue through the read-only gateway must fail.
	_, err := gw.CreateIssue(context.Background(), domain.CreateIssueInput{
		Title: "readonly-test-must-not-persist",
		Type:  "task",
	})
	if err == nil {
		t.Fatal("CreateIssue through a read-only gateway must return an error; got nil")
	}

	// The error must indicate that bd rejected the operation with a read-only
	// enforcement message.
	errStr := err.Error()
	if !strings.Contains(errStr, "not allowed in read-only mode") &&
		!strings.Contains(errStr, "read-only") &&
		!strings.Contains(errStr, "readonly") {
		t.Fatalf("expected read-only rejection error; got: %v", err)
	}
}

// TestReadOnlyBdHelperAllowsReadsBlocksWrites verifies that BdList succeeds
// while a direct --readonly create invocation fails.
func TestReadOnlyBdHelperAllowsReadsBlocksWrites(t *testing.T) {
	t.Parallel()

	writableDS := datasets.Fixture(t)

	readOnlyDS := datasets.Dataset{
		Name:     "fixture-readonly-helper",
		Path:     writableDS.Path,
		ReadOnly: true,
	}

	// BdList must succeed (read operation).
	out, err := datasets.BdList(t, readOnlyDS)
	if err != nil {
		t.Fatalf("BdList on read-only dataset failed unexpectedly: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("BdList returned empty output; expected JSON array")
	}

	// Direct --readonly create invocation must fail.
	cmd := exec.Command("bd", "--readonly", "create", "--title", "readonly-test-must-fail", "--type", "task")
	cmd.Dir = readOnlyDS.Path
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("bd --readonly create must fail; got nil error (write was not blocked)")
	}
}

// TestExternalMtimesUnchanged verifies that running the External dataset
// does not modify any file under .beads/. Skipped when BWB_PARITY_EXTERNAL_PATH
// is unset; otherwise targets that path.
func TestExternalMtimesUnchanged(t *testing.T) {
	t.Parallel()

	externalPath := os.Getenv(datasets.EnvParityExternalPath)
	if externalPath == "" {
		t.Skipf("%s not set; skipping mtime-change verification", datasets.EnvParityExternalPath)
	}

	beadsDir := filepath.Join(externalPath, ".beads")

	// Snapshot mtimes before exercising the dataset.
	before := snapshotMtimes(t, beadsDir)

	ds := datasets.External(t)
	gw := datasets.NewGateway(t, ds)

	if _, err := gw.ListIssues(context.Background(), domain.IssueListQuery{}); err != nil {
		t.Fatalf("ListIssues on External failed: %v", err)
	}

	if _, err := datasets.BdList(t, ds); err != nil {
		t.Fatalf("BdList on External failed: %v", err)
	}

	// Snapshot mtimes after.
	after := snapshotMtimes(t, beadsDir)

	for path, beforeMtime := range before {
		if afterMtime, ok := after[path]; !ok {
			t.Errorf("file disappeared after test: %q", path)
		} else if !afterMtime.Equal(beforeMtime) {
			t.Errorf("mtime changed for %q: before=%v after=%v", path, beforeMtime, afterMtime)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("new file created during test: %q", path)
		}
	}
}

// TestBdCountHelperFixture verifies BdCount returns parseable JSON against the
// fixture dataset.
func TestBdCountHelperFixture(t *testing.T) {
	t.Parallel()

	ds := datasets.Fixture(t)

	out, err := datasets.BdCount(t, ds, "--by-status")
	if err != nil {
		t.Fatalf("BdCount failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("BdCount returned empty output")
	}
	trimmed := strings.TrimSpace(string(out))
	if !strings.HasPrefix(trimmed, "{") {
		t.Fatalf("BdCount output does not look like JSON object: %q", trimmed[:clamp(len(trimmed), 60)])
	}
}

// TestBdReadyHelperFixture verifies BdReady returns a JSON array.
func TestBdReadyHelperFixture(t *testing.T) {
	t.Parallel()

	ds := datasets.Fixture(t)

	out, err := datasets.BdReady(t, ds)
	if err != nil {
		t.Fatalf("BdReady failed: %v", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if !strings.HasPrefix(trimmed, "[") {
		t.Fatalf("BdReady output does not look like JSON array: %q", trimmed[:clamp(len(trimmed), 60)])
	}
}

// TestBdBlockedHelperFixture verifies BdBlocked returns a JSON array.
func TestBdBlockedHelperFixture(t *testing.T) {
	t.Parallel()

	ds := datasets.Fixture(t)

	out, err := datasets.BdBlocked(t, ds)
	if err != nil {
		t.Fatalf("BdBlocked failed: %v", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if !strings.HasPrefix(trimmed, "[") {
		t.Fatalf("BdBlocked output does not look like JSON array: %q", trimmed[:clamp(len(trimmed), 60)])
	}
}

// TestBdSearchHelperFixture verifies BdSearch returns a JSON array when given
// a text query.
func TestBdSearchHelperFixture(t *testing.T) {
	t.Parallel()

	ds := datasets.Fixture(t)

	// bd search requires a text argument to produce JSON output.
	// "fixture" matches the seeded issue titles in the embedded fixture repo.
	out, err := datasets.BdSearch(t, ds, "fixture")
	if err != nil {
		// bd search may return exit 0 with empty JSON array when no results match.
		// Only fatal on unexpected error.
		t.Fatalf("BdSearch failed: %v", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if !strings.HasPrefix(trimmed, "[") {
		t.Fatalf("BdSearch output does not look like JSON array: %q", trimmed[:clamp(len(trimmed), 60)])
	}
}

// snapshotMtimes walks dir and records every file's mtime keyed by absolute path.
// The embeddeddolt/ subdirectory is excluded because Dolt updates its internal
// noms journal/manifest files as a side-effect of read queries (read-cache
// maintenance). These are storage-engine internals, not user-visible issue data.
// The --readonly flag prevents logical writes (create/update/close); it does not
// prevent Dolt from maintaining its own read-side state.
func snapshotMtimes(t *testing.T, dir string) map[string]time.Time {
	t.Helper()

	result := make(map[string]time.Time)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("snapshotMtimes: ReadDir(%q): %v", dir, err)
	}

	for _, entry := range entries {
		// Skip Dolt's embedded storage directory — it updates internal noms files
		// on every read query (journal/manifest maintenance). This is expected and
		// does not indicate that --readonly has been bypassed.
		if entry.IsDir() && entry.Name() == "embeddeddolt" {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("snapshotMtimes: Info(%q): %v", fullPath, err)
		}
		if entry.IsDir() {
			for sub, mtime := range snapshotMtimes(t, fullPath) {
				result[sub] = mtime
			}
			continue
		}
		result[fullPath] = info.ModTime()
	}

	return result
}

// clamp returns n clamped to [0, max].
func clamp(n, max int) int {
	if n > max {
		return max
	}
	return n
}
