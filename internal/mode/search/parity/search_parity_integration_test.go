//go:build integration

// Package parity contains integration tests that assert bwb's SearchIssues
// gateway output matches what `bd search` (or `bd list`/`bd ready`/`bd blocked`)
// returns for the same query on the same database.
//
// # Flag-parity notes
//
// The SearchIssues gateway uses different bd verbs for different query shapes:
//
//   - Empty text + WorkStateAny → `bd list --json --all [--limit N]`
//     (no `bd search` equivalent for "all issues unfiltered")
//   - Text present + WorkStateAny → `bd search <text> --json --status all [--limit N]`
//   - Text present + status filter → `bd search <text> --json --status <s> [--limit N]`
//   - WorkStateReady (any text) → `bd ready --json` + in-memory text/priority filter
//   - WorkStateBlocked (any text) → `bd blocked --json` + in-memory text/priority filter
//
// For ready/blocked cases there is no `bd search` equivalent. These sub-tests
// compare against `bd ready --json` / `bd blocked --json` (raw backend source)
// after applying the same in-memory filtering the gateway applies. The
// divergence is intentional by design; see docs/TESTING.md §known limitations.
package parity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/testing/datasets"
)

const (
	// parityLimit is the page size used for all parity queries. Choose a value
	// large enough that the fixture (3 issues) always returns fewer results than
	// the limit, while remaining realistic for real-repo runs.
	parityLimit = 20

	// firstNIDs is the number of leading IDs to compare in order.
	firstNIDs = 10
)

// queryCase describes one query-matrix entry.
type queryCase struct {
	name  string
	query domain.SearchIssuesQuery
}

// queryMatrix is the fixed set of queries exercised against every dataset.
// The matrix is intentionally small and uses literal strings so tests are
// deterministic and reviewable.
var queryMatrix = []queryCase{
	{
		name:  "empty",
		query: domain.SearchIssuesQuery{Limit: parityLimit},
	},
	{
		name:  "text=test",
		query: domain.SearchIssuesQuery{Text: "test", Limit: parityLimit},
	},
	{
		name:  "text=fix",
		query: domain.SearchIssuesQuery{Text: "fix", Limit: parityLimit},
	},
	{
		name:  "text=render",
		query: domain.SearchIssuesQuery{Text: "render", Limit: parityLimit},
	},
	{
		name:  "text=data",
		query: domain.SearchIssuesQuery{Text: "data", Limit: parityLimit},
	},
	{
		name: "text=test,status=closed",
		query: domain.SearchIssuesQuery{
			Text:     "test",
			Statuses: []string{"closed"},
			Limit:    parityLimit,
		},
	},
	{
		name: "workstate=ready",
		query: domain.SearchIssuesQuery{
			WorkState: domain.WorkStateReady,
			Limit:     parityLimit,
		},
	},
	{
		name: "workstate=blocked",
		query: domain.SearchIssuesQuery{
			WorkState: domain.WorkStateBlocked,
			Limit:     parityLimit,
		},
	},
}

// TestSearchParityFixture runs the full parity matrix against the embedded
// fixture dataset. Always runs (no env gate required).
//
// Routing: stays on minimal anchor — this test verifies base gateway routing
// and count-vs-rows consistency; exact issue IDs are not asserted here but
// the small corpus size ensures results are deterministic.
func TestSearchParityFixture(t *testing.T) {
	t.Parallel()
	ds := datasets.Fixture(t)
	runSearchParityMatrix(t, ds)
}

// TestSearchParityScaleFixture runs the full parity matrix against the scale
// fixture (~590 issues). Gated behind BWB_SCALE_FIXTURE=1 (seeding takes
// several minutes).
//
// Routing: migrated to scale — exercises search corpus depth (keywords appearing
// in 20+ issues), relevance ordering under realistic data, and result-count
// parity when limit < total results.
func TestSearchParityScaleFixture(t *testing.T) {
	t.Parallel()
	ds := datasets.ScaleFixture(t) // skips if BWB_SCALE_FIXTURE != 1
	runSearchParityMatrix(t, ds)
}

// TestSearchParityThisRepo runs the full parity matrix against this
// repository's .beads/ directory. Gated behind BWB_PARITY_THIS_REPO=1.
func TestSearchParityThisRepo(t *testing.T) {
	t.Parallel()
	ds := datasets.ThisRepo(t)
	runSearchParityMatrix(t, ds)
}

