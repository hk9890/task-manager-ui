package caching_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/repository/caching"
	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

// ---- fake backing repository ----

// stubRepository is a function-field fake used to count calls and inject
// custom responses. It does not use ErrorInjectingRepository because this
// test needs to count calls AND inject custom return values, which is a
// different concern from failure-path injection.
type stubRepository struct {
	mu sync.Mutex

	dashboardCalls int
	dashboardFn    func(ctx context.Context) (repository.DashboardData, error)

	issueCalls int
	issueFn    func(ctx context.Context, id string) (domain.IssueDetail, error)

	searchCalls int
	searchFn    func(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error)

	createIssueCalls int
	createIssueFn    func(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error)

	updateIssueCalls int
	updateIssueFn    func(ctx context.Context, id string, input domain.UpdateIssueInput) error

	closeIssueCalls int
	closeIssueFn    func(ctx context.Context, id string, input domain.CloseIssueInput) error

	addCommentCalls int
	addCommentFn    func(ctx context.Context, id string, input domain.AddCommentInput) error

	healthCheckCalls int
	healthCheckFn    func(ctx context.Context) error

	catalogsCalls int
	catalogsFn    func(ctx context.Context) (repository.Catalogs, error)
}

func (s *stubRepository) Dashboard(ctx context.Context) (repository.DashboardData, error) {
	s.mu.Lock()
	s.dashboardCalls++
	fn := s.dashboardFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return repository.DashboardData{}, nil
}

func (s *stubRepository) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	s.mu.Lock()
	s.issueCalls++
	fn := s.issueFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, id)
	}
	return domain.IssueDetail{}, nil
}

func (s *stubRepository) Search(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	s.mu.Lock()
	s.searchCalls++
	fn := s.searchFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, query)
	}
	return domain.SearchResultPage{}, nil
}

func (s *stubRepository) CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	s.mu.Lock()
	s.createIssueCalls++
	fn := s.createIssueFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, input)
	}
	return domain.CreateIssueResult{}, nil
}

func (s *stubRepository) UpdateIssue(ctx context.Context, id string, input domain.UpdateIssueInput) error {
	s.mu.Lock()
	s.updateIssueCalls++
	fn := s.updateIssueFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, id, input)
	}
	return nil
}

func (s *stubRepository) CloseIssue(ctx context.Context, id string, input domain.CloseIssueInput) error {
	s.mu.Lock()
	s.closeIssueCalls++
	fn := s.closeIssueFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, id, input)
	}
	return nil
}

func (s *stubRepository) AddComment(ctx context.Context, id string, input domain.AddCommentInput) error {
	s.mu.Lock()
	s.addCommentCalls++
	fn := s.addCommentFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, id, input)
	}
	return nil
}

func (s *stubRepository) HealthCheck(ctx context.Context) error {
	s.mu.Lock()
	s.healthCheckCalls++
	fn := s.healthCheckFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return nil
}

func (s *stubRepository) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	s.mu.Lock()
	s.catalogsCalls++
	fn := s.catalogsFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return repository.Catalogs{}, nil
}

// ---- helpers ----

func ptr[T any](v T) *T { return &v }

var errBacking = errors.New("backing error")

// ---- Dashboard tests ----

func TestDashboard_CacheMiss_ThenHit(t *testing.T) {
	wantData := repository.DashboardData{
		ClosedTotal: 42,
	}
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return wantData, nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// First call: cache miss → backing called.
	got, err := c.Dashboard(ctx)
	if err != nil {
		t.Fatalf("first Dashboard: unexpected error %v", err)
	}
	if got.ClosedTotal != wantData.ClosedTotal {
		t.Fatalf("first Dashboard: got ClosedTotal=%d, want %d", got.ClosedTotal, wantData.ClosedTotal)
	}
	if stub.dashboardCalls != 1 {
		t.Fatalf("first Dashboard: expected 1 backing call, got %d", stub.dashboardCalls)
	}

	// Second call: cache hit → no additional backing call.
	got2, err := c.Dashboard(ctx)
	if err != nil {
		t.Fatalf("second Dashboard: unexpected error %v", err)
	}
	if got2.ClosedTotal != wantData.ClosedTotal {
		t.Fatalf("second Dashboard: got ClosedTotal=%d, want %d", got2.ClosedTotal, wantData.ClosedTotal)
	}
	if stub.dashboardCalls != 1 {
		t.Fatalf("second Dashboard: expected still 1 backing call, got %d", stub.dashboardCalls)
	}
}

func TestDashboard_BackingError_DirtyFlagPreserved(t *testing.T) {
	callCount := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			callCount++
			if callCount == 1 {
				return repository.DashboardData{}, errBacking
			}
			return repository.DashboardData{ClosedTotal: 7}, nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// First call: error → dirty flag NOT cleared.
	_, err := c.Dashboard(ctx)
	if !errors.Is(err, errBacking) {
		t.Fatalf("expected errBacking, got %v", err)
	}

	// Second call: dirty flag still set → backing called again.
	got, err := c.Dashboard(ctx)
	if err != nil {
		t.Fatalf("second Dashboard: unexpected error %v", err)
	}
	if got.ClosedTotal != 7 {
		t.Fatalf("second Dashboard: got ClosedTotal=%d, want 7", got.ClosedTotal)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 backing calls, got %d", callCount)
	}
}

// ---- Issue tests ----

func TestIssue_CacheMiss_ThenHit(t *testing.T) {
	wantDetail := domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:    "issue-1",
			Title: "test issue",
		},
		Description: "a description",
	}
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			if id == "issue-1" {
				return wantDetail, nil
			}
			return domain.IssueDetail{}, errors.New("not found")
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// First call: cache miss → backing called.
	got, err := c.Issue(ctx, "issue-1")
	if err != nil {
		t.Fatalf("first Issue: unexpected error %v", err)
	}
	if got.Summary.Title != wantDetail.Summary.Title {
		t.Fatalf("first Issue: got title=%q, want %q", got.Summary.Title, wantDetail.Summary.Title)
	}
	if stub.issueCalls != 1 {
		t.Fatalf("first Issue: expected 1 backing call, got %d", stub.issueCalls)
	}

	// Second call: cache hit → no additional backing call.
	got2, err := c.Issue(ctx, "issue-1")
	if err != nil {
		t.Fatalf("second Issue: unexpected error %v", err)
	}
	if got2.Summary.ID != "issue-1" {
		t.Fatalf("second Issue: got ID=%q, want issue-1", got2.Summary.ID)
	}
	if stub.issueCalls != 1 {
		t.Fatalf("second Issue: expected still 1 backing call, got %d", stub.issueCalls)
	}
}

func TestIssue_BackingError_NotCached(t *testing.T) {
	stub := &stubRepository{
		issueFn: func(_ context.Context, _ string) (domain.IssueDetail, error) {
			return domain.IssueDetail{}, errBacking
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// First call: error → not cached.
	_, err := c.Issue(ctx, "issue-1")
	if !errors.Is(err, errBacking) {
		t.Fatalf("expected errBacking, got %v", err)
	}

	// Second call: still hits backing (miss not cached).
	_, err = c.Issue(ctx, "issue-1")
	if !errors.Is(err, errBacking) {
		t.Fatalf("second Issue: expected errBacking, got %v", err)
	}
	if stub.issueCalls != 2 {
		t.Fatalf("expected 2 backing calls (miss not cached), got %d", stub.issueCalls)
	}
}

// ---- Search tests ----

func TestSearch_AlwaysPassesThrough(t *testing.T) {
	stub := &stubRepository{
		searchFn: func(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
			return domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "x"}}}}, nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Three calls: all should hit backing.
	for i := 1; i <= 3; i++ {
		_, err := c.Search(ctx, domain.SearchIssuesQuery{Text: "q"})
		if err != nil {
			t.Fatalf("Search call %d: unexpected error %v", i, err)
		}
	}
	if stub.searchCalls != 3 {
		t.Fatalf("expected 3 backing calls for Search, got %d", stub.searchCalls)
	}
}

// ---- Catalogs tests ----

