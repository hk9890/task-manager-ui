package board

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/dashboard"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
	"github.com/hk9890/task-manager-ui/internal/ui/scroll"
)

// Section titles for the fixed four-column layout.
const (
	sectionTitleNotReady   = "Not Ready"
	sectionTitleReady      = "Ready"
	sectionTitleInProgress = "In Progress"
	sectionTitleDone       = "Done"

	// dashboardTitle is the title shown in the board header.
	dashboardTitle = "Default"

	// doneColumnIndex is the fixed index of the Done column in m.columns.
	doneColumnIndex = 3

	// loadMoreThreshold is the number of remaining loaded items below which
	// the model dispatches a background load-more when the cursor approaches
	// the end of the Done column.
	loadMoreThreshold = 5
)

// dashboardLoadedMsg carries the result of a Dashboard repository call.
type dashboardLoadedMsg struct {
	data repository.DashboardData
	err  error
}

// loadMoreClosedDoneMsg carries the result of a load-more Dashboard call for
// the Done column. opts is echoed back from the originating loadMoreClosedCmd
// so the handler can verify it is processing the expected page (offset / limit).
// On err != nil the handler clears doneLoadInFlight and surfaces the error on
// the Done column; on success it merges via dashboard.Compose with PriorClosed
// set to the current Done issues.
type loadMoreClosedDoneMsg struct {
	data repository.DashboardData
	opts repository.DashboardOptions
	err  error
}

// columnData holds the loaded data for one board column after composition.
type columnData struct {
	title   string
	issues  []domain.IssueSummary
	total   int
	exact   bool
	loading bool
	err     error
}

type refreshMode int

const (
	// refreshModeReload performs a full reset of board state (focus, selection,
	// scroll, columns). It is used for the cold-start load (Init) and the
	// user-initiated reload (r key).
	refreshModeReload refreshMode = iota
	// refreshModeAuto is a background refresh that preserves the current
	// selection anchor instead of resetting board state.
	refreshModeAuto
)

type refreshAnchor struct {
	focusedColumn   int
	focusedRow      int
	selectedIssueID string
}

// Model is the standalone board mode controller backed by repository calls.
type Model struct {
	ctx    context.Context
	repo   repository.Repository
	logger *slog.Logger
	keys   config.ResolvedKeyBindings
	width  int
	height int

	// columns holds the four fixed board columns after composition.
	columns []columnData

	// inflight is true while a startReload is in progress (set at startReload entry,
	// cleared by compose when the dashboard result lands and composition runs).
	// It is distinct from IsLoading() (column loading state) to avoid Init/cold-start
	// ambiguity: initialLoadingColumns() sets column loading=true at construction, but
	// inflight is false until the first startReload call.
	inflight bool

	focusedColumn int
	selectedRow   map[int]int
	// scrollOffset tracks the viewport top-row offset for each column so that
	// the selected row is always within the visible window.
	scrollOffset map[int]int

	refreshMode   refreshMode
	refreshAnchor *refreshAnchor

	// --- Done column load-more state ---

	// doneLoadedCount is the number of closed issues currently in
	// Done.Issues after the last successful page (initial or load-more).
	// It is set to len(merged) after each composition that touches the Done
	// column, so it stays in sync even when the composer dedups on ID conflict.
	doneLoadedCount int

	// doneLoadInFlight is true while a loadMoreClosedCmd is outstanding.
	// It prevents parallel load-more dispatches.
	doneLoadInFlight bool

	// doneClosedTotal is the authoritative DB total from the last Dashboard
	// response. Used to decide whether there are more pages to fetch.
	doneClosedTotal int
}

// NewModel creates a board mode controller.
// ctx is stored on the model and used for repository calls; callers should
// pass the application lifecycle context so repository operations can be
// cancelled when the app exits.
// logger may be nil; a nil logger falls back to slog.Default().
func NewModel(ctx context.Context, repo repository.Repository, logger *slog.Logger, resolved ...config.ResolvedKeyBindings) *Model {
	if logger == nil {
		logger = slog.Default()
	}
	var keys config.ResolvedKeyBindings
	if len(resolved) > 0 {
		keys = resolved[0]
	} else {
		var err error
		keys, err = config.ResolveKeyBindings(config.DefaultKeyBindings())
		if err != nil {
			panic(fmt.Sprintf("invalid default board keybindings: %v", err))
		}
	}

	m := &Model{
		ctx:          ctx,
		repo:         repo,
		logger:       logger,
		keys:         keys,
		selectedRow:  map[int]int{},
		scrollOffset: map[int]int{},
		refreshMode:  refreshModeReload,
	}
	m.columns = initialLoadingColumns()
	return m
}

