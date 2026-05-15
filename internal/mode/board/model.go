package board

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/mode"
	uiboard "github.com/hk9890/beads-workbench/internal/ui/board"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

type dashboardsLoadedMsg struct {
	dashboards []dashboard.Definition
	err        error
}

type sectionLoadedMsg struct {
	sectionIndex int
	issues       []domain.IssueSummary
	err          error
}

type countIssuesLoadedMsg struct {
	result domain.IssueCountResult
	err    error
}

type countReadyLoadedMsg struct {
	count int
	err   error
}

type countBlockedLoadedMsg struct {
	count int
	err   error
}

type sectionState struct {
	id      string
	title   string
	issues  []domain.IssueSummary
	errText string
	loaded  bool
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

// Model is the standalone board mode controller backed by dashboard queries.
type Model struct {
	gateway   beads.BeadsGateway
	provider  dashboard.Provider
	keys      config.ResolvedKeyBindings
	width     int
	height    int
	loading   bool
	loadError string

	dashboardID     string
	dashboardTitle  string
	sections        []sectionState
	sectionQueries  []dashboard.Query
	pendingLoads    int

	focusedColumn int
	selectedRow   map[int]int

	refreshMode   refreshMode
	refreshAnchor *refreshAnchor

	// True counts fetched after sections render (off the startup critical path).
	sectionCounts      domain.IssueCountResult
	readyCount         int
	blockedCount       int
	countsLoaded       bool
	readyCountLoaded   bool
	blockedCountLoaded bool
}

// NewModel creates a board mode controller.
func NewModel(gateway beads.BeadsGateway, provider dashboard.Provider, resolved ...config.ResolvedKeyBindings) *Model {
	if provider == nil {
		provider = dashboard.NewBuiltInProvider()
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

	return &Model{
		gateway:       gateway,
		provider:      provider,
		keys:          keys,
		loading:       true,
		selectedRow:   map[int]int{},
		focusedColumn: 0,
		refreshMode:   refreshModeManual,
	}
}

// Init loads built-in dashboards then section data from gateway.
func (m *Model) Init() tea.Cmd {
	m.loading = true
	return loadDashboardsCmd(m.provider)
}

// Update processes board-specific messages and keybindings.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return nil
	case dashboardsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.loadError = msg.err.Error()
			m.sections = nil
			m.pendingLoads = 0
			return nil
		}

		if err := dashboard.ValidateDefinitions(msg.dashboards); err != nil {
			m.loadError = err.Error()
			m.sections = nil
			m.pendingLoads = 0
			return nil
		}

		def := msg.dashboards[0]
		m.dashboardID = def.ID
		m.dashboardTitle = def.Title
		m.loadError = ""
		if m.refreshMode != refreshModeAuto {
			m.focusedColumn = 0
		}
		m.sections = make([]sectionState, len(def.Sections))
		m.sectionQueries = make([]dashboard.Query, len(def.Sections))
		m.selectedRow = make(map[int]int, len(def.Sections))
		for i, section := range def.Sections {
			m.sections[i] = sectionState{id: section.ID, title: section.Title}
			m.sectionQueries[i] = section.Query
			m.selectedRow[i] = 0
		}
		m.pendingLoads = len(def.Sections)
		if m.pendingLoads == 0 {
			return nil
		}

		capacity := m.sectionItemCapacity()
		cmds := make([]tea.Cmd, 0, len(def.Sections))
		for i, section := range def.Sections {
			q := applyQueryLimit(section.Query, capacity)
			cmds = append(cmds, loadSectionCmd(m.gateway, i, q))
		}
		return tea.Batch(cmds...)
	case sectionLoadedMsg:
		if msg.sectionIndex < 0 || msg.sectionIndex >= len(m.sections) {
			return nil
		}

		section := m.sections[msg.sectionIndex]
		section.loaded = true
		if msg.err != nil {
			section.errText = msg.err.Error()
			section.issues = nil
		} else {
			section.errText = ""
			section.issues = msg.issues
		}
		m.sections[msg.sectionIndex] = section
		if m.pendingLoads > 0 {
			m.pendingLoads--
		}

		if m.pendingLoads == 0 {
			m.settleAfterRefreshLoad()
			return tea.Batch(m.selectionChangedCmd(), m.dispatchCountCmds())
		}
		return nil
	case countIssuesLoadedMsg:
		if msg.err == nil {
			m.sectionCounts = msg.result
		}
		m.countsLoaded = true
		return nil
	case countReadyLoadedMsg:
		if msg.err == nil {
			m.readyCount = msg.count
		}
		m.readyCountLoaded = true
		return nil
	case countBlockedLoadedMsg:
		if msg.err == nil {
			m.blockedCount = msg.count
		}
		m.blockedCountLoaded = true
		return nil
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
			if m.focusedColumn < len(m.sections)-1 {
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
			m.loading = true
			m.loadError = ""
			m.pendingLoads = 0
			m.refreshMode = refreshModeManual
			m.refreshAnchor = nil
			m.invalidateCounts()
			return loadDashboardsCmd(m.provider)
		}
	}

	return nil
}

