package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
	"github.com/hk9890/task-manager-ui/internal/mode"
	detailsmode "github.com/hk9890/task-manager-ui/internal/mode/details"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	"github.com/hk9890/task-manager-ui/internal/testing/ui"
	uidetails "github.com/hk9890/task-manager-ui/internal/ui/details"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
)

// mustNewModel wraps NewModel and fails the test if an error is returned.
// It pre-sets sizeKnown=true and installs no-op scheduler functions so that
// tests run without real time-based ticks and without any global shared state.
// Tests that specifically validate the sizeKnown=false/empty-view behaviour
// should call NewModelWithOptions directly and leave sizeKnown at its zero value.
func mustNewModel(t *testing.T, services Services) Model {
	t.Helper()
	m, err := NewModel(services)
	if err != nil {
		t.Fatalf("NewModel returned unexpected error: %v", err)
	}
	m.sizeKnown = true
	m.scheduleRefreshTick = func() tea.Cmd { return nil }
	m.scheduleToastDismiss = func(_ time.Duration, _ int) tea.Cmd { return nil }
	m.scheduleSpinnerTick = func() tea.Cmd { return nil }
	return m
}

// mustNewModelWithOptions wraps NewModelWithOptions and fails the test if an error is returned.
// It pre-sets sizeKnown=true and installs no-op scheduler functions (same as
// mustNewModel). Tests that specifically validate the sizeKnown=false/empty-view
// behaviour should call NewModelWithOptions directly and leave sizeKnown at its
// zero value.
func mustNewModelWithOptions(t *testing.T, services Services, runtime RuntimeOptions) Model {
	t.Helper()
	m, err := NewModelWithOptions(services, runtime)
	if err != nil {
		t.Fatalf("NewModelWithOptions returned unexpected error: %v", err)
	}
	m.sizeKnown = true
	m.scheduleRefreshTick = func() tea.Cmd { return nil }
	m.scheduleToastDismiss = func(_ time.Duration, _ int) tea.Cmd { return nil }
	m.scheduleSpinnerTick = func() tea.Cmd { return nil }
	return m
}

func TestModelInitUsesBoardControllerAndBuiltInDashboardQueries(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	// tm-3 has status="blocked" → goes into DashboardData.Blocked → NotReady column.
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-3", Title: "Blocked", Status: "blocked", Priority: 1})
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	if got := firstSelectionID(m, mode.Board); got != "tm-3" {
		t.Fatalf("expected board selection from board controller, got %q", got)
	}

	if !gw.hasDashboardCall() {
		t.Fatalf("expected Dashboard call from board controller")
	}

	if m.renderBody() == "" {
		t.Fatalf("expected board body rendering from board controller")
	}
}

// TestModelInitDoesNotPreloadSearch asserts that app.Model.Init fires no
// SearchIssues call.  Search init is deferred until the user first activates
// search mode (ticket t8kp).
func TestModelInitDoesNotPreloadSearch(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if gw.hasSearchCall() {
		t.Fatalf("expected no Search call during startup; got calls=%#v", gw.Calls())
	}
}

// TestModelFirstSearchModeSwitchTriggersSearchInit asserts that the first
// transition to search mode fires exactly one SearchIssues call (lazy init),
// and that a second transition does NOT fire another SearchIssues call.
func TestModelFirstSearchModeSwitchTriggersSearchInit(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Startup must not have triggered search.
	if gw.hasSearchCall() {
		t.Fatalf("expected no Search call during startup; got calls=%#v", gw.Calls())
	}
	if m.searchInitDone {
		t.Fatalf("expected searchInitDone=false after startup")
	}

	// First switch to search mode: lazy init must fire.
	mark := gw.resetMark()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected search active after toggle, got %s", m.active)
	}
	if !gw.hasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected Search call on first search mode activation; got calls=%#v", gw.Calls())
	}
	if !m.searchInitDone {
		t.Fatalf("expected searchInitDone=true after first search activation")
	}

	// Return to board and go back to search: should NOT re-trigger Search
	// from the lazy init path (auto-refresh may run if stale, but lazy init does not).
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt}) // toggle back to board
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	mark = gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt}) // toggle to search again
	m = next.(Model)
	// Only run the immediate Update result; don't recurse into auto-refresh
	// commands — we only want to check that lazySearchInitCmd itself is a no-op.
	_ = cmd
	if !m.searchInitDone {
		t.Fatalf("expected searchInitDone still true on second search activation")
	}
	// The lazy init flag must be set; subsequent refresh is handled by auto-refresh,
	// not by Init. Confirm no second Search call came from the lazy path.
	// (Auto-refresh may or may not fire depending on stale cadence; we apply no
	// messages to avoid triggering it.)
	if gw.hasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected lazy init NOT to re-fire Search on second search activation; got calls=%#v", gw.Calls())
	}
}

func TestModelStartupSynchronizesSelectionAfterBoardInitSelectionMessage(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "startup detail"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
				if strings.Contains(header, "Selected: tm-1 (open)") {
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
	if !strings.Contains(header, "Selected: tm-1 (open)") {
		t.Fatalf("expected startup header to show active board selection after init messages, got:\n%s", header)
	}
}

func TestModelBoardNavigationUpdatesShellSelectionAndDetailState(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress one", "task", 2)
	gw.seedInProgress("tm-4", "In progress two", "task", 1)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-4", Title: "In progress two", Status: "in_progress", Priority: 1}, Description: "detail for tm-4"})
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-2", Title: "In progress one", Status: "in_progress", Priority: 2}, Description: "detail for tm-2"})
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	if got := firstSelectionID(m, mode.Board); got != "tm-1" {
		t.Fatalf("expected initial board selection tm-1, got %q", got)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving board column")
	}
	// After moving right: InProgress column sorted by priority: [tm-4(P1), tm-2(P2)].
	// First item selected is tm-4 (highest priority).
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "tm-4" {
		t.Fatalf("expected board selection tm-4 after moving right, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "tm-4" {
		t.Fatalf("expected shell detail state to load tm-4, got %q", m.detail.Detail.Summary.ID)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving board row")
	}
	m = applyMessages(t, m, runBatch(cmd))
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("expected board selection tm-2 after moving down, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "tm-2" {
		t.Fatalf("expected shell detail state to update to tm-2, got %q", m.detail.Detail.Summary.ID)
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
		next, _ = m.Update(cmd())
		m = next.(Model)
	}

	if m.detail.TargetID != "tm-2" {
		t.Fatalf("expected detail target to track board selection, got %q", m.detail.TargetID)
	}
}

func TestModelSearchTextEntryIsNotHijackedByShellHotkeys(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	mark := gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected active mode to stay search while typing, got %s", m.active)
	}
	if gw.callCountSince(mark, fakes.MethodSearch) != 0 {
		t.Fatalf("expected typing in search query not to run search until enter, got %#v", gw.Calls())
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if got := m.search.SessionState().AppliedQuery; got != "b" {
		t.Fatalf("expected applied search query %q, got %q", "b", got)
	}
}

func TestModelSearchModeRendersRepresentativeErrorAndEmptyStates(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Enter search mode.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// Trigger a repository-backed search error.
	gw.SetError(fakes.MethodSearch, errors.New("search boom"))
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
	gw.SetError(fakes.MethodSearch, nil)
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
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// tm-3 seeded so empty-query search returns it in the results panel.
	gw.seedReady("tm-3", "Search result", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
	withModelNow(t, time.Unix(0, 0))

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	if m.focusKnown {
		t.Fatal("expected no focus events observed at startup")
	}

	withModelNow(t, time.Unix(61, 0))
	mark := gw.resetMark()
	next, cmd := m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gw.hasCallSince(mark, fakes.MethodDashboard) {
		t.Fatalf("expected board refresh from tick fallback without focus events, calls=%#v", gw.Calls())
	}
}

func TestModelFocusRegainRefreshesOnceAndSkipsRepeatedFocus(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	mark := gw.resetMark()
	next, cmd := m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.hasCallSince(mark, fakes.MethodDashboard) {
		t.Fatalf("expected initial focus event not to force refresh, calls=%#v", gw.Calls())
	}

	next, cmd = m.Update(tea.BlurMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	m.markSurfaceRefreshed(mode.Board)
	mark = gw.resetMark()
	next, cmd = m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if !gw.hasCallSince(mark, fakes.MethodDashboard) {
		t.Fatalf("expected focus regain to refresh active board, calls=%#v", gw.Calls())
	}

	mark = gw.resetMark()
	next, cmd = m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.hasCallSince(mark, fakes.MethodDashboard) {
		t.Fatalf("expected repeated focus while focused to avoid refresh spam, calls=%#v", gw.Calls())
	}
}

func TestModelFocusRegainInDetailRefreshesImmediatelyWithoutStaleOrDirty(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
	mark := gw.resetMark()

	next, cmd = m.Update(tea.BlurMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected focus regain to refresh active detail immediately, calls=%#v", gw.Calls())
	}
	if gw.hasCallSince(mark, fakes.MethodDashboard) || gw.hasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected focus regain in detail to refresh only active detail surface, calls=%#v", gw.Calls())
	}
}

func TestModelRefreshTickReloadsOnlyActiveSearchSurface(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
	mark := gw.resetMark()
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gw.hasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected search surface refresh on tick when search is active, calls=%#v", gw.Calls())
	}
	if gw.hasCallSince(mark, fakes.MethodDashboard) || gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected tick refresh to target only active search surface, calls=%#v", gw.Calls())
	}
}

func TestModelRefreshTickBoardAutoRefreshDoesNotSwitchModeOrClearDetailState(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-3", Title: "Blocked", Status: "blocked", Priority: 0})
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	if m.active != mode.Board {
		t.Fatalf("expected board active after init, got %s", m.active)
	}

	m.detail.SelectionID = "tm-3"
	m.detail.TargetID = "tm-3"
	m.detail.Detail = domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-3", Title: "Blocked", Status: "blocked"}, Description: "cached detail"}
	m.detail.Error = ""
	m.detail.Loading = false

	mark := gw.resetMark()
	next, cmd := m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Board {
		t.Fatalf("expected board auto-refresh not to force mode switch, got %s", m.active)
	}
	if m.detail.Detail.Summary.ID != "tm-3" || m.detail.Detail.Description != "cached detail" {
		t.Fatalf("expected board auto-refresh not to clear shell detail cache, got %#v", m.detail.Detail)
	}
	if gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected board auto-refresh not to force detail reload when selection remains, calls=%#v", gw.Calls())
	}
}

