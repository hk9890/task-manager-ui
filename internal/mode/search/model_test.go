package search

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uidetails "github.com/hk9890/task-manager-ui/internal/ui/details"
	uisearch "github.com/hk9890/task-manager-ui/internal/ui/search"
)

// searchRepo bundles the memory repo (for seeding) and the error-injecting
// wrapper (for call tracking and error injection). It satisfies
// repository.Repository via the embedded ErrorInjectingRepository.
type searchRepo struct {
	repo *memoryrepo.Repository
	*fakes.ErrorInjectingRepository
}

// newSearchRepo creates a searchRepo with an empty memory repository.
func newSearchRepo() *searchRepo {
	repo := memoryrepo.New()
	return &searchRepo{
		repo:                     repo,
		ErrorInjectingRepository: fakes.NewErrorInjecting(repo),
	}
}

// hasSearchCall reports whether any Search call appears in calls.
func hasSearchCall(calls []fakes.Call) bool {
	return countSearchCalls(calls) > 0
}

// TestSearchQueryAcceptsSpaceForMultiWord guards the query box against dropping
// spaces. Bubble Tea delivers a lone space as tea.KeySpace (not tea.KeyRunes), so
// the query handler must accept KeySpace too; otherwise multi-word (AND-of-words)
// queries could never be typed and the backend's AND-of-words semantics would be
// unreachable from the UI.
func TestSearchQueryAcceptsSpaceForMultiWord(t *testing.T) {
	gw := newSearchRepo()
	m := NewModel(context.Background(), gw, nil)
	m.SetSize(120, 30)

	for _, r := range "delta gamma" {
		if r == ' ' {
			_ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if m.draftQuery != "delta gamma" {
		t.Fatalf("draftQuery = %q, want %q (space dropped?)", m.draftQuery, "delta gamma")
	}
}

func TestSearchModeTextEntryRendersResultsInProgramHarness(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Backend search", Status: "open", Type: "task", Priority: 1})

	tm := testui.NewTestModelWithSize(t, testui.ControllerAdapter{Controller: NewModel(context.Background(), gw, nil)}, 120, 30)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		view := string(bts)
		return strings.Contains(view, "b│") && strings.Contains(view, "Backend search") && strings.Contains(view, "Content")
	})

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}
}

func TestSearchModeInitLoadsDefaultResultsForEmptyQuery(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Default one", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)

	if !hasSearchCall(gw.Calls()) {
		t.Fatalf("expected empty init query to load default search results, calls=%#v", gw.Calls())
	}

	if !strings.Contains(m.View(0), "Default one") {
		t.Fatalf("expected default results view after init, got:\n%s", m.View(0))
	}
}

func TestSearchModeTextQuerySendsRepositorySearch(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "gw test issue", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)

	callsBefore := len(gw.Calls())
	pressAndResolve(m, testui.SearchTypeTextKeys("gw")...)
	if countSearchCalls(gw.Calls()[callsBefore:]) != 0 {
		t.Fatalf("expected no search call before explicit enter, got %#v", gw.Calls()[callsBefore:])
	}
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Verify the applied query was set to "gw" (observable state instead of arg capture).
	if m.appliedQuery != "gw" {
		t.Fatalf("expected appliedQuery=%q after enter, got %q", "gw", m.appliedQuery)
	}
}

func TestSearchModeFocusNavigationAndSelection(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	// Titles contain "g" so the query "g" matches both.
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Gig one", Status: "open", Type: "task", Priority: 1})
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Gig two", Status: "in_progress", Type: "bug", Priority: 2})
	m := initModel(gw)

	pressAndResolve(m, testui.SearchTypeTextKeys("g")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected initial search focus, got %v", m.focus)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected right in query to be no-op, got %v", m.focus)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected down to move focus to results, got %v", m.focus)
	}

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = applyMessages(m, drainCmd(cmd))
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-2" {
		t.Fatalf("expected down to move selection to tm-2, got %#v", got)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.focus != uisearch.FocusContent {
		t.Fatalf("expected right to move focus to content, got %v", m.focus)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected left to move focus back to results, got %v", m.focus)
	}
}

