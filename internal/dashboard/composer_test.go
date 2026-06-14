package dashboard

import (
	"testing"
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// helpers

func makeSummary(id string, priority int, updatedAt time.Time) domain.IssueSummary {
	return domain.IssueSummary{
		ID:        id,
		Priority:  priority,
		UpdatedAt: updatedAt,
	}
}

func makeBlocked(id string, priority int, updatedAt time.Time) domain.BlockedIssueView {
	return domain.BlockedIssueView{
		Issue: makeSummary(id, priority, updatedAt),
	}
}

var (
	t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = t0.Add(1 * time.Hour)
	t2 = t0.Add(2 * time.Hour)
	t3 = t0.Add(3 * time.Hour)
)

// makeLarge returns n IssueSummary values with the same priority and UpdatedAt.
func makeLarge(n int) []domain.IssueSummary {
	out := make([]domain.IssueSummary, n)
	for i := range out {
		out[i] = makeSummary("id", 1, t0)
	}
	return out
}

// makeClosedLarge returns n IssueSummary values with strictly descending UpdatedAt.
func makeClosedLarge(n int) []domain.IssueSummary {
	base := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]domain.IssueSummary, n)
	for i := range out {
		// newest first (descending): index 0 is the most recent
		out[i] = makeSummary("id", 1, base.Add(-time.Duration(i)*time.Second))
	}
	return out
}

func makeLargeBlocked(n int) []domain.BlockedIssueView {
	out := make([]domain.BlockedIssueView, n)
	for i := range out {
		out[i] = makeBlocked("id", 1, t0)
	}
	return out
}

// ---- table-driven tests ----

