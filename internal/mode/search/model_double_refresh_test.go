package search

// Tests for double-refresh concurrency bug in search mode.
//
// The bug (internal/mode/search/model.go:243-244): the SearchActionReload keyboard
// handler calls m.triggerSearchPreservingSelection() with no guard. This means
// pressing 'r' while a search is already in-flight enqueues a second concurrent
// gateway call and overwrites m.pendingSelectionAnchor.
//
// These tests are deliberately RED on current (unfixed) code per TDD discipline for
// epic 5q6t. They will turn GREEN as sibling task 5q6t.2 lands its fix.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/repository"
	memoryrepo "github.com/hk9890/beads-workbench/internal/repository/memory"
)

// searchReloadKeyMsg returns a tea.KeyMsg for the 'r' key (the SearchActionReload default).
func searchReloadKeyMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}
}

// countSearchCalls returns the number of Search calls in the given slice.
func countSearchCalls(calls []repository.Call) int {
	n := 0
	for _, c := range calls {
		if c.Method == repository.MethodSearch {
			n++
		}
	}
	return n
}

// newSettledSearchModel creates a search model with the given gateway, calls
// Init(), drains all messages so the model is fully settled (loading=false),
// and then applies a search query to set appliedQuery so reload has something
// to re-trigger.
func newSettledSearchModel(t *testing.T, gateway *repository.ErrorInjectingRepository) *Model {
	t.Helper()
	m := initModel(gateway)
	if m.loading {
		t.Fatalf("setup: expected loading=false after initModel settle")
	}
	// appliedQuery is set by the init empty-query load. Verify we have a clean state.
	return m
}

// TestSearchManualReloadIgnoredWhileInFlight verifies that pressing 'r' a second
// time while the first reload is still in-flight is a no-op: no new gateway call
// is dispatched, pendingSelectionAnchor is not overwritten, and the returned Cmd
// is nil.
//
// CURRENTLY FAILS: the bug at model.go:243-244 calls
// triggerSearchPreservingSelection() unconditionally, firing a new gateway call
// and overwriting pendingSelectionAnchor on every keypress.
func TestSearchManualReloadIgnoredWhileInFlight(t *testing.T) {
	t.Parallel()

	repo := memoryrepo.New()
	repo.Seed(memoryrepo.Issue{ID: "bw-1", Title: "Result one", Status: "open", Priority: 1})
	repo.Seed(memoryrepo.Issue{ID: "bw-2", Title: "Result two", Status: "open", Priority: 2})
	gateway := repository.NewErrorInjecting(repo)

	m := newSettledSearchModel(t, gateway)

	// Navigate to results to establish a meaningful selection anchor.
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	// Focus should now be on results (FocusResults).

	// Record call count before reload so we measure only reload-triggered calls.
	callsBefore := len(gateway.Calls())

	// === First reload: fire 'r', capture Cmd, do NOT drain ===
	firstCmd := m.Update(searchReloadKeyMsg())
	if firstCmd == nil {
		t.Fatalf("first reload: expected non-nil Cmd from 'r' keypress")
	}

	// After first reload: model must be loading.
	if !m.loading {
		t.Fatalf("first reload: expected loading=true after 'r' keypress")
	}

	// The anchor set by the first reload — capture it before the second keypress.
	anchorAfterFirst := m.pendingSelectionAnchor

	// Execute the first Cmd's closure to record gateway calls (but don't apply
	// the returned messages to the model yet — leaving it "in flight").
	firstMsgs := drainCmd(firstCmd)
	callsAfterFirst := countSearchCalls(gateway.Calls()[callsBefore:])
	if callsAfterFirst != 1 {
		t.Fatalf("first reload: expected exactly 1 Search call, got %d", callsAfterFirst)
	}

	// Record count to measure second-reload calls.
	callsBeforeSecond := len(gateway.Calls())

	// === Second reload: fire 'r' again WITHOUT applying first Cmd's results ===
	secondCmd := m.Update(searchReloadKeyMsg())

	// === ASSERTIONS — these FAIL on current (buggy) code ===

	// Assert 1: gateway must NOT have received a second Search call.
	callsFromSecond := countSearchCalls(gateway.Calls()[callsBeforeSecond:])
	if callsFromSecond != 0 {
		// Execute the second batch to count calls.
		if secondCmd != nil {
			secondMsgs := drainCmd(secondCmd)
			_ = secondMsgs
		}
		t.Errorf("second reload: expected 0 new Search calls while in-flight, got %d", callsFromSecond)
	}

	// Assert 2: pendingSelectionAnchor must not have been overwritten.
	// The anchor from the first keypress must survive the second keypress.
	if m.pendingSelectionAnchor != anchorAfterFirst {
		t.Errorf("second reload: pendingSelectionAnchor was overwritten by second keypress; want %#v, got %#v",
			anchorAfterFirst, m.pendingSelectionAnchor)
	}

	// Assert 3: second Update must return nil Cmd (no new gateway batch).
	if secondCmd != nil {
		secondMsgs := drainCmd(secondCmd)
		_ = secondMsgs
		t.Errorf("second reload: expected nil Cmd (no new gateway batch) while in-flight, got non-nil Cmd")
	}

	// Settle the first reload's messages and verify loading becomes false.
	for _, msg := range firstMsgs {
		_ = m.Update(msg)
	}
	if m.loading {
		t.Errorf("after draining first reload: expected loading=false")
	}
}