func TestSearchModeUpOnFirstResultReturnsFocusToQuery(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	// Titles contain "g" so the query "g" matches both.
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Gig one", Status: "open", Type: "task", Priority: 1})
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Gig two", Status: "in_progress", Type: "bug", Priority: 2})
	m := initModel(gw)

	pressAndResolve(m, testui.SearchTypeTextKeys("g")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-1" {
		t.Fatalf("expected first result selected after search, got %#v", got)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected down from query to move focus to results, got %v", m.focus)
	}

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cmd != nil {
		m = applyMessages(m, drainCmd(cmd))
	}
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected up on first result to return focus to query, got %v", m.focus)
	}
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-1" {
		t.Fatalf("expected first result selection to stay on tm-1, got %#v", got)
	}
	// Expect exactly 2 Search calls: init empty-query load + explicit enter search.
	if n := countSearchCalls(gw.Calls()); n != 2 {
		t.Fatalf("expected only init + explicit enter search calls (2), got %d", n)
	}
}

func TestSearchModeClearingQueryRestoresDefaultResults(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	// Seed issues: tm-1 and tm-2 match any query (no filter text), tm-9 matches "x".
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Default first", Status: "open", Type: "task", Priority: 1})
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Default second", Status: "in_progress", Type: "bug", Priority: 2})
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-9", Title: "Filtered x only", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)

	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-1" {
		t.Fatalf("expected initial selection on default results, got %#v", got)
	}

	// Search for "x" — only tm-9 matches.
	pressAndResolve(m, testui.SearchTypeTextKeys("x")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-9" {
		t.Fatalf("expected filtered selection tm-9, got %#v", got)
	}

	// Clear query with backspace and re-submit — default results restore.
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyBackspace})
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.appliedQuery != "" {
		t.Fatalf("expected appliedQuery empty after clear+enter, got %q", m.appliedQuery)
	}
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-1" {
		t.Fatalf("expected selection reset to first default result, got %#v", got)
	}
	if !strings.Contains(m.View(0), "Default first") || !strings.Contains(m.View(0), "Default second") {
		t.Fatalf("expected restored default results in view, got:\n%s", m.View(0))
	}
}

func TestSearchModeRepresentativeStates(t *testing.T) {
	t.Parallel()

	t.Run("error state", func(t *testing.T) {
		m := NewModel(context.Background(), newSearchRepo(), nil)
		_ = m.Update(searchLoadedMsg{err: errors.New("boom")})

		view := m.View(0)
		if !strings.Contains(view, "Search failed.") || !strings.Contains(view, "boom") || !strings.Contains(view, "failed") {
			t.Fatalf("expected error state in view, got:\n%s", view)
		}
	})

	t.Run("no results state", func(t *testing.T) {
		m := NewModel(context.Background(), newSearchRepo(), nil)
		m.draftQuery = "xyz"
		cmd := m.Update(searchLoadedMsg{appliedQuery: "xyz", page: domain.SearchResultPage{}})
		if cmd != nil {
			_ = cmd()
		}

		if !strings.Contains(m.View(0), "No matches for \"xyz\".") {
			t.Fatalf("expected no-results state in view, got:\n%s", m.View(0))
		}
	})

	t.Run("open detail action from results", func(t *testing.T) {
		gw := newSearchRepo()
		// Title contains "b" so the query "b" returns this result.
		gw.repo.Seed(memoryrepo.Issue{ID: "tm-7", Title: "Backend result", Status: "open", Type: "task", Priority: 1})
		m := initModel(gw)
		pressAndResolve(m, testui.SearchTypeTextKeys("b")...)
		pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

		_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatalf("expected action request command on enter")
		}
		msg := cmd()
		testui.AssertActionRequest(t, msg, mode.Search, mode.ActionOpenDetail)
	})
}