func TestCatalogs_CacheMiss_ThenHit(t *testing.T) {
	wantCats := repository.Catalogs{
		Types: []domain.TypeOption{{Name: "task"}},
	}
	stub := &stubRepository{
		catalogsFn: func(_ context.Context) (repository.Catalogs, error) {
			return wantCats, nil
		},
	}
	now := time.Unix(1000, 0)
	c := caching.New(stub, caching.WithCatalogsTTL(10*time.Minute), caching.WithClock(func() time.Time { return now }))
	ctx := context.Background()

	// First call: cache miss.
	got, err := c.Catalogs(ctx)
	if err != nil {
		t.Fatalf("first Catalogs: unexpected error %v", err)
	}
	if len(got.Types) != 1 || got.Types[0].Name != "task" {
		t.Fatalf("first Catalogs: unexpected types %v", got.Types)
	}
	if stub.catalogsCalls != 1 {
		t.Fatalf("first Catalogs: expected 1 backing call, got %d", stub.catalogsCalls)
	}

	// Second call within TTL: cache hit.
	got2, err := c.Catalogs(ctx)
	if err != nil {
		t.Fatalf("second Catalogs: unexpected error %v", err)
	}
	if len(got2.Types) != 1 {
		t.Fatalf("second Catalogs: unexpected types %v", got2.Types)
	}
	if stub.catalogsCalls != 1 {
		t.Fatalf("second Catalogs (within TTL): expected still 1 backing call, got %d", stub.catalogsCalls)
	}
}

func TestCatalogs_TTLExpiry_Refetches(t *testing.T) {
	wantCats := repository.Catalogs{
		Types: []domain.TypeOption{{Name: "task"}},
	}
	stub := &stubRepository{
		catalogsFn: func(_ context.Context) (repository.Catalogs, error) {
			return wantCats, nil
		},
	}
	base := time.Unix(1000, 0)
	var nowT atomic.Int64
	nowT.Store(base.UnixNano())
	clockFn := func() time.Time { return time.Unix(0, nowT.Load()) }

	ttl := 5 * time.Minute
	c := caching.New(stub, caching.WithCatalogsTTL(ttl), caching.WithClock(clockFn))
	ctx := context.Background()

	// First call: cache miss.
	if _, err := c.Catalogs(ctx); err != nil {
		t.Fatalf("first Catalogs: %v", err)
	}
	if stub.catalogsCalls != 1 {
		t.Fatalf("expected 1 backing call after first fetch, got %d", stub.catalogsCalls)
	}

	// Advance clock past TTL.
	nowT.Store(base.Add(ttl + time.Second).UnixNano())

	// Second call: TTL expired → should refetch.
	if _, err := c.Catalogs(ctx); err != nil {
		t.Fatalf("second Catalogs: %v", err)
	}
	if stub.catalogsCalls != 2 {
		t.Fatalf("expected 2 backing calls after TTL expiry, got %d", stub.catalogsCalls)
	}

	// Third call within new TTL: should hit cache.
	if _, err := c.Catalogs(ctx); err != nil {
		t.Fatalf("third Catalogs: %v", err)
	}
	if stub.catalogsCalls != 2 {
		t.Fatalf("expected still 2 backing calls, got %d", stub.catalogsCalls)
	}
}

func TestCatalogs_BackingError_NotCached(t *testing.T) {
	stub := &stubRepository{
		catalogsFn: func(_ context.Context) (repository.Catalogs, error) {
			return repository.Catalogs{}, errBacking
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	_, err := c.Catalogs(ctx)
	if !errors.Is(err, errBacking) {
		t.Fatalf("expected errBacking, got %v", err)
	}

	// Should still hit backing on second call.
	_, err = c.Catalogs(ctx)
	if !errors.Is(err, errBacking) {
		t.Fatalf("expected errBacking on second call, got %v", err)
	}
	if stub.catalogsCalls != 2 {
		t.Fatalf("expected 2 backing calls (error not cached), got %d", stub.catalogsCalls)
	}
}

// ---- HealthCheck tests ----

func TestHealthCheck_AlwaysPassesThrough(t *testing.T) {
	stub := &stubRepository{
		healthCheckFn: func(_ context.Context) error {
			return nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		if err := c.HealthCheck(ctx); err != nil {
			t.Fatalf("HealthCheck call %d: unexpected error %v", i, err)
		}
	}
	if stub.healthCheckCalls != 3 {
		t.Fatalf("expected 3 backing calls, got %d", stub.healthCheckCalls)
	}
}

// ---- CreateIssue tests ----

func TestCreateIssue_Success_DashboardDirty(t *testing.T) {
	// Arrange: pre-populate the dashboard cache.
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 1}, nil
		},
		createIssueFn: func(_ context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error) {
			return domain.CreateIssueResult{IssueID: "new-1"}, nil
		},
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id, Title: "fetched"}}, nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Warm dashboard cache.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	dashboardCallsBefore := stub.dashboardCalls

	// Create issue.
	result, err := c.CreateIssue(ctx, domain.CreateIssueInput{Title: "new issue"})
	if err != nil {
		t.Fatalf("CreateIssue: unexpected error %v", err)
	}
	if result.IssueID != "new-1" {
		t.Fatalf("CreateIssue: got IssueID=%q, want new-1", result.IssueID)
	}

	// Dashboard cache should now be dirty → next Dashboard() hits backing.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashboardCallsBefore+1 {
		t.Fatalf("expected Dashboard re-fetch after CreateIssue, backing calls: %d", stub.dashboardCalls)
	}

	// Issue should be reachable via cache (no backing call for the created ID).
	issueCallsBefore := stub.issueCalls
	got, err := c.Issue(ctx, "new-1")
	if err != nil {
		t.Fatalf("Issue after CreateIssue: unexpected error %v", err)
	}
	if got.Summary.ID != "new-1" {
		t.Fatalf("Issue after CreateIssue: got ID=%q, want new-1", got.Summary.ID)
	}
	// The issue should have been seeded and not require a backing call.
	if stub.issueCalls != issueCallsBefore {
		t.Fatalf("expected Issue to be served from cache after CreateIssue, got %d backing calls", stub.issueCalls-issueCallsBefore)
	}
}

func TestCreateIssue_BackingError_NoMutation(t *testing.T) {
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 99}, nil
		},
		createIssueFn: func(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
			return domain.CreateIssueResult{}, errBacking
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Warm dashboard cache.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	dashboardCallsBefore := stub.dashboardCalls

	_, err := c.CreateIssue(ctx, domain.CreateIssueInput{Title: "fail"})
	if !errors.Is(err, errBacking) {
		t.Fatalf("expected errBacking, got %v", err)
	}

	// Cache should be untouched; dashboard should still be served from cache.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashboardCallsBefore {
		t.Fatalf("expected Dashboard still from cache after failed CreateIssue, got %d extra backing calls", stub.dashboardCalls-dashboardCallsBefore)
	}
}

// ---- UpdateIssue tests ----

func TestUpdateIssue_Success_InvalidatesCache(t *testing.T) {
	// Pre-seed an issue into the cache.
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id, Title: "updated from backing"}}, nil
		},
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 5}, nil
		},
		updateIssueFn: func(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
			return nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Warm issue cache.
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	issueCalls1 := stub.issueCalls

	// Warm dashboard cache.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	dashCalls1 := stub.dashboardCalls

	// Update the issue.
	newTitle := "new title"
	if err := c.UpdateIssue(ctx, "issue-1", domain.UpdateIssueInput{Title: &newTitle}); err != nil {
		t.Fatalf("UpdateIssue: unexpected error %v", err)
	}

	// Issue cache should have been dropped; next Issue call hits backing.
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != issueCalls1+1 {
		t.Fatalf("expected Issue re-fetch after UpdateIssue, got %d extra backing calls", stub.issueCalls-issueCalls1)
	}

	// Dashboard should be dirty; next Dashboard call hits backing.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashCalls1+1 {
		t.Fatalf("expected Dashboard re-fetch after UpdateIssue, got %d extra backing calls", stub.dashboardCalls-dashCalls1)
	}
}

func TestUpdateIssue_BackingError_NoMutation(t *testing.T) {
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id, Title: "cached"}}, nil
		},
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 3}, nil
		},
		updateIssueFn: func(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
			return errBacking
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Warm caches.
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	issueCalls := stub.issueCalls
	dashCalls := stub.dashboardCalls

	newTitle := "fail"
	err := c.UpdateIssue(ctx, "issue-1", domain.UpdateIssueInput{Title: &newTitle})
	if !errors.Is(err, errBacking) {
		t.Fatalf("expected errBacking, got %v", err)
	}

	// Issue and Dashboard caches should be untouched.
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != issueCalls {
		t.Fatalf("expected Issue still from cache after failed UpdateIssue")
	}
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashCalls {
		t.Fatalf("expected Dashboard still from cache after failed UpdateIssue")
	}
}

