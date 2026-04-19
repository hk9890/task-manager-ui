package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/config"
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
	refreshTickInterval   = 60 * time.Second
)

type refreshTickMsg struct{}

var scheduleRefreshTickCmd = func() tea.Cmd {
	return tea.Tick(refreshTickInterval, func(_ time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

var modelNow = time.Now

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

type mutationKind string

const (
	mutationCreate        mutationKind = "create"
	mutationUpdate        mutationKind = "update"
	mutationClose         mutationKind = "close"
	mutationComment       mutationKind = "comment"
	mutationStatus        mutationKind = "status"
	mutationPriorityCycle mutationKind = "priority_cycle"
)

type mutationCatalogsLoadedMsg struct {
	kind     mutationKind
	issue    domain.IssueSummary
	statuses []domain.StatusOption
	types    []domain.TypeOption
	labels   []domain.LabelOption
	err      error
}

type statusCatalogLoadedMsg struct {
	issue    domain.IssueSummary
	statuses []domain.StatusOption
	err      error
}

type mutationResultMsg struct {
	kind      mutationKind
	issueID   string
	createdID string
	err       error
}

type mutationDialogState struct {
	kind        mutationKind
	issue       domain.IssueSummary
	statusNames map[string]struct{}
	typeNames   map[string]struct{}
	labelNames  map[string]struct{}
	statusList  string
	typeList    string
	labelList   string
}

type surfaceRefreshState struct {
	dirty       bool
	lastRefresh time.Time
}

// Model is the root Bubble Tea shell for Beads Workbench.
//
// v1 detail presentation model keeps browse and full detail separated:
//   - Board/Search prioritize high-density triage browsing.
//   - Full issue inspection stays in dedicated detail mode.
type Model struct {
	services Services
	keys     config.ResolvedKeyBindings

	active     mode.ID
	lastBrowse mode.ID

	selectedByMode map[mode.ID]*mode.Selection

	board  *boardmode.Model
	search *searchmode.Model

	detail detailsmode.Model

	toast toaster.Model

	help     modal.Model
	showHelp bool

	actionModal     modal.Model
	showActionModal bool
	actionState     mutationDialogState

	focusKnown      bool
	terminalFocused bool

	refreshStateBySurface map[mode.ID]surfaceRefreshState

	width  int
	height int
}

// NewModel builds the root shell model.
func NewModel(services Services) Model {
	keys, err := config.ResolveKeyBindings(services.Config.KeyBindings)
	if err != nil {
		panic(fmt.Sprintf("invalid resolved keybindings in app model: %v", err))
	}

	now := modelNow()

	helpText := shellKeyHelp(keys)
	help := modal.NewWithKeys(modal.Config{
		Title:       "Keyboard Help",
		Message:     helpText,
		HideButtons: true,
		Required:    false,
		MinWidth:    72,
	}, modal.BindingsFromConfig(keys))

	return Model{
		services:       services,
		keys:           keys,
		active:         mode.Board,
		lastBrowse:     mode.Board,
		selectedByMode: make(map[mode.ID]*mode.Selection),
		board:          boardmode.NewModel(services.Gateway, dashboard.NewBuiltInProvider(), keys),
		search:         searchmode.NewModel(services.Gateway, keys),
		toast:          toaster.New(),
		help:           help,
		width:          defaultViewportWidth,
		height:         defaultViewportHeight,
		refreshStateBySurface: map[mode.ID]surfaceRefreshState{
			mode.Board:  {lastRefresh: now},
			mode.Search: {lastRefresh: now},
			mode.Detail: {},
		},
	}
}

// Init loads initial board and search controllers.
func (m Model) Init() tea.Cmd {
	m.applyWorkspaceSizeToBrowseModes()
	return tea.Batch(m.board.Init(), m.search.Init(), scheduleRefreshTickCmd())
}

// Update handles root-level shell messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	modeCmd := tea.Cmd(nil)
	if !m.shouldCaptureKeyForOverlay(msg) {
		modeCmd = m.forwardModeMessages(msg)
	}

	if m.showActionModal {
		if size, ok := msg.(tea.WindowSizeMsg); ok {
			m.width = size.Width
			m.height = size.Height
			m.actionModal.SetSize(m.width, m.height)
			return m, modeCmd
		}

		if _, ok := msg.(modal.CancelMsg); ok {
			m.showActionModal = false
			return m, modeCmd
		}

		if submit, ok := msg.(modal.SubmitMsg); ok {
			m.showActionModal = false
			return m, batchCmds(modeCmd, submitMutationCmd(m.services, m.actionState, submit.Values))
		}

		nextModal, cmd := m.actionModal.Update(msg)
		m.actionModal = nextModal
		return m, batchCmds(modeCmd, cmd)
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
	case tea.FocusMsg:
		wasBlurred := m.focusKnown && !m.terminalFocused
		m.focusKnown = true
		m.terminalFocused = true
		if !wasBlurred {
			return m, modeCmd
		}
		return m, batchCmds(modeCmd, m.maybeAutoRefreshActiveSurfaceCmdOnFocusRegain())
	case tea.BlurMsg:
		m.focusKnown = true
		m.terminalFocused = false
		return m, modeCmd
	case refreshTickMsg:
		return m, batchCmds(modeCmd, scheduleRefreshTickCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applyWorkspaceSizeToBrowseModes()
		m.help.SetSize(m.width, m.height)
		m.detail.ClampScroll(m.detailViewportWidth(), m.detailViewportHeight())
		return m, modeCmd
	case detailLoadedMsg:
		if msg.issueID != m.detail.TargetID {
			return m, modeCmd
		}

		m.detail.Loading = false
		m.markSurfaceRefreshed(mode.Detail)
		if msg.err != nil {
			m.detail.Detail = domain.IssueDetail{}
			m.detail.Error = msg.err.Error()
			return m, batchCmds(modeCmd, m.showToast("Failed to load selected issue details", toaster.StyleError))
		}

		m.detail.Error = ""
		m.detail.ApplyLoadedDetail(msg.issueID, msg.detail)
		m.detail.ClampScroll(m.detailViewportWidth(), m.detailViewportHeight())
		return m, modeCmd
	case editIssueResultMsg:
		if msg.err != nil {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Failed to edit issue %s", msg.issueID), toaster.StyleError))
		}

		if !msg.updated {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("No changes saved for issue %s", msg.issueID), toaster.StyleInfo))
		}

		m.markBrowseSurfacesDirty()

		selection := m.currentSelection()
		if selection == nil || selection.Issue.ID == "" {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Updated issue %s", msg.issueID), toaster.StyleSuccess))
		}

		m.detail.SelectionID = selection.Issue.ID
		m.detail.SelectBrowserIssue(selection.Issue.ID)
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
		return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Launched %q in background (no return flow). Use e for edit/save round-trip.", msg.action), toaster.StyleInfo))
	case mutationCatalogsLoadedMsg:
		if msg.err != nil {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Failed to load mutation catalogs: %v", msg.err), toaster.StyleError))
		}

		dialog := buildMutationDialog(msg.kind, msg.issue, msg.statuses, msg.types, msg.labels)
		m.actionState = dialog
		m.actionModal = mutationModal(dialog, m.keys)
		m.actionModal.SetSize(m.width, m.height)
		m.showActionModal = true
		return m, batchCmds(modeCmd, m.actionModal.Init())
	case statusCatalogLoadedMsg:
		if msg.err != nil {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Failed to load status catalog: %v", msg.err), toaster.StyleError))
		}

		dialog := buildMutationDialog(mutationStatus, msg.issue, msg.statuses, nil, nil)
		m.actionState = dialog
		m.actionModal = mutationModal(dialog, m.keys)
		m.actionModal.SetSize(m.width, m.height)
		m.showActionModal = true
		return m, batchCmds(modeCmd, m.actionModal.Init())
	case mutationResultMsg:
		if msg.err != nil {
			return m, batchCmds(modeCmd, m.showToast(msg.err.Error(), toaster.StyleError))
		}

		m.markBrowseSurfacesDirty()

		switch msg.kind {
		case mutationCreate:
			return m, batchCmds(modeCmd,
				m.showToast(fmt.Sprintf("Created issue %s", emptyFallback(msg.createdID, "(unknown)")), toaster.StyleSuccess),
				m.maybeAutoRefreshActiveSurfaceCmd(),
			)
		case mutationUpdate:
			return m, batchCmds(modeCmd,
				m.showToast(fmt.Sprintf("Updated issue %s", msg.issueID), toaster.StyleSuccess),
				loadDetailCmd(m.services, msg.issueID),
				m.maybeAutoRefreshActiveSurfaceCmd(),
			)
		case mutationClose:
			return m, batchCmds(modeCmd,
				m.showToast(fmt.Sprintf("Closed issue %s", msg.issueID), toaster.StyleSuccess),
				loadDetailCmd(m.services, msg.issueID),
				m.maybeAutoRefreshActiveSurfaceCmd(),
			)
		case mutationComment:
			return m, batchCmds(modeCmd,
				m.showToast(fmt.Sprintf("Added comment to %s", msg.issueID), toaster.StyleSuccess),
				loadDetailCmd(m.services, msg.issueID),
				m.maybeAutoRefreshActiveSurfaceCmd(),
			)
		case mutationStatus:
			return m, batchCmds(modeCmd,
				m.showToast(fmt.Sprintf("Updated issue status for %s", msg.issueID), toaster.StyleSuccess),
				loadDetailCmd(m.services, msg.issueID),
				m.maybeAutoRefreshActiveSurfaceCmd(),
			)
		case mutationPriorityCycle:
			return m, batchCmds(modeCmd,
				m.showToast(fmt.Sprintf("Updated issue priority for %s", msg.issueID), toaster.StyleSuccess),
				loadDetailCmd(m.services, msg.issueID),
				m.maybeAutoRefreshActiveSurfaceCmd(),
			)
		default:
			return m, modeCmd
		}
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
		searchCaptured := false
		if m.active == mode.Search {
			if m.search.CapturesShellKey(msg) {
				searchCaptured = true
			}
		}
		if searchCaptured {
			return m, modeCmd
		}

		if m.active == mode.Detail {
			m.detail.Keys = m.keys
			consumed, intent := m.detail.HandleKey(msg, m.detailViewportWidth(), m.detailViewportHeight())
			if m.detail.ConsumeOpenStatusDialogIntent() {
				issue := m.detail.Detail.Summary
				if strings.TrimSpace(issue.ID) == "" {
					if selection := m.currentSelection(); selection != nil {
						issue = selection.Issue
					}
				}
				if strings.TrimSpace(issue.ID) == "" {
					return m, batchCmds(modeCmd, m.showToast("No selected issue to update status", toaster.StyleWarn))
				}
				return m, batchCmds(modeCmd, loadStatusCatalogForIssueCmd(m.services, issue))
			}
			if m.detail.ConsumeCyclePriorityIntent() {
				issue := m.detail.Detail.Summary
				if strings.TrimSpace(issue.ID) == "" {
					if selection := m.currentSelection(); selection != nil {
						issue = selection.Issue
					}
				}
				if strings.TrimSpace(issue.ID) == "" {
					return m, batchCmds(modeCmd, m.showToast("No selected issue to update priority", toaster.StyleWarn))
				}
				return m, batchCmds(modeCmd, cyclePriorityForIssueCmd(m.services, issue))
			}
			if intent != nil {
				issueID := strings.TrimSpace(intent.IssueID)
				if issueID == "" {
					return m, modeCmd
				}
				m.active = mode.Detail
				m.detail.SelectionID = issueID
				m.detail.SelectBrowserIssue(issueID)
				m.detail.TargetID = issueID
				m.detail.Loading = true
				m.detail.Error = ""
				m.detail.ScrollOffset = 0
				return m, batchCmds(modeCmd, loadDetailCmd(m.services, issueID))
			}
			if consumed {
				return m, modeCmd
			}
		}

		switch {
		case m.keys.Match(config.ShellContext, config.ShellActionQuit, msg):
			return m, batchCmds(modeCmd, tea.Quit)
		case m.keys.Match(config.ShellContext, config.ShellActionHelp, msg):
			m.showHelp = true
			m.help.SetSize(m.width, m.height)
			return m, modeCmd
		case m.keys.Match(config.ShellContext, config.ShellActionModeBoard, msg):
			m.active = mode.Board
			m.lastBrowse = mode.Board
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
		case m.keys.Match(config.ShellContext, config.ShellActionModeSearch, msg):
			m.active = mode.Search
			m.lastBrowse = mode.Search
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
		case m.keys.Match(config.ShellContext, config.ShellActionToggleSearch, msg):
			if m.active == mode.Detail {
				m.active = mode.Board
				m.lastBrowse = mode.Board
				return m, modeCmd
			}
			if m.active == mode.Search {
				m.active = mode.Board
				m.lastBrowse = mode.Board
				return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
			}
			m.active = mode.Search
			m.lastBrowse = mode.Search
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
		case m.keys.Match(config.ShellContext, config.ShellActionModeDetail, msg):
			if m.active == mode.Board || m.active == mode.Search {
				m.lastBrowse = m.active
			}
			if m.currentSelection() == nil {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to open in detail mode", toaster.StyleWarn))
			}
			m.active = mode.Detail
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
		case m.keys.Match(config.ShellContext, config.ShellActionModeCycleNext, msg):
			m.active = nextMode(m.active, m.lastBrowse)
			m.lastBrowse = m.active
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
		case m.keys.Match(config.ShellContext, config.ShellActionModeCyclePrev, msg):
			m.active = prevMode(m.active, m.lastBrowse)
			m.lastBrowse = m.active
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
		case m.keys.Match(config.ShellContext, config.ShellActionEscape, msg):
			if m.active == mode.Detail {
				m.active = m.lastBrowse
				return m, modeCmd
			}
			if m.active == mode.Search {
				m.active = mode.Board
				m.lastBrowse = mode.Board
				return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
			}
			m.toast = m.toast.Hide()
			return m, modeCmd
		case m.keys.Match(config.ShellContext, config.ShellActionReloadDetail, msg):
			if m.active != mode.Detail {
				return m, modeCmd
			}
			if m.currentSelection() == nil {
				return m, modeCmd
			}
			m.detail.Loading = true
			m.detail.Error = ""
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
		case m.keys.Match(config.ShellContext, config.ShellActionEditIssue, msg):
			issueID, ok := m.selectedIssueID()
			if !ok {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to edit", toaster.StyleWarn))
			}
			return m, batchCmds(modeCmd, editIssueCmd(m.services, issueID))
		case m.keys.Match(config.ShellContext, config.ShellActionCreateIssue, msg):
			return m, batchCmds(modeCmd, loadMutationCatalogsCmd(m.services, mutationCreate, domain.IssueSummary{}))
		case m.keys.Match(config.ShellContext, config.ShellActionUpdateIssue, msg):
			selection := m.currentSelection()
			if selection == nil || selection.Issue.ID == "" {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to update", toaster.StyleWarn))
			}
			return m, batchCmds(modeCmd, loadMutationCatalogsCmd(m.services, mutationUpdate, selection.Issue))
		case m.keys.Match(config.ShellContext, config.ShellActionCloseIssue, msg):
			selection := m.currentSelection()
			if selection == nil || selection.Issue.ID == "" {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to close", toaster.StyleWarn))
			}
			m.actionState = mutationDialogState{kind: mutationClose, issue: selection.Issue}
			m.actionModal = mutationModal(m.actionState, m.keys)
			m.actionModal.SetSize(m.width, m.height)
			m.showActionModal = true
			return m, batchCmds(modeCmd, m.actionModal.Init())
		case m.keys.Match(config.ShellContext, config.ShellActionCommentIssue, msg):
			selection := m.currentSelection()
			if selection == nil || selection.Issue.ID == "" {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to comment on", toaster.StyleWarn))
			}
			m.actionState = mutationDialogState{kind: mutationComment, issue: selection.Issue}
			m.actionModal = mutationModal(m.actionState, m.keys)
			m.actionModal.SetSize(m.width, m.height)
			m.showActionModal = true
			return m, batchCmds(modeCmd, m.actionModal.Init())
		case m.keys.Match(config.ShellContext, config.ShellActionLaunchNvim, msg):
			if m.active != mode.Detail {
				return m, modeCmd
			}
			issueContext, ok := m.selectedIssueContext()
			if !ok {
				return m, batchCmds(modeCmd, m.showToast("No selected issue for launcher", toaster.StyleWarn))
			}
			return m, batchCmds(modeCmd, launchActionCmd(m.services, "nvim", issueContext))
		case m.keys.Match(config.ShellContext, config.ShellActionLaunchOpencode, msg):
			if m.active != mode.Detail {
				return m, modeCmd
			}
			issueContext, ok := m.selectedIssueContext()
			if !ok {
				return m, batchCmds(modeCmd, m.showToast("No selected issue for launcher", toaster.StyleWarn))
			}
			return m, batchCmds(modeCmd, launchActionCmd(m.services, "opencode", issueContext))
		case m.keys.Match(config.ShellContext, config.ShellActionLaunchShell, msg):
			if m.active != mode.Detail {
				return m, modeCmd
			}
			issueContext, ok := m.selectedIssueContext()
			if !ok {
				return m, batchCmds(modeCmd, m.showToast("No selected issue for launcher", toaster.StyleWarn))
			}
			return m, batchCmds(modeCmd, launchActionCmd(m.services, "shell-command", issueContext))
		}
	}

	return m, modeCmd
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
	if m.showActionModal {
		view = m.actionModal.Overlay(view)
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
		if m.active == mode.Detail {
			m.detail = detailsmode.Model{}
		}
		return nil
	}

	if m.detail.SelectionID != selection.Issue.ID {
		m.detail.ScrollOffset = 0
	}
	m.detail.SelectionID = selection.Issue.ID
	m.detail.SelectBrowserIssue(selection.Issue.ID)

	if m.detail.Loading && m.detail.TargetID == selection.Issue.ID {
		return nil
	}
	if !m.detail.Loading && m.detail.Detail.Summary.ID == selection.Issue.ID && m.detail.Error == "" && !m.shouldRefreshSurface(mode.Detail) {
		return nil
	}

	m.detail.Loading = true
	m.detail.Error = ""
	m.detail.TargetID = selection.Issue.ID
	return loadDetailCmd(m.services, selection.Issue.ID)
}

func (m Model) renderHeader() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(styles.ShellTitleColor).Render("Beads Workbench")

	tab := func(id mode.ID, label string) string {
		base := lipgloss.NewStyle().Padding(0, 1)
		if m.active == id {
			return base.Foreground(styles.ShellTabActiveTextColor).Background(styles.ShellTabActiveBgColor).Bold(true).Render(label)
		}
		return base.Foreground(styles.ShellTabInactiveColor).Render(label)
	}

	left := lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		tab(mode.Board, "Board"),
		" ",
		tab(mode.Search, "Search"),
	)

	context := lipgloss.NewStyle().Foreground(styles.ShellContextColor).Render(m.headerContext())
	if m.width <= 0 {
		return left
	}

	leftWidth := lipgloss.Width(left)
	contextWidth := lipgloss.Width(context)
	if leftWidth+1+contextWidth > m.width {
		available := max(0, m.width-leftWidth-1)
		if available <= 0 {
			return left
		}
		context = lipgloss.NewStyle().Foreground(styles.ShellContextColor).Render(styles.TruncateString(m.headerContext(), available))
		contextWidth = lipgloss.Width(context)
	}

	spacer := strings.Repeat(" ", max(1, m.width-leftWidth-contextWidth))
	return left + spacer + context
}

