package search

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

// Model is the standalone search mode controller.
type Model struct {
	gateway beads.BeadsGateway

	width  int
	height int

	loading bool
	errText string

	query string
	focus uisearch.FocusPane

	results     []domain.IssueSummary
	selectedRow int
	typing      bool
}

var _ mode.Controller = (*Model)(nil)

// NewModel creates a search mode controller.
func NewModel(gateway beads.BeadsGateway) *Model {
	return &Model{
		gateway: gateway,
		focus:   uisearch.FocusQuery,
	}
}

// ID returns search mode identifier.
func (m *Model) ID() mode.ID {
	return mode.Search
}

// Init returns nil because search loads on demand when text is entered.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update processes search-specific messages and keybindings.
func (m *Model) Update(msg tea.Msg) (mode.Controller, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil
	case searchLoadedMsg:
		m.loading = false
		m.typing = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			m.results = nil
			m.selectedRow = 0
			return m, m.selectionChangedCmd()
		}

		m.errText = ""
		m.results = msg.issues
		m.normalizeSelection()
		return m, m.selectionChangedCmd()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (mode.Controller, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m, nil
	case tea.KeyEnter:
		if m.focus == uisearch.FocusResults && m.currentSelection() != nil {
			return m, func() tea.Msg {
				return mode.ActionRequestMsg{Mode: mode.Search, Action: mode.ActionOpenDetail}
			}
		}
		return m, nil
	case tea.KeyUp:
		if m.focus == uisearch.FocusResults && m.moveSelection(-1) {
			return m, m.selectionChangedCmd()
		}
		return m, nil
	case tea.KeyDown:
		if m.focus == uisearch.FocusResults && m.moveSelection(1) {
			return m, m.selectionChangedCmd()
		}
		return m, nil
	case tea.KeyLeft:
		return m.moveFocusLeft(), nil
	case tea.KeyRight:
		return m.moveFocusRight(), nil
	case tea.KeyBackspace:
		if m.focus != uisearch.FocusQuery {
			return m, nil
		}
		if m.query == "" {
			return m, nil
		}
		runes := []rune(m.query)
		m.query = string(runes[:len(runes)-1])
		m.typing = true
		return m.triggerSearch()
	case tea.KeyCtrlU:
		if m.focus != uisearch.FocusQuery {
			return m, nil
		}
		if m.query == "" {
			return m, nil
		}
		m.query = ""
		m.typing = false
		return m.triggerSearch()
	case tea.KeyCtrlJ, tea.KeyTab:
		return m.cycleFocus(1), nil
	case tea.KeyShiftTab, tea.KeyCtrlK:
		return m.cycleFocus(-1), nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "j":
			if m.focus == uisearch.FocusResults && m.moveSelection(1) {
				return m, m.selectionChangedCmd()
			}
			return m, nil
		case "k":
			if m.focus == uisearch.FocusResults && m.moveSelection(-1) {
				return m, m.selectionChangedCmd()
			}
			return m, nil
		case "h":
			return m.moveFocusLeft(), nil
		case "l":
			return m.moveFocusRight(), nil
		case "/":
			m.focus = uisearch.FocusQuery
			return m, nil
		case "r":
			return m.triggerSearch()
		}

		if m.focus != uisearch.FocusQuery {
			return m, nil
		}
		m.query += string(msg.Runes)
		m.typing = true
		return m.triggerSearch()
	default:
		return m, nil
	}
}

func (m *Model) moveFocusLeft() mode.Controller {
	switch m.focus {
	case uisearch.FocusPreview:
		m.focus = uisearch.FocusResults
	case uisearch.FocusResults:
		m.focus = uisearch.FocusQuery
	}
	return m
}

func (m *Model) moveFocusRight() mode.Controller {
	switch m.focus {
	case uisearch.FocusQuery:
		if len(m.results) > 0 {
			m.focus = uisearch.FocusResults
		}
	case uisearch.FocusResults:
		m.focus = uisearch.FocusPreview
	}
	return m
}

func (m *Model) cycleFocus(delta int) mode.Controller {
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
		return m
	}
	m.focus = order[idx]
	return m
}

func (m *Model) triggerSearch() (mode.Controller, tea.Cmd) {
	query := domain.SearchIssuesQuery{
		Text:   strings.TrimSpace(m.query),
		Limit:  defaultSearchLimit,
		Offset: 0,
	}
	if strings.TrimSpace(query.Text) == "" {
		m.loading = false
		m.errText = ""
		m.results = nil
		m.selectedRow = 0
		m.focus = uisearch.FocusQuery
		m.typing = false
		return m, m.selectionChangedCmd()
	}

	m.loading = false
	m.errText = ""
	return m, loadSearchCmd(m.gateway, query)
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
	if m.focus != uisearch.FocusQuery {
		return false
	}
	if msg.Type == tea.KeyRunes {
		switch string(msg.Runes) {
		case "1", "2", "3":
			return false
		default:
			return true
		}
	}
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyCtrlU:
		return true
	default:
		return false
	}
}

func selectedDetail(selection *mode.Selection) domain.IssueDetail {
	if selection == nil {
		return domain.IssueDetail{}
	}
	return domain.IssueDetail{Summary: selection.Issue}
}