// ---- CloseIssue tests ----

func TestCloseIssue_Success_InvalidatesCache(t *testing.T) {
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{}, nil
		},
		closeIssueFn: func(_ context.Context, _ string, _ domain.CloseIssueInput) error {
			return nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Warm caches.
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	issueCalls := stub.issueCalls
	dashCalls := stub.dashboardCalls

	if err := c.CloseIssue(ctx, "issue-1", domain.CloseIssueInput{Reason: "done"}); err != nil {
		t.Fatalf("CloseIssue: unexpected error %v", err)
	}

	// Both caches should be invalidated.
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != issueCalls+1 {
		t.Fatalf("expected Issue re-fetch after CloseIssue")
	}
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashCalls+1 {
		t.Fatalf("expected Dashboard re-fetch after CloseIssue")
	}
}

func TestCloseIssue_BackingError_NoMutation(t *testing.T) {
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{}, nil
		},
		closeIssueFn: func(_ context.Context, _ string, _ domain.CloseIssueInput) error {
			return errBacking
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	issueCalls := stub.issueCalls
	dashCalls := stub.dashboardCalls

	err := c.CloseIssue(ctx, "issue-1", domain.CloseIssueInput{Reason: "fail"})
	if !errors.Is(err, errBacking) {
		t.Fatalf("expected errBacking, got %v", err)
	}

	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != issueCalls {
		t.Fatalf("expected Issue still from cache after failed CloseIssue")
	}
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashCalls {
		t.Fatalf("expected Dashboard still from cache after failed CloseIssue")
	}
}

// ---- AddComment tests ----

func TestAddComment_Success_IssueDropped_DashboardUntouched(t *testing.T) {
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 10}, nil
		},
		addCommentFn: func(_ context.Context, _ string, _ domain.AddCommentInput) error {
			return nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Warm caches.
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	issueCalls := stub.issueCalls
	dashCalls := stub.dashboardCalls

	if err := c.AddComment(ctx, "issue-1", domain.AddCommentInput{Body: "hi"}); err != nil {
		t.Fatalf("AddComment: unexpected error %v", err)
	}

	// Issue should be invalidated.
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != issueCalls+1 {
		t.Fatalf("expected Issue re-fetch after AddComment")
	}

	// Dashboard should NOT be dirty; cache still valid.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashCalls {
		t.Fatalf("expected Dashboard still cached after AddComment, got %d extra backing calls", stub.dashboardCalls-dashCalls)
	}
}

func TestAddComment_BackingError_NoMutation(t *testing.T) {
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
		addCommentFn: func(_ context.Context, _ string, _ domain.AddCommentInput) error {
			return errBacking
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	issueCalls := stub.issueCalls

	err := c.AddComment(ctx, "issue-1", domain.AddCommentInput{Body: "fail"})
	if !errors.Is(err, errBacking) {
		t.Fatalf("expected errBacking, got %v", err)
	}

	// Issue cache should still be intact.
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != issueCalls {
		t.Fatalf("expected Issue still from cache after failed AddComment")
	}
}

// ---- Background refresh tick tests ----

// vcStatusFuncFromSlice builds a vcStatusFunc that returns successive hash
// values from a slice. Once exhausted, it returns the last value repeatedly.
// Call count is tracked via the returned *int.
func vcStatusFuncFromSlice(hashes []string) (func(context.Context) (string, error), *int) {
	var mu sync.Mutex
	calls := 0
	fn := func(_ context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls-1 < len(hashes) {
			return hashes[calls-1], nil
		}
		return hashes[len(hashes)-1], nil
	}
	return fn, &calls
}

func TestStartStopLifecycle(t *testing.T) {
	fn, _ := vcStatusFuncFromSlice([]string{"hash-a"})
	c := caching.New(&stubRepository{},
		caching.WithVCStatusFunc(fn),
		caching.WithRefreshInterval(50*time.Millisecond),
	)
	ctx := context.Background()

	c.Start(ctx)

	// Second Start should be no-op (idempotent).
	c.Start(ctx)

	// Stop should block until goroutine exits.
	c.Stop()

	// Second Stop should be safe.
	c.Stop()
}

func TestStartNoVCStatusFunc(t *testing.T) {
	// No vcStatusFunc → Start is a no-op; Stop is safe to call.
	c := caching.New(&stubRepository{})
	ctx := context.Background()
	c.Start(ctx)
	c.Stop() // should not block or panic
}

func TestTickFirstHashIsBaseline(t *testing.T) {
	// First RefreshIfChanged call records hash but does NOT invalidate the cache.
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 1}, nil
		},
	}
	fn, _ := vcStatusFuncFromSlice([]string{"hash-a"})
	c := caching.New(stub, caching.WithVCStatusFunc(fn))
	ctx := context.Background()

	// Warm dashboard cache.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	dashCallsBefore := stub.dashboardCalls

	// First tick: should record baseline, NOT set dashboardDirty.
	c.RefreshIfChanged(ctx)

	// Dashboard should still be served from cache (not dirty).
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashCallsBefore {
		t.Fatalf("first tick should not invalidate dashboard; backing calls before=%d after=%d",
			dashCallsBefore, stub.dashboardCalls)
	}
}

func TestTickUnchangedHashDoesNotInvalidate(t *testing.T) {
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 1}, nil
		},
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
	}
	// Same hash returned on both ticks.
	fn, _ := vcStatusFuncFromSlice([]string{"hash-a", "hash-a"})
	c := caching.New(stub, caching.WithVCStatusFunc(fn))
	ctx := context.Background()

	// Warm caches.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	dashCallsBefore := stub.dashboardCalls
	issueCallsBefore := stub.issueCalls

	// Tick 1: baseline.
	c.RefreshIfChanged(ctx)
	// Tick 2: same hash → no invalidation.
	c.RefreshIfChanged(ctx)

	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashCallsBefore {
		t.Fatalf("unchanged hash: expected no dashboard re-fetch; got %d extra calls",
			stub.dashboardCalls-dashCallsBefore)
	}

	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != issueCallsBefore {
		t.Fatalf("unchanged hash: expected no issue re-fetch; got %d extra calls",
			stub.issueCalls-issueCallsBefore)
	}
}

func TestTickChangedHashInvalidatesDashboardAndIssues(t *testing.T) {
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 99}, nil
		},
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
	}
	// Tick 1 returns hash-a (baseline); tick 2 returns hash-b (change).
	fn, _ := vcStatusFuncFromSlice([]string{"hash-a", "hash-b"})
	c := caching.New(stub, caching.WithVCStatusFunc(fn))
	ctx := context.Background()

	// Warm caches.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	dashCallsBefore := stub.dashboardCalls
	issueCallsBefore := stub.issueCalls

	// Tick 1: baseline (no invalidation).
	c.RefreshIfChanged(ctx)

	// Tick 2: hash changed → invalidation.
	c.RefreshIfChanged(ctx)

	// Dashboard must be re-fetched on next call.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashCallsBefore+1 {
		t.Fatalf("changed hash: expected Dashboard re-fetch; backing calls before=%d after=%d",
			dashCallsBefore, stub.dashboardCalls)
	}

	// Issue must also be re-fetched (cache was Reset).
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != issueCallsBefore+1 {
		t.Fatalf("changed hash: expected Issue re-fetch; backing calls before=%d after=%d",
			issueCallsBefore, stub.issueCalls)
	}
}

func TestVCStatusFuncErrorDoesNotCorruptState(t *testing.T) {
	errVC := errors.New("vc error")
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 5}, nil
		},
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
	}

	callCount := 0
	fn := func(_ context.Context) (string, error) {
		callCount++
		if callCount == 1 {
			return "hash-a", nil // baseline
		}
		return "", errVC // all subsequent calls fail
	}

	c := caching.New(stub, caching.WithVCStatusFunc(fn))
	ctx := context.Background()

	// Warm caches.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	dashCallsBefore := stub.dashboardCalls
	issueCallsBefore := stub.issueCalls

	// Tick 1: baseline.
	c.RefreshIfChanged(ctx)
	// Tick 2: error → state unchanged.
	c.RefreshIfChanged(ctx)

	// Cache should still be intact.
	if _, err := c.Dashboard(ctx); err != nil {
		t.Fatal(err)
	}
	if stub.dashboardCalls != dashCallsBefore {
		t.Fatalf("error tick: expected Dashboard still from cache; got %d extra calls",
			stub.dashboardCalls-dashCallsBefore)
	}

	if _, err := c.Issue(ctx, "issue-1"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != issueCallsBefore {
		t.Fatalf("error tick: expected Issue still from cache; got %d extra calls",
			stub.issueCalls-issueCallsBefore)
	}
}