func (m Model) renderBody() string {
	workspaceWidth, workspaceHeight := m.workspaceSize()
	m.board.SetSize(workspaceWidth, workspaceHeight)
	m.search.SetSize(workspaceWidth, workspaceHeight)

	if m.active == mode.Detail {
		return m.detail.View(m.detailViewportWidth(), m.detailViewportHeight(), false)
	}

	var browse string
	if m.active == mode.Board {
		browse = m.board.View()
	} else {
		browse = m.search.View()
	}

	return browse
}

func (m Model) detailViewportHeight() int {
	_, workspaceHeight := m.workspaceSize()
	return workspaceHeight
}

func (m Model) detailViewportWidth() int {
	workspaceWidth, _ := m.workspaceSize()
	return workspaceWidth
}

func (m Model) workspaceSize() (int, int) {
	workspaceWidth := max(1, m.width)
	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())
	workspaceHeight := max(1, m.height-headerHeight-footerHeight)
	return workspaceWidth, workspaceHeight
}

func (m Model) applyWorkspaceSizeToBrowseModes() {
	workspaceWidth, workspaceHeight := m.workspaceSize()
	m.board.SetSize(workspaceWidth, workspaceHeight)
	m.search.SetSize(workspaceWidth, workspaceHeight)
}

func (m Model) renderFooter() string {
	if !m.services.Config.UI.ShowModeSwitcherHelp {
		return ""
	}

	return lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor).Render(footerHelpText(m.active, m.width, m.keys))
}

