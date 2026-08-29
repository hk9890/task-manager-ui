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
	docsmode "github.com/hk9890/task-manager-ui/internal/mode/docs"
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

	// ctx is the application lifecycle context, cancelled when the process is
	// shutting down. Shell-issued repository reads use it so quitting abandons
	// them. Never nil — NewModelWithOptions defaults it to context.Background().
	ctx context.Context

	// fatalErrTitle and fatalErrBody are set when a startup health check detects
	// that the app cannot run. When fatalErrTitle is non-empty, View() renders
	// the fatal error screen and Update() only handles quit keys and window resize.
	fatalErrTitle string
	fatalErrBody  string

	active     mode.ID
	lastBrowse mode.ID

	selectedByMode map[mode.ID]*mode.Selection

	// drillSelection is the issue Detail drilled into from its Dependencies
	// rail. While it is set and Detail is active it IS the shell's selection:
	// the browse tab's row is not what the operator is looking at, so acting on
	// it would edit, close, comment on or launch against the wrong issue. It is
	// cleared whenever the shell leaves Detail or a browse tab moves its own
	// selection.
	drillSelection *mode.Selection

	board  *boardmode.Model
	docs   *docsmode.Model
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

	// searchInitDone and docsInitDone track whether the first lazy init has been
	// fired for those modes. Neither is pre-loaded at startup; the first mode
	// switch triggers Init() and sets the flag so subsequent entries do not
	// reload.
	searchInitDone bool
	docsInitDone   bool

	refreshStateBySurface map[mode.ID]surfaceRefreshState

	spinnerFrame int

	// spinnerTicking is true while a loading.TickMsg is scheduled. The tick used
	// to be armed at startup and re-armed unconditionally, so View() ran ten
	// times a second for the life of the process — every frame re-rendering the
	// issue markdown from scratch — and every frame after the first was
	// byte-identical and thrown away by Bubble Tea's diff. It is now armed only
	// while something is loading, and there is at most one chain at a time.
	spinnerTicking bool

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

	ctx := runtime.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	return Model{
		services:       services,
		keys:           keys,
		ctx:            ctx,
		active:         mode.Board,
		lastBrowse:     mode.Board,
		selectedByMode: make(map[mode.ID]*mode.Selection),
		board:          boardmode.NewModel(ctx, services.Repo, logging.WithComponent(services.Logger, "board"), keys),
		docs:           docsmode.NewModel(ctx, services.Repo, logging.WithComponent(services.Logger, "docs"), keys),
		search:         searchmode.NewModel(ctx, services.Repo, logging.WithComponent(services.Logger, "search"), keys),
		detail:         detail.Model{Keys: keys},
		toast:          toaster.New(),
		help:           help,
		width:          defaultViewportWidth,
		height:         defaultViewportHeight,
		refreshStateBySurface: map[mode.ID]surfaceRefreshState{
			mode.Board:  {lastRefresh: now},
			mode.Docs:   {lastRefresh: now},
			mode.Search: {lastRefresh: now},
			mode.Detail: {},
		},
		runtime:              runtime,
		scheduleRefreshTick:  defaultScheduleRefreshTick,
		scheduleToastDismiss: defaultScheduleToastDismiss,
		scheduleSpinnerTick:  defaultScheduleSpinnerTick,
	}, nil
}

// logger returns the injected runtime logger, which carries the session_id,
// project_root and build_version provenance and writes to the persistent JSON
// Lines log. Never write to slog.Default() from a runtime path: that is the
// stock stderr handler, and startInteractive suppresses stderr precisely
// because a stray write corrupts the alt-screen frame Bubble Tea owns.
func (m Model) logger() *slog.Logger {
	if m.services.Logger != nil {
		return m.services.Logger
	}
	return slog.Default()
}