func TestModelRefreshTickSearchAutoRefreshDoesNotSwitchModeOrClearDetailState(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// tm-9 appears in search results (empty-query matches all).
	gw.seedReady("tm-9", "Search result", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	m.detail.SelectionID = "tm-9"
	m.detail.TargetID = "tm-9"
	m.detail.Detail = domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-9", Title: "Search result", Status: "open"}, Description: "cached detail"}
	m.detail.Error = ""
	m.detail.Loading = false

	mark := gw.resetMark()
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected search auto-refresh not to force mode switch, got %s", m.active)
	}
	if m.detail.Detail.Summary.ID != "tm-9" || m.detail.Detail.Description != "cached detail" {
		t.Fatalf("expected search auto-refresh not to clear shell detail cache, got %#v", m.detail.Detail)
	}
	if gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected search auto-refresh not to force detail reload when selection remains, calls=%#v", gw.Calls())
	}
}

func TestModelFocusRegainInSearchReloadsWithoutMutatingQuery(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	mark := gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.callCountSince(mark, fakes.MethodSearch) != 0 {
		t.Fatalf("expected query edit not to search before enter, got %#v", gw.Calls())
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if got := m.search.SessionState().AppliedQuery; got != "x" {
		t.Fatalf("expected applied search query %q, got %q", "x", got)
	}
	m.markSurfaceRefreshed(mode.Search)
	mark = gw.resetMark()

	next, cmd = m.Update(tea.BlurMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.FocusMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gw.hasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected focus regain in search to refresh immediately, calls=%#v", gw.Calls())
	}
	if gw.hasCallSince(mark, fakes.MethodDashboard) || gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected search focus regain to refresh only active search surface, calls=%#v", gw.Calls())
	}
	if got := m.search.SessionState().AppliedQuery; got != "x" {
		t.Fatalf("expected applied search query preserved as %q after focus regain, got %q", "x", got)
	}
}

func TestModelSearchHeaderUsesPageMetadataAndDraftQueryState(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// Seed tm-9 with "x" in title so the query "x" matches it.
	gw.seedReady("tm-9", "x Search result", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
	// Memory repo returns 1 real match; searchResultCount falls back to len(Results).
	if !strings.Contains(header, "Search: 1 results") {
		t.Fatalf("expected search header to reflect result count, got:\n%s", header)
	}
	if !strings.Contains(header, "Selected: tm-9 (open)") {
		t.Fatalf("expected header to keep active search selection, got:\n%s", header)
	}
	if got := m.search.SessionState(); got.DraftQuery != "xy" || got.AppliedQuery != "x" {
		t.Fatalf("expected app shell to preserve draft/applied query split, got %#v", got)
	}
}

func TestModelSearchPreviewSyncKeepsLastLoadedPreviewDuringReloadAndError(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// tm-9 with "x" in title so query "x" finds it.
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-9", Title: "x Search result", Status: "open", Priority: 1}, Description: "cached detail"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	if m.detail.Detail.Summary.ID != "tm-9" {
		t.Fatalf("expected search selection detail to load, got %#v", m.detail.Detail)
	}
	m.renderBody()
	if got := m.search.SessionState(); len(got.Page.Results) != 1 {
		t.Fatalf("expected search page state present before reload, got %#v", got)
	}

	cmd = m.search.AutoRefresh()
	if cmd == nil {
		t.Fatal("expected search auto-refresh command")
	}
	if session := m.search.SessionState(); !session.Loading || !session.Reloading {
		t.Fatalf("expected search session to mark reload in flight, got %#v", session)
	}
	gw.SetError(fakes.MethodSearch, errors.New("refresh boom"))

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

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	mark := gw.resetMark()
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

	if gw.callCountSince(mark, fakes.MethodSearch) != 0 {
		t.Fatalf("expected no repository calls before queued typing command resolves, got %#v", gw.Calls())
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	calls := gw.Calls()
	if gw.callCountSince(mark, fakes.MethodSearch) != 1 {
		t.Fatalf("expected only one enter-triggered search call while auto-refresh is suppressed, got %#v", calls)
	}
	if m.search.IsLoading() {
		t.Fatalf("expected typing-triggered search to settle")
	}
}

func TestModelRefreshTickSkipsWhileModalsOpenAndDetailLoading(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	mark := gw.resetMark()
	m.showHelp = true
	next, cmd := m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.callCountSince(mark, fakes.MethodDashboard)+gw.callCountSince(mark, fakes.MethodSearch)+gw.callCountSince(mark, fakes.MethodIssue) != 0 {
		t.Fatalf("expected no auto-refresh while help modal is open, calls=%#v", gw.Calls())
	}

	mark = gw.resetMark()
	m.showHelp = false
	m.showActionModal = true
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.callCountSince(mark, fakes.MethodDashboard)+gw.callCountSince(mark, fakes.MethodSearch)+gw.callCountSince(mark, fakes.MethodIssue) != 0 {
		t.Fatalf("expected no auto-refresh while action modal is open, calls=%#v", gw.Calls())
	}

	mark = gw.resetMark()
	m.showActionModal = false
	m.active = mode.Detail
	m.detail.Loading = true
	m.detail.TargetID = firstSelectionID(m, mode.Board)
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected duplicate detail reload suppression while loading, calls=%#v", gw.Calls())
	}
}

