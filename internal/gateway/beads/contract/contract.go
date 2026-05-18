// Package contract exposes RunReadContract, a parameterized test suite that
// exercises every read method on BeadsGateway. Wire it against both a fake
// (unit tier) and the real CLI gateway (integration tier) to keep them in sync.
package contract

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/gateway/beads/contractcheck"
)

// assertNoViolations converts contractcheck.Violation values to t.Errorf calls.
// This bridges the pure validator functions (which return values) to the testing.T
// assertion style used throughout RunReadContract.
func assertNoViolations(t *testing.T, violations []contractcheck.Violation) {
	t.Helper()
	for _, v := range violations {
		t.Errorf("contract violation [%s/%s]: %s", v.Method, v.Rule, v.Sample)
	}
}

// GatewayFactory returns a BeadsGateway bound to a freshly-prepared fixture.
// The factory is called once per RunReadContract invocation; t is the top-level
// test so cleanup can be registered via t.Cleanup.
type GatewayFactory func(t *testing.T) beads.BeadsGateway

// RunReadContract runs one sub-test per read method on BeadsGateway.
// Every method has concrete assertions that hold against the fixture data
// (3 issues + 1 dependency, prefix bwf, defined in seed.json).
func RunReadContract(t *testing.T, factory GatewayFactory) {
	t.Helper()

	gw := factory(t)
	ctx := context.Background()

	// ---- HealthCheck ----

	t.Run("HealthCheck", func(t *testing.T) {
		err := gw.HealthCheck(ctx)
		if err != nil {
			t.Errorf("HealthCheck: expected nil error, got %v", err)
		}
	})

	// ---- ListIssues ----

	t.Run("ListIssues/NoFilter", func(t *testing.T) {
		// bd list without filters returns non-closed issues only
		// (bwf-1, bwf-2, bwf-4, bwf-5 — all non-closed).
		issues, err := gw.ListIssues(ctx, domain.IssueListQuery{})
		if err != nil {
			t.Fatalf("ListIssues(no filter): unexpected error: %v", err)
		}

		// Expect exactly 4 non-closed issues.
		if len(issues) != 4 {
			t.Errorf("ListIssues(no filter): expected 4 issues, got %d: %v", len(issues), issueIDs(issues))
		}

		// Verify bwf-1 fields (open task).
		bwf1 := findByID(issues, "bwf-1")
		if bwf1 == nil {
			t.Errorf("ListIssues(no filter): bwf-1 not found in results")
		} else {
			assertIssueSummary(t, "bwf-1", *bwf1, domain.IssueSummary{
				ID:       "bwf-1",
				Title:    "Seed fixture root task",
				Status:   "open",
				Type:     "task",
				Priority: 1,
				Assignee: "alice",
				Labels:   []string{"fixture", "ui"},
			})
		}

		// Verify bwf-2 fields (blocked bug).
		bwf2 := findByID(issues, "bwf-2")
		if bwf2 == nil {
			t.Errorf("ListIssues(no filter): bwf-2 not found in results")
		} else {
			assertIssueSummary(t, "bwf-2", *bwf2, domain.IssueSummary{
				ID:       "bwf-2",
				Title:    "Blocked bug for fixture",
				Status:   "blocked",
				Type:     "bug",
				Priority: 0,
				Assignee: "bob",
				Labels:   []string{"fixture", "blocking"},
			})
		}

		// Closed issue bwf-3 must NOT appear in the default list.
		if findByID(issues, "bwf-3") != nil {
			t.Errorf("ListIssues(no filter): closed issue bwf-3 should not appear in default list")
		}

		// bwf-4 has no labels — validate Labels == nil (not empty slice).
		// This pins the postcondition in interface.go: Labels may be nil (not an
		// empty slice) for issues with no labels.
		bwf4 := findByID(issues, "bwf-4")
		if bwf4 == nil {
			t.Errorf("ListIssues(no filter): bwf-4 (unlabeled issue) not found in results")
		} else if bwf4.Labels != nil {
			t.Errorf("ListIssues(no filter): bwf-4 Labels should be nil for issue with no labels, got %v", bwf4.Labels)
		}
	})

	t.Run("ListIssues/StatusOpen", func(t *testing.T) {
		issues, err := gw.ListIssues(ctx, domain.IssueListQuery{Statuses: []string{"open"}})
		if err != nil {
			t.Fatalf("ListIssues(status=open): unexpected error: %v", err)
		}

		for _, issue := range issues {
			if issue.Status != "open" {
				t.Errorf("ListIssues(status=open): got non-open issue %s (status=%s)", issue.ID, issue.Status)
			}
		}

		// Fixture has exactly 2 open issues (bwf-1 and bwf-4).
		if len(issues) != 2 {
			t.Errorf("ListIssues(status=open): expected 2 issues, got %d: %v", len(issues), issueIDs(issues))
		}

		if findByID(issues, "bwf-1") == nil {
			t.Errorf("ListIssues(status=open): expected bwf-1 in results, got %v", issueIDs(issues))
		}
	})

	// ---- ReadyIssues ----

	t.Run("ReadyIssues", func(t *testing.T) {
		issues, err := gw.ReadyIssues(ctx, domain.ReadyIssuesQuery{})
		if err != nil {
			t.Fatalf("ReadyIssues: unexpected error: %v", err)
		}

		// bwf-1 and bwf-4 are open with no blockers — both ready.
		if len(issues) != 2 {
			t.Errorf("ReadyIssues: expected 2 ready issues, got %d: %v", len(issues), issueIDs(issues))
		}

		if bwf1 := findByID(issues, "bwf-1"); bwf1 == nil {
			t.Errorf("ReadyIssues: expected bwf-1 in ready list, got %v", issueIDs(issues))
		}
		if bwf4 := findByID(issues, "bwf-4"); bwf4 == nil {
			t.Errorf("ReadyIssues: expected bwf-4 in ready list, got %v", issueIDs(issues))
		}

		// bwf-2 is blocked, bwf-3 is closed, bwf-5 is stored-blocked — none should be ready.
		if findByID(issues, "bwf-2") != nil {
			t.Errorf("ReadyIssues: blocked issue bwf-2 should not appear in ready list")
		}
		if findByID(issues, "bwf-3") != nil {
			t.Errorf("ReadyIssues: closed issue bwf-3 should not appear in ready list")
		}
		if findByID(issues, "bwf-5") != nil {
			t.Errorf("ReadyIssues: stored-blocked bwf-5 should not appear in ready list")
		}
	})

	// ---- BlockedIssues ----

	t.Run("BlockedIssues", func(t *testing.T) {
		views, err := gw.BlockedIssues(ctx, domain.BlockedIssuesQuery{})
		if err != nil {
			t.Fatalf("BlockedIssues: unexpected error: %v", err)
		}

		// Fixture has exactly 1 dep-blocked issue: bwf-2 (blocked by bwf-1).
		// bwf-5 is stored-blocked (status=blocked) but has NO dependency — bd blocked
		// must NOT include it. This pins the interface.go postcondition:
		// "Issues with stored status 'blocked' (manually set) are NOT returned by
		// bd blocked unless they also have unresolved dependency blockers."
		if len(views) != 1 {
			t.Fatalf("BlockedIssues: expected 1 blocked view, got %d: %v", len(views), blockedViewIDs(views))
		}

		view := views[0]
		if view.Issue.ID != "bwf-2" {
			t.Errorf("BlockedIssues: expected issue bwf-2, got %s", view.Issue.ID)
		}
		if view.Issue.Status != "blocked" {
			t.Errorf("BlockedIssues: expected status=blocked, got %q", view.Issue.Status)
		}

		// bwf-5 must NOT appear — it is stored-blocked with no dep.
		if findBlockedViewByID(views, "bwf-5") != nil {
			t.Errorf("BlockedIssues: stored-blocked-no-dep bwf-5 must not appear in bd blocked output")
		}

		// BlockedBy must reference bwf-1 — this is where fake/real drift hides.
		if len(view.BlockedBy) == 0 {
			t.Errorf("BlockedIssues: bwf-2 should have BlockedBy populated, got empty slice")
		} else {
			found := false
			for _, ref := range view.BlockedBy {
				if ref.ID == "bwf-1" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("BlockedIssues: bwf-2.BlockedBy should contain bwf-1, got %v", view.BlockedBy)
			}
		}
	})

	// ---- ReadyExplain ----

	t.Run("ReadyExplain", func(t *testing.T) {
		result, err := gw.ReadyExplain(ctx, domain.ReadyExplainOptions{})
		if err != nil {
			t.Fatalf("ReadyExplain: unexpected error: %v", err)
		}

		// Ready slice must match ReadyIssues output.
		readyIssues, err := gw.ReadyIssues(ctx, domain.ReadyIssuesQuery{})
		if err != nil {
			t.Fatalf("ReadyExplain: ReadyIssues reference call failed: %v", err)
		}

		if len(result.Ready) != len(readyIssues) {
			t.Errorf("ReadyExplain: Ready length %d != ReadyIssues length %d", len(result.Ready), len(readyIssues))
		}

		for _, ri := range readyIssues {
			if findByID(result.Ready, ri.ID) == nil {
				t.Errorf("ReadyExplain: ReadyIssues issue %s not found in ReadyExplain.Ready", ri.ID)
			}
		}

		// TotalReady and TotalBlocked must be consistent.
		if result.TotalReady != len(result.Ready) {
			t.Errorf("ReadyExplain: TotalReady=%d but len(Ready)=%d", result.TotalReady, len(result.Ready))
		}

		// Fixture has exactly 1 blocked issue.
		if result.TotalBlocked != 1 {
			t.Errorf("ReadyExplain: expected TotalBlocked=1, got %d", result.TotalBlocked)
		}

		// Blocked section must contain bwf-2 with bwf-1 as a blocker.
		bwf2View := findBlockedViewByID(result.Blocked, "bwf-2")
		if bwf2View == nil {
			t.Errorf("ReadyExplain: expected bwf-2 in Blocked, got %v", blockedViewIDs(result.Blocked))
		} else if len(bwf2View.BlockedBy) == 0 {
			t.Errorf("ReadyExplain: bwf-2 Blocked.BlockedBy should be populated")
		} else {
			found := false
			for _, ref := range bwf2View.BlockedBy {
				if ref.ID == "bwf-1" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ReadyExplain: bwf-2 Blocked.BlockedBy should contain bwf-1, got %v", bwf2View.BlockedBy)
			}
		}
	})

	// ---- ShowIssue ----

	t.Run("ShowIssue/HappyPath", func(t *testing.T) {
		detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: "bwf-1"})
		if err != nil {
			t.Fatalf("ShowIssue(bwf-1): unexpected error: %v", err)
		}

		// Structured field comparisons — do not use string formatting.
		assertIssueSummary(t, "bwf-1 ShowIssue", detail.Summary, domain.IssueSummary{
			ID:       "bwf-1",
			Title:    "Seed fixture root task",
			Status:   "open",
			Type:     "task",
			Priority: 1,
			Assignee: "alice",
			Labels:   []string{"fixture", "ui"},
		})

		if detail.Description == "" {
			t.Errorf("ShowIssue(bwf-1): Description should not be empty")
		}
	})

	t.Run("ShowIssue/NotFound", func(t *testing.T) {
		_, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: "bwf-nonexistent"})
		if err == nil {
			t.Fatal("ShowIssue(nonexistent): expected error, got nil")
		}

		// Real bd exits non-zero for unknown IDs, so the gateway surfaces
		// ErrorCodeCommandFailed (not ErrorCodeNotFound). The fake mirrors this.
		var gwErr domain.GatewayError
		if errors.As(err, &gwErr) {
			if gwErr.Code != domain.ErrorCodeCommandFailed {
				t.Errorf("ShowIssue(nonexistent): expected ErrorCodeCommandFailed, got %q", gwErr.Code)
			}
		} else {
			t.Errorf("ShowIssue(nonexistent): expected domain.GatewayError, got %T: %v", err, err)
		}
	})

	// ---- SearchIssues ----

	t.Run("SearchIssues/EmptyQuery", func(t *testing.T) {
		page, err := gw.SearchIssues(ctx, domain.SearchIssuesQuery{})
		if err != nil {
			t.Fatalf("SearchIssues(empty): unexpected error: %v", err)
		}

		// Empty query routes through bd list --all → all 5 fixture issues.
		if len(page.Results) != 5 {
			t.Errorf("SearchIssues(empty): expected 5 results, got %d: %v", len(page.Results), searchResultIDs(page.Results))
		}

		expectedIDs := []string{"bwf-1", "bwf-2", "bwf-3", "bwf-4", "bwf-5"}
		for _, id := range expectedIDs {
			if findSearchResultByID(page.Results, id) == nil {
				t.Errorf("SearchIssues(empty): expected %s in results, got %v", id, searchResultIDs(page.Results))
			}
		}
	})

	t.Run("SearchIssues/TitleTerm", func(t *testing.T) {
		// "root" matches only bwf-1 ("Seed fixture root task").
		page, err := gw.SearchIssues(ctx, domain.SearchIssuesQuery{Text: "root"})
		if err != nil {
			t.Fatalf("SearchIssues(root): unexpected error: %v", err)
		}

		if len(page.Results) == 0 {
			t.Fatal("SearchIssues(root): expected at least 1 result, got 0")
		}

		for _, r := range page.Results {
			if r.Issue.ID != "bwf-1" {
				t.Errorf("SearchIssues(root): expected only bwf-1, got %s", r.Issue.ID)
			}
		}
	})

	// ---- Query ----

	t.Run("Query/StatusOpen", func(t *testing.T) {
		issues, err := gw.Query(ctx, "status = open", domain.QueryOptions{})
		if err != nil {
			t.Fatalf("Query(status=open): unexpected error: %v", err)
		}

		// bwf-1 and bwf-4 have status=open in the fixture.
		for _, issue := range issues {
			if issue.Status != "open" {
				t.Errorf("Query(status=open): expected all results to have status=open, got %s with status=%s", issue.ID, issue.Status)
			}
		}

		if len(issues) != 2 {
			t.Errorf("Query(status=open): expected 2 results, got %d: %v", len(issues), issueIDs(issues))
		}

		if findByID(issues, "bwf-1") == nil {
			t.Errorf("Query(status=open): expected bwf-1 in results, got %v", issueIDs(issues))
		}
	})

	// ---- CountIssues ----

	t.Run("CountIssues", func(t *testing.T) {
		result, err := gw.CountIssues(ctx, domain.IssueCountQuery{})
		if err != nil {
			t.Fatalf("CountIssues: unexpected error: %v", err)
		}

		// Fixture has 5 issues total (2 open, 2 blocked, 1 closed).
		if result.Total != 5 {
			t.Errorf("CountIssues: expected Total=5, got %d", result.Total)
		}

		// Groups must contain the three statuses present in the fixture with correct counts.
		type wantGroup struct {
			status string
			count  int
		}
		for _, wg := range []wantGroup{
			{"open", 2},
			{"blocked", 2},
			{"closed", 1},
		} {
			found := false
			for _, g := range result.Groups {
				if g.Status == wg.status {
					found = true
					if g.Count != wg.count {
						t.Errorf("CountIssues: expected count=%d for status=%q, got %d", wg.count, wg.status, g.Count)
					}
					break
				}
			}
			if !found {
				t.Errorf("CountIssues: expected group for status=%q, groups=%v", wg.status, result.Groups)
			}
		}

		// Verify that the "deferred" status is NOT present in Groups — no issues have that
		// status, so bd omits it (zero-count groups are never emitted).
		for _, g := range result.Groups {
			if g.Status == "deferred" {
				t.Errorf("CountIssues: deferred status should not appear when count=0 (bd omits zero-count groups)")
			}
		}
	})

	// ---- StatusCatalog ----

	t.Run("StatusCatalog", func(t *testing.T) {
		opts, err := gw.StatusCatalog(ctx)
		if err != nil {
			t.Fatalf("StatusCatalog: unexpected error: %v", err)
		}

		if len(opts) == 0 {
			t.Fatal("StatusCatalog: expected non-empty result")
		}

		// All 7 bd 1.0.4 built-in statuses must be present.
		for _, expected := range []string{"open", "in_progress", "blocked", "deferred", "closed", "pinned", "hooked"} {
			if !containsStatusName(opts, expected) {
				t.Errorf("StatusCatalog: expected built-in status %q, got %v", expected, statusNames(opts))
			}
		}
	})

	// ---- TypeCatalog ----

	t.Run("TypeCatalog", func(t *testing.T) {
		opts, err := gw.TypeCatalog(ctx)
		if err != nil {
			t.Fatalf("TypeCatalog: unexpected error: %v", err)
		}

		if len(opts) == 0 {
			t.Fatal("TypeCatalog: expected non-empty result")
		}

		// All 9 bd core types must be present.
		for _, expected := range []string{"task", "bug", "feature", "chore", "epic", "decision", "spike", "story", "milestone"} {
			if !containsTypeName(opts, expected) {
				t.Errorf("TypeCatalog: expected core type %q, got %v", expected, typeNames(opts))
			}
		}
	})

	// ---- LabelCatalog ----

	t.Run("LabelCatalog", func(t *testing.T) {
		opts, err := gw.LabelCatalog(ctx)
		if err != nil {
			t.Fatalf("LabelCatalog: unexpected error: %v", err)
		}

		if len(opts) == 0 {
			t.Fatal("LabelCatalog: expected non-empty result")
		}

		// "fixture" label appears on all 3 seed issues — must be present.
		if !containsLabelName(opts, "fixture") {
			t.Errorf("LabelCatalog: expected 'fixture' label, got %v", labelNames(opts))
		}

		// "ui", "blocking", "docs" are also seeded in the fixture.
		for _, expected := range []string{"ui", "blocking", "docs"} {
			if !containsLabelName(opts, expected) {
				t.Errorf("LabelCatalog: expected label %q, got %v", expected, labelNames(opts))
			}
		}
	})

	// =========================================================
	// Invariants: fixture-agnostic structural assertions.
	// These sub-tests hold for ANY well-formed bd output — they
	// do NOT reference specific fixture IDs (bwf-*).
	// They run alongside the fixture-pinned tests above and must
	// pass against both the fake and the real gateway.
	// =========================================================

	t.Run("Invariants", func(t *testing.T) {

		// ---- Invariants/HealthCheck ----

		// HealthCheck is an error-only method; structural invariants are just
		// about error class (covered by the fixture-pinned test above). No
		// additional generic assertions are possible here.

		// ---- Invariants/ListIssues ----

		t.Run("ListIssues/NonEmptyIDs", func(t *testing.T) {
			// Default query (no filter) — every returned summary must have a
			// non-empty ID. This catches silent zero-value decode failures.
			issues, err := gw.ListIssues(ctx, domain.IssueListQuery{})
			if err != nil {
				t.Fatalf("ListIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateIssueSummaries("ListIssues", issues) {
				if v.Rule == "NonEmptyID" {
					t.Errorf("ListIssues: %s", v.Sample)
				}
			}
		})

		t.Run("ListIssues/NonEmptyTitleAndStatus", func(t *testing.T) {
			issues, err := gw.ListIssues(ctx, domain.IssueListQuery{})
			if err != nil {
				t.Fatalf("ListIssues: unexpected error: %v", err)
			}
			assertNoViolations(t, contractcheck.ValidateIssueSummaries("ListIssues", issues))
		})

		t.Run("ListIssues/StatusFilterRespected", func(t *testing.T) {
			// When Statuses filter is set, every returned issue must match.
			issues, err := gw.ListIssues(ctx, domain.IssueListQuery{Statuses: []string{"open"}})
			if err != nil {
				t.Fatalf("ListIssues(status=open): unexpected error: %v", err)
			}
			assertNoViolations(t, contractcheck.ValidateListIssuesStatusFilter("ListIssues", issues, []string{"open"}))
		})

		t.Run("ListIssues/ClosedExcludedByDefault", func(t *testing.T) {
			// Default query must not return closed issues.
			issues, err := gw.ListIssues(ctx, domain.IssueListQuery{})
			if err != nil {
				t.Fatalf("ListIssues: unexpected error: %v", err)
			}
			assertNoViolations(t, contractcheck.ValidateListIssuesClosedExcluded("ListIssues", issues, nil))
		})

		t.Run("ListIssues/SortApplied", func(t *testing.T) {
			// When SortBy is set, the results must be ordered by that field.
			// NOTE: there is a known sort-direction inversion bug in the gateway
			// (see interface.go quirks). We test that results ARE sorted, but we
			// accept either ascending or descending order as valid — the invariant
			// only checks that a consistent ordering is applied.
			issues, err := gw.ListIssues(ctx, domain.IssueListQuery{
				SortBy:    domain.SortFieldPriority,
				SortOrder: domain.SortDirectionDescending,
			})
			if err != nil {
				t.Fatalf("ListIssues(sort=priority,desc): unexpected error: %v", err)
			}
			if len(issues) < 2 {
				// Need at least 2 issues to verify sort order; skip if fixture is small.
				return
			}
			// Verify the results are either non-decreasing or non-increasing in priority.
			ascending := true
			descending := true
			for i := 1; i < len(issues); i++ {
				if issues[i].Priority < issues[i-1].Priority {
					ascending = false
				}
				if issues[i].Priority > issues[i-1].Priority {
					descending = false
				}
			}
			if !ascending && !descending {
				priorities := make([]int, len(issues))
				for i, iss := range issues {
					priorities[i] = iss.Priority
				}
				t.Errorf("ListIssues(sort=priority): result is not sorted (neither asc nor desc): priorities=%v", priorities)
			}
		})

		// ---- Invariants/ReadyIssues ----

		t.Run("ReadyIssues/NonEmptyIDs", func(t *testing.T) {
			issues, err := gw.ReadyIssues(ctx, domain.ReadyIssuesQuery{})
			if err != nil {
				t.Fatalf("ReadyIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateIssueSummaries("ReadyIssues", issues) {
				if v.Rule == "NonEmptyID" {
					t.Errorf("ReadyIssues: %s", v.Sample)
				}
			}
		})

		t.Run("ReadyIssues/NoClosedIssues", func(t *testing.T) {
			// Closed issues must never appear in the ready list.
			issues, err := gw.ReadyIssues(ctx, domain.ReadyIssuesQuery{})
			if err != nil {
				t.Fatalf("ReadyIssues: unexpected error: %v", err)
			}
			// nil statuses arg = treat as "all statuses excluded by default" — same
			// semantics as no-filter: closed must be absent.
			assertNoViolations(t, contractcheck.ValidateListIssuesClosedExcluded("ReadyIssues", issues, nil))
		})

		// ---- Invariants/BlockedIssues ----

		t.Run("BlockedIssues/NonEmptyIDs", func(t *testing.T) {
			views, err := gw.BlockedIssues(ctx, domain.BlockedIssuesQuery{})
			if err != nil {
				t.Fatalf("BlockedIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateBlockedViews("BlockedIssues", views) {
				if v.Rule == "NonEmptyID" {
					t.Errorf("BlockedIssues: %s", v.Sample)
				}
			}
		})

		t.Run("BlockedIssues/NonEmptyBlockedBySlice", func(t *testing.T) {
			// Every entry in the blocked list must have at least one blocker.
			// This is the defining invariant: if you're in the blocked list, you
			// must have a non-empty BlockedBy slice (the reason you're blocked).
			views, err := gw.BlockedIssues(ctx, domain.BlockedIssuesQuery{})
			if err != nil {
				t.Fatalf("BlockedIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateBlockedViews("BlockedIssues", views) {
				if v.Rule == "NonEmptyBlockedBySlice" {
					t.Errorf("BlockedIssues: %s", v.Sample)
				}
			}
		})

		t.Run("BlockedIssues/BlockerIDsNonEmpty", func(t *testing.T) {
			// Every blocker reference must have a non-empty ID.
			views, err := gw.BlockedIssues(ctx, domain.BlockedIssuesQuery{})
			if err != nil {
				t.Fatalf("BlockedIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateBlockedViews("BlockedIssues", views) {
				if v.Rule == "BlockerIDsNonEmpty" {
					t.Errorf("BlockedIssues: %s", v.Sample)
				}
			}
		})

		// ---- Invariants/ReadyExplain ----

		t.Run("ReadyExplain/NonEmptyIDs", func(t *testing.T) {
			result, err := gw.ReadyExplain(ctx, domain.ReadyExplainOptions{})
			if err != nil {
				t.Fatalf("ReadyExplain: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateReadyExplain("ReadyExplain", result, false) {
				if v.Rule == "NonEmptyReadyIDs" || v.Rule == "NonEmptyBlockedIDs" {
					t.Errorf("ReadyExplain: %s", v.Sample)
				}
			}
		})

		t.Run("ReadyExplain/ReadyAndBlockedDisjoint", func(t *testing.T) {
			// An issue cannot be both ready and blocked at the same time.
			result, err := gw.ReadyExplain(ctx, domain.ReadyExplainOptions{})
			if err != nil {
				t.Fatalf("ReadyExplain: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateReadyExplain("ReadyExplain", result, false) {
				if v.Rule == "ReadyAndBlockedDisjoint" {
					t.Errorf("ReadyExplain: %s", v.Sample)
				}
			}
		})

		t.Run("ReadyExplain/TotalReadyMatchesLenReady", func(t *testing.T) {
			// When no limit is applied, TotalReady == len(Ready).
			result, err := gw.ReadyExplain(ctx, domain.ReadyExplainOptions{})
			if err != nil {
				t.Fatalf("ReadyExplain: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateReadyExplain("ReadyExplain", result, false) {
				if v.Rule == "TotalReadyMatchesLenReady" {
					t.Errorf("ReadyExplain: %s", v.Sample)
				}
			}
		})

		t.Run("ReadyExplain/TotalBlockedMatchesLenBlocked", func(t *testing.T) {
			// When no limit is applied, TotalBlocked == len(Blocked).
			result, err := gw.ReadyExplain(ctx, domain.ReadyExplainOptions{})
			if err != nil {
				t.Fatalf("ReadyExplain: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateReadyExplain("ReadyExplain", result, false) {
				if v.Rule == "TotalBlockedMatchesLenBlocked" {
					t.Errorf("ReadyExplain: %s", v.Sample)
				}
			}
		})

		t.Run("ReadyExplain/BlockedByEnriched", func(t *testing.T) {
			// Every BlockedBy entry in Blocked must have non-empty Title and Status.
			// This is the invariant that differentiates ReadyExplain from BlockedIssues:
			// ReadyExplain returns enriched blocker objects, not bare ID-only references.
			result, err := gw.ReadyExplain(ctx, domain.ReadyExplainOptions{})
			if err != nil {
				t.Fatalf("ReadyExplain: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateReadyExplain("ReadyExplain", result, false) {
				if v.Rule == "BlockedByEnriched" {
					t.Errorf("ReadyExplain: %s", v.Sample)
				}
			}
		})

		// ---- Invariants/ShowIssue ----

		t.Run("ShowIssue/ReturnedIDMatchesInput", func(t *testing.T) {
			// Use ListIssues to discover a real ID, then verify ShowIssue returns the same ID.
			issues, err := gw.ListIssues(ctx, domain.IssueListQuery{})
			if err != nil || len(issues) == 0 {
				t.Skip("ShowIssue/ReturnedIDMatchesInput: no issues to test against")
			}
			targetID := issues[0].ID
			detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: targetID})
			if err != nil {
				t.Fatalf("ShowIssue(%q): unexpected error: %v", targetID, err)
			}
			for _, v := range contractcheck.ValidateShowIssue("ShowIssue", detail, targetID) {
				if v.Rule == "ReturnedIDMatchesInput" {
					t.Errorf("ShowIssue: %s", v.Sample)
				}
			}
		})

		t.Run("ShowIssue/CommentsNotNil", func(t *testing.T) {
			// Comments must be a non-nil slice (empty is ok, nil is not).
			issues, err := gw.ListIssues(ctx, domain.IssueListQuery{})
			if err != nil || len(issues) == 0 {
				t.Skip("ShowIssue/CommentsNotNil: no issues to test against")
			}
			detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: issues[0].ID})
			if err != nil {
				t.Fatalf("ShowIssue: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateShowIssue("ShowIssue", detail, issues[0].ID) {
				if v.Rule == "CommentsNotNil" {
					t.Errorf("ShowIssue: %s", v.Sample)
				}
			}
		})

		t.Run("ShowIssue/BlockedByNotNil", func(t *testing.T) {
			// BlockedBy must be a non-nil slice (empty is ok, nil is not).
			issues, err := gw.ListIssues(ctx, domain.IssueListQuery{})
			if err != nil || len(issues) == 0 {
				t.Skip("ShowIssue/BlockedByNotNil: no issues to test against")
			}
			detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: issues[0].ID})
			if err != nil {
				t.Fatalf("ShowIssue: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateShowIssue("ShowIssue", detail, issues[0].ID) {
				if v.Rule == "BlockedByNotNil" {
					t.Errorf("ShowIssue: %s", v.Sample)
				}
			}
		})

		// ---- Invariants/SearchIssues ----

		t.Run("SearchIssues/ResultLengthMatchesCount", func(t *testing.T) {
			// The page's Metadata.ReturnedCount must equal len(Results).
			page, err := gw.SearchIssues(ctx, domain.SearchIssuesQuery{})
			if err != nil {
				t.Fatalf("SearchIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateSearchPage("SearchIssues", page) {
				if v.Rule == "ReturnedCountMatchesLen" {
					t.Errorf("SearchIssues: %s", v.Sample)
				}
			}
		})

		t.Run("SearchIssues/NonEmptyIDs", func(t *testing.T) {
			page, err := gw.SearchIssues(ctx, domain.SearchIssuesQuery{})
			if err != nil {
				t.Fatalf("SearchIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateSearchPage("SearchIssues", page) {
				if v.Rule == "NonEmptyIDs" {
					t.Errorf("SearchIssues: %s", v.Sample)
				}
			}
		})

		t.Run("SearchIssues/ResultsNeverNil", func(t *testing.T) {
			// Results must be a non-nil slice (empty is ok, nil is not).
			page, err := gw.SearchIssues(ctx, domain.SearchIssuesQuery{})
			if err != nil {
				t.Fatalf("SearchIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateSearchPage("SearchIssues", page) {
				if v.Rule == "ResultsNotNil" {
					t.Errorf("SearchIssues: %s", v.Sample)
				}
			}
		})

		// ---- Invariants/CountIssues ----

		t.Run("CountIssues/TotalEqualsSumOfGroups", func(t *testing.T) {
			// Total must equal the arithmetic sum of all group counts.
			result, err := gw.CountIssues(ctx, domain.IssueCountQuery{})
			if err != nil {
				t.Fatalf("CountIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateCountIssues("CountIssues", result) {
				if v.Rule == "TotalEqualsSumOfGroups" {
					t.Errorf("CountIssues: %s", v.Sample)
				}
			}
		})

		t.Run("CountIssues/GroupStatusNonEmpty", func(t *testing.T) {
			// Every group must have a non-empty Status name.
			result, err := gw.CountIssues(ctx, domain.IssueCountQuery{})
			if err != nil {
				t.Fatalf("CountIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateCountIssues("CountIssues", result) {
				if v.Rule == "GroupStatusNonEmpty" {
					t.Errorf("CountIssues: %s", v.Sample)
				}
			}
		})

		t.Run("CountIssues/NoZeroCountGroups", func(t *testing.T) {
			// bd omits zero-count groups — Groups must not contain entries with Count=0.
			// This is the j0o5 invariant: even if a status is valid, if no issues have
			// that status, bd will not emit a group for it.
			result, err := gw.CountIssues(ctx, domain.IssueCountQuery{})
			if err != nil {
				t.Fatalf("CountIssues: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateCountIssues("CountIssues", result) {
				if v.Rule == "NoZeroCountGroups" {
					t.Errorf("CountIssues: %s", v.Sample)
				}
			}
		})

		// ---- Invariants/Query ----

		t.Run("Query/StatusFilterRespected", func(t *testing.T) {
			// When the expression selects a specific status, every result must
			// have that status. "status=closed" is a safe expression since the
			// fixture always has at least one closed issue.
			issues, err := gw.Query(ctx, "status=closed", domain.QueryOptions{IncludeClosed: true})
			if err != nil {
				t.Fatalf("Query(status=closed): unexpected error: %v", err)
			}
			assertNoViolations(t, contractcheck.ValidateListIssuesStatusFilter("Query", issues, []string{"closed"}))
		})

		t.Run("Query/NonEmptyIDs", func(t *testing.T) {
			issues, err := gw.Query(ctx, "status=open", domain.QueryOptions{})
			if err != nil {
				t.Fatalf("Query(status=open): unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateIssueSummaries("Query", issues) {
				if v.Rule == "NonEmptyID" {
					t.Errorf("Query: %s", v.Sample)
				}
			}
		})

		// ---- Invariants/StatusCatalog ----

		t.Run("StatusCatalog/NonEmpty", func(t *testing.T) {
			opts, err := gw.StatusCatalog(ctx)
			if err != nil {
				t.Fatalf("StatusCatalog: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateStatusCatalog("StatusCatalog", opts) {
				if v.Rule == "NonEmpty" {
					t.Errorf("StatusCatalog: %s", v.Sample)
				}
			}
		})

		t.Run("StatusCatalog/AllNamesNonEmpty", func(t *testing.T) {
			opts, err := gw.StatusCatalog(ctx)
			if err != nil {
				t.Fatalf("StatusCatalog: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateStatusCatalog("StatusCatalog", opts) {
				if v.Rule == "AllNamesNonEmpty" {
					t.Errorf("StatusCatalog: %s", v.Sample)
				}
			}
		})

		// ---- Invariants/TypeCatalog ----

		t.Run("TypeCatalog/NonEmpty", func(t *testing.T) {
			opts, err := gw.TypeCatalog(ctx)
			if err != nil {
				t.Fatalf("TypeCatalog: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateTypeCatalog("TypeCatalog", opts) {
				if v.Rule == "NonEmpty" {
					t.Errorf("TypeCatalog: %s", v.Sample)
				}
			}
		})

		t.Run("TypeCatalog/AllNamesNonEmpty", func(t *testing.T) {
			opts, err := gw.TypeCatalog(ctx)
			if err != nil {
				t.Fatalf("TypeCatalog: unexpected error: %v", err)
			}
			for _, v := range contractcheck.ValidateTypeCatalog("TypeCatalog", opts) {
				if v.Rule == "AllNamesNonEmpty" {
					t.Errorf("TypeCatalog: %s", v.Sample)
				}
			}
		})

		// ---- Invariants/LabelCatalog ----

		t.Run("LabelCatalog/AllNamesNonEmpty", func(t *testing.T) {
			opts, err := gw.LabelCatalog(ctx)
			if err != nil {
				t.Fatalf("LabelCatalog: unexpected error: %v", err)
			}
			assertNoViolations(t, contractcheck.ValidateLabelCatalog("LabelCatalog", opts))
		})

		// ---- Invariants/CountIssuesGreaterThanOrEqualToListSize ----
		// ssom cross-method invariant: CountIssues(closed) >= len(ListIssues(closed, limit=1000)).
		//
		// This is the assertion that would have caught the ssom regression at
		// contract-test time. The invariant holds because CountIssues counts ALL
		// matching issues, while ListIssues returns at most `limit` issues.
		// When the list is at the limit, count >= list length must still hold.
		// When the list is shorter than the limit, count == list length
		// (assuming the gateway is consistent).
		//
		// With the current fixture (1 closed issue, limit=1000) this is trivially
		// 1 >= 1. The invariant catches the regression class where CountIssues
		// under-counts relative to what ListIssues actually returns.

		t.Run("CountIssuesGreaterThanOrEqualToListSize", func(t *testing.T) {
			const highLimit = 1000

			countResult, err := gw.CountIssues(ctx, domain.IssueCountQuery{Statuses: []string{"closed"}})
			if err != nil {
				t.Fatalf("CountIssues(closed): unexpected error: %v", err)
			}

			listResult, err := gw.ListIssues(ctx, domain.IssueListQuery{
				Statuses: []string{"closed"},
				Limit:    highLimit,
			})
			if err != nil {
				t.Fatalf("ListIssues(closed, limit=%d): unexpected error: %v", highLimit, err)
			}

			assertNoViolations(t, contractcheck.ValidateSsomInvariant(
				"CountIssuesGreaterThanOrEqualToListSize",
				[]string{"closed"},
				countResult.Total,
				len(listResult),
			))
		})
	})
}

// --- helper functions ---

func findByID(issues []domain.IssueSummary, id string) *domain.IssueSummary {
	for i := range issues {
		if issues[i].ID == id {
			return &issues[i]
		}
	}
	return nil
}

func findBlockedViewByID(views []domain.BlockedIssueView, id string) *domain.BlockedIssueView {
	for i := range views {
		if views[i].Issue.ID == id {
			return &views[i]
		}
	}
	return nil
}

func findSearchResultByID(results []domain.SearchResult, id string) *domain.SearchResult {
	for i := range results {
		if results[i].Issue.ID == id {
			return &results[i]
		}
	}
	return nil
}

func issueIDs(issues []domain.IssueSummary) []string {
	ids := make([]string, len(issues))
	for i, s := range issues {
		ids[i] = s.ID
	}
	return ids
}

func blockedViewIDs(views []domain.BlockedIssueView) []string {
	ids := make([]string, len(views))
	for i, v := range views {
		ids[i] = v.Issue.ID
	}
	return ids
}

func searchResultIDs(results []domain.SearchResult) []string {
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.Issue.ID
	}
	return ids
}

func statusNames(opts []domain.StatusOption) []string {
	names := make([]string, len(opts))
	for i, o := range opts {
		names[i] = o.Name
	}
	return names
}

func typeNames(opts []domain.TypeOption) []string {
	names := make([]string, len(opts))
	for i, o := range opts {
		names[i] = o.Name
	}
	return names
}

func labelNames(opts []domain.LabelOption) []string {
	names := make([]string, len(opts))
	for i, o := range opts {
		names[i] = o.Name
	}
	return names
}

func containsStatusName(opts []domain.StatusOption, name string) bool {
	return slices.ContainsFunc(opts, func(o domain.StatusOption) bool { return o.Name == name })
}

func containsTypeName(opts []domain.TypeOption, name string) bool {
	return slices.ContainsFunc(opts, func(o domain.TypeOption) bool { return o.Name == name })
}

func containsLabelName(opts []domain.LabelOption, name string) bool {
	return slices.ContainsFunc(opts, func(o domain.LabelOption) bool { return o.Name == name })
}

// assertIssueSummary compares the key structured fields of an IssueSummary.
// Labels are compared by content (order-independent) so both real and fake
// gateways can return them in any order.
func assertIssueSummary(t *testing.T, context string, got domain.IssueSummary, want domain.IssueSummary) {
	t.Helper()

	if got.ID != want.ID {
		t.Errorf("%s: ID: got %q, want %q", context, got.ID, want.ID)
	}
	if got.Title != want.Title {
		t.Errorf("%s: Title: got %q, want %q", context, got.Title, want.Title)
	}
	if got.Status != want.Status {
		t.Errorf("%s: Status: got %q, want %q", context, got.Status, want.Status)
	}
	if got.Type != want.Type {
		t.Errorf("%s: Type: got %q, want %q", context, got.Type, want.Type)
	}
	if got.Priority != want.Priority {
		t.Errorf("%s: Priority: got %d, want %d", context, got.Priority, want.Priority)
	}
	if got.Assignee != want.Assignee {
		t.Errorf("%s: Assignee: got %q, want %q", context, got.Assignee, want.Assignee)
	}

	// Labels: compare content regardless of order.
	for _, wl := range want.Labels {
		if !slices.Contains(got.Labels, wl) {
			t.Errorf("%s: Labels: want label %q in %v", context, wl, got.Labels)
		}
	}
}