// TestSearchParityExternal runs the full parity matrix against an arbitrary
// external repository whose root path is read from BWB_PARITY_EXTERNAL_PATH.
func TestSearchParityExternal(t *testing.T) {
	t.Parallel()
	ds := datasets.External(t)
	runSearchParityMatrix(t, ds)
}

// TestSearchParityExternalMtimeUnchanged verifies that running the External
// search parity matrix does not modify any file under .beads/ (excluding
// Dolt's internal embeddeddolt storage which it manages on every read).
func TestSearchParityExternalMtimeUnchanged(t *testing.T) {
	t.Parallel()

	externalPath := os.Getenv(datasets.EnvParityExternalPath)
	if externalPath == "" {
		t.Skipf("%s not set; skipping mtime-change verification", datasets.EnvParityExternalPath)
	}

	beadsDir := filepath.Join(externalPath, ".beads")
	before := snapshotMtimes(t, beadsDir)

	ds := datasets.External(t)
	runSearchParityMatrix(t, ds)

	after := snapshotMtimes(t, beadsDir)

	for path, beforeMtime := range before {
		if afterMtime, ok := after[path]; !ok {
			t.Errorf("file disappeared after test: %q", path)
		} else if !afterMtime.Equal(beforeMtime) {
			t.Errorf("mtime changed for %q: before=%v after=%v", path, beforeMtime, afterMtime)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("new file created during test: %q", path)
		}
	}
}

// runSearchParityMatrix exercises all queryMatrix entries against ds.
func runSearchParityMatrix(t *testing.T, ds datasets.Dataset) {
	t.Helper()

	repo := datasets.NewRepository(t, ds)
	ctx := context.Background()

	for _, qc := range queryMatrix {
		qc := qc // capture
		t.Run(fmt.Sprintf("%s/%s", ds.Name, qc.name), func(t *testing.T) {
			t.Parallel()

			page, err := repo.Search(ctx, qc.query)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			// Sub-test: ReturnedCount == len(Results) (internal consistency).
			t.Run("count_vs_rows", func(t *testing.T) {
				assertCountVsRows(t, page)
			})

			// Sub-test: parity with bd CLI output.
			t.Run("parity_with_bd", func(t *testing.T) {
				assertParityWithBd(t, ds, qc, page)
			})
		})
	}
}

// assertCountVsRows asserts that Metadata.ReturnedCount == len(Results).
// This catches a count-vs-rows mismatch class where the gateway reports a
// different number than it actually returns in the results slice.
func assertCountVsRows(t *testing.T, page domain.SearchResultPage) {
	t.Helper()

	if page.Metadata.ReturnedCount != len(page.Results) {
		t.Errorf(
			"count-vs-rows mismatch: Metadata.ReturnedCount=%d but len(Results)=%d",
			page.Metadata.ReturnedCount, len(page.Results),
		)
	}
}

// assertParityWithBd compares the gateway's SearchIssues output against the
// appropriate bd CLI command for the given query case.
func assertParityWithBd(t *testing.T, ds datasets.Dataset, qc queryCase, page domain.SearchResultPage) {
	t.Helper()

	q := qc.query

	switch q.WorkState {
	case domain.WorkStateReady:
		assertParityReadyWorkState(t, ds, qc, page)
	case domain.WorkStateBlocked:
		assertParityBlockedWorkState(t, ds, qc, page)
	default:
		if strings.TrimSpace(q.Text) == "" {
			// Empty query routes to bd list --json --all [--limit N]
			assertParityBdList(t, ds, qc, page)
		} else {
			// Text query routes to bd search <text> --json --status <s> [--limit N]
			assertParityBdSearch(t, ds, qc, page)
		}
	}
}

