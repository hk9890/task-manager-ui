package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/logging"
	"github.com/hk9890/task-manager-ui/internal/mode"
	boardmode "github.com/hk9890/task-manager-ui/internal/mode/board"
	"github.com/hk9890/task-manager-ui/internal/mode/detail"
	searchmode "github.com/hk9890/task-manager-ui/internal/mode/search"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// Model is the root Bubble Tea shell for Task Manager UI.
//
// v1 detail presentation model keeps browse and full detail separated:
//   - Board/Search prioritize high-density triage browsing.
//   - Full issue inspection stays in dedicated detail mode.
type Model struct {
	services Services
	keys     config.ResolvedKeyBindings

	// fatalErrTitle and fatalErrBody are set when a startup health check detects
	// that the app cannot run. When fatalErrTitle is non-empty, View() renders
	// the fatal error screen and Update() only handles quit keys and window resize.
	fatalErrTitle string
	fatalErrBody  string

	active     mode.ID
	lastBrowse mode.ID

	selectedByMode map[mode.ID]*mode.Selection

	board  *boardmode.Model
	search *searchmode.Model

	detail detail.Model

	toast toaster.Model

	help     modal.Model
	showHelp bool

	actionModal     modal.Model
	showActionModal bool
	actionState     mutationDialogState

	focusKnown      bool
	terminalFocused bool

	// searchInitDone tracks whether the first lazy search init has been fired.
	// Search mode is not pre-loaded at startup; the first mode switch to Search
	// triggers Init() and sets this flag so subsequent entries do not reload.
	searchInitDone bool

	refreshStateBySurface map[mode.ID]surfaceRefreshState

	spinnerFrame int

	width  int
	height int

	// sizeKnown is set to true once the first tea.WindowSizeMsg has been
	// processed. View() returns an empty string until sizeKnown is true so that
	// the first rendered frame always uses the actual terminal dimensions rather
	// than the defaultViewportWidth/defaultViewportHeight placeholders. This
	// prevents the "doubled column-top borders" artifact that occurred when
	// Bubble Tea rendered a short default-size frame immediately on startup and
	// then a taller post-resize frame that the terminal renderer could not fully
	// overwrite.
	sizeKnown bool

	// pendingDialog guards an in-flight async dialog-open. It is set when the
	// app dispatches an async catalog-load Cmd (status or create/update) and
	// cleared at a single choke point at the top of the tea.KeyMsg branch so
	// that any key — particularly ESC — arriving during the load window can
	// cancel the pending open before the catalog response arrives. The
	// catalog-loaded handlers check the guard before opening the modal; if the
	// guard is not active they drop the result silently.
	pendingDialog pendingDialogGuard

	runtime RuntimeOptions

	// scheduleRefreshTick, scheduleToastDismiss, scheduleSpinnerTick are the
	// per-Model scheduler functions. Production code initialises them to the
	// default*Schedule* functions; tests override them directly on the Model
	// instance without needing a global mutex.
	scheduleRefreshTick  func() tea.Cmd
	scheduleToastDismiss func(time.Duration, int) tea.Cmd
	scheduleSpinnerTick  func() tea.Cmd

	// onEditIssueResult is a test-only hook called after editIssueResultMsg is
	// fully processed and the toast has been set. It is nil in production.
	// Tests can use it to replace a time.Sleep settle budget with a precise
	// synchronisation point. Set via the model field directly in test code
	// (the field is unexported; it is accessible from within package app).
	onEditIssueResult func()
}

// NewModel builds the root shell model.
func NewModel(services Services) (Model, error) {
	return NewModelWithOptions(services, RuntimeOptions{})
}

// NewModelWithOptions builds the root shell model with runtime toggles.
// It returns an error if the keybindings in services.Config cannot be resolved,
// which can happen when callers construct Config directly (tests, programmatic
// embed) without going through config.Load.
func NewModelWithOptions(services Services, runtime RuntimeOptions) (Model, error) {
	keys, err := config.ResolveKeyBindings(services.Config.KeyBindings)
	if err != nil {
		return Model{}, fmt.Errorf("invalid keybindings in app model: %w", err)
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
		// context.Background() is used here because the app model has no parent
		// context today. This preserves prior behaviour while making future
		// cancellation threading possible without touching the mode packages.
		board:  boardmode.NewModel(context.Background(), services.Repo, logging.WithComponent(services.Logger, "board"), keys),
		search: searchmode.NewModel(context.Background(), services.Repo, logging.WithComponent(services.Logger, "search"), keys),
		detail: detail.Model{Keys: keys},
		toast:  toaster.New(),
		help:   help,
		width:  defaultViewportWidth,
		height: defaultViewportHeight,
		refreshStateBySurface: map[mode.ID]surfaceRefreshState{
			mode.Board:  {lastRefresh: now},
			mode.Search: {lastRefresh: now},
			mode.Detail: {},
		},
		runtime:              runtime,
		scheduleRefreshTick:  defaultScheduleRefreshTick,
		scheduleToastDismiss: defaultScheduleToastDismiss,
		scheduleSpinnerTick:  defaultScheduleSpinnerTick,
	}, nil
}

