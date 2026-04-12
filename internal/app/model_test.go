package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	launchereditor "github.com/hk9890/beads-workbench/internal/launcher/editor"
	"github.com/hk9890/beads-workbench/internal/mode"
	detailsmode "github.com/hk9890/beads-workbench/internal/mode/details"
	"github.com/hk9890/beads-workbench/internal/testing/e2e/embeddedfixture"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	"github.com/hk9890/beads-workbench/internal/testing/ui"
	"github.com/hk9890/beads-workbench/internal/ui/modal"
)

func TestModelInitUsesBoardControllerAndBuiltInDashboardQueries(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked", Status: "blocked", Priority: 1}}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	if m.board == nil {
		t.Fatalf("expected board controller to be configured")
	}

	initCmd := m.Init()
	if initCmd == nil {
		t.Fatalf("expected init command")
	}

	msgs := runBatch(initCmd)
	m = applyMessages(t, m, msgs)

	if got := firstSelectionID(m, mode.Board); got != "bw-3" {
		t.Fatalf("expected board selection from board controller, got %q", got)
	}

	if !gateway.HasCall(string(fakes.MethodReadyIssues)) {
		t.Fatalf("expected ready issues query from built-in dashboard")
	}
	if !gateway.HasCall(string(fakes.MethodListIssues)) {
		t.Fatalf("expected list issues query from built-in dashboard")
	}
	if !gateway.HasCall(string(fakes.MethodBlockedIssues)) {
		t.Fatalf("expected blocked issues query from built-in dashboard")
	}

	if m.renderBody() == "" {
		t.Fatalf("expected board body rendering from board controller")
	}
}

func TestModelStartupSynchronizesSelectionWhenBoardContentBecomesVisible(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "startup detail"}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	queue := runBatch(m.Init())

	observedVisibleBoardState := false
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]

		next, cmd := m.Update(msg)
		m = next.(Model)

		if !observedVisibleBoardState && !m.boardIsLoading() {
			body := m.renderBody()
			if strings.Contains(body, "Ready first") {
				header := m.renderHeader()
				if strings.Contains(header, "Selected: none") {
					t.Fatalf("expected startup header selection to sync once board content is visible, got header:\n%s\nbody:\n%s", header, body)
				}
				if !strings.Contains(header, "Selected: bw-1 (open)") {
					t.Fatalf("expected startup header to show active board selection, got:\n%s", header)
				}
				footer := m.renderFooter()
				if !strings.Contains(footer, "Board:") {
					t.Fatalf("expected mode-specific help footer in board mode, got:\n%s", footer)
				}
				observedVisibleBoardState = true
			}
		}

		queue = append(queue, runBatch(cmd)...)
	}

	if !observedVisibleBoardState {
		t.Fatalf("expected to observe visible startup board state during init flow")
	}
}