func TestModelMutationResultMarksBrowseDirtyAndRefreshesOnlyActiveSurface(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	mark := gw.resetMark()
	next, cmd := m.Update(mutationResultMsg{kind: mutationStatus, issueID: "tm-1"})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gw.hasCallSince(mark, fakes.MethodDashboard) {
		t.Fatalf("expected board to refresh immediately when active and dirty after write, calls=%#v", gw.Calls())
	}
	if gw.hasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected hidden search surface not to refresh from board-active write, calls=%#v", gw.Calls())
	}
	if !gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected write flow to keep immediate detail reload, calls=%#v", gw.Calls())
	}

	if state := m.refreshStateBySurface[mode.Board]; state.dirty {
		t.Fatalf("expected active board dirty flag to clear after refresh")
	}
	if state := m.refreshStateBySurface[mode.Search]; !state.dirty {
		t.Fatalf("expected inactive search to remain dirty until next eligible refresh")
	}

	mark = gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected active mode search after toggle, got %s", m.active)
	}
	if !gw.hasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected dirty search to refresh on activation, calls=%#v", gw.Calls())
	}
	if gw.hasCallSince(mark, fakes.MethodDashboard) {
		t.Fatalf("expected only newly active search to refresh on activation, calls=%#v", gw.Calls())
	}
	if state := m.refreshStateBySurface[mode.Search]; state.dirty {
		t.Fatalf("expected search dirty flag to clear after activation refresh")
	}
}

func TestModelRefreshTickHonorsStaleCadenceForActiveSurface(t *testing.T) {
	withModelNow(t, time.Unix(0, 0))

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m.markSurfaceRefreshed(mode.Board)

	mark := gw.resetMark()
	withModelNow(t, time.Unix(59, 0))
	next, cmd := m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if gw.hasCallSince(mark, fakes.MethodDashboard) {
		t.Fatalf("expected no board refresh before stale interval elapses, calls=%#v", gw.Calls())
	}

	mark = gw.resetMark()
	withModelNow(t, time.Unix(60, 0))
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gw.hasCallSince(mark, fakes.MethodDashboard) {
		t.Fatalf("expected board refresh at ~60s stale threshold, calls=%#v", gw.Calls())
	}
}

func TestModelWithNoAutoRefreshSkipsTickSchedulingInInit(t *testing.T) {
	refreshMarkerSeen := false

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModelWithOptions(t, services, RuntimeOptions{DisableAutoRefresh: true})
	// Install a non-nil scheduler so we can detect if it fires (it should not).
	m.scheduleRefreshTick = func() tea.Cmd {
		return func() tea.Msg { return refreshTickMsg{} }
	}
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

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModelWithOptions(t, services, RuntimeOptions{DisableAutoRefresh: true})
	m = applyMessages(t, m, runBatch(m.Init()))

	mark := gw.resetMark()
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

	if gw.callCountSince(mark, fakes.MethodDashboard)+gw.callCountSince(mark, fakes.MethodSearch)+gw.callCountSince(mark, fakes.MethodIssue) != 0 {
		t.Fatalf("expected no auto-refresh side effects from focus/tick when disabled, calls=%#v", gw.Calls())
	}

	mark = gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gw.hasCallSince(mark, fakes.MethodDashboard) {
		t.Fatalf("expected manual reload to include board data refresh, calls=%#v", gw.Calls())
	}
}

func TestModelRefreshInDetailDoesNotBackgroundPollInactiveBrowseSurfaces(t *testing.T) {

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "detail"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
	mark := gw.resetMark()
	next, cmd = m.Update(refreshTickMsg{})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if !gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected active detail to refresh when eligible, calls=%#v", gw.Calls())
	}
	if gw.hasCallSince(mark, fakes.MethodDashboard) || gw.hasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected no background refresh of inactive board/search surfaces, calls=%#v", gw.Calls())
	}
}

func TestModelDefaultTabAndShiftTabDoNotCycleModes(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	cfg := config.Default()
	cfg.UI.ShowModeSwitcherHelp = false

	services, err := NewServices(gw, cfg, t.TempDir())
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
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-2", Title: "In progress", Status: "in_progress", Priority: 2}, Description: "detail"})

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

	services, err := NewServices(gw, cfg, t.TempDir())
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
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("expected configured board move-right key to select tm-2, got %q", got)
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

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}, Description: "detail"})
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}})

	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{
			config.ShellActionCommentIssue: {"ctrl+a"},
		},
	})

	services, err := NewServices(gw, cfg, t.TempDir())
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

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1, func(i *memoryrepo.Issue) {
		i.Assignee = "hans"
		i.Labels = []string{"infra"}
	})
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
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
	if fakeEditor.Calls[0].IssueID != "tm-1" {
		t.Fatalf("expected selected issue tm-1, got %q", fakeEditor.Calls[0].IssueID)
	}

	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected edit hotkey to avoid launcher service, got %#v", fakeLauncher.Calls)
	}
}

func TestModelEditHotkeyShowsErrorToastWhenEditorFails(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{PrepareErr: errors.New("editor boom")}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
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
	if !strings.Contains(view, "Failed to edit issue tm-1") {
		t.Fatalf("expected editor failure toast, got:\n%s", view)
	}

	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected no launcher calls when editor fails, got %#v", fakeLauncher.Calls)
	}
}

func TestModelCreateIssueFlowUsesRepositoryCatalogsAndCreateIssue(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
		[]domain.TypeOption{{Name: "task"}, {Name: "bug"}},
		[]domain.LabelOption{{Name: "ui"}, {Name: "infra"}},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	if !gw.hasCatalogsCall() {
		t.Fatalf("expected catalogs to be queried, calls=%#v", gw.Calls())
	}

	if !gw.hasCreateIssueCall() {
		t.Fatalf("expected create issue repository call, calls=%#v", gw.Calls())
	}
}

func TestModelUpdateCloseAndCommentFlowsUseRepositoryWrites(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1, func(i *memoryrepo.Issue) { i.Labels = []string{"ui"} })
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1, Labels: []string{"ui"}}})
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
		[]domain.TypeOption{{Name: "task"}, {Name: "bug"}},
		[]domain.LabelOption{{Name: "ui"}, {Name: "infra"}},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	if !gw.hasUpdateIssueCall() {
		t.Fatalf("expected update issue call, calls=%#v", gw.Calls())
	}
	if !gw.hasCloseIssueCall() {
		t.Fatalf("expected close issue call, calls=%#v", gw.Calls())
	}
	if !gw.hasAddCommentCall() {
		t.Fatalf("expected add comment call, calls=%#v", gw.Calls())
	}
}

func TestModelBuiltInLauncherHotkeysUseLauncherService(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1, func(i *memoryrepo.Issue) { i.Labels = []string{"ui"} })
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
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

	gw := newTestRepository()
	gw.seedReady("tm-9", "Ninth", "task", 2)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: strings.Join(longLines, "\n"),
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

