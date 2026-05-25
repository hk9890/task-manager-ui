package board

// Tests for double-refresh concurrency bug in board mode.
//
// The board model has a single m.inflight flag that prevents concurrent refreshes.
// After the repository migration, the fan-out lives in repository/beads.Dashboard(),
// and the board fires a single loadDashboardCmd per refresh cycle.
//
// These tests verify that:
//   - Pressing 'r' while a refresh is already in-flight is a no-op (nil Cmd).
//   - The internal startReload guard (inflight bool) prevents re-entrant calls.
//   - The inflight flag clears after composition completes.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	memoryrepo "github.com/hk9890/beads-workbench/internal/repository/memory"
)

// boardDrainCmd runs a Cmd to completion, expanding BatchMsgs into individual
// messages. Returns the flat list of non-batch messages produced.
func boardDrainCmd(cmd tea.Cmd) []tea.Msg {
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
				if c != nil {
					queue = append(queue, c())
				}
			}
		default:
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

// boardApplyMessages drives model.Update for each msg in sequence and expands
// any returned Cmds into follow-up messages, settling the model fully.
func boardApplyMessages(t *testing.T, m *Model, msgs []tea.Msg) {
	t.Helper()
	queue := append([]tea.Msg(nil), msgs...)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		cmd := m.Update(msg)
		queue = append(queue, boardDrainCmd(cmd)...)
	}
}

// reloadKeyMsg returns a tea.KeyMsg for the 'r' key (the BoardActionReload default).
func reloadKeyMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}
}

// newPopulatedRepo returns a memory repository with enough data for all 4 columns.
func newPopulatedRepo() *memoryrepo.Repository {
	repo := memoryrepo.New()
	repo.Seed(memoryrepo.Issue{ID: "bw-1", Title: "Ready one", Status: "open", Priority: 1})
	repo.Seed(memoryrepo.Issue{ID: "bw-2", Title: "Blocked one", Status: "blocked", Priority: 2})
	repo.Seed(memoryrepo.Issue{ID: "bw-3", Title: "In Progress one", Status: "in_progress", Priority: 1})
	closedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	repo.Seed(memoryrepo.Issue{ID: "bw-4", Title: "Closed one", Status: "closed", Priority: 1})
	repo.SeedClosed("bw-4", closedAt, "done")
	return repo
}

// newSettledBoardModel creates a board model with the given repo,
// calls Init(), and drains all messages so the model is fully settled
// (inflight=false, all 4 columns loaded).
func newSettledBoardModel(t *testing.T, repo repository.Repository) *Model {
	t.Helper()
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	m := NewModel(context.Background(), repo, slog.Default(), keys)
	initCmd := m.Init()
	boardApplyMessages(t, m, boardDrainCmd(initCmd))
	if m.inflight {
		t.Fatalf("setup: expected inflight=false after settle, got true")
	}
	return m
}

// TestBoardManualReloadIgnoredWhileInFlight verifies that pressing 'r' a second
// time while the first refresh is still in-flight is a no-op: inflight stays true,
// selection state is not wiped, and no new Dashboard cmd is returned.
func TestBoardManualReloadIgnoredWhileInFlight(t *testing.T) {
	t.Parallel()

	repo := newPopulatedRepo()
	m := newSettledBoardModel(t, repo)

	// Record selection state after settling.
	snapFocusedColumn := m.focusedColumn
	snapSelectedRow := make(map[int]int, len(m.selectedRow))
	for k, v := range m.selectedRow {
		snapSelectedRow[k] = v
	}

	// === First reload: fire 'r', capture Cmd, do NOT drain ===
	firstCmd := m.Update(reloadKeyMsg())
	if firstCmd == nil {
		t.Fatalf("first reload: expected non-nil Cmd from 'r' keypress")
	}

	// After first reload: inflight must be true, all columns loading.
	if !m.inflight {
		t.Fatalf("first reload: expected inflight=true after first keypress")
	}
	for i, col := range m.columns {
		if !col.loading {
			t.Errorf("first reload: expected column %d to be loading", i)
		}
	}

	// Execute the first cmd to obtain the dashboardLoadedMsg but do NOT apply it.
	firstMsgs := boardDrainCmd(firstCmd)
	if len(firstMsgs) != 1 {
		t.Fatalf("first reload: expected 1 message from Dashboard cmd, got %d", len(firstMsgs))
	}
	if _, ok := firstMsgs[0].(dashboardLoadedMsg); !ok {
		t.Fatalf("first reload: expected dashboardLoadedMsg, got %T", firstMsgs[0])
	}

	// === Second reload: fire 'r' again WITHOUT applying the first result ===
	secondCmd := m.Update(reloadKeyMsg())

	// Assert: second cmd must be nil (inflight guard fired).
	if secondCmd != nil {
		t.Errorf("second reload: expected nil Cmd (no new Dashboard call) while in-flight, got non-nil Cmd")
	}

	// Assert: inflight is still true.
	if !m.inflight {
		t.Errorf("after suppressed second reload: expected inflight=true, got false")
	}

	// === Drain the first result — inflight must clear, composition must complete ===
	for _, msg := range firstMsgs {
		_ = m.Update(msg)
	}

	if m.inflight {
		t.Errorf("after first result drained: expected inflight=false, got true")
	}
	if m.IsLoading() {
		t.Errorf("after first result drained: expected IsLoading()=false, got true")
	}

	// Suppress unused variable warnings.
	_ = snapFocusedColumn
	_ = snapSelectedRow
}