func TestModelBoardNavigationUpdatesShellSelectionAndDetailState(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{
		{ID: "bw-2", Title: "In progress one", Status: "in_progress", Priority: 2},
		{ID: "bw-4", Title: "In progress two", Status: "in_progress", Priority: 1},
	}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if got := firstSelectionID(m, mode.Board); got != "bw-1" {
		t.Fatalf("expected initial board selection bw-1, got %q", got)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving board column")
	}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-2", Title: "In progress one", Status: "in_progress", Priority: 2}, Description: "detail for bw-2"}
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "bw-2" {
		t.Fatalf("expected board selection bw-2 after moving right, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "bw-2" {
		t.Fatalf("expected shell detail state to load bw-2, got %q", m.detail.Detail.Summary.ID)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving board row")
	}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-4", Title: "In progress two", Status: "in_progress", Priority: 1}, Description: "detail for bw-4"}
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "bw-4" {
		t.Fatalf("expected board selection bw-4 after moving down, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "bw-4" {
		t.Fatalf("expected shell detail state to update to bw-4, got %q", m.detail.Detail.Summary.ID)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected board open-detail action request command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	if m.active != mode.Detail {
		t.Fatalf("expected active mode detail after board enter, got %s", m.active)
	}
	if cmd != nil {
		gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-4", Title: "In progress two", Status: "in_progress", Priority: 1}, Description: "detail for bw-4"}
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	if m.detail.TargetID != "bw-4" {
		t.Fatalf("expected detail target to track board selection, got %q", m.detail.TargetID)
	}
}

func TestModelSearchTextEntryIsNotHijackedByShellHotkeys(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected active mode search before typing, got %s", m.active)
	}

	gateway.ResetCalls()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected active mode to stay search while typing, got %s", m.active)
	}

	ui.AssertLatestSearchQueryText(t, gateway.Calls, "b")
}

func TestModelSearchModeRendersRepresentativeErrorAndEmptyStates(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Enter search mode.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// Trigger a gateway-backed search error.
	gateway.SetError(fakes.MethodSearchIssues, errors.New("search boom"))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if view := m.View(); !strings.Contains(view, "search boom") {
		t.Fatalf("expected search error state in shell view, got:\n%s", view)
	}

	// Clear error and run another non-empty query that returns no results.
	gateway.SetError(fakes.MethodSearchIssues, nil)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if view := m.View(); !strings.Contains(view, "No results found.") {
		t.Fatalf("expected search empty state in shell view, got:\n%s", view)
	}

	if got := firstSelectionID(m, mode.Search); got != "" {
		t.Fatalf("expected no search selection in empty state, got %q", got)
	}
}

func TestModelCtrlSpaceTogglesSearchAndEscReturnsBoard(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected ctrl+space equivalent to enter search, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Board {
		t.Fatalf("expected esc from search to return to board, got %s", m.active)
	}
	if m.lastBrowse != mode.Board {
		t.Fatalf("expected lastBrowse to return to board, got %s", m.lastBrowse)
	}
}

func TestModelDefaultTabAndShiftTabDoNotCycleModes(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Board {
		t.Fatalf("expected shift+tab from board not to switch modes, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected ctrl+space to switch to search, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Board {
		t.Fatalf("expected ctrl+space to return to board, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected detail mode after hotkey 3, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected tab from detail not to cycle modes by default, got %s", m.active)
	}
}

func TestModelShowModeSwitcherHelpControlsFooterVisibility(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	cfg := config.Default()
	cfg.UI.ShowModeSwitcherHelp = false

	services, err := NewServices(gateway, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if footer := m.renderFooter(); strings.TrimSpace(footer) != "" {
		t.Fatalf("expected footer help hidden when ShowModeSwitcherHelp is false, got:\n%s", footer)
	}
}

func TestModelUsesConfiguredShellAndBoardKeyBindings(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}, Description: "detail"}

	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{
			config.ShellActionHelp:         {"F1"},
			config.ShellActionModeSearch:   {"/"},
			config.ShellActionToggleSearch: {"ctrl+s"},
			config.ShellActionQuit:         {"ctrl+q"},
		},
		Board: map[string][]string{
			config.BoardActionMoveRight: {"d"},
			config.BoardActionMoveDown:  {"s"},
		},
	})

	services, err := NewServices(gateway, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if footer := m.renderFooter(); !strings.Contains(footer, "ctrl+s search") || !strings.Contains(footer, "ctrl+q quit") {
		t.Fatalf("expected footer to reflect configured bindings, got:\n%s", footer)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected configured mode_search key to switch to search, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Board {
		t.Fatalf("expected configured toggle_search key to return to board, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "bw-2" {
		t.Fatalf("expected configured board move-right key to select bw-2, got %q", got)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if m.showHelp {
		t.Fatal("expected default help key to stop working after override")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	m = next.(Model)
	if m.showHelp {
		t.Fatal("expected plain F rune not to trigger help")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = next.(Model)
	if !m.showHelp {
		t.Fatal("expected configured help key to show help")
	}
	m.showHelp = false

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected configured quit key to return quit command")
	}
	msgs := runBatch(cmd)
	foundQuit := false
	for _, msg := range msgs {
		if _, ok := msg.(tea.QuitMsg); ok {
			foundQuit = true
			break
		}
	}
	if !foundQuit {
		t.Fatalf("expected quit command batch, got %#v", msgs)
	}
}

func TestModelEditHotkeyUsesEditorService(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Assignee: "hans", Labels: []string{"infra"}, Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	services.Editor = fakeEditor

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(fakeEditor.Calls) != 1 {
		t.Fatalf("expected one editor call, got %d", len(fakeEditor.Calls))
	}
	if fakeEditor.Calls[0].IssueID != "bw-1" {
		t.Fatalf("expected selected issue bw-1, got %q", fakeEditor.Calls[0].IssueID)
	}

	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected edit hotkey to avoid launcher service, got %#v", fakeLauncher.Calls)
	}
}

func TestModelEditHotkeyShowsErrorToastWhenEditorFails(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{Err: errors.New("editor boom")}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	services.Editor = fakeEditor

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected launcher command after edit hotkey")
	}

	next, _ = m.Update(cmd())
	m = next.(Model)

	view := m.View()
	if !strings.Contains(view, "Failed to edit issue bw-1") {
		t.Fatalf("expected editor failure toast, got:\n%s", view)
	}

	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected no launcher calls when editor fails, got %#v", fakeLauncher.Calls)
	}
}

func TestModelCreateIssueFlowUsesGatewayCatalogsAndCreateIssue(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.StatusCatalogResponse = []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}}
	gateway.TypeCatalogResponse = []domain.TypeOption{{Name: "task"}, {Name: "bug"}}
	gateway.LabelCatalogResponse = []domain.LabelOption{{Name: "ui"}, {Name: "infra"}}
	gateway.CreateIssueResponse = domain.CreateIssueResult{IssueID: "bw-99"}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected command for create flow")
	}

	next, cmd = m.Update(cmd())
	m = next.(Model)

	if !m.showActionModal {
		t.Fatalf("expected action modal to open for create")
	}

	submit := modal.SubmitMsg{Values: map[string]string{
		"title":       "Create from modal",
		"type":        "task",
		"priority":    "2",
		"assignee":    "hans",
		"labels":      "ui,infra",
		"description": "created from test",
	}}
	next, cmd = m.Update(submit)
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected submit mutation command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	if !gateway.HasCall(string(fakes.MethodStatusCatalog)) || !gateway.HasCall(string(fakes.MethodTypeCatalog)) || !gateway.HasCall(string(fakes.MethodLabelCatalog)) {
		t.Fatalf("expected status/type/label catalogs to be queried, calls=%#v", gateway.Calls)
	}

	if !gateway.HasCall(string(fakes.MethodCreateIssue)) {
		t.Fatalf("expected create issue gateway call, calls=%#v", gateway.Calls)
	}
}