func (m *Model) showToast(message string, style toaster.Style) tea.Cmd {
	m.toast = m.toast.Show(message, style)
	return toaster.ScheduleDismiss(3 * time.Second)
}

func (m Model) boardIsLoading() bool {
	if m.board == nil {
		return false
	}
	return m.board.IsLoading()
}

func (m Model) searchIsLoading() bool {
	if m.search == nil {
		return false
	}
	return m.search.IsLoading()
}

func (m Model) searchResultCount() int {
	if m.search == nil {
		return 0
	}
	return m.search.ResultCount()
}

func (m *Model) maybeAutoRefreshActiveSurfaceCmd() tea.Cmd {
	return m.maybeAutoRefreshActiveSurfaceCmdWithPolicy(false)
}

func (m *Model) maybeAutoRefreshActiveSurfaceCmdOnFocusRegain() tea.Cmd {
	return m.maybeAutoRefreshActiveSurfaceCmdWithPolicy(true)
}

func (m *Model) maybeAutoRefreshActiveSurfaceCmdWithPolicy(force bool) tea.Cmd {
	if m.showHelp || m.showActionModal {
		return nil
	}
	if m.focusKnown && !m.terminalFocused {
		return nil
	}
	if !force && !m.shouldRefreshSurface(m.active) {
		return nil
	}
	return m.refreshActiveSurfaceCmd()
}