func TestStopDuringTickWaits(t *testing.T) {
	// Arrange: vcStatusFunc that blocks until a signal, so we can observe Stop
	// waiting for the in-progress tick to complete.
	gate := make(chan struct{})
	unblock := make(chan struct{})

	var gateOnce sync.Once
	fn := func(_ context.Context) (string, error) {
		gateOnce.Do(func() { close(gate) }) // signal that first tick has started
		<-unblock                           // wait until test unblocks
		return "h", nil
	}

	c := caching.New(&stubRepository{},
		caching.WithVCStatusFunc(fn),
		caching.WithRefreshInterval(1*time.Millisecond),
	)
	ctx := context.Background()
	c.Start(ctx)

	// Wait for the first tick to enter vcStatusFunc.
	<-gate

	// Stop in a separate goroutine; it should block until tick finishes.
	stopDone := make(chan struct{})
	go func() {
		c.Stop()
		close(stopDone)
	}()

	// Unblock the tick.
	close(unblock)

	// Stop should complete promptly.
	select {
	case <-stopDone:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after tick unblocked")
	}
}

func TestContextCancellationExitsGoroutine(t *testing.T) {
	fn, _ := vcStatusFuncFromSlice([]string{"hash-a"})
	c := caching.New(&stubRepository{},
		caching.WithVCStatusFunc(fn),
		caching.WithRefreshInterval(50*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)

	// Cancel the parent context; the goroutine should exit on its own.
	cancel()

	// Stop should return promptly (goroutine already exiting or exited).
	done := make(chan struct{})
	go func() {
		c.Stop()
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after ctx cancellation")
	}
}

func TestConcurrentReadsDuringTick(t *testing.T) {
	// Verifies no data races when RefreshIfChanged fires concurrently with reads.
	var issueCallCount atomic.Int64
	var dashCallCount atomic.Int64

	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCallCount.Add(1)
			return repository.DashboardData{ClosedTotal: int(dashCallCount.Load())}, nil
		},
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			issueCallCount.Add(1)
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
		catalogsFn: func(_ context.Context) (repository.Catalogs, error) {
			return repository.Catalogs{}, nil
		},
	}

	// Alternate hashes to trigger invalidation.
	var hashCall atomic.Int64
	fn := func(_ context.Context) (string, error) {
		n := hashCall.Add(1)
		if n%2 == 0 {
			return "hash-even", nil
		}
		return "hash-odd", nil
	}

	c := caching.New(stub,
		caching.WithVCStatusFunc(fn),
		caching.WithRefreshInterval(1*time.Millisecond),
	)
	ctx := context.Background()
	c.Start(ctx)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				_, _ = c.Dashboard(ctx)
			case 1:
				_, _ = c.Issue(ctx, "issue-1")
			case 2:
				_, _ = c.Catalogs(ctx)
			}
		}(i)
	}

	wg.Wait()
	c.Stop()
}

// ---- Concurrent test ----

// TestConcurrentReadWrite spawns N reader goroutines and M writer goroutines
// simultaneously to verify no data races occur. It does not check specific
// values, only that the operations complete without panicking and are race-clean
// (run with -race).
func TestConcurrentReadWrite(t *testing.T) {
	var issueCallCount atomic.Int64
	var dashCallCount atomic.Int64

	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCallCount.Add(1)
			return repository.DashboardData{ClosedTotal: int(dashCallCount.Load())}, nil
		},
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			issueCallCount.Add(1)
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
		catalogsFn: func(_ context.Context) (repository.Catalogs, error) {
			return repository.Catalogs{}, nil
		},
		updateIssueFn: func(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
			return nil
		},
		addCommentFn: func(_ context.Context, _ string, _ domain.AddCommentInput) error {
			return nil
		},
		createIssueFn: func(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
			return domain.CreateIssueResult{IssueID: "concurrent-1"}, nil
		},
	}

	c := caching.New(stub)
	ctx := context.Background()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			switch i % 5 {
			case 0:
				_, _ = c.Dashboard(ctx)
			case 1:
				_, _ = c.Issue(ctx, "issue-1")
			case 2:
				_, _ = c.Catalogs(ctx)
			case 3:
				newTitle := "updated"
				_ = c.UpdateIssue(ctx, "issue-1", domain.UpdateIssueInput{Title: &newTitle})
			case 4:
				_, _ = c.CreateIssue(ctx, domain.CreateIssueInput{Title: "concurrent"})
			}
		}(i)
	}

	wg.Wait()
}

// ---- Hydrate tests ----

// seedMemoryWithIssue seeds a memory.Repository with a single issue and saves
// it to the given path using filestorage.Save. Returns the seeded issue ID.
func seedFileWithIssue(t *testing.T, path string) string {
	t.Helper()
	r := memory.New()
	r.Seed(memory.Issue{
		ID:          "hydrate-1",
		Title:       "hydrated issue",
		Status:      "open",
		Priority:    2,
		Type:        "task",
		Description: "a description",
	})
	if err := filestorage.Save(r, path); err != nil {
		t.Fatalf("seedFileWithIssue: filestorage.Save: %v", err)
	}
	return "hydrate-1"
}

func TestHydrateEmptyPath(t *testing.T) {
	c := caching.New(&stubRepository{})
	if err := c.Hydrate("", ""); err != nil {
		t.Fatalf("Hydrate(\"\") returned error: %v", err)
	}
	// SaveNow should be a no-op (path still empty).
	if err := c.SaveNow(); err != nil {
		t.Fatalf("SaveNow after Hydrate(\"\") returned error: %v", err)
	}
}

func TestHydrateMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	c := caching.New(&stubRepository{})
	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate with missing file returned error: %v", err)
	}
	// After hydrating a missing file, future saves should be allowed (path is set).
	// Verify by calling SaveNow — should succeed (writes an empty repo).
	if err := c.SaveNow(); err != nil {
		t.Fatalf("SaveNow after Hydrate(missing) returned error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist after SaveNow: %v", err)
	}
}

func TestHydrateSchemaMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	// Write a manifest with a wrong schema version by hand (never use Save,
	// which always writes the current version).
	type badManifest struct {
		SchemaVersion int    `json:"schema_version"`
		SyncedAt      string `json:"synced_at"`
		BDCommitHash  string `json:"bd_commit_hash"`
	}
	bm := badManifest{SchemaVersion: 999, SyncedAt: "2026-01-01T00:00:00Z"}
	mBytes, _ := json.MarshalIndent(bm, "", "  ")
	if err := os.WriteFile(path+".manifest.json", mBytes, 0o600); err != nil {
		t.Fatalf("write bad manifest: %v", err)
	}
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty jsonl: %v", err)
	}

	// Hydrate must not return an error — degrade to cold start.
	backingIssueCalls := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			return repository.DashboardData{ClosedTotal: 77}, nil
		},
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			backingIssueCalls++
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
	}
	c := caching.New(stub)
	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate with schema mismatch returned error: %v", err)
	}

	// Issue cache must be empty (cold start): Issue query must go to backing.
	if _, err := c.Issue(context.Background(), "some-id"); err != nil {
		t.Fatalf("Issue after schema-mismatch Hydrate: %v", err)
	}
	if backingIssueCalls != 1 {
		t.Fatalf("expected Issue to hit backing after schema-mismatch Hydrate (cold start); got %d calls", backingIssueCalls)
	}

	// dashboardDirty is true by default on New, so Dashboard also hits backing.
	dashCalls := 0
	stub.dashboardFn = func(_ context.Context) (repository.DashboardData, error) {
		dashCalls++
		return repository.DashboardData{}, nil
	}
	if _, err := c.Dashboard(context.Background()); err != nil {
		t.Fatalf("Dashboard after schema-mismatch Hydrate: %v", err)
	}
	if dashCalls != 1 {
		t.Fatalf("expected Dashboard to hit backing after schema-mismatch Hydrate; got %d calls", dashCalls)
	}
}

func TestHydrateSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")
	issueID := seedFileWithIssue(t, path)

	dashCalls := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCalls++
			return repository.DashboardData{ClosedTotal: 42}, nil
		},
	}
	c := caching.New(stub)
	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate returned unexpected error: %v", err)
	}

	// Issue should be served from the hydrated cache without hitting backing.
	issueCalls := 0
	stub.issueFn = func(_ context.Context, _ string) (domain.IssueDetail, error) {
		issueCalls++
		return domain.IssueDetail{}, errors.New("should not be called")
	}
	got, err := c.Issue(context.Background(), issueID)
	if err != nil {
		t.Fatalf("Issue after Hydrate returned error: %v", err)
	}
	if got.Summary.ID != issueID {
		t.Fatalf("Issue: got ID=%q, want %q", got.Summary.ID, issueID)
	}
	if got.Summary.Title != "hydrated issue" {
		t.Fatalf("Issue: got Title=%q, want %q", got.Summary.Title, "hydrated issue")
	}
	if issueCalls != 0 {
		t.Fatalf("Issue should be served from cache after Hydrate; got %d backing calls", issueCalls)
	}

	// dashboardDirty must be true: next Dashboard call must hit backing.
	if _, err := c.Dashboard(context.Background()); err != nil {
		t.Fatalf("Dashboard after Hydrate returned error: %v", err)
	}
	if dashCalls != 1 {
		t.Fatalf("expected Dashboard to hit backing after Hydrate (dashboardDirty=true); got %d calls", dashCalls)
	}
}

func TestHydrateOtherError(t *testing.T) {
	dir := t.TempDir()
	loadPath := filepath.Join(dir, "prior-session", "repo.jsonl")
	if err := os.MkdirAll(filepath.Dir(loadPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writePath := filepath.Join(dir, "own-session", "repo.jsonl")
	if err := os.MkdirAll(filepath.Dir(writePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a valid manifest but a corrupt JSONL file at loadPath.
	type goodManifest struct {
		SchemaVersion int    `json:"schema_version"`
		SyncedAt      string `json:"synced_at"`
		BDCommitHash  string `json:"bd_commit_hash"`
	}
	gm := goodManifest{SchemaVersion: filestorage.SchemaVersion, SyncedAt: "2026-01-01T00:00:00Z"}
	mBytes, _ := json.MarshalIndent(gm, "", "  ")
	if err := os.WriteFile(loadPath+".manifest.json", mBytes, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(loadPath, []byte("not-valid-json\n"), 0o600); err != nil {
		t.Fatalf("write bad jsonl: %v", err)
	}

	c := caching.New(&stubRepository{})
	err := c.Hydrate(loadPath, writePath)
	if err == nil {
		t.Fatal("Hydrate with corrupt JSONL: expected error, got nil")
	}

	// writePath IS set even on load error (own session path is safe to write to).
	// SaveNow should write to writePath, not to the corrupt loadPath.
	if saveErr := c.SaveNow(); saveErr != nil {
		t.Fatalf("SaveNow after failed Hydrate returned error: %v", saveErr)
	}
	// The corrupt load file must NOT have been overwritten.
	contents, _ := os.ReadFile(loadPath)
	if string(contents) != "not-valid-json\n" {
		t.Fatalf("corrupt load file was overwritten after failed Hydrate")
	}
	// writePath should have been written by SaveNow.
	if _, statErr := os.Stat(writePath); statErr != nil {
		t.Fatalf("expected writePath to be written after SaveNow, got: %v", statErr)
	}
}

// ---- SaveNow tests ----

func TestSaveNowEmptyPath(t *testing.T) {
	c := caching.New(&stubRepository{})
	// No Hydrate called → cacheFilePath is empty → no-op.
	if err := c.SaveNow(); err != nil {
		t.Fatalf("SaveNow with empty path returned error: %v", err)
	}
}

func TestSaveNowSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	// Hydrate a fresh (non-existent) path so cacheFilePath is set.
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id, Title: "from backing"}}, nil
		},
	}
	c := caching.New(stub)
	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// Populate cache via Issue fetch (cache miss → backing seed).
	if _, err := c.Issue(context.Background(), "saved-1"); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// SaveNow should write the file.
	if err := c.SaveNow(); err != nil {
		t.Fatalf("SaveNow returned error: %v", err)
	}

	// Verify the file exists and contains saved-1.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file after SaveNow: %v", err)
	}

	// Reload via a fresh caching repo and verify the issue is present.
	c2 := caching.New(&stubRepository{})
	if err := c2.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate c2: %v", err)
	}
	got, err := c2.Issue(context.Background(), "saved-1")
	if err != nil {
		t.Fatalf("Issue on reloaded repo: %v", err)
	}
	if got.Summary.ID != "saved-1" {
		t.Fatalf("reloaded issue: got ID=%q, want saved-1", got.Summary.ID)
	}
}

func TestSaveNowConcurrentMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id, Title: "concurrent"}}, nil
		},
		updateIssueFn: func(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
			return nil
		},
	}
	c := caching.New(stub)
	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	// Seed an issue.
	if _, err := c.Issue(context.Background(), "conc-1"); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 50 {
			newTitle := "mutated"
			_ = c.UpdateIssue(context.Background(), "conc-1", domain.UpdateIssueInput{Title: &newTitle})
		}
	}()
	go func() {
		defer wg.Done()
		for range 50 {
			_ = c.SaveNow()
		}
	}()
	wg.Wait()

	// Final save: file must be well-formed JSON.
	if err := c.SaveNow(); err != nil {
		t.Fatalf("final SaveNow: %v", err)
	}
	if _, err := filestorage.Load(path); err != nil {
		t.Fatalf("Load after concurrent mutation+save: %v", err)
	}
}

func TestSaveNowError(t *testing.T) {
	dir := t.TempDir()
	// Point path to a non-existent subdirectory — os.Rename will fail.
	path := filepath.Join(dir, "does", "not", "exist", "repo.jsonl")

	stub := &stubRepository{}
	c := caching.New(stub)
	// Manually set cacheFilePath via Hydrate — the missing parent will cause
	// the "file does not exist" branch to fire, setting cacheFilePath, but the
	// parent directory for writing also doesn't exist so Save will fail.
	// Create the manifest path so Load sees "missing jsonl" not "missing manifest":
	// Simplest: use a path whose parent doesn't exist — both manifest and JSONL
	// reads will fail with ErrNotExist, triggering cold-start (cacheFilePath set),
	// then Save fails because rename can't create the missing parent dirs.
	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if err := c.SaveNow(); err == nil {
		t.Fatal("SaveNow to non-existent parent dir: expected error, got nil")
	}
}

// ---- Periodic save tick test ----

func TestPeriodicSaveTickFires(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id, Title: "tick-saved"}}, nil
		},
	}
	fn, _ := vcStatusFuncFromSlice([]string{"hash-a"})
	c := caching.New(stub,
		caching.WithVCStatusFunc(fn),
		caching.WithRefreshInterval(10*time.Second),  // slow refresh
		caching.WithSaveInterval(5*time.Millisecond), // fast save
	)
	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// Seed an issue so there's something to save.
	if _, err := c.Issue(context.Background(), "tick-1"); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	ctx := context.Background()
	c.Start(ctx)

	// Give the save ticker time to fire at least once.
	time.Sleep(100 * time.Millisecond)
	c.Stop()

	// Verify the file was written.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file after periodic save: %v", err)
	}
	// Verify file is well-formed.
	if _, err := filestorage.Load(path); err != nil {
		t.Fatalf("Load after periodic save: %v", err)
	}
}

// ---- Hash-based hydrate tests ----

// seedFileWithHash saves a memory.Repository to path with an explicit bd commit
// hash, returning the seeded issue ID.
func seedFileWithHash(t *testing.T, path string, hash string) string {
	t.Helper()
	r := memory.New()
	r.Seed(memory.Issue{
		ID:          "hash-issue-1",
		Title:       "hash-seeded issue",
		Status:      "open",
		Priority:    1,
		Type:        "task",
		Description: "seeded for hash test",
	})
	if err := filestorage.SaveWithHash(r, path, hash); err != nil {
		t.Fatalf("seedFileWithHash: filestorage.SaveWithHash: %v", err)
	}
	return "hash-issue-1"
}

