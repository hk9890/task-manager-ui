package repository_test

import (
	"context"
	"errors"
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

// backends returns the two conformance backends under their display names, for
// clause tests that assert both implementations behave identically.
func backends(t *testing.T) []struct {
	name string
	repo repository.Repository
} {
	t.Helper()
	return []struct {
		name string
		repo repository.Repository
	}{
		{"memory", buildMemoryBackend(t)},
		{"taskmgr", buildTaskmgrBackend(t)},
	}
}

// TestRepositoryContractConformance pins the clauses the Repository interface
// godoc states, one subtest per clause, against both backends. Prose in
// repository.go is not enforcement: before this table existed the interface
// documented an Issue() error contract that neither backend implemented and a
// concurrency guarantee only one of them provided, and nothing failed. A clause
// worth writing down belongs here.
func TestRepositoryContractConformance(t *testing.T) {
	t.Run("Catalogs name sets match across backends", func(t *testing.T) {
		bs := backends(t)
		names := func(r repository.Repository) (statuses, types []string) {
			c, err := r.Catalogs(context.Background())
			if err != nil {
				t.Fatalf("Catalogs: %v", err)
			}
			for _, s := range c.Statuses {
				statuses = append(statuses, s.Name)
			}
			for _, ty := range c.Types {
				types = append(types, ty.Name)
			}
			sort.Strings(statuses)
			sort.Strings(types)
			return statuses, types
		}

		memStatuses, memTypes := names(bs[0].repo)
		tmStatuses, tmTypes := names(bs[1].repo)

		if !equalStrings(memStatuses, tmStatuses) {
			t.Errorf("status catalogs diverged: memory=%v taskmgr=%v — a form fed by the memory fixture would offer values the real store rejects",
				memStatuses, tmStatuses)
		}
		if !equalStrings(memTypes, tmTypes) {
			t.Errorf("type catalogs diverged: memory=%v taskmgr=%v — a form fed by the memory fixture would offer values the real store rejects",
				memTypes, tmTypes)
		}
	})

	t.Run("Issue on an unknown ID returns ErrIssueNotFound", func(t *testing.T) {
		for _, b := range backends(t) {
			_, err := b.repo.Issue(context.Background(), "no-such-issue")
			if !errors.Is(err, repository.ErrIssueNotFound) {
				t.Errorf("%s: Issue(unknown) = %v, want repository.ErrIssueNotFound", b.name, err)
			}
		}
	})

	t.Run("write methods on an unknown ID return ErrorCodeCommandFailed", func(t *testing.T) {
		for _, b := range backends(t) {
			ctx := context.Background()
			status := "closed"
			for op, err := range map[string]error{
				"UpdateIssue": b.repo.UpdateIssue(ctx, "no-such-issue", domain.UpdateIssueInput{Status: &status}),
				"CloseIssue":  b.repo.CloseIssue(ctx, "no-such-issue", domain.CloseIssueInput{Reason: "done"}),
				"AddComment":  b.repo.AddComment(ctx, "no-such-issue", domain.AddCommentInput{Body: "hi"}),
			} {
				var re domain.RepositoryError
				if !errors.As(err, &re) || re.Code != domain.ErrorCodeCommandFailed {
					t.Errorf("%s: %s(unknown) = %v, want domain.RepositoryError with ErrorCodeCommandFailed", b.name, op, err)
				}
			}
		}
	})

	t.Run("a cancelled context is reported as ctx.Err", func(t *testing.T) {
		for _, b := range backends(t) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := b.repo.Dashboard(ctx, repository.DashboardOptions{}); !errors.Is(err, context.Canceled) {
				t.Errorf("%s: Dashboard(cancelled) = %v, want context.Canceled", b.name, err)
			}
			if _, err := b.repo.Search(ctx, domain.SearchIssuesQuery{Text: "widget"}); !errors.Is(err, context.Canceled) {
				t.Errorf("%s: Search(cancelled) = %v, want context.Canceled", b.name, err)
			}
		}
	})
}

// TestSearchScopeExcludesClosedUnlessAsked pins the search scope contract on both
// backends: a default search sees open work only, and IncludeClosed widens it to
// the closed history.
//
// This replaced an inverted pin (TestSearchIncludesClosedIssuesByDefault), which
// required closed issues in every default search. That default did not survive
// contact with a real store: closed issues accumulate without bound, so at ~880
// closed against ~10 open every search returned effectively nothing but finished
// work, and the search UI offered no way to narrow it.
//
// The test also guards cross-backend parity: the widened query must find the same
// closed issue on both backends (T2 from the 2026-06-27 project review).
func TestSearchScopeExcludesClosedUnlessAsked(t *testing.T) {
	mem := buildMemoryBackendWithClosed(t)
	tm := buildTaskmgrBackendWithClosed(t)

	closedInResults := func(t *testing.T, backend repository.Repository, includeClosed bool) bool {
		t.Helper()
		page, err := backend.Search(context.Background(), domain.SearchIssuesQuery{
			Text:          "archived",
			IncludeClosed: includeClosed,
		})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		for _, res := range page.Results {
			if res.Issue.Status == "closed" {
				return true
			}
		}
		return false
	}

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

			if closedInResults(t, tc.backend, false) {
				t.Errorf("%s: default search for %q returned a closed issue — closed history is out of scope unless asked for",
					tc.name, "archived")
			}
			if !closedInResults(t, tc.backend, true) {
				t.Errorf("%s: search for %q with IncludeClosed did not return the closed issue — the widened scope must reach it",
					tc.name, "archived")
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