// TestModelDetailModeLeftBrowserUpDownMovesCursorOnlyThenEnterLoads verifies
// the decoupled navigation flow for an issue with a parent group (the parent
// shows as the last row of the dependency browser).
// After decoupling (Q5): ↑/↓ only moves the cursor highlight (no load cmd);
// Enter triggers OpenRelatedIssueIntent → loadDetailCmd (non-nil cmd).
func TestModelDetailModeLeftBrowserUpDownMovesCursorOnlyThenEnterLoads(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Root", "task", 1)
	gw.seedInProgress("tm-9", "Other", "task", 2)
	// tm-1 (viewed) has a blocker, a downstream issue, and a parent. The
	// dependency browser lists those deps followed by the parent row — a stable
	// 3 rows. Parent-only: the parent's other children (siblings) are not
	// surfaced. Pressing Enter on tm-6 (same shape) keeps the panel at 3 rows.
	parent := domain.IssueReference{ID: "tm-0", Title: "Parent epic"}
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:            domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 1},
		BlockedBy:          []domain.IssueReference{{ID: "tm-5", Title: "Upstream"}},
		Blocks:             []domain.IssueReference{{ID: "tm-6", Title: "Downstream"}},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{Parent: parent},
	})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:            domain.IssueSummary{ID: "tm-6", Title: "Downstream", Status: "in_progress", Type: "bug", Priority: 2},
		BlockedBy:          []domain.IssueReference{{ID: "tm-7", Title: "Upstream two"}},
		Blocks:             []domain.IssueReference{{ID: "tm-8", Title: "Downstream two"}},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{Parent: parent},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	m.detail.ContentScrollOffset = 5

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// (Q6a) Down only moves cursor — no preview load command (nil cmd).
	prevIndex := m.detail.BrowserSelectedIndex
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	// cmd may be nil or a no-op batch; it must NOT trigger a detail reload.
	mark := gw.resetMark()
	m = applyMessages(t, m, runBatch(cmd))
	if gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected down on browser to NOT trigger repository.Issue call, got calls=%#v", gw.Calls())
	}
	if m.detail.BrowserSelectedIndex == prevIndex {
		t.Errorf("expected BrowserSelectedIndex to advance after down, still at %d", prevIndex)
	}
	// Selection stays anchored; no TargetID change from the arrow alone.
	if m.detail.SelectionID != "tm-1" {
		t.Errorf("expected SelectionID to remain tm-1 after arrow, got %q", m.detail.SelectionID)
	}
	if got := firstSelectionID(m, mode.Board); got != "tm-1" {
		t.Errorf("expected board selection to stay anchored on tm-1, got %q", got)
	}

	// (Q6b) Enter triggers OpenRelatedIssueIntent → loadDetailCmd (non-nil cmd).
	mark = gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Errorf("expected app to remain in detail mode after Enter on browser panel, got %s", m.active)
	}
	if !gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected Enter on browser to trigger repository.Issue call (loadDetailCmd), calls=%#v", gw.Calls())
	}
	// Scroll must be reset and Loading must have been set (may now be false after applyMessages resolves the load).
	if m.detail.ContentScrollOffset != 0 {
		t.Errorf("expected ContentScrollOffset=0 after Enter-reload, got %d", m.detail.ContentScrollOffset)
	}
	// Full navigation: the Dependencies pane now reflects the DRILLED issue
	// (tm-6: its blocker tm-7, downstream tm-8, and parent tm-0) — not the
	// issue we came from.
	if got := browserIDs(m.detail.BrowserItems); strings.Join(got, ",") != "tm-7,tm-8,tm-0" {
		t.Errorf("expected drilled issue's deps+parent in browser, got %v", got)
	}
	if m.detail.SelectionID != "tm-6" {
		t.Errorf("expected SelectionID to follow the drill to tm-6, got %q", m.detail.SelectionID)
	}
}

// TestModelDetailModeDependenciesWithoutParentGroupUpDownMovesCursorOnlyThenEnterLoads
// verifies the decoupled navigation flow for deps-only (no parent group). After
// decoupling (Q5): ↑/↓ only moves the cursor (no load); Enter triggers reload.
func TestModelDetailModeDependenciesWithoutParentGroupUpDownMovesCursorOnlyThenEnterLoads(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Root", "task", 1)
	gw.seedInProgress("tm-9", "Other", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 1},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-3", Title: "Blocker"},
		},
		Blocks: []domain.IssueReference{
			{ID: "tm-5", Title: "Downstream"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-4", Title: "Related"},
		},
	})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-5", Title: "Downstream", Status: "in_progress", Type: "task", Priority: 2},
	})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-4", Title: "Related", Status: "in_progress", Type: "bug", Priority: 2},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	// Down twice: moves cursor to index 2 (tm-4 in the Related group). No load occurs.
	mark := gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected first down to NOT trigger Issue call, calls=%#v", gw.Calls())
	}

	mark = gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected second down to NOT trigger Issue call, calls=%#v", gw.Calls())
	}

	// Cursor is now on tm-4 (index 2). Enter triggers reload.
	mark = gw.resetMark()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Errorf("expected app to remain in detail mode after Enter on dependencies pane, got %s", m.active)
	}
	if !gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected Enter on dependencies pane to trigger repository.Issue call, calls=%#v", gw.Calls())
	}
	// TargetID must point to the cursor row (tm-4).
	if m.detail.TargetID != "tm-4" {
		t.Errorf("expected Enter to set TargetID=tm-4 (cursor row), got %q", m.detail.TargetID)
	}
	// Drilling is a full navigation: SelectionID now follows the target so the
	// Dependencies pane (and all panes) reflect tm-4, not the issue we came from.
	if m.detail.SelectionID != "tm-4" {
		t.Errorf("expected SelectionID to follow the drill to tm-4, got %q", m.detail.SelectionID)
	}
}

// TestModelDetailRoundTripEpicToChildAndBackViaParent is the end-to-end proof of
// the user-requested flow: open an epic's detail, drill into one of its children
// from the Children group, and then jump back to the epic via the child's own
// Parent row. The key property is that drilling is a FULL navigation — every
// pane, including the Dependencies rail, reflects the issue you land on — so the
// child shows its Parent group (which is what lets you go back up).
func TestModelDetailRoundTripEpicToChildAndBackViaParent(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-epic", "Auth epic", "epic", 1)
	// Epic's detail lists its child in the Children group.
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:  domain.IssueSummary{ID: "tm-epic", Title: "Auth epic", Status: "open", Type: "epic", Priority: 1},
		Children: []domain.IssueReference{{ID: "tm-child", Title: "Login crash", Type: "bug", Priority: 0, Status: "open"}},
	})
	// Child's detail lists the epic in its Parent group. Seeded in_progress (not
	// "open" with no deps) so it does not also land in the Ready column — the
	// epic stays the sole ready issue and thus the default board selection.
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:            domain.IssueSummary{ID: "tm-child", Title: "Login crash", Status: "in_progress", Type: "bug", Priority: 0},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{Parent: domain.IssueReference{ID: "tm-epic", Title: "Auth epic", Type: "epic", Priority: 1, Status: "open"}},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 160
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	// Open the epic's detail (the ready selection).
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.detail.SelectionID != "tm-epic" || m.detail.Detail.Summary.ID != "tm-epic" {
		t.Fatalf("setup: expected epic detail, got selection=%q detail=%q", m.detail.SelectionID, m.detail.Detail.Summary.ID)
	}
	if got := browserIDs(m.detail.BrowserItems); strings.Join(got, ",") != "tm-child" {
		t.Fatalf("expected epic deps pane to list its child, got %v", got)
	}

	// Focus the Dependencies pane and Enter on the child → drill down.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.detail.SelectionID != "tm-child" || m.detail.Detail.Summary.ID != "tm-child" {
		t.Fatalf("expected to land on the child, got selection=%q detail=%q", m.detail.SelectionID, m.detail.Detail.Summary.ID)
	}
	// The fix: the child's Dependencies pane now shows its OWN Parent group.
	if got := browserIDs(m.detail.BrowserItems); strings.Join(got, ",") != "tm-epic" {
		t.Fatalf("expected child deps pane to show its parent epic (drill must update the rail), got %v", got)
	}

	// Focus the Dependencies pane and Enter on the Parent row → jump back up.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.detail.SelectionID != "tm-epic" || m.detail.Detail.Summary.ID != "tm-epic" {
		t.Fatalf("expected round-trip back to the epic, got selection=%q detail=%q", m.detail.SelectionID, m.detail.Detail.Summary.ID)
	}
	if got := browserIDs(m.detail.BrowserItems); strings.Join(got, ",") != "tm-child" {
		t.Fatalf("expected epic deps pane to list its child again after round-trip, got %v", got)
	}
}

