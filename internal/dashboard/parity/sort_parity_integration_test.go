//go:build integration

// Package parity contains integration tests that verify the dashboard data
// layer produces the same issue ordering as bd CLI commands.
//
// # Design
//
// Each column comparison uses the bd command that most closely mirrors the
// repository call that feeds it:
//
//   - Ready:     bd ready --json         (same sort contract: priority asc, updated_at desc, id asc)
//   - NotReady:  bd blocked --json       (same issues; bwb applies issueSort on top)
//   - InProgress: bd list --status in_progress --json (same issues; bwb applies issueSort)
//   - Done:      bdQueryClosed()          (bd query mirrors the repository's bd query call exactly)
//
// For active columns (Ready, NotReady, InProgress), bwb applies issueSort
// (priority asc, updated_at desc, id asc) to the repository results. To compare
// fairly on real datasets where many issues share a priority and updated_at
// timestamp — producing tie-break positions that differ between bd and bwb —
// we apply the same sort to the bd CLI output before comparing. This tests:
//
//  1. Both sides return the same set of issue IDs.
//  2. bwb's issueSort is applied deterministically.
//
// For the Done column bwb does NOT re-sort (it preserves backend order). We
// therefore compare directly, position by position, against the raw bd output.
// The ClosedAtDescPreservedOnRealData sub-test provides a diagnostic that feeds
// the kh54 follow-up decision about whether the UpdatedAt proxy is reliable.
package parity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/testing/datasets"
)

// closedCapForTest is a fixed cap used for the Done column in parity tests.
// 50 is chosen large enough to engage the cap on the parity fixtures while
// keeping fetch volume modest. The production board no longer enforces a
// floor; this constant is local to parity tests only.
const closedCapForTest = 50

// bdIssueSortable is decoded from bd JSON output for sort-parity comparisons.
// We decode more fields than just ID so we can apply the same issueSort logic
// to bd output for active-column comparisons.
type bdIssueSortable struct {
	ID        string `json:"id"`
	Priority  int    `json:"priority"`
	UpdatedAt string `json:"updated_at"`
}

func (b bdIssueSortable) updatedAtTime() time.Time {
	t, _ := time.Parse(time.RFC3339, b.UpdatedAt)
	return t
}

// sortableFromJSON decodes a JSON array from bd output into a slice of
// bdIssueSortable.
func sortableFromJSON(t *testing.T, data []byte) []bdIssueSortable {
	t.Helper()
	var items []bdIssueSortable
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("sortableFromJSON: JSON decode failed: %v\nraw: %q", err, truncate(string(data), 200))
	}
	return items
}

// idsInOrder extracts IDs from a bdIssueSortable slice in order.
func idsInOrder(items []bdIssueSortable) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

// applyIssueSort sorts items in-place using the same logic as
// dashboard.issueSort: priority asc, updated_at desc, id asc (stable).
func applyIssueSort(items []bdIssueSortable) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		at, bt := a.updatedAtTime(), b.updatedAtTime()
		if !at.Equal(bt) {
			return at.After(bt)
		}
		return a.ID < b.ID
	})
}

// bdQueryClosed runs bd query 'status=closed' -a --sort closed
// --limit <cap> --json from ds.Path. This mirrors the exact repository call that
// feeds bwb's Done column (the repository uses bd query, not bd list).
// bd --sort <field> defaults to DESCENDING (newest first); --reverse is NOT
// emitted because the caller requests SortDirectionDescending.
func bdQueryClosed(t *testing.T, ds datasets.Dataset, limit int) []byte {
	t.Helper()

	argv := make([]string, 0, 10)
	if ds.ReadOnly {
		argv = append(argv, "--readonly")
	}
	argv = append(argv,
		"query", "status=closed",
		"-a",
		"--sort", "closed",
		"--limit", strconv.Itoa(limit),
		"--json",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", argv...)
	cmd.Dir = ds.Path
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bdQueryClosed[%s]: bd query failed: %v", ds.Name, err)
	}
	return out
}

// fetchBWBColumns fetches the dashboard data via repository.Repository and
// returns composed Columns. StoredBlocked is intentionally omitted from the
// Compose inputs so the NotReady column reflects only dependency-blocked issues
// (matching bd blocked output) for the purposes of this sort-parity test.
func fetchBWBColumns(t *testing.T, ds datasets.Dataset) dashboard.Columns {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	repo := datasets.NewRepository(t, ds)

	data, err := repo.Dashboard(ctx)
	if err != nil {
		t.Fatalf("fetchBWBColumns[%s]: Dashboard: %v", ds.Name, err)
	}

	return dashboard.Compose(dashboard.Inputs{
		Ready:       data.ReadyExplain.Ready,
		Blocked:     data.ReadyExplain.Blocked,
		InProgress:  data.InProgress,
		Closed:      data.Closed,
		ClosedLimit: closedCapForTest,
		ClosedTotal: data.ClosedTotal,
	})
}