func TestModelUpdateCloseAndCommentFlowsUseGatewayWrites(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1, Labels: []string{"ui"}}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1, Labels: []string{"ui"}}}
	gateway.StatusCatalogResponse = []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}}
	gateway.TypeCatalogResponse = []domain.TypeOption{{Name: "task"}, {Name: "bug"}}
	gateway.LabelCatalogResponse = []domain.LabelOption{{Name: "ui"}, {Name: "infra"}}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected command for update flow")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	next, cmd = m.Update(modal.SubmitMsg{Values: map[string]string{
		"title":    "Updated title",
		"status":   "in_progress",
		"type":     "task",
		"priority": "2",
		"assignee": "hans",
		"labels":   "ui,infra",
	}})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected update mutation command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected modal init command for close flow")
	}
	next, cmd = m.Update(modal.SubmitMsg{Values: map[string]string{"reason": "done"}})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected close mutation command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected modal init command for comment flow")
	}
	next, cmd = m.Update(modal.SubmitMsg{Values: map[string]string{"body": "looks good"}})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected comment mutation command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	if !gateway.HasCall(string(fakes.MethodUpdateIssue)) {
		t.Fatalf("expected update issue call, calls=%#v", gateway.Calls)
	}
	if !gateway.HasCall(string(fakes.MethodCloseIssue)) {
		t.Fatalf("expected close issue call, calls=%#v", gateway.Calls)
	}
	if !gateway.HasCall(string(fakes.MethodAddComment)) {
		t.Fatalf("expected add comment call, calls=%#v", gateway.Calls)
	}
}

