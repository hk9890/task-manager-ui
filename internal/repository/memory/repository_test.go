package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

// ---- helpers ----

func staticClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func seqIDs() func() string {
	var mu sync.Mutex
	n := 0
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		n++
		return "gen-" + itoa(n)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 8)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func ptr[T any](v T) *T { return &v }

// newRepo creates a Repository wired with a fixed clock and sequential IDs.
func newRepo(base time.Time) *memory.Repository {
	return memory.New(memory.WithClock(staticClock(base)), memory.WithIDGenerator(seqIDs()))
}

// ---- HealthCheck ----

func TestHealthCheck_AlwaysNil(t *testing.T) {
	r := memory.New()
	if err := r.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: expected nil, got %v", err)
	}
}

func TestHealthCheck_CancelledContext(t *testing.T) {
	r := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.HealthCheck(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("HealthCheck: expected context.Canceled, got %v", err)
	}
}

// ---- Catalogs ----

func TestCatalogs_DefaultsPresent(t *testing.T) {
	r := memory.New()
	cats, err := r.Catalogs(context.Background())
	if err != nil {
		t.Fatalf("Catalogs: unexpected error %v", err)
	}
	if len(cats.Statuses) == 0 {
		t.Fatal("Catalogs: expected non-empty statuses")
	}
	if len(cats.Types) == 0 {
		t.Fatal("Catalogs: expected non-empty types")
	}
	// Verify known statuses present.
	statusNames := make(map[string]bool)
	for _, s := range cats.Statuses {
		statusNames[s.Name] = true
	}
	for _, want := range []string{"open", "in_progress", "blocked", "closed"} {
		if !statusNames[want] {
			t.Errorf("Catalogs: expected status %q not found", want)
		}
	}
}

func TestCatalogs_Seeded(t *testing.T) {
	r := memory.New()
	custom := repository.Catalogs{
		Statuses: []domain.StatusOption{{Name: "custom-status"}},
		Types:    []domain.TypeOption{{Name: "custom-type"}},
		Labels:   []domain.LabelOption{{Name: "custom-label"}},
	}
	r.SeedCatalogs(custom)

	cats, err := r.Catalogs(context.Background())
	if err != nil {
		t.Fatalf("Catalogs: unexpected error %v", err)
	}
	if len(cats.Statuses) != 1 || cats.Statuses[0].Name != "custom-status" {
		t.Errorf("Catalogs: expected seeded status, got %v", cats.Statuses)
	}
}

// ---- Empty store ----

func TestEmptyStore_Dashboard(t *testing.T) {
	r := memory.New()
	d, err := r.Dashboard(context.Background(), repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: unexpected error %v", err)
	}
	if d.ReadyExplain.Ready == nil {
		t.Error("Dashboard: ReadyExplain.Ready must not be nil")
	}
	if d.ReadyExplain.Blocked == nil {
		t.Error("Dashboard: ReadyExplain.Blocked must not be nil")
	}
	if d.InProgress == nil {
		t.Error("Dashboard: InProgress must not be nil")
	}
	if d.Closed == nil {
		t.Error("Dashboard: Closed must not be nil")
	}
	if d.Blocked == nil {
		t.Error("Dashboard: Blocked must not be nil")
	}
	if d.ClosedTotal != 0 {
		t.Errorf("Dashboard: ClosedTotal want 0, got %d", d.ClosedTotal)
	}
}

func TestEmptyStore_Search(t *testing.T) {
	r := memory.New()
	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{Text: "anything"})
	if err != nil {
		t.Fatalf("Search: unexpected error %v", err)
	}
	if len(page.Results) != 0 {
		t.Errorf("Search: expected empty results, got %d", len(page.Results))
	}
	if page.Results == nil {
		t.Error("Search: Results must not be nil")
	}
}

func TestEmptyStore_Issue_NotFound(t *testing.T) {
	r := memory.New()
	_, err := r.Issue(context.Background(), "nonexistent")
	if !errors.Is(err, repository.ErrIssueNotFound) {
		t.Errorf("Issue: expected ErrIssueNotFound, got %v", err)
	}
}

// ---- Seed + Issue round-trip ----

func TestIssue_RoundTrip(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := newRepo(base)

	r.Seed(memory.Issue{
		ID:          "bd-1",
		Title:       "My issue",
		Status:      "open",
		Type:        "task",
		Priority:    2,
		Assignee:    "alice",
		Labels:      []string{"backend"},
		Description: "desc",
		Notes:       "notes",
	})

	detail, err := r.Issue(context.Background(), "bd-1")
	if err != nil {
		t.Fatalf("Issue: unexpected error %v", err)
	}
	if detail.Summary.ID != "bd-1" {
		t.Errorf("ID: want bd-1, got %s", detail.Summary.ID)
	}
	if detail.Summary.Title != "My issue" {
		t.Errorf("Title: want My issue, got %s", detail.Summary.Title)
	}
	if detail.Summary.Status != "open" {
		t.Errorf("Status: want open, got %s", detail.Summary.Status)
	}
	if detail.Summary.Assignee != "alice" {
		t.Errorf("Assignee: want alice, got %s", detail.Summary.Assignee)
	}
	if len(detail.Summary.Labels) != 1 || detail.Summary.Labels[0] != "backend" {
		t.Errorf("Labels: want [backend], got %v", detail.Summary.Labels)
	}
	if detail.Description != "desc" {
		t.Errorf("Description: want desc, got %s", detail.Description)
	}
	if detail.Notes != "notes" {
		t.Errorf("Notes: want notes, got %s", detail.Notes)
	}
}

func TestIssue_DefaultStatusAndType(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "x", Title: "test"})

	detail, err := r.Issue(context.Background(), "x")
	if err != nil {
		t.Fatalf("Issue: unexpected error %v", err)
	}
	if detail.Summary.Status != "open" {
		t.Errorf("Status: want open, got %s", detail.Summary.Status)
	}
	if detail.Summary.Type != "task" {
		t.Errorf("Type: want task, got %s", detail.Summary.Type)
	}
}

// ---- Comments ----

func TestSeedComments(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	r := newRepo(base)
	r.Seed(memory.Issue{ID: "bd-1", Title: "issue"})

	r.SeedComments("bd-1",
		memory.Comment{Author: "alice", Body: "hello"},
		memory.Comment{Author: "bob", Body: "world"},
	)

	detail, err := r.Issue(context.Background(), "bd-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(detail.Comments) != 2 {
		t.Fatalf("Comments: want 2, got %d", len(detail.Comments))
	}
	if detail.Comments[0].Author != "alice" || detail.Comments[0].Body != "hello" {
		t.Errorf("Comments[0]: got author=%q body=%q", detail.Comments[0].Author, detail.Comments[0].Body)
	}
	if detail.Comments[1].Author != "bob" {
		t.Errorf("Comments[1] author: want bob, got %s", detail.Comments[1].Author)
	}
}