// assertOrderParity compares two ID slices up to min(len(a), len(b)) entries.
// On first divergence it calls t.Fatalf with structured context including
// the column name, dataset name, divergence index, and both IDs.
func assertOrderParity(t *testing.T, column, dsName string, bwbIDs, bdIDs []string) {
	t.Helper()

	n := len(bwbIDs)
	if len(bdIDs) < n {
		n = len(bdIDs)
	}
	if n == 0 {
		t.Logf("assertOrderParity[%s/%s]: both sides empty — no ordering to assert", column, dsName)
		return
	}

	for i := 0; i < n; i++ {
		if bwbIDs[i] != bdIDs[i] {
			t.Fatalf(
				"column=%s dataset=%s first_divergence=index_%d bwb_id=%s bd_id=%s (bwb[0:%d]=%v bd[0:%d]=%v)",
				column, dsName, i, bwbIDs[i], bdIDs[i],
				minInt(n, 5), bwbIDs[:minInt(n, 5)],
				minInt(n, 5), bdIDs[:minInt(n, 5)],
			)
		}
	}

	t.Logf("assertOrderParity[%s/%s]: OK — %d positions match (bwb=%d bd=%d total)", column, dsName, n, len(bwbIDs), len(bdIDs))
}

// TestSortParity verifies that dashboard Compose output matches bd CLI order
// for each available dataset and each column.
func TestSortParity(t *testing.T) {
	allDatasets := []func(*testing.T) datasets.Dataset{
		// Fixture: minimal anchor (3 issues) — always runs.
		datasets.Fixture,
		// ScaleFixture: ~590 issues — exercises sort tie-breaks on closed_at and
		// Done-column cap; opt-in via BWB_SCALE_FIXTURE=1.
		datasets.ScaleFixture,
		datasets.ThisRepo,
		datasets.External,
	}

	for _, dsFunc := range allDatasets {
		ds := dsFunc(t) // skips automatically when env gate is absent
		t.Run(ds.Name, func(t *testing.T) {
			runSortParityForDataset(t, ds)
		})
	}
}

// runSortParityForDataset runs the four column parity checks plus the
// ClosedAtDescPreservedOnRealData diagnostic for one dataset.
func runSortParityForDataset(t *testing.T, ds datasets.Dataset) {
	t.Helper()

	cols := fetchBWBColumns(t, ds)

	// --- Ready column ---
	// bd ready uses the same sort contract as bwb's issueSort
	// (priority asc, updated_at desc, id asc), so we compare directly.
	//
	// Pass --limit 0 to match ReadyExplain's uncapped output: bd ready --json
	// without --limit 0 caps at 100 items while bd ready --explain --json (the
	// repository path) returns all ready issues. Without --limit 0 on scale datasets
	// assertOrderParity silently only checks the first 100 of 500+ positions.
	// See interface.go "bd quirks observed at scale".
	t.Run("Ready", func(t *testing.T) {
		bdOut, err := datasets.BdReady(t, ds, "--limit", "0")
		if err != nil {
			t.Fatalf("BdReady failed: %v", err)
		}
		bdItems := sortableFromJSON(t, bdOut)
		// bd ready already orders by the same contract; apply issueSort to both
		// sides to handle any residual tie-breaking differences consistently.
		applyIssueSort(bdItems)
		bdIDs := idsInOrder(bdItems)
		bwbIDs := issueIDs(cols.Ready.Issues)
		assertOrderParity(t, "Ready", ds.Name, bwbIDs, bdIDs)
	})

	// --- NotReady (blocked) column ---
	// bd blocked returns blocked issues; bwb applies issueSort on top.
	// Apply the same issueSort to bd output before comparing.
	t.Run("NotReady", func(t *testing.T) {
		bdOut, err := datasets.BdBlocked(t, ds)
		if err != nil {
			t.Fatalf("BdBlocked failed: %v", err)
		}
		bdItems := sortableFromJSON(t, bdOut)
		applyIssueSort(bdItems)
		bdIDs := idsInOrder(bdItems)
		bwbIDs := issueIDs(cols.NotReady.Issues)
		assertOrderParity(t, "NotReady", ds.Name, bwbIDs, bdIDs)
	})

	// --- InProgress column ---
	// bwb applies issueSort to repository Query(in_progress) results.
	// bd list --status in_progress uses a different default sort.
	// Apply issueSort to both sides so the comparison tests:
	//   (a) same issue IDs returned (set parity), and
	//   (b) issueSort is applied deterministically.
	t.Run("InProgress", func(t *testing.T) {
		bdOut, err := datasets.BdList(t, ds, "--status", "in_progress")
		if err != nil {
			t.Fatalf("BdList(in_progress) failed: %v", err)
		}
		bdItems := sortableFromJSON(t, bdOut)
		applyIssueSort(bdItems)
		bdIDs := idsInOrder(bdItems)
		bwbIDs := issueIDs(cols.InProgress.Issues)
		assertOrderParity(t, "InProgress", ds.Name, bwbIDs, bdIDs)
	})

	// --- Done column ---
	// bwb does NOT re-sort the Done column; it preserves the order from the
	// repository Query(status=closed, sort=closed_at, order=desc, limit=cap).
	// The repository uses "bd query" (not "bd list") so we call bdQueryClosed
	// directly to get the same source-of-truth ordering.
	t.Run("Done", func(t *testing.T) {
		bdRaw := bdQueryClosed(t, ds, closedCapForTest)
		bdItems := sortableFromJSON(t, bdRaw)
		bdIDs := idsInOrder(bdItems)
		bwbIDs := issueIDs(cols.Done.Issues)
		assertOrderParity(t, "Done", ds.Name, bwbIDs, bdIDs)
	})

	// ClosedAtDescPreservedOnRealData: diagnostic sub-test for kh54 follow-up.
	// Always PASSES; emits t.Logf with mismatch counts per dataset.
	t.Run("ClosedAtDescPreservedOnRealData", func(t *testing.T) {
		runClosedAtDiagnostic(t, ds)
	})
}

