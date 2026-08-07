package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/mode/detail"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
)

// countColumnTopLines returns the number of lines in the rendered view that
// contain at least one column-top corner character (╭). Each such line
// represents one "row" of column tops. A correct board render with all columns
// visible has exactly one such line.
func countColumnTopLines(view string) int {
	count := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "╭") {
			count++
		}
	}
	return count
}

// assertExactlyOneColumnTopLine fails if the view does not have exactly 1 line
// containing column-top corner characters.
func assertExactlyOneColumnTopLine(t *testing.T, label, view string) {
	t.Helper()
	got := countColumnTopLines(view)
	if got != 1 {
		t.Errorf("%s: expected exactly 1 column-top line, got %d — frame stacking or missing columns detected\nview:\n%s",
			label, got, view)
	}
}

// newRegressionServices creates services with a repository that has all 4 board
// columns populated and a non-zero closed count.
func newRegressionServices(t *testing.T) Services {
	t.Helper()
	repo := memoryrepo.New()
	// Ready issue: open with no deps → shows in Ready column.
	repo.Seed(memoryrepo.Issue{ID: "reg-1", Title: "Ready issue", Status: "open", Priority: 1})
	// Blocked issue (stored): status=blocked → shows in Not Ready column.
	repo.Seed(memoryrepo.Issue{ID: "reg-2", Title: "Blocked issue", Status: "blocked", Priority: 2})
	// In-progress issue.
	repo.Seed(memoryrepo.Issue{ID: "reg-3", Title: "In Progress", Status: "in_progress", Priority: 1})
	// Closed issues — seed several so ClosedTotal is non-zero.
	closedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 56; i++ {
		id := fmt.Sprintf("reg-closed-%d", i)
		repo.Seed(memoryrepo.Issue{ID: id, Title: "Closed issue", Status: "closed"})
		repo.SeedClosed(id, closedAt, "done")
	}

	services, err := NewServices(repo, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	return services
}

// debugColumnTopReport returns a human-readable report of which lines contain
// column-top corners, useful for diagnosing doubled-header failures.
func debugColumnTopReport(view string) string {
	var b strings.Builder
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "╭") {
			fmt.Fprintf(&b, "  line %d contains ╭: %s\n", i, line)
		}
	}
	return b.String()
}

// TestViewReturnsEmptyBeforeWindowSizeMsg verifies that View() returns an empty
// string before any WindowSizeMsg has been received. This prevents Bubble Tea
// from producing a short default-size first frame that the renderer cannot
// fully overwrite when the taller post-resize frame is produced.
func TestViewReturnsEmptyBeforeWindowSizeMsg(t *testing.T) {

	services := newRegressionServices(t)
	// Use NewModelWithOptions directly (NOT mustNewModelWithOptions) so that
	// sizeKnown stays at its zero value false. Install no-op schedulers to
	// prevent real tea.Tick commands from causing an infinite applyMessages loop.
	m, err := NewModelWithOptions(services, RuntimeOptions{DisableAutoRefresh: true})
	if err != nil {
		t.Fatalf("NewModelWithOptions: %v", err)
	}
	m.scheduleRefreshTick = func() tea.Cmd { return nil }
	m.scheduleToastDismiss = func(_ time.Duration, _ int) tea.Cmd { return nil }
	m.scheduleSpinnerTick = func() tea.Cmd { return nil }

	// Before any WindowSizeMsg, View() must return empty string.
	got := m.View()
	if got != "" {
		t.Errorf("expected View() to return empty string before WindowSizeMsg, got %q (len=%d)", got, len(got))
	}

	// After Init() completes (repository responses drained), still no WindowSizeMsg
	// has arrived — View() must still return empty.
	m = applyMessages(t, m, runBatch(m.Init()))
	got = m.View()
	if got != "" {
		t.Errorf("expected View() to return empty string after init but before WindowSizeMsg, got %q (len=%d)", got, len(got))
	}

	// After WindowSizeMsg, View() must return a non-empty string with exactly
	// one column-top line.
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 180, Height: 60}})
	got = m.View()
	if got == "" {
		t.Errorf("expected View() to return non-empty string after WindowSizeMsg")
	}
	assertExactlyOneColumnTopLine(t, "first render after WindowSizeMsg (180x60)", got)
}