func TestSearchModeTabInSearchOnlyCyclesFromQueryFocusAndIsCapturedByMode(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	// Title contains "g" so query "g" matches.
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-7", Title: "Repository result", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)

	pressAndResolve(m, testui.SearchTypeTextKeys("g")...)
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected query focus after typing, got %v", m.focus)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected tab to cycle query->results, got %v", m.focus)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != uisearch.FocusContent {
		t.Fatalf("expected tab to cycle results->content, got %v", m.focus)
	}

	if !m.CapturesShellKey(tea.KeyMsg{Type: tea.KeyTab}) {
		t.Fatalf("expected search mode to capture tab for shell-level routing")
	}
}

func TestSearchModeQueryFocusAllowsPreviouslySwallowedLetters(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "jkhlr test issue", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)

	callsBefore := len(gw.Calls())
	pressAndResolve(m, testui.SearchTypeTextKeys(testui.SearchFragileQueryRunes())...)
	if countSearchCalls(gw.Calls()[callsBefore:]) != 0 {
		t.Fatalf("expected no search before enter, got %#v", gw.Calls()[callsBefore:])
	}
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.appliedQuery != "jkhlr" {
		t.Fatalf("expected appliedQuery=%q after enter, got %q", "jkhlr", m.appliedQuery)
	}
}

func TestSearchModeReloadPreservesQueryAndSelection(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	// Titles contain "x" so the query "x" matches both.
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Exact one", Status: "open", Type: "task", Priority: 1})
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Exact two", Status: "in_progress", Type: "bug", Priority: 2})
	m := initModel(gw)

	pressAndResolve(m, testui.SearchTypeTextKeys("x")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-2" {
		t.Fatalf("expected second result selected before reload, got %#v", got)
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected focus-query binding before reload, got %v", m.focus)
	}

	callsBefore := len(gw.Calls())
	cmd := m.Reload()
	m = applyMessages(m, drainCmd(cmd))

	newCalls := gw.Calls()[callsBefore:]
	if !hasSearchCall(newCalls) {
		t.Fatalf("expected at least one Search call after Reload, got %#v", newCalls)
	}
	if m.appliedQuery != "x" {
		t.Fatalf("expected reload to preserve query %q, got %q", "x", m.appliedQuery)
	}
	if m.draftQuery != "x" {
		t.Fatalf("expected reload to preserve draft query, got %q", m.draftQuery)
	}
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-2" {
		t.Fatalf("expected reload to preserve selected result, got %#v", got)
	}
}

func TestSearchModeAutoRefreshSkipsWhileActivelyTypingInQuery(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	m := initModel(gw)

	callsBefore := len(gw.Calls())
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Fatalf("expected typing not to trigger search until enter")
	}
	if !m.typing {
		t.Fatalf("expected typing flag while editing query")
	}

	auto := m.AutoRefresh()
	if auto != nil {
		t.Fatalf("expected auto refresh suppression while actively typing")
	}

	if countSearchCalls(gw.Calls()[callsBefore:]) != 0 {
		t.Fatalf("expected no repository calls while editing query, got %#v", gw.Calls()[callsBefore:])
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = applyMessages(m, drainCmd(cmd))
	newCalls := gw.Calls()[callsBefore:]
	if countSearchCalls(newCalls) != 1 {
		t.Fatalf("expected exactly one enter-triggered search call, got %d (%#v)", countSearchCalls(newCalls), newCalls)
	}
	if m.typing {
		t.Fatalf("expected typing false after search resolves")
	}
}

func TestSearchModeAutoRefreshPreservesQueryAndSelectionWhenPossible(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	// Titles contain "x" so the query "x" matches both.
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Exact one", Status: "open", Type: "task", Priority: 1})
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Exact two", Status: "in_progress", Type: "bug", Priority: 2})
	m := initModel(gw)

	pressAndResolve(m, testui.SearchTypeTextKeys("x")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-2" {
		t.Fatalf("expected second result selected before auto refresh, got %#v", got)
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected focus-query binding before auto refresh, got %v", m.focus)
	}

	callsBefore := len(gw.Calls())
	cmd := m.AutoRefresh()
	m = applyMessages(m, drainCmd(cmd))

	newCalls := gw.Calls()[callsBefore:]
	if !hasSearchCall(newCalls) {
		t.Fatalf("expected at least one Search call after AutoRefresh, got %#v", newCalls)
	}
	if m.appliedQuery != "x" {
		t.Fatalf("expected auto refresh to preserve applied query, got %q", m.appliedQuery)
	}
	if m.draftQuery != "x" {
		t.Fatalf("expected auto refresh to preserve query, got %q", m.draftQuery)
	}
	if got := m.SessionState().AppliedQuery; got != "x" {
		t.Fatalf("expected applied query to remain x after auto refresh, got %q", got)
	}
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-2" {
		t.Fatalf("expected auto refresh to preserve selected result, got %#v", got)
	}
}

