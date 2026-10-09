package search

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
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
// the rows down. A result is issuerow.Height lines, and each of them is that
// result: the title line and the line under it that carries the ID.
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
			lines := strings.Split(testui.AnsiEscapePattern.ReplaceAllString(view, ""), "\n")

			x, y := 0, 0
			for row, issue := range state.Results {
				x, y = testui.FindCell(t, view, issue.Title)
				if !strings.Contains(lines[y+1], "P2 OPN "+issue.ID) {
					t.Fatalf("result %d: the line under the title does not carry its ID:\n%s", row, view)
				}
				for line := 0; line < issuerow.Height; line++ {
					hit, ok := HitTest(state, x, y+line)
					if !ok || hit.Pane != FocusResults || hit.Row != row {
						t.Errorf("HitTest on line %d of %q (%d,%d) = %+v, %v; want result %d", line, issue.Title, x, y+line, hit, ok, row)
					}
				}
			}

			// The line under the last result's second line is no result.
			if hit, ok := HitTest(state, x, y+issuerow.Height); !ok || hit.Pane != FocusResults || hit.Row != -1 {
				t.Errorf("HitTest under the last result = %+v, %v; want the results pane with no row", hit, ok)
			}
		})
	}
}

// TestHitTestReportsNoRowForAResultThePaneClips covers a result list longer
// than the pane: the frame's bottom border is drawn where a half-drawn
// result's second line would be, and a click there is not that result.
func TestHitTestReportsNoRowForAResultThePaneClips(t *testing.T) {
	t.Parallel()

	// Height 12 leaves the results pane 7 content lines: three results and the
	// first line of the fourth.
	state := hitTestState(160, 12)
	view := Render(state)

	x, y := testui.FindCell(t, view, state.Results[3].Title)
	if hit, ok := HitTest(state, x, y); !ok || hit.Row != 3 {
		t.Fatalf("HitTest on the half-drawn result = %+v, %v; want result 3", hit, ok)
	}
	if hit, ok := HitTest(state, x, y+1); !ok || hit.Pane != FocusResults || hit.Row != -1 {
		t.Fatalf("HitTest on the bottom border = %+v, %v; want the results pane with no row", hit, ok)
	}
	if strings.Contains(testui.AnsiEscapePattern.ReplaceAllString(view, ""), state.Results[4].Title) {
		t.Fatalf("result 4 is drawn in a pane that has no room for it:\n%s", view)
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

func TestRenderBandsTheSelectedAndTheHoveredResult(t *testing.T) {
	testui.ForceTrueColor(t)

	state := hitTestState(160, 24)
	state.Hover = &Hit{Pane: FocusResults, Row: 3}
	view := Render(state)

	selected, hovered := testui.RowBand(t, view, "hit-00"), testui.RowBand(t, view, "hit-03")
	if selected == "" || hovered == "" || selected == hovered {
		t.Fatalf("bands: selected %q, hovered %q; want two different backgrounds", selected, hovered)
	}
	if plain := testui.RowBand(t, view, "hit-02"); plain != "" {
		t.Fatalf("a plain result carries the band %q", plain)
	}
}

// TestHitTestFindsEveryDrawnResultInAScrolledPane scrolls a list longer than
// the pane through every offset and asks the frame where each result is.
func TestHitTestFindsEveryDrawnResultInAScrolledPane(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		width int
		query string
	}{
		{name: "wide", width: 160, query: "hit"},
		{name: "narrow with banner", width: 90, query: "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := hitTestState(tc.width, 12)
			state.Query = tc.query
			for offset := range state.Results {
				state.ScrollOffset = offset
				state.SelectedID = state.Results[offset].ID
				view := Render(state)
				plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")

				for row, issue := range state.Results {
					// The preview repeats the selected title; a result row leads with its type.
					drawn := "T " + issue.Title
					if row < offset {
						if strings.Contains(plain, drawn) {
							t.Fatalf("offset %d: result %d above the window is drawn:\n%s", offset, row, view)
						}
						continue
					}
					if !strings.Contains(plain, drawn) {
						continue
					}
					x, y := testui.FindCell(t, view, drawn)
					if hit, ok := HitTest(state, x, y); !ok || hit.Pane != FocusResults || hit.Row != row {
						t.Errorf("offset %d: HitTest on %q (%d,%d) = %+v, %v; want result %d", offset, drawn, x, y, hit, ok, row)
					}
				}
				if !strings.Contains(plain, "T "+state.Results[offset].Title) {
					t.Fatalf("offset %d: the first result of the window is not drawn:\n%s", offset, view)
				}
			}
		})
	}
}

// TestResultsHeaderSaysWhenThePaneClipsTheLoadedResults reads the header at
// each rail width: the loaded count alone when every result is drawn, and the
// drawn count before it when not, in a frame the header never widens.
func TestResultsHeaderSaysWhenThePaneClipsTheLoadedResults(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		width, height int
		offset        int
		want          string
	}{
		{name: "fits", width: 160, height: 24, want: "─ 5 capped · open ─╮"},
		{name: "clipped", width: 160, height: 12, want: " 3 of 5 capped · open ─╮"},
		{name: "scrolled to the end", width: 160, height: 12, offset: 3, want: " 2 of 5 capped · open ─╮"},
		{name: "clipped in the 80-column rail", width: 80, height: 12, want: " 3/5 capped · open ─╮"},
		{name: "clipped in the narrowest rail", width: 60, height: 12, want: " 3/5 capped · open ─╮"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			state := hitTestState(tc.width, tc.height)
			state.ScrollOffset = tc.offset
			state.SelectedID = state.Results[tc.offset].ID
			state.Metadata = domain.SearchResultMetadata{ReturnedCount: 5, Completeness: domain.SearchResultCompletenessMaybeMore}
			view := Render(state)
			lines := strings.Split(testui.AnsiEscapePattern.ReplaceAllString(view, ""), "\n")

			_, y := testui.FindCell(t, view, "Results ─")
			if !strings.Contains(lines[y], tc.want) {
				t.Errorf("header = %q, want it to hold %q", lines[y], tc.want)
			}
			if got, want := lipgloss.Width(lines[y]), lipgloss.Width(lines[y+1]); got != want {
				t.Errorf("the header line is %d cells and the line under it %d:\n%s", got, want, view)
			}
		})
	}
}

// TestRenderResultsContentStopsAtTheLastRowThePaneHasALineFor: a search loads
// far more results than the pane draws, and the rows the box would cut away
// are not rendered. The row with a line for its title only still is.
func TestRenderResultsContentStopsAtTheLastRowThePaneHasALineFor(t *testing.T) {
	t.Parallel()

	state := hitTestState(160, 24)
	state.ScrollOffset = 1
	for _, tc := range []struct {
		paneHeight, rows int
	}{
		{paneHeight: 6, rows: 2}, // four lines inside
		{paneHeight: 7, rows: 3}, // five: two rows and the title of a third
	} {
		lines := renderResultsContent(state, 60, tc.paneHeight)
		if len(lines) != tc.rows*issuerow.Height {
			t.Fatalf("pane of %d: rendered %d lines, want the %d of %d rows", tc.paneHeight, len(lines), tc.rows*issuerow.Height, tc.rows)
		}
		last := testui.AnsiEscapePattern.ReplaceAllString(lines[len(lines)-issuerow.Height], "")
		if want := fmt.Sprintf("hit-%02d", tc.rows); !strings.Contains(last, want) {
			t.Fatalf("pane of %d: the last row rendered is %q, want %s", tc.paneHeight, last, want)
		}
	}
}
