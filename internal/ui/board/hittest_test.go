package board

import (
	"fmt"
	"testing"
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// hitTestState is a board whose every issue has a title that appears once, so
// a test can ask the rendered frame where the renderer put it.
func hitTestState(width, height int) State {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	column := func(name string, count int, age time.Duration) Column {
		rows := make([]domain.IssueSummary, count)
		for idx := range rows {
			rows[idx] = domain.IssueSummary{
				ID:        fmt.Sprintf("tm-%s%d", name, idx),
				Title:     fmt.Sprintf("%s-issue-%02d", name, idx),
				Type:      "task",
				Status:    "open",
				Priority:  2,
				UpdatedAt: now.Add(-age * time.Duration(idx)),
			}
		}
		return Column{Title: name, Rows: rows, SelectedRow: -1, Total: count, TotalIsExact: true, AgeMarkers: true}
	}

	return State{
		DashboardTitle: "Default",
		Columns: []Column{
			column("aa", 3, time.Hour),
			// One row a day apart: a divider lands between rows 1 and 2, and a
			// second one further down.
			column("bb", 12, 30*time.Hour),
			column("cc", 2, time.Hour),
			column("dd", 4, time.Hour),
		},
		FocusedColumn: 1,
		Width:         width,
		Height:        height,
		Now:           now,
	}
}

// TestHitTestFindsEveryIssueWhereRenderDrewIt asks the rendered frame where
// each issue is and checks HitTest answers with that issue, across the layouts
// that move rows around: clipped columns, age dividers, a scrolled window and
// a pinned error row.
func TestHitTestFindsEveryIssueWhereRenderDrewIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*State)
		width  int
		height int
		// drawn lists the issues expected on screen as column, row.
		drawn [][2]int
	}{
		{name: "all columns", width: 200, height: 24, drawn: [][2]int{{0, 0}, {0, 2}, {1, 0}, {1, 1}, {1, 2}, {1, 11}, {2, 1}, {3, 3}}},
		{name: "columns clipped around the focus", width: 110, height: 24, drawn: [][2]int{{0, 0}, {1, 5}, {2, 1}}},
		{
			name:  "scrolled window",
			width: 200, height: 10,
			mutate: func(s *State) { s.Columns[1].ScrollOffset = 6; s.Columns[1].SelectedRow = 8 },
			drawn:  [][2]int{{1, 6}, {1, 8}, {0, 1}},
		},
		{
			name:  "pinned error row",
			width: 200, height: 24,
			mutate: func(s *State) { s.Columns[1].Error = "store is locked" },
			drawn:  [][2]int{{1, 0}, {1, 4}},
		},
		{
			name:  "no title line",
			width: 200, height: 24,
			mutate: func(s *State) { s.DashboardTitle = "" },
			drawn:  [][2]int{{0, 0}, {3, 2}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := hitTestState(tc.width, tc.height)
			if tc.mutate != nil {
				tc.mutate(&state)
			}
			view := Render(state)

			for _, want := range tc.drawn {
				issue := state.Columns[want[0]].Rows[want[1]]
				x, y := testui.FindCell(t, view, issue.Title)
				hit, ok := HitTest(state, x, y)
				if !ok || hit.Column != want[0] || hit.Row != want[1] {
					t.Errorf("HitTest at %q (%d,%d) = %+v, %v; want column %d row %d", issue.Title, x, y, hit, ok, want[0], want[1])
				}
			}
		})
	}
}

// TestHitTestReportsNoIssueOffTheRows covers the cells of a column that hold
// no issue. A click there must select nothing.
func TestHitTestReportsNoIssueOffTheRows(t *testing.T) {
	t.Parallel()

	state := hitTestState(200, 24)
	state.Columns[1].Error = "store is locked"
	view := Render(state)

	for _, text := range []string{"older than 1 day", "older than 1 week", "load failed", "bb ─"} {
		x, y := testui.FindCell(t, view, text)
		hit, ok := HitTest(state, x, y)
		if !ok || hit.Column != 1 || hit.Row != -1 {
			t.Errorf("HitTest on %q (%d,%d) = %+v, %v; want column 1 with no row", text, x, y, hit, ok)
		}
	}

	// Below the last row of a short column.
	x, y := testui.FindCell(t, view, "cc-issue-01")
	if hit, ok := HitTest(state, x, y+3); !ok || hit.Column != 2 || hit.Row != -1 {
		t.Errorf("HitTest below the last row = %+v, %v; want column 2 with no row", hit, ok)
	}

	// The gap between two columns, the title line, and outside the frame.
	x, y = testui.FindCell(t, view, "aa-issue-00")
	gapX, _ := testui.FindCell(t, view, "bb ─")
	for _, cell := range [][2]int{{gapX - 4, y}, {x, 0}, {x, 24}, {-1, y}, {200, y}} {
		if hit, ok := HitTest(state, cell[0], cell[1]); ok {
			t.Errorf("HitTest at %v = %+v, want no hit", cell, hit)
		}
	}
}

func TestHitTestIgnoresSkeletonRows(t *testing.T) {
	t.Parallel()

	state := hitTestState(200, 24)
	state.Columns[0] = Column{Title: "aa", Loading: true, SelectedRow: -1}
	if hit, ok := HitTest(state, 3, 3); !ok || hit.Column != 0 || hit.Row != -1 {
		t.Fatalf("HitTest on a skeleton row = %+v, %v; want column 0 with no row", hit, ok)
	}
}

// TestRenderBandsTheSelectedAndTheHoveredRow pins the two row bands: the
// selected row's, a different one on the row under the pointer, none on any
// other row, and the selection's on a row that is both.
func TestRenderBandsTheSelectedAndTheHoveredRow(t *testing.T) {
	testui.ForceTrueColor(t)

	state := hitTestState(200, 24)
	state.Columns[1].SelectedRow = 0
	state.Hover = &Hit{Column: 0, Row: 2}
	view := Render(state)

	selected := testui.RowBand(t, view, "bb-issue-00")
	hovered := testui.RowBand(t, view, "aa-issue-02")
	if selected == "" || hovered == "" || selected == hovered {
		t.Fatalf("bands: selected %q, hovered %q; want two different backgrounds", selected, hovered)
	}
	if plain := testui.RowBand(t, view, "aa-issue-01"); plain != "" {
		t.Fatalf("a row that is neither selected nor hovered carries the band %q", plain)
	}

	state.Hover = &Hit{Column: 1, Row: 0}
	if both := testui.RowBand(t, Render(state), "bb-issue-00"); both != selected {
		t.Fatalf("a selected row under the pointer carries %q, want the selection's %q", both, selected)
	}
}