func TestSearchModeSessionStatePreservesLastLoadedResultsDuringReloadAndError(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "First abc", Status: "open", Type: "task", Priority: 1,
		Description: "abc tag"})
	m := initModel(gw)

	pressAndResolve(m, testui.SearchTypeTextKeys("abc")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	session := m.SessionState()
	if session.DraftQuery != "abc" || session.AppliedQuery != "abc" {
		t.Fatalf("expected synced draft/applied query after search, got %#v", session)
	}
	if len(session.Page.Results) != 1 {
		t.Fatalf("expected 1 result after abc search, got %d", len(session.Page.Results))
	}

	cmd := m.Reload()
	if cmd == nil {
		t.Fatal("expected reload command")
	}
	session = m.SessionState()
	if !session.Loading || !session.Reloading {
		t.Fatalf("expected reload state while request in flight, got %#v", session)
	}
	if len(session.Page.Results) != 1 {
		t.Fatalf("expected prior results retained during reload, got %#v", session.Page.Results)
	}

	m = applyMessages(m, []tea.Msg{searchLoadedMsg{appliedQuery: "abc", err: errors.New("reload failed")}})
	session = m.SessionState()
	if session.Error != "reload failed" {
		t.Fatalf("expected reload error captured, got %#v", session)
	}
	if len(session.Page.Results) != 1 {
		t.Fatalf("expected last loaded results retained on reload error, got %#v", session.Page.Results)
	}
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-1" {
		t.Fatalf("expected selection retained on reload error, got %#v", got)
	}
	if !strings.Contains(m.View(0), "reload failed") || !strings.Contains(m.View(0), "First abc") || !strings.Contains(m.View(0), "failed") || !strings.Contains(m.View(0), "abc") {
		t.Fatalf("expected view to preserve rows and show refresh error, got:\n%s", m.View(0))
	}
}

