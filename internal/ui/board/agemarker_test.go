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
	const height = 7 // 4 content rows

	// Scrolled to the fourth issue: the divider drawn directly above it opens
	// the window, the selected row is on screen, and the header counts issues
	// only — the divider is not one of the "N of M".
	plain := renderAged(rows, 3, 3, height, true, markerNow)
	testui.AssertContainsAll(t, plain, "older than 1 week", "› T P2 OPN tm-03", "tm-04", "tm-05", "3 of 6")
	testui.AssertNotContainsAny(t, plain, "older than 1 day", "tm-02")

	// From the top the window holds the first divider and two issues, and the
	// header says so.
	plain = renderAged(rows, 0, 0, height, true, markerNow)
	testui.AssertContainsAll(t, plain, "› T P2 OPN tm-00", "older than 1 day", "tm-01", "tm-02", "3 of 6")
	testui.AssertNotContainsAny(t, plain, "older than 1 week", "tm-03")

	// Without markers the same window is four plain issue rows.
	plain = renderAged(rows, 0, 0, height, false, markerNow)
	testui.AssertContainsAll(t, plain, "tm-00", "tm-01", "tm-02", "tm-03", "4 of 6")
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

	// Four fresh issues and eight just under a day old fill a 9-row window
	// with the selection on the last line. Twenty-five minutes later the
	// 1-day divider lands inside that window and would push tm-08 off the
	// last line; the window slides one line instead.
	ages := make([]time.Duration, 12)
	for i := range ages {
		ages[i] = 2 * time.Hour
		if i >= 4 {
			ages[i] = 23*time.Hour + 40*time.Minute + time.Duration(i)*time.Minute
		}
	}
	rows := agedRows(ages...)
	plain := renderAged(rows, 0, 8, 12, true, markerNow)
	testui.AssertContainsAll(t, plain, "tm-00", "› T P2 OPN tm-08", "9 of 12")
	testui.AssertNotContainsAny(t, plain, "older than")

	plain = renderAged(rows, 0, 8, 12, true, markerNow.Add(25*time.Minute))
	testui.AssertContainsAll(t, plain, "tm-01", "older than 1 day", "› T P2 OPN tm-08", "8 of 12")
	testui.AssertNotContainsAny(t, plain, "tm-00")

	// Two content rows, three stale issues, the first selected: the window
	// keeps the divider it has room for and the selected row, never two
	// dividers and no issue.
	rows = agedRows(30*day, 31*day, 32*day)
	plain = renderAged(rows, 0, 0, 5, true, markerNow)
	testui.AssertContainsAll(t, plain, "older than 1 week", "› T P2 OPN tm-00", "1 of 3")
	testui.AssertNotContainsAny(t, plain, "older than 1 day")
}

func TestEnsureVisibleCountsOnlyTheDividersInsideTheWindow(t *testing.T) {
	t.Parallel()

	rows := agedRows(time.Hour, day, 2*day, 3*day, 4*day, 5*day, 6*day, 7*day+time.Hour, 8*day, 9*day, 10*day, 11*day, 12*day, 13*day, 14*day, 15*day, 16*day, 17*day, 18*day, 19*day)
	column := func(offset, selected int) Column {
		return Column{Rows: rows, ScrollOffset: offset, SelectedRow: selected, TotalIsExact: true, AgeMarkers: true}
	}
	const capacity = 9

	// Walking down from the top: the first slide happens one row early
	// because the 1-day divider sits inside the window.
	offset := 0
	for sel := 0; sel < len(rows); sel++ {
		offset = EnsureVisible(column(offset, sel), capacity, markerNow)
		plain := renderAged(rows, offset, sel, capacity+3, true, markerNow)
		want := fmt.Sprintf("› T P2 OPN tm-%02d", sel)
		if !strings.Contains(plain, want) {
			t.Fatalf("sel %d offset %d: %q not drawn:\n%s", sel, offset, want, plain)
		}
	}

	// At the end both dividers are above the window, so it holds nine issues
	// and the header counts all of them.
	if offset != 11 {
		t.Fatalf("expected offset 11 at the last row, got %d", offset)
	}
	plain := renderAged(rows, offset, 19, capacity+3, true, markerNow)
	testui.AssertContainsAll(t, plain, "tm-11", "› T P2 OPN tm-19", "9 of 20")
	testui.AssertNotContainsAny(t, plain, "older than")

	// Moving back up slides only when the selection leaves the window.
	if got := EnsureVisible(column(11, 10), capacity, markerNow); got != 10 {
		t.Fatalf("expected offset 10 when moving above the window, got %d", got)
	}
	if got := EnsureVisible(column(11, 15), capacity, markerNow); got != 11 {
		t.Fatalf("expected offset unchanged inside the window, got %d", got)
	}

	// The pinned error row is reserved too.
	withErr := column(0, 8)
	withErr.Error = "boom"
	if got := EnsureVisible(withErr, capacity, markerNow); got != 3 {
		t.Fatalf("expected offset 3 with an error row and a divider in the window, got %d", got)
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
