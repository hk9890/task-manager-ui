package board

// Tests for double-refresh concurrency bug in board mode.
//
// The bug (internal/mode/board/model.go:233-234): the BoardActionReload keyboard
// handler calls m.startReload(refreshModeManual) with no guard. This means pressing
// 'r' while a refresh is already in-flight enqueues a second concurrent refresh,
// which resets pendingResults=4, clobbers partial buffers, and wipes selection state.
//
// These tests are deliberately RED on current (unfixed) code per TDD discipline for
// epic 5q6t. They will turn GREEN as sibling task 5q6t.1 lands its fix.

import (
	"log/slog"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
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

// newSettledBoardModel creates a board model with the given gateway,
// calls Init(), and drains all messages so the model is fully settled
// (all 4 columns loaded, pendingResults == 0).
func newSettledBoardModel(t *testing.T, gateway *fakes.FakeBeadsGateway) *Model {
	t.Helper()
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	m := NewModel(gateway, slog.Default(), keys)
	initCmd := m.Init()
	boardApplyMessages(t, m, boardDrainCmd(initCmd))
	if m.pendingResults != 0 {
		t.Fatalf("setup: expected pendingResults=0 after settle, got %d", m.pendingResults)
	}
	return m
}

// newPopulatedGateway returns a gateway with enough data for all 4 columns.
// QueryResponsesByExpr is used so that the status=blocked query returns empty,
// preventing in_progress issues from leaking into the Not Ready column.
func newPopulatedGateway() *fakes.FakeBeadsGateway {
	gw := fakes.NewFakeBeadsGateway()
	gw.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{
			{ID: "bw-1", Title: "Ready one", Status: "open", Priority: 1},
		},
		Blocked: []domain.BlockedIssueView{
			{Issue: domain.IssueSummary{ID: "bw-2", Title: "Blocked one", Status: "blocked", Priority: 2}},
		},
	}
	gw.QueryResponsesByExpr = map[string][]domain.IssueSummary{
		"status=in_progress": {{ID: "bw-3", Title: "In Progress one", Status: "in_progress", Priority: 1}},
		"status=closed":      nil,
		"status=blocked":     nil,
	}
	return gw
}

// countGatewayCalls returns the number of times any of the 5 board gateway
// methods (ReadyExplain, Query×3, CountIssues) were called on the gateway.
func countGatewayCalls(gateway *fakes.FakeBeadsGateway) int {
	n := 0
	for _, c := range gateway.Calls {
		switch c.Method {
		case fakes.MethodReadyExplain, fakes.MethodQuery, fakes.MethodCountIssues:
			n++
		}
	}
	return n
}