func TestSearchModeSessionStateDistinguishesDraftAndAppliedQuery(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	m := initModel(gw)

	pressAndResolve(m, testui.SearchTypeTextKeys("foo")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	session := m.SessionState()
	if session.DraftQuery != "foox" {
		t.Fatalf("expected draft query to include unsent edit, got %#v", session)
	}
	if session.AppliedQuery != "foo" {
		t.Fatalf("expected applied query to remain last submitted value, got %#v", session)
	}
}

func TestSearchModeReusableScenarioHelpersCoverTypingFragileAndClear(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Default first", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)

	callsBefore := len(gw.Calls())
	pressAndResolve(m, testui.SearchTypeTextKeys(testui.SearchFragileQueryRunes())...)
	if countSearchCalls(gw.Calls()[callsBefore:]) != 0 {
		t.Fatalf("expected no search before enter, got %#v", gw.Calls()[callsBefore:])
	}
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.appliedQuery != testui.SearchFragileQueryRunes() {
		t.Fatalf("expected appliedQuery=%q after fragile rune enter, got %q", testui.SearchFragileQueryRunes(), m.appliedQuery)
	}

	pressAndResolve(m, testui.SearchClearQueryKeys()...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.appliedQuery != "" {
		t.Fatalf("expected appliedQuery empty after clear+enter, got %q", m.appliedQuery)
	}
}

func TestSearchModeUsesConfiguredBindingsAndPassesShellKeysThrough(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.MergeKeyBindings(config.DefaultKeyBindings(), &config.KeyBindingOverride{
		Search: map[string][]string{
			config.SearchActionMoveDown:       {"n"},
			config.SearchActionMoveUp:         {"p"},
			config.SearchActionFocusLeft:      {"a"},
			config.SearchActionFocusRight:     {"d"},
			config.SearchActionFocusQuery:     {"ctrl+f"},
			config.SearchActionReload:         {"ctrl+r"},
			config.SearchActionOpenDetail:     {"space"},
			config.SearchActionCycleFocusNext: {"ctrl+n"},
			config.SearchActionCycleFocusPrev: {"ctrl+p"},
		},
		Shell: map[string][]string{
			config.ShellActionQuit: {"ctrl+q"},
		},
	}))
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	gw := newSearchRepo()
	// Titles contain "g" so the query "g" matches both.
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Gig one", Status: "open", Type: "task", Priority: 1})
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Gig two", Status: "in_progress", Type: "bug", Priority: 2})
	m := testui.InitializeController(NewModel(context.Background(), gw, nil, keys)).(*Model)

	pressAndResolve(m, testui.SearchTypeTextKeys("g")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected configured next-focus binding to reach results, got %v", m.focus)
	}

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = applyMessages(m, drainCmd(cmd))
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-2" {
		t.Fatalf("expected configured move-down binding to select tm-2, got %#v", got)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected configured focus-left binding to stay on results, got %v", m.focus)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected configured focus-query binding to keep query focus, got %v", m.focus)
	}

	if m.CapturesShellKey(tea.KeyMsg{Type: tea.KeyCtrlQ}) {
		t.Fatal("expected configured shell quit key to pass through search capture")
	}
	if !m.CapturesShellKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")}) {
		t.Fatal("expected plain text rune to be captured while query focused")
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.focus != uisearch.FocusContent {
		t.Fatalf("expected configured focus-right binding to reach content, got %v", m.focus)
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected configured focus-left binding to return to results, got %v", m.focus)
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if cmd == nil {
		t.Fatal("expected configured open-detail binding to emit action request")
	}
	if action, ok := cmd().(mode.ActionRequestMsg); !ok || action.Action != mode.ActionOpenDetail {
		t.Fatalf("expected open detail action request, got %#v", cmd())
	}
}

func newSearchFakeRepository() *searchRepo {
	return newSearchRepo()
}

func initModel(repository repository.Repository) *Model {
	return testui.InitializeController(NewModel(context.Background(), repository, nil)).(*Model)
}

func pressAndResolve(m *Model, keys ...tea.KeyMsg) {
	resolved := testui.ApplyControllerKeySequence(m, keys...).(*Model)
	*m = *resolved
}

func applyMessages(m *Model, msgs []tea.Msg) *Model {
	for _, msg := range msgs {
		cmd := m.Update(msg)
		for _, follow := range drainCmd(cmd) {
			_ = m.Update(follow)
		}
	}
	return m
}

func drainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	queue := []tea.Msg{cmd()}
	var out []tea.Msg
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, nested := range batch {
				if nested != nil {
					queue = append(queue, nested())
				}
			}
			continue
		}
		out = append(out, msg)
	}

	return out
}

func TestSearchItemCapacity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		height int
		want   int
	}{
		{height: 0, want: 20},  // before first WindowSizeMsg: safe default
		{height: 1, want: 1},   // min clamp
		{height: 24, want: 17}, // 24 - 7 = 17
		{height: 30, want: 23}, // 30 - 7 = 23
	}

	for _, tc := range cases {
		m := &Model{height: tc.height}
		got := m.searchItemCapacity()
		if got != tc.want {
			t.Errorf("searchItemCapacity() with height=%d: got %d, want %d", tc.height, got, tc.want)
		}
	}
}

func TestSearchModeWindowSizeDoesNotTriggerRequery(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "First", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)

	// Record call count after init.
	callsBefore := len(gw.Calls())

	// Send a resize; must not issue a new search.
	cmd := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	if cmd != nil {
		t.Fatalf("expected WindowSizeMsg handler to return nil cmd, got %T", cmd)
	}

	if len(gw.Calls()) != callsBefore {
		t.Fatalf("expected no new repository calls on resize, got %d new call(s)", len(gw.Calls())-callsBefore)
	}
}