func TestModelBuiltInLauncherHotkeysUseLauncherService(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1, Labels: []string{"ui"}}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(fakeLauncher.Calls) != 1 || fakeLauncher.Calls[0].Action != "nvim" {
		t.Fatalf("expected nvim launcher call before toast assertion, got %#v", fakeLauncher.Calls)
	}

	next, _ = m.Update(launchActionResultMsg{action: "nvim", err: nil})
	m = next.(Model)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(fakeLauncher.Calls) != 3 {
		t.Fatalf("expected 3 launcher calls, got %d", len(fakeLauncher.Calls))
	}

	actions := []string{fakeLauncher.Calls[0].Action, fakeLauncher.Calls[1].Action, fakeLauncher.Calls[2].Action}
	if actions[0] != "nvim" || actions[1] != "opencode" || actions[2] != "shell-command" {
		t.Fatalf("expected launcher actions [nvim opencode shell-command], got %#v", actions)
	}
}

func TestModelDetailModeSupportsScrollingLongContent(t *testing.T) {
	t.Parallel()

	longLines := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		longLines = append(longLines, "Line "+strconv.Itoa(i))
	}

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: strings.Join(longLines, "\n"),
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m.width = 90
	m.height = 16
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	viewTop := m.View()
	if !strings.Contains(viewTop, "Metadata") || !strings.Contains(viewTop, "Core") || !strings.Contains(viewTop, "Type    : task") {
		t.Fatalf("expected metadata section in initial detail view, got:\n%s", viewTop)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	viewPaged := m.View()
	if viewPaged == viewTop {
		t.Fatalf("expected detail view to change after page down")
	}
	if !strings.Contains(viewPaged, "Line 1") {
		t.Fatalf("expected first description lines after paging, got:\n%s", viewPaged)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	viewEnd := m.View()
	if !strings.Contains(viewEnd, "Line 80") {
		t.Fatalf("expected to reach bottom content after end key, got:\n%s", viewEnd)
	}
}

func TestModelDetailModeLeftBrowserEnterOpensIssueAndStaysInDetailMode(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-9", Title: "Other", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{
			Parent: domain.IssueReference{ID: "bw-1", Title: "Root"},
			Children: []domain.IssueReference{
				{ID: "bw-5", Title: "Sibling target"},
				{ID: "bw-6", Title: "Sibling peer"},
			},
		},
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m.width = 160
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	m.detail.ScrollOffset = 5

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-5", Title: "Sibling target", Status: "in_progress", Type: "bug", Priority: 2},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{
			Parent: domain.IssueReference{ID: "bw-1", Title: "Root"},
			Children: []domain.IssueReference{
				{ID: "bw-5", Title: "Sibling target"},
				{ID: "bw-6", Title: "Sibling peer"},
			},
		},
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected enter on left browser panel to trigger detail load command")
	}

	if m.active != mode.Detail {
		t.Fatalf("expected app to remain in detail mode after related open, got %s", m.active)
	}
	if m.detail.TargetID != "bw-5" || m.detail.SelectionID != "bw-5" {
		t.Fatalf("expected browser issue bw-5 to become detail target/selection, got target=%q selection=%q", m.detail.TargetID, m.detail.SelectionID)
	}
	if m.detail.ScrollOffset != 0 {
		t.Fatalf("expected browser issue open to reset content scroll offset, got %d", m.detail.ScrollOffset)
	}

	m = applyMessages(t, m, runBatch(cmd))
	if m.detail.Detail.Summary.ID != "bw-5" {
		t.Fatalf("expected loaded browser issue detail bw-5, got %q", m.detail.Detail.Summary.ID)
	}
	if len(m.detail.BrowserItems) != 3 {
		t.Fatalf("expected stable parent-group browser items (parent + siblings), got %d", len(m.detail.BrowserItems))
	}
}

func TestModelDetailMetadataEnterOpensStatusDialogAndSubmitsStatusUpdate(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "Other", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1},
	}
	gateway.StatusCatalogResponse = []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}, {Name: "blocked"}}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m.width = 140
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected status catalog load command after enter on metadata status")
	}

	next, cmd = m.Update(cmd())
	m = next.(Model)
	_ = cmd

	if !m.showActionModal {
		t.Fatal("expected status action modal to open")
	}

	next, cmd = m.Update(modal.SubmitMsg{Values: map[string]string{"status": "in_progress"}})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected status update submit command")
	}

	next, _ = m.Update(cmd())
	m = next.(Model)

	if !gateway.HasCall(string(fakes.MethodStatusCatalog)) {
		t.Fatalf("expected status catalog query, calls=%#v", gateway.Calls)
	}
	if !gateway.HasCall(string(fakes.MethodUpdateIssue)) {
		t.Fatalf("expected status update issue call, calls=%#v", gateway.Calls)
	}

	foundStatusUpdate := false
	for _, call := range gateway.Calls {
		if call.Method != fakes.MethodUpdateIssue {
			continue
		}
		updateCall, ok := call.Input.(fakes.UpdateIssueCall)
		if !ok {
			continue
		}
		if updateCall.Input.Status == nil || *updateCall.Input.Status != "in_progress" {
			t.Fatalf("expected status-only update to in_progress, got %#v", updateCall.Input)
		}
		if updateCall.Input.Priority != nil {
			t.Fatalf("expected priority editing out of scope; got priority update %#v", *updateCall.Input.Priority)
		}
		foundStatusUpdate = true
	}
	if !foundStatusUpdate {
		t.Fatal("expected to capture update issue input for status edit")
	}
}

