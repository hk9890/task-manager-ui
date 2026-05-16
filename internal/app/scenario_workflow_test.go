package app

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
)

func TestModelReusableBoardSearchDetailScenarioCoversTypingClearScrollAndBack(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}}}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: longScenarioDetail(90),
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := testui.InitializeModel(mustNewModel(t, services)).(Model)
	m.width, m.height = 120, 24

	m = testui.ApplyKeySequence(m, testui.BoardToSearchKeys()...).(Model)
	if m.active != mode.Search {
		t.Fatalf("expected board->search scenario to land in search mode, got %s", m.active)
	}

	gateway.ResetCalls()
	m = testui.ApplyKeySequence(m, testui.SearchTypeTextKeys(testui.SearchFragileQueryRunes())...).(Model)
	m = testui.ApplyKeySequence(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	testui.AssertLatestSearchQueryText(t, gateway.Calls, testui.SearchFragileQueryRunes())

	m = testui.ApplyKeySequence(m, testui.SearchClearQueryKeys()...).(Model)
	m = testui.ApplyKeySequence(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	testui.AssertLatestSearchQueryText(t, gateway.Calls, "")

	m = testui.ApplyKeySequence(m, testui.SearchFocusResultsKeys()...).(Model)
	m = testui.ApplyKeySequence(m, testui.OpenDetailKeys()...).(Model)
	if m.active != mode.Detail {
		t.Fatalf("expected search->detail open scenario, got %s", m.active)
	}

	m = testui.ApplyKeySequence(m, testui.DetailScrollKeys()...).(Model)
	if m.detail.ScrollOffset == 0 {
		t.Fatalf("expected detail scroll scenario to move viewport offset")
	}

	m = testui.ApplyKeySequence(m, testui.DetailBackKeys()...).(Model)
	if m.active != mode.Search {
		t.Fatalf("expected detail back scenario to return to search, got %s", m.active)
	}
}

func TestModelReusableDetailToolScenarioCoversEditorAndLaunchersWithFakes(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}, Description: "detail"}

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	services.Editor = fakeEditor

	m := testui.InitializeModel(mustNewModel(t, services)).(Model)
	m = testui.ApplyKeySequence(m, testui.OpenDetailKeys()...).(Model)
	if m.active != mode.Detail {
		t.Fatalf("expected open detail scenario before tool actions, got %s", m.active)
	}

	m = testui.ApplyKeySequence(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}).(Model)
	if len(fakeEditor.Calls) != 1 || fakeEditor.Calls[0].IssueID != "bw-1" {
		t.Fatalf("expected edit seam call for bw-1, got %#v", fakeEditor.Calls)
	}

	m = testui.ApplyKeySequence(m,
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")},
	).(Model)

	if len(fakeLauncher.Calls) != 3 {
		t.Fatalf("expected 3 launcher seam calls, got %d", len(fakeLauncher.Calls))
	}
	actions := []string{fakeLauncher.Calls[0].Action, fakeLauncher.Calls[1].Action, fakeLauncher.Calls[2].Action}
	if strings.Join(actions, ",") != "nvim,opencode,shell-command" {
		t.Fatalf("expected launcher actions [nvim opencode shell-command], got %#v", actions)
	}
}

func longScenarioDetail(lines int) string {
	out := make([]string, 0, lines)
	for i := 1; i <= lines; i++ {
		out = append(out, "Line "+strconv.Itoa(i))
	}
	return strings.Join(out, "\n")
}
