package board

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/mode"
	uiboard "github.com/hk9890/beads-workbench/internal/ui/board"
)

// Section titles for the fixed four-column layout.
const (
	sectionTitleNotReady   = "Not Ready"
	sectionTitleReady      = "Ready"
	sectionTitleInProgress = "In Progress"
	sectionTitleDone       = "Done"

	// dashboardTitle is the title shown in the board header.
	dashboardTitle = "Default"
)

// readyExplainLoadedMsg carries the result of a ReadyExplain gateway call.
type readyExplainLoadedMsg struct {
	result domain.ReadyExplainResult
	err    error
}

// inProgressLoadedMsg carries the result of a Query(in_progress) gateway call.
type inProgressLoadedMsg struct {
	issues []domain.IssueSummary
	err    error
}

// closedLoadedMsg carries the result of a Query(closed) gateway call.
type closedLoadedMsg struct {
	issues []domain.IssueSummary
	err    error
}

// closedCountLoadedMsg carries the result of a CountIssues(status=closed) gateway call.
type closedCountLoadedMsg struct {
	count int
	err   error
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
	refreshModeManual refreshMode = iota
	refreshModeAuto
)

type refreshAnchor struct {
	focusedColumn   int
	focusedRow      int
	selectedIssueID string
}

// Model is the standalone board mode controller backed by direct gateway calls.
type Model struct {
	gateway beads.BeadsGateway
	logger  *slog.Logger
	keys    config.ResolvedKeyBindings
	width   int
	height  int

	// columns holds the four fixed board columns after composition.
	columns []columnData

	// inflight is true while a startReload is in progress (set at startReload entry,
	// cleared by maybeCompose when all 4 results land and composition runs).
	// It is distinct from IsLoading() (column loading state) to avoid Init/cold-start
	// ambiguity: initialLoadingColumns() sets column loading=true at construction, but
	// inflight is false until the first startReload call.
	inflight bool

	// pendingResults counts how many of the 4 parallel gateway calls are outstanding.
	pendingResults int
	// partialReady / partialInProgress / partialClosed / partialClosedCount hold
	// in-flight results until all 4 arrive and composition can run.
	partialReadyExplain    *domain.ReadyExplainResult
	partialReadyExplainErr error
	partialInProgress      []domain.IssueSummary
	partialInProgressErr   error
	partialClosed          []domain.IssueSummary
	partialClosedErr       error
	partialClosedCount     int
	partialClosedCountErr  error

	focusedColumn int
	selectedRow   map[int]int

	refreshMode   refreshMode
	refreshAnchor *refreshAnchor
}

// NewModel creates a board mode controller.
// logger may be nil; a nil logger falls back to slog.Default().
func NewModel(gateway beads.BeadsGateway, logger *slog.Logger, resolved ...config.ResolvedKeyBindings) *Model {
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
		gateway:     gateway,
		logger:      logger,
		keys:        keys,
		selectedRow: map[int]int{},
		refreshMode: refreshModeManual,
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

// Init loads board data from the gateway via 3 parallel calls.
func (m *Model) Init() tea.Cmd {
	return m.startReload(refreshModeManual)
}

// Update processes board-specific messages and keybindings.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return nil

	case readyExplainLoadedMsg:
		m.partialReadyExplainErr = msg.err
		if msg.err == nil {
			m.partialReadyExplain = &msg.result
		}
		m.pendingResults--
		return m.maybeCompose()

	case inProgressLoadedMsg:
		m.partialInProgressErr = msg.err
		if msg.err == nil {
			m.partialInProgress = msg.issues
		}
		m.pendingResults--
		return m.maybeCompose()

	case closedLoadedMsg:
		m.partialClosedErr = msg.err
		if msg.err == nil {
			m.partialClosed = msg.issues
		}
		m.pendingResults--
		return m.maybeCompose()

	case closedCountLoadedMsg:
		m.partialClosedCountErr = msg.err
		if msg.err == nil {
			m.partialClosedCount = msg.count
		}
		m.pendingResults--
		return m.maybeCompose()

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
				return m.selectionChangedCmd()
			}
			return nil
		case m.keys.Match(config.BoardContext, config.BoardActionMoveDown, msg):
			previous := m.selectedRow[m.focusedColumn]
			m.moveRow(1)
			if m.selectedRow[m.focusedColumn] != previous {
				return m.selectionChangedCmd()
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
					"component", "board",
					"trigger", "board-manual")
				return nil
			}
			return m.startReload(refreshModeManual)
		}
	}

	return nil
}

