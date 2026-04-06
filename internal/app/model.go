package app

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode"
	boardmode "github.com/hk9890/beads-workbench/internal/mode/board"
	detailsmode "github.com/hk9890/beads-workbench/internal/mode/details"
	searchmode "github.com/hk9890/beads-workbench/internal/mode/search"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
	"github.com/hk9890/beads-workbench/internal/ui/modal"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
	"github.com/hk9890/beads-workbench/internal/ui/toaster"
)

const (
	defaultViewportWidth  = 120
	defaultViewportHeight = 34
)

type detailLoadedMsg struct {
	issueID string
	detail  domain.IssueDetail
	err     error
}

type editIssueResultMsg struct {
	issueID string
	updated bool
	err     error
}

type launchActionResultMsg struct {
	action string
	err    error
}

// Model is the root Bubble Tea shell for Beads Workbench.
//
// v1 detail presentation model keeps browse and full detail separated:
//   - Board/Search prioritize high-density triage browsing.
//   - Full issue inspection stays in dedicated detail mode.
type Model struct {
	services Services

	active     mode.ID
	lastBrowse mode.ID

	selectedByMode map[mode.ID]*mode.Selection

	board  mode.Controller
	search mode.Controller

	detail detailsmode.Model

	toast toaster.Model

	help     modal.Model
	showHelp bool

	width  int
	height int
}

// NewModel builds the root shell model.
func NewModel(services Services) Model {
	help := modal.New(modal.Config{
		Title:       "Keyboard Help",
		Message:     shellKeyHelp(),
		HideButtons: true,
		Required:    false,
		MinWidth:    72,
	})

	return Model{
		services:       services,
		active:         mode.Board,
		lastBrowse:     mode.Board,
		selectedByMode: make(map[mode.ID]*mode.Selection),
		board:          boardmode.NewModel(services.Gateway, dashboard.NewBuiltInProvider()),
		search:         searchmode.NewModel(services.Gateway),
		toast:          toaster.New(),
		help:           help,
		width:          defaultViewportWidth,
		height:         defaultViewportHeight,
	}
}

// Init loads initial board and search controllers.
func (m Model) Init() tea.Cmd {
	m.board.SetSize(m.width, m.height)
	m.search.SetSize(m.width, m.height)
	return tea.Batch(m.board.Init(), m.search.Init())
}

