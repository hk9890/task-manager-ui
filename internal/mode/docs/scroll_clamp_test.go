package docs

import (
	"fmt"
	"strings"
	"testing"

	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// selectedGutter is the gutter the renderer draws down both lines of the
// selected doc.
var selectedGutter, _ = styles.SelectionPrefix(true, false)

// assertSelectionDrawn fails unless the column draws both lines of the selected
// doc, each behind the selection gutter: the title, and the ID directly under
// it. An offset that leaves either line outside the window fails it.
func assertSelectionDrawn(t *testing.T, m *Model) {
	t.Helper()

	selection := m.currentSelection()
	if selection == nil {
		t.Fatal("assertSelectionDrawn: the column has no selection")
	}
	plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	lines := strings.Split(plain, "\n")
	for idx := 0; idx+1 < len(lines); idx++ {
		if strings.Contains(lines[idx], selectedGutter) && strings.Contains(lines[idx], selection.Issue.Title) &&
			strings.Contains(lines[idx+1], selectedGutter) && strings.Contains(lines[idx+1], selection.Issue.ID) {
			return
		}
	}
	t.Fatalf("the selected doc %s is not drawn with both of its lines:\n%s", selection.Issue.ID, plain)
}

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
	m.shown = m.issues
	m.total = 6
	m.clampSelection()

	// Six two-line docs under the two age dividers are fourteen lines, and the
	// column holds twenty-seven.
	if lines := len(m.issues)*issuerow.Height + 2; lines > m.itemCapacity() {
		t.Fatalf("setup: the %d lines of the shrunk list do not fit the %d-line column", lines, m.itemCapacity())
	}
	if m.scrollOffset != 0 {
		t.Errorf("a list shorter than the window is scrolled to %d; every row fits, so the offset must be 0", m.scrollOffset)
	}
	assertSelectionDrawn(t, m)
	plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	for _, doc := range m.issues {
		if !strings.Contains(plain, doc.Title) || !strings.Contains(plain, doc.ID) {
			t.Errorf("%s fits the column and is not drawn whole:\n%s", doc.ID, plain)
		}
	}
}

// TestClampKeepsAWindowTheOperatorScrolledTo pins the bound the clamp pulls an
// offset back to: the last window that is full, counted in issues.
//
// A row is issuerow.Height lines. The bound was the row count less
// itemCapacity(), which counts lines, so it took every offset in the second
// half of a long list for one past the end: each resize and each refresh pulled
// the window back until the selection sat on its last row.
func TestClampKeepsAWindowTheOperatorScrolledTo(t *testing.T) {
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

	// Height 30 leaves 26 lines: thirteen docs. Forty steps
	// down scroll the window to rows 28..40, five steps up keep it there.
	m := loadedModel(t, gw)
	for range 40 {
		m.moveRow(1)
	}
	for range 5 {
		m.moveRow(-1)
	}
	if m.selectedRow != 35 || m.scrollOffset != 28 {
		t.Fatalf("setup: selected row %d at offset %d, want row 35 at offset 28", m.selectedRow, m.scrollOffset)
	}

	m.SetSize(120, 30)
	if m.scrollOffset != 28 {
		t.Errorf("a resize to the same size moved the window from row 28 to row %d", m.scrollOffset)
	}
	assertSelectionDrawn(t, m)

	// Past the last full window the offset is pulled back to it: the last
	// thirteen docs, with no blank line a doc above could fill.
	m.scrollOffset = 48
	m.selectedRow = 49
	m.clampSelection()
	if m.scrollOffset != 37 {
		t.Errorf("an offset past the end was clamped to %d, want 37, the last full window", m.scrollOffset)
	}
	assertSelectionDrawn(t, m)
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

	capacity := m.itemCapacity() / issuerow.Height
	if m.selectedRow < m.scrollOffset || m.selectedRow >= m.scrollOffset+capacity {
		t.Errorf("after the resize the selected row %d is outside the window [%d,%d) — the chevron is off screen",
			m.selectedRow, m.scrollOffset, m.scrollOffset+capacity)
	}
	assertSelectionDrawn(t, m)
}
