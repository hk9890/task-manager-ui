package board

import (
	"errors"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// TestFailedManualReloadReportsTheEmptiedSelection pins that a failed reload
// tells the shell what the board is now showing.
//
// A manual reload clears the columns and the selection maps before the load, so
// on failure the board draws four empty columns and currentSelection() is nil.
// composeFailed returned nil instead of a SelectionChangedMsg, so the shell kept
// the pre-reload issue: the header still named it, and e, x, a and u still acted
// on it although it was neither on screen nor refreshed.
func TestFailedManualReloadReportsTheEmptiedSelection(t *testing.T) {
	t.Parallel()

	m := newSettledBoardModel(t, newPopulatedRepo())
	if m.currentSelection() == nil {
		t.Fatal("setup: expected a selection after settling")
	}

	// Press r, then fail the load it dispatched.
	if cmd := m.Update(reloadKeyMsg()); cmd == nil {
		t.Fatal("expected a Dashboard cmd from the reload key")
	}
	cmd := m.Update(dashboardLoadedMsg{err: errors.New("store busy")})

	if m.currentSelection() != nil {
		t.Fatalf("expected no selection after a failed manual reload, got %+v", m.currentSelection())
	}

	msgs := testui.DrainCmd(cmd)
	var reported *mode.SelectionChangedMsg
	for _, msg := range msgs {
		if changed, ok := msg.(mode.SelectionChangedMsg); ok {
			reported = &changed
		}
	}
	if reported == nil {
		t.Fatalf("failed reload emitted no SelectionChangedMsg; the shell keeps a selection the board no longer draws (got %d messages)", len(msgs))
	}
	if reported.Selection != nil {
		t.Errorf("expected a nil Selection, got %+v", reported.Selection)
	}
	if reported.Mode != mode.Board {
		t.Errorf("expected the message to come from the board, got %q", reported.Mode)
	}
}

// TestResizeKeepsTheSelectedRowInTheScrollWindow pins the clamp on resize.
//
// sectionItemCapacity() derives the scroll window from the terminal height, so
// shrinking the terminal shrinks the window under an offset that was valid for
// the old one. clampScrollOffsets existed but nothing called it from SetSize, so
// the selected row and its chevron sat below the last drawn row until the
// operator pressed j or k.
func TestResizeKeepsTheSelectedRowInTheScrollWindow(t *testing.T) {
	t.Parallel()

	repo := newPopulatedRepo()
	for i := range 40 {
		repo.Seed(memoryrepo.Issue{
			ID:       fmt.Sprintf("tm-ready-%02d", i),
			Title:    fmt.Sprintf("Ready work %02d", i),
			Status:   "open",
			Priority: 2,
		})
	}

	m := newSettledBoardModel(t, repo)
	m.SetSize(200, 60)

	// Focus the column the seeded issues landed in, then walk down it so the
	// selection is well past a small window.
	for i := range m.columns {
		if len(m.columns[i].issues) > 30 {
			m.focusedColumn = i
		}
	}
	for range 25 {
		m.moveRow(1)
	}

	selected := m.selectedRow[m.focusedColumn]
	if selected < 20 {
		t.Fatalf("setup: expected the cursor well down the column, got row %d", selected)
	}

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})

	offset := m.scrollOffset[m.focusedColumn]
	window := m.sectionItemCapacity()
	if selected := m.selectedRow[m.focusedColumn]; selected < offset || selected >= offset+window {
		t.Errorf("after the resize the selected row %d is outside the window [%d,%d) — the chevron is off screen",
			selected, offset, offset+window)
	}
}

// TestStaleLoadMorePageDoesNotReleaseTheLatch pins the ordering inside
// applyLoadMoreClosed: a page a reload superseded is dropped, and the in-flight
// latch it would have released is not its own to release — startReload cleared
// it, and a dispatch made after that reload may be holding it.
func TestStaleLoadMorePageDoesNotReleaseTheLatch(t *testing.T) {
	t.Parallel()

	m := newSettledBoardModel(t, newPopulatedRepo())

	// A load-more is outstanding at the offset the column currently ends at.
	m.doneLoadedCount = 30
	m.doneLoadInFlight = true

	// A page from before a reload arrives: its offset no longer matches.
	if cmd := m.applyLoadMoreClosed(loadMoreClosedDoneMsg{offset: 10}); cmd != nil {
		t.Errorf("a stale page should produce no command, got one")
	}
	if !m.doneLoadInFlight {
		t.Error("a stale page released the latch of the load that is still in flight")
	}

	// The current page does release it.
	m.applyLoadMoreClosed(loadMoreClosedDoneMsg{offset: 30})
	if m.doneLoadInFlight {
		t.Error("the current page did not release the latch")
	}
}
