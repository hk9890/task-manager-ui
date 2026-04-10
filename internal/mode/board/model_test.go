package board

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
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

type staticProvider struct {
	defs []dashboard.Definition
	err  error
}

func resolvedBoardKeys(t *testing.T) config.ResolvedKeyBindings {
	t.Helper()
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	return keys
}

func (p staticProvider) Dashboards(_ context.Context) ([]dashboard.Definition, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.defs, nil
}

func TestBoardModeLoadsBuiltInQueriesAndRendersGolden(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{
		{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"},
		{ID: "bw-2", Title: "Ready second", Priority: 2, Status: "open", Type: "task"},
	}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-3", Title: "In progress", Priority: 2, Status: "in_progress", Type: "feature"}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "bw-4", Title: "Blocked now", Priority: 1, Status: "blocked", Type: "bug"}}}

	provider := staticProvider{defs: []dashboard.Definition{{
		ID:    "default",
		Title: "Default",
		Sections: []dashboard.Section{
			{ID: "not_ready", Title: "Not Ready", Query: dashboard.Query{Type: dashboard.QueryTypeBlockedIssues, BlockedIssues: domain.BlockedIssuesQuery{Limit: 25}}},
			{ID: "ready", Title: "Ready", Query: dashboard.Query{Type: dashboard.QueryTypeReadyIssues, ReadyIssues: domain.ReadyIssuesQuery{Limit: 25}}},
			{ID: "in_progress", Title: "In Progress", Query: dashboard.Query{Type: dashboard.QueryTypeListIssues, ListIssues: domain.IssueListQuery{Statuses: []string{"in_progress"}, Limit: 25}}},
			{ID: "done", Title: "Done", Query: dashboard.Query{Type: dashboard.QueryTypeListIssues, ListIssues: domain.IssueListQuery{Statuses: []string{"closed"}, SortBy: domain.SortFieldUpdatedAt, SortOrder: domain.SortDirectionDescending, Limit: 25}}},
		},
	}}}

	tm := testui.NewTestModel(t, controllerAdapter{controller: NewModel(gateway, provider, resolvedBoardKeys(t))})
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})

	testui.WaitForOutputContainsAll(t, tm.Output(), "Default", "Not Ready", "Ready", "In Progress", "bw-1", "bw-3")

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}

	final, ok := tm.FinalModel(t).(controllerAdapter)
	if !ok {
		t.Fatalf("expected final model adapter")
	}

	finalModel, ok := final.controller.(*Model)
	if !ok {
		t.Fatalf("expected wrapped board model, got %T", final.controller)
	}

	if sel := finalModel.CurrentSelection(); sel == nil || sel.Issue.ID != "bw-4" {
		t.Fatalf("expected initial selection bw-4 from Not Ready lane, got %#v", sel)
	}

	testui.AssertMatchesGoldenNormalized(t, []byte(finalModel.View()), "model_loaded.golden")
}