// Init fires the startup health check and the spinner tick. Board loads are
// deferred until the health check passes (see startupHealthCheckMsg handler in
// Update). Search is deferred further until the user first switches to search
// mode; see lazySearchInitCmd.
func (m Model) Init() tea.Cmd {
	m.applyWorkspaceSizeToBrowseModes()
	healthCheckCmd := func() tea.Msg {
		err := m.services.Repo.HealthCheck(m.ctx)
		return startupHealthCheckMsg{err: err}
	}
	sweepCmd := m.services.SweepStaleTempFiles()
	// The spinner tick is not armed here. Update arms it whenever something
	// starts loading and stops re-arming when nothing is, so an idle app draws
	// no frames; Init cannot set the armed flag anyway (value receiver).
	if m.runtime.DisableAutoRefresh {
		return tea.Batch(healthCheckCmd, sweepCmd)
	}
	return tea.Batch(healthCheckCmd, sweepCmd, m.scheduleRefreshTick())
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

// lazyDocsInitCmd fires m.docs.Init() exactly once — the first time the active
// mode is Docs. It mirrors lazySearchInitCmd: docs are not pre-loaded at
// startup, and re-entering the tab afterwards goes through the normal
// auto-refresh path.
func (m *Model) lazyDocsInitCmd() tea.Cmd {
	if m.active != mode.Docs {
		return nil
	}
	if m.docsInitDone {
		return nil
	}
	m.docsInitDone = true
	m.markSurfaceRefreshed(mode.Docs)
	return m.docs.Init()
}

// Update handles root-level shell messages.
//
// It wraps update so the spinner tick is armed at one choke point: whatever a
// handler did, the tick runs if and only if something is loading afterwards.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)

	model, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	model.syncSearchPreviewDetailState()
	return model, batchCmds(cmd, model.ensureSpinnerTickCmd())
}

