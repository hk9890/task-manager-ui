package board

// Controller-async contract tests for beads-workbench-vtvb.8.
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
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
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

// fixedDashboardRepo always returns a configured DashboardData regardless of
// opts. Used as the inner repo for the delayed wrapper so load-more responses
// contain predictable data.
type fixedDashboardRepo struct {
	resp repository.DashboardData
}

func (r *fixedDashboardRepo) Dashboard(_ context.Context, _ repository.DashboardOptions) (repository.DashboardData, error) {
	return r.resp, nil
}
func (r *fixedDashboardRepo) Issue(_ context.Context, _ string) (domain.IssueDetail, error) {
	return domain.IssueDetail{}, nil
}
func (r *fixedDashboardRepo) Search(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return domain.SearchResultPage{}, nil
}
func (r *fixedDashboardRepo) CreateIssue(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return domain.CreateIssueResult{}, nil
}
func (r *fixedDashboardRepo) UpdateIssue(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
	return nil
}
func (r *fixedDashboardRepo) CloseIssue(_ context.Context, _ string, _ domain.CloseIssueInput) error {
	return nil
}
func (r *fixedDashboardRepo) AddComment(_ context.Context, _ string, _ domain.AddCommentInput) error {
	return nil
}
func (r *fixedDashboardRepo) HealthCheck(_ context.Context) error { return nil }
func (r *fixedDashboardRepo) Catalogs(_ context.Context) (repository.Catalogs, error) {
	return repository.Catalogs{}, nil
}

var _ repository.Repository = (*fixedDashboardRepo)(nil)

// dashboardCallCounter wraps a repository.Repository and counts completed
// Dashboard calls. The counter increments after the inner.Dashboard returns
// (i.e., after the delayed gate has been passed), not when enqueued.
type dashboardCallCounter struct {
	mu    sync.Mutex
	n     int
	inner repository.Repository
}

func newDashboardCallCounter(inner repository.Repository) *dashboardCallCounter {
	return &dashboardCallCounter{inner: inner}
}

func (c *dashboardCallCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func (c *dashboardCallCounter) Dashboard(ctx context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	data, err := c.inner.Dashboard(ctx, opts)
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
	return data, err
}
func (c *dashboardCallCounter) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	return c.inner.Issue(ctx, id)
}
func (c *dashboardCallCounter) Search(ctx context.Context, q domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return c.inner.Search(ctx, q)
}
func (c *dashboardCallCounter) CreateIssue(ctx context.Context, inp domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return c.inner.CreateIssue(ctx, inp)
}
func (c *dashboardCallCounter) UpdateIssue(ctx context.Context, id string, inp domain.UpdateIssueInput) error {
	return c.inner.UpdateIssue(ctx, id, inp)
}
func (c *dashboardCallCounter) CloseIssue(ctx context.Context, id string, inp domain.CloseIssueInput) error {
	return c.inner.CloseIssue(ctx, id, inp)
}
func (c *dashboardCallCounter) AddComment(ctx context.Context, id string, inp domain.AddCommentInput) error {
	return c.inner.AddComment(ctx, id, inp)
}
func (c *dashboardCallCounter) HealthCheck(ctx context.Context) error {
	return c.inner.HealthCheck(ctx)
}
func (c *dashboardCallCounter) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	return c.inner.Catalogs(ctx)
}

var _ repository.Repository = (*dashboardCallCounter)(nil)

// waitForInFlight polls delayed.InFlight() until it reaches want or the
// deadline expires. Returns the final InFlight() value.
func waitForInFlight(delayed *fakes.DelayedDashboardRepository, want int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if n := delayed.InFlight(); n == want {
			return n
		}
		time.Sleep(time.Millisecond)
	}
	return delayed.InFlight()
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
	inner := &fixedDashboardRepo{resp: loadMoreResp}
	counter := newDashboardCallCounter(inner)
	delayed := fakes.NewDelayedDashboardRepository(counter)

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
		// Brief pause so any goroutine started above can reach delayed.Dashboard
		// before the next assertion. 5ms is ample for a goroutine to enter the
		// channel select; 0ms would be a race.
		time.Sleep(5 * time.Millisecond)
	}

	// --- Step 3: assert exactly 1 in-flight call (guard blocked the rest) ---

	if n := delayed.InFlight(); n != 1 {
		t.Errorf("step 3: expected 1 in-flight Dashboard call after 5 guarded j presses, got %d", n)
	}
	if !m.doneLoadInFlight {
		t.Error("step 3: expected doneLoadInFlight=true throughout the in-flight window")
	}
	// No completed calls yet (delayed gate not released).
	if n := counter.count(); n != 0 {
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
	if n := counter.count(); n != 1 {
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
		if counter.count() == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if n := counter.count(); n != 2 {
		t.Errorf("step 6: expected 2 total Dashboard calls (first + second load-more), got %d", n)
	}
}
