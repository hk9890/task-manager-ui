package search

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/e2e/embeddedfixture"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
	uisearch "github.com/hk9890/beads-workbench/internal/ui/search"
)

func TestSearchModeTextEntryRendersResultsInProgramHarness(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "Gateway search", Status: "open", Type: "task", Priority: 1}}}, Metadata: domain.SearchResultMetadata{ReturnedCount: 1, Completeness: domain.SearchResultCompletenessExact}}

	tm := testui.NewTestModelWithSize(t, testui.ControllerAdapter{Controller: NewModel(gateway)}, 120, 30)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		view := string(bts)
		return strings.Contains(view, "g│") && strings.Contains(view, "Gateway search") && strings.Contains(view, "Content")
	})

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}
}

func TestSearchModeInitLoadsDefaultResultsForEmptyQuery(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "Default one", Status: "open", Type: "task", Priority: 1}}}, Metadata: domain.SearchResultMetadata{ReturnedCount: 1, Completeness: domain.SearchResultCompletenessExact}}
	m := initModel(gateway)

	if !gateway.HasCall(string(fakes.MethodSearchIssues)) {
		t.Fatalf("expected empty init query to load default search results, calls=%#v", gateway.Calls)
	}

	if !strings.Contains(m.View(), "Default one") {
		t.Fatalf("expected default results view after init, got:\n%s", m.View())
	}
}

func TestSearchModeTextQuerySendsGatewaySearch(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	m := initModel(gateway)

	gateway.ResetCalls()
	pressAndResolve(m, testui.SearchTypeTextKeys("gw")...)
	if len(gateway.Calls) != 0 {
		t.Fatalf("expected no search call before explicit enter, got %#v", gateway.Calls)
	}
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	testui.AssertLatestSearchQueryText(t, gateway.Calls, "gw")
}

func TestSearchModeFocusNavigationAndSelection(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}},
		{Issue: domain.IssueSummary{ID: "bw-2", Title: "Second", Status: "in_progress", Type: "bug", Priority: 2}},
	}}
	m := initModel(gateway)

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
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected down to move selection to bw-2, got %#v", got)
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

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}},
		{Issue: domain.IssueSummary{ID: "bw-2", Title: "Second", Status: "in_progress", Type: "bug", Priority: 2}},
	}}
	m := initModel(gateway)

	pressAndResolve(m, testui.SearchTypeTextKeys("g")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-1" {
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
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-1" {
		t.Fatalf("expected first result selection to stay on bw-1, got %#v", got)
	}
	if len(gateway.Calls) != 2 {
		t.Fatalf("expected only init + explicit enter search calls, got %#v", gateway.Calls)
	}
}

func TestSearchModeClearingQueryRestoresDefaultResults(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "Default first", Status: "open", Type: "task", Priority: 1}},
		{Issue: domain.IssueSummary{ID: "bw-2", Title: "Default second", Status: "in_progress", Type: "bug", Priority: 2}},
	}}
	m := initModel(gateway)

	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-1" {
		t.Fatalf("expected initial selection on default results, got %#v", got)
	}

	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-9", Title: "Filtered only", Status: "open", Type: "task", Priority: 1}}}}
	pressAndResolve(m, testui.SearchTypeTextKeys("x")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-9" {
		t.Fatalf("expected filtered selection bw-9, got %#v", got)
	}

	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "Default first", Status: "open", Type: "task", Priority: 1}},
		{Issue: domain.IssueSummary{ID: "bw-2", Title: "Default second", Status: "in_progress", Type: "bug", Priority: 2}},
	}}
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyBackspace})
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	testui.AssertLatestSearchQueryText(t, gateway.Calls, "")
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-1" {
		t.Fatalf("expected selection reset to first default result, got %#v", got)
	}
	if !strings.Contains(m.View(), "Default first") || !strings.Contains(m.View(), "Default second") {
		t.Fatalf("expected restored default results in view, got:\n%s", m.View())
	}
}