// ensureSpinnerTickCmd arms the spinner tick when work is in flight and no tick
// is already scheduled. Nothing waits silently (docs/DESIGN-GUIDE.md), and
// nothing spins while nothing waits.
func (m *Model) ensureSpinnerTickCmd() tea.Cmd {
	if m.spinnerTicking || len(m.loadingStates()) == 0 {
		return nil
	}
	m.spinnerTicking = true
	return m.scheduleSpinnerTick()
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle startup health check result before any other processing.
	if check, ok := msg.(startupHealthCheckMsg); ok {
		if check.err != nil {
			// Log every failure code, not only the one with a fatal screen. An
			// unreadable store directory or a corrupt store reached neither the
			// log docs/MONITORING.md points a diagnosing agent at nor a toast,
			// so the operator saw only whatever the following Dashboard call
			// happened to render.
			m.logger().Error("task-manager health check failed", "error", check.err)

			var gwErr domain.RepositoryError
			if errors.As(check.err, &gwErr) && gwErr.Code == domain.ErrorCodeNoDatabaseFound {
				m.fatalErrTitle = "no task-manager store here"
				m.fatalErrBody = "No task-manager store resolved for this directory: no local .tasks store, and no central store registered for it.\n\nRun 'taskmgr init' to create one, use --cwd to point at a directory that has one, or use --store-name to open a central store by name ('taskmgr store list' shows them)."
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

	if model, cmd, handled := m.handleOverlayMessage(msg, modeCmd); handled {
		return model, cmd
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
		// This tick has fired; Update re-arms it only while work is in flight.
		m.spinnerTicking = false
		return m, modeCmd
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
		return m, batchCmds(modeCmd, m.openMutationModal(dialog))
	case mutationResultMsg:
		return m.handleMutationResult(modeCmd, msg)
	case mode.SelectionChangedMsg:
		if !mode.IsBrowse(msg.Mode) {
			return m, modeCmd
		}
		m.selectedByMode[msg.Mode] = msg.Selection
		if msg.Mode == m.active {
			m.lastBrowse = msg.Mode
		}
		// A browse tab moving its own selection supersedes any drill-in.
		m.clearDrillSelection()
		return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
	case mode.ActionRequestMsg:
		switch msg.Action {
		case mode.ActionOpenStatusDialog:
			issue, ok := m.dialogTargetIssue(msg.Mode)
			if !ok {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to update status", toaster.StyleWarn))
			}
			m.pendingDialog = pendingDialogGuard{active: true, kind: mutationStatus}
			return m, batchCmds(modeCmd, loadMutationCatalogsCmd(m.ctx, m.services, mutationStatus, issue))
		case mode.ActionOpenPriorityDialog:
			issue, ok := m.dialogTargetIssue(msg.Mode)
			if !ok {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to update priority", toaster.StyleWarn))
			}
			return m, batchCmds(modeCmd, m.openMutationModal(buildMutationDialog(mutationPriority, issue, nil, nil, nil)))
		}
		if msg.Action != mode.ActionOpenDetail {
			return m, modeCmd
		}
		if mode.IsBrowse(msg.Mode) {
			m.lastBrowse = msg.Mode
		}
		m.clearDrillSelection()
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
		return m.handleShellKey(msg, modeCmd)
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

func (m Model) docsIsLoading() bool {
	if m.docs == nil {
		return false
	}
	return m.docs.IsLoading()
}

func (m Model) searchIsLoading() bool {
	if m.search == nil {
		return false
	}
	// Use IsLoading() so both browse modes are queried uniformly (board also
	// exposes IsLoading()); SessionState() remains for the richer search bundle.
	return m.search.IsLoading()
}

// handleShellKey handles one key press for the shell: the pending-dialog
// choke point, the mode-local capture and intent checks, and the shell
// keybinding switch. It is split out of update() so that message routing and
// key handling are readable separately; update() was 483 lines with a
// fifteen-case message switch and a nineteen-case key switch in one body.
//
// It takes the same modeCmd update() would have batched and returns the same
// (tea.Model, tea.Cmd) pair, so the branch is a move, not a rewrite.
func (m Model) handleShellKey(msg tea.KeyMsg, modeCmd tea.Cmd) (tea.Model, tea.Cmd) {
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
		consumed, intent, actionCmd := m.detail.HandleKey(msg, m.detailViewportWidth(), m.detailViewportHeight())
		if actionCmd != nil {
			return m, batchCmds(modeCmd, actionCmd)
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
			// The drilled issue becomes the shell's selection, so e/x/u/a
			// and the launchers act on what is on screen rather than on the
			// browse tab's row.
			m.drillSelection = &mode.Selection{Issue: domain.IssueSummary{
				ID:       issueID,
				Title:    intent.Ref.Title,
				Status:   intent.Ref.Status,
				Type:     intent.Ref.Type,
				Priority: intent.Ref.Priority,
			}}
			m.detail.Loading = true
			m.detail.Error = ""
			m.detail.SetDrillFromDepsFocus()
			m.detail.ApplyLoadedDetail(issueID, detail.PlaceholderDetail(issueID, intent.Ref, true))
			return m, batchCmds(modeCmd, loadDetailCmd(m.ctx, m.services, issueID))
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
		m.enterBrowseMode(mode.Board)
		return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionModeDocs, msg):
		m.enterBrowseMode(mode.Docs)
		return m, batchCmds(modeCmd, m.lazyDocsInitCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionModeSearch, msg):
		m.enterBrowseMode(mode.Search)
		return m, batchCmds(modeCmd, m.lazySearchInitCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionToggleSearch, msg):
		if m.active == mode.Detail {
			m.enterBrowseMode(mode.Board)
			return m, modeCmd
		}
		if m.active == mode.Search {
			m.enterBrowseMode(mode.Board)
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
		}
		m.enterBrowseMode(mode.Search)
		return m, batchCmds(modeCmd, m.lazySearchInitCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionModeDetail, msg):
		if mode.IsBrowse(m.active) {
			m.lastBrowse = m.active
		}
		if m.currentSelection() == nil {
			return m, batchCmds(modeCmd, m.showToast("No selected issue to open in detail mode", toaster.StyleWarn))
		}
		// Opening Detail from a browse tab starts from that tab's row, not
		// from wherever an earlier drill-in ended up.
		m.clearDrillSelection()
		m.active = mode.Detail
		return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionModeCycleNext, msg):
		m.applyModeCycle(nextMode(m.active, m.lastBrowse))
		return m, batchCmds(modeCmd, m.lazySearchInitCmd(), m.lazyDocsInitCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionModeCyclePrev, msg):
		m.applyModeCycle(prevMode(m.active, m.lastBrowse))
		return m, batchCmds(modeCmd, m.lazySearchInitCmd(), m.lazyDocsInitCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionEscape, msg):
		// If a dialog-open was in flight when ESC arrived, the guard has
		// already been cleared at the top of this branch. Consume ESC as
		// "cancel the pending open" and keep the current mode — do NOT pop
		// Detail → Board (or Search → Board) while the load is in progress.
		if hadPendingDialog {
			return m, modeCmd
		}
		if m.active == mode.Detail {
			m.enterBrowseMode(m.lastBrowse)
			return m, modeCmd
		}
		// Board is the home tab: Escape from any other browse tab returns
		// there before it starts dismissing toasts.
		if mode.IsBrowse(m.active) && m.active != mode.Board {
			m.enterBrowseMode(mode.Board)
			return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
		}
		m.toast = m.toast.Hide()
		return m, modeCmd
	case m.keys.Match(config.ShellContext, config.ShellActionReloadDetail, msg):
		if m.active != mode.Detail {
			return m, modeCmd
		}
		return m, batchCmds(modeCmd, m.reloadDetailCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionEditIssue, msg):
		issueID, ok := m.selectedIssueID()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue to edit", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, prepareEditCmd(m.ctx, m.services, issueID))
	case m.keys.Match(config.ShellContext, config.ShellActionCreateIssue, msg):
		m.pendingDialog = pendingDialogGuard{active: true, kind: mutationCreate}
		return m, batchCmds(modeCmd, loadMutationCatalogsCmd(m.ctx, m.services, mutationCreate, domain.IssueSummary{}))
	case m.keys.Match(config.ShellContext, config.ShellActionUpdateIssue, msg):
		selection := m.currentSelection()
		if selection == nil || selection.Issue.ID == "" {
			return m, batchCmds(modeCmd, m.showToast("No selected issue to update", toaster.StyleWarn))
		}
		m.pendingDialog = pendingDialogGuard{active: true, kind: mutationUpdate}
		return m, batchCmds(modeCmd, loadMutationCatalogsCmd(m.ctx, m.services, mutationUpdate, selection.Issue))
	case m.keys.Match(config.ShellContext, config.ShellActionCloseIssue, msg):
		selection := m.currentSelection()
		if selection == nil || selection.Issue.ID == "" {
			return m, batchCmds(modeCmd, m.showToast("No selected issue to close", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, m.openMutationModal(mutationDialogState{kind: mutationClose, issue: selection.Issue}))
	case m.keys.Match(config.ShellContext, config.ShellActionCommentIssue, msg):
		selection := m.currentSelection()
		if selection == nil || selection.Issue.ID == "" {
			return m, batchCmds(modeCmd, m.showToast("No selected issue to comment on", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, m.openMutationModal(mutationDialogState{kind: mutationComment, issue: selection.Issue}))
	case m.keys.Match(config.ShellContext, config.ShellActionLaunchNvim, msg):
		if m.active != mode.Detail {
			return m, modeCmd
		}
		issueContext, ok := m.selectedIssueContext()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue for launcher", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, launchActionCmd(m.ctx, m.services, "nvim", issueContext))
	case m.keys.Match(config.ShellContext, config.ShellActionLaunchOpencode, msg):
		if m.active != mode.Detail {
			return m, modeCmd
		}
		issueContext, ok := m.selectedIssueContext()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue for launcher", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, launchActionCmd(m.ctx, m.services, "opencode", issueContext))
	case m.keys.Match(config.ShellContext, config.ShellActionLaunchShell, msg):
		if m.active != mode.Detail {
			return m, modeCmd
		}
		issueContext, ok := m.selectedIssueContext()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue for launcher", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, launchActionCmd(m.ctx, m.services, "shell-command", issueContext))
	}

	return m, modeCmd
}

// handleOverlayMessage routes msg to whichever overlay is open. handled is
// false when none is, in which case the caller falls through to the message
// switch. An open overlay consumes the message: that is why this runs before
// routing and not inside it.
func (m Model) handleOverlayMessage(msg tea.Msg, modeCmd tea.Cmd) (tea.Model, tea.Cmd, bool) {
	if m.showActionModal {
		if size, ok := msg.(tea.WindowSizeMsg); ok {
			m.sizeKnown = true
			m.width = size.Width
			m.height = size.Height
			m.actionModal.SetSize(m.width, m.height)
			return m, modeCmd, true
		}

		if _, ok := msg.(modal.CancelMsg); ok {
			m.showActionModal = false
			return m, modeCmd, true
		}

		if submit, ok := msg.(modal.SubmitMsg); ok {
			m.showActionModal = false
			return m, batchCmds(modeCmd, submitMutationCmd(m.services, m.actionState, submit.Values)), true
		}

		nextModal, cmd := m.actionModal.Update(msg)
		m.actionModal = nextModal
		return m, batchCmds(modeCmd, cmd), true
	}

	if m.showHelp {
		// Close through the same action that opens it. Matching a literal "?"
		// made the toggle one-way for anyone who rebound toggle_help — the
		// example config in docs/CONFIGURATION.md binds it to F1 — leaving
		// Escape as the only way out.
		if k, ok := msg.(tea.KeyMsg); ok && m.keys.Match(config.ShellContext, config.ShellActionHelp, k) {
			m.showHelp = false
			return m, modeCmd, true
		}

		if _, ok := msg.(modal.CancelMsg); ok {
			m.showHelp = false
			return m, modeCmd, true
		}
		if _, ok := msg.(modal.SubmitMsg); ok {
			m.showHelp = false
			return m, modeCmd, true
		}

		nextHelp, cmd := m.help.Update(msg)
		m.help = nextHelp

		if size, ok := msg.(tea.WindowSizeMsg); ok {
			m.sizeKnown = true
			m.width = size.Width
			m.height = size.Height
			m.help.SetSize(m.width, m.height)
		}

		return m, batchCmds(modeCmd, cmd), true
	}

	return m, nil, false
}
