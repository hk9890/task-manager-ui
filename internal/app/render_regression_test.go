package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
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
