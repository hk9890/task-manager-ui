package repository_test

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/repository/taskmgr"
)

// conformanceIssue is a backend-agnostic seed record. IDs differ between backends
// (memory accepts explicit IDs; taskmgr generates them), so the conformance
// assertions compare by Title instead.
type conformanceIssue struct {
	title       string
	description string
}

var conformanceSeed = []conformanceIssue{
	{title: "Widget redesign", description: "new chassis layout"},
	{title: "Widget cleanup", description: "tidy imports"},
	{title: "Gadget audit", description: "security review"},
}

func buildMemoryBackend(t *testing.T) repository.Repository {
	t.Helper()
	r := memory.New()
	for i, s := range conformanceSeed {
		r.Seed(memory.Issue{
			ID:          fmt.Sprintf("m-%d", i+1),
			Title:       s.title,
			Description: s.description,
			Status:      "open",
		})
	}
	return r
}

func buildTaskmgrBackend(t *testing.T) repository.Repository {
	t.Helper()
	store, err := tasks.Init(t.TempDir(), "tm")
	if err != nil {
		t.Fatalf("tasks.Init: %v", err)
	}
	r := taskmgr.New(store, taskmgr.WithAuthor("tester"))
	for _, s := range conformanceSeed {
		if _, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{
			Title:       s.title,
			Description: s.description,
		}); err != nil {
			t.Fatalf("CreateIssue(%q): %v", s.title, err)
		}
	}
	return r
}

func searchTitles(t *testing.T, r repository.Repository, query string) []string {
	t.Helper()
	page, err := r.Search(context.Background(), domain.SearchIssuesQuery{Text: query})
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	titles := make([]string, 0, len(page.Results))
	for _, res := range page.Results {
		titles = append(titles, res.Issue.Title)
	}
	sort.Strings(titles)
	return titles
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// buildMemoryBackendWithClosed returns a memory repository seeded with the
// standard conformanceSeed plus one closed issue whose title contains "archived".
// Used by conformance tests that need a closed issue present.
func buildMemoryBackendWithClosed(t *testing.T) repository.Repository {
	t.Helper()
	r := memory.New()
	for i, s := range conformanceSeed {
		r.Seed(memory.Issue{
			ID:          fmt.Sprintf("m-%d", i+1),
			Title:       s.title,
			Description: s.description,
			Status:      "open",
		})
	}
	r.Seed(memory.Issue{
		ID:     "m-closed",
		Title:  "archived widget",
		Status: "closed",
	})
	return r
}

// buildTaskmgrBackendWithClosed returns a taskmgr repository seeded with the
// standard conformanceSeed plus one closed issue whose title contains "archived".
func buildTaskmgrBackendWithClosed(t *testing.T) repository.Repository {
	t.Helper()
	store, err := tasks.Init(t.TempDir(), "tm")
	if err != nil {
		t.Fatalf("tasks.Init: %v", err)
	}
	r := taskmgr.New(store, taskmgr.WithAuthor("tester"))
	for _, s := range conformanceSeed {
		if _, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{
			Title:       s.title,
			Description: s.description,
		}); err != nil {
			t.Fatalf("CreateIssue(%q): %v", s.title, err)
		}
	}
	res, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{
		Title: "archived widget",
	})
	if err != nil {
		t.Fatalf("CreateIssue(archived): %v", err)
	}
	if err := r.CloseIssue(context.Background(), res.IssueID, domain.CloseIssueInput{Reason: "done"}); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}
	return r
}

// TestSearchIncludesClosedIssuesByDefault pins that both backends return closed
// issues in a default-Statuses (empty) search. The taskmgr backend sets
// IncludeClosed:true unconditionally in search.go; the memory backend has no
// exclusion when Statuses is empty. Deleting IncludeClosed:true from the taskmgr
// backend's buildCriteria call must break this test — that is the pin.
//
// The test also guards cross-backend parity: the same "archived" query must find
// the closed issue on both backends (T2 from the 2026-06-27 project review).
func TestSearchIncludesClosedIssuesByDefault(t *testing.T) {
	mem := buildMemoryBackendWithClosed(t)
	tm := buildTaskmgrBackendWithClosed(t)

	for _, tc := range []struct {
		name    string
		backend repository.Repository
	}{
		{"memory", mem},
		{"taskmgr", tm},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			page, err := tc.backend.Search(context.Background(), domain.SearchIssuesQuery{
				Text: "archived",
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			found := false
			for _, res := range page.Results {
				if res.Issue.Status == "closed" {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: default-Statuses search for %q did not return the closed issue; got %v — IncludeClosed must be true",
					tc.name, "archived", page.Results)
			}
		})
	}
}

// TestSearchConformanceAcrossBackends pins the parity contract the memory backend
// claims ("mirrors the task-manager SDK's TextAllWords semantics so search behaves
// identically across the memory and taskmgr backends"): identical text queries
// against equivalently-seeded memory and taskmgr backends must return the same set
// of issues. This is the shared contract the two independent suites previously
// lacked, and it guards against the search-semantics drift the project-review
// flagged (the now-fixed notes-search divergence being one instance).
//
// Seed: {"Widget redesign" desc="new chassis layout"}, {"Widget cleanup" desc="tidy imports"},
// {"Gadget audit" desc="security review"}.
func TestSearchConformanceAcrossBackends(t *testing.T) {
	mem := buildMemoryBackend(t)
	tm := buildTaskmgrBackend(t)

	// Ground-truth table: sorted expected titles for each query.
	// Whole-word queries (unambiguous under both substring and word-boundary
	// matching) exercising single-field, cross-field AND, and no-match cases.
	cases := []struct {
		query      string
		wantTitles []string // sorted; nil means no results expected
	}{
		{"widget", []string{"Widget cleanup", "Widget redesign"}}, // matches both widget titles
		{"widget redesign", []string{"Widget redesign"}},          // AND within a title
		{"chassis", []string{"Widget redesign"}},                  // description only
		{"tidy", []string{"Widget cleanup"}},                      // description only
		{"audit security", []string{"Gadget audit"}},              // AND across title + description
		{"widget absent", nil},                                    // one absent word excludes all
		{"nonexistentxyzzy", nil},                                 // matches nothing
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.query, func(t *testing.T) {
			wantTitles := tc.wantTitles
			if wantTitles == nil {
				wantTitles = []string{}
			}

			memTitles := searchTitles(t, mem, tc.query)
			tmTitles := searchTitles(t, tm, tc.query)

			// Ground-truth assertion: each backend must return the expected titles.
			if !equalStrings(memTitles, wantTitles) {
				t.Errorf("memory search %q: got %v, want %v", tc.query, memTitles, wantTitles)
			}
			if !equalStrings(tmTitles, wantTitles) {
				t.Errorf("taskmgr search %q: got %v, want %v", tc.query, tmTitles, wantTitles)
			}
			// Parity check: backends must also agree with each other.
			if !equalStrings(memTitles, tmTitles) {
				t.Errorf("search %q diverged: memory=%v taskmgr=%v (backends must agree)", tc.query, memTitles, tmTitles)
			}
		})
	}
}
