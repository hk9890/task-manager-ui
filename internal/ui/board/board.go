package board

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/ui/shared/issuerow"
	"github.com/hk9890/beads-workbench/internal/ui/skeleton"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const (
	defaultBoardWidth   = 100
	defaultBoardHeight  = 20
	columnGap           = 2
	minRenderableWidth  = 24
	minReadableColumn   = 32
	minReadableWideCol  = 40
	wideLayoutThreshold = 150
	fallbackColumnWidth = 18
)

// Column is one board section column.
type Column struct {
	Title       string
	Rows        []domain.IssueSummary
	SelectedRow int
	// Error is a non-empty string when a gateway call for this column failed.
	// The renderer shows an inline error row at the top of the column content.
	Error string
	// Loading is true while the column's data is being fetched. When Loading
	// is true and Rows is empty, the renderer shows skeleton placeholder rows.
	// When Loading is true and Rows is non-empty, stale rows are shown as-is
	// (the global header spinner from 0x36.6 signals the in-flight state).
	Loading bool
	// Total is the number of issues in this column as reported by the gateway.
	// TotalIsExact is false when the backend may have more issues than were returned
	// (e.g. the Done column was capped), in which case the renderer shows "N+".
	Total        int
	TotalIsExact bool
}

// State is the full board renderer input.
type State struct {
	DashboardTitle string
	Columns        []Column
	FocusedColumn  int
	Width          int
	Height         int
}

// Render renders a multi-column board dashboard using section borders.
func Render(state State) string {
	if len(state.Columns) == 0 {
		return "No board sections configured."
	}

	width := state.Width
	if width <= 0 {
		width = defaultBoardWidth
	}
	height := state.Height
	if height <= 0 {
		height = defaultBoardHeight
	}

	start, end := visibleColumnRange(width, len(state.Columns), state.FocusedColumn)
	visible := state.Columns[start:end]

	visibleAvailableWidth := width - (columnGap * (len(visible) - 1))
	if visibleAvailableWidth < minRenderableWidth {
		visibleAvailableWidth = minRenderableWidth
	}

	columnWidths := distributeWidths(visibleAvailableWidth, len(visible))
	columnHeight := max(3, height-1)
	renderedCols := make([]string, 0, len(visible))
	for idx, col := range visible {
		innerWidth := columnWidths[idx] - 2
		if innerWidth < 1 {
			innerWidth = 1
		}

		rows := renderColumnRows(col, innerWidth)
		plus := ""
		if !col.TotalIsExact {
			plus = "+"
		}
		topRight := fmt.Sprintf("%d%s", col.Total, plus)
		renderedCols = append(renderedCols, styles.FormSection(styles.FormSectionConfig{
			Width:              columnWidths[idx],
			Height:             columnHeight,
			TopLeft:            col.Title,
			TopRight:           topRight,
			Content:            rows,
			Focused:            (start + idx) == state.FocusedColumn,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		}))
	}

	head := strings.TrimSpace(state.DashboardTitle)
	if len(visible) < len(state.Columns) {
		head = fmt.Sprintf("%s · cols %d-%d/%d", head, start+1, end, len(state.Columns))
	}
	columns := lipgloss.JoinHorizontal(lipgloss.Top, joinWithGap(renderedCols, strings.Repeat(" ", columnGap))...)
	if head == "" {
		return columns
	}

	return head + "\n" + columns
}

func visibleColumnRange(width, total, focused int) (start, end int) {
	if total <= 0 {
		return 0, 0
	}

	if focused < 0 {
		focused = 0
	}
	if focused >= total {
		focused = total - 1
	}

	maxVisible := maxVisibleColumns(width)
	if maxVisible >= total {
		return 0, total
	}

	start = focused - (maxVisible / 2)
	if start < 0 {
		start = 0
	}
	end = start + maxVisible
	if end > total {
		end = total
		start = end - maxVisible
	}

	return start, end
}

func maxVisibleColumns(width int) int {
	if width <= 0 {
		width = defaultBoardWidth
	}

	minReadable := minReadableColumn
	if width >= wideLayoutThreshold {
		minReadable = minReadableWideCol
	}

	max := (width + columnGap) / (minReadable + columnGap)
	if max < 1 {
		max = 1
	}

	return max
}

func distributeWidths(total, count int) []int {
	if count <= 0 {
		return nil
	}
	if total < count {
		total = count
	}

	base := total / count
	if base < fallbackColumnWidth {
		base = fallbackColumnWidth
		total = base * count
	}

	remainder := total % count
	widths := make([]int, count)
	for i := 0; i < count; i++ {
		widths[i] = base
		if i >= count-remainder {
			widths[i]++
		}
	}

	return widths
}

// skeletonRows returns ~6 skeleton placeholder rows for a loading column.
func skeletonRows(maxWidth int) []string {
	const numSkeletonRows = 6
	rows := make([]string, 0, numSkeletonRows)
	for i := 0; i < numSkeletonRows; i++ {
		rows = append(rows, skeleton.SkeletonRow(maxWidth, 2))
	}
	return rows
}

func renderColumnRows(col Column, maxWidth int) []string {
	var rows []string

	// Inline error row at the top (if any).
	if strings.TrimSpace(col.Error) != "" {
		errRow := styles.TruncateString("⚠ load failed: "+col.Error, maxWidth)
		rows = append(rows, errRow)
	}

	if col.Loading {
		if len(col.Rows) == 0 {
			// Cold-start: no data yet — show skeleton rows.
			rows = append(rows, skeletonRows(maxWidth)...)
			return rows
		}
		// Refresh: stale rows on screen while new data is in flight.
		for idx, issue := range col.Rows {
			rows = append(rows, issuerow.RenderCompact(issuerow.RenderConfig{
				Issue:    issue,
				Selected: idx == col.SelectedRow,
				Width:    maxWidth,
				Styled:   true,
			}))
		}
		return rows
	}

	// Not loading — render normally.
	if len(rows) == 0 && len(col.Rows) == 0 {
		rows = append(rows, "(no issues)")
		return rows
	}

	for idx, issue := range col.Rows {
		rows = append(rows, issuerow.RenderCompact(issuerow.RenderConfig{
			Issue:    issue,
			Selected: idx == col.SelectedRow,
			Width:    maxWidth,
			Styled:   true,
		}))
	}

	return rows
}

func joinWithGap(parts []string, gap string) []string {
	if len(parts) == 0 {
		return nil
	}

	out := make([]string, 0, len(parts)*2-1)
	for i, part := range parts {
		if i > 0 {
			out = append(out, gap)
		}
		out = append(out, part)
	}

	return out
}
