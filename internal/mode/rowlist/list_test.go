package rowlist

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
)

// The docs tab and the store search drive a List through their own tests.
// These cases are the branches neither of them reaches.

func stateOf(l *List, rows, height int) uiboard.State {
	issues := make([]domain.IssueSummary, 0, rows)
	for i := range rows {
		issues = append(issues, domain.IssueSummary{ID: fmt.Sprintf("tm-%02d", i), Title: fmt.Sprintf("Row %02d", i)})
	}
	return uiboard.State{
		Columns: []uiboard.Column{{
			Title:        "Rows",
			Rows:         issues,
			SelectedRow:  l.SelectedRow,
			ScrollOffset: l.ScrollOffset,
			Total:        rows,
			TotalIsExact: true,
		}},
		Width:  80,
		Height: height,
	}
}

func TestPageKeyBeforeTheFirstSizeMovesByTheDefaultWindow(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	var l List
	moved, handled := l.MoveKey(keys, tea.KeyMsg{Type: tea.KeyPgDown}, stateOf(&l, 40, 0))
	if !moved || !handled || l.SelectedRow != pageRows(0) || pageRows(0) != 10 {
		t.Fatalf("page down with no height: moved %v, handled %v, row %d; want the ten rows of the default window", moved, handled, l.SelectedRow)
	}

	if moved, handled = l.MoveKey(keys, tea.KeyMsg{Type: tea.KeyEnter}, stateOf(&l, 40, 0)); moved || handled {
		t.Fatalf("enter is no move key: moved %v, handled %v", moved, handled)
	}
}

func TestPointerOffTheRowsMovesAndMarksNothing(t *testing.T) {
	t.Parallel()

	l := List{SelectedRow: 1}
	state := stateOf(&l, 2, 20)

	// The list head is no column.
	if _, ok := uiboard.HitTest(state, 2, 0); ok {
		t.Fatal("setup: the list head reports a column")
	}
	for _, kind := range []mode.MouseKind{mode.MouseClick, mode.MouseWheelUp, mode.MouseMove} {
		if moved, open := l.Mouse(mode.MouseMsg{Kind: kind, X: 2, Y: 0}, state); moved || open || l.SelectedRow != 1 {
			t.Fatalf("kind %d on the list head: moved %v, open %v, row %d", kind, moved, open, l.SelectedRow)
		}
		if hover := l.Hover(state); hover != nil {
			t.Fatalf("kind %d on the list head marks %#v", kind, hover)
		}
	}

	// The space under the last row is the column, and no row.
	if hit, ok := uiboard.HitTest(state, 2, 12); !ok || hit.Row >= 0 {
		t.Fatalf("setup: the space under the rows reports %#v (%v)", hit, ok)
	}
	if moved, open := l.Mouse(mode.MouseMsg{Kind: mode.MouseMove, X: 2, Y: 12}, state); moved || open {
		t.Fatalf("the pointer under the rows: moved %v, open %v", moved, open)
	}
	if hover := l.Hover(state); hover != nil {
		t.Fatalf("the pointer under the rows marks %#v", hover)
	}
}

func TestSelectionOutsideTheRowsIsTheFirstRow(t *testing.T) {
	t.Parallel()

	l := List{SelectedRow: 7}
	rows := stateOf(&l, 2, 20).Columns[0].Rows
	if got := l.SelectedID(rows); got != "tm-00" {
		t.Fatalf("a selected row past the end reads %q, want the first row", got)
	}
	if got := l.SelectedID(nil); got != "" || l.Selection(nil) != nil {
		t.Fatalf("no rows read %q", got)
	}
}
