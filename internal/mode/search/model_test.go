package search

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
	uisearch "github.com/hk9890/beads-workbench/internal/ui/search"
)

type controllerAdapter struct {
	controller mode.Controller
}

func (a controllerAdapter) Init() tea.Cmd {
	return a.controller.Init()
}

func (a controllerAdapter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := a.controller.Update(msg)
	return controllerAdapter{controller: next}, cmd
}

func (a controllerAdapter) View() string {
	return a.controller.View()
}

func TestSearchModeTextEntryRendersResultsInProgramHarness(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "Gateway search", Status: "open", Type: "task", Priority: 1}}}, Total: 1}

	tm := testui.NewTestModelWithSize(t, controllerAdapter{controller: NewModel(gateway)}, 120, 30)
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

func TestSearchModeInitDoesNotRunGatewaySearchForEmptyQuery(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	m := NewModel(gateway)

	for _, msg := range runBatch(m.Init()) {
		next, cmd := m.Update(msg)
		m = next.(*Model)
		for _, follow := range runBatch(cmd) {
			next, _ = m.Update(follow)
			m = next.(*Model)
		}
	}

	if hasGatewayCall(gateway.Calls, fakes.MethodSearchIssues) {
		t.Fatalf("expected empty init query to avoid gateway search call, calls=%#v", gateway.Calls)
	}

	if !strings.Contains(m.View(), "Start typing to search issues.") {
		t.Fatalf("expected empty state view after init, got:\n%s", m.View())
	}
}

func TestSearchModeTextQuerySendsGatewaySearch(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	m := initModel(t, gateway)

	gateway.ResetCalls()
	pressAndResolve(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	pressAndResolve(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})

	query := latestSearchQuery(t, gateway.Calls)
	if query.Text != "gw" {
		t.Fatalf("expected text query gw, got %q", query.Text)
	}
}

func TestSearchModeFocusNavigationAndSelection(t *testing.T) {
	t.Parallel()

	gateway := newSearchFakeGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-1", Title: "First", Status: "open", Type: "task", Priority: 1}},
		{Issue: domain.IssueSummary{ID: "bw-2", Title: "Second", Status: "in_progress", Type: "bug", Priority: 2}},
	}}
	m := initModel(t, gateway)

	pressAndResolve(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if m.focus != uisearch.FocusQuery {
		t.Fatalf("expected initial search focus, got %v", m.focus)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(*Model)
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected right to move focus to results, got %v", m.focus)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(*Model)
	m = applyMessages(m, runBatch(cmd))
	if got := m.currentSelection(); got == nil || got.Issue.ID != "bw-2" {
		t.Fatalf("expected j to move selection to bw-2, got %#v", got)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(*Model)
	if m.focus != uisearch.FocusPreview {
		t.Fatalf("expected right to move focus to preview, got %v", m.focus)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(*Model)
	if m.focus != uisearch.FocusResults {
		t.Fatalf("expected left to move focus back to results, got %v", m.focus)
	}
}

func TestSearchModeRepresentativeStates(t *testing.T) {
	t.Parallel()

	t.Run("error state", func(t *testing.T) {
		m := NewModel(newSearchFakeGateway())
		next, _ := m.Update(searchLoadedMsg{err: errors.New("boom")})
		m = next.(*Model)

		view := m.View()
		if !strings.Contains(view, "Search failed") || !strings.Contains(view, "boom") {
			t.Fatalf("expected error state in view, got:\n%s", view)
		}
	})

	t.Run("no results state", func(t *testing.T) {
		m := NewModel(newSearchFakeGateway())
		m.query = "xyz"
		next, cmd := m.Update(searchLoadedMsg{issues: nil})
		m = next.(*Model)
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
		m := initModel(t, gateway)
		pressAndResolve(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})

		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = next.(*Model)
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatalf("expected action request command on enter")
		}
		msg := cmd()
		action, ok := msg.(mode.ActionRequestMsg)
		if !ok {
			t.Fatalf("expected ActionRequestMsg, got %T", msg)
		}
		if action.Action != mode.ActionOpenDetail || action.Mode != mode.Search {
			t.Fatalf("unexpected action request: %#v", action)
		}
	})
}

func newSearchFakeGateway() *fakes.FakeBeadsGateway {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	return gateway
}

func initModel(t *testing.T, gateway *fakes.FakeBeadsGateway) *Model {
	t.Helper()

	m := NewModel(gateway)
	initCmd := m.Init()
	for _, msg := range runBatch(initCmd) {
		next, cmd := m.Update(msg)
		m = next.(*Model)
		for _, follow := range runBatch(cmd) {
			next, _ = m.Update(follow)
			m = next.(*Model)
		}
	}

	return m
}

func runBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	queue := []tea.Msg{cmd()}
	var out []tea.Msg
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, b := range batch {
				if b != nil {
					queue = append(queue, b())
				}
			}
			continue
		}
		out = append(out, msg)
	}

	return out
}

func pressAndResolve(t *testing.T, m *Model, key tea.KeyMsg) {
	t.Helper()

	next, cmd := m.Update(key)
	mCopy := next.(*Model)
	*m = *mCopy

	for _, msg := range runBatch(cmd) {
		next, nested := m.Update(msg)
		mCopy = next.(*Model)
		*m = *mCopy
		for _, follow := range runBatch(nested) {
			next, _ = m.Update(follow)
			mCopy = next.(*Model)
			*m = *mCopy
		}
	}
}

func latestSearchQuery(t *testing.T, calls []fakes.GatewayCall) domain.SearchIssuesQuery {
	t.Helper()

	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		if call.Method != fakes.MethodSearchIssues {
			continue
		}
		input, ok := call.Input.(fakes.SearchIssuesCall)
		if !ok {
			t.Fatalf("expected SearchIssuesCall payload, got %T", call.Input)
		}
		return input.Query
	}

	t.Fatalf("no search calls recorded: %#v", calls)
	return domain.SearchIssuesQuery{}
}

func hasGatewayCall(calls []fakes.GatewayCall, method fakes.GatewayMethod) bool {
	for _, call := range calls {
		if call.Method == method {
			return true
		}
	}

	return false
}

func applyMessages(m *Model, msgs []tea.Msg) *Model {
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = next.(*Model)
		for _, follow := range runBatch(cmd) {
			next, _ = m.Update(follow)
			m = next.(*Model)
		}
	}
	return m
}
