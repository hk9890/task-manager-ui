package taskmgr

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
)

func ptr[T any](v T) *T { return &v }

func newTestRepo(t *testing.T) (*Repository, *tasks.Store) {
	t.Helper()
	store, err := tasks.Init(t.TempDir(), "tm")
	if err != nil {
		t.Fatalf("tasks.Init: %v", err)
	}
	return New(store, WithAuthor("tester")), store
}

func mustCreate(t *testing.T, r *Repository, in domain.CreateIssueInput) string {
	t.Helper()
	res, err := r.CreateIssue(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateIssue(%q): %v", in.Title, err)
	}
	return res.IssueID
}

func TestCreateAndIssue(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	id := mustCreate(t, r, domain.CreateIssueInput{
		Title: "First task", Description: "body text",
		Type: "bug", Priority: ptr(1), Assignee: "ada", Labels: []string{"area:x"},
	})

	d, err := r.Issue(ctx, id)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if d.Summary.Title != "First task" || d.Summary.Type != "bug" || d.Summary.Priority != 1 {
		t.Errorf("summary mismatch: %+v", d.Summary)
	}
	if d.Summary.Assignee != "ada" {
		t.Errorf("assignee = %q, want ada", d.Summary.Assignee)
	}
	if d.Creator != "tester" {
		t.Errorf("creator = %q, want tester", d.Creator)
	}
	if d.Description != "body text" {
		t.Errorf("description = %q", d.Description)
	}
	if len(d.Summary.Labels) != 1 || d.Summary.Labels[0] != "area:x" {
		t.Errorf("labels = %v", d.Summary.Labels)
	}
}

func TestIssueNotFound(t *testing.T) {
	r, _ := newTestRepo(t)
	_, err := r.Issue(context.Background(), "tm-9999")
	if !errors.Is(err, repository.ErrIssueNotFound) {
		t.Fatalf("Issue unknown id: got %v, want ErrIssueNotFound", err)
	}
}

func TestWriteUnknownIDCommandFailed(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()
	missing := "tm-9999"

	cases := map[string]func() error{
		"UpdateIssue": func() error { return r.UpdateIssue(ctx, missing, domain.UpdateIssueInput{Priority: ptr(1)}) },
		"CloseIssue":  func() error { return r.CloseIssue(ctx, missing, domain.CloseIssueInput{Reason: "x"}) },
		"AddComment":  func() error { return r.AddComment(ctx, missing, domain.AddCommentInput{Body: "hi"}) },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			err := fn()
			var re domain.RepositoryError
			if !errors.As(err, &re) {
				t.Fatalf("got %T (%v), want domain.RepositoryError", err, err)
			}
			if re.Code != domain.ErrorCodeCommandFailed {
				t.Errorf("code = %q, want command_failed", re.Code)
			}
		})
	}
}

func TestCreateEmptyTitleValidation(t *testing.T) {
	r, _ := newTestRepo(t)
	_, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{Title: ""})
	var re domain.RepositoryError
	if !errors.As(err, &re) {
		t.Fatalf("got %T (%v), want domain.RepositoryError", err, err)
	}
	if re.Code != domain.ErrorCodeValidationFailed {
		t.Errorf("code = %q, want validation_failed", re.Code)
	}
}