func TestHydrateMatchingHashSkipsDashboardRefetch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")
	const matchHash = "HASH-X"

	// Seed the file with a known issue so Dashboard() returns non-empty data.
	r := memory.New()
	r.Seed(memory.Issue{
		ID:     "dash-issue-1",
		Title:  "dashboard test issue",
		Status: "open",
		Type:   "task",
	})
	if err := filestorage.SaveWithHash(r, path, matchHash); err != nil {
		t.Fatalf("SaveWithHash: %v", err)
	}

	dashCalls := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCalls++
			return repository.DashboardData{ClosedTotal: 99}, nil
		},
	}
	vcFn, _ := vcStatusFuncFromSlice([]string{matchHash})
	c := caching.New(stub, caching.WithVCStatusFunc(vcFn))

	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// dashboardDirty should be false: Dashboard must NOT hit backing.
	got, err := c.Dashboard(context.Background())
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dashCalls != 0 {
		t.Fatalf("matching hash: expected 0 backing Dashboard calls, got %d", dashCalls)
	}
	// The dashboard should reflect the seeded issue (1 ready issue, open with no deps).
	if len(got.ReadyExplain.Ready) != 1 {
		t.Errorf("matching hash: expected 1 ready issue from hydrated memory, got %d", len(got.ReadyExplain.Ready))
	}
}

func TestHydrateMatchingHashSeedsLastHash(t *testing.T) {
	// When Hydrate finds a matching hash it should seed lastHash so the first
	// RefreshIfChanged call with the same hash does NOT invalidate the cache.
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")
	const matchHash = "HASH-SEED"

	seedFileWithHash(t, path, matchHash)

	dashCalls := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCalls++
			return repository.DashboardData{ClosedTotal: 5}, nil
		},
	}
	// vcStatusFunc always returns the same hash.
	vcFn, _ := vcStatusFuncFromSlice([]string{matchHash, matchHash})
	c := caching.New(stub, caching.WithVCStatusFunc(vcFn))

	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	// Confirm we're not dirty yet.
	if _, err := c.Dashboard(context.Background()); err != nil {
		t.Fatalf("Dashboard after Hydrate: %v", err)
	}
	if dashCalls != 0 {
		t.Fatalf("matching hash: expected 0 backing calls after Hydrate, got %d", dashCalls)
	}

	// First RefreshIfChanged: lastHash is already seeded so same hash should
	// NOT mark the cache dirty.
	c.RefreshIfChanged(context.Background())

	if _, err := c.Dashboard(context.Background()); err != nil {
		t.Fatalf("Dashboard after RefreshIfChanged: %v", err)
	}
	if dashCalls != 0 {
		t.Fatalf("same hash after Hydrate: expected 0 backing calls, got %d", dashCalls)
	}
}

func TestHydrateMismatchedHashKeepsDirty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	seedFileWithHash(t, path, "HASH-X")

	dashCalls := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCalls++
			return repository.DashboardData{ClosedTotal: 7}, nil
		},
	}
	// vcStatusFunc returns a DIFFERENT hash than what was persisted.
	vcFn, _ := vcStatusFuncFromSlice([]string{"HASH-Y"})
	c := caching.New(stub, caching.WithVCStatusFunc(vcFn))

	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// dashboardDirty must be true: first Dashboard call hits backing.
	if _, err := c.Dashboard(context.Background()); err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dashCalls != 1 {
		t.Fatalf("mismatched hash: expected 1 backing Dashboard call, got %d", dashCalls)
	}
}

// TestHydrate_HashMismatch_NoStalePerID verifies that when Hydrate detects a
// confirmed hash mismatch (persisted hash != current hash), the stale loaded
// per-ID data is NOT swapped into the in-memory cache. Issue calls must
// fall through to the backing store and return current data, not the
// session-A stale values that were saved to disk.
func TestHydrate_HashMismatch_NoStalePerID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	// Pre-populate disk with issue I having Title="old" and manifest hash="X".
	r := memory.New()
	r.Seed(memory.Issue{
		ID:     "hash-issue-1",
		Title:  "old",
		Status: "open",
		Type:   "task",
	})
	if err := filestorage.SaveWithHash(r, path, "HASH-X"); err != nil {
		t.Fatalf("SaveWithHash: %v", err)
	}

	// Backing returns the current (new) data for issue I.
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{
				Summary: domain.IssueSummary{
					ID:    id,
					Title: "new from backing",
				},
			}, nil
		},
	}
	// vcStatusFunc returns "HASH-Y" — a mismatch with the persisted "HASH-X".
	vcFn, _ := vcStatusFuncFromSlice([]string{"HASH-Y"})
	c := caching.New(stub, caching.WithVCStatusFunc(vcFn))

	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// Issue("hash-issue-1") must NOT serve the stale Title="old" from disk.
	// It must fall through to backing and return Title="new from backing".
	got, err := c.Issue(context.Background(), "hash-issue-1")
	if err != nil {
		t.Fatalf("Issue after hash-mismatch Hydrate: %v", err)
	}
	if got.Summary.Title == "old" {
		t.Fatalf("stale per-ID data served after hash mismatch: got Title=%q, want %q",
			got.Summary.Title, "new from backing")
	}
	if got.Summary.Title != "new from backing" {
		t.Fatalf("Issue: got Title=%q, want %q", got.Summary.Title, "new from backing")
	}
}

func TestHydrateEmptyPersistedHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	// Save without a hash (legacy file or shutdown without vcStatusFunc).
	seedFileWithIssue(t, path) // uses filestorage.Save → empty hash

	dashCalls := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCalls++
			return repository.DashboardData{ClosedTotal: 3}, nil
		},
	}
	vcFn, _ := vcStatusFuncFromSlice([]string{"HASH-Z"})
	c := caching.New(stub, caching.WithVCStatusFunc(vcFn))

	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// Empty persisted hash → safe default → dashboardDirty=true.
	if _, err := c.Dashboard(context.Background()); err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dashCalls != 1 {
		t.Fatalf("empty persisted hash: expected 1 backing Dashboard call, got %d", dashCalls)
	}
}

func TestHydrateVCStatusFuncError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	seedFileWithHash(t, path, "HASH-X")

	dashCalls := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCalls++
			return repository.DashboardData{ClosedTotal: 2}, nil
		},
	}
	// vcStatusFunc returns an error.
	errVC := errors.New("vc failure")
	vcFn := func(_ context.Context) (string, error) {
		return "", errVC
	}
	c := caching.New(stub, caching.WithVCStatusFunc(vcFn))

	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// vcStatusFunc error → safe default → dashboardDirty=true.
	if _, err := c.Dashboard(context.Background()); err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dashCalls != 1 {
		t.Fatalf("vcStatusFunc error: expected 1 backing Dashboard call, got %d", dashCalls)
	}
}

func TestHydrateNoVCStatusFunc(t *testing.T) {
	// vcStatusFunc == nil → safe default → dashboardDirty=true even if hash persisted.
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	seedFileWithHash(t, path, "HASH-X")

	dashCalls := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCalls++
			return repository.DashboardData{ClosedTotal: 1}, nil
		},
	}
	// No WithVCStatusFunc → vcStatusFunc is nil.
	c := caching.New(stub)

	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	if _, err := c.Dashboard(context.Background()); err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dashCalls != 1 {
		t.Fatalf("no vcStatusFunc: expected 1 backing Dashboard call, got %d", dashCalls)
	}
}

// ---- SaveNow hash persistence tests ----

func TestSaveNowWritesCurrentHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	const wantHash = "HASH-Z"
	vcFn, _ := vcStatusFuncFromSlice([]string{wantHash})
	c := caching.New(&stubRepository{}, caching.WithVCStatusFunc(vcFn))

	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if err := c.SaveNow(); err != nil {
		t.Fatalf("SaveNow: %v", err)
	}

	_, manifest, err := filestorage.LoadWithManifest(path)
	if err != nil {
		t.Fatalf("LoadWithManifest: %v", err)
	}
	if manifest.BDCommitHash != wantHash {
		t.Errorf("BDCommitHash: got %q, want %q", manifest.BDCommitHash, wantHash)
	}
}