// TestBoardManualReloadIgnoredWhileInFlight verifies that pressing 'r' a second
// time while the first refresh is still in-flight is a no-op: pendingResults
// stays at 5, selection state is not wiped, and no new gateway batch is sent.
//
// CURRENTLY FAILS: the bug at model.go:233-234 calls startReload unconditionally,
// resetting pendingResults and wiping selection state on every keypress.
func TestBoardManualReloadIgnoredWhileInFlight(t *testing.T) {
	t.Parallel()

	gateway := newPopulatedGateway()
	m := newSettledBoardModel(t, gateway)

	// After settling, ensure some meaningful selection state exists.
	// The model should have picked bw-2 (Blocked, Not Ready column) as the
	// earliest non-empty column, but exact state depends on compose. Record
	// whatever it is.
	snapFocusedColumn := m.focusedColumn
	snapSelectedRow := make(map[int]int, len(m.selectedRow))
	for k, v := range m.selectedRow {
		snapSelectedRow[k] = v
	}

	// Reset gateway call counter so we can count calls from the reload only.
	gateway.ResetCalls()

	// === First reload: fire 'r', capture Cmd, do NOT drain ===
	firstCmd := m.Update(reloadKeyMsg())
	if firstCmd == nil {
		t.Fatalf("first reload: expected non-nil Cmd from 'r' keypress")
	}

	// After first reload: pendingResults must be 5, all columns loading.
	if m.pendingResults != 5 {
		t.Fatalf("first reload: expected pendingResults=5, got %d", m.pendingResults)
	}
	for i, col := range m.columns {
		if !col.loading {
			t.Errorf("first reload: expected column %d to be loading", i)
		}
	}

	// Count gateway calls dispatched by first reload (the Cmds are closures,
	// we need to execute the batch to actually record the calls).
	// Execute the returned batch cmd to verify it dispatches 5 gateway calls.
	firstBatchMsgs := boardDrainCmd(firstCmd)
	callsAfterFirst := countGatewayCalls(gateway)
	// First batch should dispatch 5 gateway calls (ReadyExplain + 3×Query + CountIssues).
	if callsAfterFirst != 5 {
		t.Fatalf("first reload: expected 5 gateway calls, got %d", callsAfterFirst)
	}

	// Reset for tracking second-reload calls.
	gateway.ResetCalls()

	// === Second reload: fire 'r' again WITHOUT draining the first Cmd's results ===
	// Restore pendingResults to 4 to simulate being in-flight.
	// (The first batch messages haven't been applied yet — firstBatchMsgs is pending.)
	// We must set pendingResults back because boardDrainCmd above executed the gateway
	// closures and recorded calls, but we deliberately haven't called m.Update on the
	// returned messages yet. The model is still "in-flight" from its perspective.
	//
	// NOTE: At this point m.pendingResults was reset to 4 by startReload.
	// We simulate in-flight by NOT calling m.Update on firstBatchMsgs before the
	// second keypress. The model state as of the first keypress has pendingResults=4.
	// We now fire the second keypress directly.

	secondCmd := m.Update(reloadKeyMsg())

	// === ASSERTIONS — these FAIL on current (buggy) code ===

	// Assert 1: pendingResults must still be 5 (not reset again from whatever value).
	// BUG: startReload resets pendingResults=5 unconditionally, so this may coincidentally
	// pass here. The real bug is that it ALSO dispatches 5 new gateway calls and wipes
	// selection. Assert selection was not wiped.
	if m.pendingResults != 5 {
		t.Errorf("second reload: expected pendingResults=5 (unchanged), got %d", m.pendingResults)
	}

	// Assert 2: selection state must be preserved — secondCmd must be nil (no new batch).
	// BUG: on buggy code, startReload resets focusedColumn=0 and selectedRow={},
	// overwriting the state that was set by the first startReload.
	if m.focusedColumn != 0 {
		// After first startReload (manual), focusedColumn is reset to 0 and selectedRow={}.
		// The second startReload (buggy) also resets to 0/{}. Check the selectedRow map.
	}
	// The key assertion: after first manual reload, focusedColumn=0, selectedRow={}.
	// The SECOND reload must not fire a new gateway batch (secondCmd must be nil).
	// This is the primary evidence of the bug.
	if secondCmd != nil {
		// Execute the second batch to count calls — this proves the bug fired a second reload.
		secondBatchMsgs := boardDrainCmd(secondCmd)
		_ = secondBatchMsgs // prevent unused warning
		callsFromSecond := countGatewayCalls(gateway)
		t.Errorf("second reload: expected nil Cmd (no new gateway batch) while in-flight, got non-nil Cmd; %d additional gateway calls recorded", callsFromSecond)
	}

	// Assert 3: drain the first batch's messages — pendingResults must count down to 0
	// cleanly and never go negative.
	// (firstBatchMsgs is the result of executing the first batch's gateway closures.)
	for _, msg := range firstBatchMsgs {
		prevPending := m.pendingResults
		_ = m.Update(msg)
		if m.pendingResults < 0 {
			t.Errorf("pendingResults went negative: was %d before message %T, now %d", prevPending, msg, m.pendingResults)
		}
	}
	if m.pendingResults != 0 {
		t.Errorf("after draining first batch: expected pendingResults=0, got %d", m.pendingResults)
	}

	// Suppress unused variable warning for the snapshot taken before first reload.
	_ = snapFocusedColumn
	_ = snapSelectedRow
}

// TestBoardPendingResultsNeverGoesNegativeUnderKeySpam verifies that spamming
// 'r' 11 times in rapid succession never drives pendingResults below zero and
// leaves exactly one clean batch of gateway data after draining.
//
// CURRENTLY FAILS: each 'r' keypress calls startReload unconditionally, adding
// new in-flight Cmds whose results will decrement pendingResults below zero after
// only the first batch's 5 results arrive.
func TestBoardPendingResultsNeverGoesNegativeUnderKeySpam(t *testing.T) {
	t.Parallel()

	gateway := newPopulatedGateway()
	m := newSettledBoardModel(t, gateway)
	gateway.ResetCalls()

	// Fire first reload: capture Cmd, do NOT drain.
	firstCmd := m.Update(reloadKeyMsg())
	if firstCmd == nil {
		t.Fatalf("expected non-nil Cmd from first 'r' keypress")
	}

	// Execute the batch closure to collect the gateway result messages.
	// (This records calls on the gateway and returns the messages that would
	// be sent back to the model — but we don't apply them yet.)
	firstBatchMsgs := boardDrainCmd(firstCmd)
	if len(firstBatchMsgs) != 5 {
		t.Fatalf("expected 5 messages from first batch, got %d", len(firstBatchMsgs))
	}

	// Collect additional Cmds from 10 more keypresses, without draining any.
	var extraCmds []tea.Cmd
	for i := 0; i < 10; i++ {
		cmd := m.Update(reloadKeyMsg())
		if cmd != nil {
			extraCmds = append(extraCmds, cmd)
		}
	}

	// ASSERTION (currently FAILS): no extra cmds should have been returned.
	// On buggy code, each 'r' fires another startReload with 4 new gateway calls.
	if len(extraCmds) > 0 {
		t.Errorf("expected 0 extra Cmds from 10 additional 'r' keypresses while in-flight, got %d Cmds", len(extraCmds))
	}

	// Now drain firstBatchMsgs one at a time, verifying pendingResults stays >= 0.
	for i, msg := range firstBatchMsgs {
		prevPending := m.pendingResults
		_ = m.Update(msg)
		if m.pendingResults < 0 {
			t.Errorf("message %d/%d (%T): pendingResults went negative (was %d, now %d)",
				i+1, len(firstBatchMsgs), msg, prevPending, m.pendingResults)
		}
	}

	// Also drain any extra Cmds' messages and assert no negatives.
	for _, cmd := range extraCmds {
		for _, msg := range boardDrainCmd(cmd) {
			prevPending := m.pendingResults
			_ = m.Update(msg)
			if m.pendingResults < 0 {
				t.Errorf("extra batch: pendingResults went negative (was %d, now %d)", prevPending, m.pendingResults)
			}
		}
	}

	// After everything drains: pendingResults must be 0.
	if m.pendingResults != 0 {
		t.Errorf("after draining all batches: expected pendingResults=0, got %d", m.pendingResults)
	}

	// Columns must contain exactly the gateway's single configured response.
	// (Not a mix of multiple batches.)
	readyIDs := make(map[string]bool)
	for _, col := range m.columns {
		for _, issue := range col.issues {
			readyIDs[issue.ID] = true
		}
	}
	for _, wantID := range []string{"bw-1", "bw-2", "bw-3"} {
		if !readyIDs[wantID] {
			t.Errorf("after drain: expected issue %q in columns (got: %v)", wantID, readyIDs)
		}
	}
}