// ---- CreateIssue ----

func TestCreateIssue_HappyPath(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	r := newRepo(base)

	result, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{
		Title:    "New task",
		Type:     "bug",
		Priority: ptr(3),
		Labels:   []string{"backend"},
	})
	if err != nil {
		t.Fatalf("CreateIssue: unexpected error %v", err)
	}
	if result.IssueID == "" {
		t.Fatal("CreateIssue: expected non-empty ID")
	}

	detail, err := r.Issue(context.Background(), result.IssueID)
	if err != nil {
		t.Fatalf("Issue after create: %v", err)
	}
	if detail.Summary.Title != "New task" {
		t.Errorf("Title: want New task, got %s", detail.Summary.Title)
	}
	if detail.Summary.Status != "open" {
		t.Errorf("Status: want open, got %s", detail.Summary.Status)
	}
	if detail.Summary.Type != "bug" {
		t.Errorf("Type: want bug, got %s", detail.Summary.Type)
	}
	if detail.Summary.Priority != 3 {
		t.Errorf("Priority: want 3, got %d", detail.Summary.Priority)
	}
	if len(detail.Summary.Labels) != 1 || detail.Summary.Labels[0] != "backend" {
		t.Errorf("Labels: want [backend], got %v", detail.Summary.Labels)
	}
}

func TestCreateIssue_EmptyTitle(t *testing.T) {
	r := memory.New()
	_, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{Title: ""})
	if err == nil {
		t.Fatal("CreateIssue: expected error for empty title")
	}
	var ge domain.RepositoryError
	if !errors.As(err, &ge) || ge.Code != domain.ErrorCodeValidationFailed {
		t.Errorf("CreateIssue: expected ErrorCodeValidationFailed, got %v", err)
	}
}

func TestCreateIssue_IDGeneration(t *testing.T) {
	ids := make([]string, 0)
	idgen := seqIDs()
	r := memory.New(memory.WithIDGenerator(idgen))

	for i := 0; i < 3; i++ {
		res, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "x"})
		if err != nil {
			t.Fatalf("CreateIssue[%d]: %v", i, err)
		}
		for _, prev := range ids {
			if prev == res.IssueID {
				t.Errorf("CreateIssue: duplicate ID %q", res.IssueID)
			}
		}
		ids = append(ids, res.IssueID)
	}
}

func TestCreateIssue_DefaultTypeIsTask(t *testing.T) {
	r := memory.New()
	res, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "no-type"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	detail, _ := r.Issue(context.Background(), res.IssueID)
	if detail.Summary.Type != "task" {
		t.Errorf("Type: want task, got %s", detail.Summary.Type)
	}
}

// ---- UpdateIssue ----

func TestUpdateIssue_MergeFields(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tick := base.Add(time.Hour)
	clk := func() time.Time { return tick }

	r := memory.New(memory.WithClock(func() time.Time { return base }))
	r.Seed(memory.Issue{
		ID:     "bd-2",
		Title:  "Old title",
		Status: "open",
		Labels: []string{"old"},
	})

	// Advance the clock before update.
	r2 := memory.New(memory.WithClock(clk))
	r2.Seed(memory.Issue{ID: "bd-2", Title: "Old title", Status: "open", Labels: []string{"old"}})

	err := r2.UpdateIssue(context.Background(), "bd-2", domain.UpdateIssueInput{
		Title:  ptr("New title"),
		Status: ptr("in_progress"),
		Labels: []string{"new"},
	})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	detail, err := r2.Issue(context.Background(), "bd-2")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if detail.Summary.Title != "New title" {
		t.Errorf("Title: want New title, got %s", detail.Summary.Title)
	}
	if detail.Summary.Status != "in_progress" {
		t.Errorf("Status: want in_progress, got %s", detail.Summary.Status)
	}
	if len(detail.Summary.Labels) != 1 || detail.Summary.Labels[0] != "new" {
		t.Errorf("Labels: want [new], got %v", detail.Summary.Labels)
	}
	if !detail.Summary.UpdatedAt.Equal(tick) {
		t.Errorf("UpdatedAt: want %v, got %v", tick, detail.Summary.UpdatedAt)
	}
}

func TestUpdateIssue_ClearLabels(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "x", Title: "t", Labels: []string{"a", "b"}})

	if err := r.UpdateIssue(context.Background(), "x", domain.UpdateIssueInput{ClearLabels: true}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	detail, _ := r.Issue(context.Background(), "x")
	if len(detail.Summary.Labels) != 0 {
		t.Errorf("Labels after clear: want [], got %v", detail.Summary.Labels)
	}
}

func TestUpdateIssue_NonExistent(t *testing.T) {
	r := memory.New()
	err := r.UpdateIssue(context.Background(), "nonexistent", domain.UpdateIssueInput{})
	if err == nil {
		t.Fatal("UpdateIssue: expected error for unknown ID")
	}
	var ge domain.RepositoryError
	if !errors.As(err, &ge) || ge.Code != domain.ErrorCodeCommandFailed {
		t.Errorf("UpdateIssue: expected ErrorCodeCommandFailed, got %v", err)
	}
}

func TestUpdateIssue_NilFieldsPreserved(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "bd-3", Title: "Keep title", Status: "open", Priority: 5})

	// Update only description; title/status/priority must be unchanged.
	if err := r.UpdateIssue(context.Background(), "bd-3", domain.UpdateIssueInput{
		Description: ptr("new desc"),
	}); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	detail, _ := r.Issue(context.Background(), "bd-3")
	if detail.Summary.Title != "Keep title" {
		t.Errorf("Title: want Keep title, got %s", detail.Summary.Title)
	}
	if detail.Summary.Status != "open" {
		t.Errorf("Status: want open, got %s", detail.Summary.Status)
	}
	if detail.Summary.Priority != 5 {
		t.Errorf("Priority: want 5, got %d", detail.Summary.Priority)
	}
	if detail.Description != "new desc" {
		t.Errorf("Description: want 'new desc', got %s", detail.Description)
	}
}

// ---- CloseIssue ----