// TestSearchModeLoadingStaysSetDuringReload verifies that m.loading remains
// true while a reload request is in flight. The app-level loadingStates()
// function depends on this to drive the header spinner for the search surface.
func TestSearchModeLoadingStaysSetDuringReload(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "First", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)

	// After init+resolve, loading should be false.
	if m.loading {
		t.Fatalf("expected loading=false after init resolves, got true")
	}

	// Trigger a reload — loading must become true before the response arrives.
	cmd := m.Reload()
	if cmd == nil {
		t.Fatal("expected Reload to return a command")
	}
	if !m.loading {
		t.Fatalf("expected m.loading=true while reload is in flight, got false")
	}
	if !m.reloading {
		t.Fatalf("expected m.reloading=true while reload is in flight (has prior page), got false")
	}
}

// TestSearchModeTypingWhileLoadingIsAccepted is a regression test verifying
// that handleKey accepts query edits even when m.loading is true. The model
// must not gate text input on the loading flag.
func TestSearchModeTypingWhileLoadingIsAccepted(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	m := NewModel(context.Background(), gw, nil)

	// Manually set loading=true (simulating an in-flight request).
	m.loading = true
	m.focus = uisearch.FocusQuery

	// Type a rune — should update draftQuery without blocking.
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Fatalf("expected typing while loading to return nil cmd (no new search), got %T", cmd)
	}
	if m.draftQuery != "x" {
		t.Fatalf("expected draftQuery to accept typed rune while loading, got %q", m.draftQuery)
	}

	// Backspace should also work.
	cmd = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if cmd != nil {
		t.Fatalf("expected backspace while loading to return nil cmd, got %T", cmd)
	}
	if m.draftQuery != "" {
		t.Fatalf("expected backspace to remove typed rune while loading, got %q", m.draftQuery)
	}
}

// TestSearchModeMetadataPaneFocusAndSelection exercises ensureMetadataSelection
// and moveMetadataSelection by driving focus to the metadata pane and sending
// up/down arrow keys. It also verifies that ensureMetadataSelection resets a
// stale (invalid) metadataSelectedField value before navigating.
func TestSearchModeMetadataPaneFocusAndSelection(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "First", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)

	// Navigate: Query -> Results -> Content -> Metadata.
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected down from query to move focus to results, got %v", m.focus)
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.focus != uisearch.FocusContent {
		t.Fatalf("expected right from results to move focus to content, got %v", m.focus)
	}
	// This right-key triggers moveFocusRight -> ensureMetadataSelection.
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.focus != uisearch.FocusMetadata {
		t.Fatalf("expected right from content to move focus to metadata, got %v", m.focus)
	}
	// ensureMetadataSelection should have left a valid field selected.
	if m.metadataSelectedField != uidetails.MetadataFieldStatus && m.metadataSelectedField != uidetails.MetadataFieldPriority {
		t.Fatalf("expected valid metadata field after entering metadata pane, got %q", m.metadataSelectedField)
	}

	// Initially on status; move down to priority (covers moveMetadataSelection(+1)).
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.metadataSelectedField != uidetails.MetadataFieldPriority {
		t.Fatalf("expected down to move metadata selection to priority, got %q", m.metadataSelectedField)
	}

	// Move up back to status (covers moveMetadataSelection(-1)).
	_ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.metadataSelectedField != uidetails.MetadataFieldStatus {
		t.Fatalf("expected up to move metadata selection to status, got %q", m.metadataSelectedField)
	}

	// Verify clamping: down past the end stays at the last field.
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // status -> priority
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // already at last; must stay at priority
	if m.metadataSelectedField != uidetails.MetadataFieldPriority {
		t.Fatalf("expected selection clamped at priority on over-scroll, got %q", m.metadataSelectedField)
	}

	// Verify clamping at top: move up past first field stays at status.
	_ = m.Update(tea.KeyMsg{Type: tea.KeyUp}) // priority -> status
	_ = m.Update(tea.KeyMsg{Type: tea.KeyUp}) // already at first; must stay at status
	if m.metadataSelectedField != uidetails.MetadataFieldStatus {
		t.Fatalf("expected selection clamped at status on over-scroll, got %q", m.metadataSelectedField)
	}

	// Verify ensureMetadataSelection resets a stale/invalid field value.
	// Inject an invalid MetadataFieldKey directly, then trigger ensureMetadataSelection
	// via cycleFocus reaching the metadata pane.
	m.metadataSelectedField = uidetails.MetadataFieldKey("stale-invalid")
	m.ensureMetadataSelection()
	if m.metadataSelectedField != uidetails.MetadataFieldStatus {
		t.Fatalf("expected ensureMetadataSelection to reset stale field to status, got %q", m.metadataSelectedField)
	}
}