// initialLoadingColumns returns the 4 fixed board columns in their cold-start
// loading state (loading=true, no issues, no error).
func initialLoadingColumns() []columnData {
	return []columnData{
		{title: sectionTitleNotReady, loading: true},
		{title: sectionTitleReady, loading: true},
		{title: sectionTitleInProgress, loading: true},
		{title: sectionTitleDone, loading: true},
	}
}

// Init loads board data from the repository via 3 parallel calls.
func (m *Model) Init() tea.Cmd {
	return m.startReload(refreshModeReload)
}

// Update processes board-specific messages and keybindings.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return nil

	case dashboardLoadedMsg:
		return m.compose(msg.data, msg.err)

	case loadMoreClosedDoneMsg:
		return m.applyLoadMoreClosed(msg)

	case tea.KeyMsg:
		switch {
		case m.keys.Match(config.BoardContext, config.BoardActionMoveLeft, msg):
			previous := m.focusedColumn
			if m.focusedColumn > 0 {
				m.focusedColumn--
			}
			m.normalizeSelectionForFocusedColumn()
			if m.focusedColumn != previous {
				return m.selectionChangedCmd()
			}
			return nil
		case m.keys.Match(config.BoardContext, config.BoardActionMoveRight, msg):
			previous := m.focusedColumn
			if m.focusedColumn < len(m.columns)-1 {
				m.focusedColumn++
			}
			m.normalizeSelectionForFocusedColumn()
			if m.focusedColumn != previous {
				return m.selectionChangedCmd()
			}
			return nil
		case m.keys.Match(config.BoardContext, config.BoardActionMoveUp, msg):
			previous := m.selectedRow[m.focusedColumn]
			m.moveRow(-1)
			if m.selectedRow[m.focusedColumn] != previous {
				return tea.Batch(m.selectionChangedCmd(), m.maybeLoadMoreClosed())
			}
			return nil
		case m.keys.Match(config.BoardContext, config.BoardActionMoveDown, msg):
			previous := m.selectedRow[m.focusedColumn]
			m.moveRow(1)
			if m.selectedRow[m.focusedColumn] != previous {
				return tea.Batch(m.selectionChangedCmd(), m.maybeLoadMoreClosed())
			}
			return nil
		case m.keys.Match(config.BoardContext, config.BoardActionOpenDetail, msg):
			if m.currentSelection() == nil {
				return nil
			}
			return func() tea.Msg {
				return mode.ActionRequestMsg{Mode: mode.Board, Action: mode.ActionOpenDetail}
			}
		case m.keys.Match(config.BoardContext, config.BoardActionReload, msg):
			if m.inflight {
				m.logger.Debug("manual board refresh suppressed; refresh already in flight",
					"trigger", "board-manual")
				return nil
			}
			return m.startReload(refreshModeReload)
		case m.keys.Match(config.BoardContext, config.BoardActionLoadMore, msg):
			// Explicit load-more: dispatch regardless of cursor proximity,
			// but still respect the in-flight guard and "nothing more" check.
			if m.focusedColumn != doneColumnIndex {
				return nil
			}
			return m.dispatchLoadMoreClosed()
		}
	}

	return nil
}

// View renders the standalone board dashboard.
func (m *Model) View(skeletonPhase int) string {
	if len(m.columns) == 0 {
		return "No board sections available."
	}

	uiColumns := make([]uiboard.Column, 0, len(m.columns))
	for colIdx, col := range m.columns {
		selectedRow := -1
		if colIdx == m.focusedColumn {
			selectedRow = m.selectedRow[colIdx]
		}
		errStr := ""
		if col.err != nil {
			errStr = col.err.Error()
		}
		uiColumns = append(uiColumns, uiboard.Column{
			Title:        col.title,
			Rows:         col.issues,
			SelectedRow:  selectedRow,
			ScrollOffset: m.scrollOffset[colIdx],
			Total:        col.total,
			TotalIsExact: col.exact,
			Loading:      col.loading,
			Error:        errStr,
		})
	}

	return uiboard.Render(uiboard.State{
		DashboardTitle: dashboardTitle,
		Columns:        uiColumns,
		FocusedColumn:  m.focusedColumn,
		Width:          m.width,
		Height:         m.height,
		SkeletonPhase:  skeletonPhase,
	})
}

