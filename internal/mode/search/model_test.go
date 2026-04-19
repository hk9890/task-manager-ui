package search

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

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
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "Gateway search", Status: "open", Type: "task", Priority: 1}}}, Total: 1}

	tm := testui.NewTestModelWithSize(t, testui.ControllerAdapter{Controller: NewModel(gateway)}, 120, 30)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})

	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		view := string(bts)
		return strings.Contains(view, "g│") && strings.Contains(view, "Gateway search") && strings.Contains(view, "Preview")
	})

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}
}

func TestSearchModeInitLoadsDefaultResultsForEmptyQuery(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "Default one", Status: "open", Type: "task", Priority: 1}}}, Total: 1}
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
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected initial search focus, got %v", m.focus)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected right to move focus to results, got %v", m.focus)
	}

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = applyMessages(m, drainCmd(cmd))
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected j to move selection to bw-2, got %#v", got)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.focus != uisearch.FocusPreview {
		t.Fatalf("expected right to move focus to preview, got %v", m.focus)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected left to move focus back to results, got %v", m.focus)
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
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-9" {
		t.Fatalf("expected filtered selection bw-9, got %#v", got)
	}

	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "Default first", Status: "open", Type: "task", Priority: 1}},
		{Issue: domain.IssueSummary{ID: "bw-2", Title: "Default second", Status: "in_progress", Type: "bug", Priority: 2}},
	}}
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyBackspace})

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
		if !strings.Contains(view, "Search failed") || !strings.Contains(view, "boom") {
			t.Fatalf("expected error state in view, got:\n%s", view)
		}
	})

	t.Run("no results state", func(t *testing.T) {
		m := NewModel(newSearchFakeGateway())
		m.query = "xyz"
		cmd := m.Update(searchLoadedMsg{issues: nil})
		if cmd != nil {
			_ = cmd()
		}

		if !strings.Contains(m.View(), "No results found.") {
			t.Fatalf("expected no-results state in view, got:\n%s", m.View())
		}
	})

	t.Run("open detail action from results", func(t *testing.T) {
		gateway := newSearchFakeGateway()
		gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-7", Title: "Result", Status: "open", Type: "task", Priority: 1}}}}
		m := initModel(gateway)
		pressAndResolve(m, testui.SearchTypeTextKeys("g")...)

		_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
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
	if m.focus != uisearch.FocusPreview {
		t.Fatalf("expected tab to cycle results->preview, got %v", m.focus)
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
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected second result selected before reload, got %#v", got)
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected query focus before reload, got %v", m.focus)
	}

	gateway.ResetCalls()
	cmd := m.Reload()
	m = applyMessages(m, drainCmd(cmd))

	testui.AssertLatestSearchQueryText(t, gateway.Calls, "x")
	if m.query != "x" {
		t.Fatalf("expected reload to preserve query, got %q", m.query)
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
	if cmd == nil {
		t.Fatalf("expected typing to trigger search")
	}
	if !m.typing {
		t.Fatalf("expected typing flag while waiting for query search response")
	}

	auto := m.AutoRefresh()
	if auto != nil {
		t.Fatalf("expected auto refresh suppression while actively typing")
	}

	if len(gateway.Calls) != 0 {
		t.Fatalf("expected no gateway calls before queued typing command resolves, got %#v", gateway.Calls)
	}

	m = applyMessages(m, drainCmd(cmd))
	if len(gateway.Calls) != 1 || gateway.Calls[0].Method != fakes.MethodSearchIssues {
		t.Fatalf("expected exactly one typing-triggered search call, got %#v", gateway.Calls)
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
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected second result selected before auto refresh, got %#v", got)
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected focus query before auto refresh, got %v", m.focus)
	}

	gateway.ResetCalls()
	cmd := m.AutoRefresh()
	m = applyMessages(m, drainCmd(cmd))

	testui.AssertLatestSearchQueryText(t, gateway.Calls, "x")
	if m.query != "x" {
		t.Fatalf("expected auto refresh to preserve query, got %q", m.query)
	}
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected auto refresh to preserve selected result, got %#v", got)
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

	testui.WaitForOutputContainsAll(t, tm.Output(), "Search", "bwf-1")

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
	testui.AssertLatestSearchQueryText(t, gateway.Calls, testui.SearchFragileQueryRunes())

	pressAndResolve(m, testui.SearchClearQueryKeys()...)
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
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected configured focus-left binding to return to query, got %v", m.focus)
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
	if m.focus != uisearch.FocusPreview {
		t.Fatalf("expected configured focus-right binding to reach preview, got %v", m.focus)
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
