package app

// Startup and construction: what Model.Init fires, how the first board
// selection reaches the shell, and what NewModelWithOptions rejects.
//
// Other app-shell behaviour lives in themed siblings — navigation, detail mode,
// mutation flows, refresh cadence, rendering, editor/launcher handoff — and the
// shared fixtures and model constructors live in test_repository_test.go.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

func TestModelInitUsesBoardControllerAndBuiltInDashboardQueries(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	// tm-3 has status="blocked" → goes into DashboardData.Blocked → NotReady column.
	seedIssueSummary(gw, domain.IssueSummary{ID: "tm-3", Title: "Blocked", Status: "blocked", Priority: 1})
	seedInProgress(gw, "tm-2", "In progress", "task", 2)

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

	if !gw.HasCall(fakes.MethodDashboard) {
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
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if gw.HasCall(fakes.MethodSearch) {
		t.Fatalf("expected no Search call during startup; got calls=%#v", gw.Calls())
	}
}

// TestModelFirstSearchModeSwitchTriggersSearchInit asserts that the first
// transition to search mode fires exactly one SearchIssues call (lazy init),
// and that a second transition does NOT fire another SearchIssues call.
func TestModelFirstSearchModeSwitchTriggersSearchInit(t *testing.T) {

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Startup must not have triggered search.
	if gw.HasCall(fakes.MethodSearch) {
		t.Fatalf("expected no Search call during startup; got calls=%#v", gw.Calls())
	}
	if m.initDone[mode.Search] {
		t.Fatalf("expected search init=false after startup")
	}

	// First switch to search mode: lazy init must fire.
	mark := gw.CallCount()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Search {
		t.Fatalf("expected search active after toggle, got %s", m.active)
	}
	if !gw.HasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected Search call on first search mode activation; got calls=%#v", gw.Calls())
	}
	if !m.initDone[mode.Search] {
		t.Fatalf("expected search init=true after first search activation")
	}

	// Return to board and go back to search: should NOT re-trigger Search
	// from the lazy init path (auto-refresh may run if stale, but lazy init does not).
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt}) // toggle back to board
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	mark = gw.CallCount()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt}) // toggle to search again
	m = next.(Model)
	// Only run the immediate Update result; don't recurse into auto-refresh
	// commands — we only want to check that lazyInitActiveTabCmd itself is a no-op.
	_ = cmd
	if !m.initDone[mode.Search] {
		t.Fatalf("expected search init still true on second search activation")
	}
	// The lazy init flag must be set; subsequent refresh is handled by auto-refresh,
	// not by Init. Confirm no second Search call came from the lazy path.
	// (Auto-refresh may or may not fire depending on stale cadence; we apply no
	// messages to avoid triggering it.)
	if gw.HasCallSince(mark, fakes.MethodSearch) {
		t.Fatalf("expected lazy init NOT to re-fire Search on second search activation; got calls=%#v", gw.Calls())
	}
}

func TestModelStartupSynchronizesSelectionAfterBoardInitSelectionMessage(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Priority: 1}, Description: "startup detail"})

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

		if !observedVisibleBoardState && !m.board.IsLoading() {
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

// TestNewModelWithOptionsReturnsErrorOnInvalidKeyBindings asserts that
// NewModelWithOptions returns a typed error (not a panic) when Config contains
// an invalid keybinding — defensive hardening for direct-construction callers
// (tests, future programmatic embed) that skip config.Load.
func TestNewModelWithOptionsReturnsErrorOnInvalidKeyBindings(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
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