// SetSize updates render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// sectionItemCapacity returns the number of issue rows that fit in a section
// at the current terminal height.
func (m *Model) sectionItemCapacity() int {
	if m.height == 0 {
		return 20 // safe default before first WindowSizeMsg
	}
	rows := m.height - 3
	if rows < 1 {
		rows = 1
	}
	return rows
}

// CurrentSelection returns the active issue selection for tests. Production
// code uses the unexported currentSelection helper directly; this exported
// wrapper exists as a test seam so model_test.go can assert on selection
// state without exposing internals to other packages.
func (m *Model) CurrentSelection() *mode.Selection {
	return m.currentSelection()
}

// ScrollOffsetForColumn returns the per-column scroll offset for the given
// column index. This is a test seam exported for assertions in model_test.go.
func (m *Model) ScrollOffsetForColumn(colIdx int) int {
	return m.scrollOffset[colIdx]
}

// IsLoading reports whether any column is still in its loading state.
// This reflects the visual loading state of columns (true during cold-start and
// while a refresh is in flight, false once composition completes).
// For refresh-concurrency guards, use m.inflight directly — it is false at
// construction so Init()/cold-start is not ambiguous.
func (m *Model) IsLoading() bool {
	for _, col := range m.columns {
		if col.loading {
			return true
		}
	}
	return false
}

// AutoRefresh reloads board data while preserving user context when possible.
func (m *Model) AutoRefresh() tea.Cmd {
	if m.inflight {
		return nil
	}
	return m.startReload(refreshModeAuto)
}

// startReload captures the selection anchor (if auto), marks all columns
// loading, and dispatches all 3 repository calls in a single tea.Batch. It is
// the single entry point for initial load, manual reload, and auto-refresh.
func (m *Model) startReload(rm refreshMode) tea.Cmd {
	// Defense-in-depth: guard against re-entrant calls from future callers that
	// may not check IsLoading() at the call site. The call-site guards in the
	// keyboard handler and AutoRefresh are the primary protection; this
	// guard is a second line of defense so the invariant is maintained regardless
	// of how startReload is called.
	if m.inflight {
		m.logger.Debug("startReload re-entry suppressed; refresh already in flight",
			"mode", rm)
		return nil
	}
	m.inflight = true

	// Capture anchor before clearing state so it reflects the current selection.
	var anchor *refreshAnchor
	if rm == refreshModeAuto {
		anchor = m.captureRefreshAnchor()
	}

	// Mark all columns as loading, preserve existing issues for stale rendering.
	for i := range m.columns {
		m.columns[i].loading = true
		m.columns[i].err = nil
	}

	m.refreshMode = rm
	m.refreshAnchor = anchor

	// Reset load-more state before any new full reload so the next compose
	// sets doneLoadedCount from scratch. This is the "r resets page 1"
	// contract; this reset is the safety net for all reload modes.
	m.doneLoadedCount = 0
	m.doneLoadInFlight = false

	if rm == refreshModeReload {
		// Full reset: move focus to col 0, clear selection and scroll maps, reset columns.
		m.focusedColumn = 0
		m.selectedRow = map[int]int{}
		m.scrollOffset = map[int]int{}
		m.columns = initialLoadingColumns()
	}

	// Build opts before entering the Cmd closure so no model state is read
	// inside the closure (Bubble Tea runs Cmds outside the Update loop — per
	// grill Q4, reading model state there is a race smell).
	opts := repository.DashboardOptions{ClosedLimit: m.sectionItemCapacity()}
	return loadDashboardCmd(m.ctx, m.repo, opts)
}

