package search

import (
	"fmt"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func hitTestState(width, height int) State {
	results := make([]domain.IssueSummary, 5)
	for idx := range results {
		results[idx] = domain.IssueSummary{
			ID:       fmt.Sprintf("tm-%d", idx),
			Title:    fmt.Sprintf("hit-%02d", idx),
			Type:     "task",
			Status:   "open",
			Priority: 2,
		}
	}
	// The preview repeats the selected result's title; rename it there so every
	// result title is drawn once.
	preview := results[0]
	preview.Title = "preview-title"
	return State{
		Query:        "hit",
		AppliedQuery: "hit",
		Focus:        FocusResults,
		Results:      results,
		SelectedID:   "tm-0",
		SelectedDetail: domain.IssueDetail{
			Summary:     preview,
			Description: "preview-body",
		},
		Width:  width,
		Height: height,
	}
}

// TestHitTestFindsEveryResultWhereRenderDrewIt asks the rendered frame where
// each result is, in both layouts and with the stale-results banner pushing
// the rows down.
func TestHitTestFindsEveryResultWhereRenderDrewIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		width  int
		mutate func(*State)
	}{
		{name: "wide", width: 160},
		{name: "narrow", width: 90},
		{name: "wide with banner", width: 160, mutate: func(s *State) { s.Query = "other" }},
		{name: "narrow with error banner", width: 90, mutate: func(s *State) { s.Error = "store is locked" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := hitTestState(tc.width, 24)
			if tc.mutate != nil {
				tc.mutate(&state)
			}
			view := Render(state)

			for row, issue := range state.Results {
				x, y := testui.FindCell(t, view, issue.Title)
				hit, ok := HitTest(state, x, y)
				if !ok || hit.Pane != FocusResults || hit.Row != row {
					t.Errorf("HitTest at %q (%d,%d) = %+v, %v; want result %d", issue.Title, x, y, hit, ok, row)
				}
			}
		})
	}
}

func TestHitTestNamesThePaneUnderThePointer(t *testing.T) {
	t.Parallel()

	for _, width := range []int{160, 90} {
		state := hitTestState(width, 24)
		state.Query = "other"
		view := Render(state)

		for text, want := range map[string]FocusPane{
			"other":        FocusQuery,
			"Results ─":    FocusResults,
			"are stale":    FocusResults,
			"preview-body": FocusContent,
			"Core":         FocusMetadata,
		} {
			x, y := testui.FindCell(t, view, text)
			hit, ok := HitTest(state, x, y)
			if !ok || hit.Pane != want || hit.Row != -1 {
				t.Errorf("width %d: HitTest on %q (%d,%d) = %+v, %v; want pane %d with no row", width, text, x, y, hit, ok, want)
			}
		}

		// The gap between the rail and the content pane.
		x, y := testui.FindCell(t, view, "Content ─")
		if hit, ok := HitTest(state, x-4, y); ok {
			t.Errorf("width %d: HitTest in the column gap = %+v, want no hit", width, hit)
		}
	}
}

func TestHitTestIgnoresRowsThatAreNotResults(t *testing.T) {
	t.Parallel()

	state := hitTestState(160, 24)
	state.Results = nil
	state.Loading = true
	view := Render(state)
	_, y := testui.FindCell(t, view, "Results ─")
	if hit, ok := HitTest(state, 3, y+1); !ok || hit.Row != -1 {
		t.Fatalf("HitTest on a skeleton row = %+v, %v; want the results pane with no row", hit, ok)
	}
}

func TestRenderMarksTheHoveredResult(t *testing.T) {
	t.Parallel()

	state := hitTestState(160, 24)
	state.SelectedID = ""
	_, y := testui.FindCell(t, Render(state), "hit-03")
	state.Hover = &Hit{Pane: FocusResults, Row: 3}
	if _, hoverY := testui.FindCell(t, Render(state), "›"); hoverY != y {
		t.Fatalf("hover chevron drawn on row %d, want row %d", hoverY, y)
	}
}