// assertParityBdList compares gateway output (from bd list --all) against
// bd list --json --all --limit N.
func assertParityBdList(t *testing.T, ds datasets.Dataset, qc queryCase, gwPage domain.SearchResultPage) {
	t.Helper()

	q := qc.query

	// Replicate what searchIssuesFromList sends to bd:
	// bd [--readonly] list --json --all [--status ...] [--limit N]
	// When no statuses, it uses --all (which bd list --all does).
	var bdArgs []string
	if len(q.Statuses) == 0 {
		bdArgs = append(bdArgs, "--all")
	} else {
		bdArgs = append(bdArgs, "--status", strings.Join(q.Statuses, ","))
	}

	limit := withOffsetWindow(q.Limit, q.Offset)
	if limit > 0 {
		bdArgs = append(bdArgs, "--limit", fmt.Sprintf("%d", limit))
	}

	raw, err := datasets.BdList(t, ds, bdArgs...)
	if err != nil {
		t.Fatalf("bd list failed (args=%v): %v", bdArgs, err)
	}

	bdItems := decodeBdIssueArray(t, raw)
	bdIDs := extractIDs(bdItems)
	// Apply paginate(offset, limit) to match gateway
	bdIDs = applyPaginate(bdIDs, q.Offset, q.Limit)

	gwIDs := extractResultIDs(gwPage.Results)

	assertIDCountMatch(t, ds.Name, qc.name, "bd list", gwIDs, bdIDs)
	assertLeadingIDsMatch(t, ds.Name, qc.name, "bd list", gwIDs, bdIDs)
	assertCompletenessConsistency(t, gwPage, q.Limit)
}

// assertParityBdSearch compares gateway output (from bd search) against
// bd search <text> --json --status <s> --limit N.
func assertParityBdSearch(t *testing.T, ds datasets.Dataset, qc queryCase, gwPage domain.SearchResultPage) {
	t.Helper()

	q := qc.query
	text := strings.TrimSpace(q.Text)

	// Replicate what SearchIssues sends to bd search:
	// bd search <text> --json --status all|<s> [--limit N]
	var bdArgs []string
	bdArgs = append(bdArgs, text)

	statuses := q.Statuses
	if len(statuses) == 0 {
		statuses = []string{"all"}
	}
	bdArgs = append(bdArgs, "--status", strings.Join(statuses, ","))

	limit := withOffsetWindow(q.Limit, q.Offset)
	if limit > 0 {
		bdArgs = append(bdArgs, "--limit", fmt.Sprintf("%d", limit))
	}

	raw, err := datasets.BdSearch(t, ds, bdArgs...)
	if err != nil {
		t.Fatalf("bd search failed (args=%v): %v", bdArgs, err)
	}

	bdItems := decodeBdIssueArray(t, raw)
	bdIDs := extractIDs(bdItems)
	bdIDs = applyPaginate(bdIDs, q.Offset, q.Limit)

	gwIDs := extractResultIDs(gwPage.Results)

	assertIDCountMatch(t, ds.Name, qc.name, "bd search", gwIDs, bdIDs)
	assertLeadingIDsMatch(t, ds.Name, qc.name, "bd search", gwIDs, bdIDs)
	assertCompletenessConsistency(t, gwPage, q.Limit)
}

// assertParityReadyWorkState compares gateway output (from bd ready +
// in-memory filter) against bd ready --json. The gateway applies additional
// in-memory filtering that bd ready does not natively support, so we replicate
// the same filter logic here.
//
// Design note: `bd search` has no ready-state semantics. The gateway
// intentionally routes through `bd ready --json` and filters in-memory.
// This sub-test therefore validates the in-memory filter correctness, not
// a `bd search` round-trip.
func assertParityReadyWorkState(t *testing.T, ds datasets.Dataset, qc queryCase, gwPage domain.SearchResultPage) {
	t.Helper()

	q := qc.query

	raw, err := datasets.BdReady(t, ds)
	if err != nil {
		t.Fatalf("bd ready failed: %v", err)
	}

	bdItems := decodeBdIssueArray(t, raw)
	// Apply the same in-memory filter the gateway uses (issueMatchesSearchQuery).
	filtered := inMemoryFilter(bdItems, q)
	bdIDs := extractIDs(filtered)
	bdIDs = applyPaginate(bdIDs, q.Offset, q.Limit)

	gwIDs := extractResultIDs(gwPage.Results)

	// For ready/blocked the bd source is different from bd search — document this
	// clearly so failures are traceable.
	assertIDCountMatch(t, ds.Name, qc.name, "bd ready (in-memory)", gwIDs, bdIDs)
	assertLeadingIDsMatch(t, ds.Name, qc.name, "bd ready (in-memory)", gwIDs, bdIDs)
	assertCompletenessConsistency(t, gwPage, q.Limit)
}