func TestSaveNowWithoutVCStatusFunc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	// No vcStatusFunc.
	c := caching.New(&stubRepository{})
	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if err := c.SaveNow(); err != nil {
		t.Fatalf("SaveNow: unexpected error: %v", err)
	}

	_, manifest, err := filestorage.LoadWithManifest(path)
	if err != nil {
		t.Fatalf("LoadWithManifest: %v", err)
	}
	if manifest.BDCommitHash != "" {
		t.Errorf("BDCommitHash: got %q, want empty string", manifest.BDCommitHash)
	}
}

func TestSaveNowVCStatusErrorStillSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	errVC := errors.New("vc failure")
	vcFn := func(_ context.Context) (string, error) {
		return "", errVC
	}
	c := caching.New(&stubRepository{}, caching.WithVCStatusFunc(vcFn))
	if err := c.Hydrate(path, path); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	// SaveNow must not return error when vcStatusFunc fails.
	if err := c.SaveNow(); err != nil {
		t.Fatalf("SaveNow with vcStatusFunc error: unexpected error: %v", err)
	}

	_, manifest, err := filestorage.LoadWithManifest(path)
	if err != nil {
		t.Fatalf("LoadWithManifest: %v", err)
	}
	if manifest.BDCommitHash != "" {
		t.Errorf("BDCommitHash: got %q, want empty string", manifest.BDCommitHash)
	}
}

// ---- Round-trip test ----

func TestRoundTripHydrateThenSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo.jsonl")

	// First session: hydrate empty path → populate → save.
	stub1 := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{
				Summary: domain.IssueSummary{
					ID:       id,
					Title:    "round-trip issue",
					Priority: 3,
					Type:     "bug",
				},
				Description: "round-trip description",
			}, nil
		},
	}
	c1 := caching.New(stub1)
	if err := c1.Hydrate(path, path); err != nil {
		t.Fatalf("session1 Hydrate: %v", err)
	}
	if _, err := c1.Issue(context.Background(), "rt-1"); err != nil {
		t.Fatalf("session1 Issue: %v", err)
	}
	if err := c1.SaveNow(); err != nil {
		t.Fatalf("session1 SaveNow: %v", err)
	}

	// Second session: hydrate from file → verify mutations preserved.
	c2 := caching.New(&stubRepository{
		issueFn: func(_ context.Context, _ string) (domain.IssueDetail, error) {
			return domain.IssueDetail{}, errors.New("backing should not be called")
		},
	})
	if err := c2.Hydrate(path, path); err != nil {
		t.Fatalf("session2 Hydrate: %v", err)
	}
	got, err := c2.Issue(context.Background(), "rt-1")
	if err != nil {
		t.Fatalf("session2 Issue: %v", err)
	}
	if got.Summary.ID != "rt-1" {
		t.Fatalf("round-trip: got ID=%q, want rt-1", got.Summary.ID)
	}
	if got.Summary.Title != "round-trip issue" {
		t.Fatalf("round-trip: got Title=%q, want %q", got.Summary.Title, "round-trip issue")
	}
	if got.Summary.Priority != 3 {
		t.Fatalf("round-trip: got Priority=%d, want 3", got.Summary.Priority)
	}
	if got.Summary.Type != "bug" {
		t.Fatalf("round-trip: got Type=%q, want bug", got.Summary.Type)
	}
	if got.Description != "round-trip description" {
		t.Fatalf("round-trip: got Description=%q, want %q", got.Description, "round-trip description")
	}
}

// ---- Per-ID isolation tests ----

