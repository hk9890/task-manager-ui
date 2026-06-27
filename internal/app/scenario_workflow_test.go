package app

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func TestModelReusableBoardSearchDetailScenarioCoversTypingClearScrollAndBack(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// Seed the search result so typing "jkhlr" still matches via text search.
	// Memory repo Search() matches on Title, Description, Notes.
	// We include the fragile query runes in Description so memory repo's
	// text search returns tm-1 for that query.
	gw.seedSearchResult(memoryrepo.Issue{
		ID:          "tm-1",
		Title:       "Ready first",
		Status:      "open",
		Type:        "task",
		Priority:    1,
		Description: testui.SearchFragileQueryRunes(),
	})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: longScenarioDetail(90),
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := testui.InitializeModel(mustNewModel(t, services)).(Model)
	m.width, m.height = 120, 24

	m = testui.ApplyKeySequence(m, testui.BoardToSearchKeys()...).(Model)
	if m.active != mode.Search {
		t.Fatalf("expected board->search scenario to land in search mode, got %s", m.active)
	}

	m = testui.ApplyKeySequence(m, testui.SearchTypeTextKeys(testui.SearchFragileQueryRunes())...).(Model)
	m = testui.ApplyKeySequence(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	// Verify the applied query directly from search state instead of repository call inspection.
	if got := m.search.SessionState().AppliedQuery; got != testui.SearchFragileQueryRunes() {
		t.Fatalf("expected applied query %q after typing, got %q", testui.SearchFragileQueryRunes(), got)
	}

	m = testui.ApplyKeySequence(m, testui.SearchClearQueryKeys()...).(Model)
	m = testui.ApplyKeySequence(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	// After clearing, the applied query should be empty.
	if got := m.search.SessionState().AppliedQuery; got != "" {
		t.Fatalf("expected empty applied query after clear, got %q", got)
	}

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
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: "detail",
	})

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
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
	if len(fakeEditor.Calls) != 1 || fakeEditor.Calls[0].IssueID != "tm-1" {
		t.Fatalf("expected edit seam call for tm-1, got %#v", fakeEditor.Calls)
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
