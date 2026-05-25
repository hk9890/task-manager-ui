//go:build integration

package parity_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/testing/datasets"
)

// bdCountByStatusResult is the JSON shape returned by "bd count --by-status --json".
type bdCountByStatusResult struct {
	Groups []struct {
		Count int    `json:"count"`
		Group string `json:"group"`
	} `json:"groups"`
	Total int `json:"total"`
}

// bdReadyIssue is a minimal JSON element from "bd ready --json".
type bdReadyIssue struct {
	ID string `json:"id"`
}

// bdBlockedIssue is a minimal JSON element from "bd blocked --json".
type bdBlockedIssue struct {
	ID string `json:"id"`
}

// runDashboardFetch calls repo.Dashboard and composes the result into
// dashboard.Columns. It is the core of the parity test: if the production
// fetch path ever drifts, this helper captures it.
//
// closedLimit is passed to dashboard.Compose as the cap sent to bd; it is used
// to determine whether the visible row list is truncated. The lean Repository
// uses defaultLeanClosedLimit (50) internally, so closedLimit should be ≤50 to
// match the actual data returned.
func runDashboardFetch(t *testing.T, repo repository.Repository, closedLimit int) dashboard.Columns {
	t.Helper()

	if closedLimit <= 0 {
		closedLimit = 50
	}

	ctx := context.Background()

	data, err := repo.Dashboard(ctx, repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("runDashboardFetch: Dashboard failed: %v", err)
	}

	return dashboard.Compose(dashboard.Inputs{
		Ready:         data.ReadyExplain.Ready,
		Blocked:       data.ReadyExplain.Blocked,
		StoredBlocked: data.Blocked,
		InProgress:    data.InProgress,
		Closed:        data.Closed,
		ClosedLimit:   closedLimit,
		ClosedTotal:   data.ClosedTotal,
	})
}

// parseBdCountByStatus decodes the JSON output of "bd count --by-status --json"
// and returns a map of status group name → count.
func parseBdCountByStatus(t *testing.T, raw []byte) map[string]int {
	t.Helper()

	var result bdCountByStatusResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("parseBdCountByStatus: JSON unmarshal failed: %v\nraw: %s", err, raw)
	}

	out := make(map[string]int, len(result.Groups))
	for _, g := range result.Groups {
		out[g.Group] = g.Count
	}
	return out
}

// parseBdReadyCount decodes the JSON output of "bd ready --json" and returns the
// number of ready issues (len of the array).
func parseBdReadyCount(t *testing.T, raw []byte) int {
	t.Helper()

	var issues []bdReadyIssue
	if err := json.Unmarshal(raw, &issues); err != nil {
		t.Fatalf("parseBdReadyCount: JSON unmarshal failed: %v\nraw: %s", err, raw)
	}
	return len(issues)
}

// parseBdBlockedCount decodes the JSON output of "bd blocked --json" and returns
// the number of blocked issues (len of the array).
func parseBdBlockedCount(t *testing.T, raw []byte) int {
	t.Helper()

	var issues []bdBlockedIssue
	if err := json.Unmarshal(raw, &issues); err != nil {
		t.Fatalf("parseBdBlockedCount: JSON unmarshal failed: %v\nraw: %s", err, raw)
	}
	return len(issues)
}

// parseBdBlockedOverlap counts the IDs that appear in both rawA and rawB.
// It parses both as bdBlockedIssue arrays and returns the size of the intersection.
// Used to compute the union size = |A| + |B| - |A∩B|.
func parseBdBlockedOverlap(t *testing.T, rawA, rawB []byte) int {
	t.Helper()

	var a []bdBlockedIssue
	if err := json.Unmarshal(rawA, &a); err != nil {
		t.Fatalf("parseBdBlockedOverlap: unmarshal rawA: %v", err)
	}
	var b []bdBlockedIssue
	if err := json.Unmarshal(rawB, &b); err != nil {
		t.Fatalf("parseBdBlockedOverlap: unmarshal rawB: %v", err)
	}

	setA := make(map[string]struct{}, len(a))
	for _, item := range a {
		setA[item.ID] = struct{}{}
	}
	count := 0
	for _, item := range b {
		if _, ok := setA[item.ID]; ok {
			count++
		}
	}
	return count
}