func TestSearchModeRepresentativeStates(t *testing.T) {
	t.Parallel()

	t.Run("error state", func(t *testing.T) {
		m := NewModel(newSearchFakeGateway())
		_ = m.Update(searchLoadedMsg{err: errors.New("boom")})

		view := m.View()
		if !strings.Contains(view, "Search failed.") || !strings.Contains(view, "boom") || !strings.Contains(view, "failed") {
			t.Fatalf("expected error state in view, got:\n%s", view)
		}
	})

	t.Run("no results state", func(t *testing.T) {
		m := NewModel(newSearchFakeGateway())
		m.draftQuery = "xyz"
		cmd := m.Update(searchLoadedMsg{appliedQuery: "xyz", page: domain.SearchResultPage{}})
		if cmd != nil {
			_ = cmd()
		}

		if !strings.Contains(m.View(), "No matches for \"xyz\".") {
			t.Fatalf("expected no-results state in view, got:\n%s", m.View())
		}
	})

	t.Run("open detail action from results", func(t *testing.T) {
		gateway := newSearchFakeGateway()
		gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-7", Title: "Result", Status: "open", Type: "task", Priority: 1}}}}
		m := initModel(gateway)
		pressAndResolve(m, testui.SearchTypeTextKeys("g")...)
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

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-7", Title: "Result", Status: "open", Type: "task", Priority: 1}}}}
	m := initModel(gateway)

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

	gateway := newSearchFakeGateway()
	m := initModel(gateway)

	gateway.ResetCalls()
	pressAndResolve(m, testui.SearchTypeTextKeys(testui.SearchFragileQueryRunes())...)
	if len(gateway.Calls) != 0 {
		t.Fatalf("expected no search before enter, got %#v", gateway.Calls)
	}
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	testui.AssertLatestSearchQueryText(t, gateway.Calls, "jkhlr")
}

func TestSearchModeReloadPreservesQueryAndSelection(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}},
		{Issue: domain.IssueSummary{ID: "bw-2", Title: "Second", Status: "in_progress", Type: "bug", Priority: 2}},
	}}
	m := initModel(gateway)

	pressAndResolve(m, testui.SearchTypeTextKeys("x")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected second result selected before reload, got %#v", got)
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected focus-query binding before reload, got %v", m.focus)
	}

	gateway.ResetCalls()
	cmd := m.Reload()
	m = applyMessages(m, drainCmd(cmd))

	testui.AssertLatestSearchQueryText(t, gateway.Calls, "x")
	if m.draftQuery != "x" {
		t.Fatalf("expected reload to preserve query, got %q", m.draftQuery)
	}
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected reload to preserve selected result, got %#v", got)
	}
}

func TestSearchModeAutoRefreshSkipsWhileActivelyTypingInQuery(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	m := initModel(gateway)

	gateway.ResetCalls()
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

	if len(gateway.Calls) != 0 {
		t.Fatalf("expected no gateway calls while editing query, got %#v", gateway.Calls)
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = applyMessages(m, drainCmd(cmd))
	if len(gateway.Calls) != 1 || gateway.Calls[0].Method != fakes.MethodSearchIssues {
		t.Fatalf("expected exactly one enter-triggered search call, got %#v", gateway.Calls)
	}
	if m.typing {
		t.Fatalf("expected typing false after search resolves")
	}
}

func TestSearchModeAutoRefreshPreservesQueryAndSelectionWhenPossible(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}},
		{Issue: domain.IssueSummary{ID: "bw-2", Title: "Second", Status: "in_progress", Type: "bug", Priority: 2}},
	}}
	m := initModel(gateway)

	pressAndResolve(m, testui.SearchTypeTextKeys("x")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyDown})
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected second result selected before auto refresh, got %#v", got)
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected focus-query binding before auto refresh, got %v", m.focus)
	}

	gateway.ResetCalls()
	cmd := m.AutoRefresh()
	m = applyMessages(m, drainCmd(cmd))

	testui.AssertLatestSearchQueryText(t, gateway.Calls, "x")
	if m.draftQuery != "x" {
		t.Fatalf("expected auto refresh to preserve query, got %q", m.draftQuery)
	}
	if got := m.SessionState().AppliedQuery; got != "x" {
		t.Fatalf("expected applied query to remain x after auto refresh, got %q", got)
	}
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected auto refresh to preserve selected result, got %#v", got)
	}
}