func TestCloseIssue_HappyPath(t *testing.T) {
	closedAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	r := memory.New(memory.WithClock(staticClock(closedAt)))
	r.Seed(memory.Issue{ID: "bd-4", Title: "t", Status: "open"})

	err := r.CloseIssue(context.Background(), "bd-4", domain.CloseIssueInput{Reason: "done"})
	if err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}

	detail, err := r.Issue(context.Background(), "bd-4")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if detail.Summary.Status != "closed" {
		t.Errorf("Status: want closed, got %s", detail.Summary.Status)
	}
	if detail.CloseReason != "done" {
		t.Errorf("CloseReason: want done, got %s", detail.CloseReason)
	}
	if !detail.ClosedAt.Equal(closedAt) {
		t.Errorf("ClosedAt: want %v, got %v", closedAt, detail.ClosedAt)
	}
	if !detail.Summary.UpdatedAt.Equal(closedAt) {
		t.Errorf("UpdatedAt: want %v, got %v", closedAt, detail.Summary.UpdatedAt)
	}
}

func TestCloseIssue_DefaultReason(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "bd-5", Title: "t"})
	_ = r.CloseIssue(context.Background(), "bd-5", domain.CloseIssueInput{})

	detail, _ := r.Issue(context.Background(), "bd-5")
	if detail.CloseReason != "Closed" {
		t.Errorf("CloseReason: want Closed, got %s", detail.CloseReason)
	}
}

func TestCloseIssue_NonExistent(t *testing.T) {
	r := memory.New()
	err := r.CloseIssue(context.Background(), "nope", domain.CloseIssueInput{})
	if err == nil {
		t.Fatal("CloseIssue: expected error for unknown ID")
	}
	var ge domain.RepositoryError
	if !errors.As(err, &ge) || ge.Code != domain.ErrorCodeCommandFailed {
		t.Errorf("CloseIssue: expected ErrorCodeCommandFailed, got %v", err)
	}
}

// ---- AddComment ----

func TestAddComment_HappyPath(t *testing.T) {
	base := time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)
	r := newRepo(base)
	r.Seed(memory.Issue{ID: "bd-6", Title: "t"})

	err := r.AddComment(context.Background(), "bd-6", domain.AddCommentInput{Body: "first comment"})
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	detail, _ := r.Issue(context.Background(), "bd-6")
	if len(detail.Comments) != 1 {
		t.Fatalf("Comments: want 1, got %d", len(detail.Comments))
	}
	if detail.Comments[0].Body != "first comment" {
		t.Errorf("Comment body: want 'first comment', got %s", detail.Comments[0].Body)
	}
	if detail.Comments[0].Author != "memory-user" {
		t.Errorf("Comment author: want memory-user, got %s", detail.Comments[0].Author)
	}
}

func TestAddComment_BumpsUpdated(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	comment := base.Add(time.Hour)
	calls := 0
	clk := func() time.Time {
		calls++
		if calls == 1 {
			return base
		}
		return comment
	}
	r := memory.New(memory.WithClock(clk))
	r.Seed(memory.Issue{ID: "x", Title: "t", Created: base, Updated: base})

	_ = r.AddComment(context.Background(), "x", domain.AddCommentInput{Body: "b"})

	detail, _ := r.Issue(context.Background(), "x")
	if !detail.Summary.UpdatedAt.Equal(comment) {
		t.Errorf("UpdatedAt: want %v, got %v", comment, detail.Summary.UpdatedAt)
	}
}

func TestAddComment_NonExistent(t *testing.T) {
	r := memory.New()
	err := r.AddComment(context.Background(), "nope", domain.AddCommentInput{Body: "b"})
	if err == nil {
		t.Fatal("AddComment: expected error for unknown ID")
	}
	var ge domain.RepositoryError
	if !errors.As(err, &ge) || ge.Code != domain.ErrorCodeCommandFailed {
		t.Errorf("AddComment: expected ErrorCodeCommandFailed, got %v", err)
	}
}

// ---- Dashboard semantics ----

func TestDashboard_ReadyVsBlocked(t *testing.T) {
	r := memory.New()

	// dep closed → bd-open is ready.
	r.Seed(memory.Issue{ID: "dep-1", Status: "closed"})
	r.Seed(memory.Issue{ID: "bd-open", Status: "open", DependsOn: []string{"dep-1"}})

	// dep open → bd-waiting is dep-blocked.
	r.Seed(memory.Issue{ID: "dep-2", Status: "open"})
	r.Seed(memory.Issue{ID: "bd-waiting", Status: "open", DependsOn: []string{"dep-2"}})

	// No deps → bd-nodep is ready.
	r.Seed(memory.Issue{ID: "bd-nodep", Status: "open"})

	// Status == "blocked" (stored, no deps) → in DashboardData.Blocked but NOT in
	// ReadyExplain.Ready. bd ready --explain only includes status=open issues in
	// the ready set (per bd 1.0.4 "bd ready --help" and contract test bwf-5).
	r.Seed(memory.Issue{ID: "bd-status-blocked", Status: "blocked"})

	d, err := r.Dashboard(context.Background(), repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}

	readyIDs := issueIDs(d.ReadyExplain.Ready)
	if !containsID(readyIDs, "bd-open") {
		t.Errorf("ReadyExplain.Ready: expected bd-open, got %v", readyIDs)
	}
	if !containsID(readyIDs, "bd-nodep") {
		t.Errorf("ReadyExplain.Ready: expected bd-nodep, got %v", readyIDs)
	}
	// dep-blocked issue should not be ready.
	if containsID(readyIDs, "bd-waiting") {
		t.Errorf("ReadyExplain.Ready: bd-waiting should not be ready, got %v", readyIDs)
	}
	// bd-status-blocked has stored status="blocked" with no deps. bd ready explicitly
	// excludes non-open-status issues from Ready (per interface.go postcondition).
	if containsID(readyIDs, "bd-status-blocked") {
		t.Errorf("ReadyExplain.Ready: bd-status-blocked (stored-blocked) must NOT be in Ready, got %v", readyIDs)
	}

	// bd-waiting should be in ReadyExplain.Blocked.
	blockedIDs := blockedViewIDs(d.ReadyExplain.Blocked)
	if !containsID(blockedIDs, "bd-waiting") {
		t.Errorf("ReadyExplain.Blocked: expected bd-waiting, got %v", blockedIDs)
	}

	// DashboardData.Blocked should only contain status=="blocked" issues.
	blockedSummaryIDs := issueIDs(d.Blocked)
	if !containsID(blockedSummaryIDs, "bd-status-blocked") {
		t.Errorf("Blocked: expected bd-status-blocked, got %v", blockedSummaryIDs)
	}
	// bd-waiting has status "open", not "blocked".
	if containsID(blockedSummaryIDs, "bd-waiting") {
		t.Errorf("Blocked: bd-waiting (status=open) should not be in Blocked, got %v", blockedSummaryIDs)
	}
}

