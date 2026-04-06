package board

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/mode"
	uiboard "github.com/hk9890/beads-workbench/internal/ui/board"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
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

type sectionState struct {
	id      string
	title   string
	issues  []domain.IssueSummary
	errText string
	loaded  bool
}

// Model is the standalone board mode controller backed by dashboard queries.
type Model struct {
	gateway   beads.BeadsGateway
	provider  dashboard.DashboardDefinitionProvider
	width     int
	height    int
	loading   bool
	loadError string

	dashboardID    string
	dashboardTitle string
	sections       []sectionState
	pendingLoads   int

	focusedColumn int
	selectedRow   map[int]int
}

var _ mode.Controller = (*Model)(nil)

// NewModel creates a board mode controller.
func NewModel(gateway beads.BeadsGateway, provider dashboard.DashboardDefinitionProvider) *Model {
	if provider == nil {
		provider = dashboard.NewBuiltInProvider()
	}

	return &Model{
		gateway:       gateway,
		provider:      provider,
		loading:       true,
		selectedRow:   map[int]int{},
		focusedColumn: 0,
	}
}

// ID returns board mode identifier.
func (m *Model) ID() mode.ID {
	return mode.Board
}

// Init loads built-in dashboards then section data from gateway.
func (m *Model) Init() tea.Cmd {
	m.loading = true
	return loadDashboardsCmd(m.provider)
}

// Update processes board-specific messages and keybindings.
func (m *Model) Update(msg tea.Msg) (mode.Controller, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil
	case dashboardsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.loadError = msg.err.Error()
			m.sections = nil
			m.pendingLoads = 0
			return m, nil
		}

		if len(msg.dashboards) == 0 {
			m.loadError = "no dashboards available"
			m.sections = nil
			m.pendingLoads = 0
			return m, nil
		}

		def := msg.dashboards[0]
		m.dashboardID = def.ID
		m.dashboardTitle = def.Title
		m.loadError = ""
		m.focusedColumn = 0
		m.sections = make([]sectionState, len(def.Sections))
		for i, section := range def.Sections {
			m.sections[i] = sectionState{id: section.ID, title: section.Title}
			m.selectedRow[i] = 0
		}
		m.pendingLoads = len(def.Sections)
		if m.pendingLoads == 0 {
			return m, nil
		}

		cmds := make([]tea.Cmd, 0, len(def.Sections))
		for i, section := range def.Sections {
			cmds = append(cmds, loadSectionCmd(m.gateway, i, section.Query))
		}
		return m, tea.Batch(cmds...)
	case sectionLoadedMsg:
		if msg.sectionIndex < 0 || msg.sectionIndex >= len(m.sections) {
			return m, nil
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
			m.normalizeFocus()
			return m, m.selectionChangedCmd()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			previous := m.focusedColumn
			if m.focusedColumn > 0 {
				m.focusedColumn--
			}
			m.normalizeSelectionForFocusedColumn()
			if m.focusedColumn != previous {
				return m, m.selectionChangedCmd()
			}
			return m, nil
		case "right", "l", "tab":
			previous := m.focusedColumn
			if m.focusedColumn < len(m.sections)-1 {
				m.focusedColumn++
			}
			m.normalizeSelectionForFocusedColumn()
			if m.focusedColumn != previous {
				return m, m.selectionChangedCmd()
			}
			return m, nil
		case "up", "k":
			previous := m.selectedRow[m.focusedColumn]
			m.moveRow(-1)
			if m.selectedRow[m.focusedColumn] != previous {
				return m, m.selectionChangedCmd()
			}
			return m, nil
		case "down", "j":
			previous := m.selectedRow[m.focusedColumn]
			m.moveRow(1)
			if m.selectedRow[m.focusedColumn] != previous {
				return m, m.selectionChangedCmd()
			}
			return m, nil
		case "enter", "o":
			if m.currentSelection() == nil {
				return m, nil
			}
			return m, func() tea.Msg {
				return mode.ActionRequestMsg{Mode: mode.Board, Action: mode.ActionOpenDetail}
			}
		case "r":
			m.loading = true
			m.loadError = ""
			m.pendingLoads = 0
			return m, loadDashboardsCmd(m.provider)
		}
	}

	return m, nil
}

// View renders the standalone board dashboard.
func (m *Model) View() string {
	if m.loading {
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
		rows := make([]uiboard.Row, 0, len(section.issues))
		selectedRow := m.selectedRow[colIdx]
		for rowIdx, issue := range section.issues {
			rows = append(rows, uiboard.Row{
				ID:       issue.ID,
				Title:    issue.Title,
				Type:     issue.Type,
				Status:   issue.Status,
				Priority: issue.Priority,
				Selected: colIdx == m.focusedColumn && rowIdx == selectedRow,
			})
		}
		columns = append(columns, uiboard.Column{Title: section.title, Rows: rows, Error: section.errText})
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

// CurrentSelection returns the current board issue selection.
func (m *Model) CurrentSelection() *mode.Selection {
	return m.currentSelection()
}

// IsLoading reports whether dashboard or section queries are still loading.
func (m *Model) IsLoading() bool {
	return m.loading || m.pendingLoads > 0
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

func loadDashboardsCmd(provider dashboard.DashboardDefinitionProvider) tea.Cmd {
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