func TestModelLauncherSuccessToastClarifiesBackgroundLifecycle(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, _ := m.Update(launchActionResultMsg{action: "nvim", err: nil})
	m = next.(Model)

	view := m.View()
	if !strings.Contains(view, "background (no return flow)") || !strings.Contains(view, "Use e for edit/save round-trip") {
		t.Fatalf("expected launcher lifecycle guidance toast, got:\n%s", view)
	}
}

func TestModelDetailModeRendersStandaloneDetailGolden(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "Ninth detail",
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	view := m.View()
	if !strings.Contains(view, "Detail: j/k scroll") || !strings.Contains(view, "home/end bounds") {
		t.Fatalf("expected detail footer help to include configurable detail bindings, got:\n%s", view)
	}
}

func TestModelWideBoardViewPrioritizesBoardAndResponsiveColumns(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "beads-workbench-yze.4.2", Title: "Implement create update close and comment actions in the app", Status: "open", Type: "task", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "beads-workbench-yze.4.3", Title: "Implement launcher framework with issue-context interpolation", Status: "in_progress", Type: "task", Priority: 1}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "beads-workbench-yze.4.5", Title: "Add editor and launcher integration tests", Status: "blocked", Type: "task", Priority: 1}}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       "beads-workbench-yze.4.2",
			Title:    "Implement create update close and comment actions in the app",
			Status:   "open",
			Type:     "task",
			Priority: 1,
			Assignee: "alice",
			Labels:   []string{"ui", "shell"},
		},
		Description: "Show selected issue context clearly in browse mode.",
		BlockedBy:   []domain.IssueReference{{ID: "bw-9", Title: "Upstream migration"}},
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m.width = 160
	m.height = 42
	m = applyMessages(t, m, runBatch(m.Init()))

	view := m.View()
	if strings.Contains(view, "Selected Issue") {
		t.Fatalf("expected no selected issue sidebar in board view, got:\n%s", view)
	}
	if strings.Contains(view, "Title:") || strings.Contains(view, "Description:") {
		t.Fatalf("expected full detail fields to stay in dedicated detail mode, got:\n%s", view)
	}
	if !strings.Contains(view, "Default") {
		t.Fatalf("expected board header in wide view, got:\n%s", view)
	}
	if !strings.Contains(view, "Implement create update") {
		t.Fatalf("expected readable board row title text in wide view, got:\n%s", view)
	}
}

func TestModelBoardShellUsesSingleLineHeaderAndFooterHelpAt120Cols(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked", Status: "blocked", Type: "bug", Priority: 0}}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	header := m.renderHeader()
	if strings.Contains(header, "\n") {
		t.Fatalf("expected single-line header, got:\n%s", header)
	}
	if strings.Contains(header, "Detail") {
		t.Fatalf("expected detail to be contextual and absent from top tabs, got:\n%s", header)
	}

	footer := m.renderFooter()
	if !strings.Contains(footer, "ctrl+space search") {
		t.Fatalf("expected board footer help with ctrl+space hint, got:\n%s", footer)
	}
}

