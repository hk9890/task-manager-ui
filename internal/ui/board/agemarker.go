package board

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// ageThreshold is one age marker: a divider row drawn before the first issue
// whose last change is older than Age. The rows must already be ordered by
// UpdatedAt descending, so every issue below the divider is at least that old.
type ageThreshold struct {
	Age   time.Duration
	Label string
	Short string
}

var ageThresholds = [...]ageThreshold{
	{Age: 24 * time.Hour, Label: "older than 1 day", Short: "> 1 day"},
	{Age: 7 * 24 * time.Hour, Label: "older than 1 week", Short: "> 1 week"},
}

// ageMarker is a divider to draw before the issue at index Before. Count is
// the number of issues at or after Before.
type ageMarker struct {
	threshold ageThreshold
	Before    int
	Count     int
}

// ageMarkers returns the dividers for rows at now, in the order they are drawn.
// Two markers may share the same Before index; they then stack. A zero now
// disables markers, and an issue with no change date is never counted as old:
// an unknown date is not evidence of age.
func ageMarkers(rows []domain.IssueSummary, now time.Time) []ageMarker {
	if now.IsZero() || len(rows) == 0 {
		return nil
	}
	markers := make([]ageMarker, 0, len(ageThresholds))
	for _, threshold := range ageThresholds {
		for idx, issue := range rows {
			if !issue.UpdatedAt.IsZero() && now.Sub(issue.UpdatedAt) > threshold.Age {
				markers = append(markers, ageMarker{threshold: threshold, Before: idx, Count: len(rows) - idx})
				break
			}
		}
	}
	return markers
}

// AgeMarkerCount returns the number of divider rows Render inserts into a
// column with rows at now. A mode model subtracts it from its scroll window so
// the selected row stays inside what the renderer draws.
func AgeMarkerCount(rows []domain.IssueSummary, now time.Time) int {
	return len(ageMarkers(rows, now))
}

const (
	markerEdge = "───"
	markerRule = '─'
)

// renderAgeMarker draws one divider row at width: the rule, the label, the
// rule again, the count, the rule. The label falls back to its short form when
// the long one does not fit, and the count is dropped after that.
func renderAgeMarker(marker ageMarker, width int, styled bool) string {
	count := strconv.Itoa(marker.Count)
	line := markerLine(marker.threshold.Label, count, width)
	if line == "" {
		line = markerLine(marker.threshold.Short, count, width)
	}
	if line == "" {
		line = markerLine(marker.threshold.Short, "", width)
	}
	if line == "" {
		line = textutil.TruncateString(markerEdge+" "+marker.threshold.Short, width)
	}
	if !styled {
		return line
	}
	return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(line)
}

// markerLine lays out "─── label ─── count ───" to exactly width, or returns
// "" when even a single rule cell between the parts would not fit.
func markerLine(label, count string, width int) string {
	var right string
	if count != "" {
		right = " " + count + " " + markerEdge
	} else {
		right = " " + markerEdge
	}
	fixed := lipgloss.Width(markerEdge+" "+label+" ") + lipgloss.Width(right)
	fill := width - fixed
	if fill < 1 {
		return ""
	}
	return markerEdge + " " + label + " " + strings.Repeat(string(markerRule), fill) + right
}