func (m *Model) refreshActiveSurfaceCmd() tea.Cmd {
	switch m.active {
	case mode.Board:
		if m.boardIsLoading() {
			return nil
		}
		m.markSurfaceRefreshed(mode.Board)
		return m.board.AutoRefresh()
	case mode.Search:
		if m.searchIsLoading() {
			return nil
		}
		m.markSurfaceRefreshed(mode.Search)
		return m.search.AutoRefresh()
	case mode.Detail:
		if m.detail.Loading {
			return nil
		}
		selection := m.currentSelection()
		if selection == nil || selection.Issue.ID == "" {
			return nil
		}
		m.detail.SelectionID = selection.Issue.ID
		m.detail.SelectBrowserIssue(selection.Issue.ID)
		m.detail.Loading = true
		m.detail.Error = ""
		m.detail.TargetID = selection.Issue.ID
		m.markSurfaceRefreshed(mode.Detail)
		return loadDetailCmd(m.services, selection.Issue.ID)
	default:
		return nil
	}
}

func (m *Model) markBrowseSurfacesDirty() {
	m.markSurfaceDirty(mode.Board, mode.Search)
}

func (m *Model) markSurfaceDirty(surfaces ...mode.ID) {
	if m.refreshStateBySurface == nil {
		m.refreshStateBySurface = make(map[mode.ID]surfaceRefreshState)
	}
	for _, surface := range surfaces {
		state := m.refreshStateBySurface[surface]
		state.dirty = true
		m.refreshStateBySurface[surface] = state
	}
}

func (m *Model) markSurfaceRefreshed(surface mode.ID) {
	if m.refreshStateBySurface == nil {
		m.refreshStateBySurface = make(map[mode.ID]surfaceRefreshState)
	}
	state := m.refreshStateBySurface[surface]
	state.dirty = false
	state.lastRefresh = modelNow()
	m.refreshStateBySurface[surface] = state
}

