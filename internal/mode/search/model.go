package search

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/mode"
	uisearch "github.com/hk9890/beads-workbench/internal/ui/search"
)

const defaultSearchLimit = 40

type searchLoadedMsg struct {
	issues []domain.IssueSummary
	err    error
}

type selectionAnchor struct {
	issueID string
	row     int
}

// Model is the standalone search mode controller.
type Model struct {
	gateway beads.BeadsGateway
	keys    config.ResolvedKeyBindings

	width  int
	height int

	loading bool
	errText string

	query string
	focus uisearch.FocusPane

	results     []domain.IssueSummary
	selectedRow int
	typing      bool

	pendingSelectionAnchor *selectionAnchor
}

// NewModel creates a search mode controller.
func NewModel(gateway beads.BeadsGateway, resolved ...config.ResolvedKeyBindings) *Model {
	keys := config.ResolvedKeyBindings{}
	if len(resolved) > 0 {
		keys = resolved[0]
	} else {
		var err error
		keys, err = config.ResolveKeyBindings(config.DefaultKeyBindings())
		if err != nil {
			panic(err)
		}
	}
	return &Model{
		gateway: gateway,
		keys:    keys,
		focus:   uisearch.FocusQuery,
	}
}

// Init loads default all-issues search results for empty query.
func (m *Model) Init() tea.Cmd {
	m.loading = true
	m.errText = ""
	m.typing = false
	return loadSearchCmd(m.gateway, domain.SearchIssuesQuery{Limit: defaultSearchLimit, Offset: 0})
}

// Update processes search-specific messages and keybindings.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return nil
	case searchLoadedMsg:
		m.loading = false
		m.typing = false
		anchor := m.pendingSelectionAnchor
		m.pendingSelectionAnchor = nil
		if msg.err != nil {
			m.errText = msg.err.Error()
			m.results = nil
			m.selectedRow = 0
			return m.selectionChangedCmd()
		}

		m.errText = ""
		m.results = msg.issues
		if anchor != nil {
			m.restoreSelectionFromAnchor(anchor)
		} else {
			m.normalizeSelection()
		}
		return m.selectionChangedCmd()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return nil
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		return nil
	case tea.KeyBackspace:
		if m.focus != uisearch.FocusQuery {
			return nil
		}
		if m.query == "" {
			return nil
		}
		runes := []rune(m.query)
		m.query = string(runes[:len(runes)-1])
		m.typing = true
		return m.triggerSearch()
	case tea.KeyCtrlU:
		if m.focus != uisearch.FocusQuery {
			return nil
		}
		if m.query == "" {
			return nil
		}
		m.query = ""
		m.typing = false
		return m.triggerSearch()
	}

	switch {
	case msg.Type == tea.KeyRunes && m.focus == uisearch.FocusQuery:
		m.query += string(msg.Runes)
		m.typing = true
		return m.triggerSearch()
	case m.keys.Match(config.SearchContext, config.SearchActionOpenDetail, msg):
		if m.focus == uisearch.FocusResults && m.currentSelection() != nil {
			return func() tea.Msg {
				return mode.ActionRequestMsg{Mode: mode.Search, Action: mode.ActionOpenDetail}
			}
		}
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionMoveUp, msg):
		if m.focus == uisearch.FocusResults && m.moveSelection(-1) {
			return m.selectionChangedCmd()
		}
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionMoveDown, msg):
		if m.focus == uisearch.FocusResults && m.moveSelection(1) {
			return m.selectionChangedCmd()
		}
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionFocusLeft, msg):
		m.moveFocusLeft()
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionFocusRight, msg):
		m.moveFocusRight()
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionCycleFocusNext, msg):
		m.cycleFocus(1)
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionCycleFocusPrev, msg):
		m.cycleFocus(-1)
		return nil
	case msg.Type == tea.KeyRunes:
		switch {
		case m.keys.Match(config.SearchContext, config.SearchActionFocusQuery, msg):
			m.focus = uisearch.FocusQuery
			return nil
		case m.keys.Match(config.SearchContext, config.SearchActionReload, msg):
			return m.triggerSearchPreservingSelection()
		}

		return nil
	default:
		return nil
	}
}

func (m *Model) moveFocusLeft() {
	switch m.focus {
	case uisearch.FocusPreview:
		m.focus = uisearch.FocusResults
	case uisearch.FocusResults:
		m.focus = uisearch.FocusQuery
	}
}

func (m *Model) moveFocusRight() {
	switch m.focus {
	case uisearch.FocusQuery:
		if len(m.results) > 0 {
			m.focus = uisearch.FocusResults
		}
	case uisearch.FocusResults:
		m.focus = uisearch.FocusPreview
	}
}