func TestDashboard_ClosedSortedDesc(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(24 * time.Hour)
	t3 := t2.Add(24 * time.Hour)

	r := memory.New()
	r.Seed(memory.Issue{ID: "c1", Status: "closed", Updated: t1})
	r.Seed(memory.Issue{ID: "c2", Status: "closed", Updated: t2})
	r.Seed(memory.Issue{ID: "c3", Status: "closed", Updated: t3})

	// Use CloseIssue to set ClosedAt with a controllable clock per issue.
	// Instead, seed with a custom approach by closing via CloseIssue.
	// Use three separate repos to get distinct ClosedAt timestamps.
	r2 := memory.New()
	clks := []time.Time{t3, t1, t2}
	clkIdx := 0
	clkFn := func() time.Time {
		t := clks[clkIdx]
		clkIdx++
		return t
	}
	r3 := memory.New(memory.WithClock(clkFn))
	r3.Seed(memory.Issue{ID: "c1", Status: "open"})
	r3.Seed(memory.Issue{ID: "c2", Status: "open"})
	r3.Seed(memory.Issue{ID: "c3", Status: "open"})
	// Close in order: c1 at t3, c2 at t1, c3 at t2.
	_ = r2

	// Use direct Seed with closed status — but we need ClosedAt set.
	// To test sort order properly, use a controlled scenario via direct seeding
	// by setting Created time as a proxy for close order then verifying.
	// Actually: the sort is on storedIssue.closed. Let's use CloseIssue with a
	// ticking clock.
	rFinal := memory.New()
	ticks := []time.Time{t3, t1, t2}
	idx := 0
	tFn := func() time.Time {
		v := ticks[idx%len(ticks)]
		idx++
		return v
	}
	rSort := memory.New(memory.WithClock(tFn))
	rSort.Seed(memory.Issue{ID: "s1", Status: "open"})
	rSort.Seed(memory.Issue{ID: "s2", Status: "open"})
	rSort.Seed(memory.Issue{ID: "s3", Status: "open"})

	_ = rSort.CloseIssue(context.Background(), "s1", domain.CloseIssueInput{}) // ClosedAt = t3
	_ = rSort.CloseIssue(context.Background(), "s2", domain.CloseIssueInput{}) // ClosedAt = t1
	_ = rSort.CloseIssue(context.Background(), "s3", domain.CloseIssueInput{}) // ClosedAt = t2

	_ = rFinal

	d, err := rSort.Dashboard(context.Background(), repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if len(d.Closed) != 3 {
		t.Fatalf("Closed: want 3, got %d", len(d.Closed))
	}

	// Expected order DESC: s1 (t3), s3 (t2), s2 (t1).
	wantOrder := []string{"s1", "s3", "s2"}
	for i, want := range wantOrder {
		if d.Closed[i].ID != want {
			t.Errorf("Closed[%d]: want %s, got %s", i, want, d.Closed[i].ID)
		}
	}
}

func TestDashboard_InProgress(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "ip-1", Status: "in_progress"})
	r.Seed(memory.Issue{ID: "open-1", Status: "open"})

	d, err := r.Dashboard(context.Background(), repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}

	ipIDs := issueIDs(d.InProgress)
	if !containsID(ipIDs, "ip-1") {
		t.Errorf("InProgress: expected ip-1, got %v", ipIDs)
	}
	if containsID(ipIDs, "open-1") {
		t.Errorf("InProgress: open-1 should not be in_progress, got %v", ipIDs)
	}
}