// TestNoDoubledColumnHeadersAfterWindowSizeMsg is the primary regression test
// for the doubled-column-headers bug: taskmgr-ui produced TWO rows of
// column-top borders when a tall terminal sent a WindowSizeMsg after the
// initial default-size render.
//
// Sequence:
//  1. Build model, send WindowSizeMsg{180, 60} — sizeKnown becomes true
//  2. View() — assert exactly 1 column-top line
//  3. Run Init() and drain all repository responses
//  4. View() — assert exactly 1 column-top line (NOT 2)
//  5. Resize to {200, 80}
//  6. View() — assert exactly 1 column-top line
func TestNoDoubledColumnHeadersAfterWindowSizeMsg(t *testing.T) {

	services := newRegressionServices(t)
	// Use NewModelWithOptions directly so sizeKnown starts false. Install no-op
	// schedulers to prevent real tea.Tick commands from causing an infinite loop.
	m, err := NewModelWithOptions(services, RuntimeOptions{DisableAutoRefresh: true})
	if err != nil {
		t.Fatalf("NewModelWithOptions: %v", err)
	}
	m.scheduleRefreshTick = func() tea.Cmd { return nil }
	m.scheduleToastDismiss = func(_ time.Duration, _ int) tea.Cmd { return nil }
	m.scheduleSpinnerTick = func() tea.Cmd { return nil }

	// --- Step 1: send WindowSizeMsg{180, 60} — sets sizeKnown=true ---
	// Width=180 ensures all 4 columns are visible (at 120 only 3 fit).
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 180, Height: 60}})
	v := m.View()
	t.Logf("step1 (after WindowSizeMsg 180x60, before init): %d column-top lines\n%s",
		countColumnTopLines(v), debugColumnTopReport(v))
	assertExactlyOneColumnTopLine(t, "step 1: after WindowSizeMsg, before init", v)

	// --- Step 2: run Init() and drain all repository responses ---
	m = applyMessages(t, m, runBatch(m.Init()))
	v = m.View()
	t.Logf("step2 (after init+data 180x60): %d column-top lines\n%s",
		countColumnTopLines(v), debugColumnTopReport(v))
	assertExactlyOneColumnTopLine(t, "step 2: after init + data loaded (180x60)", v)

	// --- Step 3: resize to a different (wider, taller) size ---
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 200, Height: 80}})
	v = m.View()
	t.Logf("step3 (after WindowSizeMsg 200x80): %d column-top lines\n%s",
		countColumnTopLines(v), debugColumnTopReport(v))
	assertExactlyOneColumnTopLine(t, "step 3: after second WindowSizeMsg (200x80)", v)
}

// TestNoDoubledColumnHeadersPresizeDataResize verifies the pre-size→data→resize
// scenario (cold start on a tall terminal):
//  1. WindowSizeMsg arrives before Init/data
//  2. Data is loaded
//  3. Terminal is resized again
func TestNoDoubledColumnHeadersPresizeDataResize(t *testing.T) {

	services := newRegressionServices(t)
	m, err := NewModelWithOptions(services, RuntimeOptions{DisableAutoRefresh: true})
	if err != nil {
		t.Fatalf("NewModelWithOptions: %v", err)
	}
	m.scheduleRefreshTick = func() tea.Cmd { return nil }
	m.scheduleToastDismiss = func(_ time.Duration, _ int) tea.Cmd { return nil }
	m.scheduleSpinnerTick = func() tea.Cmd { return nil }

	// Send size before any data.
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 180, Height: 30}})
	v := m.View()
	t.Logf("pre-size 180x30 before data: %d column-top lines", countColumnTopLines(v))
	assertExactlyOneColumnTopLine(t, "pre-size 180x30 before data", v)

	// Load data.
	m = applyMessages(t, m, runBatch(m.Init()))
	v = m.View()
	t.Logf("after data loaded at 180x30: %d column-top lines", countColumnTopLines(v))
	assertExactlyOneColumnTopLine(t, "after data loaded at 180x30", v)

	// Resize to tall terminal — this is the failing case from the bug report.
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 180, Height: 60}})
	v = m.View()
	t.Logf("after resize to 180x60: %d column-top lines\n%s",
		countColumnTopLines(v), debugColumnTopReport(v))
	assertExactlyOneColumnTopLine(t, "after resize to 180x60", v)

	// One more resize.
	m = applyMessages(t, m, []tea.Msg{tea.WindowSizeMsg{Width: 200, Height: 80}})
	v = m.View()
	t.Logf("after resize to 200x80: %d column-top lines", countColumnTopLines(v))
	assertExactlyOneColumnTopLine(t, "after resize to 200x80", v)
}