// compose runs dashboard.Compose from a single DashboardData result and
// settles the focus/selection. It is the single composition entry point for
// both successful and error dashboard loads.
func (m *Model) compose(data repository.DashboardData, loadErr error) tea.Cmd {
	cols := dashboard.Compose(dashboard.Inputs{
		Ready:         data.ReadyExplain.Ready,
		Blocked:       data.ReadyExplain.Blocked,
		StoredBlocked: data.Blocked,
		InProgress:    data.InProgress,
		Closed:        data.Closed,
		ClosedLimit:   m.sectionItemCapacity(),
		ClosedTotal:   data.ClosedTotal,
	})

	// Emit warnings to slog.
	for _, w := range cols.Warnings {
		if w.Threshold == -1 {
			m.logger.Warn("backend sort assumption broken",
				"group", w.Group,
				"count", w.Count,
				"threshold", w.Threshold,
			)
		} else {
			m.logger.Warn("cardinality threshold exceeded",
				"group", w.Group,
				"count", w.Count,
				"threshold", w.Threshold,
			)
		}
	}

	// Build the four fixed columns, clearing loading flags atomically.
	// On error the same error is shown on all columns; on success err is nil.
	m.columns = []columnData{
		{title: sectionTitleNotReady, issues: cols.NotReady.Issues, total: cols.NotReady.Total, exact: cols.NotReady.TotalIsExact, loading: false, err: loadErr},
		{title: sectionTitleReady, issues: cols.Ready.Issues, total: cols.Ready.Total, exact: cols.Ready.TotalIsExact, loading: false, err: loadErr},
		{title: sectionTitleInProgress, issues: cols.InProgress.Issues, total: cols.InProgress.Total, exact: cols.InProgress.TotalIsExact, loading: false, err: loadErr},
		{title: sectionTitleDone, issues: cols.Done.Issues, total: cols.Done.Total, exact: cols.Done.TotalIsExact, loading: false, err: loadErr},
	}

	// Ensure selectedRow and scrollOffset maps have an entry for each column.
	for i := range m.columns {
		if _, ok := m.selectedRow[i]; !ok {
			m.selectedRow[i] = 0
		}
		if _, ok := m.scrollOffset[i]; !ok {
			m.scrollOffset[i] = 0
		}
	}

	// Initialize load-more counters from the initial Dashboard result.
	// doneLoadedCount = len of the Done issues slice produced by Compose
	// (authoritative after dedup); doneClosedTotal = DB count from response.
	m.doneLoadedCount = len(m.columns[doneColumnIndex].issues)
	m.doneClosedTotal = data.ClosedTotal

	m.settleAfterRefreshLoad()
	// Composition complete — clear the in-flight flag so future reload requests
	// (keyboard or auto-refresh) are permitted.
	m.inflight = false
	return m.selectionChangedCmd()
}

func (m *Model) currentSelection() *mode.Selection {
	if len(m.columns) == 0 || m.focusedColumn < 0 || m.focusedColumn >= len(m.columns) {
		return nil
	}

	issues := m.columns[m.focusedColumn].issues
	if len(issues) == 0 {
		return nil
	}

	row := m.selectedRow[m.focusedColumn]
	if row < 0 || row >= len(issues) {
		row = 0
	}

	selection := mode.Selection{Issue: issues[row]}
	return &selection
}

func (m *Model) normalizeFocus() {
	if len(m.columns) == 0 {
		m.focusedColumn = 0
		return
	}
	m.selectEarliestNonEmptyColumn()
	if m.focusedColumn < 0 {
		m.focusedColumn = 0
	}
	if m.focusedColumn >= len(m.columns) {
		m.focusedColumn = len(m.columns) - 1
	}
	m.normalizeSelectionForFocusedColumn()
}

func (m *Model) settleAfterRefreshLoad() {
	if m.refreshMode == refreshModeAuto {
		m.restoreFromAnchor(m.refreshAnchor)
	} else {
		m.normalizeFocus()
	}
	m.refreshMode = refreshModeReload
	m.refreshAnchor = nil
}

func (m *Model) captureRefreshAnchor() *refreshAnchor {
	anchor := &refreshAnchor{focusedColumn: m.focusedColumn, focusedRow: m.selectedRow[m.focusedColumn]}
	if selection := m.currentSelection(); selection != nil {
		anchor.selectedIssueID = selection.Issue.ID
	}
	return anchor
}

func (m *Model) restoreFromAnchor(anchor *refreshAnchor) {
	if len(m.columns) == 0 {
		m.focusedColumn = 0
		return
	}

	if anchor == nil {
		m.normalizeFocus()
		return
	}

	if anchor.selectedIssueID != "" {
		if col, row, ok := m.findIssue(anchor.selectedIssueID); ok {
			m.focusedColumn = col
			m.selectedRow[col] = row
			m.normalizeSelectionForFocusedColumn()
			return
		}
	}

	m.focusedColumn = clamp(anchor.focusedColumn, 0, len(m.columns)-1)
	if len(m.columns[m.focusedColumn].issues) > 0 {
		m.selectedRow[m.focusedColumn] = clamp(anchor.focusedRow, 0, len(m.columns[m.focusedColumn].issues)-1)
		m.normalizeSelectionForFocusedColumn()
		return
	}

	m.selectEarliestNonEmptyColumn()
	m.normalizeSelectionForFocusedColumn()
}

