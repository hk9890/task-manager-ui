//go:build integration

package parity_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	beadsgateway "github.com/hk9890/beads-workbench/internal/gateway/beads"
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

// runDashboardFetch exercises the same 4 parallel gateway calls that
// internal/mode/board/model.go startReload dispatches, and runs
// dashboard.Compose with the results. It is the core of the parity test:
// if the production fetch path ever drifts, this helper captures it.
//
// closedLimit mirrors model.closedLimit(); 50 is the default safe floor used
// by the board model before the first WindowSizeMsg.
func runDashboardFetch(t *testing.T, gw beadsgateway.BeadsGateway, closedLimit int) dashboard.Columns {
	t.Helper()

	if closedLimit <= 0 {
		closedLimit = 50
	}

	ctx := context.Background()

	// Call 1: ReadyExplain (gives Ready + Blocked groups).
	readyResult, readyErr := gw.ReadyExplain(ctx, domain.ReadyExplainOptions{Limit: 0})
	if readyErr != nil {
		t.Fatalf("runDashboardFetch: ReadyExplain failed: %v", readyErr)
	}

	// Call 2: Query in_progress.
	inProgress, ipErr := gw.Query(ctx, "status=in_progress", domain.QueryOptions{Limit: 0})
	if ipErr != nil {
		t.Fatalf("runDashboardFetch: Query(in_progress) failed: %v", ipErr)
	}

	// Call 3: Query closed, sorted by closed_at desc, capped at closedLimit.
	closed, clErr := gw.Query(ctx, "status=closed", domain.QueryOptions{
		IncludeClosed: true,
		SortBy:        domain.SortFieldClosedAt,
		SortOrder:     domain.SortDirectionDescending,
		Limit:         closedLimit,
	})
	if clErr != nil {
		t.Fatalf("runDashboardFetch: Query(closed) failed: %v", clErr)
	}

	// Call 4: CountIssues(status=closed) — the real DB population count.
	countResult, countErr := gw.CountIssues(ctx, domain.IssueCountQuery{
		Statuses: []string{"closed"},
	})
	if countErr != nil {
		t.Fatalf("runDashboardFetch: CountIssues(closed) failed: %v", countErr)
	}

	return dashboard.Compose(dashboard.Inputs{
		Ready:       readyResult.Ready,
		Blocked:     readyResult.Blocked,
		InProgress:  inProgress,
		Closed:      closed,
		ClosedLimit: closedLimit,
		ClosedTotal: countResult.Total,
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

// TestCountParityDashboardVsBdCount is a permanent integration test that catches
// any drift between the dashboard column totals reported by the bwb data path and
// the source-of-truth counts from bd count --by-status.
//
// It runs the same 4 gateway calls that the board model fires on startup, feeds
// the result through dashboard.Compose, and compares the resulting column Totals
// against bd count and bd ready output per dataset.
func TestCountParityDashboardVsBdCount(t *testing.T) {
	allDatasets := []struct {
		name string
		get  func(t *testing.T) datasets.Dataset
	}{
		{"fixture", datasets.Fixture},
		{"this-repo", datasets.ThisRepo},
		{"external", datasets.External},
	}

	for _, entry := range allDatasets {
		entry := entry // capture range variable
		t.Run(entry.name, func(t *testing.T) {
			// This will skip if the env gate is off (ThisRepo, External).
			ds := entry.get(t)

			gw := datasets.NewGateway(t, ds)
			cols := runDashboardFetch(t, gw, 50)

			// Source-of-truth: bd count --by-status --json
			countRaw, err := datasets.BdCount(t, ds, "--by-status")
			if err != nil {
				t.Fatalf("BdCount --by-status failed on dataset %q: %v", ds.Name, err)
			}
			statusCounts := parseBdCountByStatus(t, countRaw)

			// Source-of-truth for Ready: bd ready --json | len
			readyRaw, err := datasets.BdReady(t, ds)
			if err != nil {
				t.Fatalf("BdReady failed on dataset %q: %v", ds.Name, err)
			}
			bdReadyCount := parseBdReadyCount(t, readyRaw)

			// Source-of-truth for NotReady (blocked): bd blocked --json | len.
			// "blocked" is a readiness state, not a bd status field, so it does
			// not appear in bd count --by-status output. bd blocked --json is the
			// authoritative count.
			blockedRaw, err := datasets.BdBlocked(t, ds)
			if err != nil {
				t.Fatalf("BdBlocked failed on dataset %q: %v", ds.Name, err)
			}
			bdBlockedCount := parseBdBlockedCount(t, blockedRaw)

			// --- NotReady (blocked) ---
			t.Run("NotReadyTotalMatchesBdBlocked", func(t *testing.T) {
				bwbTotal := cols.NotReady.Total
				if bwbTotal != bdBlockedCount {
					delta := bwbTotal - bdBlockedCount
					t.Errorf("column=NotReady dataset=%s bwb_total=%d bd_count=%d delta=%d",
						ds.Name, bwbTotal, bdBlockedCount, delta)
				} else {
					t.Logf("column=NotReady dataset=%s bwb_total=%d bd_count=%d OK",
						ds.Name, bwbTotal, bdBlockedCount)
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