func TestDashboard_ClosedTotal(t *testing.T) {
	r := memory.New()
	for i := 0; i < 5; i++ {
		id := "c" + itoa(i)
		r.Seed(memory.Issue{ID: id, Status: "closed"})
	}
	r.Seed(memory.Issue{ID: "open-x", Status: "open"})

	d, err := r.Dashboard(context.Background(), repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.ClosedTotal != 5 {
		t.Errorf("ClosedTotal: want 5, got %d", d.ClosedTotal)
	}
}

// ---- Dashboard ClosedOffset pagination ----

// TestDashboard_ClosedOffset_Pages seeds 100 closed issues with distinct
// ClosedAt timestamps (newest first when sorted DESC), then asserts:
//   - Page 0 (offset=0, limit=40) returns the 40 newest issues.
//   - Page 1 (offset=40, limit=40) returns the next 40 issues.
//   - The two pages have no overlap and together form a contiguous ClosedAt DESC
//     sequence.
//   - ClosedTotal reflects the full 100, independent of the page window.
func TestDashboard_ClosedOffset_Pages(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	r := memory.New()
	const total = 100
	// Seed 100 closed issues. Issue i gets ClosedAt = base + i*hour so that
	// issue 99 is newest (highest ClosedAt) and issue 0 is oldest.
	for i := 0; i < total; i++ {
		id := "cl-" + itoa(i)
		r.Seed(memory.Issue{ID: id, Status: "closed"})
		r.SeedClosed(id, base.Add(time.Duration(i)*time.Hour), "done")
	}

	// Page 0: offset=0, limit=40 → newest 40 issues (indices 99..60 by ClosedAt DESC).
	page0, err := r.Dashboard(context.Background(), repository.DashboardOptions{
		ClosedOffset: 0,
		ClosedLimit:  40,
	})
	if err != nil {
		t.Fatalf("Dashboard page0: %v", err)
	}
	if page0.ClosedTotal != total {
		t.Errorf("page0 ClosedTotal: want %d, got %d", total, page0.ClosedTotal)
	}
	if len(page0.Closed) != 40 {
		t.Fatalf("page0 Closed len: want 40, got %d", len(page0.Closed))
	}

	// Page 1: offset=40, limit=40 → next 40 issues (indices 59..20 by ClosedAt DESC).
	page1, err := r.Dashboard(context.Background(), repository.DashboardOptions{
		ClosedOffset: 40,
		ClosedLimit:  40,
	})
	if err != nil {
		t.Fatalf("Dashboard page1: %v", err)
	}
	if page1.ClosedTotal != total {
		t.Errorf("page1 ClosedTotal: want %d, got %d", total, page1.ClosedTotal)
	}
	if len(page1.Closed) != 40 {
		t.Fatalf("page1 Closed len: want 40, got %d", len(page1.Closed))
	}

	// No overlap between the two pages.
	page0IDs := make(map[string]struct{}, 40)
	for _, s := range page0.Closed {
		page0IDs[s.ID] = struct{}{}
	}
	for _, s := range page1.Closed {
		if _, dup := page0IDs[s.ID]; dup {
			t.Errorf("overlap: issue %q appears in both page0 and page1", s.ID)
		}
	}

	// Verify ClosedAt DESC ordering within and across pages.
	// Because issue "cl-i" was closed at base+i*hour, the DESC order is
	// cl-99, cl-98, ..., cl-0. We recover the numeric suffix and verify it
	// is strictly decreasing across the combined slice.
	combined := append(page0.Closed, page1.Closed...)
	for i := 1; i < len(combined); i++ {
		prevN := closedOffsetIssueIndex(combined[i-1].ID)
		currN := closedOffsetIssueIndex(combined[i].ID)
		if prevN <= currN {
			t.Errorf("ClosedAt not DESC at combined[%d]→[%d]: index %d <= %d (IDs: %s, %s)",
				i-1, i, prevN, currN, combined[i-1].ID, combined[i].ID)
		}
	}
}

// TestDashboard_ClosedOffset_BeyondEnd asserts that an offset beyond the last
// closed issue returns an empty slice (not an error) and that ClosedTotal still
// reflects the full count.
func TestDashboard_ClosedOffset_BeyondEnd(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := memory.New()
	const total = 100
	for i := 0; i < total; i++ {
		id := "cl-" + itoa(i)
		r.Seed(memory.Issue{ID: id, Status: "closed"})
		r.SeedClosed(id, base.Add(time.Duration(i)*time.Hour), "done")
	}

	data, err := r.Dashboard(context.Background(), repository.DashboardOptions{
		ClosedOffset: 200,
		ClosedLimit:  40,
	})
	if err != nil {
		t.Fatalf("Dashboard offset=200: unexpected error %v", err)
	}
	if len(data.Closed) != 0 {
		t.Errorf("Closed len: want 0, got %d", len(data.Closed))
	}
	if data.Closed == nil {
		t.Error("Closed must not be nil (empty slice required)")
	}
	if data.ClosedTotal != total {
		t.Errorf("ClosedTotal: want %d, got %d", total, data.ClosedTotal)
	}
}

// ---- Search ----

func TestSearch_TextFilter(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "a", Title: "Fix the Repository bug", Status: "open"})
	r.Seed(memory.Issue{ID: "b", Title: "Unrelated task", Status: "open"})
	r.Seed(memory.Issue{ID: "c", Title: "nothing", Description: "REPOSITORY mentioned here", Status: "open"})

	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{Text: "repository"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(page.Results) != 2 {
		t.Errorf("Search: want 2 results, got %d", len(page.Results))
	}
	ids := searchResultIDs(page.Results)
	if !containsID(ids, "a") || !containsID(ids, "c") {
		t.Errorf("Search: expected a and c, got %v", ids)
	}
}

func TestSearch_TextInNotes(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "n1", Title: "boring", Notes: "secret keyword hidden", Status: "open"})
	r.Seed(memory.Issue{ID: "n2", Title: "boring", Notes: "nothing here", Status: "open"})

	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{Text: "secret keyword"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(page.Results) != 1 || page.Results[0].Issue.ID != "n1" {
		t.Errorf("Search: expected n1, got %v", searchResultIDs(page.Results))
	}
}

func TestSearch_StatusFilter(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "open-1", Status: "open"})
	r.Seed(memory.Issue{ID: "closed-1", Status: "closed"})
	r.Seed(memory.Issue{ID: "ip-1", Status: "in_progress"})

	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{
		Statuses: []string{"open", "in_progress"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	ids := searchResultIDs(page.Results)
	if containsID(ids, "closed-1") {
		t.Errorf("Search: closed-1 should be excluded, got %v", ids)
	}
	if !containsID(ids, "open-1") || !containsID(ids, "ip-1") {
		t.Errorf("Search: open-1 and ip-1 should be included, got %v", ids)
	}
}

func TestSearch_TypeFilter(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "t1", Type: "bug", Status: "open"})
	r.Seed(memory.Issue{ID: "t2", Type: "task", Status: "open"})

	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{Types: []string{"bug"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	ids := searchResultIDs(page.Results)
	if len(ids) != 1 || ids[0] != "t1" {
		t.Errorf("Search: want [t1], got %v", ids)
	}
}

func TestSearch_LabelFilter(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "l1", Labels: []string{"backend", "ci"}, Status: "open"})
	r.Seed(memory.Issue{ID: "l2", Labels: []string{"backend"}, Status: "open"})
	r.Seed(memory.Issue{ID: "l3", Labels: []string{"ci"}, Status: "open"})

	// Must have BOTH labels.
	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{Labels: []string{"backend", "ci"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	ids := searchResultIDs(page.Results)
	if len(ids) != 1 || ids[0] != "l1" {
		t.Errorf("Search: want [l1], got %v", ids)
	}
}

func TestSearch_AssigneeFilter(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "a1", Assignee: "alice", Status: "open"})
	r.Seed(memory.Issue{ID: "a2", Assignee: "bob", Status: "open"})

	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{Assignee: "alice"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	ids := searchResultIDs(page.Results)
	if len(ids) != 1 || ids[0] != "a1" {
		t.Errorf("Search: want [a1], got %v", ids)
	}
}

func TestSearch_PriorityBounds(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "p1", Priority: 1, Status: "open"})
	r.Seed(memory.Issue{ID: "p3", Priority: 3, Status: "open"})
	r.Seed(memory.Issue{ID: "p5", Priority: 5, Status: "open"})

	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{
		PriorityMin: ptr(2),
		PriorityMax: ptr(4),
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	ids := searchResultIDs(page.Results)
	if len(ids) != 1 || ids[0] != "p3" {
		t.Errorf("Search priority bounds: want [p3], got %v", ids)
	}
}

func TestSearch_WorkStateReady(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "dep", Status: "closed"})
	r.Seed(memory.Issue{ID: "ready", Status: "open", DependsOn: []string{"dep"}})
	r.Seed(memory.Issue{ID: "blocked", Status: "open", DependsOn: []string{"dep", "not-closed"}})
	r.Seed(memory.Issue{ID: "not-closed", Status: "open"})
	r.Seed(memory.Issue{ID: "nodep", Status: "open"})

	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{
		WorkState: domain.WorkStateReady,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	ids := searchResultIDs(page.Results)
	if !containsID(ids, "ready") {
		t.Errorf("Search ready: expected ready, got %v", ids)
	}
	if !containsID(ids, "nodep") {
		t.Errorf("Search ready: expected nodep, got %v", ids)
	}
	if containsID(ids, "blocked") {
		t.Errorf("Search ready: blocked should not be ready, got %v", ids)
	}
	if containsID(ids, "dep") {
		t.Errorf("Search ready: closed dep should not appear, got %v", ids)
	}
}