// assertParityBlockedWorkState mirrors assertParityReadyWorkState for the
// blocked work-state path.
func assertParityBlockedWorkState(t *testing.T, ds datasets.Dataset, qc queryCase, gwPage domain.SearchResultPage) {
	t.Helper()

	q := qc.query

	raw, err := datasets.BdBlocked(t, ds)
	if err != nil {
		t.Fatalf("bd blocked failed: %v", err)
	}

	bdItems := decodeBdIssueArray(t, raw)
	filtered := inMemoryFilter(bdItems, q)
	bdIDs := extractIDs(filtered)
	bdIDs = applyPaginate(bdIDs, q.Offset, q.Limit)

	gwIDs := extractResultIDs(gwPage.Results)

	assertIDCountMatch(t, ds.Name, qc.name, "bd blocked (in-memory)", gwIDs, bdIDs)
	assertLeadingIDsMatch(t, ds.Name, qc.name, "bd blocked (in-memory)", gwIDs, bdIDs)
	assertCompletenessConsistency(t, gwPage, q.Limit)
}

// assertIDCountMatch fails if the gw result count and bd result count differ.
func assertIDCountMatch(t *testing.T, dataset, query, bdSource string, gwIDs, bdIDs []string) {
	t.Helper()

	if len(gwIDs) != len(bdIDs) {
		t.Errorf(
			"result count mismatch: dataset=%q query=%q bd_source=%q gw_count=%d bd_count=%d\n  gw_ids=%v\n  bd_ids=%v",
			dataset, query, bdSource, len(gwIDs), len(bdIDs), gwIDs, bdIDs,
		)
	}
}

// assertLeadingIDsMatch fails if the first min(firstNIDs, len) IDs differ in order.
func assertLeadingIDsMatch(t *testing.T, dataset, query, bdSource string, gwIDs, bdIDs []string) {
	t.Helper()

	n := firstNIDs
	if len(gwIDs) < n {
		n = len(gwIDs)
	}
	if len(bdIDs) < n {
		n = len(bdIDs)
	}

	for i := 0; i < n; i++ {
		if gwIDs[i] != bdIDs[i] {
			t.Errorf(
				"ID order divergence at index %d: dataset=%q query=%q bd_source=%q gw_id=%q bd_id=%q\n  gw_first_%d=%v\n  bd_first_%d=%v",
				i, dataset, query, bdSource, gwIDs[i], bdIDs[i], n, gwIDs[:n], n, bdIDs[:n],
			)
			return // stop at first divergence for clarity
		}
	}
}

// assertCompletenessConsistency checks that the Completeness flag in the
// gateway metadata is consistent with whether bd hit its limit.
//
// When the gateway returns exactly `limit` results and limit > 0, the
// Completeness must be MaybeMore (bd may have more). When fewer than limit
// results are returned, Completeness should not be MaybeMore (unless the
// source is capped-backend like ready/blocked, which use their own cap).
func assertCompletenessConsistency(t *testing.T, page domain.SearchResultPage, requestedLimit int) {
	t.Helper()

	m := page.Metadata
	if requestedLimit <= 0 {
		// Uncapped: any completeness is fine.
		return
	}

	if len(page.Results) < requestedLimit {
		// Fewer than limit returned → bd did not hit cap → MaybeMore is unexpected
		// for bd_search and bd_list_fallback sources.
		if m.Completeness == domain.SearchResultCompletenessMaybeMore &&
			(m.Source == domain.SearchResultSourceBDSearch || m.Source == domain.SearchResultSourceBDListFallback) {
			t.Errorf(
				"completeness consistency: returned %d < limit %d but Completeness=%q (expected Partial); source=%q",
				len(page.Results), requestedLimit, m.Completeness, m.Source,
			)
		}
	}
}

// --- bd JSON decoding helpers ---

// minimalBdIssue is a minimal struct for decoding bd JSON output. We only
// need the ID field for parity ordering assertions.
type minimalBdIssue struct {
	ID string `json:"id"`
	// Status is used for in-memory filtering in ready/blocked paths.
	Status string `json:"status"`
	// Title is used for text-filter matching in ready/blocked paths.
	Title string `json:"title"`
	// Priority is used for priority-range filtering.
	Priority int `json:"priority"`
	// Assignee is used for assignee filtering.
	Assignee string `json:"assignee"`
	// Labels is used for label filtering.
	Labels []string `json:"labels"`
}

