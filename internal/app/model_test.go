package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestMain(m *testing.M) {
	scheduleRefreshTickCmd = func() tea.Cmd { return nil }
	modelNow = time.Now
	os.Exit(m.Run())
}

// mustNewModel wraps NewModel and fails the test if an error is returned.
func mustNewModel(t *testing.T, services Services) Model {
	t.Helper()
	m, err := NewModel(services)
	if err != nil {
		t.Fatalf("NewModel returned unexpected error: %v", err)
	}
	return m
}

// mustNewModelWithOptions wraps NewModelWithOptions and fails the test if an error is returned.
func mustNewModelWithOptions(t *testing.T, services Services, runtime RuntimeOptions) Model {
	t.Helper()
	m, err := NewModelWithOptions(services, runtime)
	if err != nil {
		t.Fatalf("NewModelWithOptions returned unexpected error: %v", err)
	}
	return m
}

func TestModelInitUsesBoardControllerAndBuiltInDashboardQueries(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready:   []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}},
		Blocked: []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked", Status: "blocked", Priority: 1}}},
	}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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

	if !gateway.HasCall(string(fakes.MethodReadyExplain)) {
		t.Fatalf("expected ReadyExplain call from board controller")
	}
	if !gateway.HasCall(string(fakes.MethodQuery)) {
		t.Fatalf("expected Query calls from board controller")
	}

	if m.renderBody() == "" {
		t.Fatalf("expected board body rendering from board controller")
	}
}

func TestModelStartupSynchronizesSelectionAfterBoardInitSelectionMessage(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "startup detail"}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
				if strings.Contains(header, "Selected: bw-1 (open)") {
					observedVisibleBoardState = true
				}
				footer := m.renderFooter()
				if !strings.Contains(footer, "Board:") {
					t.Fatalf("expected mode-specific help footer in board mode, got:\n%s", footer)
				}
			}
		}

		queue = append(queue, runBatch(cmd)...)
	}

	if !observedVisibleBoardState {
		t.Fatalf("expected to observe visible startup board state during init flow")
	}

	header := m.renderHeader()
	if !strings.Contains(header, "Selected: bw-1 (open)") {
		t.Fatalf("expected startup header to show active board selection after init messages, got:\n%s", header)
	}
}

func TestModelBoardNavigationUpdatesShellSelectionAndDetailState(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{
		{ID: "bw-2", Title: "In progress one", Status: "in_progress", Priority: 2},
		{ID: "bw-4", Title: "In progress two", Status: "in_progress", Priority: 1},
	}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	// After moving right: InProgress column sorted by priority: [bw-4(P1), bw-2(P2)].
	// First item selected is bw-4 (highest priority).
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-4", Title: "In progress two", Status: "in_progress", Priority: 1}, Description: "detail for bw-4"}
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "bw-4" {
		t.Fatalf("expected board selection bw-4 after moving right, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "bw-4" {
		t.Fatalf("expected shell detail state to load bw-4, got %q", m.detail.Detail.Summary.ID)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving board row")
	}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-2", Title: "In progress one", Status: "in_progress", Priority: 2}, Description: "detail for bw-2"}
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "bw-2" {
		t.Fatalf("expected board selection bw-2 after moving down, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "bw-2" {
		t.Fatalf("expected shell detail state to update to bw-2, got %q", m.detail.Detail.Summary.ID)
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
		gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-2", Title: "In progress one", Status: "in_progress", Priority: 2}, Description: "detail for bw-2"}
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	if m.detail.TargetID != "bw-2" {
		t.Fatalf("expected detail target to track board selection, got %q", m.detail.TargetID)
	}
}

func TestModelSearchTextEntryIsNotHijackedByShellHotkeys(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 200
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
	if len(gateway.Calls) != 0 {
		t.Fatalf("expected typing in search query not to run search until enter, got %#v", gateway.Calls)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	ui.AssertLatestSearchQueryText(t, gateway.Calls, "b")
}

func TestModelSearchModeRendersRepresentativeErrorAndEmptyStates(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if view := m.View(); !strings.Contains(view, "No matches for \"xy\".") {
		t.Fatalf("expected search empty state in shell view, got:\n%s", view)
	}

	if got := firstSelectionID(m, mode.Search); got != "" {
		t.Fatalf("expected no search selection in empty state, got %q", got)
	}
}

func TestModelCtrlSpaceTogglesSearchAndEscReturnsBoard(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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

func TestModelSearchEscFromResultsFocusReturnsToBoard(t *testing.T) {
	// Regression: Esc must trigger shell escape (return to board) even when
	// search focus is on Results / Content / Metadata, not just on Query.
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{
		{Issue: domain.IssueSummary{ID: "bw-3", Title: "Search result", Status: "open", Priority: 1}},
	}}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Enter search mode.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected search active after ctrl+space, got %s", m.active)
	}

	// Press down arrow to move search focus from Query to Results.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// Confirm search focus is now on Results (CapturesShellKey must return false for Esc).
	if m.search.CapturesShellKey(tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Fatal("expected CapturesShellKey to return false for Esc when focus is Results — shell escape must be reachable")
	}

	// Press Esc — shell escape handler should fire and return to board.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Board {
		t.Fatalf("expected Esc from Results focus to return to board, got %s", m.active)
	}
	if m.lastBrowse != mode.Board {
		t.Fatalf("expected lastBrowse to be board after Esc from search Results, got %s", m.lastBrowse)
	}
}

func TestModelRefreshTickFallbackWithoutFocusEventsReloadsActiveBoard(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })
	withModelNow(t, time.Unix(0, 0))

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	if m.focusKnown {
		t.Fatal("expected no focus events observed at startup")
	}

	withModelNow(t, time.Unix(61, 0))
	gateway.ResetCalls()
	next, cmd := m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gateway.HasCall(string(fakes.MethodReadyExplain)) || !gateway.HasCall(string(fakes.MethodQuery)) {
		t.Fatalf("expected board refresh from tick fallback without focus events, calls=%#v", gateway.Calls)
	}
}

