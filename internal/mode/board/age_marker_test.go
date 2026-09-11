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
}