func TestModelShowModeSwitcherHelpControlsFooterVisibility(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	cfg := config.Default()
	cfg.UI.ShowModeSwitcherHelp = false

	services, err := NewServices(gw, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if footer := m.renderFooter(); strings.TrimSpace(footer) != "" {
		t.Fatalf("expected footer help hidden when ShowModeSwitcherHelp is false, got:\n%s", footer)
	}
}

func TestModelDetailModeRendersConfigurableFooterHelp(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-9", "Ninth", "task", 2)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: "Ninth detail",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	view := m.View()
	if !strings.Contains(view, "Detail: j/k scroll") || !strings.Contains(view, "home/end bounds") {
		t.Fatalf("expected detail footer help to include configurable detail bindings, got:\n%s", view)
	}
}

func TestModelWideBoardViewPrioritizesBoardAndResponsiveColumns(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("task-manager-ui-yze.4.2", "Implement create update close and comment actions in the app", "task", 1, func(iss *memoryrepo.Issue) {
		iss.Assignee = "alice"
		iss.Labels = []string{"ui", "shell"}
	})
	gw.seedIssueSummary(domain.IssueSummary{ID: "task-manager-ui-yze.4.5", Title: "Add editor and launcher integration tests", Status: "blocked", Type: "task", Priority: 1})
	gw.seedInProgress("task-manager-ui-yze.4.3", "Implement launcher framework with issue-context interpolation", "task", 1)
	gw.seedIssueDetail(domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       "task-manager-ui-yze.4.2",
			Title:    "Implement create update close and comment actions in the app",
			Status:   "open",
			Type:     "task",
			Priority: 1,
			Assignee: "alice",
			Labels:   []string{"ui", "shell"},
		},
		Description: "Show selected issue context clearly in browse mode.",
		BlockedBy:   []domain.IssueReference{{ID: "tm-9", Title: "Upstream migration"}},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 160
	m.height = 42
	m = applyMessages(t, m, runBatch(m.Init()))

	view := m.View()
	if strings.Contains(view, "Selected Issue") {
		t.Fatalf("expected no selected issue sidebar in board view, got:\n%s", view)
	}
	if strings.Contains(view, "Title:") || strings.Contains(view, "Description:") {
		t.Fatalf("expected full detail fields to stay in dedicated detail mode, got:\n%s", view)
	}
	if !strings.Contains(view, "Default") {
		t.Fatalf("expected board header in wide view, got:\n%s", view)
	}
	if !strings.Contains(view, "Implement create update") {
		t.Fatalf("expected readable board row title text in wide view, got:\n%s", view)
	}
}

func TestModelBoardShellUsesSingleLineHeaderAndFooterHelpAt120Cols(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-3", Title: "Blocked", Status: "blocked", Type: "bug", Priority: 0})
	gw.seedInProgress("tm-2", "In progress", "task", 2)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	header := m.renderHeader()
	if strings.Contains(header, "\n") {
		t.Fatalf("expected single-line header, got:\n%s", header)
	}
	if strings.Contains(header, "Detail") {
		t.Fatalf("expected detail to be contextual and absent from top tabs, got:\n%s", header)
	}

	footer := m.renderFooter()
	if !strings.Contains(footer, "ctrl+space search") {
		t.Fatalf("expected board footer help with ctrl+space hint, got:\n%s", footer)
	}
}

func TestModelBoardDetailBoardRoundTripPreservesLayoutAndFocus(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "tm-3", Title: "Blocked now", Status: "blocked", Type: "bug", Priority: 0})
	gw.seedInProgress("tm-2", "In progress one", "task", 1)
	// Pre-seed detail for both issues used during the round-trip.
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: "detail for ready issue",
	})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-2", Title: "In progress one", Status: "in_progress", Type: "task", Priority: 1},
		Description: "detail for in-progress issue",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("expected board selection tm-2 before round-trip, got %q", got)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected detail mode after enter, got %s", m.active)
	}
	detailView := m.View()
	if !strings.Contains(detailView, "In progress one") {
		t.Fatalf("expected dedicated detail layout with selected issue content, got:\n%s", detailView)
	}
	if strings.Contains(detailView, "Issue Detail") {
		t.Fatalf("expected dedicated detail layout without extra shell wrapper heading, got:\n%s", detailView)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Board {
		t.Fatalf("expected board mode after esc from detail, got %s", m.active)
	}
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Fatalf("expected board selection to remain on tm-2 after round-trip, got %q", got)
	}

	boardView := m.View()
	if strings.Contains(boardView, "Selected Issue") {
		t.Fatalf("expected board layout without detail sidebar after round-trip, got:\n%s", boardView)
	}
	if !strings.Contains(boardView, "In progress one") {
		t.Fatalf("expected board row title to remain visible after round-trip, got:\n%s", boardView)
	}

	testui.AssertMatchesGoldenNormalized(t, []byte(boardView), "model_roundtrip_board_back_w120.golden")
}

