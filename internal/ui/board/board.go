package board

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/ui/shared/renderhelpers"
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
	minTitleWidth       = 8
)

// Row is one renderable board issue row.
type Row struct {
	ID       string
	Title    string
	Type     string
	Status   string
	Priority int
	Selected bool
}

// Column is one board section column.
type Column struct {
	Title string
	Rows  []Row
	Error string
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

	availableWidth := width - (columnGap * (len(state.Columns) - 1))
	if availableWidth < minRenderableWidth {
		availableWidth = minRenderableWidth
	}

	start, end := visibleColumnRange(width, len(state.Columns), state.FocusedColumn)
	visible := state.Columns[start:end]

	visibleAvailableWidth := width - (columnGap * (len(visible) - 1))
	if visibleAvailableWidth < minRenderableWidth {
		visibleAvailableWidth = minRenderableWidth
	}

	columnWidths := distributeWidths(visibleAvailableWidth, len(visible))
	columnHeight := renderhelpers.MaxInt(3, height-1)
	renderedCols := make([]string, 0, len(visible))
	for idx, col := range visible {
		innerWidth := columnWidths[idx] - 2
		if innerWidth < 1 {
			innerWidth = 1
		}

		rows := renderColumnRows(col, innerWidth)
		renderedCols = append(renderedCols, styles.FormSection(styles.FormSectionConfig{
			Width:              columnWidths[idx],
			Height:             columnHeight,
			TopLeft:            col.Title,
			TopRight:           fmt.Sprintf("%d", len(col.Rows)),
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

func renderColumnRows(col Column, maxWidth int) []string {
	if strings.TrimSpace(col.Error) != "" {
		return []string{styles.TruncateString("Error: "+col.Error, maxWidth)}
	}

	if len(col.Rows) == 0 {
		return []string{"(no issues)"}
	}

	rows := make([]string, 0, len(col.Rows))
	for _, row := range col.Rows {
		rows = append(rows, renderRowLine(row, maxWidth))
	}

	return rows
}

func renderRowLine(row Row, maxWidth int) string {
	prefix := "  "
	if row.Selected {
		prefix = "› "
	}

	title := strings.TrimSpace(row.Title)
	if title == "" {
		title = "(untitled)"
	}

	idWidth := minInt(12, renderhelpers.MaxInt(7, maxWidth/5))
	id := renderhelpers.CompactIssueID(row.ID, idWidth)
	meta := strings.Join([]string{renderhelpers.CompactIssueType(row.Type), renderhelpers.CompactPriority(row.Priority), renderhelpers.CompactIssueState(row.Status), id}, " ")
	titlePrefix := prefix + meta + " "
	titleWidth := maxWidth - lipgloss.Width(titlePrefix)
	if titleWidth < minTitleWidth {
		return styles.TruncateString(prefix+meta, maxWidth)
	}

	return titlePrefix + styles.TruncateString(title, titleWidth)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