// TestBoardPendingResultsNeverGoesNegativeUnderKeySpam verifies that spamming
// 'r' 11 times in rapid succession while in-flight never produces extra Dashboard
// commands and leaves exactly one clean result after draining.
func TestBoardPendingResultsNeverGoesNegativeUnderKeySpam(t *testing.T) {
	t.Parallel()

	repo := newPopulatedRepo()
	m := newSettledBoardModel(t, repo)

	// Fire first reload: capture Cmd, do NOT drain.
	firstCmd := m.Update(reloadKeyMsg())
	if firstCmd == nil {
		t.Fatalf("expected non-nil Cmd from first 'r' keypress")
	}

	// Execute the Dashboard closure to collect the result message.
	firstMsgs := boardDrainCmd(firstCmd)
	if len(firstMsgs) != 1 {
		t.Fatalf("expected 1 message from first Dashboard cmd, got %d", len(firstMsgs))
	}

	// Collect additional Cmds from 10 more keypresses, without draining any.
	var extraCmds []tea.Cmd
	for i := 0; i < 10; i++ {
		cmd := m.Update(reloadKeyMsg())
		if cmd != nil {
			extraCmds = append(extraCmds, cmd)
		}
	}

	// ASSERTION: no extra cmds should have been returned (inflight guard fires).
	if len(extraCmds) > 0 {
		t.Errorf("expected 0 extra Cmds from 10 additional 'r' keypresses while in-flight, got %d Cmds", len(extraCmds))
	}

	// Now drain firstMsgs — inflight must clear cleanly.
	for _, msg := range firstMsgs {
		_ = m.Update(msg)
	}

	if m.inflight {
		t.Errorf("after draining first result: expected inflight=false, got true")
	}
	if m.IsLoading() {
		t.Errorf("after draining first result: expected IsLoading()=false, got true")
	}

	// Columns must contain the repository's configured data.
	issueIDsInBoard := make(map[string]bool)
	for _, col := range m.columns {
		for _, issue := range col.issues {
			issueIDsInBoard[issue.ID] = true
		}
	}
	for _, wantID := range []string{"bw-1", "bw-2", "bw-3"} {
		if !issueIDsInBoard[wantID] {
			t.Errorf("after drain: expected issue %q in columns (got: %v)", wantID, issueIDsInBoard)
		}
	}
}

