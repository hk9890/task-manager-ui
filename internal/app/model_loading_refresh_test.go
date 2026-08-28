package app

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
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

// TestSpinnerTickRunsOnlyWhileSomethingIsLoading pins that an idle app draws no
// frames.
//
// The tick was armed in Init and re-armed unconditionally, so View() ran ten
// times a second for the life of the process — every frame re-rendering the
// issue markdown from scratch, and every frame after the first byte-identical
// and discarded by Bubble Tea's diff.
func TestSpinnerTickRunsOnlyWhileSomethingIsLoading(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	ticks := 0
	m.scheduleSpinnerTick = func() tea.Cmd {
		ticks++
		return nil
	}

	// Startup: the board is loading, so the spinner must run.
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 40}})
	if ticks == 0 {
		t.Fatal("no spinner tick was armed while the board was loading")
	}
	if !m.spinnerTicking {
		t.Fatal("spinnerTicking is false while a tick is outstanding")
	}

	m = applyMessages(t, m, runBatch(m.Init()))
	if len(m.loadingStates()) != 0 {
		t.Fatalf("setup: expected an idle app after the initial load, loading=%v", m.loadingStates())
	}

	// Idle: the outstanding tick fires once more and is not re-armed.
	armed := ticks
	m = applyMessages(t, m, []tea.Msg{loading.TickMsg{}})
	if ticks != armed {
		t.Errorf("the spinner re-armed itself while nothing was loading: %d new ticks", ticks-armed)
	}
	if m.spinnerTicking {
		t.Error("spinnerTicking is still set after the last tick fired")
	}

	// Any further message must not arm it either.
	m = applyMessages(t, m, []tea.Msg{tea.KeyMsg{Type: tea.KeyDown}})
	if ticks != armed {
		t.Errorf("an idle key press armed %d spinner ticks", ticks-armed)
	}
}