func TestModelSharedWorkspaceContractUsesFullBodyHeightAcrossModes(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress one", "task", 2)
	// Seed a search result so the search mode body renders something.
	gw.seedSearchResult(memoryrepo.Issue{ID: "tm-2", Title: "In progress one", Status: "in_progress", Priority: 2})
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m.width = 120
	m.height = 34

	expectedWidth, expectedHeight := m.workspaceSize()

	m.active = mode.Board
	boardBody := m.renderBody()
	if strings.TrimSpace(boardBody) == "" {
		t.Fatal("expected non-empty board body rendering")
	}

	m.active = mode.Search
	body := m.renderBody()
	if !strings.Contains(body, "Search") {
		t.Fatalf("expected active search view rendering, got: %q", body)
	}

	longLines := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		longLines = append(longLines, fmt.Sprintf("Line %d", i))
	}
	m.active = mode.Detail
	m.detail = detail.Model{
		SelectionID: "tm-1",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-1", Title: "Issue one", Status: "open", Type: "task", Priority: 1},
			Description: strings.Join(longLines, "\n"),
		},
	}

	detailBody := m.renderBody()
	if strings.Contains(detailBody, "Issue Detail") {
		t.Fatalf("expected detail body to avoid extra boxed wrapper heading, got:\n%s", detailBody)
	}
	if got := len(strings.Split(detailBody, "\n")); got != expectedHeight {
		t.Fatalf("expected detail body height %d lines, got %d", expectedHeight, got)
	}
	if m.detailViewportWidth() != expectedWidth {
		t.Fatalf("expected detail viewport width %d, got %d", expectedWidth, m.detailViewportWidth())
	}
	if m.detailViewportHeight() != expectedHeight {
		t.Fatalf("expected detail viewport height %d, got %d", expectedHeight, m.detailViewportHeight())
	}
}