func (m *Model) shouldRefreshSurface(surface mode.ID) bool {
	state, ok := m.refreshStateBySurface[surface]
	if !ok {
		return true
	}
	if state.dirty {
		return true
	}
	if state.lastRefresh.IsZero() {
		return true
	}
	return modelNow().Sub(state.lastRefresh) >= refreshTickInterval
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
	return m.board.Update(msg)
}

func (m *Model) forwardSearchMessage(msg tea.Msg) tea.Cmd {
	if m.search == nil || !m.shouldForwardToSearch(msg) {
		return nil
	}
	return m.search.Update(msg)
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

func nextMode(current mode.ID, lastBrowse mode.ID) mode.ID {
	switch current {
	case mode.Board:
		return mode.Search
	case mode.Search:
		return mode.Board
	case mode.Detail:
		if lastBrowse == mode.Search {
			return mode.Board
		}
		return mode.Search
	default:
		return mode.Board
	}
}

func prevMode(current mode.ID, lastBrowse mode.ID) mode.ID {
	switch current {
	case mode.Board:
		return mode.Search
	case mode.Search:
		return mode.Board
	case mode.Detail:
		if lastBrowse == mode.Search {
			return mode.Board
		}
		return mode.Search
	default:
		return mode.Search
	}
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

func shellKeyHelp(keys config.ResolvedKeyBindings) string {
	return strings.Join([]string{
		"Mode switching:",
		fmt.Sprintf("  %s = toggle Board/Search", keys.DisplayLabel(config.ShellContext, config.ShellActionToggleSearch)),
		fmt.Sprintf("  %s = open selected issue detail", keys.DisplayLabel(config.ShellContext, config.ShellActionModeDetail)),
		"",
		"Selection:",
		fmt.Sprintf("  Board: %s switch columns, %s move within a column", combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveLeft, config.BoardActionMoveRight), combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveUp, config.BoardActionMoveDown)),
		fmt.Sprintf("  Search: type to query; %s focuses query; %s/%s switch panes; %s/%s moves results; %s/%s cycles focus", keys.DisplayLabel(config.SearchContext, config.SearchActionFocusQuery), keys.DisplayLabel(config.SearchContext, config.SearchActionFocusLeft), keys.DisplayLabel(config.SearchContext, config.SearchActionFocusRight), keys.DisplayLabel(config.SearchContext, config.SearchActionMoveDown), keys.DisplayLabel(config.SearchContext, config.SearchActionMoveUp), keys.DisplayLabel(config.SearchContext, config.SearchActionCycleFocusNext), keys.DisplayLabel(config.SearchContext, config.SearchActionCycleFocusPrev)),
		"",
		"Actions:",
		fmt.Sprintf("  %s = create issue (inline modal)", keys.DisplayLabel(config.ShellContext, config.ShellActionCreateIssue)),
		fmt.Sprintf("  %s = update selected issue metadata", keys.DisplayLabel(config.ShellContext, config.ShellActionUpdateIssue)),
		fmt.Sprintf("  %s = close selected issue", keys.DisplayLabel(config.ShellContext, config.ShellActionCloseIssue)),
		fmt.Sprintf("  %s = add comment to selected issue", keys.DisplayLabel(config.ShellContext, config.ShellActionCommentIssue)),
		fmt.Sprintf("  %s = edit selected issue in external editor", keys.DisplayLabel(config.ShellContext, config.ShellActionEditIssue)),
		fmt.Sprintf("  %s/%s/%s = launch external tools (detail mode, background fire-and-forget)", keys.DisplayLabel(config.ShellContext, config.ShellActionLaunchNvim), keys.DisplayLabel(config.ShellContext, config.ShellActionLaunchOpencode), keys.DisplayLabel(config.ShellContext, config.ShellActionLaunchShell)),
		"  launcher actions do not provide in-app return/save handling",
		fmt.Sprintf("  use %s for edit/save round-trip that reloads detail", keys.DisplayLabel(config.ShellContext, config.ShellActionEditIssue)),
		fmt.Sprintf("  %s = open selected issue in detail mode", keys.DisplayLabel(config.BoardContext, config.BoardActionOpenDetail)),
		fmt.Sprintf("  detail scroll: %s/%s, %s/%s, %s/%s", keys.DisplayLabel(config.DetailContext, config.DetailActionScrollDown), keys.DisplayLabel(config.DetailContext, config.DetailActionScrollUp), keys.DisplayLabel(config.DetailContext, config.DetailActionPageUp), keys.DisplayLabel(config.DetailContext, config.DetailActionPageDown), keys.DisplayLabel(config.DetailContext, config.DetailActionHome), keys.DisplayLabel(config.DetailContext, config.DetailActionEnd)),
		fmt.Sprintf("  %s = reload detail mode from gateway", keys.DisplayLabel(config.ShellContext, config.ShellActionReloadDetail)),
		fmt.Sprintf("  %s = return from detail/search to browse / dismiss toast", keys.DisplayLabel(config.ShellContext, config.ShellActionEscape)),
		fmt.Sprintf("  %s = toggle help", keys.DisplayLabel(config.ShellContext, config.ShellActionHelp)),
		fmt.Sprintf("  %s = quit", keys.DisplayLabel(config.ShellContext, config.ShellActionQuit)),
		"",
		"Detail presentation model (v1): dedicated detail mode",
		"  - Board/Search prioritize overview triage density",
		fmt.Sprintf("  - %s opens full issue detail view", keys.DisplayLabel(config.BoardContext, config.BoardActionOpenDetail)),
	}, "\n")
}

func combineDisplayLabels(keys config.ResolvedKeyBindings, context, first, second string) string {
	left := keys.DisplayLabel(context, first)
	right := keys.DisplayLabel(context, second)
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + " or " + right
}

func (m Model) headerContext() string {
	variants := m.headerContextVariants()
	if len(variants) == 0 {
		return ""
	}

	if m.width <= 0 {
		return variants[0]
	}

	for _, v := range variants {
		if lipgloss.Width(v) <= m.width/2 {
			return v
		}
	}

	return variants[len(variants)-1]
}