// TestCountParityDashboardVsBdCount is a permanent integration test that catches
// any drift between the dashboard column totals reported by the bwb data path and
// the source-of-truth counts from bd count --by-status.
//
// It runs the same 4 repository calls that the board model fires on startup, feeds
// the result through dashboard.Compose, and compares the resulting column Totals
// against bd count and bd ready output per dataset.
func TestCountParityDashboardVsBdCount(t *testing.T) {
	allDatasets := []struct {
		name string
		get  func(t *testing.T) datasets.Dataset
	}{
		// fixture: minimal anchor (3 issues) — exact-ID baseline; always runs.
		{"fixture", datasets.Fixture},
		// scale-fixture: ~590 issues — exercises Done-column cap (>50 closed);
		// opt-in via BWB_SCALE_FIXTURE=1 (seeding takes several minutes).
		{"scale-fixture", datasets.ScaleFixture},
		{"this-repo", datasets.ThisRepo},
		{"external", datasets.External},
	}

	for _, entry := range allDatasets {
		entry := entry // capture range variable
		t.Run(entry.name, func(t *testing.T) {
			// This will skip if the env gate is off (ThisRepo, External).
			ds := entry.get(t)

			repo := datasets.NewRepository(t, ds)
			cols := runDashboardFetch(t, repo, 50)

			// Source-of-truth: bd count --by-status --json
			countRaw, err := datasets.BdCount(t, ds, "--by-status")
			if err != nil {
				t.Fatalf("BdCount --by-status failed on dataset %q: %v", ds.Name, err)
			}
			statusCounts := parseBdCountByStatus(t, countRaw)

			// Source-of-truth for Ready: bd ready --json | len
			//
			// Pass --limit 0 to match ReadyExplain's uncapped output.
			// bd ready --explain --json (used by ReadyExplain) bypasses the default
			// 100-item limit; bd ready --json without --limit 0 caps at 100. On
			// scale datasets with >100 ready issues the counts diverge by ~400
			// without this flag. See interface.go "bd quirks observed at scale".
			readyRaw, err := datasets.BdReady(t, ds, "--limit", "0")
			if err != nil {
				t.Fatalf("BdReady failed on dataset %q: %v", ds.Name, err)
			}
			bdReadyCount := parseBdReadyCount(t, readyRaw)

			// Source-of-truth for NotReady: deduplicated union of dep-blocked
			// (bd blocked) and stored-blocked (bd list --status blocked).
			// bwb's NotReady column contains both populations; issues appearing
			// in both are counted once. "bd blocked" returns dep-blocked issues
			// regardless of stored status. "bd list --status blocked" returns issues
			// whose stored status is blocked, regardless of dep graph.
			blockedRaw, err := datasets.BdBlocked(t, ds)
			if err != nil {
				t.Fatalf("BdBlocked failed on dataset %q: %v", ds.Name, err)
			}
			bdDepBlockedCount := parseBdBlockedCount(t, blockedRaw)

			storedBlockedRaw, err := datasets.BdList(t, ds, "--status", "blocked")
			if err != nil {
				t.Fatalf("BdList --status blocked failed on dataset %q: %v", ds.Name, err)
			}
			bdStoredBlockedCount := parseBdBlockedCount(t, storedBlockedRaw)
			bdNotReadyCount := bdDepBlockedCount + bdStoredBlockedCount - parseBdBlockedOverlap(t, blockedRaw, storedBlockedRaw)

			// --- NotReady (blocked) ---
			t.Run("NotReadyTotalMatchesBdBlocked", func(t *testing.T) {
				bwbTotal := cols.NotReady.Total
				if bwbTotal != bdNotReadyCount {
					delta := bwbTotal - bdNotReadyCount
					t.Errorf("column=NotReady dataset=%s bwb_total=%d bd_notready=%d (dep=%d stored=%d) delta=%d",
						ds.Name, bwbTotal, bdNotReadyCount, bdDepBlockedCount, bdStoredBlockedCount, delta)
				} else {
					t.Logf("column=NotReady dataset=%s bwb_total=%d bd_notready=%d (dep=%d stored=%d) OK",
						ds.Name, bwbTotal, bdNotReadyCount, bdDepBlockedCount, bdStoredBlockedCount)
				}
			})

			// --- Ready ---
			t.Run("ReadyTotalMatchesBdReady", func(t *testing.T) {
				bwbTotal := cols.Ready.Total
				if bwbTotal != bdReadyCount {
					delta := bwbTotal - bdReadyCount
					t.Errorf("column=Ready dataset=%s bwb_total=%d bd_count=%d delta=%d",
						ds.Name, bwbTotal, bdReadyCount, delta)
				} else {
					t.Logf("column=Ready dataset=%s bwb_total=%d bd_count=%d OK",
						ds.Name, bwbTotal, bdReadyCount)
				}
			})

			// --- InProgress ---
			t.Run("InProgressTotalMatchesBdCount", func(t *testing.T) {
				bdInProgress := statusCounts["in_progress"] // 0 if absent
				bwbTotal := cols.InProgress.Total
				if bwbTotal != bdInProgress {
					delta := bwbTotal - bdInProgress
					t.Errorf("column=InProgress dataset=%s bwb_total=%d bd_count=%d delta=%d",
						ds.Name, bwbTotal, bdInProgress, delta)
				} else {
					t.Logf("column=InProgress dataset=%s bwb_total=%d bd_count=%d OK",
						ds.Name, bwbTotal, bdInProgress)
				}
			})

			// --- Done (closed) — THE assertion that catches the ssom-class bug ---
			t.Run("DoneTotalMatchesBdCount", func(t *testing.T) {
				bdClosed := statusCounts["closed"] // 0 if absent
				bwbTotal := cols.Done.Total
				if bwbTotal != bdClosed {
					delta := bwbTotal - bdClosed
					t.Errorf("column=Done dataset=%s bwb_total=%d bd_count=%d delta=%d",
						ds.Name, bwbTotal, bdClosed, delta)
				} else {
					t.Logf("column=Done dataset=%s bwb_total=%d bd_count=%d OK",
						ds.Name, bwbTotal, bdClosed)
				}
			})

			// Summary log to make pass/fail visible at the dataset level.
			t.Logf("dataset=%s NotReady=%d Ready=%d InProgress=%d Done=%d",
				ds.Name,
				cols.NotReady.Total,
				cols.Ready.Total,
				cols.InProgress.Total,
				cols.Done.Total,
			)
		})
	}
}
