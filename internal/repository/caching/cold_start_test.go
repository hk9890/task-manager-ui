package caching_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/repository/caching"
	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
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
		dashboardFn: func(_ context.Context, _ repository.DashboardOptions) (repository.DashboardData, error) {
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

// TestColdStart_DegenerateFile_DashboardFansOutToBackingOnFirstCall pins
// fbea Success Criterion #4 (file-present-but-degenerate test).
//
// When a cache file EXISTS but is degenerate — it contains only a small number
// of issues in the memory snapshot and has no dashboardCache in the v2 header
// (as would be produced by a session that saved but never ran Dashboard) — the
// first Dashboard() call MUST fan out to the backing store and return the
// full backing state.
//
// Before fbea.3: Hydrate would precompute dashboardCache from the small memory
// snapshot (1 issue), set dashboardDirty=false, and Dashboard() would return
// the 1-issue result — missing 99+ issues. (Bug: precompute from partial memory.)
//
// After fbea.3: Hydrate detects that the v2 header has no dashboardCache
// (empty/zero-value), leaves dashboardDirty=true, and the first Dashboard()
// call fans out to backing → full set returned.
func TestColdStart_DegenerateFile_DashboardFansOutToBackingOnFirstCall(t *testing.T) {
	const nBackingIssues = 100
	const nFileIssues = 1

	dir := t.TempDir()
	filePath := filepath.Join(dir, "repo.jsonl")
	writePath := filepath.Join(dir, "repo-write.jsonl")
	const matchHash = "degenerate-session-hash"

	// Write a degenerate v2 file: 1 issue in memory, empty dashboardCache in header.
	// This simulates a session that was saved but never fetched Dashboard from backing.
	fileMemory := memory.New()
	fileMemory.Seed(memory.Issue{
		ID:     "degenerate-issue-1",
		Title:  "only issue in degenerate file",
		Status: "closed",
	})
	snapshot := fileMemory.Snapshot()
	// SaveSnapshotV2WithHash with zero-value DashboardData = empty dashboardCache.
	if err := filestorage.SaveSnapshotV2WithHash(snapshot, repository.DashboardData{}, nil, 0, filePath, matchHash); err != nil {
		t.Fatalf("create degenerate fixture: %v", err)
	}

	// Build a backing with nBackingIssues closed issues.
	backingDashCalls := 0
	backingIssues := make([]domain.IssueSummary, nBackingIssues)
	for i := range backingIssues {
		backingIssues[i] = domain.IssueSummary{
			ID:     fmt.Sprintf("bk-%03d", i+1),
			Title:  fmt.Sprintf("Backing issue %d", i+1),
			Status: "closed",
		}
	}
	stub := &stubRepository{
		dashboardFn: func(_ context.Context, _ repository.DashboardOptions) (repository.DashboardData, error) {
			backingDashCalls++
			return repository.DashboardData{
				Closed:      backingIssues,
				ClosedTotal: nBackingIssues,
			}, nil
		},
	}

	// Inject a vcStatusFunc returning the matching hash so Hydrate does NOT
	// treat the file as stale.
	vcFn := func(_ context.Context) (string, error) {
		return matchHash, nil
	}

	c := caching.New(stub, caching.WithVCStatusFunc(vcFn))
	if err := c.Hydrate(context.Background(), filePath, writePath); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// First Dashboard call: must fan out to backing because the file's
	// dashboardCache was empty. Returns nBackingIssues.
	dash, err := c.Dashboard(context.Background(), repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if backingDashCalls != 1 {
		t.Fatalf("expected exactly 1 backing Dashboard call; got %d (fbea.6: degenerate file must trigger re-fetch)", backingDashCalls)
	}
	if len(dash.Closed) != nBackingIssues {
		t.Fatalf("expected %d closed issues from backing; got %d (file only had %d — degenerate)",
			nBackingIssues, len(dash.Closed), nFileIssues)
	}
	if dash.ClosedTotal != nBackingIssues {
		t.Fatalf("expected ClosedTotal=%d; got %d", nBackingIssues, dash.ClosedTotal)
	}

	// Second Dashboard call: must be served from cache (no extra backing call).
	_, err = c.Dashboard(context.Background(), repository.DashboardOptions{})
	if err != nil {
		t.Fatalf("second Dashboard: %v", err)
	}
	if backingDashCalls != 1 {
		t.Fatalf("expected second Dashboard to be served from cache; got %d total backing calls", backingDashCalls)
	}
}