func (m *Model) cycleFocus(delta int) {
	order := []uisearch.FocusPane{uisearch.FocusQuery, uisearch.FocusResults, uisearch.FocusPreview}
	idx := 0
	for i, focus := range order {
		if focus == m.focus {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = len(order) - 1
	}
	if idx >= len(order) {
		idx = 0
	}
	if len(m.results) == 0 && order[idx] != uisearch.FocusQuery {
		m.focus = uisearch.FocusQuery
		return
	}
	m.focus = order[idx]
}

func (m *Model) triggerSearch() tea.Cmd {
	return m.triggerSearchWithAnchor(nil)
}

func (m *Model) triggerSearchPreservingSelection() tea.Cmd {
	anchor := m.captureSelectionAnchor()
	return m.triggerSearchWithAnchor(anchor)
}

func (m *Model) triggerSearchWithAnchor(anchor *selectionAnchor) tea.Cmd {
	query := domain.SearchIssuesQuery{
		Text:   strings.TrimSpace(m.query),
		Limit:  defaultSearchLimit,
		Offset: 0,
	}
	m.loading = true
	m.errText = ""
	m.pendingSelectionAnchor = anchor
	return loadSearchCmd(m.gateway, query)
}

// View renders the standalone search surface.
func (m *Model) View() string {
	return uisearch.Render(uisearch.State{
		Loading:        m.loading,
		Error:          m.errText,
		Query:          m.query,
		Focus:          m.focus,
		Typing:         m.typing,
		Results:        m.results,
		SelectedID:     m.selectedIssueID(),
		SelectedDetail: selectedDetail(m.currentSelection()),
		Width:          m.width,
		Height:         m.height,
	})
}

// SetSize updates render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// IsLoading reports whether a gateway search is active.
func (m *Model) IsLoading() bool {
	return m.loading
}

// Reload refreshes current search results without mutating query input state.
func (m *Model) Reload() tea.Cmd {
	if m.loading {
		return nil
	}
	return m.triggerSearchPreservingSelection()
}

// AutoRefresh refreshes search when safe for active query editing.
func (m *Model) AutoRefresh() tea.Cmd {
	if m.loading {
		return nil
	}
	if m.focus == uisearch.FocusQuery && m.typing {
		return nil
	}
	return m.triggerSearchPreservingSelection()
}

// ResultCount returns the current result count.
func (m *Model) ResultCount() int {
	return len(m.results)
}

// CurrentSelection returns the current search issue selection.
func (m *Model) CurrentSelection() *mode.Selection {
	return m.currentSelection()
}

func (m *Model) moveSelection(delta int) bool {
	if len(m.results) == 0 {
		return false
	}
	previous := m.selectedRow
	m.selectedRow += delta
	m.normalizeSelection()
	return m.selectedRow != previous
}

func (m *Model) normalizeSelection() {
	if len(m.results) == 0 {
		m.selectedRow = 0
		return
	}
	if m.selectedRow < 0 {
		m.selectedRow = 0
	}
	if m.selectedRow >= len(m.results) {
		m.selectedRow = len(m.results) - 1
	}
}

func (m *Model) selectedIssueID() string {
	selection := m.currentSelection()
	if selection == nil {
		return ""
	}
	return selection.Issue.ID
}

func (m *Model) captureSelectionAnchor() *selectionAnchor {
	anchor := &selectionAnchor{row: m.selectedRow}
	if sel := m.currentSelection(); sel != nil {
		anchor.issueID = sel.Issue.ID
	}
	return anchor
}

func (m *Model) restoreSelectionFromAnchor(anchor *selectionAnchor) {
	if anchor == nil {
		m.normalizeSelection()
		return
	}
	if anchor.issueID != "" {
		for idx, issue := range m.results {
			if issue.ID == anchor.issueID {
				m.selectedRow = idx
				m.normalizeSelection()
				return
			}
		}
	}
	m.selectedRow = anchor.row
	m.normalizeSelection()
}

func (m *Model) currentSelection() *mode.Selection {
	if len(m.results) == 0 || m.selectedRow < 0 || m.selectedRow >= len(m.results) {
		return nil
	}
	selection := mode.Selection{Issue: m.results[m.selectedRow]}
	return &selection
}

func (m *Model) selectionChangedCmd() tea.Cmd {
	selection := m.currentSelection()
	return func() tea.Msg {
		return mode.SelectionChangedMsg{Mode: mode.Search, Selection: selection}
	}
}

func loadSearchCmd(gateway beads.BeadsGateway, query domain.SearchIssuesQuery) tea.Cmd {
	return func() tea.Msg {
		page, err := gateway.SearchIssues(context.Background(), query)
		if err != nil {
			return searchLoadedMsg{err: err}
		}

		issues := make([]domain.IssueSummary, 0, len(page.Results))
		for _, result := range page.Results {
			issues = append(issues, result.Issue)
		}

		return searchLoadedMsg{issues: issues}
	}
}

// CapturesShellKey reports whether active search input should consume a key
// before shell-level keybindings are evaluated.
func (m *Model) CapturesShellKey(msg tea.KeyMsg) bool {
	if m.keys.Match(config.SearchContext, config.SearchActionCycleFocusNext, msg) || m.keys.Match(config.SearchContext, config.SearchActionCycleFocusPrev, msg) {
		return true
	}

	if m.focus != uisearch.FocusQuery {
		return false
	}
	if msg.Type == tea.KeyRunes {
		return true
	}
	if shellKeysPassThrough(m.keys, msg) {
		return false
	}
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyCtrlU:
		return true
	default:
		return false
	}
}

func shellKeysPassThrough(keys config.ResolvedKeyBindings, msg tea.KeyMsg) bool {
	for _, action := range []string{
		config.ShellActionQuit,
		config.ShellActionHelp,
		config.ShellActionModeBoard,
		config.ShellActionModeSearch,
		config.ShellActionToggleSearch,
		config.ShellActionModeDetail,
		config.ShellActionModeCycleNext,
		config.ShellActionModeCyclePrev,
		config.ShellActionEscape,
	} {
		if keys.Match(config.ShellContext, action, msg) {
			return true
		}
	}
	return false
}

func selectedDetail(selection *mode.Selection) domain.IssueDetail {
	if selection == nil {
		return domain.IssueDetail{}
	}
	return domain.IssueDetail{Summary: selection.Issue}
}