func TestBoardModeNavigationEmitsSelectionChangedAndActionRequest(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}
	gateway.ListIssuesResponse = []domain.IssueSummary{
		{ID: "bw-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"},
		{ID: "bw-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"},
	}

	provider := staticProvider{defs: []dashboard.Definition{{
		ID:    "default",
		Title: "Default",
		Sections: []dashboard.Section{
			{ID: "ready", Title: "Ready", Query: dashboard.Query{Type: dashboard.QueryTypeReadyIssues, ReadyIssues: domain.ReadyIssuesQuery{Limit: 25}}},
			{ID: "in_progress", Title: "In Progress", Query: dashboard.Query{Type: dashboard.QueryTypeListIssues, ListIssues: domain.IssueListQuery{Statuses: []string{"in_progress"}, Limit: 25}}},
		},
	}}}

	m := NewModel(gateway, provider, resolvedBoardKeys(t))
	m.dashboardID = "default"
	m.dashboardTitle = "Default"
	m.sections = []sectionState{
		{title: "Ready", issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}, loaded: true},
		{title: "In Progress", issues: []domain.IssueSummary{{ID: "bw-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}, {ID: "bw-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"}}, loaded: true},
	}
	m.focusedColumn = 0
	m.selectedRow[0] = 0
	m.selectedRow[1] = 0
	m.loading = false
	m.SetSize(100, 24)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(*Model)
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving right")
	}
	msg := cmd()
	selChanged, ok := msg.(mode.SelectionChangedMsg)
	if !ok {
		t.Fatalf("expected SelectionChangedMsg, got %T", msg)
	}
	if selChanged.Selection == nil || selChanged.Selection.Issue.ID != "bw-7" {
		t.Fatalf("expected selection bw-7 after moving right, got %#v", selChanged.Selection)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*Model)
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving down")
	}
	msg = cmd()
	selChanged, ok = msg.(mode.SelectionChangedMsg)
	if !ok {
		t.Fatalf("expected SelectionChangedMsg, got %T", msg)
	}
	if selChanged.Selection == nil || selChanged.Selection.Issue.ID != "bw-8" {
		t.Fatalf("expected selection bw-8 after moving down, got %#v", selChanged.Selection)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*Model)
	if cmd == nil {
		t.Fatalf("expected action request command on enter")
	}
	msg = cmd()
	action, ok := msg.(mode.ActionRequestMsg)
	if !ok {
		t.Fatalf("expected ActionRequestMsg, got %T", msg)
	}
	if action.Action != mode.ActionOpenDetail {
		t.Fatalf("expected action %q got %q", mode.ActionOpenDetail, action.Action)
	}

	testui.AssertMatchesGoldenNormalized(t, []byte(m.View()), "model_navigation.golden")
}

