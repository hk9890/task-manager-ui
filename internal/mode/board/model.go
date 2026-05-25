package board

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/repository"
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

// dashboardLoadedMsg carries the result of a Dashboard repository call.
type dashboardLoadedMsg struct {
	data repository.DashboardData
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
	refreshModeManual refreshMode = iota
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

	refreshMode   refreshMode
	refreshAnchor *refreshAnchor
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
		ctx:         ctx,
		repo:        repo,
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

// Init loads board data from the repository via 3 parallel calls.
func (m *Model) Init() tea.Cmd {
	return m.startReload(refreshModeManual)
}

// Update processes board-specific messages and keybindings.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return nil

	case dashboardLoadedMsg:
		return m.compose(msg.data, msg.err)

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
					"trigger", "board-manual")
				return nil
			}
			return m.startReload(refreshModeManual)
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
// the single entry point for both manual reload and auto-refresh.
func (m *Model) startReload(rm refreshMode) tea.Cmd {
	// Defense-in-depth: guard against re-entrant calls from future callers that
	// may not check IsLoading() at the call site. The call-site guards in the
	// keyboard handler (5q6t.1) and AutoRefresh are the primary protection; this
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

	if rm == refreshModeManual {
		// Full reset: move focus to col 0, clear selection map, reset columns.
		m.focusedColumn = 0
		m.selectedRow = map[int]int{}
		m.columns = initialLoadingColumns()
	}

	return loadDashboardCmd(m.ctx, m.repo)
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

	// Ensure selectedRow map has an entry for each column.
	for i := range m.columns {
		if _, ok := m.selectedRow[i]; !ok {
			m.selectedRow[i] = 0
		}
	}

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

// loadDashboardCmd fires the Dashboard repository call and wraps the result
// in a dashboardLoadedMsg. ctx is the model's lifetime context set at
// construction; it does not change after NewModel returns, so reading it
// inside the closure at BubbleTea-execute time is safe.
func loadDashboardCmd(ctx context.Context, repo repository.Repository) tea.Cmd {
	return func() tea.Msg {
		data, err := repo.Dashboard(ctx)
		return dashboardLoadedMsg{data: data, err: err}
	}
}
