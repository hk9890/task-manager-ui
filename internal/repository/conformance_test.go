package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
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

// buildMemoryBackendWithDoc returns a memory repository seeded with the standard
// conformanceSeed plus one open, dependency-free issue of type "doc".
func buildMemoryBackendWithDoc(t *testing.T) repository.Repository {
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
		ID:     "m-doc",
		Title:  conformanceDocTitle,
		Status: "open",
		Type:   "doc",
	})
	return r
}

// buildTaskmgrBackendWithDoc returns a taskmgr repository seeded with the standard
// conformanceSeed plus one open, dependency-free issue of type "doc".
func buildTaskmgrBackendWithDoc(t *testing.T) repository.Repository {
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
	if _, err := r.CreateIssue(context.Background(), domain.CreateIssueInput{
		Title: conformanceDocTitle,
		Type:  "doc",
	}); err != nil {
		t.Fatalf("CreateIssue(doc): %v", err)
	}
	return r
}

// conformanceDocTitle is the title of the seeded doc-type issue. It shares no
// word with conformanceSeed so an assertion cannot match it by accident.
const conformanceDocTitle = "onboarding handbook"

// TestOpenDocIsNeitherReadyNorBlocked pins that a document never reaches the
// board's work columns on either backend.
//
// tasks.Type.IsWork() is false for "doc" and no other type, and the SDK applies
// it inside Store.Ready/Store.Blocked rather than leaving it to callers
// (TASK-STORAGE-SPEC §9), so the taskmgr backend cannot surface a doc there. The
// memory backend computed Ready and Blocked from status and dependencies alone,
// which put an open doc in Ready — a board the product cannot produce, certified
// by the fixture that exists to stand in for it. Deleting the doc skip from
// memory's Dashboard must break this test; that is the pin.
func TestOpenDocIsNeitherReadyNorBlocked(t *testing.T) {
	for _, b := range []struct {
		name string
		repo repository.Repository
	}{
		{"memory", buildMemoryBackendWithDoc(t)},
		{"taskmgr", buildTaskmgrBackendWithDoc(t)},
	} {
		data, err := b.repo.Dashboard(context.Background(), repository.DashboardOptions{})
		if err != nil {
			t.Fatalf("%s: Dashboard: %v", b.name, err)
		}

		var readyTitles []string
		for _, iss := range data.ReadyExplain.Ready {
			readyTitles = append(readyTitles, iss.Title)
		}
		var blockedTitles []string
		for _, view := range data.ReadyExplain.Blocked {
			blockedTitles = append(blockedTitles, view.Issue.Title)
		}

		for _, c := range []struct {
			set    string
			titles []string
		}{
			{"Ready", readyTitles},
			{"Blocked", blockedTitles},
		} {
			for _, got := range c.titles {
				if got == conformanceDocTitle {
					t.Errorf("%s: ReadyExplain.%s contains the doc %q — docs are not work and must appear in neither set (got %v)",
						b.name, c.set, conformanceDocTitle, c.titles)
				}
			}
		}

		// Guard against the assertion passing because the board is empty: the
		// work-type seed must still be Ready on both backends.
		if len(readyTitles) != len(conformanceSeed) {
			t.Errorf("%s: ReadyExplain.Ready = %v, want the %d work-type seed issues — the doc exclusion must not drop real work",
				b.name, readyTitles, len(conformanceSeed))
		}
	}
}

// freshBackends returns one empty backend of each kind. The conformance seed is
// deliberately absent: write-path assertions build exactly the state they need.
func freshBackends(t *testing.T) []struct {
	name string
	repo repository.Repository
} {
	t.Helper()

	store, err := tasks.Init(t.TempDir(), "tm")
	if err != nil {
		t.Fatalf("tasks.Init: %v", err)
	}

	return []struct {
		name string
		repo repository.Repository
	}{
		{"memory", memory.New()},
		{"taskmgr", taskmgr.New(store, taskmgr.WithAuthor("tester"))},
	}
}