// decodeBdIssueArray decodes bd --json output into a slice of minimal issues.
// bd returns either a JSON array directly or a wrapper object; we handle both.
func decodeBdIssueArray(t *testing.T, raw []byte) []minimalBdIssue {
	t.Helper()

	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) == 0 || trimmed == "null" {
		return nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var items []minimalBdIssue
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatalf("decodeBdIssueArray: failed to decode JSON array: %v\nraw: %s", err, raw)
		}
		return items
	}

	// Some bd commands wrap the array in an object; try common wrapper keys.
	var wrapper struct {
		Issues  []minimalBdIssue `json:"issues"`
		Results []minimalBdIssue `json:"results"`
		Items   []minimalBdIssue `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("decodeBdIssueArray: unrecognised JSON shape: %v\nraw: %s", err, raw)
	}
	if len(wrapper.Issues) > 0 {
		return wrapper.Issues
	}
	if len(wrapper.Results) > 0 {
		return wrapper.Results
	}
	return wrapper.Items
}

// extractIDs returns the ID field from each issue in order.
func extractIDs(items []minimalBdIssue) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

// extractResultIDs returns the ID field from each search result in order.
func extractResultIDs(results []domain.SearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.Issue.ID)
	}
	return ids
}

// applyPaginate replicates the gateway's paginate(offset, limit) helper.
func applyPaginate(ids []string, offset, limit int) []string {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(ids) {
		return []string{}
	}
	page := ids[offset:]
	if limit <= 0 {
		return page
	}
	if len(page) <= limit {
		return page
	}
	return page[:limit]
}

// withOffsetWindow replicates the gateway's withOffsetWindow helper.
func withOffsetWindow(limit, offset int) int {
	if limit <= 0 {
		return 0
	}
	if offset <= 0 {
		return limit
	}
	return limit + offset
}

// inMemoryFilter replicates the gateway's filterIssueSummariesForSearch /
// issueMatchesSearchQuery logic applied to raw bd items.
//
// This is intentionally a faithful replication of the gateway's filtering so
// that ready/blocked parity tests exercise the same logic path without
// importing the internal gateway package.
func inMemoryFilter(items []minimalBdIssue, q domain.SearchIssuesQuery) []minimalBdIssue {
	out := make([]minimalBdIssue, 0, len(items))
	for _, item := range items {
		if !matchesQuery(item, q) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func matchesQuery(item minimalBdIssue, q domain.SearchIssuesQuery) bool {
	if text := strings.TrimSpace(q.Text); text != "" {
		lower := strings.ToLower(text)
		if !strings.Contains(strings.ToLower(item.ID), lower) &&
			!strings.Contains(strings.ToLower(item.Title), lower) {
			return false
		}
	}

	if len(q.Statuses) > 0 {
		found := false
		for _, s := range q.Statuses {
			if item.Status == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if q.Assignee != "" && item.Assignee != q.Assignee {
		return false
	}

	for _, label := range q.Labels {
		if strings.TrimSpace(label) == "" {
			continue
		}
		found := false
		for _, l := range item.Labels {
			if l == label {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if q.PriorityMin != nil && item.Priority < *q.PriorityMin {
		return false
	}
	if q.PriorityMax != nil && item.Priority > *q.PriorityMax {
		return false
	}

	return true
}

// --- mtime helpers ---

// snapshotMtimes walks dir and records every file's mtime keyed by absolute path,
// excluding Dolt's embeddeddolt/ storage directory (read-cache maintenance).
func snapshotMtimes(t *testing.T, dir string) map[string]time.Time {
	t.Helper()

	result := make(map[string]time.Time)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("snapshotMtimes: ReadDir(%q): %v", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == "embeddeddolt" {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("snapshotMtimes: Info(%q): %v", fullPath, err)
		}
		if entry.IsDir() {
			for sub, mtime := range snapshotMtimes(t, fullPath) {
				result[sub] = mtime
			}
			continue
		}
		result[fullPath] = info.ModTime()
	}

	return result
}