func TestModelFocusRegainRefreshesOnceAndSkipsRepeatedFocus(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	gateway.ResetCalls()
	next, cmd := m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gateway.HasCall(string(fakes.MethodReadyExplain)) {
		t.Fatalf("expected initial focus event not to force refresh, calls=%#v", gateway.Calls)
	}

	next, cmd = m.Update(tea.BlurMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	m.markSurfaceRefreshed(mode.Board)
	next, cmd = m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if !gateway.HasCall(string(fakes.MethodReadyExplain)) || !gateway.HasCall(string(fakes.MethodQuery)) {
		t.Fatalf("expected focus regain to refresh active board, calls=%#v", gateway.Calls)
	}

	gateway.ResetCalls()
	next, cmd = m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gateway.HasCall(string(fakes.MethodReadyExplain)) {
		t.Fatalf("expected repeated focus while focused to avoid refresh spam, calls=%#v", gateway.Calls)
	}
}

func TestModelFocusRegainInDetailRefreshesImmediatelyWithoutStaleOrDirty(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected detail active before focus-regain refresh test, got %s", m.active)
	}

	m.markSurfaceRefreshed(mode.Detail)
	gateway.ResetCalls()

	next, cmd = m.Update(tea.BlurMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("expected focus regain to refresh active detail immediately, calls=%#v", gateway.Calls)
	}
	if gateway.HasCall(string(fakes.MethodReadyExplain)) || gateway.HasCall(string(fakes.MethodQuery)) || gateway.HasCall(string(fakes.MethodSearchIssues)) {
		t.Fatalf("expected focus regain in detail to refresh only active detail surface, calls=%#v", gateway.Calls)
	}
}

func TestModelRefreshTickReloadsOnlyActiveSearchSurface(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected active mode search before tick, got %s", m.active)
	}

	m.markSurfaceDirty(mode.Search)
	m.markSurfaceDirty(mode.Search)
	gateway.ResetCalls()
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gateway.HasCall(string(fakes.MethodSearchIssues)) {
		t.Fatalf("expected search surface refresh on tick when search is active, calls=%#v", gateway.Calls)
	}
	if gateway.HasCall(string(fakes.MethodReadyExplain)) || gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("expected tick refresh to target only active search surface, calls=%#v", gateway.Calls)
	}
}

func TestModelRefreshTickBoardAutoRefreshDoesNotSwitchModeOrClearDetailState(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready:   []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}},
		Blocked: []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked", Status: "blocked", Priority: 0}}},
	}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	if m.active != mode.Board {
		t.Fatalf("expected board active after init, got %s", m.active)
	}

	m.detail.SelectionID = "bw-3"
	m.detail.TargetID = "bw-3"
	m.detail.Detail = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-3", Title: "Blocked", Status: "blocked"}, Description: "cached detail"}
	m.detail.Error = ""
	m.detail.Loading = false

	gateway.ResetCalls()
	next, cmd := m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Board {
		t.Fatalf("expected board auto-refresh not to force mode switch, got %s", m.active)
	}
	if m.detail.Detail.Summary.ID != "bw-3" || m.detail.Detail.Description != "cached detail" {
		t.Fatalf("expected board auto-refresh not to clear shell detail cache, got %#v", m.detail.Detail)
	}
	if gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("expected board auto-refresh not to force detail reload when selection remains, calls=%#v", gateway.Calls)
	}
}

func TestModelRefreshTickSearchAutoRefreshDoesNotSwitchModeOrClearDetailState(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-9", Title: "Search result", Status: "open", Priority: 1}}}}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected search active before refresh, got %s", m.active)
	}

	m.detail.SelectionID = "bw-9"
	m.detail.TargetID = "bw-9"
	m.detail.Detail = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-9", Title: "Search result", Status: "open"}, Description: "cached detail"}
	m.detail.Error = ""
	m.detail.Loading = false

	gateway.ResetCalls()
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected search auto-refresh not to force mode switch, got %s", m.active)
	}
	if m.detail.Detail.Summary.ID != "bw-9" || m.detail.Detail.Description != "cached detail" {
		t.Fatalf("expected search auto-refresh not to clear shell detail cache, got %#v", m.detail.Detail)
	}
	if gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("expected search auto-refresh not to force detail reload when selection remains, calls=%#v", gateway.Calls)
	}
}