// TestWriteValidationConformance pins that both backends reject the same
// invalid writes. The memory backend validated an empty title on create and
// nothing at all on update, so a UI test seeded with it certified create and
// update flows that the production backend rejects — an uppercase label from
// the create dialog being the concrete case.
func TestWriteValidationConformance(t *testing.T) {
	invalidPriority := 99
	longTitle := strings.Repeat("x", 201)

	creates := []struct {
		name  string
		input domain.CreateIssueInput
	}{
		{"empty title", domain.CreateIssueInput{Title: "   "}},
		{"multi-line title", domain.CreateIssueInput{Title: "first\nsecond"}},
		{"over-long title", domain.CreateIssueInput{Title: longTitle}},
		{"unknown type", domain.CreateIssueInput{Title: "ok", Type: "bogus"}},
		{"priority out of range", domain.CreateIssueInput{Title: "ok", Priority: &invalidPriority}},
		{"uppercase label", domain.CreateIssueInput{Title: "ok", Labels: []string{"UI"}}},
		{"label with a space", domain.CreateIssueInput{Title: "ok", Labels: []string{"needs review"}}},
		{"multi-line assignee", domain.CreateIssueInput{Title: "ok", Assignee: "a\nb"}},
	}

	for _, tc := range creates {
		t.Run("create/"+tc.name, func(t *testing.T) {
			for _, b := range freshBackends(t) {
				_, err := b.repo.CreateIssue(context.Background(), tc.input)
				if err == nil {
					t.Errorf("%s: CreateIssue(%+v) succeeded, want a validation error", b.name, tc.input)
					continue
				}
				var repoErr domain.RepositoryError
				if !errors.As(err, &repoErr) || repoErr.Code != domain.ErrorCodeValidationFailed {
					t.Errorf("%s: CreateIssue error = %v, want ErrorCodeValidationFailed", b.name, err)
				}
			}
		})
	}

	updates := []struct {
		name  string
		input domain.UpdateIssueInput
	}{
		{"empty title", domain.UpdateIssueInput{Title: strPtr("  ")}},
		{"multi-line title", domain.UpdateIssueInput{Title: strPtr("first\nsecond")}},
		{"unknown status", domain.UpdateIssueInput{Status: strPtr("not-a-status")}},
		{"unknown type", domain.UpdateIssueInput{Type: strPtr("bogus")}},
		{"priority out of range", domain.UpdateIssueInput{Priority: &invalidPriority}},
		{"uppercase label", domain.UpdateIssueInput{Labels: []string{"UI"}}},
	}

	for _, tc := range updates {
		t.Run("update/"+tc.name, func(t *testing.T) {
			for _, b := range freshBackends(t) {
				created, err := b.repo.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "subject"})
				if err != nil {
					t.Fatalf("%s: CreateIssue: %v", b.name, err)
				}

				err = b.repo.UpdateIssue(context.Background(), created.IssueID, tc.input)
				if err == nil {
					t.Errorf("%s: UpdateIssue(%+v) succeeded, want a validation error", b.name, tc.input)
					continue
				}
				var repoErr domain.RepositoryError
				if !errors.As(err, &repoErr) || repoErr.Code != domain.ErrorCodeValidationFailed {
					t.Errorf("%s: UpdateIssue error = %v, want ErrorCodeValidationFailed", b.name, err)
				}

				// The rejected update must not have landed.
				detail, err := b.repo.Issue(context.Background(), created.IssueID)
				if err != nil {
					t.Fatalf("%s: Issue: %v", b.name, err)
				}
				if detail.Summary.Title != "subject" {
					t.Errorf("%s: rejected update changed the issue: title = %q", b.name, detail.Summary.Title)
				}
			}
		})
	}
}

// TestCreateDefaultsConformance pins the create-time defaults. An unset priority
// stored P0 on the memory backend and P2 on the production one, so a blank
// Priority field in the create dialog sorted to the top of every column under
// test and to the middle in reality.
func TestCreateDefaultsConformance(t *testing.T) {
	for _, b := range freshBackends(t) {
		created, err := b.repo.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "defaults"})
		if err != nil {
			t.Fatalf("%s: CreateIssue: %v", b.name, err)
		}

		detail, err := b.repo.Issue(context.Background(), created.IssueID)
		if err != nil {
			t.Fatalf("%s: Issue: %v", b.name, err)
		}
		if detail.Summary.Priority != 2 {
			t.Errorf("%s: default priority = %d, want 2", b.name, detail.Summary.Priority)
		}
		if detail.Summary.Type != "task" {
			t.Errorf("%s: default type = %q, want task", b.name, detail.Summary.Type)
		}
		if detail.Summary.Status != "open" {
			t.Errorf("%s: default status = %q, want open", b.name, detail.Summary.Status)
		}
	}
}

