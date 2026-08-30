package docs

import (
	"fmt"
	"testing"

	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// TestClampPullsTheWindowBackInsideAShrunkList pins the half of the clamp
// board's clampScrollOffsets has and docs did not.
//
// scroll.EnsureVisible only slides far enough to reveal the selected row, so a
// list that shrank under a scrolled offset kept that offset: the column drew its
// last row or two with everything above them unreachable until the operator
// pressed k. Board's comment says docs does this "for the same reason"; it did
// not.
func TestClampPullsTheWindowBackInsideAShrunkList(t *testing.T) {
	gw := fakes.NewTracked()
	for i := range 50 {
		gw.Memory.Seed(memoryrepo.Issue{
			ID:       fmt.Sprintf("tm-doc-%02d", i),
			Title:    fmt.Sprintf("Design note %02d", i),
			Status:   "open",
			Type:     "doc",
			Priority: 2,
		})
	}

	m := loadedModel(t, gw)
	for range 40 {
		m.moveRow(1)
	}
	if m.scrollOffset == 0 {
		t.Fatal("setup: expected the list to be scrolled")
	}

	// The list shrinks under the offset, as an auto-refresh that drops rows does.
	m.issues = m.issues[:6]
	m.total = 6
	m.clampSelection()

	capacity := m.itemCapacity()
	if maxOffset := len(m.issues) - capacity; maxOffset < 0 && m.scrollOffset != 0 {
		t.Errorf("a list shorter than the window is scrolled to %d; every row fits, so the offset must be 0", m.scrollOffset)
	}
	if m.selectedRow < m.scrollOffset || m.selectedRow >= m.scrollOffset+capacity {
		t.Errorf("selected row %d is outside the window [%d,%d)", m.selectedRow, m.scrollOffset, m.scrollOffset+capacity)
	}
}

// TestResizeKeepsTheSelectionInTheWindow pins that a resize re-derives the
// window: itemCapacity() reads the height, and SetSize did not clamp.
func TestResizeKeepsTheSelectionInTheWindow(t *testing.T) {
	gw := fakes.NewTracked()
	for i := range 50 {
		gw.Memory.Seed(memoryrepo.Issue{
			ID:       fmt.Sprintf("tm-doc-%02d", i),
			Title:    fmt.Sprintf("Design note %02d", i),
			Status:   "open",
			Type:     "doc",
			Priority: 2,
		})
	}

	m := loadedModel(t, gw)
	m.SetSize(120, 60)
	for range 40 {
		m.moveRow(1)
	}

	m.SetSize(100, 20)

	capacity := m.itemCapacity()
	if m.selectedRow < m.scrollOffset || m.selectedRow >= m.scrollOffset+capacity {
		t.Errorf("after the resize the selected row %d is outside the window [%d,%d) — the chevron is off screen",
			m.selectedRow, m.scrollOffset, m.scrollOffset+capacity)
	}
}