func TestModelDetailMetadataEnterOpensStatusDialogAndSubmitsStatusUpdate(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Root", "task", 1)
	gw.seedInProgress("tm-2", "Other", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 1}})
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}, {Name: "blocked"}},
		[]domain.TypeOption{{Name: "task"}},
		[]domain.LabelOption{},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	if !gw.hasCatalogsCall() {
		t.Fatalf("expected status catalog query, calls=%#v", gw.Calls())
	}
	if !gw.hasUpdateIssueCall() {
		t.Fatalf("expected status update issue call, calls=%#v", gw.Calls())
	}

	// Verify observable state: tm-1 should now have status "in_progress".
	updated := gw.issueState("tm-1")
	if updated == nil {
		t.Fatal("expected to find tm-1 in repository after update")
	}
	if updated.Summary.Status != "in_progress" {
		t.Fatalf("expected status updated to in_progress, got %q", updated.Summary.Status)
	}
}

func TestModelDetailMetadataStatusDialogEscapeCancelsWithoutSaving(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Root", "task", 1)
	gw.seedInProgress("tm-2", "Other", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 1}})
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}, {Name: "blocked"}},
		[]domain.TypeOption{{Name: "task"}},
		[]domain.LabelOption{},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	if gw.hasUpdateIssueCall() {
		t.Fatalf("expected no UpdateIssue call on escape cancel, calls=%#v", gw.Calls())
	}
}

func TestModelDetailMetadataStatusDialogEnterUnchangedIsNoOp(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Root", "task", 1)
	gw.seedInProgress("tm-2", "Other", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 1}})
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}, {Name: "blocked"}},
		[]domain.TypeOption{{Name: "task"}},
		[]domain.LabelOption{},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	if gw.hasUpdateIssueCall() {
		t.Fatalf("expected no UpdateIssue call on unchanged enter no-op, calls=%#v", gw.Calls())
	}

	if !m.toast.Visible() {
		t.Fatal("expected no-change toast to be visible after unchanged enter")
	}
}

func TestModelDetailMetadataEnterOnPriorityOpensDialogAndSubmitsPriorityUpdate(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Root", "task", 4)
	gw.seedInProgress("tm-2", "Other", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 4}})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	if !gw.hasUpdateIssueCall() {
		t.Fatalf("expected priority cycle update issue call, calls=%#v", gw.Calls())
	}

	// Verify observable state: tm-1 priority should be 0 after update.
	updated := gw.issueState("tm-1")
	if updated == nil {
		t.Fatal("expected to find tm-1 in repository after priority update")
	}
	if updated.Summary.Priority != 0 {
		t.Fatalf("expected priority updated to 0, got %d", updated.Summary.Priority)
	}
	// Status must be unchanged.
	if updated.Summary.Status != "open" {
		t.Fatalf("expected status unchanged after priority-only update, got %q", updated.Summary.Status)
	}
}

func TestModelDetailMetadataPriorityDialogEscapeCancelsWithoutSaving(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Root", "task", 1)
	gw.seedInProgress("tm-2", "Other", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 3}})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	if gw.hasUpdateIssueCall() {
		t.Fatalf("expected no UpdateIssue call on priority escape cancel, calls=%#v", gw.Calls())
	}
}

func TestModelLauncherSuccessToastClarifiesBackgroundLifecycle(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
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

	gw := newTestRepository()
	gw.seedReady("tm-9", "Ninth", "task", 2)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "Ninth detail",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	gw := newTestRepository()
	gw.seedReady("task-manager-ui-yze.4.2", "Implement create update close and comment actions in the app", "task", 1, func(iss *memoryrepo.Issue) {
		iss.Assignee = "alice"
		iss.Labels = []string{"ui", "shell"}
	})
	gw.seedIssueSummary(domain.IssueSummary{ID: "task-manager-ui-yze.4.5", Title: "Add editor and launcher integration tests", Status: "blocked", Type: "task", Priority: 1})
	gw.seedInProgress("task-manager-ui-yze.4.3", "Implement launcher framework with issue-context interpolation", "task", 1)
	gw.seedIssueDetail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       "task-manager-ui-yze.4.2",
			Title:    "Implement create update close and comment actions in the app",
			Status:   "open",
			Type:     "task",
			Priority: 1,
			Assignee: "alice",
			Labels:   []string{"ui", "shell"},
		},
		Description: "Show selected issue context clearly in browse mode.",
		BlockedBy:   []domain.IssueReference{{ID: "tm-9", Title: "Upstream migration"}},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-3", Title: "Blocked", Status: "blocked", Type: "bug", Priority: 0})
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	gw := newTestRepository()
	gw.seedReady("tm-9", "Ninth", "task", 2)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	// Seed initial detail (before edit) — memory repo returns last-seeded for a given ID,
	// so we seed "after edit" after Init() has loaded the "before" state.
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "detail before edit",
	})

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	fakeEditor := &fakes.FakeEditor{ApplyResult: launchereditor.Result{Updated: true}}
	services.Editor = fakeEditor

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if m.detail.Detail.Summary.ID != "tm-9" {
		t.Fatalf("expected initial detail load for selected issue tm-9, got %q", m.detail.Detail.Summary.ID)
	}

	// Re-seed with the "after edit" detail so subsequent Issue() call returns updated data.
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth edited", Status: "open", Type: "task", Priority: 2},
		Description: "detail after edit",
	})
	mark := gw.resetMark()

	// Phase 1: press 'e' → prepareEditCmd.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected edit command from edit hotkey")
	}

	// Phase 2: run prepareEditCmd → editIssuePreparedMsg; model returns tea.Exec cmd.
	preparedMsg := cmd()
	prepared, ok := preparedMsg.(editIssuePreparedMsg)
	if !ok {
		t.Fatalf("expected editIssuePreparedMsg, got %T", preparedMsg)
	}
	next, execCmd := m.Update(prepared)
	m = next.(Model)
	if execCmd == nil {
		t.Fatalf("expected tea.Exec command after prepare message")
	}

	// Phase 3: inject editorExitedMsg directly (bypasses real tea.Exec in unit tests).
	next, applyCmd := m.Update(editorExitedMsg{prepared: prepared.prepared, execErr: nil})
	m = next.(Model)
	if applyCmd == nil {
		t.Fatalf("expected apply command after editor exited message")
	}
	m = applyMessages(t, m, runBatch(applyCmd))

	if len(fakeEditor.Calls) != 1 {
		t.Fatalf("expected one editor call, got %d", len(fakeEditor.Calls))
	}
	if fakeEditor.Calls[0].IssueID != "tm-9" {
		t.Fatalf("expected editor call for tm-9, got %q", fakeEditor.Calls[0].IssueID)
	}

	if !gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("expected detail reload via Issue after successful update, calls=%#v", gw.Calls())
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

	gw := newTestRepository()
	gw.seedReady("tm-9", "Ninth", "task", 2)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "detail before edit",
	})

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
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

	mark := gw.resetMark()
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
	if fakeEditor.Calls[0].IssueID != "tm-9" {
		t.Fatalf("expected selected detail issue tm-9, got %q", fakeEditor.Calls[0].IssueID)
	}

	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected no launcher calls for edit hotkey, got %#v", fakeLauncher.Calls)
	}

	if gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Fatalf("did not expect issue reload from launcher action, calls=%#v", gw.Calls())
	}
}

func TestModelBoardDetailBoardRoundTripPreservesLayoutAndFocus(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-3", Title: "Blocked now", Status: "blocked", Type: "bug", Priority: 0})
	gw.seedInProgress("tm-2", "In progress one", "task", 1)
	// Pre-seed detail for both issues used during the round-trip.
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: "detail for ready issue",
	})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-2", Title: "In progress one", Status: "in_progress", Type: "task", Priority: 1},
		Description: "detail for in-progress issue",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("expected board selection tm-2 before round-trip, got %q", got)
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
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("expected board selection to remain on tm-2 after round-trip, got %q", got)
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

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress one", "task", 2)
	// Seed a search result so the search mode body renders something.
	gw.seedSearchResult(memoryrepo.Issue{ID: "tm-2", Title: "In progress one", Status: "in_progress", Priority: 2})
	services, err := NewServices(gw, config.Default(), t.TempDir())
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
		SelectionID: "tm-1",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-1", Title: "Issue one", Status: "open", Type: "task", Priority: 1},
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