// Update handles root-level shell messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	modeCmd := m.forwardModeMessages(msg)
	if m.syncSelectionFromControllers() {
		modeCmd = batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
	}

	if m.showHelp {
		if k, ok := msg.(tea.KeyMsg); ok && k.String() == "?" {
			m.showHelp = false
			return m, modeCmd
		}

		if _, ok := msg.(modal.CancelMsg); ok {
			m.showHelp = false
			return m, modeCmd
		}
		if _, ok := msg.(modal.SubmitMsg); ok {
			m.showHelp = false
			return m, modeCmd
		}

		nextHelp, cmd := m.help.Update(msg)
		m.help = nextHelp

		if size, ok := msg.(tea.WindowSizeMsg); ok {
			m.width = size.Width
			m.height = size.Height
			m.help.SetSize(m.width, m.height)
		}

		return m, batchCmds(modeCmd, cmd)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.board.SetSize(m.width, m.height)
		m.search.SetSize(m.width, m.height)
		m.help.SetSize(m.width, m.height)
		return m, modeCmd
	case detailLoadedMsg:
		if msg.issueID != m.detail.TargetID {
			return m, modeCmd
		}

		m.detail.Loading = false
		if msg.err != nil {
			m.detail.Detail = domain.IssueDetail{}
			m.detail.Error = msg.err.Error()
			return m, batchCmds(modeCmd, m.showToast("Failed to load selected issue details", toaster.StyleError))
		}

		m.detail.Error = ""
		m.detail.Detail = msg.detail
		return m, modeCmd
	case editIssueResultMsg:
		if msg.err != nil {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Failed to edit issue %s", msg.issueID), toaster.StyleError))
		}

		if !msg.updated {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("No changes saved for issue %s", msg.issueID), toaster.StyleInfo))
		}

		selection := m.currentSelection()
		if selection == nil || selection.Issue.ID == "" {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Updated issue %s", msg.issueID), toaster.StyleSuccess))
		}

		m.detail.SelectionID = selection.Issue.ID
		m.detail.Loading = true
		m.detail.Error = ""
		m.detail.TargetID = selection.Issue.ID
		return m, batchCmds(modeCmd,
			m.showToast(fmt.Sprintf("Updated issue %s", msg.issueID), toaster.StyleSuccess),
			loadDetailCmd(m.services, selection.Issue.ID),
		)
	case launchActionResultMsg:
		if msg.err != nil {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Launcher action %q failed: %v", msg.action, msg.err), toaster.StyleError))
		}
		return m, modeCmd
	case mode.SelectionChangedMsg:
		if msg.Mode != mode.Board && msg.Mode != mode.Search {
			return m, modeCmd
		}
		m.selectedByMode[msg.Mode] = msg.Selection
		if msg.Mode == m.active {
			m.lastBrowse = msg.Mode
		}
		return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
	case mode.ActionRequestMsg:
		if msg.Action != mode.ActionOpenDetail {
			return m, modeCmd
		}
		if msg.Mode == mode.Board || msg.Mode == mode.Search {
			m.lastBrowse = msg.Mode
		}
		if m.currentSelection() == nil {
			return m, batchCmds(modeCmd, m.showToast("No selected issue to open in detail mode", toaster.StyleWarn))
		}
		m.active = mode.Detail
		return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
	case toaster.DismissMsg:
		m.toast = m.toast.Hide()
		return m, modeCmd
	case tea.KeyMsg:
		if m.active == mode.Search {
			type searchShellKeyCapture interface {
				CapturesShellKey(tea.KeyMsg) bool
			}

			if searchCtrl, ok := m.search.(searchShellKeyCapture); ok && searchCtrl.CapturesShellKey(msg) {
				return m, modeCmd
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, batchCmds(modeCmd, tea.Quit)
		case "?":
			m.showHelp = true
			m.help.SetSize(m.width, m.height)
			return m, modeCmd
		case "1", "b":
			m.active = mode.Board
			m.lastBrowse = mode.Board
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
		case "ctrl+@":
			if m.active == mode.Detail {
				m.active = mode.Board
				m.lastBrowse = mode.Board
				return m, modeCmd
			}
			if m.active == mode.Search {
				m.active = mode.Board
				m.lastBrowse = mode.Board
				return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
			}
			m.active = mode.Search
			m.lastBrowse = mode.Search
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
		case "2", "s":
			m.active = mode.Search
			m.lastBrowse = mode.Search
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
		case "3":
			if m.currentSelection() == nil {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to open in detail mode", toaster.StyleWarn))
			}
			m.active = mode.Detail
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
		case "tab":
			if m.active == mode.Board {
				return m, modeCmd
			}
			m.active = nextMode(m.active)
			if m.active == mode.Board || m.active == mode.Search {
				m.lastBrowse = m.active
			}
			if m.active == mode.Detail && m.currentSelection() == nil {
				m.active = mode.Board
				m.lastBrowse = mode.Board
			}
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
		case "shift+tab":
			m.active = prevMode(m.active)
			if m.active == mode.Board || m.active == mode.Search {
				m.lastBrowse = m.active
			}
			if m.active == mode.Detail && m.currentSelection() == nil {
				m.active = mode.Search
				m.lastBrowse = mode.Search
			}
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
		case "esc":
			if m.active == mode.Detail {
				m.active = m.lastBrowse
				return m, modeCmd
			}
			if m.active == mode.Search {
				m.active = mode.Board
				m.lastBrowse = mode.Board
				return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
			}
			m.toast = m.toast.Hide()
			return m, modeCmd
		case "r":
			if m.active != mode.Detail {
				return m, modeCmd
			}
			if m.currentSelection() == nil {
				return m, modeCmd
			}
			m.detail.Loading = true
			m.detail.Error = ""
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
		case "e":
			issueContext, ok := m.selectedIssueContext()
			if !ok {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to edit", toaster.StyleWarn))
			}
			return m, batchCmds(modeCmd, launchActionCmd(m.services, "editor", issueContext))
		}
	}

	return m, modeCmd
}