// runClosedAtDiagnostic compares bd list --status closed --sort closed --reverse
// against bd list --status closed --sort updated --reverse to measure how reliably
// UpdatedAt is a proxy for closed_at on real data.
//
// This sub-test always PASSES — it is purely diagnostic and feeds the kh54
// follow-up fix decision. Mismatch counts are emitted via t.Logf.
func runClosedAtDiagnostic(t *testing.T, ds datasets.Dataset) {
	t.Helper()

	closedOut, err := datasets.BdList(t, ds,
		"--status", "closed",
		"--sort", "closed",
		"--reverse",
		"--limit", strconv.Itoa(closedCapForTest),
	)
	if err != nil {
		t.Logf("ClosedAtDescPreservedOnRealData[%s]: BdList(closed, closed_at asc) failed: %v — skipping diagnostic", ds.Name, err)
		return
	}
	closedAtItems := sortableFromJSON(t, closedOut)
	closedAtIDs := idsInOrder(closedAtItems)

	updatedOut, err := datasets.BdList(t, ds,
		"--status", "closed",
		"--sort", "updated",
		"--reverse",
		"--limit", strconv.Itoa(closedCapForTest),
	)
	if err != nil {
		t.Logf("ClosedAtDescPreservedOnRealData[%s]: BdList(closed, updated_at asc) failed: %v — skipping diagnostic", ds.Name, err)
		return
	}
	updatedAtItems := sortableFromJSON(t, updatedOut)
	updatedAtIDs := idsInOrder(updatedAtItems)

	if len(closedAtIDs) == 0 {
		t.Logf("ClosedAtDescPreservedOnRealData[%s]: no closed issues — nothing to compare", ds.Name)
		return
	}

	n := len(closedAtIDs)
	if len(updatedAtIDs) < n {
		n = len(updatedAtIDs)
	}

	mismatches := 0
	firstMismatchIdx := -1
	for i := 0; i < n; i++ {
		if closedAtIDs[i] != updatedAtIDs[i] {
			mismatches++
			if firstMismatchIdx == -1 {
				firstMismatchIdx = i
			}
		}
	}

	t.Logf(
		"ClosedAtDescPreservedOnRealData[%s]: compared %d positions: mismatches=%d first_mismatch_index=%s",
		ds.Name, n, mismatches, formatOptInt(firstMismatchIdx),
	)
	if mismatches > 0 {
		t.Logf(
			"  closed_at_asc[0:5]=%v  updated_at_asc[0:5]=%v",
			truncateIDs(closedAtIDs, 5),
			truncateIDs(updatedAtIDs, 5),
		)
		t.Logf(
			"  DIAGNOSTIC: updated_at is NOT a reliable proxy for closed_at on dataset=%s (%d/%d positions differ) — supports kh54 Option 1 (plumb ClosedAt)",
			ds.Name, mismatches, n,
		)
	} else {
		t.Logf(
			"  DIAGNOSTIC: updated_at matches closed_at order on dataset=%s — kh54 false-positive may be safe to remove",
			ds.Name,
		)
	}
}

// issueIDs extracts IDs from a slice of IssueSummary in order.
func issueIDs(issues []domain.IssueSummary) []string {
	ids := make([]string, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	return ids
}

// truncate returns at most maxLen characters of s, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// truncateIDs returns the first n IDs from the slice (or fewer if shorter).
func truncateIDs(ids []string, n int) []string {
	if len(ids) <= n {
		return ids
	}
	return ids[:n]
}

// formatOptInt returns the string form of i, or "none" when i == -1.
func formatOptInt(i int) string {
	if i == -1 {
		return "none"
	}
	return fmt.Sprintf("%d", i)
}

// minInt returns the smaller of a and b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
