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

// TestSearchConformanceAcrossBackends pins the parity contract the memory backend
// claims ("mirrors the task-manager SDK's TextAllWords semantics so search behaves
// identically across the memory and taskmgr backends"): identical text queries
// against equivalently-seeded memory and taskmgr backends must return the same set
// of issues. This is the shared contract the two independent suites previously
// lacked, and it guards against the search-semantics drift the project-review
// flagged (the now-fixed notes-search divergence being one instance).
func TestSearchConformanceAcrossBackends(t *testing.T) {
	mem := buildMemoryBackend(t)
	tm := buildTaskmgrBackend(t)

	// Whole-word queries (unambiguous under both substring and word-boundary
	// matching) exercising single-field, cross-field AND, and no-match cases.
	queries := []string{
		"widget",           // matches two titles
		"widget redesign",  // AND within a title
		"chassis",          // matches one description
		"tidy",             // matches one description
		"audit security",   // AND across title + description
		"widget absent",    // one word absent -> excludes all
		"nonexistentxyzzy", // matches nothing
	}

	for _, q := range queries {
		q := q
		t.Run(q, func(t *testing.T) {
			memTitles := searchTitles(t, mem, q)
			tmTitles := searchTitles(t, tm, q)
			if !equalStrings(memTitles, tmTitles) {
				t.Errorf("search %q diverged: memory=%v taskmgr=%v (backends must agree)", q, memTitles, tmTitles)
			}
		})
	}
}