func (m *Model) syncSelectionFromControllers() bool {
	boardChanged := m.syncSelectionFromController(mode.Board, m.board)
	searchChanged := m.syncSelectionFromController(mode.Search, m.search)
	return boardChanged || searchChanged
}

func (m *Model) syncSelectionFromController(modeID mode.ID, controller mode.Controller) bool {
	if controller == nil {
		return false
	}

	type selectionProvider interface {
		CurrentSelection() *mode.Selection
	}

	provider, ok := controller.(selectionProvider)
	if !ok {
		return false
	}

	selection := provider.CurrentSelection()
	current := m.selectedByMode[modeID]
	if selectionEqual(current, selection) {
		return false
	}

	m.selectedByMode[modeID] = selection
	if modeID == m.active {
		m.lastBrowse = modeID
	}

	return true
}

func selectionEqual(a, b *mode.Selection) bool {
	if a == nil || b == nil {
		return a == b
	}

	return reflect.DeepEqual(a.Issue, b.Issue)
}

// View renders the root shell.
func (m Model) View() string {
	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()

	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	if m.toast.Visible() {
		view = m.toast.Overlay(view, m.width, m.height)
	}
	if m.showHelp {
		view = m.help.Overlay(view)
	}

	return view
}

func (m Model) currentSelection() *mode.Selection {
	if m.lastBrowse != mode.Board && m.lastBrowse != mode.Search {
		if m.active == mode.Board || m.active == mode.Search {
			return m.selectedByMode[m.active]
		}
		return nil
	}

	if m.active == mode.Board || m.active == mode.Search {
		return m.selectedByMode[m.active]
	}

	return m.selectedByMode[m.lastBrowse]
}

func (m *Model) ensureDetailForCurrentSelectionCmd() tea.Cmd {
	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		m.detail = detailsmode.Model{}
		return nil
	}

	m.detail.SelectionID = selection.Issue.ID

	if m.detail.Loading && m.detail.TargetID == selection.Issue.ID {
		return nil
	}
	if !m.detail.Loading && m.detail.Detail.Summary.ID == selection.Issue.ID && m.detail.Error == "" {
		return nil
	}

	m.detail.Loading = true
	m.detail.Error = ""
	m.detail.TargetID = selection.Issue.ID
	return loadDetailCmd(m.services, selection.Issue.ID)
}

func (m Model) renderHeader() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(styles.TextPrimaryColor).Render("Beads Workbench")

	tab := func(id mode.ID, label string) string {
		base := lipgloss.NewStyle().Padding(0, 1)
		if m.active == id {
			return base.Foreground(styles.ButtonTextColor).Background(styles.ButtonPrimaryFocusBgColor).Bold(true).Render(label)
		}
		return base.Foreground(styles.TextMutedColor).Render(label)
	}

	tabs := strings.Join([]string{
		tab(mode.Board, "1 Board"),
		tab(mode.Search, "2 Search"),
		tab(mode.Detail, "3 Detail"),
	}, " ")

	keys := "Modes: [1/2/3 ctrl+space tab]  Search: [type / j/k h/l esc]  Detail: [enter o esc]  Actions: [e edit-in-editor r refresh ? help q quit]"
	if !m.services.Config.UI.ShowModeSwitcherHelp {
		keys = ""
	}

	lines := []string{title, tabs}
	if keys == "" {
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	for _, line := range headerHelpLines(m.active, m.width) {
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(line))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) renderBody() string {
	if m.active == mode.Detail {
		return styles.FormSection(styles.FormSectionConfig{
			Width:              maxInt(60, m.width-2),
			TopLeft:            "Issue Detail",
			TopRight:           detailHeaderID(m.detail),
			Content:            strings.Split(m.detail.View(maxInt(40, m.width-8), false), "\n"),
			Focused:            true,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})
	}

	browseWidth := maxInt(40, m.width-2)
	browseHeight := maxInt(14, m.height-7)
	m.board.SetSize(browseWidth, browseHeight)
	m.search.SetSize(browseWidth, browseHeight)

	var browse string
	if m.active == mode.Board {
		browse = m.board.View()
	} else {
		browse = m.search.View()
	}

	return browse
}