// TestReopenClearsCloseFieldsConformance pins the reopen invariant. The memory
// backend assigned the status directly, so a reopened issue kept its closedAt
// and close reason and rendered as a live issue that says it was closed.
func TestReopenClearsCloseFieldsConformance(t *testing.T) {
	for _, b := range freshBackends(t) {
		created, err := b.repo.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "lifecycle"})
		if err != nil {
			t.Fatalf("%s: CreateIssue: %v", b.name, err)
		}
		if err := b.repo.CloseIssue(context.Background(), created.IssueID, domain.CloseIssueInput{Reason: "done"}); err != nil {
			t.Fatalf("%s: CloseIssue: %v", b.name, err)
		}
		if err := b.repo.UpdateIssue(context.Background(), created.IssueID, domain.UpdateIssueInput{Status: strPtr("open")}); err != nil {
			t.Fatalf("%s: UpdateIssue(reopen): %v", b.name, err)
		}

		detail, err := b.repo.Issue(context.Background(), created.IssueID)
		if err != nil {
			t.Fatalf("%s: Issue: %v", b.name, err)
		}
		if detail.Summary.Status != "open" {
			t.Errorf("%s: status after reopen = %q, want open", b.name, detail.Summary.Status)
		}
		if !detail.ClosedAt.IsZero() {
			t.Errorf("%s: reopened issue still carries ClosedAt = %v", b.name, detail.ClosedAt)
		}
		if strings.TrimSpace(detail.CloseReason) != "" {
			t.Errorf("%s: reopened issue still carries CloseReason = %q", b.name, detail.CloseReason)
		}
	}
}

// TestSearchByIssueIDConformance pins that a full issue ID is a search term on
// both backends. The SDK's virtual "text" field is lower(id + title +
// description); the memory backend matched title and description only, so ID
// search worked in production and silently returned nothing under --repo memory.
func TestSearchByIssueIDConformance(t *testing.T) {
	for _, b := range freshBackends(t) {
		created, err := b.repo.CreateIssue(context.Background(), domain.CreateIssueInput{Title: "findable by id"})
		if err != nil {
			t.Fatalf("%s: CreateIssue: %v", b.name, err)
		}

		page, err := b.repo.Search(context.Background(), domain.SearchIssuesQuery{Text: created.IssueID})
		if err != nil {
			t.Fatalf("%s: Search: %v", b.name, err)
		}
		if len(page.Results) != 1 || page.Results[0].Issue.ID != created.IssueID {
			t.Errorf("%s: Search(%q) = %d results, want the issue itself", b.name, created.IssueID, len(page.Results))
		}
	}
}

// TestPriorityBoundsConformance pins the 0..4 range at its edges. The
// out-of-range case in TestWriteValidationConformance uses 99, which leaves the
// bound itself free to drift: a fixture accepting 5 would certify a create the
// production store rejects, and the priority picker is the flow that would ship
// broken.
func TestPriorityBoundsConformance(t *testing.T) {
	accepted := []int{0, 4}
	rejected := []int{-1, 5}

	for _, p := range accepted {
		t.Run(fmt.Sprintf("accepts/%d", p), func(t *testing.T) {
			for _, b := range freshBackends(t) {
				priority := p
				if _, err := b.repo.CreateIssue(context.Background(), domain.CreateIssueInput{
					Title:    "priority edge",
					Priority: &priority,
				}); err != nil {
					t.Errorf("%s: CreateIssue with priority %d failed: %v", b.name, p, err)
				}
			}
		})
	}

	for _, p := range rejected {
		t.Run(fmt.Sprintf("rejects/%d", p), func(t *testing.T) {
			for _, b := range freshBackends(t) {
				priority := p
				_, err := b.repo.CreateIssue(context.Background(), domain.CreateIssueInput{
					Title:    "priority edge",
					Priority: &priority,
				})
				if err == nil {
					t.Errorf("%s: CreateIssue with priority %d succeeded, want a validation error", b.name, p)
					continue
				}
				var repoErr domain.RepositoryError
				if !errors.As(err, &repoErr) || repoErr.Code != domain.ErrorCodeValidationFailed {
					t.Errorf("%s: CreateIssue error = %v, want ErrorCodeValidationFailed", b.name, err)
				}
			}
		})
	}
}

func strPtr(v string) *string { return &v }