// Init fires the startup health check and the spinner tick. Board loads are
// deferred until the health check passes (see startupHealthCheckMsg handler in
// Update). Search is deferred further until the user first switches to search
// mode; see lazySearchInitCmd.
func (m Model) Init() tea.Cmd {
	m.applyWorkspaceSizeToBrowseModes()
	healthCheckCmd := func() tea.Msg {
		err := m.services.Repo.HealthCheck(context.Background())
		return startupHealthCheckMsg{err: err}
	}
	if m.runtime.DisableAutoRefresh {
		return tea.Batch(healthCheckCmd, m.scheduleSpinnerTick())
	}
	return tea.Batch(healthCheckCmd, m.scheduleRefreshTick(), m.scheduleSpinnerTick())
}

// lazySearchInitCmd fires m.search.Init() exactly once — the first time the
// active mode is Search. It is safe to call on every mode transition; it is a
// no-op when m.active is not Search, and a no-op after the first search init.
// Subsequent re-entries into search mode use the normal auto-refresh path via
// maybeAutoRefreshActiveSurfaceCmd.
//
// When it fires the initial load it also marks the search surface as refreshed
// so the dirty flag is cleared; this prevents a double-load that would occur if
// maybeAutoRefreshActiveSurfaceCmd ran immediately after (which it cannot,
// because Init sets search.loading=true and the auto-refresh path gates on that
// flag).
func (m *Model) lazySearchInitCmd() tea.Cmd {
	if m.active != mode.Search {
		return nil
	}
	if m.searchInitDone {
		return nil
	}
	m.searchInitDone = true
	m.markSurfaceRefreshed(mode.Search)
	return m.search.Init()
}