func TestModelFocusRegainInSearchReloadsWithoutMutatingQuery(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected active mode search before focus refresh, got %s", m.active)
	}

	gateway.ResetCalls()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if len(gateway.Calls) != 0 {
		t.Fatalf("expected query edit not to search before enter, got %#v", gateway.Calls)
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	ui.AssertLatestSearchQueryText(t, gateway.Calls, "x")
	m.markSurfaceRefreshed(mode.Search)
	gateway.ResetCalls()

	next, cmd = m.Update(tea.BlurMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gateway.HasCall(string(fakes.MethodSearchIssues)) {
		t.Fatalf("expected focus regain in search to refresh immediately, calls=%#v", gateway.Calls)
	}
	if gateway.HasCall(string(fakes.MethodReadyExplain)) || gateway.HasCall(string(fakes.MethodQuery)) || gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("expected search focus regain to refresh only active search surface, calls=%#v", gateway.Calls)
	}
	ui.AssertLatestSearchQueryText(t, gateway.Calls, "x")
}

func TestModelSearchHeaderUsesPageMetadataAndDraftQueryState(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{
		Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-9", Title: "Search result", Status: "open", Priority: 1}}},
		Metadata: domain.SearchResultMetadata{ReturnedCount: 7, RequestedLimit: 40, Completeness: domain.SearchResultCompletenessMaybeMore},
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	header := m.renderHeader()
	if !strings.Contains(header, "Search: 7 results") {
		t.Fatalf("expected search header to use returned-count metadata, got:\n%s", header)
	}
	if !strings.Contains(header, "Selected: bw-9 (open)") {
		t.Fatalf("expected header to keep active search selection, got:\n%s", header)
	}
	if got := m.search.SessionState(); got.DraftQuery != "xy" || got.AppliedQuery != "x" {
		t.Fatalf("expected app shell to preserve draft/applied query split, got %#v", got)
	}
}

func TestModelSearchPreviewSyncKeepsLastLoadedPreviewDuringReloadAndError(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-9", Title: "Search result", Status: "open", Priority: 1}}}}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-9", Title: "Search result", Status: "open", Priority: 1}, Description: "cached detail"}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.detail.Detail.Summary.ID != "bw-9" {
		t.Fatalf("expected search selection detail to load, got %#v", m.detail.Detail)
	}
	m.renderBody()
	if got := m.search.SessionState(); len(got.Page.Results) != 1 {
		t.Fatalf("expected search page state present before reload, got %#v", got)
	}

	gateway.ResetCalls()
	cmd = m.search.AutoRefresh()
	if cmd == nil {
		t.Fatal("expected search auto-refresh command")
	}
	if session := m.search.SessionState(); !session.Loading || !session.Reloading {
		t.Fatalf("expected search session to mark reload in flight, got %#v", session)
	}
	gateway.SetError(fakes.MethodSearchIssues, errors.New("refresh boom"))

	m = applyMessages(t, m, runBatch(cmd))
	m.renderBody()
	if got := m.search.SessionState(); got.Error != "refresh boom" || len(got.Page.Results) != 1 {
		t.Fatalf("expected last search page retained after refresh failure, got %#v", got)
	}
	if !strings.Contains(m.View(), "cached detail") {
		t.Fatalf("expected cached preview detail retained after refresh failure, got:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "refresh boom") || !strings.Contains(m.View(), "Search result") || !strings.Contains(m.View(), "failed") || !strings.Contains(m.View(), "x") {
		t.Fatalf("expected shell view to preserve search context on refresh failure, got:\n%s", m.View())
	}
}

func TestModelRefreshTickInSearchSkipsAutoRefreshWhileUserTyping(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Search {
		t.Fatalf("expected search active before typing suppression test, got %s", m.active)
	}

	gateway.ResetCalls()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	if cmd != nil {
		t.Fatalf("expected query typing not to issue search command until enter")
	}
	if !m.search.CapturesShellKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}) {
		t.Fatalf("expected search query to be focused for typing suppression case")
	}

	next, tickCmd := m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(tickCmd))

	if len(gateway.Calls) != 0 {
		t.Fatalf("expected no gateway calls before queued typing command resolves, got %#v", gateway.Calls)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if len(gateway.Calls) != 1 || gateway.Calls[0].Method != fakes.MethodSearchIssues {
		t.Fatalf("expected only one enter-triggered search call while auto-refresh is suppressed, got %#v", gateway.Calls)
	}
	if m.search.IsLoading() {
		t.Fatalf("expected typing-triggered search to settle")
	}
}

func TestModelRefreshTickSkipsWhileModalsOpenAndDetailLoading(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	gateway.ResetCalls()
	m.showHelp = true
	next, cmd := m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if len(gateway.Calls) != 0 {
		t.Fatalf("expected no auto-refresh while help modal is open, calls=%#v", gateway.Calls)
	}

	m.showHelp = false
	m.showActionModal = true
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if len(gateway.Calls) != 0 {
		t.Fatalf("expected no auto-refresh while action modal is open, calls=%#v", gateway.Calls)
	}

	m.showActionModal = false
	m.active = mode.Detail
	m.detail.Loading = true
	m.detail.TargetID = firstSelectionID(m, mode.Board)
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("expected duplicate detail reload suppression while loading, calls=%#v", gateway.Calls)
	}
}

