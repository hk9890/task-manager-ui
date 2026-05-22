//go:build integration

package parity_test

// scale_parity_integration_test.go — parity assertions that require the scale
// fixture's edge-case properties and cannot pass on the minimal anchor (3 issues).
//
// All tests in this file are gated behind BWB_SCALE_FIXTURE=1 via
// datasets.ScaleFixture, which skips automatically when the env var is unset.
//
// Edge cases exercised:
//   - Done column cap engagement: >50 closed issues forces ClosedTotal > ClosedLimit,
//     which must produce TotalIsExact=false and the "N of M" badge signal. The 3-issue
//     minimal anchor has only 1 closed issue, so it cannot trigger this path.
//   - Done.Total vs bd count parity under cap: when Done is capped at 50 rows,
//     Done.Total must still match bd count --by-status closed (not 50).

import (
	"encoding/json"
	"testing"

	"github.com/hk9890/beads-workbench/internal/testing/datasets"
)

// TestScaleParity_DoneColumnCapEngagement exercises the ssom regression class:
// when >50 issues are closed, the Done column must report TotalIsExact=false
// (the signal bwb uses to render the "N of M" badge) and Done.Total must equal
// the real DB count, not the capped row count.
//
// This test requires the scale fixture (>50 closed issues). The 3-issue minimal
// anchor cannot trigger the cap path, so this test is scale-only.
//
// regression class: ssom (Done-column cap badge; bwb reported rows ≤ 50 but
// the real closed count is 75).
func TestScaleParity_DoneColumnCapEngagement(t *testing.T) {
	// datasets.ScaleFixture skips automatically when BWB_SCALE_FIXTURE != 1.
	ds := datasets.ScaleFixture(t)

	gw := datasets.NewGateway(t, ds)
	cols := runDashboardFetch(t, gw, closedCapForTest)

	// Source-of-truth: bd count --by-status --json
	countRaw, err := datasets.BdCount(t, ds, "--by-status")
	if err != nil {
		t.Fatalf("BdCount --by-status failed on %q: %v", ds.Name, err)
	}
	statusCounts := parseBdCountByStatus(t, countRaw)
	bdClosed := statusCounts["closed"]

	// Precondition: the scale fixture must have >50 closed issues to trigger
	// the cap path. Without this the test is vacuous.
	if bdClosed <= closedCapForTest {
		t.Skipf("scale fixture has only %d closed issues (need >%d to exercise cap path)", bdClosed, closedCapForTest)
	}

	// Done.Total must equal the real DB count (not the cap).
	t.Run("DoneTotalEqualsRealClosedCount", func(t *testing.T) {
		if cols.Done.Total != bdClosed {
			t.Errorf(
				"ssom: Done.Total=%d; want %d (real bd count); delta=%d — cap may be leaking into Total",
				cols.Done.Total, bdClosed, cols.Done.Total-bdClosed,
			)
		} else {
			t.Logf("ssom cap parity OK: Done.Total=%d == bdClosed=%d", cols.Done.Total, bdClosed)
		}
	})

	// Done.TotalIsExact must be false when capped ("N of M" badge signal).
	t.Run("DoneTotalIsExactFalseWhenCapped", func(t *testing.T) {
		if cols.Done.TotalIsExact {
			t.Errorf(
				"ssom: Done.TotalIsExact=true when %d closed > cap %d; expected false (N of M badge should show)",
				bdClosed, closedCapForTest,
			)
		} else {
			t.Logf("ssom badge signal OK: TotalIsExact=false for %d closed > cap %d", bdClosed, closedCapForTest)
		}
	})

	// Done.Issues row count must not exceed the cap.
	t.Run("DoneRowsRespectCap", func(t *testing.T) {
		if len(cols.Done.Issues) > closedCapForTest {
			t.Errorf(
				"ssom: Done.Issues len=%d; want <=%d (cap must be respected)",
				len(cols.Done.Issues), closedCapForTest,
			)
		}
	})

	t.Logf("dataset=%s bdClosed=%d Done.Total=%d Done.TotalIsExact=%v Done.Issues=%d",
		ds.Name, bdClosed, cols.Done.Total, cols.Done.TotalIsExact, len(cols.Done.Issues))
}

// TestScaleParity_CapEngagement_VsBdCount verifies that the Done.Total count
// reported by the bwb data path under cap is identical to the source-of-truth
// count from `bd count --by-status` for the scale fixture.
//
// This is a strict count parity assertion scoped to the scale fixture because:
//   - The minimal anchor never exceeds the cap (only 1 closed issue), so the
//     cap path cannot be exercised.
//   - The scale fixture has 75+ closed issues, ensuring the cap fires on every run.
//
// regression class: ssom
func TestScaleParity_CapEngagement_VsBdCount(t *testing.T) {
	ds := datasets.ScaleFixture(t) // skips if BWB_SCALE_FIXTURE != 1

	gw := datasets.NewGateway(t, ds)
	cols := runDashboardFetch(t, gw, closedCapForTest)

	countRaw, err := datasets.BdCount(t, ds, "--by-status")
	if err != nil {
		t.Fatalf("BdCount failed: %v", err)
	}

	var result struct {
		Groups []struct {
			Count int    `json:"count"`
			Group string `json:"group"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(countRaw, &result); err != nil {
		t.Fatalf("unmarshal bd count: %v", err)
	}

	var bdClosed int
	for _, g := range result.Groups {
		if g.Group == "closed" {
			bdClosed = g.Count
			break
		}
	}

	if bdClosed <= closedCapForTest {
		t.Skipf("scale fixture has %d closed (need >%d); cannot exercise cap path", bdClosed, closedCapForTest)
	}

	if cols.Done.Total != bdClosed {
		t.Errorf(
			"ssom cap parity: Done.Total=%d; bd count=%d; delta=%d",
			cols.Done.Total, bdClosed, cols.Done.Total-bdClosed,
		)
	}
}
