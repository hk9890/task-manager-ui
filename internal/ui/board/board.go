package board

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
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
	// ScrollOffset is the index of the first row that should appear at the top
	// of the visible window. The renderer slices Rows[ScrollOffset:] before
	// passing to FormSection so the selection is always in view. When zero the
	// behaviour is identical to the pre-scroll implementation.
	ScrollOffset int
	// Error is a non-empty string when a repository call for this column failed.
	// The renderer shows an inline error row at the top of the column content.
	Error string
	// Loading is true while the column's data is being fetched. When Loading
	// is true and Rows is empty, the renderer shows skeleton placeholder rows.
	// When Loading is true and Rows is non-empty, stale rows are shown as-is
	// (the global header spinner signals the in-flight state).
	Loading bool
	// Total is the number of issues in this column as reported by the repository.
	// TotalIsExact is false when the rendered row list is truncated to a height
	// cap and not all issues are visible, in which case the renderer shows
	// "N of M" (e.g. "50 of 75") to communicate that only N of the M total
	// issues are shown. The exact total M is never rendered with a lower-bound
	// "+" suffix to avoid misrepresenting an exact count.
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
	SkeletonPhase  int // color-cycle index for skeleton row pulse; see loading.SkeletonPhase
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

		// innerHeight is the number of content rows that fit inside the section
		// borders. FormSection reserves 2 lines for top and bottom borders.
		innerHeight := max(1, columnHeight-2)

		// isLoadMore is true when a background page fetch is in flight for an
		// already-populated column that the user has scrolled into (offset > 0).
		// This is distinct from a full refresh (col.Loading=true, offset=0).
		isLoadMore := col.Loading && len(col.Rows) > 0 && col.ScrollOffset > 0

		// Build all rendered rows for this column.
		rows := renderColumnRows(col, innerWidth, state.SkeletonPhase, start+idx)

		// Apply scroll window: slice to [offset : offset+innerHeight] so that
		// the selected row is always visible. Only slice when not loading (skeleton
		// / stale-refresh paths manage their own row counts), or when a load-more
		// is in flight (offset > 0 indicates deep navigation with a pending page fetch).
		displayRows := rows
		if (!col.Loading || isLoadMore) && len(rows) > 0 {
			// renderColumnRows prepends an inline error row (when col.Error is set)
			// above the issue rows. ScrollOffset is an issue index, so the window
			// must be applied to the issue rows only; the error row is pinned at the
			// top and counts against innerHeight. Without this the window shifts by
			// the prefix length and clips the wrong issue (the selected row can fall
			// off-screen) whenever an error and issues coexist (e.g. a failed Done
			// load-more that keeps its rows).
			prefix := 0
			if strings.TrimSpace(col.Error) != "" {
				prefix = 1
			}
			issueRows := rows[prefix:]
			offset := col.ScrollOffset
			if offset < 0 {
				offset = 0
			}
			if offset > len(issueRows) {
				offset = len(issueRows)
			}
			end := offset + innerHeight - prefix
			if end < offset {
				end = offset
			}
			if end > len(issueRows) {
				end = len(issueRows)
			}
			windowed := issueRows[offset:end]
			displayRows = make([]string, 0, prefix+len(windowed))
			displayRows = append(displayRows, rows[:prefix]...)
			displayRows = append(displayRows, windowed...)
		}

		// Compute header badge.
		//
		// Three cases:
		//
		// (1) TotalIsExact=false (paginated column, e.g. Done with load-more):
		//     show "loaded of total" — len(col.Rows) / col.Total — so the user
		//     sees real pagination progress, not the window size. The chevron
		//     visibility property implicitly communicates window clip.
		//
		// (2) TotalIsExact=true and window clips (visibleCount < len(rows)):
		//     show "visible of total" — visibleCount / col.Total — so the user
		//     knows the rendered window is smaller than the loaded slice. This
		//     is the honesty path for Ready / NotReady / InProgress.
		//
		// (3) TotalIsExact=true and everything fits: just col.Total.
		var topRight string
		visibleCount := len(displayRows)
		switch {
		case isLoadMore:
			// Load-more in flight: show loaded count against the known total
			// so the header reflects real progress rather than the window slice.
			topRight = fmt.Sprintf("%d of %d", len(col.Rows), col.Total)
		case !col.TotalIsExact:
			// Paginated column: show loaded vs. real DB total.
			topRight = fmt.Sprintf("%d of %d", len(col.Rows), col.Total)
		case !col.Loading && visibleCount < len(rows):
			// Non-paginated but window clips: show visible vs. DB total.
			topRight = fmt.Sprintf("%d of %d", visibleCount, col.Total)
		default:
			// All loaded and all fit.
			topRight = fmt.Sprintf("%d", col.Total)
		}

		renderedCols = append(renderedCols, styles.FormSection(styles.FormSectionConfig{
			Width:              columnWidths[idx],
			Height:             columnHeight,
			TopLeft:            col.Title,
			TopRight:           topRight,
			Content:            displayRows,
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

// skeletonRowCounts varies the number of skeleton rows per board column so the
// cold-start loading state does not render as a uniform grid of identical
// columns. Indexed by absolute column index; safe-modulo handles >4 columns.
var skeletonRowCounts = [...]int{4, 6, 3, 5}

// skeletonRows returns skeleton placeholder rows for a loading column. colIndex
// selects the row count from skeletonRowCounts so adjacent columns differ in
// length rather than forming an identical block.
func skeletonRows(maxWidth, phase, colIndex int) []string {
	n := len(skeletonRowCounts)
	count := skeletonRowCounts[((colIndex%n)+n)%n]
	rows := make([]string, 0, count)
	for i := 0; i < count; i++ {
		rows = append(rows, issuerow.RenderCompactSkeleton(issuerow.SkeletonOpts{
			Width:  maxWidth,
			Seed:   i,
			Phase:  phase,
			Styled: true,
		}))
	}
	return rows
}

func renderColumnRows(col Column, maxWidth, skeletonPhase, colIndex int) []string {
	var rows []string

	// Inline error row at the top (if any).
	if strings.TrimSpace(col.Error) != "" {
		errRow := styles.TruncateString("⚠ load failed: "+col.Error, maxWidth)
		rows = append(rows, errRow)
	}

	if col.Loading {
		if len(col.Rows) == 0 {
			// Cold-start: no data yet — show skeleton rows.
			rows = append(rows, skeletonRows(maxWidth, skeletonPhase, colIndex)...)
			return rows
		}
		if col.ScrollOffset > 0 {
			// Load-more in flight: the user has scrolled deep and a background page
			// fetch is in progress. Render rows normally (not dimmed) and append a
			// single skeleton row at the end as a load-in-flight affordance.
			for idx, issue := range col.Rows {
				rows = append(rows, issuerow.RenderCompact(issuerow.RenderConfig{
					Issue:    issue,
					Selected: idx == col.SelectedRow,
					Width:    maxWidth,
					Styled:   true,
				}))
			}
			rows = append(rows, issuerow.RenderCompactSkeleton(issuerow.SkeletonOpts{
				Width:  maxWidth,
				Seed:   0,
				Phase:  skeletonPhase,
				Styled: true,
			}))
			return rows
		}
		// Refresh: stale rows on screen while new data is in flight.
		// Dim the foreground with the current skeleton phase to signal motion.
		for idx, issue := range col.Rows {
			rows = append(rows, issuerow.RenderCompact(issuerow.RenderConfig{
				Issue:    issue,
				Selected: idx == col.SelectedRow,
				Width:    maxWidth,
				Styled:   true,
				Dim:      true,
				Phase:    skeletonPhase,
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