func (m Model) headerContextVariants() []string {
	if m.active == mode.Detail {
		id := strings.TrimSpace(m.detail.Detail.Summary.ID)
		if id == "" {
			id = strings.TrimSpace(m.detail.SelectionID)
		}
		status := strings.TrimSpace(m.detail.Detail.Summary.Status)
		if id != "" && status != "" {
			return []string{fmt.Sprintf("Detail: %s · %s", id, status), fmt.Sprintf("Detail: %s", id), "Detail"}
		}
		if id != "" {
			return []string{fmt.Sprintf("Detail: %s", id), "Detail"}
		}
		return []string{"Detail"}
	}

	prefix := "Board"
	if m.active == mode.Search {
		prefix = fmt.Sprintf("Search: %d results", m.searchResultCount())
	}

	selectedLong, selectedShort := "Selected: none", "Sel: none"
	if sel := m.currentSelection(); sel != nil {
		selectedLong = fmt.Sprintf("Selected: %s (%s)", sel.Issue.ID, sel.Issue.Status)
		selectedShort = fmt.Sprintf("Sel: %s", sel.Issue.ID)
	}

	loadingSummary := loading.Summary(m.loadingStates())
	loadingShort := loadingSummary
	if loadingSummary == "Idle" {
		loadingShort = "idle"
	}

	variants := []string{
		fmt.Sprintf("%s · %s · %s", prefix, selectedLong, loadingSummary),
		fmt.Sprintf("%s · %s", prefix, selectedLong),
		fmt.Sprintf("%s · %s · %s", prefix, selectedShort, loadingShort),
		prefix,
	}

	if m.active == mode.Search {
		variants = append([]string{
			fmt.Sprintf("Search · %s · %s", selectedLong, loadingSummary),
			fmt.Sprintf("Search · %s", selectedShort),
		}, variants...)
	}

	return variants
}

func (m Model) loadingStates() []loading.State {
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
	return loadingStates
}

func footerHelpText(active mode.ID, width int, keys config.ResolvedKeyBindings) string {
	if width < 90 {
		switch active {
		case mode.Search:
			return fmt.Sprintf("Search: type %s %s/%s %s %s %s", keys.DisplayPrimary(config.SearchContext, config.SearchActionFocusQuery), keys.DisplayPrimary(config.SearchContext, config.SearchActionCycleFocusNext), keys.DisplayPrimary(config.SearchContext, config.SearchActionCycleFocusPrev), keys.DisplayPrimary(config.SearchContext, config.SearchActionMoveDown)+"/"+keys.DisplayPrimary(config.SearchContext, config.SearchActionMoveUp), keys.DisplayPrimary(config.SearchContext, config.SearchActionOpenDetail), keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape))
		case mode.Detail:
			return fmt.Sprintf("Detail: %s/%s %s/%s %s/%s %s", keys.DisplayPrimary(config.DetailContext, config.DetailActionScrollDown), keys.DisplayPrimary(config.DetailContext, config.DetailActionScrollUp), keys.DisplayPrimary(config.DetailContext, config.DetailActionPageUp), keys.DisplayPrimary(config.DetailContext, config.DetailActionPageDown), keys.DisplayPrimary(config.DetailContext, config.DetailActionHome), keys.DisplayPrimary(config.DetailContext, config.DetailActionEnd), keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape))
		default:
			return fmt.Sprintf("Board: %s %s %s %s %s %s", keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveLeft)+"/"+keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveRight), keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveDown)+"/"+keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveUp), keys.DisplayPrimary(config.BoardContext, config.BoardActionOpenDetail), keys.DisplayPrimary(config.ShellContext, config.ShellActionToggleSearch), keys.DisplayPrimary(config.ShellContext, config.ShellActionHelp), keys.DisplayPrimary(config.ShellContext, config.ShellActionQuit))
		}
	}

	switch active {
	case mode.Search:
		return fmt.Sprintf("Search: type query · %s focus · %s/%s switch panes · %s/%s results · %s detail · %s board", keys.DisplayPrimary(config.SearchContext, config.SearchActionFocusQuery), keys.DisplayPrimary(config.SearchContext, config.SearchActionFocusLeft), keys.DisplayPrimary(config.SearchContext, config.SearchActionFocusRight), keys.DisplayPrimary(config.SearchContext, config.SearchActionMoveDown), keys.DisplayPrimary(config.SearchContext, config.SearchActionMoveUp), keys.DisplayPrimary(config.SearchContext, config.SearchActionOpenDetail), keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape))
	case mode.Detail:
		return fmt.Sprintf("Detail: %s/%s scroll · %s/%s page · %s/%s bounds · %s edit · %s back", keys.DisplayPrimary(config.DetailContext, config.DetailActionScrollDown), keys.DisplayPrimary(config.DetailContext, config.DetailActionScrollUp), keys.DisplayPrimary(config.DetailContext, config.DetailActionPageUp), keys.DisplayPrimary(config.DetailContext, config.DetailActionPageDown), keys.DisplayPrimary(config.DetailContext, config.DetailActionHome), keys.DisplayPrimary(config.DetailContext, config.DetailActionEnd), keys.DisplayPrimary(config.ShellContext, config.ShellActionEditIssue), keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape))
	default:
		return fmt.Sprintf("Board: %s/%s columns · %s/%s issues · %s detail · %s search · %s help · %s quit", keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveLeft), keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveRight), keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveDown), keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveUp), keys.DisplayPrimary(config.BoardContext, config.BoardActionOpenDetail), keys.DisplayPrimary(config.ShellContext, config.ShellActionToggleSearch), keys.DisplayPrimary(config.ShellContext, config.ShellActionHelp), keys.DisplayPrimary(config.ShellContext, config.ShellActionQuit))
	}
}

func (m Model) shouldCaptureKeyForOverlay(msg tea.Msg) bool {
	if !m.showHelp && !m.showActionModal {
		return false
	}
	_, isKey := msg.(tea.KeyMsg)
	return isKey
}