func TestDashboardSections(t *testing.T) {
	r, store := newTestRepo(t)
	ctx := context.Background()

	idA := mustCreate(t, r, domain.CreateIssueInput{Title: "A open"})
	idB := mustCreate(t, r, domain.CreateIssueInput{Title: "B inprogress"})
	if err := r.UpdateIssue(ctx, idB, domain.UpdateIssueInput{Status: ptr("in_progress")}); err != nil {
		t.Fatalf("update B: %v", err)
	}
	idC := mustCreate(t, r, domain.CreateIssueInput{Title: "C blocked-status"})
	if err := r.UpdateIssue(ctx, idC, domain.UpdateIssueInput{Status: ptr("blocked")}); err != nil {
		t.Fatalf("update C: %v", err)
	}
	idF := mustCreate(t, r, domain.CreateIssueInput{Title: "F closed"})
	if err := r.CloseIssue(ctx, idF, domain.CloseIssueInput{Reason: "done"}); err != nil {
		t.Fatalf("close F: %v", err)
	}
	// Dep-blocked: blocker open, dependent open but gated. Created via the SDK
	// because the Repository create surface does not carry dependency edges.
	blocker, err := store.Create(tasks.CreateInput{Title: "D blocker"})
	if err != nil {
		t.Fatalf("store.Create blocker: %v", err)
	}
	dep, err := store.Create(tasks.CreateInput{Title: "E dep", BlockedBy: []string{blocker.ID}})
	if err != nil {
		t.Fatalf("store.Create dep: %v", err)
	}

	dash, err := r.Dashboard(ctx, repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}

	if !containsID(summaryIDs(dash.ReadyExplain.Ready), idA) {
		t.Errorf("ReadyExplain.Ready missing A: %v", summaryIDs(dash.ReadyExplain.Ready))
	}
	if !containsID(summaryIDs(dash.ReadyExplain.Ready), blocker.ID) {
		t.Errorf("ReadyExplain.Ready missing blocker D: %v", summaryIDs(dash.ReadyExplain.Ready))
	}
	if !containsID(summaryIDs(dash.InProgress), idB) {
		t.Errorf("InProgress missing B: %v", summaryIDs(dash.InProgress))
	}
	if !containsID(summaryIDs(dash.Blocked), idC) {
		t.Errorf("Blocked(status) missing C: %v", summaryIDs(dash.Blocked))
	}
	if !containsID(summaryIDs(dash.Closed), idF) {
		t.Errorf("Closed missing F: %v", summaryIDs(dash.Closed))
	}
	if dash.ClosedTotal != 1 {
		t.Errorf("ClosedTotal = %d, want 1", dash.ClosedTotal)
	}
	blockedIDs := make([]string, 0, len(dash.ReadyExplain.Blocked))
	for _, b := range dash.ReadyExplain.Blocked {
		blockedIDs = append(blockedIDs, b.Issue.ID)
	}
	if !containsID(blockedIDs, dep.ID) {
		t.Errorf("ReadyExplain.Blocked missing dep E: %v", blockedIDs)
	}
}

