package board

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
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
	// ScrollOffset is the index of the issue the visible window opens on. It
	// is an issue index, not a line: the renderer maps it through the column's
	// row layout, so a divider drawn directly above that issue opens the
	// window with it. Take it from EnsureVisible.
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
	// AgeMarkers draws a divider row before the first issue whose last change
	// is older than each age threshold (see agemarker.go). Only meaningful for
	// a column ordered by UpdatedAt descending; the Done column, ordered by
	// close date, leaves it false.
	AgeMarkers bool
}

// State is the full board renderer input.
type State struct {
	DashboardTitle string
	Columns        []Column
	FocusedColumn  int
	// ColumnStart is the first column drawn when the width holds fewer than
	// all of them; see ColumnStart. Render clamps it to the columns there are.
	ColumnStart   int
	Width         int
	Height        int
	SkeletonPhase int // color-cycle index for skeleton row pulse; see loading.SkeletonPhase
	// Now is the instant the age markers measure against.
	Now time.Time
	// Hover is the cell under the pointer, as HitTest reported it; nil when the
	// pointer is elsewhere. Its row draws the hover band.
	Hover *Hit
}

// Render renders a multi-column board dashboard using section borders.
func Render(state State) string {
	if len(state.Columns) == 0 {
		return "No board sections configured."
	}

	f := layoutFrame(state)
	start := f.start
	visible := state.Columns[start:f.end]
	columnWidths, columnHeight := f.widths, f.columnHeight
	renderedCols := make([]string, 0, len(visible))
	for idx, col := range visible {
		// isLoadMore is true when a background page fetch is in flight for an
		// already-populated column that the user has scrolled into (offset > 0).
		// This is distinct from a full refresh (col.Loading=true, offset=0).
		isLoadMore := col.Loading && len(col.Rows) > 0 && col.ScrollOffset > 0

		view := f.viewColumn(state, idx)
		displayRows, visibleIssues := view.rows, view.visibleIssues

		// Compute header badge.
		//
		// Three cases:
		//
		// (1) TotalIsExact=false (paginated column, e.g. Done with load-more):
		//     show "loaded of total" — len(col.Rows) / col.Total — so the user
		//     sees real pagination progress, not the window size. The selection
		//     bar's visibility property implicitly communicates window clip.
		//
		// (2) TotalIsExact=true and window clips (visibleIssues < len(col.Rows)):
		//     show "visible of total" — visibleIssues / col.Total — so the user
		//     knows the rendered window is smaller than the loaded slice. This
		//     is the honesty path for Ready / NotReady / InProgress.
		//
		// (3) TotalIsExact=true and everything fits: just col.Total.
		var topRight string
		switch {
		case isLoadMore:
			// Load-more in flight: show loaded count against the known total
			// so the header reflects real progress rather than the window slice.
			topRight = fmt.Sprintf("%d of %d", len(col.Rows), col.Total)
		case !col.TotalIsExact:
			// Paginated column: show loaded vs. real DB total.
			topRight = fmt.Sprintf("%d of %d", len(col.Rows), col.Total)
		case !col.Loading && visibleIssues < len(col.Rows):
			// Non-paginated but window clips: show visible vs. DB total.
			topRight = fmt.Sprintf("%d of %d", visibleIssues, col.Total)
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

	columns := lipgloss.JoinHorizontal(lipgloss.Top, joinWithGap(renderedCols, strings.Repeat(" ", columnGap))...)
	if f.head == "" {
		return columns
	}

	return f.head + "\n" + columns
}

// frame is the geometry of one board frame: which columns are drawn, how wide
// each is, and how many lines sit above them. Render draws from it and HitTest
// reads a cell back through it, so the two cannot place a column differently.
type frame struct {
	start, end   int
	widths       []int
	columnHeight int
	// innerHeight is the number of content rows that fit inside the section
	// borders. FormSection reserves 2 lines for top and bottom borders.
	innerHeight int
	// head is the title line above the columns; empty draws no line.
	head string
}

func layoutFrame(state State) frame {
	width := state.Width
	if width <= 0 {
		width = defaultBoardWidth
	}
	height := state.Height
	if height <= 0 {
		height = defaultBoardHeight
	}

	var f frame
	f.start, f.end = visibleColumnRange(width, len(state.Columns), state.ColumnStart)
	count := f.end - f.start

	available := width - (columnGap * (count - 1))
	if available < minRenderableWidth {
		available = minRenderableWidth
	}
	f.widths = distributeWidths(available, count)
	f.columnHeight = max(3, height-1)
	f.innerHeight = max(1, f.columnHeight-2)

	f.head = strings.TrimSpace(state.DashboardTitle)
	if count < len(state.Columns) {
		f.head = fmt.Sprintf("%s · cols %d-%d/%d", f.head, f.start+1, f.end, len(state.Columns))
	}
	return f
}

// columnView is what one column draws inside its borders: the lines, and how
// many of its issues are among them.
type columnView struct {
	rows          []string
	visibleIssues int
}

// windowed reports whether col draws its rows through the scroll window. A
// loading column does not — the skeleton and stale-refresh paths manage their
// own row counts — unless a load-more is in flight (offset > 0 indicates deep
// navigation with a pending page fetch).
func windowed(col Column) bool {
	return len(col.Rows) > 0 && (!col.Loading || col.ScrollOffset > 0)
}

// viewColumn builds the idx-th visible column of state.
func (f frame) viewColumn(state State, idx int) columnView {
	col := state.Columns[f.start+idx]
	innerWidth := max(1, f.widths[idx]-2)
	hover := -1
	if state.Hover != nil && state.Hover.Column == f.start+idx {
		hover = state.Hover.Row
	}

	rendered := renderColumnRows(col, innerWidth, state.SkeletonPhase, f.start+idx, state.Now, hover)
	view := columnView{rows: rendered.rows, visibleIssues: len(col.Rows)}
	if !windowed(col) {
		return view
	}

	// The pinned error row counts against innerHeight; rowLayout.window says
	// which lines of the issue area are drawn.
	issueRows := rendered.rows[rendered.prefix:]
	startRow, endRow := rendered.layout.window(col, f.innerHeight-rendered.prefix, len(issueRows))
	view.rows = make([]string, 0, rendered.prefix+endRow-startRow)
	view.rows = append(view.rows, rendered.rows[:rendered.prefix]...)
	view.rows = append(view.rows, issueRows[startRow:endRow]...)

	// An issue counts as visible only with every one of its lines drawn.
	view.visibleIssues = 0
	for _, row := range rendered.layout.issueRow {
		if row >= startRow && row+issuerow.Height <= endRow {
			view.visibleIssues++
		}
	}
	return view
}

// issueAt is the index of the issue col draws on content line line, or -1 when
// that line holds a divider, the error row, or nothing. It reads the layout
// viewColumn windows its rows with and renders none of them: the pointer asks
// on every cell it crosses.
func (f frame) issueAt(col Column, now time.Time, line int) int {
	prefix := errorRows(col)
	if line < prefix || len(col.Rows) == 0 {
		return -1
	}

	layout := layoutRows(col, now)
	target := line - prefix
	if windowed(col) {
		start, _ := layout.window(col, f.innerHeight-prefix, layout.lines())
		target += start
	}
	for idx, row := range layout.issueRow {
		if target >= row && target < row+issuerow.Height {
			return idx
		}
	}
	return -1
}

// Hit is the board cell under a point: the column, and the issue drawn there.
// Row is -1 where the column draws no issue — its border, a divider, the error
// row, or the space below the last row.
type Hit struct {
	Column int
	Row    int
}

// HitTest reports what Render(state) draws at cell (x, y), with (0, 0) the
// first cell of the frame. ok is false outside every column.
func HitTest(state State, x, y int) (hit Hit, ok bool) {
	if len(state.Columns) == 0 {
		return Hit{}, false
	}
	f := layoutFrame(state)
	line := y
	if f.head != "" {
		line--
	}
	if x < 0 || line < 0 || line >= f.columnHeight {
		return Hit{}, false
	}

	left := 0
	for idx, width := range f.widths {
		if x >= left+width {
			left += width + columnGap
			continue
		}
		if x < left {
			return Hit{}, false
		}
		hit = Hit{Column: f.start + idx, Row: -1}
		content := line - 1
		if x == left || x == left+width-1 || content < 0 || content >= f.innerHeight {
			return hit, true
		}
		hit.Row = f.issueAt(state.Columns[hit.Column], state.Now, content)
		return hit, true
	}
	return Hit{}, false
}

// ColumnStart is where the window of drawn columns starts once the focus is on
// column focused: at start when that still draws the column, and otherwise
// moved only as far as it takes. The window stays under the pointer through a
// focus change, so a click or a wheel notch never slides another column there.
func ColumnStart(start, width, total, focused int) int {
	if total <= 0 {
		return 0
	}
	focused = textutil.Clamp(focused, 0, total-1)
	start, end := visibleColumnRange(width, total, start)
	switch {
	case focused < start:
		return focused
	case focused >= end:
		return start + focused - end + 1
	}
	return start
}

func visibleColumnRange(width, total, start int) (int, int) {
	if total <= 0 {
		return 0, 0
	}

	visible := min(maxVisibleColumns(width), total)
	start = textutil.Clamp(start, 0, total-visible)
	return start, start + visible
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
var skeletonRowCounts = [...]int{3, 4, 2, 3}

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
		})...)
	}
	return rows
}