func TestModelEditIssueActionUsesEditorServiceAndUpdatesDetail(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "detail before edit",
	}

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	fakeEditor := &fakes.FakeEditor{Result: launchereditor.Result{Updated: true}}
	services.Editor = fakeEditor

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if m.detail.Detail.Summary.ID != "bw-9" {
		t.Fatalf("expected initial detail load for selected issue bw-9, got %q", m.detail.Detail.Summary.ID)
	}

	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-9", Title: "Ninth edited", Status: "open", Type: "task", Priority: 2},
		Description: "detail after edit",
	}
	gateway.ResetCalls()

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected edit command from edit hotkey")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected reload command after successful editor update")
	}
	m = applyMessages(t, m, runBatch(cmd))

	if len(fakeEditor.Calls) != 1 {
		t.Fatalf("expected one editor call, got %d", len(fakeEditor.Calls))
	}
	if fakeEditor.Calls[0].IssueID != "bw-9" {
		t.Fatalf("expected editor call for bw-9, got %q", fakeEditor.Calls[0].IssueID)
	}

	if !gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("expected detail reload via ShowIssue after successful update, calls=%#v", gateway.Calls)
	}

	if m.detail.Detail.Summary.Title != "Ninth edited" {
		t.Fatalf("expected updated detail title after reload, got %q", m.detail.Detail.Summary.Title)
	}
	if m.detail.Detail.Description != "detail after edit" {
		t.Fatalf("expected updated detail description after reload, got %q", m.detail.Detail.Description)
	}
}

func TestModelEditHotkeyInDetailModeUsesEditorService(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "detail before edit",
	}

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	fakeEditor := &fakes.FakeEditor{}
	services.Editor = fakeEditor

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	gateway.ResetCalls()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected editor command from edit hotkey")
	}

	next, _ = m.Update(cmd())
	m = next.(Model)

	if len(fakeEditor.Calls) != 1 {
		t.Fatalf("expected one editor call, got %d", len(fakeEditor.Calls))
	}
	if fakeEditor.Calls[0].IssueID != "bw-9" {
		t.Fatalf("expected selected detail issue bw-9, got %q", fakeEditor.Calls[0].IssueID)
	}

	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected no launcher calls for edit hotkey, got %#v", fakeLauncher.Calls)
	}

	if gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("did not expect issue reload from launcher action, calls=%#v", gateway.Calls)
	}
}

func TestModelEmbeddedFixtureBoardToDetailSmokeWorkflow(t *testing.T) {
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

	services, err := NewServices(gateway, config.Default(), repoPath)
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Startup lands on Not Ready lane when blocked work exists.
	if got := firstSelectionID(m, mode.Board); got != "bwf-2" {
		t.Fatalf("expected startup board selection bwf-2 from Not Ready lane, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "bwf-2" {
		t.Fatalf("expected shell detail cache to load bwf-2, got %q", m.detail.Detail.Summary.ID)
	}

	if view := m.View(); strings.Contains(view, "Selected Issue") {
		t.Fatalf("expected no sidebar on browse board, got:\n%s", view)
	}

	// Open dedicated detail mode from board selection.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected active mode detail after enter, got %s", m.active)
	}
	if m.detail.TargetID != "bwf-2" {
		t.Fatalf("expected detail target bwf-2 in dedicated mode, got %q", m.detail.TargetID)
	}

	view := m.View()
	if !strings.Contains(view, "Blocked bug for fixture") {
		t.Fatalf("expected dedicated detail rendering for fixture issue, got:\n%s", view)
	}
	if strings.Contains(view, "Issue Detail") {
		t.Fatalf("expected detail mode to avoid extra shell wrapper heading, got:\n%s", view)
	}
	if !strings.Contains(view, "Assignee: bob") {
		t.Fatalf("expected detail metadata to show fixture assignee bob, got:\n%s", view)
	}
	if strings.Contains(view, "Assignee: hans.kohlreiter@dynatrace.com") {
		t.Fatalf("expected detail metadata to avoid owner in assignee slot, got:\n%s", view)
	}
}

func TestModelEmbeddedFixtureDetailEditHotkeyUsesEditorService(t *testing.T) {
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

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	fakeEditor := &fakes.FakeEditor{}
	services.Editor = fakeEditor

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if got := firstSelectionID(m, mode.Board); got != "bwf-2" {
		t.Fatalf("expected startup board selection bwf-2 from fixture seed, got %q", got)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected detail mode after enter, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected edit command from detail edit hotkey")
	}

	next, _ = m.Update(cmd())
	m = next.(Model)

	if len(fakeEditor.Calls) != 1 {
		t.Fatalf("expected one editor call, got %d", len(fakeEditor.Calls))
	}
	if fakeEditor.Calls[0].IssueID != "bwf-2" {
		t.Fatalf("expected editor issue bwf-2 from embedded fixture, got %q", fakeEditor.Calls[0].IssueID)
	}
	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected no launcher call from detail edit hotkey, got %#v", fakeLauncher.Calls)
	}
}