func TestModelMutationResultMarksBrowseDirtyAndRefreshesOnlyActiveSurface(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	gateway.ResetCalls()
	next, cmd := m.Update(mutationResultMsg{kind: mutationStatus, issueID: "bw-1"})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gateway.HasCall(string(fakes.MethodReadyExplain)) || !gateway.HasCall(string(fakes.MethodQuery)) {
		t.Fatalf("expected board to refresh immediately when active and dirty after write, calls=%#v", gateway.Calls)
	}
	if gateway.HasCall(string(fakes.MethodSearchIssues)) {
		t.Fatalf("expected hidden search surface not to refresh from board-active write, calls=%#v", gateway.Calls)
	}
	if !gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("expected write flow to keep immediate detail reload, calls=%#v", gateway.Calls)
	}

	if state := m.refreshStateBySurface[mode.Board]; state.dirty {
		t.Fatalf("expected active board dirty flag to clear after refresh")
	}
	if state := m.refreshStateBySurface[mode.Search]; !state.dirty {
		t.Fatalf("expected inactive search to remain dirty until next eligible refresh")
	}

	gateway.ResetCalls()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected active mode search after toggle, got %s", m.active)
	}
	if !gateway.HasCall(string(fakes.MethodSearchIssues)) {
		t.Fatalf("expected dirty search to refresh on activation, calls=%#v", gateway.Calls)
	}
	if gateway.HasCall(string(fakes.MethodReadyExplain)) || gateway.HasCall(string(fakes.MethodQuery)) {
		t.Fatalf("expected only newly active search to refresh on activation, calls=%#v", gateway.Calls)
	}
	if state := m.refreshStateBySurface[mode.Search]; state.dirty {
		t.Fatalf("expected search dirty flag to clear after activation refresh")
	}
}

func TestModelRefreshTickHonorsStaleCadenceForActiveSurface(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })
	withModelNow(t, time.Unix(0, 0))

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m.markSurfaceRefreshed(mode.Board)

	gateway.ResetCalls()
	withModelNow(t, time.Unix(59, 0))
	next, cmd := m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if gateway.HasCall(string(fakes.MethodReadyExplain)) || gateway.HasCall(string(fakes.MethodQuery)) {
		t.Fatalf("expected no board refresh before stale interval elapses, calls=%#v", gateway.Calls)
	}

	gateway.ResetCalls()
	withModelNow(t, time.Unix(60, 0))
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gateway.HasCall(string(fakes.MethodReadyExplain)) || !gateway.HasCall(string(fakes.MethodQuery)) {
		t.Fatalf("expected board refresh at ~60s stale threshold, calls=%#v", gateway.Calls)
	}
}

func TestModelWithNoAutoRefreshSkipsTickSchedulingInInit(t *testing.T) {
	refreshMarkerSeen := false
	withRefreshTickScheduler(t, func() tea.Cmd {
		return func() tea.Msg {
			return refreshTickMsg{}
		}
	})

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModelWithOptions(t, services, RuntimeOptions{DisableAutoRefresh: true})
	for _, msg := range runBatch(m.Init()) {
		if _, ok := msg.(refreshTickMsg); ok {
			refreshMarkerSeen = true
			break
		}
	}

	if refreshMarkerSeen {
		t.Fatalf("expected no periodic tick scheduling when auto-refresh disabled")
	}
}

func TestModelWithNoAutoRefreshSuppressesFocusAndTickButKeepsManualBoardReload(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModelWithOptions(t, services, RuntimeOptions{DisableAutoRefresh: true})
	m = applyMessages(t, m, runBatch(m.Init()))

	gateway.ResetCalls()
	next, cmd := m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.BlurMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(gateway.Calls) != 0 {
		t.Fatalf("expected no auto-refresh side effects from focus/tick when disabled, calls=%#v", gateway.Calls)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(gateway.Calls) == 0 {
		t.Fatalf("expected manual reload to remain functional when auto-refresh disabled")
	}
	if !gateway.HasCall(string(fakes.MethodReadyExplain)) {
		t.Fatalf("expected manual reload to include board data refresh, calls=%#v", gateway.Calls)
	}
}

func TestModelRefreshInDetailDoesNotBackgroundPollInactiveBrowseSurfaces(t *testing.T) {
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected detail active for polling-scope assertion, got %s", m.active)
	}

	m.markBrowseSurfacesDirty()
	m.markSurfaceDirty(mode.Detail)
	gateway.ResetCalls()
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gateway.HasCall(string(fakes.MethodShowIssue)) {
		t.Fatalf("expected active detail to refresh when eligible, calls=%#v", gateway.Calls)
	}
	if gateway.HasCall(string(fakes.MethodReadyExplain)) || gateway.HasCall(string(fakes.MethodQuery)) || gateway.HasCall(string(fakes.MethodSearchIssues)) {
		t.Fatalf("expected no background refresh of inactive board/search surfaces, calls=%#v", gateway.Calls)
	}
}

func TestModelDefaultTabAndShiftTabDoNotCycleModes(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	cfg := config.Default()
	cfg.UI.ShowModeSwitcherHelp = false

	services, err := NewServices(gateway, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if footer := m.renderFooter(); strings.TrimSpace(footer) != "" {
		t.Fatalf("expected footer help hidden when ShowModeSwitcherHelp is false, got:\n%s", footer)
	}
}

func TestModelUsesConfiguredShellAndBoardKeyBindings(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
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

	m := mustNewModel(t, services)
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

func TestModelDetailViewShowsConfiguredCommentQuickActionLabel(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}

	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}, Description: "detail"}

	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{
			config.ShellActionCommentIssue: {"ctrl+a"},
		},
	})

	services, err := NewServices(gateway, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	view := m.View()
	if !strings.Contains(view, "ctrl+a Add comment") {
		t.Fatalf("expected detail quick actions to reflect configured comment binding, got:\n%s", view)
	}
	if strings.Contains(view, "c Add comment") {
		t.Fatalf("expected stale default add-comment label to be absent, got:\n%s", view)
	}
}