func TestModelStartupHealthCheckClearsPathOnSuccess(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	gw := newTestRepository()
	gw.SetError(fakes.MethodHealthCheck, domain.RepositoryError{
		Code:    domain.ErrorCodeNoDatabaseFound,
		Message: "no task-manager store found",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	view := m.View()
	if !strings.Contains(view, "no task-manager store here") {
		t.Fatalf("expected fatal error title in View(), got %q", view)
	}
	if !strings.Contains(view, "taskmgr") {
		t.Fatalf("expected 'taskmgr' mention in View(), got %q", view)
	}
}

func TestModelFatalErrUpdateOnlyHandlesQuitAndResize(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.SetError(fakes.MethodHealthCheck, domain.RepositoryError{
		Code:    domain.ErrorCodeNoDatabaseFound,
		Message: "no task-manager store found",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	gw := newTestRepository()
	gw.SetError(fakes.MethodHealthCheck, domain.RepositoryError{
		Code:      domain.ErrorCodeNoDatabaseFound,
		Operation: "health check",
		Message:   "no task-manager store found",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
	if !strings.Contains(view, "no task-manager store here") {
		t.Fatalf("expected no-database title in View(), got %q", view)
	}
	if !strings.Contains(view, "taskmgr") {
		t.Fatalf("expected 'taskmgr' hint in View(), got %q", view)
	}
}

func TestModelFatalErrIgnoresNonRepositoryError(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.SetError(fakes.MethodHealthCheck, errors.New("some plain error"))

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	msgs := runBatch(m.Init())
	m = applyMessages(t, m, msgs)

	// A non-RepositoryError does not set fatalErr — app loads normally.
	if m.fatalErrTitle != "" {
		t.Fatalf("expected fatalErr to be empty for non-RepositoryError, got %q", m.fatalErrTitle)
	}
}

// TestModelFixtureShapedBoardCaptureGolden verifies the full board rendering at
// w120 against the embedded-fixture golden file, using fake data seeded to
// match the same bwf-1/bwf-2 fixture shape. This replaces
// TestModelEmbeddedFixtureFullBoardCaptureGolden (which used real taskmgr+fixture).
func TestModelFixtureShapedBoardCaptureGolden(t *testing.T) {
	gw := newTestRepository()
	// Match fixture shape: bwf-2 is Blocked (Not Ready lane), bwf-1 is Ready, no InProgress.
	gw.seedReady("bwf-1", "Seed fixture root task", "task", 1, func(iss *memoryrepo.Issue) {
		iss.Assignee = "alice"
		iss.Labels = []string{"fixture", "ui"}
	})
	gw.seedIssueSummary(domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0, Assignee: "bob", Labels: []string{"fixture", "blocking"}})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0, Assignee: "bob"},
		Description: "Used to validate blocked/ready and dependency reads.",
	})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bwf-1", Title: "Seed fixture root task", Status: "open", Type: "task", Priority: 1, Assignee: "alice"},
		Description: "Root task used by integration and e2e smoke tests.",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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
		t.Fatalf("expected fixture-shaped issue title in board capture, got:\n%s", view)
	}
	if strings.Count(view, "│") < 20 {
		t.Fatalf("expected full-height board lanes rather than floating boxes, got:\n%s", view)
	}

	ui.AssertMatchesGoldenNormalized(t, []byte(view), "model_embedded_board_w120.golden")
}

// TestModelStartupBoardLayoutSanityAndNoRuntimeErrors verifies that startup
// renders a valid board layout with no error panels. This replaces
// TestModelEmbeddedFixtureStartupLoadsBoardWithoutRepositorySectionErrors
// (which used real taskmgr+fixture).
func TestModelStartupBoardLayoutSanityAndNoRuntimeErrors(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("bwf-1", "Seed fixture root task", "task", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

// TestModelMutationModalsOpenWithoutCatalogDecodeToast verifies that c/u/a keys
// open the respective action modals without triggering a "Failed to load
// mutation catalogs" toast. This replaces
// TestModelEmbeddedFixtureMutationModalsOpenWithoutCatalogDecodeToast
// (which used real taskmgr+fixture).
func TestModelMutationModalsOpenWithoutCatalogDecodeToast(t *testing.T) {
	gw := newTestRepository()
	gw.seedIssueSummary(domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0, Assignee: "bob"},
		Description: "Used to validate blocked/ready and dependency reads.",
	})
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "blocked"}, {Name: "in_progress"}},
		[]domain.TypeOption{{Name: "task"}, {Name: "bug"}, {Name: "chore"}},
		[]domain.LabelOption{{Name: "fixture"}, {Name: "blocking"}},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	// 'c' opens Create Issue modal.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected create flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Create Issue")

	// Escape closes the modal.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	if m.showActionModal {
		t.Fatal("expected create modal to close on escape")
	}

	// 'u' opens Update Issue modal (title includes selected issue ID).
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected update flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Update Issue bwf-2")

	// Escape closes the modal.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	if m.showActionModal {
		t.Fatal("expected update modal to close on escape")
	}

	// Enter dedicated detail mode so the 'a' (add comment) hotkey works.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected detail mode before comment flow, got %s", m.active)
	}

	// 'a' opens Comment modal.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected comment flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Comment on bwf-2")
}

// TestAppHandlerOpenRelatedIssueIntentPerformsReloadFocusMoveAndScrollReset
// verifies that when the Details mode emits OpenRelatedIssueIntent (via Enter on
// a Dependencies pane row), the app shell handler performs the reload + focus
// move + scroll reset it already does (Q6c). This test directly sends
// OpenRelatedIssueIntent via a synthetic KeyMsg that drives the model through
// the production code path.
func TestAppHandlerOpenRelatedIssueIntentPerformsReloadFocusMoveAndScrollReset(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Main issue", "epic", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-child", Title: "Child issue", Status: "open", Type: "task", Priority: 2})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)

	// Put the model into Detail mode with tm-1 loaded and non-zero scroll offsets.
	m.active = mode.Detail
	m.detail = detailsmode.Model{
		SelectionID: "tm-1",
		TargetID:    "tm-1",
		FocusPane:   uidetails.FocusPaneDependencies,
		Detail: domain.IssueDetail{
			Summary:  domain.IssueSummary{ID: "tm-1", Title: "Main issue", Status: "open", Type: "epic", Priority: 1},
			Children: []domain.IssueReference{{ID: "tm-child", Title: "Child issue"}},
		},
		BrowserItems: []domain.IssueReference{
			{ID: "tm-child", Title: "Child issue"},
		},
		BrowserSelectedIndex:     0, // cursor on tm-child
		ContentScrollOffset:      5,
		MetadataScrollOffset:     3,
		DependenciesScrollOffset: 1,
		Keys:                     m.keys,
	}
	m.sizeKnown = true
	m.width = 160
	m.height = 34

	mark := gw.resetMark()

	// Send Enter: drives HandleKey which should emit OpenRelatedIssueIntent{tm-child}.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	// (Q6c) App handler must: set TargetID, Loading=true, reset all scroll offsets.
	if m.detail.TargetID != "tm-child" {
		t.Errorf("expected TargetID=tm-child after Enter on dep pane, got %q", m.detail.TargetID)
	}
	if !m.detail.Loading {
		t.Error("expected detail.Loading=true after Enter on dep pane")
	}
	if m.detail.ContentScrollOffset != 0 {
		t.Errorf("expected ContentScrollOffset=0 after Enter-reload, got %d", m.detail.ContentScrollOffset)
	}
	if m.detail.MetadataScrollOffset != 0 {
		t.Errorf("expected MetadataScrollOffset=0 after Enter-reload, got %d", m.detail.MetadataScrollOffset)
	}
	if m.detail.DependenciesScrollOffset != 0 {
		t.Errorf("expected DependenciesScrollOffset=0 after Enter-reload, got %d", m.detail.DependenciesScrollOffset)
	}
	if m.active != mode.Detail {
		t.Errorf("expected mode.Detail to stay active after Enter on dep pane, got %s", m.active)
	}

	// App must have issued a detail load command (Issue call).
	m = applyMessages(t, m, runBatch(cmd))
	if !gw.hasCallSince(mark, fakes.MethodIssue) {
		t.Error("expected repository.Issue call after Enter-reload; handler must dispatch loadDetailCmd")
	}
}