func (m Model) renderFooter() string {
	selectionText := "Selected: none"
	if sel := m.currentSelection(); sel != nil {
		selectionText = fmt.Sprintf("Selected: %s (%s)", sel.Issue.ID, sel.Issue.Status)
	}

	loadingStates := make([]loading.State, 0, 3)
	if m.boardIsLoading() {
		loadingStates = append(loadingStates, loading.State{Scope: loading.ScopeBoard})
	}
	if m.searchIsLoading() {
		loadingStates = append(loadingStates, loading.State{Scope: loading.ScopeSearch})
	}
	if m.detail.Loading {
		loadingStates = append(loadingStates, loading.State{Scope: loading.ScopeDetail, Target: m.detail.TargetID})
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Render(selectionText),
		lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("  ·  "+loading.Summary(loadingStates)),
	)
}

func (m *Model) showToast(message string, style toaster.Style) tea.Cmd {
	m.toast = m.toast.Show(message, style)
	return toaster.ScheduleDismiss(3 * time.Second)
}

func (m Model) boardIsLoading() bool {
	if m.board == nil {
		return false
	}

	type loadingReporter interface {
		IsLoading() bool
	}

	reporter, ok := m.board.(loadingReporter)
	if !ok {
		return false
	}

	return reporter.IsLoading()
}

func (m Model) searchIsLoading() bool {
	if m.search == nil {
		return false
	}

	type loadingReporter interface {
		IsLoading() bool
	}

	reporter, ok := m.search.(loadingReporter)
	if !ok {
		return false
	}

	return reporter.IsLoading()
}

func (m Model) searchResultCount() int {
	if m.search == nil {
		return 0
	}

	type resultCounter interface {
		ResultCount() int
	}

	counter, ok := m.search.(resultCounter)
	if !ok {
		return 0
	}

	return counter.ResultCount()
}

func (m *Model) forwardModeMessages(msg tea.Msg) tea.Cmd {
	boardCmd := m.forwardBoardMessage(msg)
	searchCmd := m.forwardSearchMessage(msg)
	return batchCmds(boardCmd, searchCmd)
}

func (m *Model) forwardBoardMessage(msg tea.Msg) tea.Cmd {
	if m.board == nil || !m.shouldForwardToBoard(msg) {
		return nil
	}

	next, cmd := m.board.Update(msg)
	m.board = next
	return cmd
}

func (m *Model) forwardSearchMessage(msg tea.Msg) tea.Cmd {
	if m.search == nil || !m.shouldForwardToSearch(msg) {
		return nil
	}

	next, cmd := m.search.Update(msg)
	m.search = next
	return cmd
}

func (m Model) shouldForwardToBoard(msg tea.Msg) bool {
	if _, isKey := msg.(tea.KeyMsg); isKey {
		return m.active == mode.Board
	}
	return true
}

func (m Model) shouldForwardToSearch(msg tea.Msg) bool {
	if _, isKey := msg.(tea.KeyMsg); isKey {
		return m.active == mode.Search
	}
	return true
}