func TestModelEditHotkeyUsesEditorService(t *testing.T) {
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Assignee: "hans", Labels: []string{"infra"}, Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}

	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	services.Editor = fakeEditor

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{Err: errors.New("editor boom")}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	services.Editor = fakeEditor

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.StatusCatalogResponse = []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}}
	gateway.TypeCatalogResponse = []domain.TypeOption{{Name: "task"}, {Name: "bug"}}
	gateway.LabelCatalogResponse = []domain.LabelOption{{Name: "ui"}, {Name: "infra"}}
	gateway.CreateIssueResponse = domain.CreateIssueResult{IssueID: "bw-99"}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1, Labels: []string{"ui"}}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1, Labels: []string{"ui"}}}
	gateway.StatusCatalogResponse = []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}}
	gateway.TypeCatalogResponse = []domain.TypeOption{{Name: "task"}, {Name: "bug"}}
	gateway.LabelCatalogResponse = []domain.LabelOption{{Name: "ui"}, {Name: "infra"}}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1, Labels: []string{"ui"}}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}

	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: strings.Join(longLines, "\n"),
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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

func TestModelDetailModeLeftBrowserUpDownPreviewsIssueWithoutChangingAnchor(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-9", Title: "Other", Status: "in_progress", Type: "task", Priority: 2}}
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

	m := mustNewModel(t, services)
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
	if cmd == nil {
		t.Fatalf("expected down on left browser panel to trigger preview load command")
	}
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

	if m.active != mode.Detail {
		t.Fatalf("expected app to remain in detail mode after browse preview, got %s", m.active)
	}
	if m.detail.TargetID != "bw-5" {
		t.Fatalf("expected browser preview target bw-5, got %q", m.detail.TargetID)
	}
	if m.detail.SelectionID != "bw-1" {
		t.Fatalf("expected anchored selection to remain bw-1 while previewing, got %q", m.detail.SelectionID)
	}
	if got := firstSelectionID(m, mode.Board); got != "bw-1" {
		t.Fatalf("expected board selection to stay anchored on bw-1, got %q", got)
	}

	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatalf("expected enter on left browser panel to be no-op, got command")
	}

	if m.active != mode.Detail {
		t.Fatalf("expected app to remain in detail mode after browse no-op enter, got %s", m.active)
	}
	if m.detail.TargetID != "bw-5" || m.detail.SelectionID != "bw-1" {
		t.Fatalf("expected preview target bw-5 with anchored selection bw-1, got target=%q selection=%q", m.detail.TargetID, m.detail.SelectionID)
	}
	if m.detail.ScrollOffset != 0 {
		t.Fatalf("expected browser preview to reset content scroll offset, got %d", m.detail.ScrollOffset)
	}

	if m.detail.Detail.Summary.ID != "bw-1" {
		t.Fatalf("expected anchored detail to remain bw-1, got %q", m.detail.Detail.Summary.ID)
	}
	if m.detail.PreviewDetail.Summary.ID != "bw-5" {
		t.Fatalf("expected loaded browser preview detail bw-5, got %q", m.detail.PreviewDetail.Summary.ID)
	}
	if len(m.detail.BrowserItems) != 3 {
		t.Fatalf("expected stable parent-group browser items (parent + siblings) during preview, got %d", len(m.detail.BrowserItems))
	}
}

func TestModelDetailModeDependenciesWithoutParentGroupUpDownPreviewsSelectedIssue(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-9", Title: "Other", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1},
		BlockedBy: []domain.IssueReference{
			{ID: "bw-3", Title: "Blocker"},
		},
		Blocks: []domain.IssueReference{
			{ID: "bw-5", Title: "Downstream"},
		},
		Related: []domain.IssueReference{
			{ID: "bw-4", Title: "Related"},
		},
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 160
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(m.detail.BrowserItems) != 3 {
		t.Fatalf("expected dependencies to populate browser items without parent-group, got %d", len(m.detail.BrowserItems))
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected down on dependencies pane to trigger preview load command")
	}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-5", Title: "Downstream", Status: "in_progress", Type: "task", Priority: 2},
	}
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-4", Title: "Related", Status: "in_progress", Type: "bug", Priority: 2},
	}
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("expected enter on dependencies pane to be no-op")
	}

	if m.active != mode.Detail {
		t.Fatalf("expected app to remain in detail mode after dependency preview, got %s", m.active)
	}
	if m.detail.TargetID != "bw-4" || m.detail.SelectionID != "bw-1" {
		t.Fatalf("expected selected dependency bw-4 preview target with anchored selection bw-1, got target=%q selection=%q", m.detail.TargetID, m.detail.SelectionID)
	}

	if m.detail.Detail.Summary.ID != "bw-1" {
		t.Fatalf("expected anchored detail to remain bw-1, got %q", m.detail.Detail.Summary.ID)
	}
	if m.detail.PreviewDetail.Summary.ID != "bw-4" {
		t.Fatalf("expected loaded dependency issue preview bw-4, got %q", m.detail.PreviewDetail.Summary.ID)
	}
}