func (m *Model) findIssue(issueID string) (int, int, bool) {
	for colIdx, col := range m.columns {
		for rowIdx, issue := range col.issues {
			if issue.ID == issueID {
				return colIdx, rowIdx, true
			}
		}
	}
	return 0, 0, false
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func (m *Model) selectEarliestNonEmptyColumn() {
	if len(m.columns) == 0 {
		return
	}

	for idx, col := range m.columns {
		if len(col.issues) > 0 {
			m.focusedColumn = idx
			m.normalizeSelectionForFocusedColumn()
			return
		}
	}
}

func (m *Model) normalizeSelectionForFocusedColumn() {
	if len(m.columns) == 0 || m.focusedColumn < 0 || m.focusedColumn >= len(m.columns) {
		return
	}
	issues := m.columns[m.focusedColumn].issues
	if len(issues) == 0 {
		m.selectedRow[m.focusedColumn] = 0
		return
	}

	idx := m.selectedRow[m.focusedColumn]
	if idx < 0 {
		idx = 0
	}
	if idx >= len(issues) {
		idx = len(issues) - 1
	}
	m.selectedRow[m.focusedColumn] = idx
}

func (m *Model) moveRow(delta int) {
	if len(m.columns) == 0 || m.focusedColumn < 0 || m.focusedColumn >= len(m.columns) {
		return
	}
	issues := m.columns[m.focusedColumn].issues
	if len(issues) == 0 {
		m.selectedRow[m.focusedColumn] = 0
		return
	}

	idx := m.selectedRow[m.focusedColumn] + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(issues) {
		idx = len(issues) - 1
	}
	m.selectedRow[m.focusedColumn] = idx
	capacity := m.sectionItemCapacity()
	// When the focused column carries an inline error, the renderer pins that
	// error row at the top of the column and shows one fewer issue row (see
	// internal/ui/board Render). Reserve that row here so EnsureVisible keeps the
	// selected row inside the renderer's actual issue window rather than letting
	// the bottom row clip off-screen.
	if m.columns[m.focusedColumn].err != nil && capacity > 1 {
		capacity--
	}
	m.scrollOffset[m.focusedColumn] = scroll.EnsureVisible(
		m.scrollOffset[m.focusedColumn],
		idx,
		capacity,
	)
}

func (m *Model) selectionChangedCmd() tea.Cmd {
	selection := m.currentSelection()
	return func() tea.Msg {
		return mode.SelectionChangedMsg{Mode: mode.Board, Selection: selection}
	}
}

// maybeLoadMoreClosed checks whether the cursor in the Done column is within
// loadMoreThreshold rows of the end of the loaded slice and, if so, dispatches
// a background load-more (see dispatchLoadMoreClosed). It is called after every
// move-row event when the focused column is Done.
//
// Returns nil (no cmd) when:
//   - focused column is not Done,
//   - a load is already in flight,
//   - all available closed issues are already loaded, or
//   - the cursor is not yet within the threshold window.
func (m *Model) maybeLoadMoreClosed() tea.Cmd {
	if m.focusedColumn != doneColumnIndex {
		return nil
	}
	selectedRow := m.selectedRow[m.focusedColumn]
	remaining := m.doneLoadedCount - selectedRow
	if remaining >= loadMoreThreshold {
		return nil
	}
	return m.dispatchLoadMoreClosed()
}

// dispatchLoadMoreClosed sets doneLoadInFlight and returns a loadMoreClosedCmd
// if there are more pages to fetch. Returns nil if already in flight or all
// pages are loaded.
func (m *Model) dispatchLoadMoreClosed() tea.Cmd {
	if m.doneLoadInFlight {
		m.logger.Debug("load-more suppressed; already in flight")
		return nil
	}
	// Nothing (more) to load: loaded count has reached the DB total. This also
	// covers an empty Done column (total == 0, loaded == 0): the initial Dashboard
	// always sets doneClosedTotal, so 0 means "no closed issues", not "unknown".
	// The previous `&& doneClosedTotal > 0` clause let the empty case fall through
	// and dispatch a fresh backend fetch on every cursor move / load-more keypress.
	if m.doneLoadedCount >= m.doneClosedTotal {
		m.logger.Debug("load-more suppressed; all closed issues loaded",
			"loaded", m.doneLoadedCount,
			"total", m.doneClosedTotal,
		)
		return nil
	}
	pageSize := m.closedPageSize()
	opts := repository.DashboardOptions{
		ClosedLimit:  pageSize,
		ClosedOffset: m.doneLoadedCount,
	}
	m.doneLoadInFlight = true
	m.logger.Debug("dispatching load-more for Done column",
		"offset", opts.ClosedOffset,
		"limit", opts.ClosedLimit,
	)
	return loadMoreClosedCmd(m.ctx, m.repo, opts)
}

// applyLoadMoreClosed processes an incoming loadMoreClosedDoneMsg: clears the
// in-flight flag, surfaces errors on the Done column, and on success merges
// the new page into Done.Issues via dashboard.Compose with PriorClosed set.
func (m *Model) applyLoadMoreClosed(msg loadMoreClosedDoneMsg) tea.Cmd {
	m.doneLoadInFlight = false

	if msg.err != nil {
		m.logger.Warn("load-more for Done column failed", "err", msg.err)
		// Surface the error on the Done column so the user gets feedback.
		if len(m.columns) > doneColumnIndex {
			m.columns[doneColumnIndex].err = msg.err
		}
		return nil
	}

	// Capture current Done issues as PriorClosed so Compose can merge.
	var priorClosed []domain.IssueSummary
	if len(m.columns) > doneColumnIndex {
		priorClosed = m.columns[doneColumnIndex].issues
	}

	cols := dashboard.Compose(dashboard.Inputs{
		Closed:      msg.data.Closed,
		ClosedTotal: msg.data.ClosedTotal,
		PriorClosed: priorClosed,
	})

	// Update doneClosedTotal in case it changed (e.g. issues added/closed mid-session).
	m.doneClosedTotal = msg.data.ClosedTotal

	// doneLoadedCount is derived from the merged slice length (after dedup),
	// not blindly from len(msg.data.Closed), to stay in sync after ID conflicts.
	m.doneLoadedCount = len(cols.Done.Issues)

	// Replace the Done column in-place; all other columns are unchanged.
	if len(m.columns) > doneColumnIndex {
		m.columns[doneColumnIndex] = columnData{
			title:  sectionTitleDone,
			issues: cols.Done.Issues,
			total:  cols.Done.Total,
			exact:  cols.Done.TotalIsExact,
		}
	}

	// The merged slice may have shifted the issue under the cursor (dedup/replace
	// on ID conflict, or a refreshed copy of the selected issue). Re-clamp the
	// selection and, when the Done column is focused (the only column load-more
	// touches), re-emit SelectionChangedMsg so the shell's stored selection,
	// header, and any open detail pane stay in sync with the highlighted row
	// rather than referencing a stale issue.
	m.normalizeSelectionForFocusedColumn()
	if m.focusedColumn == doneColumnIndex {
		return m.selectionChangedCmd()
	}
	return nil
}

// closedPageSize returns the number of closed issues to request per load-more
// page. It is 2× sectionItemCapacity (so the visible window is always full
// after a single page), with a floor of 50 to avoid excessive round-trips on
// large terminals.
func (m *Model) closedPageSize() int {
	cap := m.sectionItemCapacity()
	if cap < 1 {
		cap = 1
	}
	size := 2 * cap
	if size < 50 {
		size = 50
	}
	return size
}

// loadDashboardCmd fires the Dashboard repository call and wraps the result
// in a dashboardLoadedMsg. ctx is the model lifetime context (set at
// NewModel) and opts is the per-call options struct; both are captured at
// closure-construction time and read inside the closure when BubbleTea
// later runs the Cmd.
func loadDashboardCmd(ctx context.Context, repo repository.Repository, opts repository.DashboardOptions) tea.Cmd {
	return func() tea.Msg {
		data, err := repo.Dashboard(ctx, opts)
		return dashboardLoadedMsg{data: data, err: err}
	}
}

// loadMoreClosedCmd fires a Dashboard call scoped to the next closed page
// (offset=doneLoadedCount, limit=pageSize) and wraps the result in a
// loadMoreClosedDoneMsg. opts is captured at construction time and echoed
// back in the message so the handler can assert it is receiving the expected
// page.
func loadMoreClosedCmd(ctx context.Context, repo repository.Repository, opts repository.DashboardOptions) tea.Cmd {
	return func() tea.Msg {
		data, err := repo.Dashboard(ctx, opts)
		return loadMoreClosedDoneMsg{data: data, opts: opts, err: err}
	}
}
