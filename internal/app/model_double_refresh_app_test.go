package app

// Regression guard for the EXISTING boardIsLoading() guard at app/model.go:1076.
//
// This test is deliberately PASSING on current code — it is a regression prevention
// test, not a bug exposure test. Its purpose is to lock down the existing app-level
// guard so that future refactors cannot silently remove it.
//
// Context: The app-level refreshActiveSurfaceCmd (model.go:1073) has a guard:
//
//	case mode.Board:
//	    if m.boardIsLoading() {
//	        return nil
//	    }
//
// This guard prevents app-triggered auto-refreshes from stacking while a board
// refresh is already in flight. The tests below verify this invariant holds even
// when refreshTickMsg fires multiple times in rapid succession.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// countBoardRepositoryCalls counts calls to the board repository method (Dashboard).
// The memory repo records a single MethodDashboard call per board refresh, unlike
// FakeRepo which fanned out into 5 sub-calls (ReadyExplain + 3×Query + CountIssues).
func countBoardRepositoryCalls(gw *appTestRepository, start int) int {
	return gw.callCountSince(start, fakes.MethodDashboard)
}

// expandCmds recursively expands a tea.Cmd (which may return a BatchMsg) into
// the set of individual leaf Cmds. This lets us inspect whether a second batch
// of repository calls was produced without actually executing them.
func expandCmds(cmd tea.Cmd) []tea.Cmd {
	if cmd == nil {
		return nil
	}
	var out []tea.Cmd
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
		} else {
			// Re-wrap the already-invoked result so callers can still use it.
			captured := msg
			out = append(out, func() tea.Msg { return captured })
		}
	}
	return out
}

// TestAppRapidMutationsDoNotEnqueueConcurrentRefreshes guards the EXISTING
// boardIsLoading() check at app/model.go:1076 against silent regression.
//
// This test PASSES on current code. It will fail if the guard is accidentally
// removed or bypassed in a future refactor.
func TestAppRapidMutationsDoNotEnqueueConcurrentRefreshes(t *testing.T) {
	// Install deterministic tick schedulers — no real time advances during this test.
	withSpinnerTickScheduler(t, func() tea.Cmd { return nil })
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	gw := newTestRepository()
	gw.seedReady("tm-10", "Ready alpha", "task", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-11", Title: "Blocked beta", Status: "blocked", Priority: 2})
	gw.seedInProgress("tm-12", "In Progress gamma", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	// --- Cold-start board load ---
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if m.active != mode.Board {
		t.Fatalf("expected board active after init, got %s", m.active)
	}
	if m.boardIsLoading() {
		t.Fatalf("expected board to have settled after draining init messages")
	}

	// Mark the call count after cold-start to measure only refresh-triggered calls.
	markAfterInit := gw.resetMark()

	// --- Phase 1: First refreshTickMsg — triggers an auto-refresh ---
	// Mark board dirty so the surface refresh guard fires.
	m.markSurfaceDirty(mode.Board)

	// Fire first refreshTickMsg. The model calls maybeAutoRefreshActiveSurfaceCmd,
	// which calls board.AutoRefresh() (since board is NOT loading). This sets
	// board loading=true and returns a Dashboard repository Cmd.
	next, firstRefreshCmd := m.Update(refreshTickMsg{})
	m = next.(Model)

	if !m.boardIsLoading() {
		t.Fatalf("expected board to be loading after first refreshTickMsg")
	}

	// The first refresh Cmd should contain 1 board repository call (Dashboard).
	firstMsgs := runBatch(firstRefreshCmd)
	callsFromFirst := countBoardRepositoryCalls(gw, markAfterInit)
	if callsFromFirst != 1 {
		t.Fatalf("first refreshTickMsg: expected 1 board repository call, got %d", callsFromFirst)
	}

	// Mark call count to measure second-tick calls.
	markAfterFirst := gw.resetMark()

	// --- Phase 2: Second refreshTickMsg while board is STILL loading ---
	// Mark dirty again (simulating another mutation arriving while in-flight).
	m.markSurfaceDirty(mode.Board)

	// Fire second refreshTickMsg WITHOUT draining the first refresh's results.
	// The boardIsLoading() guard at app/model.go:1076 should block this.
	next, secondRefreshCmd := m.Update(refreshTickMsg{})
	m = next.(Model)

	// --- ASSERTION: second refreshTickMsg must NOT produce any board repository calls ---
	// Execute whatever the second refreshCmd contains and check for board calls.
	if secondRefreshCmd != nil {
		secondMsgs := runBatch(secondRefreshCmd)
		// Filter: the second refreshTickMsg schedules the next tick (via getRefreshTickScheduler),
		// so secondRefreshCmd may contain scheduler-level Cmds. We only care that
		// NO additional board repository calls were recorded.
		_ = secondMsgs
	}
	callsFromSecond := countBoardRepositoryCalls(gw, markAfterFirst)
	if callsFromSecond != 0 {
		t.Errorf("second refreshTickMsg: expected 0 board repository calls (boardIsLoading() guard should fire), got %d; app/model.go:1076 guard is broken", callsFromSecond)
	}

	// Board must still be loading (first refresh is still in-flight).
	if !m.boardIsLoading() {
		t.Errorf("board should still be loading after second refreshTickMsg — it was not drained")
	}

	// --- Phase 3: Drain the first refresh's results — board should settle cleanly ---
	m = applyMessages(t, m, firstMsgs)

	if m.boardIsLoading() {
		t.Errorf("expected board to have settled after draining first refresh messages")
	}

}