func TestCompose(t *testing.T) {
	t.Parallel()

	type wantWarning struct {
		group     string
		threshold int
	}

	tests := []struct {
		name string
		in   Inputs

		// column expectations
		wantNotReadyLen   int
		wantReadyLen      int
		wantInProgressLen int
		wantDoneLen       int
		// wantDoneTotal overrides the Done.Total assertion when non-zero.
		// Use when ClosedTotal causes Done.Total to diverge from len(Done.Issues).
		wantDoneTotal int

		wantDoneTotalIsExact bool

		// warning expectations
		wantWarnings []wantWarning

		// optional ordered ID checks (empty = skip)
		wantNotReadyIDs   []string
		wantReadyIDs      []string
		wantInProgressIDs []string
		wantDoneIDs       []string
	}{
		{
			name:                 "all empty",
			in:                   Inputs{},
			wantNotReadyLen:      0,
			wantReadyLen:         0,
			wantInProgressLen:    0,
			wantDoneLen:          0,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "single item per group",
			in: Inputs{
				Ready:       []domain.IssueSummary{makeSummary("r1", 2, t0)},
				Blocked:     []domain.BlockedIssueView{makeBlocked("b1", 3, t0)},
				InProgress:  []domain.IssueSummary{makeSummary("p1", 1, t0)},
				Closed:      []domain.IssueSummary{makeSummary("c1", 1, t0)},
				ClosedLimit: 10,
			},
			wantNotReadyLen:      1,
			wantReadyLen:         1,
			wantInProgressLen:    1,
			wantDoneLen:          1,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "mixed items, no warnings",
			in: Inputs{
				Ready: []domain.IssueSummary{
					makeSummary("r2", 2, t1),
					makeSummary("r1", 1, t0),
				},
				Blocked: []domain.BlockedIssueView{
					makeBlocked("b1", 1, t2),
				},
				InProgress: []domain.IssueSummary{
					makeSummary("p1", 3, t3),
					makeSummary("p2", 1, t1),
				},
				Closed: []domain.IssueSummary{
					makeSummary("c1", 1, t3), // newest first (correct backend order)
					makeSummary("c2", 1, t1),
				},
				ClosedLimit: 10,
			},
			wantNotReadyLen:      1,
			wantReadyLen:         2,
			wantInProgressLen:    2,
			wantDoneLen:          2,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
			// ready: r1 (priority 1) before r2 (priority 2)
			wantReadyIDs: []string{"r1", "r2"},
			// inProgress: p2 (priority 1) before p1 (priority 3)
			wantInProgressIDs: []string{"p2", "p1"},
			// done: preserves backend order
			wantDoneIDs: []string{"c1", "c2"},
		},

		// ---- cardinality warning tests ----
		{
			name: "cardinality warning fires at >500 for Ready",
			in: Inputs{
				Ready:       makeLarge(501),
				ClosedLimit: 10,
			},
			wantReadyLen:         501,
			wantDoneTotalIsExact: true,
			wantWarnings:         []wantWarning{{"Ready", 500}},
		},
		{
			name: "cardinality warning does NOT fire at exactly 500 for Ready",
			in: Inputs{
				Ready:       makeLarge(500),
				ClosedLimit: 10,
			},
			wantReadyLen:         500,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "cardinality warning does NOT fire at 499 for Ready",
			in: Inputs{
				Ready:       makeLarge(499),
				ClosedLimit: 10,
			},
			wantReadyLen:         499,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "cardinality warning fires at >500 for Blocked",
			in: Inputs{
				Blocked:     makeLargeBlocked(501),
				ClosedLimit: 10,
			},
			wantNotReadyLen:      501,
			wantDoneTotalIsExact: true,
			wantWarnings:         []wantWarning{{"Blocked", 500}},
		},
		{
			name: "cardinality warning does NOT fire at exactly 500 for Blocked",
			in: Inputs{
				Blocked:     makeLargeBlocked(500),
				ClosedLimit: 10,
			},
			wantNotReadyLen:      500,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "cardinality warning fires at >500 for InProgress",
			in: Inputs{
				InProgress:  makeLarge(501),
				ClosedLimit: 10,
			},
			wantInProgressLen:    501,
			wantDoneTotalIsExact: true,
			wantWarnings:         []wantWarning{{"InProgress", 500}},
		},
		{
			name: "cardinality warning does NOT fire at exactly 500 for InProgress",
			in: Inputs{
				InProgress:  makeLarge(500),
				ClosedLimit: 10,
			},
			wantInProgressLen:    500,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "cardinality warnings for all three active groups simultaneously",
			in: Inputs{
				Ready:       makeLarge(501),
				Blocked:     makeLargeBlocked(502),
				InProgress:  makeLarge(503),
				ClosedLimit: 10,
			},
			wantReadyLen:         501,
			wantNotReadyLen:      502,
			wantInProgressLen:    503,
			wantDoneTotalIsExact: true,
			wantWarnings: []wantWarning{
				{"Ready", 500},
				{"Blocked", 500},
				{"InProgress", 500},
			},
		},

		// ---- Done "N of M" truncation indicator tests ----
		{
			name: "Done TotalIsExact=false when items == ClosedLimit",
			in: Inputs{
				Closed:      makeClosedLarge(5),
				ClosedLimit: 5,
			},
			wantDoneLen:          5,
			wantDoneTotalIsExact: false,
			wantWarnings:         nil,
		},
		{
			name: "Done TotalIsExact=true when items < ClosedLimit",
			in: Inputs{
				Closed:      makeClosedLarge(4),
				ClosedLimit: 5,
			},
			wantDoneLen:          4,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "Done TotalIsExact=true when ClosedLimit is 0 (unset)",
			in: Inputs{
				Closed:      makeClosedLarge(5),
				ClosedLimit: 0,
			},
			wantDoneLen:          5,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "Done TotalIsExact=false when items > ClosedLimit",
			in: Inputs{
				Closed:      makeClosedLarge(10),
				ClosedLimit: 5,
			},
			wantDoneLen:          10,
			wantDoneTotalIsExact: false,
			wantWarnings:         nil,
		},

		// ---- stable sort ordering when priorities tie ----
		{
			name: "Ready sort: priority asc, then UpdatedAt desc, then ID asc",
			in: Inputs{
				Ready: []domain.IssueSummary{
					makeSummary("b", 1, t1), // pri=1, newer
					makeSummary("a", 1, t2), // pri=1, newest
					makeSummary("c", 2, t3), // pri=2 (lower priority)
					makeSummary("d", 1, t1), // pri=1, same time as "b" → id "d" > "b"
				},
				ClosedLimit: 10,
			},
			wantReadyLen:         4,
			wantDoneTotalIsExact: true,
			// sort: pri=1 first → among pri=1: t2>t1 so "a" first, then t1 tie → "b"<"d", then pri=2 "c"
			wantReadyIDs: []string{"a", "b", "d", "c"},
		},
		{
			name: "NotReady sort: same rules as Ready, mapped from BlockedIssueView",
			in: Inputs{
				Blocked: []domain.BlockedIssueView{
					makeBlocked("z", 2, t0),
					makeBlocked("m", 1, t1),
					makeBlocked("a", 1, t1),
				},
				ClosedLimit: 10,
			},
			wantNotReadyLen:      3,
			wantDoneTotalIsExact: true,
			// pri=1: t1>t0, "a"<"m" by ID; then pri=2 "z"
			wantNotReadyIDs: []string{"a", "m", "z"},
		},
		{
			name: "InProgress sort: same rules",
			in: Inputs{
				InProgress: []domain.IssueSummary{
					makeSummary("x", 3, t3),
					makeSummary("y", 1, t0),
					makeSummary("z", 1, t1),
				},
				ClosedLimit: 10,
			},
			wantInProgressLen:    3,
			wantDoneTotalIsExact: true,
			// pri=1: z (t1) > y (t0); then pri=3: x
			wantInProgressIDs: []string{"z", "y", "x"},
		},

		// ---- Done column: no sort warning emitted (check removed in kh54) ----
		// The UpdatedAt-proxy defensive check was removed because it produced
		// false-positives on real data (bulk-closed issues share UpdatedAt).
		// Backend sort ordering is now verified by the parity test (izds) against
		// real bd data rather than a runtime proxy.
		{
			name: "Done: no warning for empty Closed slice",
			in: Inputs{
				ClosedLimit: 10,
			},
			wantDoneLen:          0,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "Done: no warning for single-item Closed slice",
			in: Inputs{
				Closed:      []domain.IssueSummary{makeSummary("c1", 1, t0)},
				ClosedLimit: 10,
			},
			wantDoneLen:          1,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "Done: no warning when first two items have equal UpdatedAt (was false-positive before kh54)",
			in: Inputs{
				Closed: []domain.IssueSummary{
					makeSummary("a", 1, t1),
					makeSummary("b", 1, t1), // equal UpdatedAt — previously triggered false-positive warning
				},
				ClosedLimit: 10,
			},
			wantDoneLen:          2,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
			wantDoneIDs:          []string{"a", "b"},
		},
		{
			name: "Done: no warning when Closed is in reverse-UpdatedAt order (ascending)",
			in: Inputs{
				Closed: []domain.IssueSummary{
					makeSummary("old", 1, t0), // older first — UpdatedAt proxy would have fired warning
					makeSummary("new", 1, t1),
				},
				ClosedLimit: 10,
			},
			wantDoneLen:          2,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
			wantDoneIDs:          []string{"old", "new"},
		},

		// ---- ClosedTotal: real DB count overrides len(Closed) as Done.Total ----
		// These cases verify the fix for ssom (capped slice total bug).
		{
			name: "ClosedTotal>len(Closed): Done.Total uses ClosedTotal, TotalIsExact=false",
			in: Inputs{
				Closed:      makeClosedLarge(50),
				ClosedLimit: 50,
				ClosedTotal: 452,
			},
			wantDoneLen:          50,  // len(Issues) = 50 (capped)
			wantDoneTotal:        452, // Total = real DB count
			wantDoneTotalIsExact: false,
			wantWarnings:         nil,
		},
		{
			name: "ClosedTotal==len(Closed): Done.Total==ClosedTotal, TotalIsExact=true",
			in: Inputs{
				Closed:      makeClosedLarge(5),
				ClosedLimit: 50,
				ClosedTotal: 5,
			},
			wantDoneLen:          5,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "ClosedTotal<len(Closed) (defensive): Done.Total uses len(Closed), TotalIsExact=true",
			in: Inputs{
				Closed:      makeClosedLarge(10),
				ClosedLimit: 50,
				ClosedTotal: 3, // shouldn't happen, but defensive
			},
			wantDoneLen:          10,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
		{
			name: "ClosedTotal==0 (unset): falls back to ClosedLimit heuristic, items==limit → not exact",
			in: Inputs{
				Closed:      makeClosedLarge(5),
				ClosedLimit: 5,
				ClosedTotal: 0,
			},
			wantDoneLen:          5,
			wantDoneTotalIsExact: false,
			wantWarnings:         nil,
		},
		{
			name: "ClosedTotal==0 (unset): falls back to ClosedLimit heuristic, items<limit → exact",
			in: Inputs{
				Closed:      makeClosedLarge(4),
				ClosedLimit: 5,
				ClosedTotal: 0,
			},
			wantDoneLen:          4,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},

		// ---- input isolation: Compose must not mutate caller's slices ----
		{
			name: "Compose does not mutate input Ready slice",
			in: Inputs{
				Ready: []domain.IssueSummary{
					makeSummary("b", 2, t0),
					makeSummary("a", 1, t0),
				},
				ClosedLimit: 10,
			},
			// We verify mutation separately below; just check counts here.
			wantReadyLen:         2,
			wantDoneTotalIsExact: true,
			wantWarnings:         nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Compose(tc.in)

			// column lengths
			if got.NotReady.Total != tc.wantNotReadyLen {
				t.Errorf("NotReady.Total = %d, want %d", got.NotReady.Total, tc.wantNotReadyLen)
			}
			if len(got.NotReady.Issues) != tc.wantNotReadyLen {
				t.Errorf("len(NotReady.Issues) = %d, want %d", len(got.NotReady.Issues), tc.wantNotReadyLen)
			}
			if got.Ready.Total != tc.wantReadyLen {
				t.Errorf("Ready.Total = %d, want %d", got.Ready.Total, tc.wantReadyLen)
			}
			if len(got.Ready.Issues) != tc.wantReadyLen {
				t.Errorf("len(Ready.Issues) = %d, want %d", len(got.Ready.Issues), tc.wantReadyLen)
			}
			if got.InProgress.Total != tc.wantInProgressLen {
				t.Errorf("InProgress.Total = %d, want %d", got.InProgress.Total, tc.wantInProgressLen)
			}
			if len(got.InProgress.Issues) != tc.wantInProgressLen {
				t.Errorf("len(InProgress.Issues) = %d, want %d", len(got.InProgress.Issues), tc.wantInProgressLen)
			}
			wantDoneTotal := tc.wantDoneLen
			if tc.wantDoneTotal != 0 {
				wantDoneTotal = tc.wantDoneTotal
			}
			if got.Done.Total != wantDoneTotal {
				t.Errorf("Done.Total = %d, want %d", got.Done.Total, wantDoneTotal)
			}
			if len(got.Done.Issues) != tc.wantDoneLen {
				t.Errorf("len(Done.Issues) = %d, want %d", len(got.Done.Issues), tc.wantDoneLen)
			}

			// TotalIsExact (active columns must always be exact)
			if !got.NotReady.TotalIsExact {
				t.Error("NotReady.TotalIsExact should always be true")
			}
			if !got.Ready.TotalIsExact {
				t.Error("Ready.TotalIsExact should always be true")
			}
			if !got.InProgress.TotalIsExact {
				t.Error("InProgress.TotalIsExact should always be true")
			}
			if got.Done.TotalIsExact != tc.wantDoneTotalIsExact {
				t.Errorf("Done.TotalIsExact = %v, want %v", got.Done.TotalIsExact, tc.wantDoneTotalIsExact)
			}

			// warnings
			if tc.wantWarnings == nil {
				if len(got.Warnings) != 0 {
					t.Errorf("Warnings = %v, want none", got.Warnings)
				}
			} else {
				if len(got.Warnings) != len(tc.wantWarnings) {
					t.Errorf("len(Warnings) = %d, want %d; got %v", len(got.Warnings), len(tc.wantWarnings), got.Warnings)
				} else {
					for i, w := range tc.wantWarnings {
						gw := got.Warnings[i]
						if gw.Group != w.group {
							t.Errorf("Warnings[%d].Group = %q, want %q", i, gw.Group, w.group)
						}
						if gw.Threshold != w.threshold {
							t.Errorf("Warnings[%d].Threshold = %d, want %d", i, gw.Threshold, w.threshold)
						}
					}
				}
			}

			// optional ordered ID checks
			if len(tc.wantNotReadyIDs) > 0 {
				assertIDs(t, "NotReady", got.NotReady.Issues, tc.wantNotReadyIDs)
			}
			if len(tc.wantReadyIDs) > 0 {
				assertIDs(t, "Ready", got.Ready.Issues, tc.wantReadyIDs)
			}
			if len(tc.wantInProgressIDs) > 0 {
				assertIDs(t, "InProgress", got.InProgress.Issues, tc.wantInProgressIDs)
			}
			if len(tc.wantDoneIDs) > 0 {
				assertIDs(t, "Done", got.Done.Issues, tc.wantDoneIDs)
			}
		})
	}
}

