package app

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// extractIssueIDsFromView returns the set of issue IDs visible in a rendered
// view string by scanning for taskmgr-style ID tokens (e.g. "tm-1", "tm-42").
func extractIssueIDsFromView(rendered string) map[string]bool {
	re := regexp.MustCompile(`\btm-\d+\b`)
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

// stripSpinnerGlyphs removes all spinner frame glyphs from a string so that
// plain-text comparisons are not confused by the header braille glyph changing
// independently of skeleton color cycling.
func stripSpinnerGlyphs(s string) string {
	for _, r := range loading.SpinnerFrames {
		s = strings.ReplaceAll(s, string(r), "")
	}
	return s
}

// TestSkeletonPhasePulse verifies that after enough loading.TickMsg dispatches
// the rendered View string changes (skeleton phase advances) while the
// ANSI-stripped, spinner-stripped plain text remains identical when a skeleton
// is visible.
//
// The test forces the board into a cold-start loading state (no data), then
// advances the spinner frame past the phase boundary (frame 0→4) and compares
// View() output before and after. ANSI codes change but the printable ▓ glyphs
// do not; only the header braille spinner glyph changes (handled by stripping).
func TestSkeletonPhasePulse(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	// Disable real tick schedulers so we control timing.
	withSpinnerTickScheduler(t, func() tea.Cmd { return nil })
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	// Empty repository — board stays in loading=true, cold-start.
	gw := newTestRepository()

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	// Init model but do NOT drain the batch — board stays loading=true, rows empty.
	m := mustNewModel(t, services)
	m.Init() // fire cmds but don't apply responses

	// Ensure we are on the board surface.
	if m.active != mode.Board {
		t.Fatalf("expected board active after init, got %s", m.active)
	}

	// Force spinner frame to 0 (phase 0).
	m.spinnerFrame = 0
	viewPhase0 := m.View()
	// Strip ANSI codes and spinner glyphs before comparing structure.
	plainPhase0 := stripSpinnerGlyphs(testui.AnsiEscapePattern.ReplaceAllString(viewPhase0, ""))

	// Advance spinner by 4 ticks to move from phase 0 to phase 1.
	// Each loading.TickMsg increments spinnerFrame by 1; phase boundary is frame/4.
	for i := 0; i < 4; i++ {
		next, _ := m.Update(loading.TickMsg{})
		m = next.(Model)
	}

	// spinnerFrame must now be 4 → phase 1.
	if m.spinnerFrame != 4 {
		t.Fatalf("expected spinnerFrame=4 after 4 ticks, got %d", m.spinnerFrame)
	}

	viewPhase1 := m.View()
	plainPhase1 := stripSpinnerGlyphs(testui.AnsiEscapePattern.ReplaceAllString(viewPhase1, ""))

	// Plain text (minus spinner glyph and ANSI codes) must be identical —
	// skeleton ▓ shape does not change between phases.
	if plainPhase0 != plainPhase1 {
		t.Fatalf("plain text differs between phase 0 and phase 1 — skeleton shape changed unexpectedly\nphase0: %q\nphase1: %q", plainPhase0, plainPhase1)
	}

	// Styled output must differ — skeleton color changed.
	if viewPhase0 == viewPhase1 {
		t.Fatalf("View() output unchanged after 4 TickMsg dispatches — skeleton phase pulse not working\nview: %q", viewPhase0)
	}
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

	// Configure the repository with known, distinguishable issue IDs.
	gw := newTestRepository()
	gw.seedReady("tm-10", "Ready issue alpha", "task", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-11", Title: "Blocked issue beta", Status: "blocked", Priority: 2})
	gw.seedInProgress("tm-12", "In Progress gamma", "task", 1)
	// Seed a search result so the search mode body renders something.
	gw.seedSearchResult(memoryrepo.Issue{ID: "tm-20", Title: "Search result delta", Status: "open", Priority: 1})

	services, err := NewServices(gw, config.Default(), t.TempDir())
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

	for _, wantID := range []string{"tm-10", "tm-11", "tm-12"} {
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
	// cmd contains the pending repository fetch commands — we capture them
	// without running them yet, so the board is "in flight".
	next, refreshCmd := m.Update(refreshTickMsg{})
	m = next.(Model)

	// Board must now be loading (in-flight).
	if !m.boardIsLoading() {
		t.Fatalf("expected board to be loading after refreshTickMsg with dirty surface")
	}

	// Capture View() BEFORE draining the in-flight repository results.
	// Stale issue IDs must still be visible — NOT replaced by skeleton or blank.
	inFlightView := m.View()
	inFlightIDs := extractIssueIDsFromView(inFlightView)

	for _, wantID := range []string{"tm-10", "tm-11", "tm-12"} {
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

	// Now drain the in-flight board refresh (repository responds with same data).
	m = applyMessages(t, m, runBatch(refreshCmd))

	if m.boardIsLoading() {
		t.Fatalf("expected board to have settled after draining refresh repository results")
	}

	// Spinner glyph must be gone once all surfaces are idle.
	settledView := m.View()
	if containsAnySpinnerGlyph(settledView) {
		t.Fatalf("spinner glyph still present in View() after board reload settled\nview:\n%s", settledView)
	}

	// Board issue IDs must still be present after reload.
	settledIDs := extractIssueIDsFromView(settledView)
	for _, wantID := range []string{"tm-10", "tm-11", "tm-12"} {
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

// TestSkeletonPhaseCyclesThroughAllShades verifies that within 4×len(shades)
// consecutive loading.TickMsg dispatches, the skeleton phase visits every
// distinct shade index (0 through len(SkeletonShades)-1). This pins the
// animation contract: the dim foreground cycles through all SkeletonShades
// values during a refresh, not just one or two.
func TestSkeletonPhaseCyclesThroughAllShades(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	// Disable real tick schedulers so we control timing.
	withSpinnerTickScheduler(t, func() tea.Cmd { return nil })
	withRefreshTickScheduler(t, func() tea.Cmd { return nil })

	// Empty repository — board stays in loading=true, cold-start.
	gw := newTestRepository()

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m.Init()
	m.spinnerFrame = 0

	// Each phase step requires 4 TickMsgs (loading.SkeletonPhase = frame/4).
	// Collect one View() per phase boundary (every 4 ticks).
	numShades := len(styles.SkeletonShades)
	views := make([]string, 0, numShades)
	for tick := 0; tick < 4*numShades; tick++ {
		next, _ := m.Update(loading.TickMsg{})
		m = next.(Model)
		if (tick+1)%4 == 0 {
			views = append(views, m.View())
		}
	}

	// Each phase boundary must produce a distinct styled output.
	// Strip spinner glyphs (they change independently of phase) before comparing.
	seen := make(map[string]bool, numShades)
	for i, v := range views {
		stripped := stripSpinnerGlyphs(v)
		if seen[stripped] {
			t.Fatalf("phase %d produced a duplicate styled view — phase did not cycle through all %d shades", i, numShades)
		}
		seen[stripped] = true
	}
	if len(seen) != numShades {
		t.Fatalf("expected %d distinct phase views, got %d", numShades, len(seen))
	}
}