// TestUpdateIssue_DoesNotBustUnrelatedIssue verifies that UpdateIssue(X) drops
// only Issue(X) from the cache, leaving Issue(Y) cached.
func TestUpdateIssue_DoesNotBustUnrelatedIssue(t *testing.T) {
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
		updateIssueFn: func(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
			return nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Prime both X and Y into the cache.
	if _, err := c.Issue(ctx, "X"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Issue(ctx, "Y"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != 2 {
		t.Fatalf("expected 2 backing calls after priming X and Y, got %d", stub.issueCalls)
	}

	// Update X.
	newTitle := "updated"
	if err := c.UpdateIssue(ctx, "X", domain.UpdateIssueInput{Title: &newTitle}); err != nil {
		t.Fatalf("UpdateIssue(X): unexpected error %v", err)
	}
	if stub.updateIssueCalls != 1 {
		t.Fatalf("expected 1 UpdateIssue backing call, got %d", stub.updateIssueCalls)
	}

	// Re-read X: cache was busted, must hit backing.
	if _, err := c.Issue(ctx, "X"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != 3 {
		t.Fatalf("Issue(X) should refetch after UpdateIssue(X): expected 3 backing calls, got %d", stub.issueCalls)
	}

	// Re-read Y: must still be served from cache (count unchanged).
	if _, err := c.Issue(ctx, "Y"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != 3 {
		t.Fatalf("Issue(Y) should still be cached after UpdateIssue(X): expected 3 backing calls, got %d", stub.issueCalls)
	}
}

// TestCloseIssue_DoesNotBustUnrelatedIssue verifies that CloseIssue(X) drops
// only Issue(X) from the cache, leaving Issue(Y) cached.
func TestCloseIssue_DoesNotBustUnrelatedIssue(t *testing.T) {
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
		closeIssueFn: func(_ context.Context, _ string, _ domain.CloseIssueInput) error {
			return nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Prime both X and Y into the cache.
	if _, err := c.Issue(ctx, "X"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Issue(ctx, "Y"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != 2 {
		t.Fatalf("expected 2 backing calls after priming X and Y, got %d", stub.issueCalls)
	}

	// Close X.
	if err := c.CloseIssue(ctx, "X", domain.CloseIssueInput{Reason: "done"}); err != nil {
		t.Fatalf("CloseIssue(X): unexpected error %v", err)
	}
	if stub.closeIssueCalls != 1 {
		t.Fatalf("expected 1 CloseIssue backing call, got %d", stub.closeIssueCalls)
	}

	// Re-read X: cache was busted, must hit backing.
	if _, err := c.Issue(ctx, "X"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != 3 {
		t.Fatalf("Issue(X) should refetch after CloseIssue(X): expected 3 backing calls, got %d", stub.issueCalls)
	}

	// Re-read Y: must still be served from cache (count unchanged).
	if _, err := c.Issue(ctx, "Y"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != 3 {
		t.Fatalf("Issue(Y) should still be cached after CloseIssue(X): expected 3 backing calls, got %d", stub.issueCalls)
	}
}

// TestAddComment_DoesNotBustUnrelatedIssue verifies that AddComment(X) drops
// only Issue(X) from the cache, leaving Issue(Y) cached.
func TestAddComment_DoesNotBustUnrelatedIssue(t *testing.T) {
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id}}, nil
		},
		addCommentFn: func(_ context.Context, _ string, _ domain.AddCommentInput) error {
			return nil
		},
	}
	c := caching.New(stub)
	ctx := context.Background()

	// Prime both X and Y into the cache.
	if _, err := c.Issue(ctx, "X"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Issue(ctx, "Y"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != 2 {
		t.Fatalf("expected 2 backing calls after priming X and Y, got %d", stub.issueCalls)
	}

	// Add comment to X.
	if err := c.AddComment(ctx, "X", domain.AddCommentInput{Body: "a comment"}); err != nil {
		t.Fatalf("AddComment(X): unexpected error %v", err)
	}
	if stub.addCommentCalls != 1 {
		t.Fatalf("expected 1 AddComment backing call, got %d", stub.addCommentCalls)
	}

	// Re-read X: cache was busted, must hit backing.
	if _, err := c.Issue(ctx, "X"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != 3 {
		t.Fatalf("Issue(X) should refetch after AddComment(X): expected 3 backing calls, got %d", stub.issueCalls)
	}

	// Re-read Y: must still be served from cache (count unchanged).
	if _, err := c.Issue(ctx, "Y"); err != nil {
		t.Fatal(err)
	}
	if stub.issueCalls != 3 {
		t.Fatalf("Issue(Y) should still be cached after AddComment(X): expected 3 backing calls, got %d", stub.issueCalls)
	}
}

// TestHydrateSeparateLoadAndWritePaths verifies that Hydrate(loadPath, writePath)
// loads issue data from loadPath but writes subsequent saves to writePath —
// the two paths are independent. After Hydrate, SaveNow writes to writePath
// and the loadPath file is not modified.
func TestHydrateSeparateLoadAndWritePaths(t *testing.T) {
	dir := t.TempDir()
	loadPath := filepath.Join(dir, "prior-session", "repo.jsonl")
	writePath := filepath.Join(dir, "own-session", "repo.jsonl")

	if err := os.MkdirAll(filepath.Dir(loadPath), 0o755); err != nil {
		t.Fatalf("mkdir loadPath dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(writePath), 0o755); err != nil {
		t.Fatalf("mkdir writePath dir: %v", err)
	}

	// Seed the prior session file with a known issue.
	priorMem := memory.New()
	priorMem.Seed(memory.Issue{
		ID:     "prior-1",
		Title:  "prior session issue",
		Status: "open",
		Type:   "task",
	})
	if err := filestorage.Save(priorMem, loadPath); err != nil {
		t.Fatalf("Save to loadPath: %v", err)
	}

	// Record the original content of loadPath so we can verify it's not modified.
	loadPathOriginalContent, err := os.ReadFile(loadPath)
	if err != nil {
		t.Fatalf("ReadFile loadPath: %v", err)
	}

	// Hydrate a new CachingRepository with separate load and write paths.
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			return domain.IssueDetail{}, errors.New("backing should not be called")
		},
	}
	c := caching.New(stub)
	if err := c.Hydrate(loadPath, writePath); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// The prior session's issue must be accessible from the hydrated cache.
	got, err := c.Issue(context.Background(), "prior-1")
	if err != nil {
		t.Fatalf("Issue after Hydrate: %v", err)
	}
	if got.Summary.ID != "prior-1" {
		t.Fatalf("Issue: got ID=%q, want prior-1", got.Summary.ID)
	}
	if got.Summary.Title != "prior session issue" {
		t.Fatalf("Issue: got Title=%q, want %q", got.Summary.Title, "prior session issue")
	}

	// SaveNow must write to writePath, not loadPath.
	if err := c.SaveNow(); err != nil {
		t.Fatalf("SaveNow: %v", err)
	}

	// writePath must exist after SaveNow.
	if _, statErr := os.Stat(writePath); statErr != nil {
		t.Fatalf("expected writePath to exist after SaveNow, got: %v", statErr)
	}

	// loadPath must not have been modified.
	loadPathCurrentContent, err := os.ReadFile(loadPath)
	if err != nil {
		t.Fatalf("ReadFile loadPath after SaveNow: %v", err)
	}
	if string(loadPathCurrentContent) != string(loadPathOriginalContent) {
		t.Fatal("loadPath was modified by SaveNow; it must be read-only")
	}

	// The written writePath must be a valid cache file containing prior-1.
	loaded, err := filestorage.Load(writePath)
	if err != nil {
		t.Fatalf("Load writePath: %v", err)
	}
	snap := loaded.Snapshot()
	found := false
	for _, s := range snap {
		if s.ID == "prior-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected prior-1 in writePath snapshot, got: %v", snap)
	}
}

// TestSaveNow_AtomicWithRespectToReset verifies that SaveNow captures the
// memory snapshot atomically under c.mu.RLock, so that a concurrent
// RefreshIfChanged (which calls memory.Reset under c.mu.Lock) cannot race
// between pointer capture and snapshot. Without the fix, SaveNow released
// c.mu before calling Snapshot(), allowing Reset to zero the map first and
// producing a 0-issue JSONL that overwrites a valid prior-session cache.
//
// Two sub-tests are run:
//
//  1. Sequential correctness: SaveNow is called first (while memory has N issues),
//     then RefreshIfChanged triggers Reset. The persisted JSONL must still contain
//     N issues — SaveNow's snapshot was taken before Reset ran.
//
//  2. Concurrent race detection: SaveNow and RefreshIfChanged are called from
//     separate goroutines. The JSONL must be a well-formed cache file (Load must
//     succeed) and no data race must be reported by the -race detector.
//     Repeated 20 times with alternating hashes to stress the scheduler.
//
// Run with: go test -race -count=10 ./internal/repository/caching/...
func TestSaveNow_AtomicWithRespectToReset(t *testing.T) {
	const issuesPerRun = 3
	ctx := context.Background()

	// seedIssues populates c with N issues via cache-miss path.
	seedIssues := func(t *testing.T, c *caching.CachingRepository, n int, prefix string) {
		t.Helper()
		for i := range n {
			id := prefix + string(rune('0'+i))
			if _, err := c.Issue(ctx, id); err != nil {
				t.Fatalf("seedIssues: Issue(%s): %v", id, err)
			}
		}
	}

	t.Run("sequential_correctness", func(t *testing.T) {
		// Strategy:
		//   1. Seed N issues.
		//   2. SaveNow → persists the snapshot. Must contain N issues.
		//   3. RefreshIfChanged(hash-change) → Reset. Memory is now empty.
		//   4. Load the JSONL written in step 2. Must still contain N issues.
		//
		// This directly verifies that SaveNow's snapshot is the pre-Reset state.
		dir := t.TempDir()
		path := filepath.Join(dir, "repo.jsonl")

		stub := &stubRepository{
			issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
				return domain.IssueDetail{Summary: domain.IssueSummary{ID: id, Title: "t-" + id}}, nil
			},
		}

		// Provide two distinct hashes: first call primes baseline, second triggers Reset.
		vcFn, _ := vcStatusFuncFromSlice([]string{"hash-seq-a", "hash-seq-b"})
		c := caching.New(stub, caching.WithVCStatusFunc(vcFn))
		if err := c.Hydrate(path, path); err != nil {
			t.Fatalf("Hydrate: %v", err)
		}

		seedIssues(t, c, issuesPerRun, "seq-")

		// Prime baseline (records hash-seq-a, no invalidation).
		c.RefreshIfChanged(ctx)

		// SaveNow while memory has issuesPerRun issues.
		if err := c.SaveNow(); err != nil {
			t.Fatalf("SaveNow: %v", err)
		}

		// Now trigger Reset via hash change (uses hash-seq-b).
		c.RefreshIfChanged(ctx)

		// Reload and verify: SaveNow persisted the pre-Reset snapshot.
		loaded, err := filestorage.Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		snap := loaded.Snapshot()
		if len(snap) != issuesPerRun {
			t.Fatalf("sequential: JSONL has %d issues, want %d; SaveNow snapshot was not atomic with Reset",
				len(snap), issuesPerRun)
		}
	})

	t.Run("concurrent_race_detector", func(t *testing.T) {
		// Strategy: run SaveNow and RefreshIfChanged(hash-change) concurrently in
		// 20 iterations. After each pair completes, Load must succeed (no corrupt
		// JSONL). The -race detector will catch any unsynchronised access.
		//
		// Whether SaveNow wins or RefreshIfChanged wins the race, the file must be
		// a valid (non-corrupt) JSONL. The fix ensures that whichever order they
		// interleave, Snapshot is always called while c.mu is held.
		const iterations = 20
		for iter := range iterations {
			dir := t.TempDir()
			path := filepath.Join(dir, "repo.jsonl")

			stub := &stubRepository{
				issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
					return domain.IssueDetail{Summary: domain.IssueSummary{ID: id, Title: "c-" + id}}, nil
				},
			}

			// Alternate hash letters per iteration to avoid vcFn exhaustion effects.
			hashA := "hash-a-" + string(rune('A'+iter%26))
			hashB := "hash-b-" + string(rune('A'+iter%26))
			vcFn, _ := vcStatusFuncFromSlice([]string{hashA, hashB})
			c := caching.New(stub, caching.WithVCStatusFunc(vcFn))

			if err := c.Hydrate(path, path); err != nil {
				t.Fatalf("iter %d: Hydrate: %v", iter, err)
			}

			seedIssues(t, c, issuesPerRun, "conc-")

			// Prime baseline (hash-a → records, no Reset).
			c.RefreshIfChanged(ctx)

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				_ = c.SaveNow()
			}()
			go func() {
				defer wg.Done()
				c.RefreshIfChanged(ctx) // uses hash-b → triggers Reset
			}()
			wg.Wait()

			// File must be well-formed regardless of which goroutine won.
			if _, err := filestorage.Load(path); err != nil {
				t.Fatalf("iter %d: Load after concurrent SaveNow+Reset: %v", iter, err)
			}
		}
	})
}