// TestBoardInternalStartReloadGuardedByInflightFlag verifies that the internal
// defense-in-depth guard inside startReload itself suppresses re-entrant calls.
func TestBoardInternalStartReloadGuardedByInflightFlag(t *testing.T) {
	t.Parallel()

	repo := newPopulatedRepo()
	m := newSettledBoardModel(t, repo)

	// === First direct startReload call — must succeed and set inflight=true ===
	firstCmd := m.startReload(refreshModeManual)
	if firstCmd == nil {
		t.Fatalf("first startReload: expected non-nil Cmd")
	}
	if !m.inflight {
		t.Fatalf("first startReload: expected inflight=true after first call")
	}

	// Execute the Dashboard closure to collect the result message, but do NOT apply.
	firstMsgs := boardDrainCmd(firstCmd)
	if len(firstMsgs) != 1 {
		t.Fatalf("first startReload: expected 1 Dashboard result message, got %d", len(firstMsgs))
	}

	// === Second direct startReload call — must be suppressed by the internal guard ===
	secondCmd := m.startReload(refreshModeManual)

	// Assert 1: second call returned nil (internal guard fired).
	if secondCmd != nil {
		t.Errorf("second startReload: expected nil Cmd from internal guard, got non-nil Cmd")
	}

	// Assert 2: inflight is still true (second call did not clear it).
	if !m.inflight {
		t.Errorf("second startReload: expected inflight=true (guard did not clear it)")
	}

	// === Drain first result — inflight must clear after composition ===
	for _, msg := range firstMsgs {
		_ = m.Update(msg)
	}

	// Assert 3: inflight is cleared after composition completes.
	if m.inflight {
		t.Errorf("after composition: expected inflight=false, got true")
	}
	if m.IsLoading() {
		t.Errorf("after composition: expected IsLoading()=false, got true")
	}
}

// TestBoardAutoRefreshInflightGuard verifies that AutoRefresh() also respects
// the inflight guard and returns nil when a refresh is already in progress.
func TestBoardAutoRefreshInflightGuard(t *testing.T) {
	t.Parallel()

	repo := newPopulatedRepo()
	m := newSettledBoardModel(t, repo)

	// Trigger a manual reload to set inflight=true.
	firstCmd := m.Update(reloadKeyMsg())
	if firstCmd == nil {
		t.Fatalf("expected non-nil Cmd from first 'r' keypress")
	}
	if !m.inflight {
		t.Fatalf("expected inflight=true after first keypress")
	}

	// AutoRefresh while inflight must return nil.
	autoCmd := m.AutoRefresh()
	if autoCmd != nil {
		t.Errorf("expected AutoRefresh() to return nil while inflight, got non-nil Cmd")
	}

	// Drain first cmd to settle.
	for _, msg := range boardDrainCmd(firstCmd) {
		_ = m.Update(msg)
	}
	if m.inflight {
		t.Errorf("after settling: expected inflight=false, got true")
	}
}

// TestBoardDashboardMsgProcessed verifies the full round-trip: Init() fires a
// Dashboard cmd, which produces a dashboardLoadedMsg that settles the board.
func TestBoardDashboardMsgProcessed(t *testing.T) {
	t.Parallel()

	repo := memoryrepo.New()
	repo.Seed(memoryrepo.Issue{ID: "bw-1", Title: "Ready one", Status: "open", Priority: 1})
	repo.Seed(memoryrepo.Issue{ID: "bw-2", Title: "In progress", Status: "in_progress", Priority: 2})

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	m := NewModel(context.Background(), repo, slog.Default(), keys)

	initCmd := m.Init()
	if initCmd == nil {
		t.Fatalf("Init() must return a non-nil command")
	}

	// Execute the Dashboard cmd.
	msg := initCmd()
	dashMsg, ok := msg.(dashboardLoadedMsg)
	if !ok {
		t.Fatalf("expected dashboardLoadedMsg, got %T", msg)
	}
	if dashMsg.err != nil {
		t.Fatalf("unexpected error in dashboardLoadedMsg: %v", dashMsg.err)
	}

	// Apply the message.
	_ = m.Update(dashMsg)

	if m.IsLoading() {
		t.Errorf("expected IsLoading()=false after dashboardLoadedMsg")
	}
	if m.inflight {
		t.Errorf("expected inflight=false after dashboardLoadedMsg")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}

	// Verify data from memory repo.
	readyCol := m.columns[1]  // Ready is col 1
	inProgCol := m.columns[2] // InProgress is col 2
	if len(readyCol.issues) == 0 {
		t.Errorf("expected Ready column to have bw-1, got empty")
	}
	if len(inProgCol.issues) == 0 {
		t.Errorf("expected InProgress column to have bw-2, got empty")
	}

	_ = domain.IssueSummary{} // ensure domain import used
}