func TestModelEmbeddedFixtureFullBoardCaptureGolden(t *testing.T) {
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

	services, err := NewServices(gateway, config.Default(), repoPath)
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	view := m.View()
	if strings.Contains(view, "Selected Issue") {
		t.Fatalf("expected board view without selected issue sidebar, got:\n%s", view)
	}
	if !strings.Contains(view, "bwf-1 Seed fixture roo") {
		t.Fatalf("expected realistic fixture issue title in board capture, got:\n%s", view)
	}
	if strings.Count(view, "│") < 20 {
		t.Fatalf("expected full-height board lanes rather than floating boxes, got:\n%s", view)
	}

	ui.AssertMatchesGoldenNormalized(t, []byte(view), "model_embedded_board_w120.golden")
}

func TestModelEmbeddedFixtureStartupLoadsBoardWithoutGatewaySectionErrors(t *testing.T) {
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

	services, err := NewServices(gateway, config.Default(), repoPath)
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	view := m.View()
	ui.AssertStartupBoardLayoutSanity(t, view)
	ui.AssertContainsAll(t, view, "bwf-1")
	ui.AssertNoObviousRuntimeErrorPanels(t, view)
}

func TestModelEmbeddedFixtureDetailShowsRelatedFromRealBDRelatedLink(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	if err := runBDInRepo(repoPath, "link", "bwf-2", "bwf-3", "--type", "related"); err != nil {
		t.Fatalf("failed to create real related link: %v", err)
	}

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir: repoPath,
		Env:     append(os.Environ(), "BD_NON_INTERACTIVE=1"),
	})
	gateway := beads.NewCLIGateway(runner)

	services, err := NewServices(gateway, config.Default(), repoPath)
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if got := firstSelectionID(m, mode.Board); got != "bwf-2" {
		t.Fatalf("expected startup board selection bwf-2 from Not Ready lane, got %q", got)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected active mode detail after enter, got %s", m.active)
	}

	view := m.View()
	if !strings.Contains(view, "Related") {
		t.Fatalf("expected related rail/section in detail view, got:\n%s", view)
	}
	if !strings.Contains(view, "bwf-3") {
		t.Fatalf("expected linked related issue bwf-3 in detail view, got:\n%s", view)
	}
	if !strings.Contains(view, "Closed fixture chore") {
		t.Fatalf("expected related issue title in detail view, got:\n%s", view)
	}
}

func TestModelBoardDetailBoardRoundTripPreservesLayoutAndFocus(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyIssuesResponse = []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}
	gateway.ListIssuesResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress one", Status: "in_progress", Type: "task", Priority: 1}}
	gateway.BlockedIssuesResponse = []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked now", Status: "blocked", Type: "bug", Priority: 0}}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: "detail for ready issue",
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := NewModel(services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: "detail for ready issue",
	}
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-2", Title: "In progress one", Status: "in_progress", Type: "task", Priority: 1},
		Description: "detail for in-progress issue",
	}
	m = applyMessages(t, m, runBatch(cmd))

	if got := firstSelectionID(m, mode.Board); got != "bw-2" {
		t.Fatalf("expected board selection bw-2 before round-trip, got %q", got)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected detail mode after enter, got %s", m.active)
	}
	detailView := m.View()
	if !strings.Contains(detailView, "In progress one") {
		t.Fatalf("expected dedicated detail layout with selected issue content, got:\n%s", detailView)
	}
	if strings.Contains(detailView, "Issue Detail") {
		t.Fatalf("expected dedicated detail layout without extra shell wrapper heading, got:\n%s", detailView)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Board {
		t.Fatalf("expected board mode after esc from detail, got %s", m.active)
	}
	if got := firstSelectionID(m, mode.Board); got != "bw-2" {
		t.Fatalf("expected board selection to remain on bw-2 after round-trip, got %q", got)
	}

	boardView := m.View()
	if strings.Contains(boardView, "Selected Issue") {
		t.Fatalf("expected board layout without detail sidebar after round-trip, got:\n%s", boardView)
	}
	if !strings.Contains(boardView, "In progress one") {
		t.Fatalf("expected board row title to remain visible after round-trip, got:\n%s", boardView)
	}

	ui.AssertMatchesGoldenNormalized(t, []byte(boardView), "model_roundtrip_board_back_w120.golden")
}

