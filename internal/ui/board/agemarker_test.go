package board

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

const day = 24 * time.Hour

var markerNow = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

// agedRows builds rows whose last change lies the given durations before
// markerNow, in the order given: the caller keeps them descending.
func agedRows(ages ...time.Duration) []domain.IssueSummary {
	rows := make([]domain.IssueSummary, len(ages))
	for i, age := range ages {
		rows[i] = domain.IssueSummary{
			ID:        fmt.Sprintf("tm-%02d", i),
			Title:     fmt.Sprintf("Issue %02d", i),
			Status:    "open",
			Type:      "task",
			Priority:  2,
			UpdatedAt: markerNow.Add(-age),
		}
	}
	return rows
}

// selectedLines is the two lines the renderer draws for issue idx of agedRows
// when it is selected: the gutter runs down both.
func selectedLines(idx int) []string {
	return []string{
		fmt.Sprintf("%sT Issue %02d", selectedGutter, idx),
		fmt.Sprintf("%s  P2 OPN tm-%02d", selectedGutter, idx),
	}
}

func TestAgeMarkersPlaceOneDividerPerCrossedThreshold(t *testing.T) {
	t.Parallel()

	undated := agedRows(time.Hour, 8*day)
	undated = append(undated, domain.IssueSummary{ID: "tm-undated"})

	cases := []struct {
		name string
		rows []domain.IssueSummary
		want []ageMarker
	}{
		{name: "all fresh", rows: agedRows(time.Hour, 23*time.Hour), want: nil},
		{name: "crosses a day only", rows: agedRows(time.Hour, 2*day),
			want: []ageMarker{{threshold: ageThresholds[0], Before: 1, Count: 1}}},
		{name: "crosses both", rows: agedRows(time.Hour, 2*day, 3*day, 8*day, 30*day),
			want: []ageMarker{{threshold: ageThresholds[0], Before: 1, Count: 4}, {threshold: ageThresholds[1], Before: 3, Count: 2}}},
		{name: "all stale stacks both at the top", rows: agedRows(8*day, 9*day),
			want: []ageMarker{{threshold: ageThresholds[0], Before: 0, Count: 2}, {threshold: ageThresholds[1], Before: 0, Count: 2}}},
		{name: "no change date is not old", rows: []domain.IssueSummary{{ID: "tm-0"}}, want: nil},
		{name: "no change date is not counted below a divider", rows: undated,
			want: []ageMarker{{threshold: ageThresholds[0], Before: 1, Count: 1}, {threshold: ageThresholds[1], Before: 1, Count: 1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ageMarkers(tc.rows, markerNow)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d markers %+v, want %d %+v", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("marker %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestRenderAgeMarkerFillsTheWidthAndDegradesLabelThenCount(t *testing.T) {
	t.Parallel()

	marker := ageMarker{threshold: ageThresholds[1], Before: 2, Count: 12}
	cases := []struct {
		width int
		want  string
	}{
		{width: 40, want: "─── older than 1 week " + strings.Repeat("─", 11) + " 12 ───"},
		{width: 26, want: "─── > 1 week ────── 12 ───"},
	}
	for _, tc := range cases {
		got := renderAgeMarker(marker, tc.width, false)
		if got != tc.want {
			t.Errorf("width %d: got %q, want %q", tc.width, got, tc.want)
		}
	}
	for width := 6; width <= 60; width++ {
		if w := lipgloss.Width(renderAgeMarker(marker, width, true)); w > width {
			t.Errorf("styled width %d: rendered %d cells", width, w)
		}
	}
}

func renderAged(rows []domain.IssueSummary, offset, selected, height int, markers bool, now time.Time) string {
	view := Render(State{
		Columns: []Column{{
			Title:        "Ready",
			Rows:         rows,
			SelectedRow:  selected,
			ScrollOffset: offset,
			Total:        len(rows),
			TotalIsExact: true,
			AgeMarkers:   markers,
		}},
		Width:  60,
		Height: height,
		Now:    now,
	})
	return testui.AnsiEscapePattern.ReplaceAllString(view, "")
}

func TestRenderWindowsOnIssueIndexAcrossDividers(t *testing.T) {
	t.Parallel()

	rows := agedRows(time.Hour, 2*day, 3*day, 8*day, 9*day, 10*day)
	const height = 11 // 7 content rows: a divider and three two-line issues

	// Scrolled to the fourth issue: the divider drawn directly above it opens
	// the window, the selected row is on screen, and the header counts issues
	// only — the divider is not one of the "N of M".
	plain := renderAged(rows, 3, 3, height, true, markerNow)
	testui.AssertContainsAll(t, plain, append(selectedLines(3), "older than 1 week", "tm-04", "tm-05", "3 of 6")...)
	testui.AssertNotContainsAny(t, plain, "older than 1 day", "tm-02")

	// From the top the window holds the first divider and three issues, and
	// the header says so.
	plain = renderAged(rows, 0, 0, height, true, markerNow)
	testui.AssertContainsAll(t, plain, append(selectedLines(0), "older than 1 day", "tm-01", "tm-02", "3 of 6")...)
	testui.AssertNotContainsAny(t, plain, "older than 1 week", "Issue 03", "tm-03")

	// Without markers the line the divider took goes to the fourth issue. It
	// is half an issue: the title is drawn, the line under it is not, and the
	// header does not count it.
	plain = renderAged(rows, 0, 0, height, false, markerNow)
	testui.AssertContainsAll(t, plain, "tm-00", "tm-01", "tm-02", "Issue 03", "3 of 6")
	testui.AssertNotContainsAny(t, plain, "tm-03")
	if strings.Contains(plain, "older than") {
		t.Fatalf("expected no divider when AgeMarkers is false:\n%s", plain)
	}
}

// TestRenderSlidesTheWindowToKeepTheSelectedRowDrawn covers the two ways a
// stored offset can be stale by the time View runs: a divider that appeared
// since it was computed, and a section too short for the stacked dividers
// and the issue below them.
func TestRenderSlidesTheWindowToKeepTheSelectedRowDrawn(t *testing.T) {
	t.Parallel()

	// Four fresh issues and eight just under a day old fill an 18-line window
	// with the selection on the last two lines. Twenty-five minutes later the
	// 1-day divider lands inside that window and would push the second line of
	// tm-08 off the last line; the window slides one line instead, which cuts
	// the first issue to the line under its title.
	ages := make([]time.Duration, 12)
	for i := range ages {
		ages[i] = 2 * time.Hour
		if i >= 4 {
			ages[i] = 23*time.Hour + 40*time.Minute + time.Duration(i)*time.Minute
		}
	}
	rows := agedRows(ages...)
	plain := renderAged(rows, 0, 8, 22, true, markerNow)
	testui.AssertContainsAll(t, plain, append(selectedLines(8), "Issue 00", "9 of 12")...)
	testui.AssertNotContainsAny(t, plain, "older than")

	plain = renderAged(rows, 0, 8, 22, true, markerNow.Add(25*time.Minute))
	testui.AssertContainsAll(t, plain, append(selectedLines(8), "tm-00", "Issue 01", "older than 1 day", "8 of 12")...)
	testui.AssertNotContainsAny(t, plain, "Issue 00")

	// Three content rows, three stale issues, the first selected: the window
	// keeps the divider it has room for and both lines of the selected row,
	// never two dividers and half an issue.
	rows = agedRows(30*day, 31*day, 32*day)
	plain = renderAged(rows, 0, 0, 7, true, markerNow)
	testui.AssertContainsAll(t, plain, append(selectedLines(0), "older than 1 week", "1 of 3")...)
	testui.AssertNotContainsAny(t, plain, "older than 1 day")

	// One content row cannot hold an issue. It draws the title of the selected
	// one, which says which issue it is, and counts no issue as visible.
	plain = renderAged(rows, 0, 1, 5, true, markerNow)
	testui.AssertContainsAll(t, plain, selectedLines(1)[0], "0 of 3")
	testui.AssertNotContainsAny(t, plain, "older than", "tm-01")
}

func TestEnsureVisibleCountsOnlyTheDividersInsideTheWindow(t *testing.T) {
	t.Parallel()

	rows := agedRows(time.Hour, day, 2*day, 3*day, 4*day, 5*day, 6*day, 7*day+time.Hour, 8*day, 9*day, 10*day, 11*day, 12*day, 13*day, 14*day, 15*day, 16*day, 17*day, 18*day, 19*day)
	column := func(offset, selected int) Column {
		return Column{Rows: rows, ScrollOffset: offset, SelectedRow: selected, TotalIsExact: true, AgeMarkers: true}
	}
	const capacity = 18 // nine two-line issues

	// Walking down from the top: the first slide happens one issue early
	// because the two dividers sit inside the window.
	offset := 0
	for sel := 0; sel < len(rows); sel++ {
		offset = EnsureVisible(column(offset, sel), capacity, markerNow)
		plain := renderAged(rows, offset, sel, capacity+headLines+2, true, markerNow)
		for _, want := range selectedLines(sel) {
			if !strings.Contains(plain, want) {
				t.Fatalf("sel %d offset %d: %q not drawn:\n%s", sel, offset, want, plain)
			}
		}
		if sel == 7 && offset != 0 {
			t.Fatalf("expected no slide while the two dividers and eight issues fit, got offset %d", offset)
		}
		if sel == 8 && offset != 1 {
			t.Fatalf("expected the first slide at the ninth issue, got offset %d", offset)
		}
	}

	// At the end both dividers are above the window, so it holds nine issues
	// and the header counts all of them.
	if offset != 11 {
		t.Fatalf("expected offset 11 at the last row, got %d", offset)
	}
	plain := renderAged(rows, offset, 19, capacity+headLines+2, true, markerNow)
	testui.AssertContainsAll(t, plain, append(selectedLines(19), "tm-11", "9 of 20")...)
	testui.AssertNotContainsAny(t, plain, "older than")

	// Moving back up slides only when the selection leaves the window.
	if got := EnsureVisible(column(11, 10), capacity, markerNow); got != 10 {
		t.Fatalf("expected offset 10 when moving above the window, got %d", got)
	}
	if got := EnsureVisible(column(11, 15), capacity, markerNow); got != 11 {
		t.Fatalf("expected offset unchanged inside the window, got %d", got)
	}

	// The pinned error row is reserved too: seventeen lines are left, and from
	// the second issue the two dividers and eight issues need eighteen.
	withErr := column(0, 8)
	withErr.Error = "boom"
	if got := EnsureVisible(withErr, capacity, markerNow); got != 2 {
		t.Fatalf("expected offset 2 with an error row and the dividers in the window, got %d", got)
	}
}

// TestEnsureVisibleKeepsBothLinesOfTheSelectedIssueDrawn walks the selection
// down past the window end and back up, with an even and an odd number of
// content rows — the odd one leaves half a row at the bottom — and with the
// rows that take lines from the issues: the dividers and the pinned error row.
// After every move both lines of the selected issue are drawn, the header
// counts the issues drawn whole, and HitTest finds the issue on either line.
func TestEnsureVisibleKeepsBothLinesOfTheSelectedIssueDrawn(t *testing.T) {
	t.Parallel()

	rows := agedRows(time.Hour, 2*time.Hour, 2*day, 3*day, 4*day, 8*day, 9*day, 10*day, 11*day, 12*day, 13*day)
	cases := []struct {
		name    string
		markers bool
		err     string
	}{
		{name: "plain"},
		{name: "age dividers", markers: true},
		{name: "pinned error row", err: "boom"},
		{name: "age dividers and pinned error row", markers: true, err: "boom"},
	}
	walk := make([]int, 0, 2*len(rows))
	for sel := range rows {
		walk = append(walk, sel)
	}
	for sel := len(rows) - 2; sel >= 0; sel-- {
		walk = append(walk, sel)
	}

	for _, tc := range cases {
		for _, capacity := range []int{4, 5, 6, 7, 8, 9} {
			t.Run(fmt.Sprintf("%s/%d content rows", tc.name, capacity), func(t *testing.T) {
				t.Parallel()

				state := State{
					Columns: []Column{{Title: "Ready", Rows: rows, Total: len(rows), TotalIsExact: true, AgeMarkers: tc.markers, Error: tc.err}},
					Width:   60,
					Height:  capacity + headLines + 2,
					Now:     markerNow,
				}
				col := &state.Columns[0]
				for _, sel := range walk {
					col.SelectedRow = sel
					col.ScrollOffset = EnsureVisible(*col, capacity, markerNow)
					view := Render(state)

					for _, line := range selectedLines(sel) {
						x, y := testui.FindCell(t, view, line)
						if hit, ok := HitTest(state, x, y); !ok || hit.Row != sel {
							t.Fatalf("sel %d offset %d: HitTest on %q (%d,%d) = %+v, %v; want row %d", sel, col.ScrollOffset, line, x, y, hit, ok, sel)
						}
					}

					plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
					whole := 0
					for idx := range rows {
						if strings.Contains(plain, fmt.Sprintf("Issue %02d", idx)) && strings.Contains(plain, fmt.Sprintf("tm-%02d", idx)) {
							whole++
						}
					}
					if want := fmt.Sprintf(" %d of %d ", whole, len(rows)); !strings.Contains(plain, want) {
						t.Fatalf("sel %d offset %d: %d issues drawn whole, header does not read %q:\n%s", sel, col.ScrollOffset, whole, want, plain)
					}
				}
			})
		}
	}
}

// TestRenderCountsOnlyTheIssuesDrawnWhole pins the half row an odd number of
// content rows leaves at the bottom: its title is drawn and a click on it
// lands on that issue, but the header does not count it.
func TestRenderCountsOnlyTheIssuesDrawnWhole(t *testing.T) {
	t.Parallel()

	rows := agedRows(time.Hour, 2*time.Hour, 3*time.Hour, 4*time.Hour)
	state := State{
		Columns: []Column{{Title: "Ready", Rows: rows, SelectedRow: 0, Total: len(rows), TotalIsExact: true}},
		Width:   60,
		Height:  9, // 5 content rows: two issues and the title of a third
		Now:     markerNow,
	}
	view := Render(state)
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	testui.AssertContainsAll(t, plain, "tm-00", "tm-01", "Issue 02", " 2 of 4 ")
	testui.AssertNotContainsAny(t, plain, "tm-02", "Issue 03")

	x, y := testui.FindCell(t, view, "Issue 02")
	if hit, ok := HitTest(state, x, y); !ok || hit.Row != 2 {
		t.Fatalf("HitTest on the half row = %+v, %v; want row 2", hit, ok)
	}

	// An even number of content rows holds whole issues only.
	state.Height = 10
	plain = testui.AnsiEscapePattern.ReplaceAllString(Render(state), "")
	testui.AssertContainsAll(t, plain, "tm-02", " 3 of 4 ")
	testui.AssertNotContainsAny(t, plain, "Issue 03")

	// With every issue drawn whole the header is the plain total.
	state.Height = 12
	plain = testui.AnsiEscapePattern.ReplaceAllString(Render(state), "")
	testui.AssertContainsAll(t, plain, "tm-03", " 4 ")
	testui.AssertNotContainsAny(t, plain, " of 4")
}

// TestMaxOffsetIsTheFirstIssueTheRestOfTheListFitsFrom pins the bound a mode
// model pulls a stale offset back to. It counts lines, as the renderer does:
// an offset past it leaves blank lines under the last issue, and one before it
// is a window the operator scrolled to and must keep.
func TestMaxOffsetIsTheFirstIssueTheRestOfTheListFitsFrom(t *testing.T) {
	t.Parallel()

	fresh := make([]time.Duration, 10)
	for i := range fresh {
		fresh[i] = time.Hour
	}
	stale := agedRows(time.Hour, time.Hour, time.Hour, time.Hour, time.Hour, time.Hour, time.Hour, 2*day, 3*day, 8*day)

	cases := []struct {
		name     string
		col      Column
		capacity int
		want     int
	}{
		{name: "empty", col: Column{}, capacity: 6, want: 0},
		{name: "every issue fits", col: Column{Rows: agedRows(fresh...)}, capacity: 20, want: 0},
		{name: "even rows hold three issues", col: Column{Rows: agedRows(fresh...)}, capacity: 6, want: 7},
		{name: "odd rows hold two issues and a blank line", col: Column{Rows: agedRows(fresh...)}, capacity: 5, want: 8},
		{name: "the error row takes a line", col: Column{Rows: agedRows(fresh...), Error: "boom"}, capacity: 6, want: 8},
		{name: "the dividers in the tail take their lines", col: Column{Rows: stale, AgeMarkers: true}, capacity: 8, want: 7},
		{name: "a section shorter than one issue", col: Column{Rows: agedRows(fresh...)}, capacity: 1, want: 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := MaxOffset(tc.col, tc.capacity, markerNow)
			if got != tc.want {
				t.Fatalf("MaxOffset = %d, want %d", got, tc.want)
			}
			if len(tc.col.Rows) == 0 {
				return
			}

			// From the bound the last issue is drawn whole, and one issue
			// earlier it no longer is.
			lastID := fmt.Sprintf("tm-%02d", len(tc.col.Rows)-1)
			draws := func(offset int) bool {
				col := tc.col
				col.ScrollOffset, col.SelectedRow, col.TotalIsExact = offset, -1, true
				view := Render(State{Columns: []Column{col}, Width: 60, Height: tc.capacity + headLines + 2, Now: markerNow})
				return strings.Contains(testui.AnsiEscapePattern.ReplaceAllString(view, ""), lastID)
			}
			if tc.capacity >= 2 && !draws(got) {
				t.Errorf("offset %d does not draw the last issue %s", got, lastID)
			}
			if got > 0 && draws(got-1) {
				t.Errorf("offset %d still draws the last issue %s: the bound is one too far", got-1, lastID)
			}
		})
	}
}

func TestRenderKeepsTheEmptyPlaceholderWhenMarkersAreOn(t *testing.T) {
	t.Parallel()

	view := Render(State{
		Columns: []Column{{Title: "Ready", TotalIsExact: true, AgeMarkers: true}},
		Width:   40,
		Height:  6,
		Now:     markerNow,
	})
	testui.AssertContainsAll(t, view, "(no issues)")
}