func loadMutationCatalogsCmd(services Services, kind mutationKind, issue domain.IssueSummary) tea.Cmd {
	return func() tea.Msg {
		statuses, err := services.Gateway.StatusCatalog(context.Background())
		if err != nil {
			return mutationCatalogsLoadedMsg{kind: kind, issue: issue, err: fmt.Errorf("status catalog: %w", err)}
		}

		types, err := services.Gateway.TypeCatalog(context.Background())
		if err != nil {
			return mutationCatalogsLoadedMsg{kind: kind, issue: issue, err: fmt.Errorf("type catalog: %w", err)}
		}

		labels, err := services.Gateway.LabelCatalog(context.Background())
		if err != nil {
			return mutationCatalogsLoadedMsg{kind: kind, issue: issue, err: fmt.Errorf("label catalog: %w", err)}
		}

		return mutationCatalogsLoadedMsg{kind: kind, issue: issue, statuses: statuses, types: types, labels: labels}
	}
}

func loadStatusCatalogForIssueCmd(services Services, issue domain.IssueSummary) tea.Cmd {
	return func() tea.Msg {
		statuses, err := services.Gateway.StatusCatalog(context.Background())
		if err != nil {
			return statusCatalogLoadedMsg{issue: issue, err: fmt.Errorf("status catalog: %w", err)}
		}
		return statusCatalogLoadedMsg{issue: issue, statuses: statuses}
	}
}

func cyclePriorityForIssueCmd(services Services, issue domain.IssueSummary) tea.Cmd {
	return func() tea.Msg {
		next := (issue.Priority + 1) % 5
		if next < 0 {
			next = 0
		}
		if err := services.Gateway.UpdateIssue(context.Background(), issue.ID, domain.UpdateIssueInput{Priority: &next}); err != nil {
			return mutationResultMsg{kind: mutationPriorityCycle, issueID: issue.ID, err: fmt.Errorf("update priority failed: %w", err)}
		}
		return mutationResultMsg{kind: mutationPriorityCycle, issueID: issue.ID}
	}
}

func buildMutationDialog(kind mutationKind, issue domain.IssueSummary, statuses []domain.StatusOption, types []domain.TypeOption, labels []domain.LabelOption) mutationDialogState {
	statusNames := make(map[string]struct{}, len(statuses))
	typeNames := make(map[string]struct{}, len(types))
	labelNames := make(map[string]struct{}, len(labels))

	statusList := make([]string, 0, len(statuses))
	typeList := make([]string, 0, len(types))
	labelList := make([]string, 0, len(labels))

	for _, option := range statuses {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		statusNames[name] = struct{}{}
		statusList = append(statusList, name)
	}

	for _, option := range types {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		typeNames[name] = struct{}{}
		typeList = append(typeList, name)
	}

	for _, option := range labels {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		labelNames[name] = struct{}{}
		labelList = append(labelList, name)
	}

	return mutationDialogState{
		kind:        kind,
		issue:       issue,
		statusNames: statusNames,
		typeNames:   typeNames,
		labelNames:  labelNames,
		statusList:  strings.Join(statusList, ", "),
		typeList:    strings.Join(typeList, ", "),
		labelList:   strings.Join(labelList, ", "),
	}
}

func mutationModal(state mutationDialogState, keys config.ResolvedKeyBindings) modal.Model {
	switch state.kind {
	case mutationCreate:
		return modal.NewWithKeys(modal.Config{
			Title:       "Create Issue",
			Message:     fmt.Sprintf("Inline quick-create flow (rich editing stays in external editor).\nTypes: %s\nLabels: %s", emptyFallback(state.typeList, "(none)"), emptyFallback(state.labelList, "(none)")),
			ConfirmText: "Create",
			MinWidth:    92,
			Required:    false,
			Inputs: []modal.InputConfig{
				{Key: "title", Label: "Title", Placeholder: "Issue title"},
				{Key: "type", Label: "Type", Placeholder: emptyFallback(state.typeList, "task")},
				{Key: "priority", Label: "Priority", Placeholder: "0-3"},
				{Key: "assignee", Label: "Assignee", Placeholder: "username"},
				{Key: "labels", Label: "Labels", Placeholder: "comma,separated"},
				{Key: "description", Label: "Description", Placeholder: "Short description"},
			},
		}, modal.BindingsFromConfig(keys))
	case mutationUpdate:
		return modal.NewWithKeys(modal.Config{
			Title:       fmt.Sprintf("Update Issue %s", state.issue.ID),
			Message:     fmt.Sprintf("Quick metadata update.\nStatuses: %s\nTypes: %s\nLabels: %s", emptyFallback(state.statusList, "(none)"), emptyFallback(state.typeList, "(none)"), emptyFallback(state.labelList, "(none)")),
			ConfirmText: "Update",
			MinWidth:    92,
			Required:    false,
			Inputs: []modal.InputConfig{
				{Key: "title", Label: "Title", Value: state.issue.Title, Placeholder: "Leave unchanged"},
				{Key: "status", Label: "Status", Value: state.issue.Status, Placeholder: emptyFallback(state.statusList, state.issue.Status)},
				{Key: "type", Label: "Type", Value: state.issue.Type, Placeholder: emptyFallback(state.typeList, state.issue.Type)},
				{Key: "priority", Label: "Priority", Value: strconv.Itoa(state.issue.Priority), Placeholder: "0-3"},
				{Key: "assignee", Label: "Assignee", Value: state.issue.Assignee, Placeholder: "username"},
				{Key: "labels", Label: "Labels", Value: strings.Join(state.issue.Labels, ","), Placeholder: "comma,separated"},
			},
		}, modal.BindingsFromConfig(keys))
	case mutationClose:
		return modal.NewWithKeys(modal.Config{
			Title:          fmt.Sprintf("Close Issue %s", state.issue.ID),
			Message:        "Provide an optional close reason.",
			ConfirmText:    "Close",
			ConfirmVariant: modal.ButtonDanger,
			Required:       false,
			MinWidth:       72,
			Inputs: []modal.InputConfig{
				{Key: "reason", Label: "Reason", Placeholder: "completed"},
			},
		}, modal.BindingsFromConfig(keys))
	case mutationComment:
		return modal.NewWithKeys(modal.Config{
			Title:       fmt.Sprintf("Comment on %s", state.issue.ID),
			Message:     "Add a comment for the selected issue.",
			ConfirmText: "Add comment",
			Required:    false,
			MinWidth:    72,
			Inputs: []modal.InputConfig{
				{Key: "body", Label: "Comment", Placeholder: "Comment text"},
			},
		}, modal.BindingsFromConfig(keys))
	case mutationStatus:
		return modal.NewWithKeys(modal.Config{
			Title:       fmt.Sprintf("Update Status %s", state.issue.ID),
			Message:     fmt.Sprintf("Set the issue status. Available: %s", emptyFallback(state.statusList, "(none)")),
			ConfirmText: "Update",
			MinWidth:    72,
			Required:    true,
			Inputs: []modal.InputConfig{
				{Key: "status", Label: "Status", Value: state.issue.Status, Placeholder: emptyFallback(state.statusList, state.issue.Status)},
			},
		}, modal.BindingsFromConfig(keys))
	default:
		return modal.NewWithKeys(modal.Config{Title: "Action", Message: "Unsupported action", Required: false}, modal.BindingsFromConfig(keys))
	}
}