// TestAppHandlerDrillIntoDepWithDepsKeepsFocusOnDependenciesRail verifies that
// when the user presses Enter on a row in the Dependencies pane to drill into an
// issue that itself has dependencies:
//   - Focus is NOT flipped to Content during the optimistic placeholder phase.
//   - After the real detailLoadedMsg arrives, focus stays on the Dependencies rail.
func TestAppHandlerDrillIntoDepWithDepsKeepsFocusOnDependenciesRail(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Main issue", "epic", 1)
	// tm-child has its own blockers so it is not a leaf.
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "tm-child", Title: "Child issue", Status: "open", Type: "task", Priority: 2},
		BlockedBy: []domain.IssueReference{{ID: "tm-blocker", Title: "Blocker"}},
	})
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-blocker", Title: "Blocker", Status: "open"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m.active = mode.Detail
	m.detail = detailsmode.Model{
		SelectionID: "tm-1",
		TargetID:    "tm-1",
		FocusPane:   uidetails.FocusPaneDependencies,
		Detail: domain.IssueDetail{
			Summary:  domain.IssueSummary{ID: "tm-1", Title: "Main issue", Status: "open", Type: "epic", Priority: 1},
			Children: []domain.IssueReference{{ID: "tm-child", Title: "Child issue"}},
		},
		BrowserItems:         []domain.IssueReference{{ID: "tm-child", Title: "Child issue"}},
		BrowserSelectedIndex: 0,
		Keys:                 m.keys,
	}
	m.sizeKnown = true
	m.width = 160
	m.height = 34

	// Send Enter to drill into tm-child (has deps → non-leaf).
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	// Placeholder phase: focus must NOT have been flipped to Content.
	if m.detail.FocusPane != uidetails.FocusPaneDependencies {
		t.Errorf("placeholder phase: expected FocusPane=Dependencies, got %v", m.detail.FocusPane)
	}

	// Deliver the real detail (tm-child has BlockedBy so its rail is non-empty).
	realDetail := domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "tm-child", Title: "Child issue", Status: "open", Type: "task", Priority: 2},
		BlockedBy: []domain.IssueReference{{ID: "tm-blocker", Title: "Blocker"}},
	}
	next, _ = m.Update(detailLoadedMsg{issueID: "tm-child", detail: realDetail})
	m = next.(Model)

	// After real load: non-empty rail → focus must stay on Dependencies.
	if m.detail.FocusPane != uidetails.FocusPaneDependencies {
		t.Errorf("after real load with deps: expected FocusPane=Dependencies, got %v", m.detail.FocusPane)
	}

	// Suppress the unused-variable warning for cmd.
	_ = cmd
}

// TestAppHandlerDrillIntoLeafDepMovesFocusToContent verifies that when the user
// presses Enter on a Dependencies pane row to drill into a leaf issue (no deps),
// focus moves to the Content pane after the real detail loads.
func TestAppHandlerDrillIntoLeafDepMovesFocusToContent(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Main issue", "epic", 1)
	// tm-leaf has no dependencies.
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-leaf", Title: "Leaf issue", Status: "open", Type: "task", Priority: 2})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m.active = mode.Detail
	m.detail = detailsmode.Model{
		SelectionID: "tm-1",
		TargetID:    "tm-1",
		FocusPane:   uidetails.FocusPaneDependencies,
		Detail: domain.IssueDetail{
			Summary:  domain.IssueSummary{ID: "tm-1", Title: "Main issue", Status: "open", Type: "epic", Priority: 1},
			Children: []domain.IssueReference{{ID: "tm-leaf", Title: "Leaf issue"}},
		},
		BrowserItems:         []domain.IssueReference{{ID: "tm-leaf", Title: "Leaf issue"}},
		BrowserSelectedIndex: 0,
		Keys:                 m.keys,
	}
	m.sizeKnown = true
	m.width = 160
	m.height = 34

	// Send Enter to drill into tm-leaf (no deps → leaf).
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	// Placeholder phase: focus must NOT have been flipped to Content yet
	// (the focus decision is deferred to real load, not triggered by the empty placeholder).
	if m.detail.FocusPane != uidetails.FocusPaneDependencies {
		t.Errorf("placeholder phase: expected FocusPane=Dependencies (deferred), got %v", m.detail.FocusPane)
	}

	// Deliver the real detail (tm-leaf has no deps → empty rail).
	realDetail := domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-leaf", Title: "Leaf issue", Status: "open", Type: "task", Priority: 2},
	}
	next, _ = m.Update(detailLoadedMsg{issueID: "tm-leaf", detail: realDetail})
	m = next.(Model)

	// After real load: empty rail → focus must move to Content.
	if m.detail.FocusPane != uidetails.FocusPaneContent {
		t.Errorf("after real load with no deps: expected FocusPane=Content, got %v", m.detail.FocusPane)
	}

	_ = cmd
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