func batchCmds(cmds ...tea.Cmd) tea.Cmd {
	filtered := make([]tea.Cmd, 0, len(cmds))
	for _, cmd := range cmds {
		if cmd != nil {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return tea.Batch(filtered...)
}

func nextMode(current mode.ID) mode.ID {
	order := []mode.ID{mode.Board, mode.Search, mode.Detail}
	for i, id := range order {
		if id == current {
			return order[(i+1)%len(order)]
		}
	}
	return mode.Board
}

func prevMode(current mode.ID) mode.ID {
	order := []mode.ID{mode.Board, mode.Search, mode.Detail}
	for i, id := range order {
		if id == current {
			if i == 0 {
				return order[len(order)-1]
			}
			return order[i-1]
		}
	}
	return mode.Search
}

func loadDetailCmd(services Services, issueID string) tea.Cmd {
	return func() tea.Msg {
		detail, err := services.Gateway.ShowIssue(context.Background(), domain.ShowIssueQuery{IssueID: issueID})
		return detailLoadedMsg{issueID: issueID, detail: detail, err: err}
	}
}

func editIssueCmd(services Services, issueID string) tea.Cmd {
	return func() tea.Msg {
		result, err := services.Editor.EditIssue(context.Background(), issueID)
		return editIssueResultMsg{issueID: issueID, updated: result.Updated, err: err}
	}
}

func launchActionCmd(services Services, action string, issue domain.IssueDetail) tea.Cmd {
	return func() tea.Msg {
		err := services.Launcher.Launch(context.Background(), action, issue)
		return launchActionResultMsg{action: action, err: err}
	}
}

func (m Model) selectedIssueID() (string, bool) {
	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		return "", false
	}

	return selection.Issue.ID, true
}

func (m Model) selectedIssueContext() (domain.IssueDetail, bool) {
	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		return domain.IssueDetail{}, false
	}

	if m.detail.Detail.Summary.ID == selection.Issue.ID {
		return m.detail.Detail, true
	}

	return domain.IssueDetail{Summary: selection.Issue}, true
}

func shellKeyHelp() string {
	return strings.Join([]string{
		"Mode switching:",
		"  1/b = Board, 2/s = Search, 3 = Detail",
		"  tab / shift+tab = cycle modes",
		"",
		"Selection:",
		"  Board: h/l (or ←/→) switch columns, j/k (or ↓/↑) move within a column",
		"  Search: use f / shift+tab to move filter focus; when focused on results, j/k moves selection",
		"",
		"Search filters:",
		"  Text + assignee: type to edit, backspace to delete",
		"  Status/type/label/priority: h/l (or ←/→) cycle values",
		"  Ready/Blocked: space/enter toggles (mutually exclusive)",
		"",
		"Actions:",
		"  e = edit selected issue in external editor",
		"  enter or o = open selected issue in detail mode",
		"  r = reload detail mode from gateway",
		"  esc = exit detail mode / dismiss toast",
		"  ? = toggle help",
		"  q = quit",
		"",
		"Detail presentation model (v1): dedicated detail mode",
		"  - Board/Search prioritize overview triage density",
		"  - enter or o opens full issue detail view",
	}, "\n")
}

func headerHelpLines(active mode.ID, width int) []string {
	if width < 110 {
		return []string{"Keys: enter detail · e editor · ? help · q quit"}
	}

	modeLine := "Modes: 1 Board · 2 Search · 3 Detail · Tab cycle · ? help · q quit"
	if width < 145 {
		switch active {
		case mode.Search:
			return []string{"Search: type to query · / focus · h/l panes · j/k results · esc board · ? help · q quit"}
		case mode.Detail:
			return []string{"Detail: e editor · r refresh · esc back · 1/2/3 modes · ? help · q quit"}
		default:
			return []string{"Board: h/l cols · j/k issues · enter detail · ctrl+space search · ? help · q quit"}
		}
	}

	switch active {
	case mode.Search:
		return []string{modeLine, "Search: type to query · / focus query · h/l move panes · j/k results · enter detail · esc board"}
	case mode.Detail:
		return []string{modeLine, "Detail: e editor · r refresh detail · esc back to browse"}
	default:
		return []string{modeLine, "Board: h/l switch columns · j/k move issues · enter detail · ctrl+space search"}
	}
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func detailHeaderID(detail detailsmode.Model) string {
	id := strings.TrimSpace(detail.Detail.Summary.ID)
	if id == "" {
		id = strings.TrimSpace(detail.SelectionID)
	}
	return id
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
