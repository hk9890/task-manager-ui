package board

// Controller-async contract tests.
//
// These tests exercise the board model against a deliberately-delayed
// repository so that async command overlap is exercised.
//
// # Why a separate file
//
// The existing model_test.go helpers (loadMoreCapture, makeClosedIssues)
// synchronously execute every Cmd before the next key arrives. That means
// doneLoadInFlight is always cleared before the next keypress is processed,
// making the race window that the in-flight guard is designed to prevent
// completely invisible.
//
// Here we use a goroutine-based driver: the loadMoreClosedCmd runs in a
// goroutine (blocked inside DelayedDashboardRepository.Dashboard), while the
// test synchronously sends additional keypresses to the model. Release()
// unblocks the goroutine, which returns the Msg to the model for processing.
// This matches real tea.Program cadence: user events can arrive before a prior
// async Cmd returns its Msg.
//
// # Regression pin
//
// TestDoneLoadMore_InFlightGuard passes on current code (post-commit ed859b4).
// If the doneLoadInFlight guard in dispatchLoadMoreClosed were removed,
// subsequent j presses during the in-flight window would each dispatch a new
// loadMoreClosedCmd, and the assertion "exactly 1 in-flight Dashboard call"
// would fail.

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// startCmdSubcmdsAsync executes cmd and, for each sub-cmd found in the result,
// starts it in a goroutine. If cmd produces a loadMoreClosedDoneMsg the
// goroutine sends it on the returned channel; all other msgs are discarded.
//
// The returned channel is buffered (capacity = number of sub-cmds started) so
// goroutines never leak when the test exits early. If cmd is nil or produces no
// loadMoreClosedDoneMsg sub-cmds, the channel will simply never receive.
func startCmdSubcmdsAsync(cmd tea.Cmd) <-chan loadMoreClosedDoneMsg {
	ch := make(chan loadMoreClosedDoneMsg, 16)
	if cmd == nil {
		return ch
	}

	msg := cmd()
	var subCmds []tea.Cmd
	switch v := msg.(type) {
	case tea.BatchMsg:
		subCmds = v
	default:
		// Single cmd result — already executed; nothing more to start.
		return ch
	}

	for _, sub := range subCmds {
		sub := sub
		if sub == nil {
			continue
		}
		go func() {
			result := sub()
			if lm, ok := result.(loadMoreClosedDoneMsg); ok {
				ch <- lm
			}
		}()
	}
	return ch
}

// waitForInFlight polls delayed.InFlight() until it reaches want or the
// deadline expires. Returns the final InFlight() value.
func waitForInFlight(delayed *fakes.DelayingRepository, want int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if n := delayed.InFlight(); n == want {
			return n
		}
		time.Sleep(time.Millisecond)
	}
	return delayed.InFlight()
}

// maxInFlightWithin polls delayed.InFlight() for the given duration and returns
// the highest value observed, returning early as soon as the count exceeds ceil.
// It lets a "count never rises above N" assertion wait on an observable
// condition over a bounded window instead of a fixed-duration sleep guess: a
// guard regression (a leaked in-flight call) is caught whenever it appears, and
// the full window is available on a slow/loaded runner.
func maxInFlightWithin(delayed *fakes.DelayingRepository, ceil int, d time.Duration) int {
	deadline := time.Now().Add(d)
	highest := delayed.InFlight()
	for time.Now().Before(deadline) {
		if n := delayed.InFlight(); n > highest {
			highest = n
		}
		if highest > ceil {
			return highest
		}
		time.Sleep(time.Millisecond)
	}
	return highest
}

