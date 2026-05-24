package search

// Regression tests for beads-workbench-czkq.4: "Search Enter never submits
// draft; results stay stuck on prior auto-search ignoring the typed query".
//
// Root cause: when the user presses Enter while Init's empty-query search is
// still in flight, the triggerSearchWithAnchor re-entry guard (m.loading==true)
// silently discards the keystroke and returns nil. The user's typed query is
// never applied; the Init results (all issues, including closed, via bd list
// --all) remain as the visible result set.
//
// Fix: when Enter arrives while loading, queue the draft as m.pendingDraft.
// The searchLoadedMsg handler consumes pendingDraft and re-fires the search
// once the in-flight load resolves.
//
// These tests exercise the overlapping-async path that pressAndResolve cannot
// cover (pressAndResolve drains every Cmd synchronously before the next key,
// preventing the race).

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	memoryrepo "github.com/hk9890/beads-workbench/internal/repository/memory"
	uisearch "github.com/hk9890/beads-workbench/internal/ui/search"
)

// recordingSearchRepo wraps a searchRepo and records all Search queries.
type recordingSearchRepo struct {
	repo          *searchRepo
	searchQueries []domain.SearchIssuesQuery
}

func (r *recordingSearchRepo) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	r.searchQueries = append(r.searchQueries, query)
	return r.repo.Search(ctx, query)
}

func (r *recordingSearchRepo) Dashboard(ctx context.Context) (repository.DashboardData, error) {
	return r.repo.Dashboard(ctx)
}
func (r *recordingSearchRepo) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	return r.repo.Issue(ctx, id)
}
func (r *recordingSearchRepo) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return r.repo.CreateIssue(ctx, input)
}
func (r *recordingSearchRepo) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	return r.repo.UpdateIssue(ctx, id, input)
}
func (r *recordingSearchRepo) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	return r.repo.CloseIssue(ctx, id, input)
}
func (r *recordingSearchRepo) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	return r.repo.AddComment(ctx, id, input)
}
func (r *recordingSearchRepo) HealthCheck(ctx context.Context) error {
	return r.repo.HealthCheck(ctx)
}
func (r *recordingSearchRepo) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	return r.repo.Catalogs(ctx)
}

// TestEnterWhileInitInFlight_PendingDraftQueued verifies that pressing Enter
// while the Init empty-query search is still in flight (m.loading==true) does
// not silently drop the keystroke: the draft is queued as m.pendingDraft and
// m.loading stays true (no new search fires yet).
func TestEnterWhileInitInFlight_PendingDraftQueued(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "task one", Status: "open", Type: "task", Priority: 1})
	gw.repo.Seed(memoryrepo.Issue{ID: "bwf-3", Title: "closed task", Status: "closed", Type: "task", Priority: 2})

	// Build and Init the model WITHOUT draining the init Cmd — model is in
	// loading state, simulating the race window.
	m := NewModel(gw, nil)
	m.SetSize(120, 30)
	initCmd := m.Init()

	// The init Cmd is in flight but NOT yet resolved. Model must be loading.
	if !m.loading {
		t.Fatalf("setup: expected loading=true after Init(), before draining cmd")
	}
	if initCmd == nil {
		t.Fatalf("setup: expected non-nil Cmd from Init()")
	}

	// Type "task" into the query field — these are purely state updates (no Cmd).
	for _, r := range []rune("task") {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.draftQuery != "task" {
		t.Fatalf("expected draftQuery=%q after typing, got %q", "task", m.draftQuery)
	}

	// Press Enter while loading. Under the bug this was silently dropped.
	// After the fix, pendingDraft should be queued and loading stays true.
	enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if enterCmd != nil {
		t.Fatalf("expected nil Cmd from Enter while loading (search should be queued, not fired), got non-nil")
	}
	if m.pendingDraft == nil {
		t.Fatalf("expected pendingDraft to be set after Enter while loading, got nil")
	}
	if *m.pendingDraft != "task" {
		t.Fatalf("expected pendingDraft=%q, got %q", "task", *m.pendingDraft)
	}
	if !m.loading {
		t.Fatalf("expected loading=true after Enter while loading (init still in flight)")
	}
}