// Update handles root-level shell messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle startup health check result before any other processing.
	if check, ok := msg.(startupHealthCheckMsg); ok {
		if check.err != nil {
			var gwErr domain.RepositoryError
			if errors.As(check.err, &gwErr) && gwErr.Code == domain.ErrorCodeNoDatabaseFound {
				m.fatalErrTitle = "no task-manager store here"
				m.fatalErrBody = "No .tasks store was found in this directory.\n\nRun 'taskmgr init' to create one, or use --cwd to point to a directory that contains one."
				slog.Default().Error("task-manager health check failed", "error", check.err)
				return m, nil
			}
		}
		// Health check passed — fire board loads now. Calling m.board.Init()
		// here (from Update, which returns the model) correctly persists the
		// board mutation (pendingResults=4, inflight=true) unlike calling it
		// from Init() (value receiver, mutations discarded).
		return m, m.board.Init()
	}

	// When a fatal error is set, only handle window resize and quit.
	if m.fatalErrTitle != "" {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.sizeKnown = true
			m.width = msg.Width
			m.height = msg.Height
		case tea.KeyMsg:
			if m.keys.Match(config.ShellContext, config.ShellActionQuit, msg) ||
				msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}
		return m, nil
	}

	modeCmd := tea.Cmd(nil)
	if !m.shouldCaptureKeyForOverlay(msg) {
		modeCmd = m.forwardModeMessages(msg)
	}

	if m.showActionModal {
		if size, ok := msg.(tea.WindowSizeMsg); ok {
			m.sizeKnown = true
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
			m.sizeKnown = true
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
		if m.runtime.DisableAutoRefresh {
			return m, modeCmd
		}
		return m, batchCmds(modeCmd, m.maybeAutoRefreshActiveSurfaceCmdOnFocusRegain())
	case tea.BlurMsg:
		m.focusKnown = true
		m.terminalFocused = false
		return m, modeCmd
	case refreshTickMsg:
		if m.runtime.DisableAutoRefresh {
			return m, modeCmd
		}
		return m, batchCmds(modeCmd, m.scheduleRefreshTick(), m.maybeAutoRefreshActiveSurfaceCmd())
	case loading.TickMsg:
		m.spinnerFrame = loading.NextFrame(m.spinnerFrame)
		return m, batchCmds(modeCmd, m.scheduleSpinnerTick())
	case tea.WindowSizeMsg:
		m.sizeKnown = true
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
			// Clear any pending drill-focus counter so a subsequent load is not
			// incorrectly treated as the real-data leg of a drill sequence.
			m.detail.ClearDrillFocus()
			return m, batchCmds(modeCmd, m.showToast("Failed to load selected issue details", toaster.StyleError))
		}

		m.detail.Error = ""
		if strings.TrimSpace(msg.issueID) == strings.TrimSpace(m.detail.SelectionID) {
			m.detail.ApplyLoadedDetail(msg.issueID, msg.detail)
		} else {
			m.detail.ApplyPreviewDetail(msg.detail)
		}
		m.detail.ClampScroll(m.detailViewportWidth(), m.detailViewportHeight())
		return m, modeCmd
	case editIssuePreparedMsg:
		return m.handleEditIssuePrepared(modeCmd, msg)
	case editorExitedMsg:
		return m.handleEditorExited(modeCmd, msg)
	case editIssueResultMsg:
		return m.handleEditIssueResult(modeCmd, msg)
	case launchActionResultMsg:
		if msg.err != nil {
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Launcher action %q failed: %v", msg.action, msg.err), toaster.StyleError))
		}
		return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Launched %q in background (no return flow). Use e for edit/save round-trip.", msg.action), toaster.StyleInfo))
	case mutationCatalogsLoadedMsg:
		if msg.err != nil {
			m.pendingDialog = pendingDialogGuard{}
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Failed to load mutation catalogs: %v", msg.err), toaster.StyleError))
		}

		// Only open the modal if the pending-dialog guard is still active for
		// this kind. If the guard was cleared by a key press (ESC or any other
		// key arriving during the load window), drop the result silently.
		if !m.pendingDialog.active || m.pendingDialog.kind != msg.kind {
			return m, modeCmd
		}
		m.pendingDialog = pendingDialogGuard{}

		dialog := buildMutationDialog(msg.kind, msg.issue, msg.statuses, msg.types, msg.labels)
		m.actionState = dialog
		m.actionModal = mutationModal(dialog, m.keys)
		m.actionModal.SetSize(m.width, m.height)
		m.showActionModal = true
		return m, batchCmds(modeCmd, m.actionModal.Init())
	case mutationResultMsg:
		return m.handleMutationResult(modeCmd, msg)
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
		// Only dismiss when the timer belongs to the toast currently shown; a
		// stale timer from a superseded toast (two toasts within the dismiss
		// window) must not hide the newer one early.
		if msg.Seq == m.toast.Seq() {
			m.toast = m.toast.Hide()
		}
		return m, modeCmd
	case tea.KeyMsg:
		// Single choke point: any key press clears the pending-dialog guard.
		// The guard is set when an async catalog-load Cmd is dispatched and must
		// be cleared before the key is processed so that the catalog-loaded
		// handler (arriving later) sees the guard is gone and drops its result.
		// We capture the guard state before clearing so ESC can use it to
		// decide whether to cancel the pending open instead of popping the mode.
		hadPendingDialog := m.pendingDialog.active
		m.pendingDialog = pendingDialogGuard{}

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
				m.pendingDialog = pendingDialogGuard{active: true, kind: mutationStatus}
				return m, batchCmds(modeCmd, loadMutationCatalogsCmd(m.services, mutationStatus, issue))
			}
			if m.detail.ConsumeOpenPriorityDialogIntent() {
				issue := m.detail.Detail.Summary
				if strings.TrimSpace(issue.ID) == "" {
					if selection := m.currentSelection(); selection != nil {
						issue = selection.Issue
					}
				}
				if strings.TrimSpace(issue.ID) == "" {
					return m, batchCmds(modeCmd, m.showToast("No selected issue to update priority", toaster.StyleWarn))
				}
				dialog := buildMutationDialog(mutationPriority, issue, nil, nil, nil)
				m.actionState = dialog
				m.actionModal = mutationModal(dialog, m.keys)
				m.actionModal.SetSize(m.width, m.height)
				m.showActionModal = true
				return m, batchCmds(modeCmd, m.actionModal.Init())
			}
			if intent != nil {
				issueID := strings.TrimSpace(intent.IssueID)
				if issueID == "" {
					return m, modeCmd
				}
				m.active = mode.Detail
				// Drilling into a related issue is a full navigation, not a peek:
				// the target becomes the new detail selection so ALL three panes —
				// including the Dependencies rail — reflect the target once loaded.
				// This is what lets you open a child from an epic and then jump
				// back via the child's own Parent row. Seeding an optimistic
				// placeholder from the row's known ref renders the header + core
				// metadata immediately, while the description and Dependencies pane
				// show their skeleton until the single taskmgr show returns.
				// ApplyLoadedDetail resets scroll offsets when the issue changes.
				//
				// Focus retention: set Loading and the drill-focus counter before the
				// placeholder ApplyLoadedDetail call so that clearBrowserPanel does not
				// flip focus away from the Dependencies pane during the in-flight window.
				// The real detailLoadedMsg will apply the correct focus decision from
				// actual rail content via the counter mechanism in ApplyLoadedDetail.
				m.detail.SelectionID = issueID
				m.detail.TargetID = issueID
				m.detail.Loading = true
				m.detail.Error = ""
				m.detail.SetDrillFromDepsFocus()
				m.detail.ApplyLoadedDetail(issueID, detail.PlaceholderDetail(issueID, intent.Ref, true))
				return m, batchCmds(modeCmd, loadDetailCmd(m.services, issueID))
			}
			if consumed {
				return m, modeCmd
			}
		}

		if m.active == mode.Search {
			if m.search.ConsumeOpenStatusDialogIntent() {
				selection := m.selectedByMode[mode.Search]
				if selection == nil || strings.TrimSpace(selection.Issue.ID) == "" {
					return m, batchCmds(modeCmd, m.showToast("No selected issue to update status", toaster.StyleWarn))
				}
				m.pendingDialog = pendingDialogGuard{active: true, kind: mutationStatus}
				return m, batchCmds(modeCmd, loadMutationCatalogsCmd(m.services, mutationStatus, selection.Issue))
			}
			if m.search.ConsumeOpenPriorityDialogIntent() {
				selection := m.selectedByMode[mode.Search]
				if selection == nil || strings.TrimSpace(selection.Issue.ID) == "" {
					return m, batchCmds(modeCmd, m.showToast("No selected issue to update priority", toaster.StyleWarn))
				}
				dialog := buildMutationDialog(mutationPriority, selection.Issue, nil, nil, nil)
				m.actionState = dialog
				m.actionModal = mutationModal(dialog, m.keys)
				m.actionModal.SetSize(m.width, m.height)
				m.showActionModal = true
				return m, batchCmds(modeCmd, m.actionModal.Init())
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
			return m, batchCmds(modeCmd, m.lazySearchInitCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
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
			return m, batchCmds(modeCmd, m.lazySearchInitCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
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
			m.applyModeCycle(nextMode(m.active, m.lastBrowse))
			return m, batchCmds(modeCmd, m.lazySearchInitCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
		case m.keys.Match(config.ShellContext, config.ShellActionModeCyclePrev, msg):
			m.applyModeCycle(prevMode(m.active, m.lastBrowse))
			return m, batchCmds(modeCmd, m.lazySearchInitCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
		case m.keys.Match(config.ShellContext, config.ShellActionEscape, msg):
			// If a dialog-open was in flight when ESC arrived, the guard has
			// already been cleared at the top of this branch. Consume ESC as
			// "cancel the pending open" and keep the current mode — do NOT pop
			// Detail → Board (or Search → Board) while the load is in progress.
			if hadPendingDialog {
				return m, modeCmd
			}
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
			return m, batchCmds(modeCmd, prepareEditCmd(m.services, issueID))
		case m.keys.Match(config.ShellContext, config.ShellActionCreateIssue, msg):
			m.pendingDialog = pendingDialogGuard{active: true, kind: mutationCreate}
			return m, batchCmds(modeCmd, loadMutationCatalogsCmd(m.services, mutationCreate, domain.IssueSummary{}))
		case m.keys.Match(config.ShellContext, config.ShellActionUpdateIssue, msg):
			selection := m.currentSelection()
			if selection == nil || selection.Issue.ID == "" {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to update", toaster.StyleWarn))
			}
			m.pendingDialog = pendingDialogGuard{active: true, kind: mutationUpdate}
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

func (m *Model) showToast(message string, style toaster.Style) tea.Cmd {
	m.toast = m.toast.Show(message, style)
	// Tag the dismiss timer with this toast's identity so a stale timer from an
	// earlier toast cannot dismiss the one now on screen (see DismissMsg handler).
	return m.scheduleToastDismiss(3*time.Second, m.toast.Seq())
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
	// Use IsLoading() so both browse modes are queried uniformly (board also
	// exposes IsLoading()); SessionState() remains for the richer search bundle.
	return m.search.IsLoading()
}

func (m Model) searchResultCount() int {
	if m.search == nil {
		return 0
	}
	session := m.search.SessionState()
	if session.Page.Metadata.ReturnedCount > 0 {
		return session.Page.Metadata.ReturnedCount
	}
	return len(session.Page.Results)
}