func TestSearchModeSessionStatePreservesLastLoadedResultsDuringReloadAndError(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{
		Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}}},
		Metadata: domain.SearchResultMetadata{ReturnedCount: 1, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessMaybeMore, Notice: "first page"},
	}
	m := initModel(gateway)

	pressAndResolve(m, testui.SearchTypeTextKeys("abc")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})

	session := m.SessionState()
	if session.DraftQuery != "abc" || session.AppliedQuery != "abc" {
		t.Fatalf("expected synced draft/applied query after search, got %#v", session)
	}
	if len(session.Page.Results) != 1 || session.Page.Metadata.Notice != "first page" {
		t.Fatalf("expected page metadata/results captured in session, got %#v", session.Page)
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
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-1" {
		t.Fatalf("expected selection retained on reload error, got %#v", got)
	}
	if !strings.Contains(m.View(), "reload failed") || !strings.Contains(m.View(), "First") || !strings.Contains(m.View(), "failed") || !strings.Contains(m.View(), "abc") {
		t.Fatalf("expected view to preserve rows and show refresh error, got:\n%s", m.View())
	}
}

func TestSearchModeSessionStateDistinguishesDraftAndAppliedQuery(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	m := initModel(gateway)

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

func TestSearchModeEmbeddedFixtureInitUsesEmptyQueryFallback(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir: repoPath,
		Env:     append(os.Environ(), "BD_NON_INTERACTIVE=1"),
	})
	gateway := beads.NewCLIGateway(runner)

	tm := testui.NewTestModelWithSize(t, testui.ControllerAdapter{Controller: NewModel(gateway)}, 120, 30)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Real bd subprocess can take ~8s in isolation and much longer under
	// parallel `go test ./...` load — default 1s budget would flake. Bump to 15s.
	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), 15*time.Second, "Search", "bwf-1")

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}

	final, ok := tm.FinalModel(t).(testui.ControllerAdapter)
	if !ok {
		t.Fatalf("expected final model adapter")
	}

	finalModel, ok := final.Controller.(*Model)
	if !ok {
		t.Fatalf("expected wrapped search model, got %T", final.Controller)
	}

	if finalModel.errText != "" {
		t.Fatalf("expected empty-query fallback search to load without errors, got %q", finalModel.errText)
	}
	if finalModel.ResultCount() == 0 {
		t.Fatalf("expected fallback search to load fixture issues, got 0")
	}
	if strings.Contains(finalModel.View(), "Search failed") {
		t.Fatalf("expected no runtime search failure in view, got:\n%s", finalModel.View())
	}
}

func TestSearchModeReusableScenarioHelpersCoverTypingFragileAndClear(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "Default first", Status: "open", Type: "task", Priority: 1}}}}
	m := initModel(gateway)

	gateway.ResetCalls()
	pressAndResolve(m, testui.SearchTypeTextKeys(testui.SearchFragileQueryRunes())...)
	if len(gateway.Calls) != 0 {
		t.Fatalf("expected no search before enter, got %#v", gateway.Calls)
	}
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	testui.AssertLatestSearchQueryText(t, gateway.Calls, testui.SearchFragileQueryRunes())

	pressAndResolve(m, testui.SearchClearQueryKeys()...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	testui.AssertLatestSearchQueryText(t, gateway.Calls, "")
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

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}},
		{Issue: domain.IssueSummary{ID: "bw-2", Title: "Second", Status: "in_progress", Type: "bug", Priority: 2}},
	}}
	m := testui.InitializeController(NewModel(gateway, keys)).(*Model)

	pressAndResolve(m, testui.SearchTypeTextKeys("g")...)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected configured next-focus binding to reach results, got %v", m.focus)
	}

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = applyMessages(m, drainCmd(cmd))
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected configured move-down binding to select bw-2, got %#v", got)
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

func newSearchFakeGateway() *fakes.FakeBeadsGateway {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	return gateway
}

func initModel(gateway *fakes.FakeBeadsGateway) *Model {
	return testui.InitializeController(NewModel(gateway)).(*Model)
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

func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
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

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}},
	}}
	m := initModel(gateway)

	// Record call count after init.
	callsBefore := len(gateway.Calls)

	// Send a resize; must not issue a new search.
	cmd := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	if cmd != nil {
		t.Fatalf("expected WindowSizeMsg handler to return nil cmd, got %T", cmd)
	}

	if len(gateway.Calls) != callsBefore {
		t.Fatalf("expected no new gateway calls on resize, got %d new call(s)", len(gateway.Calls)-callsBefore)
	}
}