func TestSearch_WorkStateBlocked(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "dep-open", Status: "open"})
	r.Seed(memory.Issue{ID: "dep-closed", Status: "closed"})
	r.Seed(memory.Issue{ID: "dep-blocked", Status: "open", DependsOn: []string{"dep-open"}})
	r.Seed(memory.Issue{ID: "not-blocked", Status: "open", DependsOn: []string{"dep-closed"}})

	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{
		WorkState: domain.WorkStateBlocked,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	ids := searchResultIDs(page.Results)
	if !containsID(ids, "dep-blocked") {
		t.Errorf("Search blocked: expected dep-blocked, got %v", ids)
	}
	if containsID(ids, "not-blocked") {
		t.Errorf("Search blocked: not-blocked should not appear, got %v", ids)
	}
}

func TestSearch_LimitOffset(t *testing.T) {
	r := memory.New()
	for i := 1; i <= 5; i++ {
		r.Seed(memory.Issue{ID: "x" + itoa(i), Status: "open"})
	}

	// Get all.
	all, _ := r.Search(context.Background(), domain.SearchIssuesQuery{})
	if len(all.Results) != 5 {
		t.Fatalf("Search all: want 5, got %d", len(all.Results))
	}

	// Limit to 2.
	page, _ := r.Search(context.Background(), domain.SearchIssuesQuery{Limit: 2})
	if len(page.Results) != 2 {
		t.Errorf("Search limit=2: want 2, got %d", len(page.Results))
	}
	if page.Metadata.ReturnedCount != 2 {
		t.Errorf("ReturnedCount: want 2, got %d", page.Metadata.ReturnedCount)
	}

	// Offset 3, no limit.
	pageOff, _ := r.Search(context.Background(), domain.SearchIssuesQuery{Offset: 3})
	if len(pageOff.Results) != 2 {
		t.Errorf("Search offset=3: want 2, got %d", len(pageOff.Results))
	}
}

func TestSearch_Completeness(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "s1", Status: "open"})
	page, _ := r.Search(context.Background(), domain.SearchIssuesQuery{})
	if page.Metadata.Completeness != domain.SearchResultCompletenessExact {
		t.Errorf("Completeness: want Exact, got %v", page.Metadata.Completeness)
	}
}

// ---- Dep resolution in Issue detail ----

func TestIssue_BlockedByAndBlocks(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "root", Status: "open"})
	r.Seed(memory.Issue{ID: "child", Status: "open", DependsOn: []string{"root"}})

	// child.BlockedBy should contain root.
	childDetail, err := r.Issue(context.Background(), "child")
	if err != nil {
		t.Fatalf("Issue child: %v", err)
	}
	if len(childDetail.BlockedBy) != 1 || childDetail.BlockedBy[0].ID != "root" {
		t.Errorf("BlockedBy: want [root], got %v", childDetail.BlockedBy)
	}

	// root.Blocks should contain child.
	rootDetail, err := r.Issue(context.Background(), "root")
	if err != nil {
		t.Fatalf("Issue root: %v", err)
	}
	if len(rootDetail.Blocks) != 1 || rootDetail.Blocks[0].ID != "child" {
		t.Errorf("Blocks: want [child], got %v", rootDetail.Blocks)
	}
}

// ---- Mutation timestamps ----

func TestMutationTimestamps_CreatedBeforeUpdated(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	after := base.Add(time.Hour)
	calls := 0
	clk := func() time.Time {
		calls++
		if calls == 1 {
			return base
		}
		return after
	}
	r := memory.New(memory.WithClock(clk))
	res, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "t"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	// Manually tick the clock by seeding.
	// Now update to advance the timestamp.
	_ = r.UpdateIssue(context.Background(), res.IssueID, domain.UpdateIssueInput{
		Description: ptr("updated"),
	})

	detail, _ := r.Issue(context.Background(), res.IssueID)
	if !detail.Summary.CreatedAt.Equal(base) {
		t.Errorf("CreatedAt: want %v, got %v", base, detail.Summary.CreatedAt)
	}
	if !detail.Summary.UpdatedAt.After(detail.Summary.CreatedAt) {
		t.Errorf("UpdatedAt (%v) must be after CreatedAt (%v)", detail.Summary.UpdatedAt, detail.Summary.CreatedAt)
	}
}

func TestClose_ClosedOnlySetWhenClosed(t *testing.T) {
	r := memory.New()
	res, _ := r.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "t"})

	detail, _ := r.Issue(context.Background(), res.IssueID)
	if !detail.ClosedAt.IsZero() {
		t.Errorf("ClosedAt should be zero before close, got %v", detail.ClosedAt)
	}

	_ = r.CloseIssue(context.Background(), res.IssueID, domain.CloseIssueInput{})

	detail2, _ := r.Issue(context.Background(), res.IssueID)
	if detail2.ClosedAt.IsZero() {
		t.Error("ClosedAt should be non-zero after close")
	}
}

// ---- Concurrency / race safety ----

func TestConcurrency_RaceSafety(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "shared", Status: "open"})

	ctx := context.Background()
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 4)

	// Concurrent reads.
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = r.Dashboard(ctx, repository.DashboardOptions{})
		}()
		go func() {
			defer wg.Done()
			_, _ = r.Issue(ctx, "shared")
		}()
		go func() {
			defer wg.Done()
			_, _ = r.Search(ctx, domain.SearchIssuesQuery{Text: "open"})
		}()
		go func() {
			defer wg.Done()
			_, _ = r.Catalogs(ctx)
		}()
	}

	// Concurrent writes mixed in.
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			_, _ = r.CreateIssue(ctx, domain.CreateIssueInput{Title: "t"})
		}(i)
	}

	wg.Wait()
}

// ---- helpers ----

func issueIDs(issues []domain.IssueSummary) []string {
	out := make([]string, len(issues))
	for i, s := range issues {
		out[i] = s.ID
	}
	return out
}

func blockedViewIDs(views []domain.BlockedIssueView) []string {
	out := make([]string, len(views))
	for i, v := range views {
		out[i] = v.Issue.ID
	}
	return out
}

func searchResultIDs(results []domain.SearchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Issue.ID
	}
	return out
}

