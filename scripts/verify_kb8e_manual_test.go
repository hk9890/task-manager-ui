//go:build verify_kb8e

// Package verifykb8e is a self-contained end-to-end verification harness for
// the kb8e Repository refactor fix tasks (kb8e.1 – kb8e.15).
//
// It creates a fresh bd project in a temp dir, seeds realistic data via the bd
// CLI, exercises every kb8e.16 checklist item programmatically, and emits a
// structured PASS/FAIL/NOT-OBSERVED report.
//
// Run with:
//
//	go test -tags=verify_kb8e -v ./scripts/...
package verifykb8e_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bd "github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	beadsrepo "github.com/hk9890/beads-workbench/internal/repository/beads"
	"github.com/hk9890/beads-workbench/internal/repository/caching"
	"github.com/hk9890/beads-workbench/internal/repository/filestorage"
	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

// --------------------------------------------------------------------------
// Result tracking
// --------------------------------------------------------------------------

type outcome string

const (
	pass        outcome = "PASS"
	fail        outcome = "FAIL"
	notObserved outcome = "NOT-OBSERVED"
)

type result struct {
	item     string
	status   outcome
	evidence string
}

var results []result

func record(item string, s outcome, evidence string) {
	results = append(results, result{item, s, evidence})
	prefix := fmt.Sprintf("[%s]", s)
	fmt.Printf("%s %s: %s\n", prefix, item, evidence)
}

// --------------------------------------------------------------------------
// bd CLI helper
// --------------------------------------------------------------------------

const bdBinary = "/home/hans/.local/share/mise/installs/github-gastownhall-beads/1.0.4/bd"