func TestModelDetailMetadataEnterOpensStatusDialogAndSubmitsStatusUpdate(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "Other", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1},
	}
	gateway.StatusCatalogResponse = []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}, {Name: "blocked"}}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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

func TestModelDetailMetadataStatusDialogEscapeCancelsWithoutSaving(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "Other", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}
	gateway.StatusCatalogResponse = []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}, {Name: "blocked"}}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		m = applyMessages(t, m, runBatch(cmd))
	}

	if m.showActionModal {
		t.Fatal("expected escape to close status action modal")
	}

	for _, call := range gateway.Calls {
		if call.Method == fakes.MethodUpdateIssue {
			t.Fatalf("expected no UpdateIssue call on escape cancel, calls=%#v", gateway.Calls)
		}
	}
}

func TestModelDetailMetadataStatusDialogEnterUnchangedIsNoOp(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "Other", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}
	gateway.StatusCatalogResponse = []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}, {Name: "blocked"}}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	if !m.showActionModal {
		t.Fatal("expected status action modal to open")
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected enter on unchanged status to submit no-op mutation")
	}

	next, cmd = m.Update(cmd())
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected no-op mutation command after submit")
	}

	next, _ = m.Update(cmd())
	m = next.(Model)

	if m.showActionModal {
		t.Fatal("expected status action modal to close after enter no-op")
	}

	for _, call := range gateway.Calls {
		if call.Method == fakes.MethodUpdateIssue {
			t.Fatalf("expected no UpdateIssue call on unchanged enter no-op, calls=%#v", gateway.Calls)
		}
	}

	if !m.toast.Visible() {
		t.Fatal("expected no-change toast to be visible after unchanged enter")
	}
}

func TestModelDetailMetadataEnterOnPriorityOpensDialogAndSubmitsPriorityUpdate(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "Other", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 4},
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 140
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected priority dialog init command after enter on metadata priority")
	}
	_ = cmd

	if !m.showActionModal {
		t.Fatal("expected priority action modal to open")
	}

	next, cmd = m.Update(modal.SubmitMsg{Values: map[string]string{"priority": "0"}})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected priority update submit command")
	}

	next, _ = m.Update(cmd())
	m = next.(Model)

	if !gateway.HasCall(string(fakes.MethodUpdateIssue)) {
		t.Fatalf("expected priority cycle update issue call, calls=%#v", gateway.Calls)
	}

	foundPriorityUpdate := false
	for _, call := range gateway.Calls {
		if call.Method != fakes.MethodUpdateIssue {
			continue
		}
		updateCall, ok := call.Input.(fakes.UpdateIssueCall)
		if !ok {
			continue
		}
		if updateCall.Input.Status != nil {
			t.Fatalf("expected priority-only update, got status update %#v", *updateCall.Input.Status)
		}
		if updateCall.Input.Priority == nil {
			t.Fatalf("expected priority update, got %#v", updateCall.Input)
		}
		if *updateCall.Input.Priority != 0 {
			t.Fatalf("expected submitted priority 0, got P%d", *updateCall.Input.Priority)
		}
		foundPriorityUpdate = true
	}
	if !foundPriorityUpdate {
		t.Fatal("expected to capture update issue input for priority dialog edit")
	}
}

func TestModelDetailMetadataPriorityDialogEscapeCancelsWithoutSaving(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "Other", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{Summary: domain.IssueSummary{ID: "bw-1", Title: "Root", Status: "open", Type: "task", Priority: 3}}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 140
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected priority dialog init command")
	}
	_ = cmd

	if !m.showActionModal {
		t.Fatal("expected priority action modal to open")
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		m = applyMessages(t, m, runBatch(cmd))
	}

	if m.showActionModal {
		t.Fatal("expected escape to close priority action modal")
	}

	for _, call := range gateway.Calls {
		if call.Method == fakes.MethodUpdateIssue {
			t.Fatalf("expected no UpdateIssue call on priority escape cancel, calls=%#v", gateway.Calls)
		}
	}
}

func TestModelLauncherSuccessToastClarifiesBackgroundLifecycle(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}

	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "Ninth detail",
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready:   []domain.IssueSummary{{ID: "beads-workbench-yze.4.2", Title: "Implement create update close and comment actions in the app", Status: "open", Type: "task", Priority: 1}},
		Blocked: []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "beads-workbench-yze.4.5", Title: "Add editor and launcher integration tests", Status: "blocked", Type: "task", Priority: 1}}},
	}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "beads-workbench-yze.4.3", Title: "Implement launcher framework with issue-context interpolation", Status: "in_progress", Type: "task", Priority: 1}}
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

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready:   []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}},
		Blocked: []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked", Status: "blocked", Type: "bug", Priority: 0}}},
	}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
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

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}}
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

	m := mustNewModel(t, services)
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

	m := mustNewModel(t, services)
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

	m := mustNewModel(t, services)
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