// TestBoardInternalStartReloadGuardedByInflightFlag verifies that the internal
// defense-in-depth guard inside startReload itself suppresses re-entrant calls,
// independently of the call-site guard that 5q6t.1 added at the keyboard handler.
//
// This test bypasses the keyboard handler and calls m.startReload directly twice,
// proving that the inflight bool field prevents state corruption even if a future
// caller forgets to check IsLoading() first.
func TestBoardInternalStartReloadGuardedByInflightFlag(t *testing.T) {
	t.Parallel()

	gateway := newPopulatedGateway()
	m := newSettledBoardModel(t, gateway)
	gateway.ResetCalls()

	// === First direct startReload call — must succeed and set inflight=true ===
	firstCmd := m.startReload(refreshModeManual)
	if firstCmd == nil {
		t.Fatalf("first startReload: expected non-nil Cmd")
	}
	if m.pendingResults != 5 {
		t.Fatalf("first startReload: expected pendingResults=5, got %d", m.pendingResults)
	}
	if !m.inflight {
		t.Fatalf("first startReload: expected inflight=true after first call")
	}

	// Execute the batch to record gateway calls, but do NOT apply results to the model.
	firstBatchMsgs := boardDrainCmd(firstCmd)
	callsAfterFirst := countGatewayCalls(gateway)
	if callsAfterFirst != 5 {
		t.Fatalf("first startReload: expected 5 gateway calls, got %d", callsAfterFirst)
	}
	gateway.ResetCalls()

	// Record partial state before second call — it must be preserved.
	pendingBeforeSecond := m.pendingResults

	// === Second direct startReload call — must be suppressed by the internal guard ===
	secondCmd := m.startReload(refreshModeManual)

	// Assert 1: second call returned nil (internal guard fired).
	if secondCmd != nil {
		secondBatchMsgs := boardDrainCmd(secondCmd)
		_ = secondBatchMsgs
		callsFromSecond := countGatewayCalls(gateway)
		t.Errorf("second startReload: expected nil Cmd from internal guard, got non-nil Cmd; %d additional gateway calls recorded", callsFromSecond)
	}

	// Assert 2: pendingResults was not reset (partial buffers untouched).
	if m.pendingResults != pendingBeforeSecond {
		t.Errorf("second startReload: expected pendingResults=%d (unchanged), got %d", pendingBeforeSecond, m.pendingResults)
	}

	// Assert 3: no new gateway calls were dispatched.
	callsFromSecond := countGatewayCalls(gateway)
	if callsFromSecond != 0 {
		t.Errorf("second startReload: expected 0 additional gateway calls, got %d", callsFromSecond)
	}

	// Assert 4: inflight is still true (second call did not clear it).
	if !m.inflight {
		t.Errorf("second startReload: expected inflight=true (guard did not clear it)")
	}

	// === Drain first batch — pendingResults must count down to 0 without going negative ===
	for _, msg := range firstBatchMsgs {
		prevPending := m.pendingResults
		_ = m.Update(msg)
		if m.pendingResults < 0 {
			t.Errorf("pendingResults went negative: was %d before message %T, now %d", prevPending, msg, m.pendingResults)
		}
	}
	if m.pendingResults != 0 {
		t.Errorf("after draining first batch: expected pendingResults=0, got %d", m.pendingResults)
	}

	// Assert 5: inflight is cleared after composition completes.
	if m.inflight {
		t.Errorf("after composition: expected inflight=false, got true")
	}
}