// runBD runs a bd command in workDir and returns (combined output, error).
func runBD(workDir string, args ...string) (string, error) {
	cmd := exec.Command(bdBinary, args...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// mustRunBD calls runBD and fatals on error. For harness setup steps only.
func mustRunBD(t *testing.T, workDir string, args ...string) string {
	t.Helper()
	out, err := runBD(workDir, args...)
	if err != nil {
		t.Fatalf("bd %v failed in %s: %v\n%s", args, workDir, err, out)
	}
	return out
}

// --------------------------------------------------------------------------
// Project factory
// --------------------------------------------------------------------------

// newProject creates a fresh bd project in a temp dir and returns its path.
func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustRunBD(t, dir, "init", "--prefix", "v")
	return dir
}

// newRunner builds a bd.CommandRunner for the given project dir.
func newRunner(projectDir string) *bd.CommandRunner {
	return bd.NewCommandRunner(bd.RunnerConfig{
		Command: bdBinary,
		WorkDir: projectDir,
	})
}

// newBeadsRepo builds a beads.Repository for projectDir.
func newBeadsRepo(projectDir string) *beadsrepo.Repository {
	return beadsrepo.New(newRunner(projectDir))
}

// newCachingRepo builds a CachingRepository wrapping a beads.Repository.
// opts are applied after the default WithVCStatusFunc option.
func newCachingRepo(projectDir string, vcFn func(context.Context) (string, error), opts ...caching.Option) *caching.CachingRepository {
	backing := newBeadsRepo(projectDir)
	allOpts := []caching.Option{caching.WithVCStatusFunc(vcFn)}
	allOpts = append(allOpts, opts...)
	return caching.New(backing, allOpts...)
}

// vcStatusFunc returns a func that calls bd vc status on projectDir.
func vcStatusFunc(projectDir string) func(context.Context) (string, error) {
	runner := newRunner(projectDir)
	return func(ctx context.Context) (string, error) {
		return bd.VCStatusHash(ctx, runner)
	}
}

// --------------------------------------------------------------------------
// Main test function
// --------------------------------------------------------------------------

func TestKB8EVerification(t *testing.T) {
	// Ensure bd binary exists.
	if _, err := os.Stat(bdBinary); err != nil {
		t.Fatalf("bd binary not found at %s: %v", bdBinary, err)
	}

	ctx := context.Background()

	// -----------------------------------------------------------------------
	// F1: cache persists
	// Verify SaveNow writes JSONL + manifest that are both non-empty.
	// -----------------------------------------------------------------------
	t.Run("F1_cache_persists", func(t *testing.T) {
		proj := newProject(t)

		// Create an issue so the per-issue cache has something to save.
		issOut := mustRunBD(t, proj, "create", "Issue for F1", "--json")
		issID := extractJSONField(issOut, "id")
		if issID == "" {
			t.Fatalf("could not extract issue ID from: %s", issOut)
		}

		cacheDir := t.TempDir()
		cachePath := filepath.Join(cacheDir, "repo.jsonl")

		c := newCachingRepo(proj, vcStatusFunc(proj))
		// Hydrate wires the cache file path for SaveNow.
		if err := c.Hydrate(ctx, "", cachePath); err != nil {
			record("F1 cache persists", fail, fmt.Sprintf("Hydrate: %v", err))
			return
		}
		// Issue() cache-miss seeds memory so SaveNow has something to write.
		if _, err := c.Issue(ctx, issID); err != nil {
			record("F1 cache persists", fail, fmt.Sprintf("Issue: %v", err))
			return
		}
		if err := c.SaveNow(); err != nil {
			record("F1 cache persists", fail, fmt.Sprintf("SaveNow: %v", err))
			return
		}

		fi, err := os.Stat(cachePath)
		if err != nil {
			record("F1 cache persists", fail, fmt.Sprintf("JSONL file missing: %v", err))
			return
		}
		manifestPath := cachePath + ".manifest.json"
		mfi, err := os.Stat(manifestPath)
		if err != nil {
			record("F1 cache persists", fail, fmt.Sprintf("manifest file missing: %v", err))
			return
		}
		if fi.Size() == 0 {
			record("F1 cache persists", fail, "JSONL file is 0 bytes")
			return
		}
		if mfi.Size() == 0 {
			record("F1 cache persists", fail, "manifest file is 0 bytes")
			return
		}
		m, err := filestorage.LoadManifest(manifestPath)
		if err != nil {
			record("F1 cache persists", fail, fmt.Sprintf("manifest parse: %v", err))
			return
		}
		record("F1 cache persists", pass,
			fmt.Sprintf("cachefile=%s size=%d manifest_hash=%s", cachePath, fi.Size(), m.BDCommitHash))
	})

	// -----------------------------------------------------------------------
	// F2: large issue load
	// Verify filestorage.Load handles SnapshotIssue JSONL lines >64 KiB
	// (tests the bufio scanner buffer fix in filestorage.go).
	// We use memory.SeedDetail + filestorage.Save/Load directly because bd's
	// Dolt backend rejects descriptions larger than its string column limit
	// (~32 KB), but the scanner buffer fix targets the *deserialization* path.
	// The test creates a 70 KB description to verify the 16 MiB scanner cap.
	// -----------------------------------------------------------------------
	t.Run("F2_large_issue_load", func(t *testing.T) {
		// 70 KB description — produces a ~70 KB JSONL line, exceeding the old
		// 64 KB bufio.Scanner default cap that caused "bufio: token too long".
		bigDesc := strings.Repeat("x", 71680) // 70 KiB

		r := memory.New()
		r.SeedDetail(domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:     "v-large",
				Title:  "Large issue F2",
				Status: "open",
				Type:   "task",
			},
			Description: bigDesc,
		})

		cacheDir := t.TempDir()
		cachePath := filepath.Join(cacheDir, "repo.jsonl")
		if err := filestorage.Save(r, cachePath); err != nil {
			record("F2 large issue load", fail, fmt.Sprintf("filestorage.Save: %v", err))
			return
		}

		loaded, err := filestorage.Load(cachePath)
		if err != nil {
			record("F2 large issue load", fail, fmt.Sprintf("filestorage.Load: %v", err))
			return
		}
		detail, err := loaded.Issue(ctx, "v-large")
		if err != nil {
			record("F2 large issue load", fail, fmt.Sprintf("Issue() after Load: %v", err))
			return
		}
		if len(detail.Description) != len(bigDesc) {
			record("F2 large issue load", fail,
				fmt.Sprintf("description_len mismatch: want %d, got %d", len(bigDesc), len(detail.Description)))
			return
		}
		record("F2 large issue load", pass,
			fmt.Sprintf("description_len=%d (>64KiB JSONL line loaded without scanner error)", len(detail.Description)))
	})

	// -----------------------------------------------------------------------
	// F3: parent/related/children round-trip via Save → Load
	// -----------------------------------------------------------------------
	t.Run("F3_parent_related_children_roundtrip", func(t *testing.T) {
		r := memory.New()

		epic := domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:     "v-epic3",
				Title:  "Epic F3",
				Status: "open",
				Type:   "epic",
			},
			ParentGroupBrowser: domain.ParentGroupBrowserContext{
				Children: []domain.IssueReference{
					{ID: "v-c1", Title: "Child 1", Type: "task", Status: "open"},
					{ID: "v-c2", Title: "Child 2", Type: "task", Status: "open"},
				},
			},
			Related: []domain.IssueReference{
				{ID: "v-rel", Title: "Related issue", Type: "task", Status: "open"},
			},
		}
		r.SeedDetail(epic)

		cacheDir := t.TempDir()
		cachePath := filepath.Join(cacheDir, "repo.jsonl")
		if err := filestorage.Save(r, cachePath); err != nil {
			record("F3 parent/related/children round-trip", fail, fmt.Sprintf("Save: %v", err))
			return
		}
		loaded, err := filestorage.Load(cachePath)
		if err != nil {
			record("F3 parent/related/children round-trip", fail, fmt.Sprintf("Load: %v", err))
			return
		}
		detail, err := loaded.Issue(ctx, "v-epic3")
		if err != nil {
			record("F3 parent/related/children round-trip", fail, fmt.Sprintf("Issue: %v", err))
			return
		}

		if len(detail.ParentGroupBrowser.Children) != 2 {
			record("F3 parent/related/children round-trip", fail,
				fmt.Sprintf("children count: want 2, got %d", len(detail.ParentGroupBrowser.Children)))
			return
		}
		if detail.ParentGroupBrowser.Children[0].Title != "Child 1" {
			record("F3 parent/related/children round-trip", fail,
				fmt.Sprintf("child[0].Title: want 'Child 1', got %q", detail.ParentGroupBrowser.Children[0].Title))
			return
		}
		if len(detail.Related) != 1 || detail.Related[0].Title != "Related issue" {
			record("F3 parent/related/children round-trip", fail,
				fmt.Sprintf("related: want [{Related issue}], got %v", detail.Related))
			return
		}
		record("F3 parent/related/children round-trip", pass,
			fmt.Sprintf("children=%d related=%d (titles and refs preserved across Save+Load)",
				len(detail.ParentGroupBrowser.Children), len(detail.Related)))
	})

	// -----------------------------------------------------------------------
	// F4: cache file integrity (never empty/zero-length between two saves)
	// -----------------------------------------------------------------------
	t.Run("F4_cache_file_integrity", func(t *testing.T) {
		proj := newProject(t)
		issOut := mustRunBD(t, proj, "create", "Issue for F4", "--json")
		issID := extractJSONField(issOut, "id")
		if issID == "" {
			t.Fatalf("could not extract issue ID from: %s", issOut)
		}

		cacheDir := t.TempDir()
		cachePath := filepath.Join(cacheDir, "repo.jsonl")

		c := newCachingRepo(proj, vcStatusFunc(proj))
		if err := c.Hydrate(ctx, "", cachePath); err != nil {
			record("F4 cache file integrity", fail, fmt.Sprintf("Hydrate: %v", err))
			return
		}
		// Seed memory via Issue() cache miss.
		if _, err := c.Issue(ctx, issID); err != nil {
			record("F4 cache file integrity", fail, fmt.Sprintf("Issue: %v", err))
			return
		}

		// First SaveNow.
		if err := c.SaveNow(); err != nil {
			record("F4 cache file integrity", fail, fmt.Sprintf("SaveNow #1: %v", err))
			return
		}
		fi1, err := os.Stat(cachePath)
		if err != nil || fi1.Size() == 0 {
			record("F4 cache file integrity", fail,
				fmt.Sprintf("after SaveNow #1: stat err=%v size=%d", err, func() int64 {
					if fi1 != nil {
						return fi1.Size()
					}
					return 0
				}()))
			return
		}

		// Mutate bd state between saves.
		mustRunBD(t, proj, "create", "Extra issue between saves F4")

		// Second SaveNow.
		if err := c.SaveNow(); err != nil {
			record("F4 cache file integrity", fail, fmt.Sprintf("SaveNow #2: %v", err))
			return
		}
		fi2, err := os.Stat(cachePath)
		if err != nil || fi2.Size() == 0 {
			record("F4 cache file integrity", fail,
				fmt.Sprintf("after SaveNow #2: stat err=%v size=%d", err, func() int64 {
					if fi2 != nil {
						return fi2.Size()
					}
					return 0
				}()))
			return
		}
		record("F4 cache file integrity", pass,
			fmt.Sprintf("size_after_save1=%d size_after_save2=%d (both non-zero, atomic renames prevent empty)",
				fi1.Size(), fi2.Size()))
	})

	// -----------------------------------------------------------------------
	// F5: stale per-ID after external change
	// Session A caches title="original". After external bd update, session B
	// Hydrate with mismatching hash must NOT return the stale session-A title.
	// -----------------------------------------------------------------------
	t.Run("F5_stale_per_ID_after_external_change", func(t *testing.T) {
		proj := newProject(t)

		issOut := mustRunBD(t, proj, "create", "original-title-F5", "--json")
		issID := extractJSONField(issOut, "id")
		if issID == "" {
			record("F5 stale per-ID after external change", fail, "could not extract issue ID")
			return
		}

		hashA, err := currentHash(proj)
		if err != nil {
			record("F5 stale per-ID after external change", fail, fmt.Sprintf("hash A: %v", err))
			return
		}

		cacheDir := t.TempDir()
		cachePathA := filepath.Join(cacheDir, "session-A.jsonl")

		// Session A: fetch and save with hash A.
		cA := caching.New(newBeadsRepo(proj), caching.WithVCStatusFunc(staticHashFn(hashA)))
		if err := cA.Hydrate(ctx, "", cachePathA); err != nil {
			record("F5 stale per-ID after external change", fail, fmt.Sprintf("session A Hydrate: %v", err))
			return
		}
		if _, err := cA.Issue(ctx, issID); err != nil {
			record("F5 stale per-ID after external change", fail, fmt.Sprintf("session A Issue: %v", err))
			return
		}
		if err := cA.SaveNow(); err != nil {
			record("F5 stale per-ID after external change", fail, fmt.Sprintf("session A SaveNow: %v", err))
			return
		}

		// Externally change the title.
		mustRunBD(t, proj, "update", issID, "--title", "new-title-F5")

		hashB, err := currentHash(proj)
		if err != nil {
			record("F5 stale per-ID after external change", fail, fmt.Sprintf("hash B: %v", err))
			return
		}

		// Session B: Hydrate with session-A cache but hash B → confirmed mismatch.
		cachePathB := filepath.Join(cacheDir, "session-B.jsonl")
		cB := caching.New(newBeadsRepo(proj), caching.WithVCStatusFunc(staticHashFn(hashB)))
		if err := cB.Hydrate(ctx, cachePathA, cachePathB); err != nil {
			record("F5 stale per-ID after external change", fail, fmt.Sprintf("session B Hydrate: %v", err))
			return
		}

		// Issue(issID) must return the new title (not the stale one from session A).
		detail, err := cB.Issue(ctx, issID)
		if err != nil {
			record("F5 stale per-ID after external change", fail, fmt.Sprintf("session B Issue: %v", err))
			return
		}
		if detail.Summary.Title != "new-title-F5" {
			record("F5 stale per-ID after external change", fail,
				fmt.Sprintf("expected=%q got=%q (stale session-A cache was not discarded)", "new-title-F5", detail.Summary.Title))
			return
		}
		record("F5 stale per-ID after external change", pass,
			fmt.Sprintf("title=%q (confirmedMismatch → stale cache discarded, backing returned fresh value)",
				detail.Summary.Title))
	})

	// -----------------------------------------------------------------------
	// F6: Hydrate-after-Start guard
	// Start MUST be called with a non-nil vcStatusFunc to launch the goroutine.
	// -----------------------------------------------------------------------
	t.Run("F6_Hydrate_after_Start_guard", func(t *testing.T) {
		proj := newProject(t)
		c := caching.New(newBeadsRepo(proj), caching.WithVCStatusFunc(vcStatusFunc(proj)))
		c.Start(ctx)
		defer c.Stop()

		err := c.Hydrate(ctx, "", filepath.Join(t.TempDir(), "should-not-exist.jsonl"))
		if err == nil {
			record("F6 Hydrate-after-Start guard", fail, "Hydrate after Start returned nil; expected error")
			return
		}
		if !strings.Contains(err.Error(), "Start") && !strings.Contains(err.Error(), "before") {
			record("F6 Hydrate-after-Start guard", fail,
				fmt.Sprintf("error %q does not mention 'Start' or 'before'", err.Error()))
			return
		}
		record("F6 Hydrate-after-Start guard", pass,
			fmt.Sprintf("error=%q", err.Error()))
	})

	// -----------------------------------------------------------------------
	// F7: Hydrate Dashboard error → dashboardDirty=true (fallback to backing)
	// Mechanism: vcFn cancels the outer ctx after returning the matching hash,
	// so the subsequent loaded.Dashboard(outerCtx) call inside Hydrate receives
	// a cancelled context. Hydrate must still return nil; the next Dashboard()
	// call must hit the backing store.
	// -----------------------------------------------------------------------
	t.Run("F7_Hydrate_Dashboard_error", func(t *testing.T) {
		proj := newProject(t)
		mustRunBD(t, proj, "create", "Issue for F7")

		cacheDir := t.TempDir()
		cachePath := filepath.Join(cacheDir, "repo.jsonl")

		const matchHash = "HASH-F7-CANCEL"
		seedRepo := memory.New()
		seedRepo.SeedDetail(domain.IssueDetail{
			Summary: domain.IssueSummary{ID: "v-f7", Title: "F7 Issue", Status: "open", Type: "task"},
		})
		if err := filestorage.SaveWithHash(seedRepo, cachePath, matchHash); err != nil {
			t.Fatalf("seed file: %v", err)
		}

		var backingDashCalls atomic.Int32
		countBacking := &countingDashboardRepo{
			Repository: newBeadsRepo(proj),
			count:      &backingDashCalls,
		}

		outerCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		vcFn := func(c context.Context) (string, error) {
			cancel() // cancel outerCtx → loaded.Dashboard(outerCtx) sees ctx.Canceled
			return matchHash, nil
		}

		c := caching.New(countBacking, caching.WithVCStatusFunc(vcFn))
		// Hydrate must return nil even though Dashboard precompute fails.
		if err := c.Hydrate(outerCtx, cachePath, filepath.Join(cacheDir, "write.jsonl")); err != nil {
			record("F7 Hydrate Dashboard error", fail,
				fmt.Sprintf("Hydrate returned unexpected error: %v", err))
			return
		}

		// First Dashboard call must hit backing (dashboardDirty=true).
		if _, err := c.Dashboard(context.Background()); err != nil {
			record("F7 Hydrate Dashboard error", fail,
				fmt.Sprintf("Dashboard after Hydrate: %v", err))
			return
		}
		calls := backingDashCalls.Load()
		if calls < 1 {
			record("F7 Hydrate Dashboard error", fail,
				fmt.Sprintf("expected ≥1 backing Dashboard call; got %d (dashboardDirty was false)", calls))
			return
		}
		record("F7 Hydrate Dashboard error", pass,
			fmt.Sprintf("Hydrate returned nil; subsequent Dashboard hit backing (calls=%d)", calls))
	})

	// -----------------------------------------------------------------------
	// F8: CreateIssue post-create detail
	// After CreateIssue, Issue(id) must return bd's true values
	// (not fabricated/default Status/Priority/Type/CreatedAt).
	// -----------------------------------------------------------------------
	t.Run("F8_CreateIssue_post_create_detail", func(t *testing.T) {
		proj := newProject(t)
		c := newCachingRepo(proj, vcStatusFunc(proj))

		res, err := c.CreateIssue(ctx, domain.CreateIssueInput{Title: "F8 Created Issue"})
		if err != nil {
			record("F8 CreateIssue post-create detail", fail, fmt.Sprintf("CreateIssue: %v", err))
			return
		}
		newID := res.IssueID
		if newID == "" {
			record("F8 CreateIssue post-create detail", fail, "CreateIssue returned empty ID")
			return
		}

		// Issue(newID) cache-misses and fetches from bd.
		detail, err := c.Issue(ctx, newID)
		if err != nil {
			record("F8 CreateIssue post-create detail", fail, fmt.Sprintf("Issue(%s): %v", newID, err))
			return
		}

		if detail.Summary.CreatedAt.IsZero() {
			record("F8 CreateIssue post-create detail", fail,
				"CreatedAt is zero — value was fabricated, not fetched from bd")
			return
		}
		if detail.Summary.Status == "" {
			record("F8 CreateIssue post-create detail", fail,
				"Status is empty — bd did not assign one")
			return
		}
		if detail.Summary.Type == "" {
			record("F8 CreateIssue post-create detail", fail,
				"Type is empty — bd did not assign one")
			return
		}
		record("F8 CreateIssue post-create detail", pass,
			fmt.Sprintf("id=%s status=%q type=%q priority=%d created_at=%s (bd's true values, not fabricated)",
				newID, detail.Summary.Status, detail.Summary.Type, detail.Summary.Priority,
				detail.Summary.CreatedAt.Format(time.RFC3339)))
	})

	// -----------------------------------------------------------------------
	// F9: cross-ref Title on cache hit
	// Issue A has BlockedBy=[{B, "Real Title B"}]. B is NOT seeded.
	// After Save+Load, Issue(A).BlockedBy[0].Title must still be "Real Title B".
	// -----------------------------------------------------------------------
	t.Run("F9_cross_ref_title_on_cache_hit", func(t *testing.T) {
		r := memory.New()
		r.SeedDetail(domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:     "v-A9",
				Title:  "Issue A9",
				Status: "open",
				Type:   "task",
			},
			BlockedBy: []domain.IssueReference{
				{ID: "v-B9", Title: "Real Title B", Type: "task", Status: "open"},
			},
		})

		cacheDir := t.TempDir()
		cachePath := filepath.Join(cacheDir, "repo.jsonl")
		if err := filestorage.Save(r, cachePath); err != nil {
			record("F9 cross-ref Title on cache hit", fail, fmt.Sprintf("Save: %v", err))
			return
		}
		loaded, err := filestorage.Load(cachePath)
		if err != nil {
			record("F9 cross-ref Title on cache hit", fail, fmt.Sprintf("Load: %v", err))
			return
		}
		detail, err := loaded.Issue(ctx, "v-A9")
		if err != nil {
			record("F9 cross-ref Title on cache hit", fail, fmt.Sprintf("Issue(v-A9): %v", err))
			return
		}
		if len(detail.BlockedBy) == 0 {
			record("F9 cross-ref Title on cache hit", fail, "BlockedBy is empty after Load")
			return
		}
		got := detail.BlockedBy[0].Title
		if got != "Real Title B" {
			record("F9 cross-ref Title on cache hit", fail,
				fmt.Sprintf("BlockedBy[0].Title: want %q, got %q", "Real Title B", got))
			return
		}
		record("F9 cross-ref Title on cache hit", pass,
			fmt.Sprintf("BlockedBy[0].Title=%q preserved across Save+Load (v-B9 never seeded)", got))
	})

	// -----------------------------------------------------------------------
	// F10: catalogs after new-label create
	// CreateIssue must invalidate the catalogs cache so the new label is visible
	// on the very next Catalogs() call (without waiting for TTL expiry).
	// -----------------------------------------------------------------------
	t.Run("F10_catalogs_after_new_label_create", func(t *testing.T) {
		proj := newProject(t)
		newLabel := fmt.Sprintf("manual-test-%s", time.Now().Format("20060102"))

		// Long TTL so cache does NOT expire between calls.
		c := newCachingRepo(proj, vcStatusFunc(proj), caching.WithCatalogsTTL(10*time.Minute))

		cats1, err := c.Catalogs(ctx)
		if err != nil {
			record("F10 catalogs after new-label create", fail,
				fmt.Sprintf("pre-create Catalogs: %v", err))
			return
		}
		for _, l := range cats1.Labels {
			if l.Name == newLabel {
				record("F10 catalogs after new-label create", notObserved,
					fmt.Sprintf("label %q already existed; test inconclusive", newLabel))
				return
			}
		}

		if _, err := c.CreateIssue(ctx, domain.CreateIssueInput{
			Title:  "F10 new-label issue",
			Labels: []string{newLabel},
		}); err != nil {
			record("F10 catalogs after new-label create", fail,
				fmt.Sprintf("CreateIssue: %v", err))
			return
		}

		cats2, err := c.Catalogs(ctx)
		if err != nil {
			record("F10 catalogs after new-label create", fail,
				fmt.Sprintf("post-create Catalogs: %v", err))
			return
		}
		found := false
		for _, l := range cats2.Labels {
			if l.Name == newLabel {
				found = true
				break
			}
		}
		if !found {
			record("F10 catalogs after new-label create", fail,
				fmt.Sprintf("label %q not in Catalogs after CreateIssue (labels=%v)", newLabel, cats2.Labels))
			return
		}
		record("F10 catalogs after new-label create", pass,
			fmt.Sprintf("label=%q visible in Catalogs after CreateIssue (TTL cache was invalidated by write)",
				newLabel))
	})

	// -----------------------------------------------------------------------
	// F11: dashboard refresh after AddComment
	// AddComment must mark dashboardDirty so the next Dashboard() re-fetches
	// from backing (because bd advances UpdatedAt on comment, affecting sort).
	// Verified by counting backing Dashboard calls with a counting wrapper.
	// -----------------------------------------------------------------------
	t.Run("F11_dashboard_refresh_after_comment", func(t *testing.T) {
		proj := newProject(t)
		issOut := mustRunBD(t, proj, "create", "Issue for F11", "--json")
		issID := extractJSONField(issOut, "id")
		if issID == "" {
			t.Fatalf("could not extract issue ID from: %s", issOut)
		}

		var dashCalls atomic.Int32
		c := caching.New(
			&countingDashboardRepo{Repository: newBeadsRepo(proj), count: &dashCalls},
			caching.WithVCStatusFunc(vcStatusFunc(proj)),
		)

		// First Dashboard call: cache miss (dashboardDirty=true by default).
		if _, err := c.Dashboard(ctx); err != nil {
			record("F11 dashboard refresh after comment", fail,
				fmt.Sprintf("Dashboard #1: %v", err))
			return
		}
		afterFirst := dashCalls.Load()

		// Second Dashboard call: must be a cache hit.
		if _, err := c.Dashboard(ctx); err != nil {
			record("F11 dashboard refresh after comment", fail,
				fmt.Sprintf("Dashboard #2: %v", err))
			return
		}
		afterSecond := dashCalls.Load()
		if afterSecond != afterFirst {
			record("F11 dashboard refresh after comment", fail,
				fmt.Sprintf("Dashboard #2 hit backing (count %d→%d); expected cache hit",
					afterFirst, afterSecond))
			return
		}

		// AddComment marks dashboardDirty.
		if err := c.AddComment(ctx, issID, domain.AddCommentInput{Body: "F11 test comment"}); err != nil {
			record("F11 dashboard refresh after comment", fail,
				fmt.Sprintf("AddComment: %v", err))
			return
		}

		// Third Dashboard call: must hit backing (dirty after comment).
		if _, err := c.Dashboard(ctx); err != nil {
			record("F11 dashboard refresh after comment", fail,
				fmt.Sprintf("Dashboard #3 post-comment: %v", err))
			return
		}
		afterThird := dashCalls.Load()
		if afterThird <= afterSecond {
			record("F11 dashboard refresh after comment", fail,
				fmt.Sprintf("Dashboard after AddComment did NOT hit backing (calls: %d→%d)",
					afterSecond, afterThird))
			return
		}
		record("F11 dashboard refresh after comment", pass,
			fmt.Sprintf("calls: first_fetch=%d cache_hit=%d after_comment=%d (dashboardDirty set by AddComment)",
				afterFirst, afterSecond, afterThird))
	})

	// -----------------------------------------------------------------------
	// F12: Creator preserved on second view and across Save+Load
	// -----------------------------------------------------------------------
	t.Run("F12_Creator_on_second_view", func(t *testing.T) {
		r := memory.New()
		r.SeedDetail(domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:     "v-F12",
				Title:  "F12 Issue",
				Status: "open",
				Type:   "task",
			},
			Creator: "alice",
		})

		detail1, err := r.Issue(ctx, "v-F12")
		if err != nil {
			record("F12 Creator on second view", fail, fmt.Sprintf("Issue #1: %v", err))
			return
		}
		if detail1.Creator != "alice" {
			record("F12 Creator on second view", fail,
				fmt.Sprintf("call #1 Creator: want 'alice', got %q", detail1.Creator))
			return
		}

		detail2, err := r.Issue(ctx, "v-F12")
		if err != nil {
			record("F12 Creator on second view", fail, fmt.Sprintf("Issue #2: %v", err))
			return
		}
		if detail2.Creator != "alice" {
			record("F12 Creator on second view", fail,
				fmt.Sprintf("call #2 Creator: want 'alice', got %q", detail2.Creator))
			return
		}

		// Also verify Creator survives Save → Load.
		cacheDir := t.TempDir()
		cachePath := filepath.Join(cacheDir, "repo.jsonl")
		if err := filestorage.Save(r, cachePath); err != nil {
			record("F12 Creator on second view", fail, fmt.Sprintf("Save: %v", err))
			return
		}
		loaded, err := filestorage.Load(cachePath)
		if err != nil {
			record("F12 Creator on second view", fail, fmt.Sprintf("Load: %v", err))
			return
		}
		detail3, err := loaded.Issue(ctx, "v-F12")
		if err != nil {
			record("F12 Creator on second view", fail, fmt.Sprintf("Issue after Load: %v", err))
			return
		}
		if detail3.Creator != "alice" {
			record("F12 Creator on second view", fail,
				fmt.Sprintf("after Load Creator: want 'alice', got %q", detail3.Creator))
			return
		}
		record("F12 Creator on second view", pass,
			"Creator='alice' on calls #1, #2, and after Save+Load round-trip")
	})

	// -----------------------------------------------------------------------
	// F13: epic children not shown in Blocks
	// beads.Repository.Issue(epic) must put children in IssueDetail.Children,
	// NOT in IssueDetail.Blocks.
	// -----------------------------------------------------------------------
	t.Run("F13_epic_children_not_in_Blocks", func(t *testing.T) {
		proj := newProject(t)

		epicOut := mustRunBD(t, proj, "create", "F13 Epic", "--type", "epic", "--json")
		epicID := extractJSONField(epicOut, "id")
		if epicID == "" {
			t.Fatalf("could not extract epic ID from: %s", epicOut)
		}

		c1Out := mustRunBD(t, proj, "create", "F13 Child 1", "--parent", epicID, "--json")
		c1ID := extractJSONField(c1Out, "id")
		c2Out := mustRunBD(t, proj, "create", "F13 Child 2", "--parent", epicID, "--json")
		c2ID := extractJSONField(c2Out, "id")

		if c1ID == "" || c2ID == "" {
			t.Fatalf("could not extract child IDs: c1=%s c2=%s", c1ID, c2ID)
		}

		detail, err := newBeadsRepo(proj).Issue(ctx, epicID)
		if err != nil {
			record("F13 epic children not in Blocks", fail,
				fmt.Sprintf("Issue(epic): %v", err))
			return
		}

		// Children must contain c1 and c2.
		childIDs := make(map[string]bool)
		for _, ch := range detail.Children {
			childIDs[ch.ID] = true
		}
		if !childIDs[c1ID] || !childIDs[c2ID] {
			record("F13 epic children not in Blocks", fail,
				fmt.Sprintf("children missing: want [%s,%s], got %v", c1ID, c2ID, detail.Children))
			return
		}

		// Blocks must NOT contain c1 or c2.
		for _, b := range detail.Blocks {
			if b.ID == c1ID || b.ID == c2ID {
				record("F13 epic children not in Blocks", fail,
					fmt.Sprintf("child %s appears in Blocks: %v", b.ID, detail.Blocks))
				return
			}
		}
		record("F13 epic children not in Blocks", pass,
			fmt.Sprintf("epic=%s children=[%s,%s] Blocks=%v (parent-child deps filtered out of Blocks)",
				epicID, c1ID, c2ID, detail.Blocks))
	})

	// -----------------------------------------------------------------------
	// F14: new child visible after external add + RefreshIfChanged
	//
	// This test exercises the parentSiblingCache.Invalidate() path added in
	// kb8e.14. The cache is only populated when a CHILD issue is fetched
	// (hasParent=true), so we must call Issue(childID) — not Issue(epicID) —
	// to prove that the Invalidate() hook actually clears stale sibling data.
	//
	// Flow:
	//   1. Create epic P + child c1 (--parent P)
	//   2. Issue(c1) → hasParent=true → parentChildSiblings(P) → cache[P]=[c1]
	//   3. Prime RefreshIfChanged baseline (hash A)
	//   4. Externally add child c2 (--parent P)
	//   5. Record hash B, store in atomic
	//   6. RefreshIfChanged → detects A→B → memory.Reset() + backing.Invalidate()
	//      (clears parentSiblingCache)
	//   7. Issue(c1) again → cache miss → re-fetches c1 from bd →
	//      parentChildSiblings(P) runs fresh → returns [c1, c2]
	//   8. Assert ParentGroupBrowser.Children contains both c1 and c2
	// -----------------------------------------------------------------------
	t.Run("F14_new_child_visible_after_external_add", func(t *testing.T) {
		proj := newProject(t)

		epicOut := mustRunBD(t, proj, "create", "F14 Epic", "--type", "epic", "--json")
		epicID := extractJSONField(epicOut, "id")
		if epicID == "" {
			t.Fatalf("could not extract epic ID from: %s", epicOut)
		}

		c1Out := mustRunBD(t, proj, "create", "F14 Child 1", "--parent", epicID, "--json")
		c1ID := extractJSONField(c1Out, "id")
		if c1ID == "" {
			t.Fatalf("could not extract c1 ID from: %s", c1Out)
		}

		// Get baseline hash AFTER creating P and c1.
		hashA, err := currentHash(proj)
		if err != nil {
			record("F14 new child visible after external add", fail,
				fmt.Sprintf("hash A: %v", err))
			return
		}

		// Build CachingRepository with a hash function that returns hashA first
		// (to set the baseline), then hashB after mutation.
		var hashB atomic.Value
		hashB.Store("")

		hashFn := func(c context.Context) (string, error) {
			if h := hashB.Load().(string); h != "" {
				return h, nil
			}
			return hashA, nil
		}

		c := caching.New(newBeadsRepo(proj), caching.WithVCStatusFunc(hashFn))

		// Prime the baseline: RefreshIfChanged call 1 sets lastHash = hashA.
		c.RefreshIfChanged(ctx)

		// Fetch CHILD c1 (not the epic) to populate parentSiblingCache[epicID].
		// hasParent=true for c1 → parentChildSiblings(epicID) → cache[epicID]=[c1].
		detail1, err := c.Issue(ctx, c1ID)
		if err != nil {
			record("F14 new child visible after external add", fail,
				fmt.Sprintf("Issue(c1) before add: %v", err))
			return
		}
		// Sanity: c1's ParentGroupBrowser must reference epic as parent.
		if detail1.ParentGroupBrowser.Parent.ID != epicID {
			record("F14 new child visible after external add", fail,
				fmt.Sprintf("c1.ParentGroupBrowser.Parent.ID: want %s, got %s",
					epicID, detail1.ParentGroupBrowser.Parent.ID))
			return
		}
		initialSiblingCount := len(detail1.ParentGroupBrowser.Children)

		// Externally add a second child c2.
		c2Out := mustRunBD(t, proj, "create", "F14 Child 2", "--parent", epicID, "--json")
		c2ID := extractJSONField(c2Out, "id")
		if c2ID == "" {
			t.Fatalf("could not extract c2 ID from: %s", c2Out)
		}

		// Record new hash after mutation.
		h2, err := currentHash(proj)
		if err != nil {
			record("F14 new child visible after external add", fail,
				fmt.Sprintf("hash B: %v", err))
			return
		}
		hashB.Store(h2)

		// RefreshIfChanged sees hash change → Reset memory + Invalidate parentSiblingCache.
		c.RefreshIfChanged(ctx)

		// Re-fetch CHILD c1: cache miss → re-fetches from bd →
		// parentChildSiblings(epicID) runs fresh (cache was cleared) → returns [c1, c2].
		detail2, err := c.Issue(ctx, c1ID)
		if err != nil {
			record("F14 new child visible after external add", fail,
				fmt.Sprintf("Issue(c1) after add: %v", err))
			return
		}

		foundC1, foundC2 := false, false
		for _, ch := range detail2.ParentGroupBrowser.Children {
			if ch.ID == c1ID {
				foundC1 = true
			}
			if ch.ID == c2ID {
				foundC2 = true
			}
		}
		if !foundC1 || !foundC2 {
			record("F14 new child visible after external add", fail,
				fmt.Sprintf("parentSiblingCache.Invalidate() did not expose c2: "+
					"siblings before=%d after=%d; want [%s,%s] in ParentGroupBrowser.Children=%v",
					initialSiblingCount, len(detail2.ParentGroupBrowser.Children),
					c1ID, c2ID, detail2.ParentGroupBrowser.Children))
			return
		}
		record("F14 new child visible after external add", pass,
			fmt.Sprintf("siblings before=%d after=%d; c1=%s and c2=%s both in "+
				"ParentGroupBrowser.Children after RefreshIfChanged (parentSiblingCache.Invalidate confirmed)",
				initialSiblingCount, len(detail2.ParentGroupBrowser.Children), c1ID, c2ID))
	})

	// -----------------------------------------------------------------------
	// F15: search completeness with Limit=0
	// bd Search with Limit=0 must return Completeness==exact, not partial.
	// -----------------------------------------------------------------------
	t.Run("F15_search_completeness", func(t *testing.T) {
		proj := newProject(t)
		mustRunBD(t, proj, "create", "F15 Search Issue A")
		mustRunBD(t, proj, "create", "F15 Search Issue B")

		page, err := newBeadsRepo(proj).Search(ctx, domain.SearchIssuesQuery{
			Text:  "F15",
			Limit: 0, // no limit → caller wants all results
		})
		if err != nil {
			record("F15 search completeness", fail, fmt.Sprintf("Search: %v", err))
			return
		}
		got := page.Metadata.Completeness
		if got != domain.SearchResultCompletenessExact {
			record("F15 search completeness", fail,
				fmt.Sprintf("Completeness: want %q, got %q (Limit=0 was treated as partial)",
					domain.SearchResultCompletenessExact, got))
			return
		}
		record("F15 search completeness", pass,
			fmt.Sprintf("Completeness=%q returned=%d (Limit=0 → exact, not partial or maybe_more)",
				got, page.Metadata.ReturnedCount))
	})

	// -----------------------------------------------------------------------
	// Summary
	// -----------------------------------------------------------------------
	var nPass, nFail, nNotObserved int
	for _, r := range results {
		switch r.status {
		case pass:
			nPass++
		case fail:
			nFail++
		case notObserved:
			nNotObserved++
		}
	}
	fmt.Printf("\nRESULT: %d/%d items passed, %d failed, %d not observed\n",
		nPass, nPass+nFail+nNotObserved, nFail, nNotObserved)

	if nFail > 0 {
		t.Errorf("%d items failed — see output above", nFail)
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// staticHashFn returns a vcStatusFunc that always returns the given hash.
func staticHashFn(hash string) func(context.Context) (string, error) {
	return func(_ context.Context) (string, error) {
		return hash, nil
	}
}

// currentHash returns the current bd vc status hash for projectDir.
func currentHash(projectDir string) (string, error) {
	runner := bd.NewCommandRunner(bd.RunnerConfig{
		Command: bdBinary,
		WorkDir: projectDir,
	})
	return bd.VCStatusHash(context.Background(), runner)
}

// extractJSONField extracts a top-level JSON string field from a JSON object.
// Returns "" if not found or not a string value.
func extractJSONField(jsonStr, field string) string {
	needle := `"` + field + `":`
	idx := strings.Index(jsonStr, needle)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(jsonStr[idx+len(needle):])
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	rest = rest[1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// countingDashboardRepo wraps repository.Repository and counts Dashboard() calls.
type countingDashboardRepo struct {
	repository.Repository
	count *atomic.Int32
}

func (c *countingDashboardRepo) Dashboard(ctx context.Context) (repository.DashboardData, error) {
	c.count.Add(1)
	return c.Repository.Dashboard(ctx)
}