// columnRows is the rendered content of one column. rows holds every line in
// draw order. prefix is the number of pinned rows (the inline error row) ahead
// of the issue rows, and layout is where each issue landed among rows[prefix:].
// layout is empty for the skeleton path, which draws no issues.
type columnRows struct {
	rows   []string
	prefix int
	layout rowLayout
}

// errorRows is the number of lines the inline error row pins at the top of
// col: one when the column carries an error, else zero.
func errorRows(col Column) int {
	if strings.TrimSpace(col.Error) != "" {
		return 1
	}
	return 0
}

// renderColumnRows renders col. hover is the index of the issue under the
// pointer, or -1.
func renderColumnRows(col Column, maxWidth, skeletonPhase, colIndex int, now time.Time, hover int) columnRows {
	var out columnRows

	// Inline error row at the top (if any).
	if errorRows(col) > 0 {
		errRow := textutil.TruncateString(styles.Glyphs.ToastWarn+" load failed: "+col.Error, maxWidth)
		out.rows = append(out.rows, errRow)
		out.prefix = 1
	}

	if col.Loading {
		if len(col.Rows) == 0 {
			// Cold-start: no data yet — show skeleton rows.
			out.rows = append(out.rows, skeletonRows(maxWidth, skeletonPhase, colIndex)...)
			return out
		}
		if col.ScrollOffset > 0 {
			// Load-more in flight: the user has scrolled deep and a background page
			// fetch is in progress. Render rows normally (not dimmed) and append a
			// single skeleton row at the end as a load-in-flight affordance.
			out.appendIssueRows(col, maxWidth, false, skeletonPhase, now, hover)
			out.rows = append(out.rows, issuerow.RenderCompactSkeleton(issuerow.SkeletonOpts{
				Width:  maxWidth,
				Seed:   0,
				Phase:  skeletonPhase,
				Styled: true,
			})...)
			return out
		}
		// Refresh: stale rows on screen while new data is in flight.
		// Dim the foreground with the current skeleton phase to signal motion.
		out.appendIssueRows(col, maxWidth, true, skeletonPhase, now, hover)
		return out
	}

	// Not loading — render normally.
	if out.prefix == 0 && len(col.Rows) == 0 {
		out.rows = append(out.rows, "(no issues)")
		return out
	}

	out.appendIssueRows(col, maxWidth, false, skeletonPhase, now, hover)
	return out
}

// appendIssueRows renders every issue of col in the order layoutRows placed
// them, drawing each age-marker divider ahead of the issue it precedes.
func (out *columnRows) appendIssueRows(col Column, maxWidth int, dim bool, phase int, now time.Time, hover int) {
	out.layout = layoutRows(col, now)
	markers := out.layout.markers
	for idx, issue := range col.Rows {
		for len(markers) > 0 && markers[0].Before == idx {
			out.rows = append(out.rows, renderAgeMarker(markers[0], maxWidth, true))
			markers = markers[1:]
		}
		out.rows = append(out.rows, issuerow.RenderCompact(issuerow.RenderConfig{
			Issue:    issue,
			Selected: idx == col.SelectedRow,
			Hovered:  idx == hover,
			Width:    maxWidth,
			Styled:   true,
			Dim:      dim,
			Phase:    phase,
		})...)
	}
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
