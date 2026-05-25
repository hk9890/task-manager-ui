package caching_test

// known_bad_repro_test.go pins fbea Success Criterion #3: a deterministic
// repro test using a fixture that models the known-bad cache scenario.
//
// The bug (Defect #1): caching.Dashboard never seeded c.memory. SaveNow
// persisted only the click-trail (issues the user had individually opened).
// On a subsequent Hydrate with matching bd-hash, the memory-recomputed
// dashboardCache returned a falsely "complete" tiny dashboard.
//
// Fix (candidate b, fbea.3): Hydrate now restores dashboardCache directly
// from the v2 header rather than recomputing it from the partial memory
// snapshot. Dashboard now also seeds memory so SaveNow persists the full set.
//
// Provenance: the original known-bad v1 fixture files are in
// testdata/known-bad-cache-fbea/session-d40f3356/ and were captured from a
// real user session. After the v2 schema bump (fbea.2) those v1 files trigger
// ErrSchemaMismatch → cold start; this test instead creates a v2-equivalent
// degenerate file programmatically to reproduce the pre-fix bug path.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/repository/caching"
	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

// knownBadHash is the bd_commit_hash recorded in the v1 known-bad fixture at
// testdata/known-bad-cache-fbea/session-d40f3356/repo.jsonl.manifest.json.
// Using the real recorded hash makes the injected vcStatusFunc credible as a
// reproduction of the actual production bug.
const knownBadHash = "0a7kepv5lr7a9l5eeoedad7gb3b2eh2e"

// TestKnownBadCacheRepro_V2Equivalent is the deterministic repro for fbea
// Defect #1.
//
// Setup:
//   - Create a v2 fixture file whose JSONL memory snapshot contains only 1 issue
//     (simulating the click-trail) but whose V2Header.DashboardCache contains a
//     full DashboardData with N > 1 issues.
//   - Construct caching.Repository with a backing stub that holds M > N issues.
//   - Inject a vcStatusFunc that returns knownBadHash (matching the manifest hash).
//
// Pre-fix behavior (main branch, before fbea.3):
//   - Hydrate loads the 1-issue memory, recomputes dashboardCache from that memory,
//     sets dashboardDirty=false. Dashboard() returns 1-issue result. → FAIL
//
// Post-fix behavior (after fbea.3):
//   - Hydrate reads dashboardCache from the v2 header directly (not from memory).
//   - dashboardDirty=false. Dashboard() returns the N-issue result from the header.
//   - If the header's DashboardCache were empty, backing would be called and return
//     the M-issue result. Either way, the user sees the full set. → PASS
func TestKnownBadCacheRepro_V2Equivalent(t *testing.T) {
	t.Parallel()

	const nInHeader = 7   // issues in the v2 header's DashboardCache
	const nInMemory = 1   // issues in the v2 JSONL memory snapshot (click-trail)
	const nInBacking = 10 // issues the backing has (superset)

	// Build the full DashboardData to embed in the v2 header.
	// This represents what a correctly-working SaveNow would have written.
	closedSummaries := make([]domain.IssueSummary, nInHeader)
	for i := range closedSummaries {
		closedSummaries[i] = domain.IssueSummary{
			ID:     idForRepro(i + 1),
			Title:  "closed issue from dashboard",
			Status: "closed",
		}
	}
	fullDashboard := repository.DashboardData{
		Closed:      closedSummaries,
		ClosedTotal: nInHeader,
	}

	// Build the memory snapshot (click-trail: only 1 issue).
	clickTrailMemory := memory.New()
	clickTrailMemory.Seed(memory.Issue{
		ID:     idForRepro(1),
		Title:  "single click-trail issue",
		Status: "closed",
	})

	// Write a v2 fixture file with the degenerate memory snapshot but full header.
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "repo.jsonl")

	snapshot := clickTrailMemory.Snapshot()
	if err := filestorage.SaveSnapshotV2WithHash(snapshot, fullDashboard, nil, fixturePath, knownBadHash); err != nil {
		t.Fatalf("create v2 fixture: %v", err)
	}

	// Verify the fixture was written with the correct schema.
	manifest, err := filestorage.LoadManifest(fixturePath + ".manifest.json")
	if err != nil {
		t.Fatalf("load fixture manifest: %v", err)
	}
	if manifest.SchemaVersion != filestorage.SchemaVersion {
		t.Fatalf("fixture schema_version=%d, want %d", manifest.SchemaVersion, filestorage.SchemaVersion)
	}

	// Build a backing repo with nInBacking issues.
	backingMem := memory.New()
	for i := 0; i < nInBacking; i++ {
		backingMem.Seed(memory.Issue{
			ID:     idForRepro(i + 1),
			Title:  "full backing issue",
			Status: "closed",
		})
	}

	// Inject a fake vcStatusFunc that returns the recorded hash verbatim.
	// Bug is reproducible iff vcStatusFunc(ctx) == manifest.BDCommitHash.
	fakeVCStatus := func(_ context.Context) (string, error) {
		return knownBadHash, nil
	}

	// Read the v1 known-bad fixture to confirm provenance (optional validation).
	// This verifies the testdata is present and the hash matches what we embedded.
	v1ManifestPath := filepath.Join("testdata", "known-bad-cache-fbea", "session-d40f3356", "repo.jsonl.manifest.json")
	if _, statErr := os.Stat(v1ManifestPath); statErr == nil {
		v1Manifest, manifestErr := filestorage.LoadManifest(v1ManifestPath)
		if manifestErr == nil && v1Manifest.BDCommitHash != knownBadHash {
			t.Errorf("knownBadHash constant %q does not match v1 fixture manifest %q; update the constant",
				knownBadHash, v1Manifest.BDCommitHash)
		}
	}

	// Construct CachingRepository.
	c := caching.New(backingMem, caching.WithVCStatusFunc(fakeVCStatus))

	writePath := filepath.Join(dir, "repo-write.jsonl")
	if err := c.Hydrate(context.Background(), fixturePath, writePath); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// Call Dashboard. After fbea.3, this returns the full set from v2 header.
	// Before fbea.3, this returns the 1-issue memory-recomputed result (the bug).
	dash, err := c.Dashboard(context.Background(), repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}

	// Assert: the dashboard must contain at least nInHeader issues total.
	// If dashboardDirty was false (v2 header served) → returns nInHeader.
	// If dashboardDirty was true (header empty, backing used) → returns nInBacking.
	// Either is correct. The failure case is: 1 issue (the click-trail only).
	totalIssues := len(dash.Closed) + len(dash.InProgress) + len(dash.Blocked) +
		len(dash.ReadyExplain.Ready) + len(dash.ReadyExplain.Blocked)
	if totalIssues < nInHeader {
		t.Errorf("Dashboard returned only %d issues total; expected >= %d (full set, not click-trail of %d) — fbea Defect #1 repro",
			totalIssues, nInHeader, nInMemory)
	}
}

func idForRepro(i int) string {
	return "repro-" + string(rune('a'+i-1))
}
