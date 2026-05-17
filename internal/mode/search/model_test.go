package search

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
	uisearch "github.com/hk9890/beads-workbench/internal/ui/search"
)

func TestSearchModeTextEntryRendersResultsInProgramHarness(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "Gateway search", Status: "open", Type: "task", Priority: 1}}}, Metadata: domain.SearchResultMetadata{ReturnedCount: 1, Completeness: domain.SearchResultCompletenessExact}}

	tm := testui.NewTestModelWithSize(t, testui.ControllerAdapter{Controller: NewModel(gateway, nil)}, 120, 30)
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

	if !strings.Contains(m.View(0), "Default one") {
		t.Fatalf("expected default results view after init, got:\n%s", m.View(0))
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
	if !strings.Contains(m.View(0), "Default first") || !strings.Contains(m.View(0), "Default second") {
		t.Fatalf("expected restored default results in view, got:\n%s", m.View(0))
	}
}

func TestSearchModeRepresentativeStates(t *testing.T) {
	t.Parallel()

	t.Run("error state", func(t *testing.T) {
		m := NewModel(newSearchFakeGateway(), nil)
		_ = m.Update(searchLoadedMsg{err: errors.New("boom")})

		view := m.View(0)
		if !strings.Contains(view, "Search failed.") || !strings.Contains(view, "boom") || !strings.Contains(view, "failed") {
			t.Fatalf("expected error state in view, got:\n%s", view)
		}
	})

	t.Run("no results state", func(t *testing.T) {
		m := NewModel(newSearchFakeGateway(), nil)
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
		Results:  []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}}},
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
	if !strings.Contains(m.View(0), "reload failed") || !strings.Contains(m.View(0), "First") || !strings.Contains(m.View(0), "failed") || !strings.Contains(m.View(0), "abc") {
		t.Fatalf("expected view to preserve rows and show refresh error, got:\n%s", m.View(0))
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
	m := testui.InitializeController(NewModel(gateway, nil, keys)).(*Model)

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
	return testui.InitializeController(NewModel(gateway, nil)).(*Model)
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

// TestSearchModeLoadingStaysSetDuringReload verifies that m.loading remains
// true while a reload request is in flight. The app-level loadingStates()
// function depends on this to drive the header spinner for the search surface.
func TestSearchModeLoadingStaysSetDuringReload(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}},
	}}
	m := initModel(gateway)

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

	gateway := newSearchFakeGateway()
	m := NewModel(gateway, nil)

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
