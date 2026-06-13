package dashboard

import (
	"strconv"
	"testing"
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// makeClosedSummary builds an IssueSummary with UpdatedAt set to the given
// time, mimicking a closed issue whose UpdatedAt proxies ClosedAt.
func makeClosedSummary(id string, updatedAt time.Time) domain.IssueSummary {
	return domain.IssueSummary{
		ID:        id,
		Status:    "closed",
		Priority:  1,
		UpdatedAt: updatedAt,
	}
}

// descAt returns a base time offset by -n hours (so index 0 is the newest,
// matching ClosedAt DESC ordering).
func descAt(base time.Time, n int) time.Time {
	return base.Add(-time.Duration(n) * time.Hour)
}

// ---- TestCompose_AppendClosed: load-more merge path ----

func TestCompose_AppendClosed(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Build a slice of n issues in UpdatedAt DESC order (newest first).
	// The i-th issue has UpdatedAt = base - i*hour; ID = "c<i>".
	makeDescPage := func(startIdx, count int) []domain.IssueSummary {
		out := make([]domain.IssueSummary, count)
		for i := range out {
			out[i] = makeClosedSummary(
				"c"+strconv.Itoa(startIdx+i),
				descAt(base, startIdx+i),
			)
		}
		return out
	}

	t.Run("prior=35 new=50 ClosedTotal=736 → Total=736 TotalIsExact=false len=85", func(t *testing.T) {
		t.Parallel()

		prior := makeDescPage(0, 35)     // indices 0..34 (newest)
		incoming := makeDescPage(35, 50) // indices 35..84 (older)

		got := Compose(Inputs{
			PriorClosed: prior,
			Closed:      incoming,
			ClosedTotal: 736,
		})

		if got.Done.Total != 736 {
			t.Errorf("Done.Total = %d, want 736", got.Done.Total)
		}
		if got.Done.TotalIsExact {
			t.Errorf("Done.TotalIsExact = true, want false (736 > 85)")
		}
		if len(got.Done.Issues) != 85 {
			t.Errorf("len(Done.Issues) = %d, want 85", len(got.Done.Issues))
		}
	})

	t.Run("prior=0 new=50 ClosedTotal=50 → TotalIsExact=true", func(t *testing.T) {
		t.Parallel()

		incoming := makeDescPage(0, 50)

		got := Compose(Inputs{
			PriorClosed: []domain.IssueSummary{}, // non-nil, empty = trigger load-more path
			Closed:      incoming,
			ClosedTotal: 50,
		})

		if !got.Done.TotalIsExact {
			t.Errorf("Done.TotalIsExact = false, want true (50 == 50)")
		}
		if got.Done.Total != 50 {
			t.Errorf("Done.Total = %d, want 50", got.Done.Total)
		}
		if len(got.Done.Issues) != 50 {
			t.Errorf("len(Done.Issues) = %d, want 50", len(got.Done.Issues))
		}
	})

	t.Run("overlap defense: incoming wins; len = union size", func(t *testing.T) {
		t.Parallel()

		// prior has IDs c0..c34 (35 issues).
		// incoming has IDs c30..c79 (50 issues, overlapping c30..c34 with prior).
		// Union = c0..c79 = 80 unique IDs.
		prior := makeDescPage(0, 35)
		incoming := makeDescPage(30, 50)

		got := Compose(Inputs{
			PriorClosed: prior,
			Closed:      incoming,
			ClosedTotal: 800,
		})

		wantLen := 80 // 35 + 50 - 5 overlap
		if len(got.Done.Issues) != wantLen {
			t.Errorf("len(Done.Issues) = %d, want %d (union of prior+incoming)", len(got.Done.Issues), wantLen)
		}

		// Verify that for the overlapping IDs, the incoming version wins.
		// Both prior and incoming used the same makeDescPage helper with the same
		// UpdatedAt per ID, so content is identical here — dedup should still
		// produce exactly 80 entries (not 85).
		seen := make(map[string]int)
		for _, iss := range got.Done.Issues {
			seen[iss.ID]++
		}
		for id, count := range seen {
			if count > 1 {
				t.Errorf("ID %q appears %d times in merged result; want 1", id, count)
			}
		}
	})

	t.Run("overlap defense: incoming version replaces prior version", func(t *testing.T) {
		t.Parallel()

		// prior contains "overlap" with stale UpdatedAt.
		// incoming contains "overlap" with a newer UpdatedAt (updated_at = now).
		staleTime := base.Add(-72 * time.Hour)
		freshTime := base.Add(-1 * time.Hour)

		prior := []domain.IssueSummary{
			makeClosedSummary("overlap", staleTime),
			makeClosedSummary("old1", base.Add(-48*time.Hour)),
		}
		incoming := []domain.IssueSummary{
			makeClosedSummary("new1", freshTime),
			{ID: "overlap", Status: "closed", Priority: 99, UpdatedAt: freshTime}, // incoming version with Priority=99
		}

		got := Compose(Inputs{
			PriorClosed: prior,
			Closed:      incoming,
			ClosedTotal: 100,
		})

		// Should have 3 unique issues: overlap, old1, new1.
		if len(got.Done.Issues) != 3 {
			t.Errorf("len(Done.Issues) = %d, want 3", len(got.Done.Issues))
		}
		// Find "overlap" and verify it is the incoming version (Priority=99).
		for _, iss := range got.Done.Issues {
			if iss.ID == "overlap" && iss.Priority != 99 {
				t.Errorf(`"overlap" issue Priority = %d, want 99 (incoming version should win)`, iss.Priority)
			}
		}
	})

	t.Run("order: prior DESC + incoming DESC → merged contiguous DESC", func(t *testing.T) {
		t.Parallel()

		// prior: indices 0..4 (newest first), incoming: indices 5..9 (older).
		// Simple concatenation already produces DESC — test that merged is DESC.
		prior := makeDescPage(0, 5)
		incoming := makeDescPage(5, 5)

		got := Compose(Inputs{
			PriorClosed: prior,
			Closed:      incoming,
			ClosedTotal: 10,
		})

		assertClosedAtDESC(t, "normal prior+incoming", got.Done.Issues)
	})

	t.Run("order: not-already-sorted across boundary → sort-merge recovers DESC", func(t *testing.T) {
		t.Parallel()

		// Construct a case where concatenating prior+incoming would NOT be DESC:
		// prior contains a very old issue at the end; incoming starts with a
		// moderately recent issue that is newer than that old prior issue.
		// After concatenation the last element of prior is OLDER than the first
		// element of incoming, so a naïve concat is out of order.
		// The composer must sort-merge to recover DESC.

		veryRecent := base.Add(-1 * time.Hour)
		moderate := base.Add(-10 * time.Hour)
		veryOld := base.Add(-100 * time.Hour)
		slightlyOld := base.Add(-20 * time.Hour)

		// prior (as if already on screen): recent, then veryOld — DESC within prior
		prior := []domain.IssueSummary{
			makeClosedSummary("p0", veryRecent),
			makeClosedSummary("p1", veryOld), // out-of-order relative to incoming
		}
		// incoming from repo: moderate, then slightlyOld — DESC within incoming
		// But moderate > veryOld, so concat(prior, incoming) would put veryOld
		// before moderate — NOT DESC.
		incoming := []domain.IssueSummary{
			makeClosedSummary("i0", moderate),
			makeClosedSummary("i1", slightlyOld),
		}

		got := Compose(Inputs{
			PriorClosed: prior,
			Closed:      incoming,
			ClosedTotal: 4,
		})

		if len(got.Done.Issues) != 4 {
			t.Fatalf("len(Done.Issues) = %d, want 4", len(got.Done.Issues))
		}
		// Expected order after sort-merge DESC by UpdatedAt:
		// veryRecent, moderate, slightlyOld, veryOld
		wantIDs := []string{"p0", "i0", "i1", "p1"}
		assertIDs(t, "Done (sort-merge recovery)", got.Done.Issues, wantIDs)
		assertClosedAtDESC(t, "sort-merge recovery", got.Done.Issues)
	})

	t.Run("PriorClosed=nil uses first-load path (existing behavior)", func(t *testing.T) {
		t.Parallel()

		// Sanity: nil PriorClosed must NOT trigger the merge path.
		// ClosedTotal=5, Closed has 3 items → TotalIsExact=false (not enough to cover 5).
		incoming := makeDescPage(0, 3)

		got := Compose(Inputs{
			PriorClosed: nil,
			Closed:      incoming,
			ClosedTotal: 5,
		})

		if got.Done.TotalIsExact {
			t.Errorf("Done.TotalIsExact = true, want false (3 < 5)")
		}
		if got.Done.Total != 5 {
			t.Errorf("Done.Total = %d, want 5", got.Done.Total)
		}
	})
}

// ---- TestMergeClosedAppend: unit tests for the helper ----

func TestMergeClosedAppend(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty prior returns copy of incoming", func(t *testing.T) {
		t.Parallel()
		incoming := []domain.IssueSummary{
			makeClosedSummary("a", base),
			makeClosedSummary("b", base.Add(-1*time.Hour)),
		}
		got := mergeClosedAppend(nil, incoming)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].ID != "a" || got[1].ID != "b" {
			t.Errorf("IDs = [%s %s], want [a b]", got[0].ID, got[1].ID)
		}
	})

	t.Run("empty incoming returns copy of prior", func(t *testing.T) {
		t.Parallel()
		prior := []domain.IssueSummary{
			makeClosedSummary("x", base),
		}
		got := mergeClosedAppend(prior, nil)
		if len(got) != 1 || got[0].ID != "x" {
			t.Errorf("got %v, want [{x}]", got)
		}
	})

	t.Run("no overlap: all prior + all incoming", func(t *testing.T) {
		t.Parallel()
		prior := []domain.IssueSummary{makeClosedSummary("p1", base)}
		incoming := []domain.IssueSummary{makeClosedSummary("i1", base.Add(-2*time.Hour))}
		got := mergeClosedAppend(prior, incoming)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
	})

	t.Run("full overlap: incoming wins, len = incoming len", func(t *testing.T) {
		t.Parallel()
		t1 := base.Add(-1 * time.Hour)
		t2 := base.Add(-2 * time.Hour)
		prior := []domain.IssueSummary{
			{ID: "a", Status: "closed", Priority: 1, UpdatedAt: t1},
			{ID: "b", Status: "closed", Priority: 1, UpdatedAt: t2},
		}
		incoming := []domain.IssueSummary{
			{ID: "a", Status: "closed", Priority: 99, UpdatedAt: t1},
			{ID: "b", Status: "closed", Priority: 99, UpdatedAt: t2},
		}
		got := mergeClosedAppend(prior, incoming)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		for _, iss := range got {
			if iss.Priority != 99 {
				t.Errorf("ID=%s Priority=%d, want 99 (incoming should win)", iss.ID, iss.Priority)
			}
		}
	})

	t.Run("does not mutate prior or incoming", func(t *testing.T) {
		t.Parallel()
		prior := []domain.IssueSummary{makeClosedSummary("p", base)}
		incoming := []domain.IssueSummary{makeClosedSummary("i", base.Add(-1*time.Hour))}

		priorOrigID := prior[0].ID
		incomingOrigID := incoming[0].ID

		mergeClosedAppend(prior, incoming)

		if prior[0].ID != priorOrigID {
			t.Errorf("prior mutated: ID = %q, want %q", prior[0].ID, priorOrigID)
		}
		if incoming[0].ID != incomingOrigID {
			t.Errorf("incoming mutated: ID = %q, want %q", incoming[0].ID, incomingOrigID)
		}
	})
}

// assertClosedAtDESC verifies that the issues slice is in UpdatedAt DESC order.
func assertClosedAtDESC(t *testing.T, label string, issues []domain.IssueSummary) {
	t.Helper()
	for i := 1; i < len(issues); i++ {
		if issues[i].UpdatedAt.After(issues[i-1].UpdatedAt) {
			t.Errorf("%s: UpdatedAt not DESC at index %d: issues[%d].UpdatedAt=%v > issues[%d].UpdatedAt=%v",
				label, i, i, issues[i].UpdatedAt, i-1, issues[i-1].UpdatedAt)
		}
	}
}