func TestSearch(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	hit := mustCreate(t, r, domain.CreateIssueInput{Title: "Widget redesign", Description: "more widget detail"})
	_ = mustCreate(t, r, domain.CreateIssueInput{Title: "Unrelated work"})

	page, err := r.Search(ctx, domain.SearchIssuesQuery{Text: "widget"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	ids := searchIDs(page.Results)
	if !containsID(ids, hit) {
		t.Errorf("expected hit %s in %v", hit, ids)
	}
	if len(page.Results) != 1 {
		t.Errorf("expected 1 result, got %d (%v)", len(page.Results), ids)
	}
	if page.Metadata.ReturnedCount != len(page.Results) {
		t.Errorf("ReturnedCount=%d != len=%d", page.Metadata.ReturnedCount, len(page.Results))
	}
	if page.Metadata.Source != domain.SearchResultSourceTaskmgrFind {
		t.Errorf("Source = %q", page.Metadata.Source)
	}
}

func TestUpdateReopenLandsOnRequestedStatus(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	id := mustCreate(t, r, domain.CreateIssueInput{Title: "lifecycle"})
	if err := r.UpdateIssue(ctx, id, domain.UpdateIssueInput{Status: ptr("closed")}); err != nil {
		t.Fatalf("close via update: %v", err)
	}
	d, err := r.Issue(ctx, id)
	if err != nil {
		t.Fatalf("issue after close: %v", err)
	}
	if d.Summary.Status != "closed" {
		t.Fatalf("status after close-via-update = %q, want closed", d.Summary.Status)
	}
	// Reopen onto a non-default status: must land on in_progress, not open.
	if err := r.UpdateIssue(ctx, id, domain.UpdateIssueInput{Status: ptr("in_progress")}); err != nil {
		t.Fatalf("reopen via update: %v", err)
	}
	d, err = r.Issue(ctx, id)
	if err != nil {
		t.Fatalf("issue after reopen: %v", err)
	}
	if d.Summary.Status != "in_progress" {
		t.Errorf("status after reopen = %q, want in_progress", d.Summary.Status)
	}
}

func TestAddCommentAndCatalogs(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	id := mustCreate(t, r, domain.CreateIssueInput{Title: "with comment", Labels: []string{"area:db"}})
	if err := r.AddComment(ctx, id, domain.AddCommentInput{Body: "a note"}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	d, err := r.Issue(ctx, id)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(d.Comments) != 1 || d.Comments[0].Body != "a note" || d.Comments[0].Author != "tester" {
		t.Errorf("comments = %+v", d.Comments)
	}

	cat, err := r.Catalogs(ctx)
	if err != nil {
		t.Fatalf("Catalogs: %v", err)
	}
	if len(cat.Statuses) != len(tasks.Statuses) {
		t.Errorf("statuses = %d, want %d", len(cat.Statuses), len(tasks.Statuses))
	}
	if len(cat.Types) != len(tasks.Types) {
		t.Errorf("types = %d, want %d", len(cat.Types), len(tasks.Types))
	}
	if !containsLabel(cat.Labels, "area:db") {
		t.Errorf("labels missing area:db: %+v", cat.Labels)
	}
}

func TestContextCancellation(t *testing.T) {
	r, _ := newTestRepo(t)
	id := mustCreate(t, r, domain.CreateIssueInput{Title: "x"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ops := map[string]func() error{
		"Dashboard":   func() error { _, e := r.Dashboard(ctx, repository.DashboardOptions{}); return e },
		"Issue":       func() error { _, e := r.Issue(ctx, id); return e },
		"Search":      func() error { _, e := r.Search(ctx, domain.SearchIssuesQuery{}); return e },
		"Catalogs":    func() error { _, e := r.Catalogs(ctx); return e },
		"HealthCheck": func() error { return r.HealthCheck(ctx) },
		"CreateIssue": func() error { _, e := r.CreateIssue(ctx, domain.CreateIssueInput{Title: "y"}); return e },
		"UpdateIssue": func() error { return r.UpdateIssue(ctx, id, domain.UpdateIssueInput{Priority: ptr(1)}) },
		"CloseIssue":  func() error { return r.CloseIssue(ctx, id, domain.CloseIssueInput{}) },
		"AddComment":  func() error { return r.AddComment(ctx, id, domain.AddCommentInput{Body: "z"}) },
	}
	for name, fn := range ops {
		t.Run(name, func(t *testing.T) {
			if err := fn(); !errors.Is(err, context.Canceled) {
				t.Errorf("%s: got %v, want context.Canceled", name, err)
			}
		})
	}
}

func TestDashboardClosedPagingAndOrder(t *testing.T) {
	r, store := newTestRepo(t)
	ctx := context.Background()
	// Deterministic, strictly increasing clock so each close gets a distinct time.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var tick int
	store.SetNow(func() time.Time { tick++; return base.Add(time.Duration(tick) * time.Hour) })

	a := mustCreate(t, r, domain.CreateIssueInput{Title: "alpha"})
	b := mustCreate(t, r, domain.CreateIssueInput{Title: "bravo"})
	c := mustCreate(t, r, domain.CreateIssueInput{Title: "charlie"})
	// Close in order a, b, c → c has the newest close time.
	for _, id := range []string{a, b, c} {
		if err := r.CloseIssue(ctx, id, domain.CloseIssueInput{Reason: "done"}); err != nil {
			t.Fatalf("close %s: %v", id, err)
		}
	}

	// Default window: newest-closed first.
	dash, err := r.Dashboard(ctx, repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if got := summaryIDs(dash.Closed); !equalSlice(got, []string{c, b, a}) {
		t.Errorf("Closed order = %v, want newest-first %v", got, []string{c, b, a})
	}
	if dash.ClosedTotal != 3 {
		t.Errorf("ClosedTotal = %d, want 3", dash.ClosedTotal)
	}

	// Limit 1 → only the newest; total still 3.
	dash, _ = r.Dashboard(ctx, repository.DashboardOptions{ClosedLimit: 1})
	if got := summaryIDs(dash.Closed); !equalSlice(got, []string{c}) {
		t.Errorf("ClosedLimit:1 = %v, want %v", got, []string{c})
	}
	if dash.ClosedTotal != 3 {
		t.Errorf("ClosedTotal with limit = %d, want 3", dash.ClosedTotal)
	}

	// Offset 1 → skip the newest.
	dash, _ = r.Dashboard(ctx, repository.DashboardOptions{ClosedOffset: 1})
	if got := summaryIDs(dash.Closed); !equalSlice(got, []string{b, a}) {
		t.Errorf("ClosedOffset:1 = %v, want %v", got, []string{b, a})
	}

	// Offset past the end → empty window, full total.
	dash, _ = r.Dashboard(ctx, repository.DashboardOptions{ClosedOffset: 5})
	if len(dash.Closed) != 0 {
		t.Errorf("ClosedOffset:5 window = %v, want empty", summaryIDs(dash.Closed))
	}
	if dash.ClosedTotal != 3 {
		t.Errorf("ClosedTotal beyond end = %d, want 3", dash.ClosedTotal)
	}
}

func TestCreateDefaultPriority(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()
	id := mustCreate(t, r, domain.CreateIssueInput{Title: "no priority"}) // Priority nil
	d, err := r.Issue(ctx, id)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if d.Summary.Priority != tasks.PriorityDefault {
		t.Errorf("default priority = %d, want PriorityDefault=%d", d.Summary.Priority, tasks.PriorityDefault)
	}
}

func TestSearchLabelMatchAll(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()
	both := mustCreate(t, r, domain.CreateIssueInput{Title: "both labels", Labels: []string{"x", "y"}})
	_ = mustCreate(t, r, domain.CreateIssueInput{Title: "one label", Labels: []string{"x"}})

	page, err := r.Search(ctx, domain.SearchIssuesQuery{Labels: []string{"x", "y"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if ids := searchIDs(page.Results); !equalSlice(ids, []string{both}) {
		t.Errorf("Labels[x,y] (AND) = %v, want only %v", ids, []string{both})
	}
}

func TestUpdateClosedFieldOnlyConflict(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()
	id := mustCreate(t, r, domain.CreateIssueInput{Title: "to close"})
	if err := r.CloseIssue(ctx, id, domain.CloseIssueInput{Reason: "done"}); err != nil {
		t.Fatalf("close: %v", err)
	}
	// A field-only update (no status change) on a closed issue is immutable.
	err := r.UpdateIssue(ctx, id, domain.UpdateIssueInput{Priority: ptr(0)})
	var re domain.RepositoryError
	if !errors.As(err, &re) || re.Code != domain.ErrorCodeConflict {
		t.Fatalf("field edit on closed issue: got %v, want conflict", err)
	}
}

func TestCreateWhitespaceTitleValidation(t *testing.T) {
	r, _ := newTestRepo(t)
	_, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "   "})
	var re domain.RepositoryError
	if !errors.As(err, &re) || re.Code != domain.ErrorCodeValidationFailed {
		t.Fatalf("whitespace title: got %v, want validation_failed", err)
	}
}

func TestEmptyStoreNonNilSlices(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()

	dash, err := r.Dashboard(ctx, repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dash.ReadyExplain.Ready == nil || dash.ReadyExplain.Blocked == nil ||
		dash.InProgress == nil || dash.Closed == nil || dash.Blocked == nil {
		t.Errorf("empty-store Dashboard has nil slice(s): %+v", dash)
	}
	if dash.ClosedTotal != 0 {
		t.Errorf("ClosedTotal = %d, want 0", dash.ClosedTotal)
	}

	page, err := r.Search(ctx, domain.SearchIssuesQuery{Text: "anything"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if page.Results == nil {
		t.Errorf("empty-store Search.Results is nil, want non-nil")
	}
}

func TestSearchInvalidStatusFilter(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := context.Background()
	mustCreate(t, r, domain.CreateIssueInput{Title: "open issue"})

	// Only-unknown status values must match nothing — not widen to all statuses.
	page, err := r.Search(ctx, domain.SearchIssuesQuery{Statuses: []string{"bogus"}})
	if err != nil {
		t.Fatalf("Search(all-invalid): %v", err)
	}
	if len(page.Results) != 0 {
		t.Errorf("all-invalid status filter returned %d results, want 0", len(page.Results))
	}

	// A mix keeps the valid values and drops the unknown one.
	page, err = r.Search(ctx, domain.SearchIssuesQuery{Statuses: []string{"open", "bogus"}})
	if err != nil {
		t.Fatalf("Search(mixed): %v", err)
	}
	if len(page.Results) != 1 {
		t.Errorf("[open,bogus] returned %d results, want 1", len(page.Results))
	}
}

// -- helpers --

func summaryIDs(s []domain.IssueSummary) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		out = append(out, x.ID)
	}
	return out
}

func searchIDs(s []domain.SearchResult) []string {
	out := make([]string, 0, len(s))
	for _, x := range s {
		out = append(out, x.Issue.ID)
	}
	return out
}

func equalSlice(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func containsLabel(labels []domain.LabelOption, want string) bool {
	for _, l := range labels {
		if l.Name == want {
			return true
		}
	}
	return false
}
