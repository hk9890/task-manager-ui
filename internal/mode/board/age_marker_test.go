package board

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// TestBoardModeScrollKeepsTheChevronVisibleAcrossAgeMarkers walks the cursor
// down a Ready column that crosses both age thresholds. The two divider rows
// take space from the issue window, so a scroll offset computed for issues
// alone would leave the selected row below the last drawn line.
func TestBoardModeScrollKeepsTheChevronVisibleAcrossAgeMarkers(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	const rowCount = 20
	ready := make([]domain.IssueSummary, rowCount)
	for i := range ready {
		// Ages step from one hour to twenty days, so the 1-day divider lands
		// after the first issue and the 1-week one after the seventh.
		age := time.Hour
		if i > 0 {
			age = time.Duration(i) * 24 * time.Hour
		}
		ready[i] = domain.IssueSummary{
			ID:        fmt.Sprintf("tm-%02d", i),
			Title:     fmt.Sprintf("Ready %02d", i),
			Status:    "open",
			Type:      "task",
			Priority:  2,
			UpdatedAt: now.Add(-age),
		}
	}

	m := newBoardModel(memoryrepo.New(fakes.FrozenClock()), resolvedBoardKeys(t))
	m.now = func() time.Time { return now }
	m.SetSize(80, 12)
	feedDashboardData(m, repository.DashboardData{ReadyExplain: domain.ReadyExplainResult{Ready: ready}})
	if m.focusedColumn != 1 {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}

	for step := 1; step < rowCount; step++ {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
		want := fmt.Sprintf("› T P2 OPN tm-%02d", step)
		if !strings.Contains(plain, want) {
			t.Fatalf("after %d presses expected %q on screen:\n%s", step, want, plain)
		}
	}

	// At the end both dividers sit above the window: every content row is an
	// issue and the header counts them all.
	plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	testui.AssertContainsAll(t, plain, "9 of 20")
	testui.AssertNotContainsAny(t, plain, "older than")
}

// TestBoardModeChevronSurvivesADividerAppearingWhileIdle: the offset is stored
// at key-press time, but View reads the clock again. An issue that crosses the
// 1-day line in between inserts a divider the offset never reserved.
func TestBoardModeChevronSurvivesADividerAppearingWhileIdle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	ready := make([]domain.IssueSummary, 12)
	for i := range ready {
		age := 2 * time.Hour
		if i >= 4 {
			age = 23*time.Hour + 40*time.Minute + time.Duration(i)*time.Minute
		}
		ready[i] = domain.IssueSummary{
			ID:        fmt.Sprintf("tm-%02d", i),
			Title:     fmt.Sprintf("Ready %02d", i),
			Status:    "open",
			Type:      "task",
			Priority:  2,
			UpdatedAt: now.Add(-age),
		}
	}

	m := newBoardModel(memoryrepo.New(fakes.FrozenClock()), resolvedBoardKeys(t))
	m.now = func() time.Time { return now }
	m.SetSize(80, 12)
	feedDashboardData(m, repository.DashboardData{ReadyExplain: domain.ReadyExplainResult{Ready: ready}})
	if m.focusedColumn != 1 {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	for range 8 {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	testui.AssertContainsAll(t, plain, "› T P2 OPN tm-08", "9 of 12")

	now = now.Add(25 * time.Minute)
	plain = testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	testui.AssertContainsAll(t, plain, "older than 1 day", "› T P2 OPN tm-08", "8 of 12")
}

// TestBoardModeTinySectionStillDrawsTheSelectedRow: two content rows cannot
// hold two stacked dividers and an issue, so the issue wins.
func TestBoardModeTinySectionStillDrawsTheSelectedRow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	ready := make([]domain.IssueSummary, 3)
	for i := range ready {
		ready[i] = domain.IssueSummary{
			ID:        fmt.Sprintf("tm-%02d", i),
			Title:     fmt.Sprintf("Ready %02d", i),
			Status:    "open",
			Type:      "task",
			Priority:  2,
			UpdatedAt: now.Add(-time.Duration(30+i) * 24 * time.Hour),
		}
	}

	m := newBoardModel(memoryrepo.New(fakes.FrozenClock()), resolvedBoardKeys(t))
	m.now = func() time.Time { return now }
	m.SetSize(80, 5)
	feedDashboardData(m, repository.DashboardData{ReadyExplain: domain.ReadyExplainResult{Ready: ready}})
	if m.focusedColumn != 1 {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	testui.AssertContainsAll(t, plain, "older than 1 week", "› T P2 OPN tm-00", "1 of 3")
	for step := 1; step < 3; step++ {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		plain = testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
		testui.AssertContainsAll(t, plain, fmt.Sprintf("› T P2 OPN tm-%02d", step))
	}
}