func containsID(ids []string, id string) bool {
	for _, s := range ids {
		if s == id {
			return true
		}
	}
	return false
}

// closedOffsetIssueIndex extracts the numeric index from an ID of the form
// "cl-<n>" used by the ClosedOffset pagination tests.
func closedOffsetIssueIndex(id string) int {
	const prefix = "cl-"
	if len(id) <= len(prefix) {
		return -1
	}
	n := 0
	for _, ch := range id[len(prefix):] {
		if ch < '0' || ch > '9' {
			return -1
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// ---- Forget tests ----

func TestForget_ExistingID_DropsIssue(t *testing.T) {
	r := memory.New()
	r.Seed(memory.Issue{ID: "x-1", Title: "to forget"})

	// Confirm it exists.
	if _, err := r.Issue(context.Background(), "x-1"); err != nil {
		t.Fatalf("before Forget: unexpected error %v", err)
	}

	r.Forget("x-1")

	// Should be gone.
	_, err := r.Issue(context.Background(), "x-1")
	if !errors.Is(err, repository.ErrIssueNotFound) {
		t.Fatalf("after Forget: expected ErrIssueNotFound, got %v", err)
	}
}

func TestForget_AbsentID_NoOp(t *testing.T) {
	r := memory.New()

	// Should not panic or return an error.
	r.Forget("does-not-exist")

	// Store is empty; verify HealthCheck still works.
	if err := r.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck after Forget of absent ID: %v", err)
	}
}

// ---- Snapshot round-trip tests ----

// TestSnapshot_LosslessRoundTrip_FullDetail verifies that Snapshot/Seed
// preserves Related, ParentID, ChildrenIDs, and Blocks (via reverse lookup)
// through a JSON encode/decode cycle, matching the SaveNow→Hydrate path.
func TestSnapshot_LosslessRoundTrip_FullDetail(t *testing.T) {
	base := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	src := memory.New(memory.WithClock(staticClock(base)))

	// Seed parent epic.
	src.Seed(memory.Issue{
		ID:     "epic-1",
		Title:  "Parent Epic",
		Type:   "epic",
		Status: "open",
	})

	// Seed a child issue that also blocks another issue and has a related ref.
	src.Seed(memory.Issue{
		ID:          "child-1",
		Title:       "Child One",
		Type:        "task",
		Status:      "open",
		ParentID:    "epic-1",
		ChildrenIDs: []string{"grandchild-1"},
		Related:     []string{"related-1"},
	})

	// Seed the related issue.
	src.Seed(memory.Issue{
		ID:     "related-1",
		Title:  "Related Issue",
		Type:   "task",
		Status: "open",
	})

	// Seed a grandchild just so ChildrenIDs resolves to a full reference.
	src.Seed(memory.Issue{
		ID:     "grandchild-1",
		Title:  "Grandchild",
		Type:   "task",
		Status: "in_progress",
	})

	// Seed an issue that depends on child-1, producing a Blocks entry via reverse lookup.
	src.Seed(memory.Issue{
		ID:        "blocker-dep-1",
		Title:     "Blocked by child-1",
		Type:      "task",
		Status:    "open",
		DependsOn: []string{"child-1"},
	})

	// Take a snapshot, JSON-encode each issue, JSON-decode back, then re-seed
	// into a fresh repository. This mirrors the SaveNow→Hydrate code path in
	// filestorage.LoadWithManifest.
	snaps := src.Snapshot()

	dst := memory.New(memory.WithClock(staticClock(base)))
	for _, snap := range snaps {
		raw, err := json.Marshal(snap)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		var decoded memory.SnapshotIssue
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		dst.Seed(memory.Issue{
			ID:          decoded.ID,
			Title:       decoded.Title,
			Status:      decoded.Status,
			Priority:    decoded.Priority,
			Type:        decoded.Type,
			Assignee:    decoded.Assignee,
			Labels:      decoded.Labels,
			Description: decoded.Description,
			Notes:       decoded.Notes,
			DependsOn:   decoded.DependsOn,
			Related:     decoded.Related,
			ParentID:    decoded.ParentID,
			ChildrenIDs: decoded.ChildrenIDs,
			Created:     decoded.Created,
			Updated:     decoded.Updated,
		})
		if decoded.Status == "closed" && !decoded.Closed.IsZero() {
			dst.SeedClosed(decoded.ID, decoded.Closed, decoded.CloseReason)
		}
	}

	ctx := context.Background()
	got, err := dst.Issue(ctx, "child-1")
	if err != nil {
		t.Fatalf("Issue(child-1): %v", err)
	}

	// Related: should contain a resolved reference to related-1.
	wantRelated := []domain.IssueReference{
		{ID: "related-1", Title: "Related Issue", Type: "task", Priority: 0, Status: "open"},
	}
	if !reflect.DeepEqual(got.Related, wantRelated) {
		t.Errorf("Related:\n  got  %+v\n  want %+v", got.Related, wantRelated)
	}

	// ParentGroupBrowser.Parent should point to epic-1.
	wantParent := domain.IssueReference{
		ID:     "epic-1",
		Title:  "Parent Epic",
		Type:   "epic",
		Status: "open",
	}
	if !reflect.DeepEqual(got.ParentGroupBrowser.Parent, wantParent) {
		t.Errorf("ParentGroupBrowser.Parent:\n  got  %+v\n  want %+v", got.ParentGroupBrowser.Parent, wantParent)
	}

	// ParentGroupBrowser.Children should contain grandchild-1.
	wantChildren := []domain.IssueReference{
		{ID: "grandchild-1", Title: "Grandchild", Type: "task", Status: "in_progress"},
	}
	if !reflect.DeepEqual(got.ParentGroupBrowser.Children, wantChildren) {
		t.Errorf("ParentGroupBrowser.Children:\n  got  %+v\n  want %+v", got.ParentGroupBrowser.Children, wantChildren)
	}

	// Blocks: reverse lookup finds blocker-dep-1 (whose DependsOn includes child-1).
	// Since no explicit blocksIDs are stored, this comes from the reverse scan.
	if len(got.Blocks) != 1 {
		t.Fatalf("Blocks: got %d entries, want 1; entries: %v", len(got.Blocks), got.Blocks)
	}
	if got.Blocks[0].ID != "blocker-dep-1" {
		t.Errorf("Blocks[0].ID: got %q, want blocker-dep-1", got.Blocks[0].ID)
	}
}

// ---- SeedDetail tests ----

// TestSeedDetail_PreservesCrossRefMetadata verifies that SeedDetail stores full
// IssueReference metadata (Title, Status, Type, Priority) for cross-referenced
// issues, and that a subsequent Issue call returns those fields verbatim — even
// when the referenced issues (B, R, P, C1) were never seeded into the store.
func TestSeedDetail_PreservesCrossRefMetadata(t *testing.T) {
	r := memory.New()

	// Seed ONLY issue A; do NOT seed B, R, P, or C1.
	detail := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       "A",
			Title:    "Issue A",
			Status:   "open",
			Type:     "task",
			Priority: 0,
		},
		BlockedBy: []domain.IssueReference{
			{ID: "B", Title: "Real B title", Status: "open", Type: "task", Priority: 1},
		},
		Related: []domain.IssueReference{
			{ID: "R", Title: "Related title", Status: "in_progress", Type: "bug", Priority: 0},
		},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{
			Parent: domain.IssueReference{
				ID: "P", Title: "Parent", Status: "open", Type: "epic", Priority: 2,
			},
			Children: []domain.IssueReference{
				{ID: "C1", Title: "Child", Status: "open", Type: "task", Priority: 0},
			},
		},
	}

	r.SeedDetail(detail)

	got, err := r.Issue(context.Background(), "A")
	if err != nil {
		t.Fatalf("Issue(A) after SeedDetail: %v", err)
	}

	// BlockedBy: cross-ref B was never seeded — must preserve all metadata.
	if len(got.BlockedBy) != 1 {
		t.Fatalf("BlockedBy: got %d entries, want 1", len(got.BlockedBy))
	}
	wantBlockedBy := domain.IssueReference{
		ID: "B", Title: "Real B title", Status: "open", Type: "task", Priority: 1,
	}
	if !reflect.DeepEqual(got.BlockedBy[0], wantBlockedBy) {
		t.Errorf("BlockedBy[0]:\n  got  %+v\n  want %+v", got.BlockedBy[0], wantBlockedBy)
	}

	// Related: cross-ref R was never seeded — must preserve all metadata.
	if len(got.Related) != 1 {
		t.Fatalf("Related: got %d entries, want 1", len(got.Related))
	}
	wantRelated := domain.IssueReference{
		ID: "R", Title: "Related title", Status: "in_progress", Type: "bug", Priority: 0,
	}
	if !reflect.DeepEqual(got.Related[0], wantRelated) {
		t.Errorf("Related[0]:\n  got  %+v\n  want %+v", got.Related[0], wantRelated)
	}

	// ParentGroupBrowser.Parent: cross-ref P was never seeded — must preserve metadata.
	wantParent := domain.IssueReference{
		ID: "P", Title: "Parent", Status: "open", Type: "epic", Priority: 2,
	}
	if !reflect.DeepEqual(got.ParentGroupBrowser.Parent, wantParent) {
		t.Errorf("ParentGroupBrowser.Parent:\n  got  %+v\n  want %+v",
			got.ParentGroupBrowser.Parent, wantParent)
	}

	// ParentGroupBrowser.Children: cross-ref C1 was never seeded — must preserve metadata.
	if len(got.ParentGroupBrowser.Children) != 1 {
		t.Fatalf("ParentGroupBrowser.Children: got %d entries, want 1",
			len(got.ParentGroupBrowser.Children))
	}
	wantChild := domain.IssueReference{
		ID: "C1", Title: "Child", Status: "open", Type: "task", Priority: 0,
	}
	if !reflect.DeepEqual(got.ParentGroupBrowser.Children[0], wantChild) {
		t.Errorf("ParentGroupBrowser.Children[0]:\n  got  %+v\n  want %+v",
			got.ParentGroupBrowser.Children[0], wantChild)
	}
}