func TestModelSharedWorkspaceContractUsesFullBodyHeightAcrossModes(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	boardSpy := &sizingSpyController{id: mode.Board, viewText: "board"}
	searchSpy := &sizingSpyController{id: mode.Search, viewText: "search"}

	m := NewModel(services)
	m.board = boardSpy
	m.search = searchSpy
	m.width = 120
	m.height = 34

	expectedWidth, expectedHeight := m.workspaceSize()

	m.active = mode.Board
	_ = m.renderBody()
	if boardSpy.lastWidth != expectedWidth || boardSpy.lastHeight != expectedHeight {
		t.Fatalf("expected board SetSize(%d,%d), got (%d,%d)", expectedWidth, expectedHeight, boardSpy.lastWidth, boardSpy.lastHeight)
	}
	if searchSpy.lastWidth != expectedWidth || searchSpy.lastHeight != expectedHeight {
		t.Fatalf("expected search SetSize(%d,%d), got (%d,%d)", expectedWidth, expectedHeight, searchSpy.lastWidth, searchSpy.lastHeight)
	}

	m.active = mode.Search
	body := m.renderBody()
	if !strings.Contains(body, "search") {
		t.Fatalf("expected active search view rendering, got: %q", body)
	}

	longLines := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		longLines = append(longLines, fmt.Sprintf("Line %d", i))
	}
	m.active = mode.Detail
	m.detail = detailsmode.Model{
		SelectionID: "bw-1",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "bw-1", Title: "Issue one", Status: "open", Type: "task", Priority: 1},
			Description: strings.Join(longLines, "\n"),
		},
	}

	detailBody := m.renderBody()
	if strings.Contains(detailBody, "Issue Detail") {
		t.Fatalf("expected detail body to avoid extra boxed wrapper heading, got:\n%s", detailBody)
	}
	if got := len(strings.Split(detailBody, "\n")); got != expectedHeight {
		t.Fatalf("expected detail body height %d lines, got %d", expectedHeight, got)
	}
	if m.detailViewportWidth() != expectedWidth {
		t.Fatalf("expected detail viewport width %d, got %d", expectedWidth, m.detailViewportWidth())
	}
	if m.detailViewportHeight() != expectedHeight {
		t.Fatalf("expected detail viewport height %d, got %d", expectedHeight, m.detailViewportHeight())
	}
}

type sizingSpyController struct {
	id         mode.ID
	viewText   string
	lastWidth  int
	lastHeight int
}

func (s *sizingSpyController) ID() mode.ID { return s.id }

func (s *sizingSpyController) Init() tea.Cmd { return nil }

func (s *sizingSpyController) Update(tea.Msg) (mode.Controller, tea.Cmd) { return s, nil }

func (s *sizingSpyController) View() string { return s.viewText }

func (s *sizingSpyController) SetSize(width, height int) {
	s.lastWidth = width
	s.lastHeight = height
}

func runBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	var msgs []tea.Msg
	queue := []tea.Msg{cmd()}
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		switch v := msg.(type) {
		case tea.BatchMsg:
			for _, c := range v {
				if c == nil {
					continue
				}
				queue = append(queue, c())
			}
		default:
			msgs = append(msgs, msg)
		}
	}

	return msgs
}

func applyMessages(t *testing.T, model Model, msgs []tea.Msg) Model {
	t.Helper()

	m := model
	queue := append([]tea.Msg(nil), msgs...)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]

		next, cmd := m.Update(msg)
		m = next.(Model)
		queue = append(queue, runBatch(cmd)...)
	}

	return m
}

func firstSelectionID(m Model, modeID mode.ID) string {
	sel := m.selectedByMode[modeID]
	if sel == nil {
		return ""
	}
	return sel.Issue.ID
}

func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runBDInRepo(repoPath string, args ...string) error {
	cmd := exec.Command("bd", args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd %s failed: %w\n%s", strings.Join(args, " "), err, out)
	}

	return nil
}