func browserIDs(refs []domain.IssueReference) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
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

	gw := newTestRepository()
	cfg := config.Default()
	// Inject an invalid keybinding: empty key slice for a required action.
	cfg.KeyBindings.Shell[config.ShellActionQuit] = []string{}

	services := Services{
		Repo:   gw,
		Config: cfg,
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

// TestHeaderSpinnerCellWidthInvariance asserts that headerSpinnerCell returns a
// string of identical lipgloss.Width whether or not any surface is loading.
func TestHeaderSpinnerCellWidthInvariance(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	services := Services{Repo: gw, Config: config.Default()}
	m := mustNewModel(t, services)
	// Drain Init so board loading completes and all surfaces are idle.
	m = applyMessages(t, m, runBatch(m.Init()))

	// idle model — no loading states
	idleCell := m.headerSpinnerCell()
	idleWidth := lipgloss.Width(idleCell)

	// simulate a loading state by setting detail.Loading
	m.detail.Loading = true
	loadingCell := m.headerSpinnerCell()
	loadingWidth := lipgloss.Width(loadingCell)

	if idleWidth != loadingWidth {
		t.Errorf("headerSpinnerCell width not invariant: idle=%d loading=%d (idle=%q loading=%q)",
			idleWidth, loadingWidth, idleCell, loadingCell)
	}
}

// TestHeaderSpinnerCellContainsGlyphWhenLoading asserts that the spinner cell
// contains one of the 10 pinned braille glyphs when loading is active, and
// contains none of them when idle.
func TestHeaderSpinnerCellContainsGlyphWhenLoading(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	services := Services{Repo: gw, Config: config.Default()}
	m := mustNewModel(t, services)
	// Drain Init so board loading completes and all surfaces are idle.
	m = applyMessages(t, m, runBatch(m.Init()))

	// Set a known frame that maps to a specific glyph.
	m.spinnerFrame = 0
	expectedGlyph := loading.Glyph(0)

	// loading active — use detail.Loading to avoid triggering repository calls
	m.detail.Loading = true
	loadingCell := m.headerSpinnerCell()
	if !strings.Contains(loadingCell, expectedGlyph) {
		t.Errorf("headerSpinnerCell when loading does not contain spinner glyph %q: got %q",
			expectedGlyph, loadingCell)
	}

	// verify none of the 10 glyphs appear when idle (all loading cleared)
	m.detail.Loading = false
	idleCell := m.headerSpinnerCell()
	for i, r := range loading.SpinnerFrames {
		g := string(r)
		if strings.Contains(idleCell, g) {
			t.Errorf("headerSpinnerCell when idle contains spinner glyph[%d] %q: got %q",
				i, g, idleCell)
		}
	}
}

// TestPendingDialogGuardStatusRaceEscCancelsOpen reproduces the async ESC race
// for the Update Status dialog:
//   - Enter on the Status metadata row dispatches an async catalog-load Cmd and
//     sets the pending-dialog guard.
//   - ESC arrives before the catalog response → guard is cleared; ESC is
//     consumed as "cancel the pending open" without popping Detail → Board.
//   - The catalog-loaded message arrives → guard is gone → modal is NOT opened.
//
// Expected: m.active == Detail, m.showActionModal == false.
func TestPendingDialogGuardStatusRaceEscCancelsOpen(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Root", "task", 1)
	gw.seedInProgress("tm-2", "Other", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 1}})
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}, {Name: "blocked"}},
		[]domain.TypeOption{{Name: "task"}},
		[]domain.LabelOption{},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 140
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	// Navigate to Detail mode.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected Detail mode after pressing 3, got %s", m.active)
	}

	// Navigate to the Metadata pane (Right arrow) and focus the Status row.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// Press Enter on the Status row — this dispatches the async catalog load and
	// sets the pending-dialog guard. Capture the Cmd but do NOT execute it yet.
	next, catalogLoadCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if catalogLoadCmd == nil {
		t.Fatal("expected async catalog-load Cmd after Enter on Status row")
	}
	if !m.pendingDialog.active {
		t.Fatal("expected pending-dialog guard to be active after dispatching catalog load")
	}
	if m.pendingDialog.kind != mutationStatus {
		t.Fatalf("expected pending-dialog kind=mutationStatus, got %q", m.pendingDialog.kind)
	}

	// ESC arrives during the load window. The guard must be cleared and ESC
	// must NOT pop Detail → Board.
	next, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if escCmd != nil {
		// Drain any follow-up commands (e.g. modeCmd from mode sub-models).
		m = applyMessages(t, m, runBatch(escCmd))
	}

	if m.active != mode.Detail {
		t.Fatalf("ESC during pending-dialog load popped mode to %s; want Detail", m.active)
	}
	if m.pendingDialog.active {
		t.Fatal("expected pending-dialog guard to be cleared after ESC")
	}

	// Now deliver the catalog-loaded message. Because the guard is gone the
	// handler must drop the result without opening the modal.
	catalogMsg := catalogLoadCmd()
	next, afterCmd := m.Update(catalogMsg)
	m = next.(Model)
	if afterCmd != nil {
		m = applyMessages(t, m, runBatch(afterCmd))
	}

	if m.showActionModal {
		t.Fatal("expected no action modal after ESC cancelled the pending-dialog open; got orphaned modal")
	}
	if m.active != mode.Detail {
		t.Fatalf("expected mode to remain Detail after catalog arrival with cancelled guard; got %s", m.active)
	}
}

// TestPendingDialogGuardCreateUpdateRaceEscCancelsOpen reproduces the async ESC
// race for the Create and Update mutation dialogs.
// Create dispatches with an empty IssueSummary (no issue ID), so the guard must
// key on kind, not issue ID.
//
//   - Press the Create-issue key → dispatches async catalog load, sets guard.
//   - ESC arrives before catalog response → guard cleared, no mode switch.
//   - mutationCatalogsLoadedMsg with kind=mutationCreate arrives → dropped, no modal.
//
// The Update path is also checked: guard is set with kind=mutationUpdate.
func TestPendingDialogGuardCreateUpdateRaceEscCancelsOpen(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		gw := newTestRepository()
		gw.seedReady("tm-1", "Root", "task", 1)
		gw.seedCatalogs(
			[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
			[]domain.TypeOption{{Name: "task"}, {Name: "bug"}},
			[]domain.LabelOption{},
		)

		services, err := NewServices(gw, config.Default(), t.TempDir())
		if err != nil {
			t.Fatalf("NewServices returned error: %v", err)
		}

		m := mustNewModel(t, services)
		m.width = 140
		m.height = 34
		m = applyMessages(t, m, runBatch(m.Init()))

		// Press "c" (ShellActionCreateIssue) — dispatches the async catalog load.
		next, catalogLoadCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
		m = next.(Model)
		if catalogLoadCmd == nil {
			t.Fatal("expected async catalog-load Cmd after Create-issue key")
		}
		if !m.pendingDialog.active {
			t.Fatal("expected pending-dialog guard to be active after dispatching create catalog load")
		}
		if m.pendingDialog.kind != mutationCreate {
			t.Fatalf("expected pending-dialog kind=mutationCreate, got %q", m.pendingDialog.kind)
		}

		// ESC during the load window cancels the pending open.
		next, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)
		if escCmd != nil {
			m = applyMessages(t, m, runBatch(escCmd))
		}

		if m.pendingDialog.active {
			t.Fatal("expected pending-dialog guard cleared after ESC")
		}

		// Deliver the catalog-loaded message. Guard is gone → modal must NOT open.
		// Construct the message directly using the empty-issue-ID shape that create uses.
		catalogMsg := mutationCatalogsLoadedMsg{
			kind:     mutationCreate,
			issue:    domain.IssueSummary{}, // empty ID — the create path
			statuses: []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
			types:    []domain.TypeOption{{Name: "task"}, {Name: "bug"}},
			labels:   []domain.LabelOption{},
		}
		next, afterCmd := m.Update(catalogMsg)
		m = next.(Model)
		if afterCmd != nil {
			m = applyMessages(t, m, runBatch(afterCmd))
		}

		if m.showActionModal {
			t.Fatal("expected no action modal after ESC cancelled the create pending-dialog open; got orphaned modal")
		}
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()

		gw := newTestRepository()
		gw.seedReady("tm-1", "Root", "task", 1)
		gw.seedCatalogs(
			[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
			[]domain.TypeOption{{Name: "task"}, {Name: "bug"}},
			[]domain.LabelOption{},
		)

		services, err := NewServices(gw, config.Default(), t.TempDir())
		if err != nil {
			t.Fatalf("NewServices returned error: %v", err)
		}

		m := mustNewModel(t, services)
		m.width = 140
		m.height = 34
		m = applyMessages(t, m, runBatch(m.Init()))

		// Press "u" (ShellActionUpdateIssue) — dispatches the async catalog load.
		next, catalogLoadCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
		m = next.(Model)
		if catalogLoadCmd == nil {
			t.Fatal("expected async catalog-load Cmd after Update-issue key")
		}
		if !m.pendingDialog.active {
			t.Fatal("expected pending-dialog guard to be active after dispatching update catalog load")
		}
		if m.pendingDialog.kind != mutationUpdate {
			t.Fatalf("expected pending-dialog kind=mutationUpdate, got %q", m.pendingDialog.kind)
		}

		// ESC during the load window cancels the pending open.
		next, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)
		if escCmd != nil {
			m = applyMessages(t, m, runBatch(escCmd))
		}

		if m.pendingDialog.active {
			t.Fatal("expected pending-dialog guard cleared after ESC")
		}

		// Deliver the catalog-loaded message. Guard is gone → modal must NOT open.
		catalogMsg := mutationCatalogsLoadedMsg{
			kind:     mutationUpdate,
			issue:    domain.IssueSummary{ID: "tm-1"},
			statuses: []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
			types:    []domain.TypeOption{{Name: "task"}, {Name: "bug"}},
			labels:   []domain.LabelOption{},
		}
		next, afterCmd := m.Update(catalogMsg)
		m = next.(Model)
		if afterCmd != nil {
			m = applyMessages(t, m, runBatch(afterCmd))
		}

		if m.showActionModal {
			t.Fatal("expected no action modal after ESC cancelled the update pending-dialog open; got orphaned modal")
		}
	})
}