func TestSeedDetail_RoundTrip(t *testing.T) {
	r := memory.New()

	detail := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       "sd-1",
			Title:    "seeded via detail",
			Status:   "in_progress",
			Type:     "bug",
			Priority: 2,
			Assignee: "alice",
			Labels:   []string{"backend"},
		},
		Description: "a description",
		Notes:       "some notes",
	}

	r.SeedDetail(detail)

	got, err := r.Issue(context.Background(), "sd-1")
	if err != nil {
		t.Fatalf("Issue after SeedDetail: %v", err)
	}
	if got.Summary.ID != "sd-1" {
		t.Errorf("ID: got %q, want sd-1", got.Summary.ID)
	}
	if got.Summary.Title != detail.Summary.Title {
		t.Errorf("Title: got %q, want %q", got.Summary.Title, detail.Summary.Title)
	}
	if got.Summary.Status != detail.Summary.Status {
		t.Errorf("Status: got %q, want %q", got.Summary.Status, detail.Summary.Status)
	}
	if got.Description != detail.Description {
		t.Errorf("Description: got %q, want %q", got.Description, detail.Description)
	}
}

// TestSeedDetail_PreservesCreator verifies that SeedDetail preserves the
// Creator field and that a subsequent Issue call returns it correctly.
func TestSeedDetail_PreservesCreator(t *testing.T) {
	r := memory.New()

	detail := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:     "creator-1",
			Title:  "issue with creator",
			Status: "open",
			Type:   "task",
		},
		Creator: "alice",
	}

	r.SeedDetail(detail)

	got, err := r.Issue(context.Background(), "creator-1")
	if err != nil {
		t.Fatalf("Issue after SeedDetail: %v", err)
	}
	if got.Creator != "alice" {
		t.Errorf("Creator: got %q, want %q", got.Creator, "alice")
	}
}

// TestSnapshotRoundTrip_PreservesCreator verifies that Creator survives a
// Snapshot → JSON marshal/unmarshal → SeedFromSnapshot → Issue round-trip.
func TestSnapshotRoundTrip_PreservesCreator(t *testing.T) {
	src := memory.New()

	detail := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:     "snap-creator-1",
			Title:  "issue for snapshot creator test",
			Status: "open",
			Type:   "task",
		},
		Creator: "alice",
	}
	src.SeedDetail(detail)

	snaps := src.Snapshot()

	dst := memory.New()
	for _, snap := range snaps {
		raw, err := json.Marshal(snap)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		var decoded memory.SnapshotIssue
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		dst.SeedFromSnapshot(decoded)
	}

	got, err := dst.Issue(context.Background(), "snap-creator-1")
	if err != nil {
		t.Fatalf("Issue after SeedFromSnapshot: %v", err)
	}
	if got.Creator != "alice" {
		t.Errorf("Creator after round-trip: got %q, want %q", got.Creator, "alice")
	}
}
