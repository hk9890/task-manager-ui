package caching_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/repository/caching"
)

// TestColdStart_DashboardFiresBackingReadWhenCacheFileMissing pins
// beads-workbench-znri.2: on the first run against a project that bwb
// has never opened (no on-disk cache file present), the caching backend
// MUST issue a read to the backing Repository when Dashboard() is
// called. The user-visible bug was an empty dashboard with no error —
// the persistent JSON Lines log showed no backing reads fired.
//
// Layered carefully: this test exercises the caching decorator in
// isolation against a stub backing. If this test passes but the
// real-app fixture run still shows an empty board, the bug lives
// further down the stack (the bd-implementation of backing.Dashboard).
func TestColdStart_DashboardFiresBackingReadWhenCacheFileMissing(t *testing.T) {
	dir := t.TempDir()
	missingLoadPath := filepath.Join(dir, "does-not-exist.jsonl")
	writePath := filepath.Join(dir, "repo.jsonl")

	dashCalls := 0
	stub := &stubRepository{
		dashboardFn: func(_ context.Context) (repository.DashboardData, error) {
			dashCalls++
			return repository.DashboardData{
				ClosedTotal: 5,
				InProgress: []domain.IssueSummary{
					{ID: "bwf-1", Title: "In progress"},
				},
			}, nil
		},
	}

	c := caching.New(stub)
	if err := c.Hydrate(context.Background(), missingLoadPath, writePath); err != nil {
		t.Fatalf("Hydrate with missing load path returned error: %v", err)
	}

	data, err := c.Dashboard(context.Background(), repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if dashCalls != 1 {
		t.Fatalf("expected exactly 1 backing Dashboard call on cold start; got %d (znri.2)", dashCalls)
	}
	if data.ClosedTotal != 5 {
		t.Fatalf("expected Dashboard result to propagate from backing (ClosedTotal=5); got %d", data.ClosedTotal)
	}
	if len(data.InProgress) != 1 {
		t.Fatalf("expected Dashboard InProgress slice from backing; got %d entries", len(data.InProgress))
	}
}

// TestColdStart_IssueFiresBackingReadWhenCacheFileMissing covers the
// per-issue read path: on cold start, asking for an Issue must fan out
// to the backing.
func TestColdStart_IssueFiresBackingReadWhenCacheFileMissing(t *testing.T) {
	dir := t.TempDir()
	missingLoadPath := filepath.Join(dir, "does-not-exist.jsonl")
	writePath := filepath.Join(dir, "repo.jsonl")

	var mu sync.Mutex
	issueCalls := 0
	stub := &stubRepository{
		issueFn: func(_ context.Context, id string) (domain.IssueDetail, error) {
			mu.Lock()
			issueCalls++
			mu.Unlock()
			return domain.IssueDetail{Summary: domain.IssueSummary{ID: id, Title: "from backing"}}, nil
		},
	}

	c := caching.New(stub)
	if err := c.Hydrate(context.Background(), missingLoadPath, writePath); err != nil {
		t.Fatalf("Hydrate with missing load path returned error: %v", err)
	}

	detail, err := c.Issue(context.Background(), "bwf-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	mu.Lock()
	got := issueCalls
	mu.Unlock()
	if got != 1 {
		t.Fatalf("expected exactly 1 backing Issue call on cold start; got %d", got)
	}
	if detail.Summary.Title != "from backing" {
		t.Fatalf("expected Issue result to propagate from backing; got %q", detail.Summary.Title)
	}
}