func TestModelEmbeddedFixtureMutationModalsOpenWithoutCatalogDecodeToast(t *testing.T) {
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

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	assertModalOpenWithoutCatalogToast := func(model Model, wantTitle string) {
		t.Helper()
		if !model.showActionModal {
			t.Fatalf("expected action modal %q to open", wantTitle)
		}
		if !strings.Contains(model.actionModal.View(), wantTitle) {
			t.Fatalf("expected modal title %q, got:\n%s", wantTitle, model.actionModal.View())
		}
		if model.toast.Visible() {
			t.Fatalf("expected no toast while opening %q modal, got:\n%s", wantTitle, model.View())
		}
		if strings.Contains(model.View(), "Failed to load mutation catalogs") {
			t.Fatalf("expected no mutation catalog decode toast while opening %q modal, got:\n%s", wantTitle, model.View())
		}
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected create flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Create Issue")

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	if m.showActionModal {
		t.Fatal("expected create modal to close on escape")
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected update flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Update Issue bwf-2")

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	if m.showActionModal {
		t.Fatal("expected update modal to close on escape")
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected detail mode before comment flow, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected comment flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Comment on bwf-2")
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

	m := mustNewModel(t, services)
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

	m := mustNewModel(t, services)
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

	m := mustNewModel(t, services)
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
	if !strings.Contains(view, "bwf-3") {
		t.Fatalf("expected related issue id in detail view, got:\n%s", view)
	}
}

func TestModelEmbeddedFixtureDetailShowsRelatesToDependentOnlyUnderRelated(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	if err := runBDInRepo(repoPath, "dep", "relate", "bwf-3", "bwf-2"); err != nil {
		t.Fatalf("failed to create real relates-to dependency: %v", err)
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

	m := mustNewModel(t, services)
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
	if !strings.Contains(view, "Blocks (0)") {
		t.Fatalf("expected relates-to dependent not to appear under blocks, got:\n%s", view)
	}
	if !strings.Contains(view, "Related (1)") {
		t.Fatalf("expected exactly one related entry from relates-to dependent, got:\n%s", view)
	}
	if strings.Count(view, "bwf-3") != 1 {
		t.Fatalf("expected relates-to-linked issue bwf-3 to render once (under Related only), got:\n%s", view)
	}
}

func TestModelBoardDetailBoardRoundTripPreservesLayoutAndFocus(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready:   []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}},
		Blocked: []domain.BlockedIssueView{{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked now", Status: "blocked", Type: "bug", Priority: 0}}},
	}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress one", Status: "in_progress", Type: "task", Priority: 1}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{}
	gateway.ShowIssueResponse = domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bw-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: "detail for ready issue",
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
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
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Status: "open", Priority: 1}}}
	gateway.QueryResponse = []domain.IssueSummary{{ID: "bw-2", Title: "In progress one", Status: "in_progress", Priority: 2}}
	gateway.SearchIssuesResponse = domain.SearchResultPage{Results: []domain.SearchResult{{Issue: domain.IssueSummary{ID: "bw-2", Title: "In progress one", Status: "in_progress", Priority: 2}}}}
	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m.width = 120
	m.height = 34

	expectedWidth, expectedHeight := m.workspaceSize()

	m.active = mode.Board
	boardBody := m.renderBody()
	if strings.TrimSpace(boardBody) == "" {
		t.Fatal("expected non-empty board body rendering")
	}

	m.active = mode.Search
	body := m.renderBody()
	if !strings.Contains(body, "Search") {
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

func TestModelStartupHealthCheckSetsFatalErrOnCommandUnavailable(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.SetError(fakes.MethodHealthCheck, domain.GatewayError{
		Code:      domain.ErrorCodeCommandUnavailable,
		Operation: "health check",
		Message:   "bd command is unavailable",
	})

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if m.fatalErrTitle == "" {
		t.Fatal("expected fatalErr to be set after CommandUnavailable health check, got empty string")
	}
}

func TestModelStartupHealthCheckClearsPathOnSuccess(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if m.fatalErrTitle != "" {
		t.Fatalf("expected fatalErr to be empty after successful health check, got %q", m.fatalErrTitle)
	}
}

func TestModelFatalErrViewRendersFatalErrorScreen(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.SetError(fakes.MethodHealthCheck, domain.GatewayError{
		Code:    domain.ErrorCodeCommandUnavailable,
		Message: "bd command is unavailable",
	})

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	view := m.View()
	if !strings.Contains(view, "beads is not available") {
		t.Fatalf("expected fatal error title in View(), got %q", view)
	}
	if !strings.Contains(view, "bd") {
		t.Fatalf("expected 'bd' mention in View(), got %q", view)
	}
}

func TestModelFatalErrUpdateOnlyHandlesQuitAndResize(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.SetError(fakes.MethodHealthCheck, domain.GatewayError{
		Code:    domain.ErrorCodeCommandUnavailable,
		Message: "bd command is unavailable",
	})

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if m.fatalErrTitle == "" {
		t.Fatal("precondition: expected fatalErr to be set")
	}

	// Window resize should update dimensions.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	if m.width != 120 || m.height != 40 {
		t.Fatalf("expected width=120 height=40 after resize, got %d %d", m.width, m.height)
	}

	// Quit key should return tea.Quit.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd from 'q' key when fatalErr is set, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}

	// Arbitrary key should be swallowed (no cmd).
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd != nil {
		t.Fatalf("expected nil cmd for non-quit key when fatalErr is set, got non-nil")
	}
}

func TestModelStartupHealthCheckSetsFatalErrOnNoDatabaseFound(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.SetError(fakes.MethodHealthCheck, domain.GatewayError{
		Code:      domain.ErrorCodeNoDatabaseFound,
		Operation: "health check",
		Message:   "no beads database found",
	})

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if m.fatalErrTitle == "" {
		t.Fatal("expected fatalErrTitle to be set after NoDatabaseFound health check")
	}
	view := m.View()
	if !strings.Contains(view, "no beads project here") {
		t.Fatalf("expected no-database title in View(), got %q", view)
	}
	if !strings.Contains(view, "bd init") {
		t.Fatalf("expected 'bd init' hint in View(), got %q", view)
	}
}

func TestModelFatalErrIgnoresNonGatewayError(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	gateway.SetError(fakes.MethodHealthCheck, errors.New("some plain error"))

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	// A non-GatewayError does not set fatalErr — app loads normally.
	if m.fatalErrTitle != "" {
		t.Fatalf("expected fatalErr to be empty for non-GatewayError, got %q", m.fatalErrTitle)
	}
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

func withRefreshTickScheduler(t *testing.T, scheduler func() tea.Cmd) {
	t.Helper()
	original := scheduleRefreshTickCmd
	scheduleRefreshTickCmd = scheduler
	t.Cleanup(func() {
		scheduleRefreshTickCmd = original
	})
}

func withModelNow(t *testing.T, now time.Time) {
	t.Helper()
	original := modelNow
	modelNow = func() time.Time { return now }
	t.Cleanup(func() {
		modelNow = original
	})
}

// TestNewModelWithOptionsReturnsErrorOnInvalidKeyBindings asserts that
// NewModelWithOptions returns a typed error (not a panic) when Config contains
// an invalid keybinding — defensive hardening for direct-construction callers
// (tests, future programmatic embed) that skip config.Load.
func TestNewModelWithOptionsReturnsErrorOnInvalidKeyBindings(t *testing.T) {
	t.Parallel()

	gateway := fakes.NewFakeBeadsGateway()
	cfg := config.Default()
	// Inject an invalid keybinding: empty key slice for a required action.
	cfg.KeyBindings.Shell[config.ShellActionQuit] = []string{}

	services := Services{
		Gateway: gateway,
		Config:  cfg,
	}

	_, err := NewModelWithOptions(services, RuntimeOptions{})
	if err == nil {
		t.Fatal("expected NewModelWithOptions to return an error for invalid keybindings, got nil")
	}
}

// TestModeCycleDirections asserts that nextMode and prevMode traverse the mode
// cycle in opposite directions for every starting mode.
//
// Forward cycle  (nextMode): Board → Search → Board (2-mode browse toggle),
//
//	Detail → the browse mode not in lastBrowse
//
// Backward cycle (prevMode): Board → Detail → Search → Board
// (prevMode ignores lastBrowse; the cycle is fixed).
//
// Together the two functions must differ at the modes where direction matters:
// Board (next→Search vs prev→Detail) and Detail (next→browse vs prev→Search).
func TestModeCycleDirections(t *testing.T) {
	t.Parallel()

	t.Run("nextMode_forward", func(t *testing.T) {
		t.Parallel()

		if got := nextMode(mode.Board, mode.Board); got != mode.Search {
			t.Errorf("nextMode(Board, Board) = %s; want Search", got)
		}
		if got := nextMode(mode.Search, mode.Board); got != mode.Board {
			t.Errorf("nextMode(Search, Board) = %s; want Board", got)
		}
		// Detail goes to the browse mode not in lastBrowse.
		if got := nextMode(mode.Detail, mode.Search); got != mode.Board {
			t.Errorf("nextMode(Detail, Search) = %s; want Board", got)
		}
		if got := nextMode(mode.Detail, mode.Board); got != mode.Search {
			t.Errorf("nextMode(Detail, Board) = %s; want Search", got)
		}
	})

	t.Run("prevMode_backward", func(t *testing.T) {
		t.Parallel()

		// Backward cycle: Board → Detail → Search → Board
		if got := prevMode(mode.Board, mode.Board); got != mode.Detail {
			t.Errorf("prevMode(Board, _) = %s; want Detail", got)
		}
		if got := prevMode(mode.Detail, mode.Board); got != mode.Search {
			t.Errorf("prevMode(Detail, _) = %s; want Search", got)
		}
		if got := prevMode(mode.Search, mode.Board); got != mode.Board {
			t.Errorf("prevMode(Search, _) = %s; want Board", got)
		}
	})

	t.Run("next_and_prev_differ_at_board_and_detail", func(t *testing.T) {
		t.Parallel()

		// Board: next→Search, prev→Detail — must differ.
		if nextMode(mode.Board, mode.Board) == prevMode(mode.Board, mode.Board) {
			t.Errorf("nextMode(Board) and prevMode(Board) both returned %s; they must differ",
				nextMode(mode.Board, mode.Board))
		}
		// Detail: next→browse (Search when lb=Board), prev→Search — both happen
		// to return Search when lb=Board, which is expected; the distinguishing
		// arm is lb=Search where next→Board but prev→Search.
		if nextMode(mode.Detail, mode.Search) == prevMode(mode.Detail, mode.Search) {
			t.Errorf("nextMode(Detail,Search) and prevMode(Detail,Search) both returned %s; they must differ",
				nextMode(mode.Detail, mode.Search))
		}
	})
}