// TestDoneLoadMore_InFlightGuard verifies that the doneLoadInFlight guard
// prevents parallel load-more dispatches under realistic async conditions.
//
// Setup: Done with 35 loaded of 736, cursor at row 31. The repository is
// wrapped in a fakes.DelayedDashboardRepository so the loadMoreClosedCmd's
// Dashboard call blocks until explicitly released.
//
// Steps:
//  1. Press j once (cursor → 32; remaining = 35-32 = 3 < threshold=5) →
//     exactly one loadMoreClosedCmd dispatched; doneLoadInFlight=true.
//  2. While the response is delayed (in-flight), press j 5 more times and
//     start each resulting sub-cmd in a goroutine (so any leaked load-more
//     also enters delayed.Dashboard and is observable via InFlight()).
//  3. Assert: exactly 1 in-flight Dashboard call; doneLoadInFlight=true.
//  4. Release the delayed response. Apply the resulting loadMoreClosedDoneMsg.
//  5. doneLoadedCount = 85 (35 prior + 50 incoming); doneLoadInFlight=false.
//  6. A 7th j press now dispatches the next load-more (ClosedOffset=85).
func TestDoneLoadMore_InFlightGuard(t *testing.T) {
	t.Parallel()

	const (
		priorLoaded = 35
		totalClosed = 736
		incomingN   = 50
	)

	// Build the incoming page the delayed repo will return when released.
	incomingIssues := make([]domain.IssueSummary, incomingN)
	for i := range incomingIssues {
		incomingIssues[i] = domain.IssueSummary{
			ID:    fmt.Sprintf("incoming-%d", i),
			Title: fmt.Sprintf("Incoming closed %d", i),
		}
	}
	loadMoreResp := repository.DashboardData{
		Closed:      incomingIssues,
		ClosedTotal: totalClosed,
	}

	// Stack: fixedDashboardRepo → counter → delayed.
	// counter records completed Dashboard calls (resolved through the delay gate).
	counter := newDashboardStub(loadMoreResp)
	delayed := fakes.NewDelayingDashboardRepository(counter)

	m := newBoardModel(delayed, resolvedBoardKeys(t))
	m.SetSize(120, 25) // sectionItemCapacity=22; closedPageSize=max(44,50)=50

	// Pre-populate Done column as if compose() already ran.
	priorIssues := makeClosedIssues(priorLoaded)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone, issues: priorIssues, total: totalClosed, exact: false},
	}
	m.doneLoadedCount = priorLoaded
	m.doneClosedTotal = totalClosed
	m.focusedColumn = doneColumnIndex
	// cursor at 31: remaining = 35-31 = 4 < threshold(5)
	m.selectedRow[doneColumnIndex] = 31

	// --- Step 1: press j once → threshold crossed → loadMoreClosedCmd dispatched ---

	cmd1 := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd1 == nil {
		t.Fatal("step 1: expected non-nil Cmd after first j press (threshold crossed)")
	}

	// doneLoadInFlight is set synchronously inside dispatchLoadMoreClosed before
	// the cmd is returned. Assert it immediately (no goroutines yet).
	if !m.doneLoadInFlight {
		t.Error("step 1: expected doneLoadInFlight=true after dispatching first load-more")
	}

	// Start the cmd's sub-cmds in goroutines. The loadMoreClosedCmd will block
	// in delayed.Dashboard; the selectionChangedCmd resolves immediately.
	loadMoreMsgCh := startCmdSubcmdsAsync(cmd1)

	// Wait until the goroutine enters delayed.Dashboard (InFlight==1).
	if n := waitForInFlight(delayed, 1, 2*time.Second); n != 1 {
		t.Errorf("step 1: expected 1 in-flight Dashboard call, got %d", n)
	}

	// --- Step 2: press j 5 more times while the response is in flight ---
	// Each cmd's sub-cmds are started in goroutines so that any leaked
	// loadMoreClosedCmd also enters delayed.Dashboard and is visible via InFlight().

	for i := 0; i < 5; i++ {
		cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		// Start sub-cmds in goroutines. With the guard working these are just
		// selectionChangedCmds (no load-more); without the guard, extra
		// loadMoreClosedCmds would accumulate in InFlight().
		_ = startCmdSubcmdsAsync(cmd)
	}

	// --- Step 3: assert exactly 1 in-flight call (guard blocked the rest) ---
	// Poll over a generous window so a leaked load-more goroutine has ample time
	// to reach delayed.Dashboard and be observed; the count must never exceed 1.
	// (A fixed sleep would pass for the wrong reason on a loaded runner where a
	// leaked goroutine simply hadn't been scheduled yet.)
	if n := maxInFlightWithin(delayed, 1, 500*time.Millisecond); n != 1 {
		t.Errorf("step 3: expected in-flight to stay at 1 after 5 guarded j presses, got %d", n)
	}
	if !m.doneLoadInFlight {
		t.Error("step 3: expected doneLoadInFlight=true throughout the in-flight window")
	}
	// No completed calls yet (delayed gate not released).
	if n := counter.dashboardCallCount(); n != 0 {
		t.Errorf("step 3: expected 0 completed Dashboard calls before release, got %d", n)
	}

	// --- Step 4: release the delayed response and apply the result ---

	delayed.Release()

	select {
	case loadMoreMsg := <-loadMoreMsgCh:
		// Apply the load-more response to the model.
		_ = m.Update(loadMoreMsg)
	case <-time.After(2 * time.Second):
		t.Fatal("step 4: timed out waiting for loadMoreClosedDoneMsg after release")
	}

	// --- Step 5: assert state after merge ---

	wantLoaded := priorLoaded + incomingN // 35 + 50 = 85
	if m.doneLoadedCount != wantLoaded {
		t.Errorf("step 5: doneLoadedCount: got %d, want %d", m.doneLoadedCount, wantLoaded)
	}
	if m.doneLoadInFlight {
		t.Error("step 5: expected doneLoadInFlight=false after load-more response applied")
	}
	if got := len(m.columns[doneColumnIndex].issues); got != wantLoaded {
		t.Errorf("step 5: Done column issue count: got %d, want %d", got, wantLoaded)
	}
	// Exactly 1 completed Dashboard call (the single released load-more).
	if n := counter.dashboardCallCount(); n != 1 {
		t.Errorf("step 5: expected 1 completed Dashboard call total, got %d", n)
	}

	// --- Step 6: 7th j press dispatches the next load-more (ClosedOffset=85) ---

	// Move cursor near the new end so the threshold triggers again.
	// After merge: doneLoadedCount=85; cursor needs remaining < 5, so row ≥ 81.
	m.selectedRow[doneColumnIndex] = 81 // remaining = 85-81 = 4 < 5

	cmd7 := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd7 == nil {
		t.Fatal("step 6: expected non-nil Cmd after 7th j press (new threshold at offset=85)")
	}
	if !m.doneLoadInFlight {
		t.Error("step 6: expected doneLoadInFlight=true after 7th j dispatches next load-more")
	}

	// Release so the second load-more completes and counter increments to 2.
	delayed.Release()
	ch7 := startCmdSubcmdsAsync(cmd7)

	select {
	case <-ch7:
		// second load-more resolved
	case <-time.After(2 * time.Second):
		t.Fatal("step 6: timed out waiting for second loadMoreClosedDoneMsg")
	}

	// Wait for counter to reflect the second completed call.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if counter.dashboardCallCount() == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if n := counter.dashboardCallCount(); n != 2 {
		t.Errorf("step 6: expected 2 total Dashboard calls (first + second load-more), got %d", n)
	}
}