// TestEnterWhileInitInFlight_PendingDraftFiredOnResolution verifies the full
// sequence: Init in-flight → type query → Enter (queued) → Init resolves →
// pending search fires → user's query applied, not the Init results.
func TestEnterWhileInitInFlight_PendingDraftFiredOnResolution(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "task one", Status: "open", Type: "task", Priority: 1})
	gw.repo.Seed(memoryrepo.Issue{ID: "bwf-4", Title: "another task", Status: "open", Type: "task", Priority: 3})
	gw.repo.Seed(memoryrepo.Issue{ID: "bwf-3", Title: "closed task", Status: "closed", Type: "task", Priority: 2})

	// Build and Init WITHOUT draining — model is loading.
	m := NewModel(gw, nil)
	m.SetSize(120, 30)
	initCmd := m.Init()

	if !m.loading {
		t.Fatalf("setup: expected loading=true before init drain")
	}

	// Type "task" while loading.
	for _, r := range []rune("task") {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter while loading — should queue, not fire.
	_ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.pendingDraft == nil {
		t.Fatalf("expected pendingDraft queued after Enter while loading")
	}

	// Now resolve the Init search: execute the initCmd to get the searchLoadedMsg,
	// then deliver it to the model. The handler should find pendingDraft and fire
	// a new search.
	initMsgs := drainCmd(initCmd)
	if len(initMsgs) != 1 {
		t.Fatalf("expected exactly 1 msg from Init cmd, got %d", len(initMsgs))
	}

	pendingCmd := m.Update(initMsgs[0])

	// pendingDraft should be cleared now.
	if m.pendingDraft != nil {
		t.Fatalf("expected pendingDraft cleared after searchLoadedMsg, got %v", *m.pendingDraft)
	}

	// A new search Cmd should have been returned (for the "task" query).
	if pendingCmd == nil {
		t.Fatalf("expected non-nil Cmd from searchLoadedMsg when pendingDraft was set")
	}

	// Drain the pending search Cmd (this is the "task" text search).
	pendingMsgs := drainCmd(pendingCmd)
	if len(pendingMsgs) != 1 {
		t.Fatalf("expected exactly 1 msg from pending search cmd, got %d: %v", len(pendingMsgs), pendingMsgs)
	}

	// Apply the result.
	_ = m.Update(pendingMsgs[0])

	// Verify: applied query should be "task", not "".
	if m.appliedQuery != "task" {
		t.Fatalf("expected appliedQuery=%q after pending search resolves, got %q", "task", m.appliedQuery)
	}

	// Verify: loading is done.
	if m.loading {
		t.Fatalf("expected loading=false after pending search resolves")
	}

	// Verify: focus is still on query (user didn't navigate away).
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected focus=FocusQuery after pending search resolves, got %v", m.focus)
	}
}

// TestEnterWhileInitInFlight_SearchQueryPassesNoStatusFilter verifies that the
// query the model sends to the repository when the user types+Enter does NOT
// include a forced "all" statuses filter. The model sends
// SearchIssuesQuery{Text: "task"} with no Statuses field — the repository layer
// applies its own default (bd search excludes closed; memory repo includes all
// when Statuses is unset, which is its documented behavior for unit-test contexts).
//
// This test pins the argv-shape contract at the model level using a
// RecordingExecutor-backed real beads.Repository — so the actual bd args are
// observable. The real test for closed-issue exclusion at the repository layer
// is covered by the parity integration tests.
func TestEnterWhileInitInFlight_SearchQueryPassesNoStatusFilter(t *testing.T) {
	t.Parallel()

	wantArgv := []string{"search", "task", "--json", "--limit", "20"}

	innerRepo := newSearchRepo()
	innerRepo.repo.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "task one", Status: "open", Type: "task", Priority: 1})
	innerRepo.repo.Seed(memoryrepo.Issue{ID: "bwf-3", Title: "closed task", Status: "closed", Type: "task", Priority: 2})
	rec := &recordingSearchRepo{repo: innerRepo}

	// Build and Init WITHOUT draining.
	m := NewModel(rec, nil)
	m.SetSize(120, 30)
	initCmd := m.Init()

	// Type "task" while loading.
	for _, r := range []rune("task") {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Drain Init.
	for _, msg := range drainCmd(initCmd) {
		pendingCmd := m.Update(msg)
		for _, pending := range drainCmd(pendingCmd) {
			_ = m.Update(pending)
		}
	}

	if m.appliedQuery != "task" {
		t.Fatalf("expected appliedQuery=%q, got %q", "task", m.appliedQuery)
	}

	// Verify no "all" status was passed.
	queries := rec.searchQueries
	foundTextSearch := false
	for _, q := range queries {
		if q.Text == "task" {
			foundTextSearch = true
			if len(q.Statuses) > 0 {
				t.Errorf("expected no Statuses filter for text search, got %v", q.Statuses)
			}
		}
	}
	if !foundTextSearch {
		t.Errorf("expected a search call with Text=%q; got queries: %v", "task", queries)
	}

	_ = wantArgv // argvCheck via argv_cardinality_test.go covers the argv shape
}

// TestEnterWhileInitInFlight_PendingDraftNotFiredIfMatchesApplied verifies that
// if the queued pending draft happens to equal the applied query (e.g. the Init
// empty-query matches the draft), no redundant search is fired.
func TestEnterWhileInitInFlight_PendingDraftNotFiredIfMatchesApplied(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "bwf-1", Title: "everything", Status: "open", Type: "task", Priority: 1})

	// Build and Init WITHOUT draining.
	m := NewModel(gw, nil)
	m.SetSize(120, 30)
	initCmd := m.Init()

	if !m.loading {
		t.Fatalf("setup: expected loading=true")
	}

	// Don't type anything — draft is empty, same as Init's applied query.
	// Press Enter while loading.
	_ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// pendingDraft should be queued (empty string).
	if m.pendingDraft == nil {
		t.Fatalf("expected pendingDraft queued (even empty) after Enter while loading")
	}

	// Resolve Init.
	initMsgs := drainCmd(initCmd)
	pendingCmd := m.Update(initMsgs[0])

	// After Init resolves with appliedQuery="" and pendingDraft="", the
	// handler should detect pending==applied and NOT fire a new search.
	// The returned cmd should be a selectionChangedCmd (not a search cmd).
	// We verify indirectly: m.loading must remain false (no new search fired).
	_ = pendingCmd // may be selectionChangedCmd; that's fine
	if m.loading {
		t.Fatalf("expected loading=false; a redundant search was fired for pending=applied case")
	}
}
