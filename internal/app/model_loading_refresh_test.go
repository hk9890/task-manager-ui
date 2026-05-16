package app

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
)

// extractIssueIDsFromView returns the set of issue IDs visible in a rendered
// view string by scanning for bd-style ID tokens (e.g. "bw-1", "bw-42").
func extractIssueIDsFromView(rendered string) map[string]bool {
	re := regexp.MustCompile(`\bbw-\d+\b`)
	found := re.FindAllString(rendered, -1)
	result := make(map[string]bool, len(found))
	for _, id := range found {
		result[id] = true
	}
	return result
}

// containsAnySpinnerGlyph reports whether the rendered string contains any of
// the pinned braille spinner glyphs from loading.SpinnerFrames.
func containsAnySpinnerGlyph(rendered string) bool {
	for _, r := range loading.SpinnerFrames {
		if strings.ContainsRune(rendered, r) {
			return true
		}
	}
	return false
}

// TestNonBlockingRefreshBoardSearchBoardFlow is an in-process integration test
// that drives the app model through a Board → reload-in-flight → Board →
// Search → Board cycle and asserts the non-blocking refresh invariants:
//
//  1. Stale board rows remain visible in View() during an in-flight reload
//     (the board is loading=true but existing issues are shown).
//  2. The header spinner glyph from loading.SpinnerFrames appears in View()
//     while a board reload is in flight.
//  3. The spinner glyph is absent from View() once the reload has settled.
//  4. The rows visible at the first board capture are still in the set visible
//     after a Board → Search → Board round-trip (data is preserved).
func TestNonBlockingRefreshBoardSearchBoardFlow(t *testing.T) {
	// Install deterministic tick schedulers — no real time passes.
	withSpinnerTickScheduler(t, func() tea.Cmd { return nil })
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	// Configure the fake gateway with known, distinguishable issue IDs.
	gateway := fakes.NewFakeBeadsGateway()
	gateway.ReadyExplainResponse = domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{
			{ID: "bw-10", Title: "Ready issue alpha", Status: "open", Priority: 1},
		},
		Blocked: []domain.BlockedIssueView{
			{Issue: domain.IssueSummary{ID: "bw-11", Title: "Blocked issue beta", Status: "blocked", Priority: 2}},
		},
	}
	gateway.QueryResponse = []domain.IssueSummary{
		{ID: "bw-12", Title: "In Progress gamma", Status: "in_progress", Priority: 1},
	}
	gateway.SearchIssuesResponse = domain.SearchResultPage{
		Results: []domain.SearchResult{
			{Issue: domain.IssueSummary{ID: "bw-20", Title: "Search result delta", Status: "open", Priority: 1}},
		},
	}

	services, err := NewServices(gateway, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	// --- Phase 1: Cold-start board load ---

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if m.active != mode.Board {
		t.Fatalf("expected board active after init, got %s", m.active)
	}
	if m.boardIsLoading() {
		t.Fatalf("expected board to have settled after draining init messages")
	}

	// Capture the View() and verify the known board issue IDs are present.
	initialView := m.View()
	initialIDs := extractIssueIDsFromView(initialView)

	for _, wantID := range []string{"bw-10", "bw-11", "bw-12"} {
		if !initialIDs[wantID] {
			t.Fatalf("cold-start board View() missing expected issue ID %q\nview:\n%s", wantID, initialView)
		}
	}

	// Spinner must be absent when idle.
	if containsAnySpinnerGlyph(initialView) {
		t.Fatalf("spinner glyph present in idle board View(); expected none\nview:\n%s", initialView)
	}

	// --- Phase 2: Board reload in-flight — stale data must stay visible ---

	// Mark board surface dirty so maybeAutoRefreshActiveSurfaceCmd fires.
	m.markSurfaceDirty(mode.Board)

	// Send a refreshTickMsg. The model's Update immediately calls
	// m.board.AutoRefresh() inside refreshActiveSurfaceCmd, which sets
	// loading=true on each column but preserves existing issues. The returned
	// cmd contains the 3 pending gateway fetch commands — we capture them
	// without running them yet, so the board is "in flight".
	next, refreshCmd := m.Update(refreshTickMsg{})
	m = next.(Model)

	// Board must now be loading (in-flight).
	if !m.boardIsLoading() {
		t.Fatalf("expected board to be loading after refreshTickMsg with dirty surface")
	}

	// Capture View() BEFORE draining the in-flight gateway results.
	// Stale issue IDs must still be visible — NOT replaced by skeleton or blank.
	inFlightView := m.View()
	inFlightIDs := extractIssueIDsFromView(inFlightView)

	for _, wantID := range []string{"bw-10", "bw-11", "bw-12"} {
		if !inFlightIDs[wantID] {
			t.Fatalf("in-flight board View() is missing stale issue ID %q — non-blocking refresh broken\nview:\n%s", wantID, inFlightView)
		}
	}

	// --- Phase 3: Spinner glyph appears during in-flight reload ---

	// Advance the spinner one tick so spinnerFrame moves from 0 to 1.
	// We pin to frame 0 first so the expected glyph is deterministic.
	m.spinnerFrame = 0
	spinnerNext, _ := m.Update(loading.TickMsg{})
	m = spinnerNext.(Model)

	spinnerView := m.View()
	if !containsAnySpinnerGlyph(spinnerView) {
		t.Fatalf("expected spinner glyph in View() during in-flight board reload\nview:\n%s", spinnerView)
	}

	// --- Phase 4: Spinner absent after results land ---

	// Now drain the in-flight board refresh (gateway responds with same data).
	m = applyMessages(t, m, runBatch(refreshCmd))

	if m.boardIsLoading() {
		t.Fatalf("expected board to have settled after draining refresh gateway results")
	}

	// Spinner glyph must be gone once all surfaces are idle.
	settledView := m.View()
	if containsAnySpinnerGlyph(settledView) {
		t.Fatalf("spinner glyph still present in View() after board reload settled\nview:\n%s", settledView)
	}

	// Board issue IDs must still be present after reload.
	settledIDs := extractIssueIDsFromView(settledView)
	for _, wantID := range []string{"bw-10", "bw-11", "bw-12"} {
		if !settledIDs[wantID] {
			t.Fatalf("settled board View() missing issue ID %q after reload\nview:\n%s", wantID, settledView)
		}
	}

	// --- Phase 5: Board → Search → Board round-trip ---
	// Verify that after switching to Search and back, the original board rows
	// are still visible.

	// Switch to Search (lazy init fires).
	searchNext, searchCmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = searchNext.(Model)
	m = applyMessages(t, m, runBatch(searchCmd))

	if m.active != mode.Search {
		t.Fatalf("expected search mode after ctrl+space toggle, got %s", m.active)
	}

	// Switch back to Board.
	boardNext, boardBackCmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = boardNext.(Model)
	m = applyMessages(t, m, runBatch(boardBackCmd))

	if m.active != mode.Board {
		t.Fatalf("expected board mode after second ctrl+space toggle, got %s", m.active)
	}

	// Board rows visible at Phase 1 must still be present after the round-trip.
	afterCycleView := m.View()
	afterCycleIDs := extractIssueIDsFromView(afterCycleView)

	for wantID := range initialIDs {
		if !afterCycleIDs[wantID] {
			t.Fatalf("Board→Search→Board cycle: board row %q missing after round-trip\nfinal view:\n%s", wantID, afterCycleView)
		}
	}
}