// TestSearchModeStaleDraftIndicatorAppearsAndClears verifies the end-to-end
// stale-results state machine:
//
//   - After typing a new draft (before Enter), the view shows the stale banner
//     and the Results badge contains "stale".
//   - After pressing Enter (search applied), the stale indicator is gone.
func TestSearchModeStaleDraftIndicatorAppearsAndClears(t *testing.T) {
	t.Parallel()

	gw := newSearchRepo()
	// Seed an issue matching "backend" so the first search returns a result.
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Prior backend result", Status: "open", Type: "task", Priority: 1})
	m := initModel(gw)
	m.SetSize(120, 28)

	// Apply an initial search so we have prior results.
	pressAndResolve(m, testui.SearchTypeTextKeys("backend")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Now type a new draft without pressing Enter.
	pressAndResolve(m, testui.SearchTypeTextKeys("zqx")...)

	// The draft differs from applied: stale indicator must appear.
	if m.draftQuery != "backendzqx" {
		t.Fatalf("expected draftQuery=backendzqx, got %q", m.draftQuery)
	}
	if m.appliedQuery != "backend" {
		t.Fatalf("expected appliedQuery=backend, got %q", m.appliedQuery)
	}
	viewStale := m.View(0)
	plain := testui.AnsiEscapePattern.ReplaceAllString(viewStale, "")
	if !strings.Contains(plain, "stale") {
		t.Fatalf("expected 'stale' badge in view while draft != applied, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Results below are stale") {
		t.Fatalf("expected stale banner in view while draft != applied, got:\n%s", plain)
	}

	// Press Enter to apply the draft search (backendzqx — no issues match).
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	// After search applied, stale indicator must be gone.
	viewApplied := m.View(0)
	plainApplied := testui.AnsiEscapePattern.ReplaceAllString(viewApplied, "")
	if strings.Contains(plainApplied, "stale") {
		t.Fatalf("expected no 'stale' badge after search is applied, got:\n%s", plainApplied)
	}
	if strings.Contains(plainApplied, "Results below are stale") {
		t.Fatalf("expected no stale banner after search is applied, got:\n%s", plainApplied)
	}
}

// TestSearchModeLogCarriesComponentSearch asserts that debug records emitted by
// the search model carry component=search (not component=dashboard or any
// other inherited value).
// Regression test for component-logging in search mode.
func TestSearchModeLogCarriesComponentSearch(t *testing.T) {
	t.Parallel()

	// Use a root logger (no component attached) — matching what main.go now
	// passes via services.Logger after the fix.
	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	rootLogger := slog.New(jsonHandler)
	// Derive the search logger exactly as NewModelWithOptions does via modeLogger.
	searchLogger := rootLogger.With("component", "search")

	repo := memoryrepo.New()
	gw := fakes.NewErrorInjecting(repo)
	m := NewModel(context.Background(), gw, searchLogger)

	// Put the model into a loading state and call Reload(). The guard path in
	// Reload (and triggerSearchWithAnchor) emits a Debug log when loading is
	// already in flight — giving us a real slog record to inspect.
	m.loading = true
	_ = m.Reload()

	output := buf.String()
	if output == "" {
		t.Fatal("expected at least one slog debug record, got empty output")
	}

	// Every emitted record must carry exactly one "component" key with value "search".
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		count := strings.Count(line, `"component":`)
		if count != 1 {
			t.Errorf("expected exactly 1 \"component\" key, got %d\nline: %s", count, line)
		}
		if !strings.Contains(line, `"component":"search"`) {
			t.Errorf("expected component=search in log line, got:\n%s", line)
		}
	}
}