// TestModelFixtureShapedBoardCaptureGolden verifies the full board rendering at
// w120 against the embedded-fixture golden file, using fake data seeded to
// match the same bwf-1/bwf-2 fixture shape. This replaces
// TestModelEmbeddedFixtureFullBoardCaptureGolden (which used real taskmgr+fixture).
func TestModelFixtureShapedBoardCaptureGolden(t *testing.T) {
	gw := newTestRepository()
	// Match fixture shape: bwf-2 is Blocked (Not Ready lane), bwf-1 is Ready, no InProgress.
	gw.seedReady("bwf-1", "Seed fixture root task", "task", 1, func(iss *memoryrepo.Issue) {
		iss.Assignee = "alice"
		iss.Labels = []string{"fixture", "ui"}
	})
	gw.seedIssueSummary(domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0, Assignee: "bob", Labels: []string{"fixture", "blocking"}})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0, Assignee: "bob"},
		Description: "Used to validate blocked/ready and dependency reads.",
	})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bwf-1", Title: "Seed fixture root task", Status: "open", Type: "task", Priority: 1, Assignee: "alice"},
		Description: "Root task used by integration and e2e smoke tests.",
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	view := m.View()
	if strings.Contains(view, "Selected Issue") {
		t.Fatalf("expected board view without selected issue sidebar, got:\n%s", view)
	}
	if !strings.Contains(view, "bwf-1 Seed fixture roo") {
		t.Fatalf("expected fixture-shaped issue title in board capture, got:\n%s", view)
	}
	if strings.Count(view, "│") < 20 {
		t.Fatalf("expected full-height board lanes rather than floating boxes, got:\n%s", view)
	}

	testui.AssertMatchesGoldenNormalized(t, []byte(view), "model_embedded_board_w120.golden")
}

// TestModelStartupBoardLayoutSanityAndNoRuntimeErrors verifies that startup
// renders a valid board layout with no error panels. This replaces
// TestModelEmbeddedFixtureStartupLoadsBoardWithoutRepositorySectionErrors
// (which used real taskmgr+fixture).
func TestModelStartupBoardLayoutSanityAndNoRuntimeErrors(t *testing.T) {
	gw := newTestRepository()
	gw.seedReady("bwf-1", "Seed fixture root task", "task", 1)
	gw.seedIssueSummary(domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	view := m.View()
	testui.AssertStartupBoardLayoutSanity(t, view)
	testui.AssertContainsAll(t, view, "bwf-1")
	testui.AssertNoObviousRuntimeErrorPanels(t, view)
}

// TestHeaderSpinnerCellWidthInvariance asserts that headerSpinnerCell returns a
// string of identical lipgloss.Width whether or not any surface is loading.
func TestHeaderSpinnerCellWidthInvariance(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	services := Services{Repo: gw, Config: config.Default()}
	m := mustNewModel(t, services)
	// Drain Init so board loading completes and all surfaces are idle.
	m = applyMessages(t, m, runBatch(m.Init()))

	// idle model — no loading states
	idleCell := m.headerSpinnerCell()
	idleWidth := lipgloss.Width(idleCell)

	// simulate a loading state by setting detail.Loading
	m.detail.Loading = true
	loadingCell := m.headerSpinnerCell()
	loadingWidth := lipgloss.Width(loadingCell)

	if idleWidth != loadingWidth {
		t.Errorf("headerSpinnerCell width not invariant: idle=%d loading=%d (idle=%q loading=%q)",
			idleWidth, loadingWidth, idleCell, loadingCell)
	}
}

// TestHeaderSpinnerCellContainsGlyphWhenLoading asserts that the spinner cell
// contains one of the 10 pinned braille glyphs when loading is active, and
// contains none of them when idle.
func TestHeaderSpinnerCellContainsGlyphWhenLoading(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	services := Services{Repo: gw, Config: config.Default()}
	m := mustNewModel(t, services)
	// Drain Init so board loading completes and all surfaces are idle.
	m = applyMessages(t, m, runBatch(m.Init()))

	// Set a known frame that maps to a specific glyph.
	m.spinnerFrame = 0
	expectedGlyph := loading.Glyph(0)

	// loading active — use detail.Loading to avoid triggering repository calls
	m.detail.Loading = true
	loadingCell := m.headerSpinnerCell()
	if !strings.Contains(loadingCell, expectedGlyph) {
		t.Errorf("headerSpinnerCell when loading does not contain spinner glyph %q: got %q",
			expectedGlyph, loadingCell)
	}

	// verify none of the 10 glyphs appear when idle (all loading cleared)
	m.detail.Loading = false
	idleCell := m.headerSpinnerCell()
	for i, r := range loading.SpinnerFrames {
		g := string(r)
		if strings.Contains(idleCell, g) {
			t.Errorf("headerSpinnerCell when idle contains spinner glyph[%d] %q: got %q",
				i, g, idleCell)
		}
	}
}
