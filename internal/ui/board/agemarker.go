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
// the number of issues at or after Before that carry a change date.
type ageMarker struct {
	threshold ageThreshold
	Before    int
	Count     int
}

// ageMarkers returns the dividers for rows at now, in the order they are drawn.
// Two markers may share the same Before index; they then stack. An issue with
// no change date is never counted as old: an unknown date is not evidence of
// age, so it neither places a divider nor counts below one.
func ageMarkers(rows []domain.IssueSummary, now time.Time) []ageMarker {
	markers := make([]ageMarker, 0, len(ageThresholds))
	for _, threshold := range ageThresholds {
		for idx, issue := range rows {
			if !issue.UpdatedAt.IsZero() && now.Sub(issue.UpdatedAt) > threshold.Age {
				markers = append(markers, ageMarker{threshold: threshold, Before: idx, Count: datedFrom(rows, idx)})
				break
			}
		}
	}
	return markers
}

func datedFrom(rows []domain.IssueSummary, idx int) int {
	count := 0
	for _, issue := range rows[idx:] {
		if !issue.UpdatedAt.IsZero() {
			count++
		}
	}
	return count
}

// rowLayout is where each issue of a column lands among the lines the
// renderer draws for it, dividers included and the pinned error row excluded.
// issueStart[i] is the first line of issue i once any divider drawn directly
// above it is included; issueRow[i] is the line of the issue itself.
type rowLayout struct {
	markers    []ageMarker
	issueStart []int
	issueRow   []int
}

func layoutRows(col Column, now time.Time) rowLayout {
	layout := rowLayout{
		issueStart: make([]int, len(col.Rows)),
		issueRow:   make([]int, len(col.Rows)),
	}
	if col.AgeMarkers {
		layout.markers = ageMarkers(col.Rows, now)
	}
	line := 0
	pending := layout.markers
	for idx := range col.Rows {
		layout.issueStart[idx] = line
		for len(pending) > 0 && pending[0].Before == idx {
			line++
			pending = pending[1:]
		}
		layout.issueRow[idx] = line
		line++
	}
	return layout
}

// lines is the number of lines the layout spans.
func (l rowLayout) lines() int {
	if len(l.issueRow) == 0 {
		return 0
	}
	return l.issueRow[len(l.issueRow)-1] + 1
}

// window is the line range [start, end) Render draws for col from its scroll
// offset, given content rows for the issue area of total lines (the layout
// plus any trailing affordance such as the load-more skeleton). The window
// opens at the offset issue's first line, divider included, and slides down
// only as far as it must to keep the selected issue on a drawn line — which is
// how a divider that appeared since the offset was computed, or a section too
// short for the stacked dividers and an issue, still shows the chevron.
func (l rowLayout) window(col Column, content, total int) (start, end int) {
	if content < 1 {
		content = 1
	}
	offset := textutil.Clamp(col.ScrollOffset, 0, len(col.Rows))
	start = l.lines()
	if offset < len(col.Rows) {
		start = l.issueStart[offset]
	}
	if sel := col.SelectedRow; sel >= 0 && sel < len(col.Rows) && l.issueRow[sel] >= start+content {
		start = l.issueRow[sel] - content + 1
	}
	end = min(start+content, total)
	return start, end
}

// EnsureVisible returns the scroll offset that keeps col.SelectedRow on a
// drawn line when the section holds capacity content rows. scroll.EnsureVisible
// counts issues; this counts the lines the renderer draws — the pinned error
// row and the age dividers between the offset and the selection — and slides
// the window as little as possible. Both mode models take their offset from
// here so the reservation and the drawing agree.
func EnsureVisible(col Column, capacity int, now time.Time) int {
	if len(col.Rows) == 0 {
		return 0
	}
	sel := textutil.Clamp(col.SelectedRow, 0, len(col.Rows)-1)
	offset := textutil.Clamp(col.ScrollOffset, 0, len(col.Rows)-1)
	if sel < offset {
		return sel
	}
	content := capacity - errorRows(col)
	if content < 1 {
		content = 1
	}
	layout := layoutRows(col, now)
	for offset < sel && layout.issueRow[sel]-layout.issueStart[offset] >= content {
		offset++
	}
	return offset
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
