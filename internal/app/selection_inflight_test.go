package app

// The in-flight guard in ensureDetailForCurrentSelectionCmd: what stops a
// re-selection issuing a second repository read while the first is outstanding.
// No test executed this branch before — deleting it left the whole repository
// green — so these are new scenarios rather than stronger assertions.

import (
	"testing"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// modelWithBoardSelection builds a shell whose board selection is the given
// issue, the way a SelectionChangedMsg from the board would leave it.
func modelWithBoardSelection(t *testing.T, gw *fakes.TrackedRepository, id, title string) Model {
	t.Helper()

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)
	selectBoardIssue(&m, id, title)

	return m
}

func selectBoardIssue(m *Model, id, title string) {
	m.active = mode.Board
	m.lastBrowse = mode.Board
	m.selectedByMode[mode.Board] = &mode.Selection{
		Issue: domain.IssueSummary{ID: id, Title: title, Status: "open", Type: "task"},
	}
}

// TestEnsureDetailSuppressesASecondLoadForTheIssueAlreadyLoading is the
// scenario the guard exists for: the same row re-selected while its load is
// still outstanding must not start a second one.
func TestEnsureDetailSuppressesASecondLoadForTheIssueAlreadyLoading(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "First", "task", 1)
	m := modelWithBoardSelection(t, gw, "tm-1", "First")

	if cmd := m.ensureDetailForCurrentSelectionCmd(); cmd == nil {
		t.Fatal("expected the first selection to dispatch a detail load")
	}
	if !m.detail.IsLoading() || m.detail.TargetID() != "tm-1" {
		t.Fatalf("expected a load in flight for tm-1, got loading=%v target=%q",
			m.detail.IsLoading(), m.detail.TargetID())
	}

	if cmd := m.ensureDetailForCurrentSelectionCmd(); cmd != nil {
		t.Fatal("a second load was dispatched for an issue already loading")
	}
}

// The guard is keyed on the target issue. An unkeyed version would strand the
// shell on the previous issue whenever the operator moved during a load.
func TestEnsureDetailStartsALoadWhenADifferentIssueIsInFlight(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "First", "task", 1)
	seedReady(gw, "tm-2", "Second", "task", 1)
	m := modelWithBoardSelection(t, gw, "tm-1", "First")

	if cmd := m.ensureDetailForCurrentSelectionCmd(); cmd == nil {
		t.Fatal("expected the first selection to dispatch a detail load")
	}

	selectBoardIssue(&m, "tm-2", "Second")
	if cmd := m.ensureDetailForCurrentSelectionCmd(); cmd == nil {
		t.Fatal("moving to a different issue while one is loading dispatched no load")
	}
	if got := m.detail.TargetID(); got != "tm-2" {
		t.Fatalf("detail target = %q, want tm-2", got)
	}
}
