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

var markerNow = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

// agedRows builds rows whose last change lies the given durations before
// markerNow, in the order given: the caller keeps them descending.
func agedRows(ages ...time.Duration) []domain.IssueSummary {
	rows := make([]domain.IssueSummary, len(ages))
	for i, age := range ages {
		rows[i] = domain.IssueSummary{
			ID:        fmt.Sprintf("tm-%d", i),
			Title:     fmt.Sprintf("Issue %d", i),
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

	const day = 24 * time.Hour
	cases := []struct {
		name string
		rows []domain.IssueSummary
		now  time.Time
		want []ageMarker
	}{
		{name: "all fresh", rows: agedRows(time.Hour, 23*time.Hour), now: markerNow, want: nil},
		{name: "crosses a day only", rows: agedRows(time.Hour, 2*day), now: markerNow,
			want: []ageMarker{{threshold: ageThresholds[0], Before: 1, Count: 1}}},
		{name: "crosses both", rows: agedRows(time.Hour, 2*day, 3*day, 8*day, 30*day), now: markerNow,
			want: []ageMarker{{threshold: ageThresholds[0], Before: 1, Count: 4}, {threshold: ageThresholds[1], Before: 3, Count: 2}}},
		{name: "all stale stacks both at the top", rows: agedRows(8*day, 9*day), now: markerNow,
			want: []ageMarker{{threshold: ageThresholds[0], Before: 0, Count: 2}, {threshold: ageThresholds[1], Before: 0, Count: 2}}},
		{name: "zero now disables", rows: agedRows(8 * day), now: time.Time{}, want: nil},
		{name: "no change date is not old", rows: []domain.IssueSummary{{ID: "tm-0"}}, now: markerNow, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ageMarkers(tc.rows, tc.now)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d markers %+v, want %d %+v", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("marker %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
			if n := AgeMarkerCount(tc.rows, tc.now); n != len(tc.want) {
				t.Errorf("AgeMarkerCount = %d, want %d", n, len(tc.want))
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
		{width: 18, want: "─── > 1 week ─ ───"},
		{width: 14, want: "─── > 1 week"},
		{width: 8, want: "─── > 1…"},
	}
	for _, tc := range cases {
		got := renderAgeMarker(marker, tc.width, false)
		if got != tc.want {
			t.Errorf("width %d: got %q, want %q", tc.width, got, tc.want)
		}
		if w := lipgloss.Width(got); w > tc.width {
			t.Errorf("width %d: rendered %d cells", tc.width, w)
		}
	}
	for width := 6; width <= 60; width++ {
		if w := lipgloss.Width(renderAgeMarker(marker, width, true)); w > width {
			t.Errorf("styled width %d: rendered %d cells", width, w)
		}
	}
}

func TestRenderWindowsOnIssueIndexAcrossDividers(t *testing.T) {
	t.Parallel()

	const day = 24 * time.Hour
	rows := agedRows(time.Hour, 2*day, 3*day, 8*day, 9*day, 10*day)
	render := func(offset, selected int, markers bool) string {
		return Render(State{
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
			Height: 7, // 4 inner rows
			Now:    markerNow,
		})
	}

	// Scrolled to the fourth issue: the divider drawn directly above it opens
	// the window, the selected row is on screen, and the header counts issues
	// only — the divider is not one of the "N of M".
	plain := testui.AnsiEscapePattern.ReplaceAllString(render(3, 3, true), "")
	testui.AssertContainsAll(t, plain, "older than 1 week", "› T P2 OPN tm-3", "tm-4", "tm-5", "3 of 6")
	testui.AssertNotContainsAny(t, plain, "older than 1 day", "tm-2")

	// From the top the window holds the first divider and two issues, and the
	// header says so.
	plain = testui.AnsiEscapePattern.ReplaceAllString(render(0, 0, true), "")
	testui.AssertContainsAll(t, plain, "› T P2 OPN tm-0", "older than 1 day", "tm-1", "tm-2", "3 of 6")
	testui.AssertNotContainsAny(t, plain, "older than 1 week", "tm-3")

	// Without markers the same window is four plain issue rows.
	plain = testui.AnsiEscapePattern.ReplaceAllString(render(0, 0, false), "")
	testui.AssertContainsAll(t, plain, "tm-0", "tm-1", "tm-2", "tm-3", "4 of 6")
	if strings.Contains(plain, "older than") {
		t.Fatalf("expected no divider when AgeMarkers is false:\n%s", plain)
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