func TestBoardModeUsesConfiguredBindings(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.MergeKeyBindings(config.DefaultKeyBindings(), &config.KeyBindingOverride{
		Board: map[string][]string{
			config.BoardActionMoveLeft:   {"a"},
			config.BoardActionMoveRight:  {"d"},
			config.BoardActionMoveUp:     {"w"},
			config.BoardActionMoveDown:   {"s"},
			config.BoardActionOpenDetail: {"space"},
			config.BoardActionReload:     {"R"},
		},
	}))
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}, {ID: "bw-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"}}

	provider := staticProvider{defs: []dashboard.Definition{{
		ID:    "default",
		Title: "Default",
		Sections: []dashboard.Section{
			{ID: "ready", Title: "Ready", Query: dashboard.Query{Type: dashboard.QueryTypeReadyIssues, ReadyIssues: domain.ReadyIssuesQuery{Limit: 25}}},
			{ID: "in_progress", Title: "In Progress", Query: dashboard.Query{Type: dashboard.QueryTypeListIssues, ListIssues: domain.IssueListQuery{Statuses: []string{"in_progress"}, Limit: 25}}},
		},
	}}}

	m := NewModel(gateway, provider, keys)
	m.dashboardID = "default"
	m.dashboardTitle = "Default"
	m.sections = []sectionState{
		{title: "Ready", issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}, loaded: true},
		{title: "In Progress", issues: []domain.IssueSummary{{ID: "bw-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}, {ID: "bw-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"}}, loaded: true},
	}
	m.focusedColumn = 0
	m.selectedRow[0] = 0
	m.selectedRow[1] = 0
	m.loading = false
	m.SetSize(100, 24)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected selection change after configured move-right key")
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected selection change after configured move-down key")
	}
	msg := cmd()
	selChanged, ok := msg.(mode.SelectionChangedMsg)
	if !ok || selChanged.Selection == nil || selChanged.Selection.Issue.ID != "bw-8" {
		t.Fatalf("expected configured move-down to select bw-8, got %#v", msg)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = next.(*Model)
	if cmd == nil {
		t.Fatal("expected action request from configured open key")
	}
	if action, ok := cmd().(mode.ActionRequestMsg); !ok || action.Action != mode.ActionOpenDetail {
		t.Fatalf("expected open detail action request, got %#v", cmd())
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = next.(*Model)
	if cmd == nil || !m.loading {
		t.Fatal("expected configured reload key to trigger dashboard reload")
	}
}

func TestBoardModeStartupFocusStableDuringAsyncLoadsAndSettlesByDashboardOrder(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	m := NewModel(gateway, staticProvider{defs: []dashboard.Definition{{
		ID:    "default",
		Title: "Default",
		Sections: []dashboard.Section{
			{ID: "not_ready", Title: "Not Ready", Query: dashboard.Query{Type: dashboard.QueryTypeBlockedIssues}},
			{ID: "ready", Title: "Ready", Query: dashboard.Query{Type: dashboard.QueryTypeReadyIssues}},
			{ID: "in_progress", Title: "In Progress", Query: dashboard.Query{Type: dashboard.QueryTypeListIssues}},
		},
	}}}, resolvedBoardKeys(t))

	m.loading = false
	m.sections = []sectionState{{title: "Not Ready", loaded: false}, {title: "Ready", loaded: false}, {title: "In Progress", loaded: false}}
	m.pendingLoads = 3
	m.focusedColumn = 0
	m.selectedRow[0] = 0
	m.selectedRow[1] = 0
	m.selectedRow[2] = 0

	next, _ := m.Update(sectionLoadedMsg{sectionIndex: 2, issues: []domain.IssueSummary{{ID: "bw-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}}})
	m = next.(*Model)

	if m.focusedColumn != 0 {
		t.Fatalf("expected startup focus to remain on first dashboard column while loads are pending, got %d", m.focusedColumn)
	}
	if sel := m.CurrentSelection(); sel != nil {
		t.Fatalf("expected no selection while focused startup column has no rows, got %#v", sel)
	}
	if strings.Contains(m.View(), "› T P2 IP bw-7 Progress one") {
		t.Fatalf("expected render to avoid mid-load focus jumps to later column, got:\n%s", m.View())
	}

	next, _ = m.Update(sectionLoadedMsg{sectionIndex: 1, issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}})
	m = next.(*Model)
	if m.focusedColumn != 0 {
		t.Fatalf("expected startup focus to stay deterministic until all section loads complete, got %d", m.focusedColumn)
	}
	if strings.Contains(m.View(), "› T P1 OPN bw-1 Ready first") {
		t.Fatalf("expected no row selection before all section loads complete, got:\n%s", m.View())
	}

	next, _ = m.Update(sectionLoadedMsg{sectionIndex: 0, issues: []domain.IssueSummary{{ID: "bw-0", Title: "Blocked first", Priority: 0, Status: "blocked", Type: "bug"}}})
	m = next.(*Model)

	if m.focusedColumn != 0 {
		t.Fatalf("expected final focus to settle on earliest non-empty dashboard column, got %d", m.focusedColumn)
	}
	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "bw-0" {
		t.Fatalf("expected current selection bw-0 after startup load completion, got %#v", sel)
	}
	if !strings.Contains(m.View(), "› B P0 BLK bw-0 Blocked first") {
		t.Fatalf("expected runtime render to show startup focus on settled first column row, got:\n%s", m.View())
	}
}

func TestBoardModeDashboardLayoutGoldensAcrossWidths(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{
		{ID: "bw-1", Title: "Ready fix login prompt", Priority: 1, Status: "open", Type: "task"},
		{ID: "bw-5", Title: "Ready improve docs outline", Priority: 2, Status: "open", Type: "task"},
		{ID: "bw-6", Title: "Ready triage inbox", Priority: 2, Status: "open", Type: "chore"},
	}
	gateway.ListIssuesResponse = []domain.IssueSummary{
		{ID: "bw-2", Title: "Implement board keyboard shortcuts", Priority: 1, Status: "in_progress", Type: "feature"},
		{ID: "bw-7", Title: "Wire detail reload behavior", Priority: 1, Status: "in_progress", Type: "task"},
		{ID: "bw-8", Title: "Polish header help copy", Priority: 2, Status: "in_progress", Type: "docs"},
	}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{
		{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked: API contract pending", Priority: 0, Status: "blocked", Type: "bug"}},
		{Issue: domain.IssueSummary{ID: "bw-9", Title: "Blocked: migration sequencing", Priority: 1, Status: "blocked", Type: "task"}},
	}

	provider := staticProvider{defs: []dashboard.Definition{{
		ID:    "default",
		Title: "Default",
		Sections: []dashboard.Section{
			{ID: "not_ready", Title: "Not Ready", Query: dashboard.Query{Type: dashboard.QueryTypeBlockedIssues, BlockedIssues: domain.BlockedIssuesQuery{Limit: 25}}},
			{ID: "ready", Title: "Ready", Query: dashboard.Query{Type: dashboard.QueryTypeReadyIssues, ReadyIssues: domain.ReadyIssuesQuery{Limit: 25}}},
			{ID: "in_progress", Title: "In Progress", Query: dashboard.Query{Type: dashboard.QueryTypeListIssues, ListIssues: domain.IssueListQuery{Statuses: []string{"in_progress"}, Limit: 25}}},
			{ID: "done", Title: "Done", Query: dashboard.Query{Type: dashboard.QueryTypeListIssues, ListIssues: domain.IssueListQuery{Statuses: []string{"closed"}, SortBy: domain.SortFieldUpdatedAt, SortOrder: domain.SortDirectionDescending, Limit: 25}}},
		},
	}}}

	tests := []struct {
		name     string
		width    int
		height   int
		golden   string
		mustShow []string
		minMeta  int
	}{
		{name: "w80", width: 80, height: 28, golden: "model_layout_w80.golden", mustShow: []string{"Default", "Not Ready", "Ready", "bw-1"}, minMeta: 4},
		{name: "w120", width: 120, height: 30, golden: "model_layout_w120.golden", mustShow: []string{"Default", "Not Ready", "In Progress", "bw-1", "bw-3"}, minMeta: 6},
		{name: "w180", width: 180, height: 34, golden: "model_layout_w180.golden", mustShow: []string{"Default", "Not Ready", "Done", "bw-1", "bw-2", "bw-3"}, minMeta: 8},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tm := testui.NewTestModelWithSize(t, controllerAdapter{controller: NewModel(gateway, provider, resolvedBoardKeys(t))}, tc.width, tc.height)
			t.Cleanup(func() {
				_ = tm.Quit()
			})

			tm.Send(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			_ = testui.WaitForOutputContainsAll(t, tm.Output(), tc.mustShow...)

			if err := tm.Quit(); err != nil {
				t.Fatalf("failed to quit teatest model: %v", err)
			}

			final, ok := tm.FinalModel(t).(controllerAdapter)
			if !ok {
				t.Fatalf("expected final model adapter")
			}
			finalModel, ok := final.controller.(*Model)
			if !ok {
				t.Fatalf("expected wrapped board model, got %T", final.controller)
			}

			view := finalModel.View()
			testui.AssertMatchesGoldenNormalized(t, []byte(view), tc.golden)
			assertCompactIssueRows(t, view, tc.minMeta)
		})
	}
}

func assertCompactIssueRows(t *testing.T, view string, minIssueMetaLines int) {
	t.Helper()

	lines := strings.Split(view, "\n")
	issueRows := 0
	for _, line := range lines {
		issueRows += strings.Count(line, " P0 ")
		issueRows += strings.Count(line, " P1 ")
		issueRows += strings.Count(line, " P2 ")
		issueRows += strings.Count(line, " P3 ")
	}

	if issueRows < minIssueMetaLines {
		t.Fatalf("expected at least %d rendered issue rows, got %d\nview:\n%s", minIssueMetaLines, issueRows, view)
	}

	for _, forbidden := range []string{"Title:", "Description:", "Assignee:", "Labels:"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("expected board layout to keep compact one-line issue rows without detail-field chrome %q\nview:\n%s", forbidden, view)
		}
	}
}