// View renders the standalone board dashboard.
func (m *Model) View() string {
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

// closedLimit is the single source of truth for the Done column cap.
// It is max(50, sectionItemCapacity()) so the Done column always loads at
// least 50 items regardless of terminal height.
func (m *Model) closedLimit() int {
	cap := m.sectionItemCapacity()
	if cap < 50 {
		return 50
	}
	return cap
}

// CurrentSelection returns the current board issue selection.
func (m *Model) CurrentSelection() *mode.Selection {
	return m.currentSelection()
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
// loading, and dispatches all 3 gateway calls in a single tea.Batch. It is
// the single entry point for both manual reload and auto-refresh.
func (m *Model) startReload(rm refreshMode) tea.Cmd {
	// Defense-in-depth: guard against re-entrant calls from future callers that
	// may not check IsLoading() at the call site. The call-site guards in the
	// keyboard handler (5q6t.1) and AutoRefresh are the primary protection; this
	// guard is a second line of defense so the invariant is maintained regardless
	// of how startReload is called.
	if m.inflight {
		m.logger.Debug("startReload re-entry suppressed; refresh already in flight",
			"component", "board",
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

	m.pendingResults = 4
	m.partialReadyExplain = nil
	m.partialReadyExplainErr = nil
	m.partialInProgress = nil
	m.partialInProgressErr = nil
	m.partialClosed = nil
	m.partialClosedErr = nil
	m.partialClosedCount = 0
	m.partialClosedCountErr = nil
	m.refreshMode = rm
	m.refreshAnchor = anchor

	if rm == refreshModeManual {
		// Full reset: move focus to col 0, clear selection map, reset columns.
		m.focusedColumn = 0
		m.selectedRow = map[int]int{}
		m.columns = initialLoadingColumns()
	}

	cl := m.closedLimit()
	return tea.Batch(
		loadReadyExplainCmd(m.gateway),
		loadInProgressCmd(m.gateway),
		loadClosedCmd(m.gateway, cl),
		loadClosedCountCmd(m.gateway),
	)
}

// maybeCompose checks whether all 3 results have arrived and, if so, runs
// dashboard.Compose and settles the focus/selection.
func (m *Model) maybeCompose() tea.Cmd {
	if m.pendingResults > 0 {
		return nil
	}

	// All 3 results landed — compose.
	ready := m.partialReadyExplain
	if ready == nil {
		ready = &domain.ReadyExplainResult{}
	}

	cols := dashboard.Compose(dashboard.Inputs{
		Ready:       ready.Ready,
		Blocked:     ready.Blocked,
		InProgress:  m.partialInProgress,
		Closed:      m.partialClosed,
		ClosedLimit: m.closedLimit(),
		ClosedTotal: m.partialClosedCount,
	})

	// Emit warnings to slog.
	logger := m.logger.With("component", "dashboard")
	for _, w := range cols.Warnings {
		if w.Threshold == -1 {
			logger.Warn("backend sort assumption broken",
				"group", w.Group,
				"count", w.Count,
				"threshold", w.Threshold,
			)
		} else {
			logger.Warn("cardinality threshold exceeded",
				"group", w.Group,
				"count", w.Count,
				"threshold", w.Threshold,
			)
		}
	}

	// Determine per-region errors:
	// Not Ready + Ready ← ReadyExplain error
	// In Progress ← in_progress query error
	// Done ← closed query error
	readyExplainErr := m.partialReadyExplainErr
	inProgressErr := m.partialInProgressErr
	closedErr := m.partialClosedErr

	// Build the four fixed columns, clearing loading flags atomically.
	m.columns = []columnData{
		{title: sectionTitleNotReady, issues: cols.NotReady.Issues, total: cols.NotReady.Total, exact: cols.NotReady.TotalIsExact, loading: false, err: readyExplainErr},
		{title: sectionTitleReady, issues: cols.Ready.Issues, total: cols.Ready.Total, exact: cols.Ready.TotalIsExact, loading: false, err: readyExplainErr},
		{title: sectionTitleInProgress, issues: cols.InProgress.Issues, total: cols.InProgress.Total, exact: cols.InProgress.TotalIsExact, loading: false, err: inProgressErr},
		{title: sectionTitleDone, issues: cols.Done.Issues, total: cols.Done.Total, exact: cols.Done.TotalIsExact, loading: false, err: closedErr},
	}

	// Ensure selectedRow map has an entry for each column.
	for i := range m.columns {
		if _, ok := m.selectedRow[i]; !ok {
			m.selectedRow[i] = 0
		}
	}

	m.settleAfterRefreshLoad()
	// All results have landed and composition is complete — clear the in-flight flag
	// so future reload requests (keyboard or auto-refresh) are permitted.
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
	m.refreshMode = refreshModeManual
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
}

func (m *Model) selectionChangedCmd() tea.Cmd {
	selection := m.currentSelection()
	return func() tea.Msg {
		return mode.SelectionChangedMsg{Mode: mode.Board, Selection: selection}
	}
}

// loadReadyExplainCmd fires the ReadyExplain gateway call (uncapped).
func loadReadyExplainCmd(gateway beads.BeadsGateway) tea.Cmd {
	return func() tea.Msg {
		result, err := gateway.ReadyExplain(context.Background(), domain.ReadyExplainOptions{Limit: 0})
		return readyExplainLoadedMsg{result: result, err: err}
	}
}

// loadInProgressCmd fires the Query(status=in_progress) gateway call (uncapped).
func loadInProgressCmd(gateway beads.BeadsGateway) tea.Cmd {
	return func() tea.Msg {
		issues, err := gateway.Query(context.Background(), "status=in_progress", domain.QueryOptions{Limit: 0})
		return inProgressLoadedMsg{issues: issues, err: err}
	}
}

// loadClosedCmd fires the Query(status=closed) gateway call with the given limit.
func loadClosedCmd(gateway beads.BeadsGateway, limit int) tea.Cmd {
	return func() tea.Msg {
		issues, err := gateway.Query(context.Background(), "status=closed", domain.QueryOptions{
			IncludeClosed: true,
			SortBy:        domain.SortFieldClosedAt,
			SortOrder:     domain.SortDirectionDescending,
			Limit:         limit,
		})
		return closedLoadedMsg{issues: issues, err: err}
	}
}

// loadClosedCountCmd fires the CountIssues(status=closed) gateway call to get
// the real DB population count of closed issues, independent of any display cap.
func loadClosedCountCmd(gateway beads.BeadsGateway) tea.Cmd {
	return func() tea.Msg {
		result, err := gateway.CountIssues(context.Background(), domain.IssueCountQuery{
			Statuses: []string{"closed"},
		})
		if err != nil {
			return closedCountLoadedMsg{err: err}
		}
		return closedCountLoadedMsg{count: result.Total}
	}
}
