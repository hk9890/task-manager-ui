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
)

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
		// bd list without filters returns non-closed issues only (bwf-1 and bwf-2).
		issues, err := gw.ListIssues(ctx, domain.IssueListQuery{})
		if err != nil {
			t.Fatalf("ListIssues(no filter): unexpected error: %v", err)
		}

		// Expect exactly 2 non-closed issues.
		if len(issues) != 2 {
			t.Errorf("ListIssues(no filter): expected 2 issues, got %d: %v", len(issues), issueIDs(issues))
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

		// Fixture has exactly 1 open issue (bwf-1).
		if len(issues) != 1 {
			t.Errorf("ListIssues(status=open): expected 1 issue, got %d: %v", len(issues), issueIDs(issues))
		}

		if len(issues) > 0 && issues[0].ID != "bwf-1" {
			t.Errorf("ListIssues(status=open): expected bwf-1, got %s", issues[0].ID)
		}
	})

	// ---- ReadyIssues ----

	t.Run("ReadyIssues", func(t *testing.T) {
		issues, err := gw.ReadyIssues(ctx, domain.ReadyIssuesQuery{})
		if err != nil {
			t.Fatalf("ReadyIssues: unexpected error: %v", err)
		}

		// bwf-1 is open with no blockers — the only ready issue in the fixture.
		if len(issues) != 1 {
			t.Errorf("ReadyIssues: expected 1 ready issue, got %d: %v", len(issues), issueIDs(issues))
		}

		if bwf1 := findByID(issues, "bwf-1"); bwf1 == nil {
			t.Errorf("ReadyIssues: expected bwf-1 in ready list, got %v", issueIDs(issues))
		}

		// bwf-2 is blocked, bwf-3 is closed — neither should be ready.
		if findByID(issues, "bwf-2") != nil {
			t.Errorf("ReadyIssues: blocked issue bwf-2 should not appear in ready list")
		}
		if findByID(issues, "bwf-3") != nil {
			t.Errorf("ReadyIssues: closed issue bwf-3 should not appear in ready list")
		}
	})

	// ---- BlockedIssues ----

	t.Run("BlockedIssues", func(t *testing.T) {
		views, err := gw.BlockedIssues(ctx, domain.BlockedIssuesQuery{})
		if err != nil {
			t.Fatalf("BlockedIssues: unexpected error: %v", err)
		}

		// Fixture has exactly 1 blocked issue (bwf-2, blocked by bwf-1).
		if len(views) != 1 {
			t.Fatalf("BlockedIssues: expected 1 blocked view, got %d", len(views))
		}

		view := views[0]
		if view.Issue.ID != "bwf-2" {
			t.Errorf("BlockedIssues: expected issue bwf-2, got %s", view.Issue.ID)
		}
		if view.Issue.Status != "blocked" {
			t.Errorf("BlockedIssues: expected status=blocked, got %q", view.Issue.Status)
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

		// Empty query routes through bd list --all → all 3 fixture issues.
		if len(page.Results) != 3 {
			t.Errorf("SearchIssues(empty): expected 3 results, got %d: %v", len(page.Results), searchResultIDs(page.Results))
		}

		expectedIDs := []string{"bwf-1", "bwf-2", "bwf-3"}
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

		// Only bwf-1 has status=open in the fixture.
		for _, issue := range issues {
			if issue.Status != "open" {
				t.Errorf("Query(status=open): expected all results to have status=open, got %s with status=%s", issue.ID, issue.Status)
			}
		}

		if len(issues) != 1 {
			t.Errorf("Query(status=open): expected 1 result, got %d: %v", len(issues), issueIDs(issues))
		}

		if len(issues) > 0 && issues[0].ID != "bwf-1" {
			t.Errorf("Query(status=open): expected bwf-1, got %s", issues[0].ID)
		}
	})

	// ---- CountIssues ----

	t.Run("CountIssues", func(t *testing.T) {
		result, err := gw.CountIssues(ctx, domain.IssueCountQuery{})
		if err != nil {
			t.Fatalf("CountIssues: unexpected error: %v", err)
		}

		// Fixture has 3 issues total (1 open, 1 blocked, 1 closed).
		if result.Total != 3 {
			t.Errorf("CountIssues: expected Total=3, got %d", result.Total)
		}

		// Groups must contain at least the three statuses present in the fixture.
		expectedStatuses := []string{"open", "blocked", "closed"}
		for _, status := range expectedStatuses {
			found := false
			for _, g := range result.Groups {
				if g.Status == status {
					found = true
					if g.Count != 1 {
						t.Errorf("CountIssues: expected count=1 for status=%q, got %d", status, g.Count)
					}
					break
				}
			}
			if !found {
				t.Errorf("CountIssues: expected group for status=%q, groups=%v", status, result.Groups)
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

		// "open" must be present — it's a bd built-in status.
		if !containsStatusName(opts, "open") {
			t.Errorf("StatusCatalog: expected 'open' status option, got %v", statusNames(opts))
		}

		// "blocked" and "closed" are also built-in.
		for _, expected := range []string{"blocked", "closed"} {
			if !containsStatusName(opts, expected) {
				t.Errorf("StatusCatalog: expected %q status option, got %v", expected, statusNames(opts))
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

		// The fixture uses task/bug/chore — all three must be in the catalog.
		for _, expected := range []string{"task", "bug", "chore"} {
			if !containsTypeName(opts, expected) {
				t.Errorf("TypeCatalog: expected type %q, got %v", expected, typeNames(opts))
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