// View renders the standalone board dashboard.
func (m *Model) View() string {
	if m.loading || m.pendingLoads > 0 {
		total := len(m.sections)
		loaded := total - m.pendingLoads
		if total > 0 {
			return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(
				fmt.Sprintf("⏳ Loading board (%d / %d sections)…", loaded, total),
			)
		}
		return loading.View(loading.State{Scope: loading.ScopeBoard})
	}
	if strings.TrimSpace(m.loadError) != "" {
		return fmt.Sprintf("Unable to load board dashboards.\nError: %s", m.loadError)
	}
	if len(m.sections) == 0 {
		return "No board sections available."
	}

	columns := make([]uiboard.Column, 0, len(m.sections))
	for colIdx, section := range m.sections {
		selectedRow := -1
		if colIdx == m.focusedColumn {
			selectedRow = m.selectedRow[colIdx]
		}
		totalCount, countLoaded := m.sectionTotalCount(colIdx)
		columns = append(columns, uiboard.Column{
			Title:       section.title,
			Rows:        section.issues,
			SelectedRow: selectedRow,
			Error:       section.errText,
			TotalCount:  totalCount,
			CountLoaded: countLoaded,
		})
	}

	return uiboard.Render(uiboard.State{
		DashboardTitle: m.dashboardTitle,
		Columns:        columns,
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
// at the current terminal height. The formula matches the actual FormSection
// inner height: max(3, height-1) - 2 = max(1, height-3).
// When height is 0 (before the first tea.WindowSizeMsg), a safe default of 20
// is returned so that Init() fires queries with a reasonable limit.
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

// CurrentSelection returns the current board issue selection.
func (m *Model) CurrentSelection() *mode.Selection {
	return m.currentSelection()
}

// IsLoading reports whether dashboard or section queries are still loading.
func (m *Model) IsLoading() bool {
	return m.loading || m.pendingLoads > 0
}

// AutoRefresh reloads board data while preserving user context when possible.
func (m *Model) AutoRefresh() tea.Cmd {
	if m.IsLoading() {
		return nil
	}
	m.loading = true
	m.loadError = ""
	m.pendingLoads = 0
	m.refreshMode = refreshModeAuto
	m.refreshAnchor = m.captureRefreshAnchor()
	m.invalidateCounts()
	return loadDashboardsCmd(m.provider)
}

func (m *Model) currentSelection() *mode.Selection {
	if len(m.sections) == 0 || m.focusedColumn < 0 || m.focusedColumn >= len(m.sections) {
		return nil
	}

	issues := m.sections[m.focusedColumn].issues
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
	if len(m.sections) == 0 {
		m.focusedColumn = 0
		return
	}
	m.selectEarliestNonEmptyColumn()
	if m.focusedColumn < 0 {
		m.focusedColumn = 0
	}
	if m.focusedColumn >= len(m.sections) {
		m.focusedColumn = len(m.sections) - 1
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
	if len(m.sections) == 0 {
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

	m.focusedColumn = clamp(anchor.focusedColumn, 0, len(m.sections)-1)
	if len(m.sections[m.focusedColumn].issues) > 0 {
		m.selectedRow[m.focusedColumn] = clamp(anchor.focusedRow, 0, len(m.sections[m.focusedColumn].issues)-1)
		m.normalizeSelectionForFocusedColumn()
		return
	}

	m.selectEarliestNonEmptyColumn()
	m.normalizeSelectionForFocusedColumn()
}

func (m *Model) findIssue(issueID string) (int, int, bool) {
	for colIdx, section := range m.sections {
		for rowIdx, issue := range section.issues {
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
	if len(m.sections) == 0 {
		return
	}

	for idx, section := range m.sections {
		if len(section.issues) > 0 {
			m.focusedColumn = idx
			m.normalizeSelectionForFocusedColumn()
			return
		}
	}
}

func (m *Model) normalizeSelectionForFocusedColumn() {
	if len(m.sections) == 0 || m.focusedColumn < 0 || m.focusedColumn >= len(m.sections) {
		return
	}
	issues := m.sections[m.focusedColumn].issues
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
	if len(m.sections) == 0 || m.focusedColumn < 0 || m.focusedColumn >= len(m.sections) {
		return
	}
	issues := m.sections[m.focusedColumn].issues
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

func loadDashboardsCmd(provider dashboard.Provider) tea.Cmd {
	return func() tea.Msg {
		dashboards, err := provider.Dashboards(context.Background())
		return dashboardsLoadedMsg{dashboards: dashboards, err: err}
	}
}

func loadSectionCmd(gateway beads.BeadsGateway, sectionIndex int, query dashboard.Query) tea.Cmd {
	return func() tea.Msg {
		issues, err := runSectionQuery(context.Background(), gateway, query)
		return sectionLoadedMsg{sectionIndex: sectionIndex, issues: issues, err: err}
	}
}

// applyQueryLimit stamps limit onto a query copy before dispatch. Provider
// definitions carry Limit==0 (meaning "caller will set"). The board model
// applies sectionItemCapacity() here so the gateway receives a concrete limit.
func applyQueryLimit(q dashboard.Query, limit int) dashboard.Query {
	switch q.Type {
	case dashboard.QueryTypeListIssues:
		q.ListIssues.Limit = limit
	case dashboard.QueryTypeReadyIssues:
		q.ReadyIssues.Limit = limit
	case dashboard.QueryTypeBlockedIssues:
		q.BlockedIssues.Limit = limit
	}
	return q
}

func runSectionQuery(ctx context.Context, gateway beads.BeadsGateway, query dashboard.Query) ([]domain.IssueSummary, error) {
	switch query.Type {
	case dashboard.QueryTypeReadyIssues:
		return gateway.ReadyIssues(ctx, query.ReadyIssues)
	case dashboard.QueryTypeListIssues:
		return gateway.ListIssues(ctx, query.ListIssues)
	case dashboard.QueryTypeBlockedIssues:
		blocked, err := gateway.BlockedIssues(ctx, query.BlockedIssues)
		if err != nil {
			return nil, err
		}

		issues := make([]domain.IssueSummary, 0, len(blocked))
		for _, item := range blocked {
			issues = append(issues, item.Issue)
		}
		return issues, nil
	default:
		return nil, fmt.Errorf("unsupported dashboard query type: %s", query.Type)
	}
}

// invalidateCounts resets stored counts so the next load cycle re-fetches them.
func (m *Model) invalidateCounts() {
	m.sectionCounts = domain.IssueCountResult{}
	m.readyCount = 0
	m.blockedCount = 0
	m.countsLoaded = false
	m.readyCountLoaded = false
	m.blockedCountLoaded = false
}

// dispatchCountCmds fires the count queries that run after sections have loaded.
// CountIssues covers in_progress and closed status groups.
// Separate uncapped ReadyIssues/BlockedIssues calls cover those section types.
func (m *Model) dispatchCountCmds() tea.Cmd {
	hasReady := false
	hasBlocked := false
	for _, q := range m.sectionQueries {
		switch q.Type {
		case dashboard.QueryTypeReadyIssues:
			hasReady = true
		case dashboard.QueryTypeBlockedIssues:
			hasBlocked = true
		}
	}

	cmds := []tea.Cmd{countIssuesCmd(m.gateway)}
	if hasReady {
		cmds = append(cmds, countReadyCmd(m.gateway))
	} else {
		// No ready section; mark ready count as loaded (0) so View stays consistent.
		m.readyCountLoaded = true
	}
	if hasBlocked {
		cmds = append(cmds, countBlockedCmd(m.gateway))
	} else {
		m.blockedCountLoaded = true
	}
	return tea.Batch(cmds...)
}

// sectionTotalCount returns the true total count and whether it has loaded for
// the section at the given index. It uses sectionCounts for ListIssues sections
// and readyCount/blockedCount for ReadyIssues/BlockedIssues sections.
func (m *Model) sectionTotalCount(colIdx int) (count int, loaded bool) {
	if colIdx < 0 || colIdx >= len(m.sectionQueries) {
		return 0, false
	}
	q := m.sectionQueries[colIdx]
	switch q.Type {
	case dashboard.QueryTypeReadyIssues:
		return m.readyCount, m.readyCountLoaded
	case dashboard.QueryTypeBlockedIssues:
		return m.blockedCount, m.blockedCountLoaded
	case dashboard.QueryTypeListIssues:
		if !m.countsLoaded {
			return 0, false
		}
		// Find the count for the primary status in this query.
		if len(q.ListIssues.Statuses) == 0 {
			return m.sectionCounts.Total, true
		}
		status := q.ListIssues.Statuses[0]
		for _, group := range m.sectionCounts.Groups {
			if group.Status == status {
				return group.Count, true
			}
		}
		// Status group absent means zero issues in that state.
		return 0, true
	}
	return 0, false
}

func countIssuesCmd(gateway beads.BeadsGateway) tea.Cmd {
	return func() tea.Msg {
		result, err := gateway.CountIssues(context.Background(), domain.IssueCountQuery{})
		return countIssuesLoadedMsg{result: result, err: err}
	}
}

func countReadyCmd(gateway beads.BeadsGateway) tea.Cmd {
	return func() tea.Msg {
		// Limit: 0 means uncapped — we count all ready issues.
		issues, err := gateway.ReadyIssues(context.Background(), domain.ReadyIssuesQuery{Limit: 0})
		return countReadyLoadedMsg{count: len(issues), err: err}
	}
}

func countBlockedCmd(gateway beads.BeadsGateway) tea.Cmd {
	return func() tea.Msg {
		// Limit: 0 means uncapped — we count all blocked issues.
		blocked, err := gateway.BlockedIssues(context.Background(), domain.BlockedIssuesQuery{Limit: 0})
		return countBlockedLoadedMsg{count: len(blocked), err: err}
	}
}