// TestComposeDoesNotMutateInputSlices verifies that Compose copies input slices
// rather than sorting in-place on the caller's data.
func TestComposeDoesNotMutateInputSlices(t *testing.T) {
	t.Parallel()

	ready := []domain.IssueSummary{
		makeSummary("b", 2, t0),
		makeSummary("a", 1, t0),
	}
	inProgress := []domain.IssueSummary{
		makeSummary("z", 3, t0),
		makeSummary("y", 1, t0),
	}

	// capture original order
	origReady := []string{ready[0].ID, ready[1].ID}
	origInProgress := []string{inProgress[0].ID, inProgress[1].ID}

	Compose(Inputs{
		Ready:      ready,
		InProgress: inProgress,
	})

	// caller's slices must be unchanged
	if ready[0].ID != origReady[0] || ready[1].ID != origReady[1] {
		t.Errorf("Compose mutated Ready input: got %s %s, want %s %s",
			ready[0].ID, ready[1].ID, origReady[0], origReady[1])
	}
	if inProgress[0].ID != origInProgress[0] || inProgress[1].ID != origInProgress[1] {
		t.Errorf("Compose mutated InProgress input: got %s %s, want %s %s",
			inProgress[0].ID, inProgress[1].ID, origInProgress[0], origInProgress[1])
	}
}

// assertIDs checks that the Issues in col have IDs in the expected order.
func assertIDs(t *testing.T, col string, issues []domain.IssueSummary, want []string) {
	t.Helper()
	if len(issues) != len(want) {
		t.Errorf("%s: len(Issues) = %d, want %d", col, len(issues), len(want))
		return
	}
	for i, w := range want {
		if issues[i].ID != w {
			t.Errorf("%s: Issues[%d].ID = %q, want %q", col, i, issues[i].ID, w)
		}
	}
}