func submitMutationCmd(services Services, state mutationDialogState, values map[string]string) tea.Cmd {
	return func() tea.Msg {
		switch state.kind {
		case mutationCreate:
			title := strings.TrimSpace(values["title"])
			if title == "" {
				return mutationResultMsg{kind: mutationCreate, err: fmt.Errorf("create issue failed: title is required")}
			}

			priority, err := parsePriority(values["priority"])
			if err != nil {
				return mutationResultMsg{kind: mutationCreate, err: fmt.Errorf("create issue failed: %w", err)}
			}

			labels := parseCommaList(values["labels"])
			result, err := services.Gateway.CreateIssue(context.Background(), domain.CreateIssueInput{
				Title:       title,
				Description: strings.TrimSpace(values["description"]),
				Type:        strings.TrimSpace(values["type"]),
				Priority:    priority,
				Assignee:    strings.TrimSpace(values["assignee"]),
				Labels:      labels,
			})
			if err != nil {
				return mutationResultMsg{kind: mutationCreate, err: fmt.Errorf("create issue failed: %w", err)}
			}

			return mutationResultMsg{kind: mutationCreate, createdID: result.IssueID}
		case mutationUpdate:
			status := strings.TrimSpace(values["status"])
			if status != "" {
				if _, ok := state.statusNames[status]; len(state.statusNames) > 0 && !ok {
					return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: unknown status %q", status)}
				}
			}

			issueType := strings.TrimSpace(values["type"])
			if issueType != "" {
				if _, ok := state.typeNames[issueType]; len(state.typeNames) > 0 && !ok {
					return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: unknown type %q", issueType)}
				}
			}

			priority, err := parseRequiredPriority(values["priority"])
			if err != nil {
				return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: %w", err)}
			}

			labels := parseCommaList(values["labels"])
			for _, label := range labels {
				if _, ok := state.labelNames[label]; len(state.labelNames) > 0 && !ok {
					return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: unknown label %q", label)}
				}
			}

			title := strings.TrimSpace(values["title"])
			assignee := strings.TrimSpace(values["assignee"])
			input := domain.UpdateIssueInput{}
			if title != "" {
				input.Title = &title
			}
			if status != "" {
				input.Status = &status
			}
			if issueType != "" {
				input.Type = &issueType
			}
			if priority != nil {
				input.Priority = priority
			}
			if assignee != "" {
				input.Assignee = &assignee
			}
			if len(labels) > 0 {
				input.Labels = labels
			} else {
				input.ClearLabels = true
			}

			if err := services.Gateway.UpdateIssue(context.Background(), state.issue.ID, input); err != nil {
				return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: %w", err)}
			}

			return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID}
		case mutationClose:
			reason := strings.TrimSpace(values["reason"])
			if err := services.Gateway.CloseIssue(context.Background(), state.issue.ID, domain.CloseIssueInput{Reason: reason}); err != nil {
				return mutationResultMsg{kind: mutationClose, issueID: state.issue.ID, err: fmt.Errorf("close issue failed: %w", err)}
			}
			return mutationResultMsg{kind: mutationClose, issueID: state.issue.ID}
		case mutationComment:
			body := strings.TrimSpace(values["body"])
			if body == "" {
				return mutationResultMsg{kind: mutationComment, issueID: state.issue.ID, err: fmt.Errorf("add comment failed: body is required")}
			}
			if err := services.Gateway.AddComment(context.Background(), state.issue.ID, domain.AddCommentInput{Body: body}); err != nil {
				return mutationResultMsg{kind: mutationComment, issueID: state.issue.ID, err: fmt.Errorf("add comment failed: %w", err)}
			}
			return mutationResultMsg{kind: mutationComment, issueID: state.issue.ID}
		case mutationStatus:
			status := strings.TrimSpace(values["status"])
			if status == "" {
				return mutationResultMsg{kind: mutationStatus, issueID: state.issue.ID, err: fmt.Errorf("update status failed: status is required")}
			}
			if _, ok := state.statusNames[status]; len(state.statusNames) > 0 && !ok {
				return mutationResultMsg{kind: mutationStatus, issueID: state.issue.ID, err: fmt.Errorf("update status failed: unknown status %q", status)}
			}

			if err := services.Gateway.UpdateIssue(context.Background(), state.issue.ID, domain.UpdateIssueInput{Status: &status}); err != nil {
				return mutationResultMsg{kind: mutationStatus, issueID: state.issue.ID, err: fmt.Errorf("update status failed: %w", err)}
			}
			return mutationResultMsg{kind: mutationStatus, issueID: state.issue.ID}
		default:
			return mutationResultMsg{kind: state.kind, issueID: state.issue.ID, err: fmt.Errorf("unsupported mutation action")}
		}
	}
}

func parseCommaList(value string) []string {
	parts := strings.Split(value, ",")
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		label := strings.TrimSpace(part)
		if label == "" {
			continue
		}
		labels = append(labels, label)
	}
	return labels
}

func parsePriority(value string) (*int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil, fmt.Errorf("priority must be an integer")
	}

	return &parsed, nil
}

func parseRequiredPriority(value string) (*int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("priority is required")
	}

	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil, fmt.Errorf("priority must be an integer")
	}

	return &parsed, nil
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
