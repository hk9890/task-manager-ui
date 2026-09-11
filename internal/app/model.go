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
	storepickermode "github.com/hk9890/task-manager-ui/internal/mode/storepicker"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
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

	// appCtx is the application lifecycle context, cancelled when the process
	// is shutting down. Never nil — NewModelWithOptions defaults it to
	// context.Background().
	appCtx context.Context

	// ctx is the active store's context, derived from appCtx. Every repository
	// read issued by the shell or a browse mode uses it, so quitting abandons
	// them and so does switching stores: bindStore cancels it and derives a new
	// one.
	ctx         context.Context
	cancelStore context.CancelFunc

	// storeOpen is false only when the app started without a store to open. The
	// operator is held on the picker until one is opened: there is no board to
	// return to and no tab worth switching to.
	storeOpen bool

	// storeEpoch identifies the active store. Work issued against a store
	// carries it (see scoped), and a result that arrives after the store it was
	// issued against was switched away is dropped instead of rendered:
	// cancelling ctx stops a read, but a Bubble Tea command that has already
	// produced its message delivers it regardless.
	storeEpoch int

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

	// storePicker is the full-screen store list. pickerReturn is the mode the
	// operator opened it from, so Escape puts them back exactly where they
	// were rather than on the home tab.
	storePicker  *storepickermode.Model
	pickerReturn mode.ID

	detail detail.Model

	toast toaster.Model

	help     modal.Model
	showHelp bool

	actionModal     modal.Model
	showActionModal bool
	actionState     mutationDialogState

	focusKnown      bool
	terminalFocused bool

	// initDone records which browse tabs have had their first lazy Init()
	// fired. Docs and Search are not pre-loaded at startup; the first switch to
	// one triggers Init() and marks it here so later entries do not reload.
	initDone map[mode.ID]bool

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

	m := Model{
		keys:   keys,
		appCtx: ctx,
		// The picker lists stores rather than reading one, so it lives on the
		// application's context and survives every store switch.
		storePicker: storepickermode.NewModel(ctx, services.StoreCatalog,
			logging.WithComponent(services.Logger, "storepicker"), keys),
		toast:                toaster.New(),
		help:                 help,
		width:                defaultViewportWidth,
		height:               defaultViewportHeight,
		runtime:              runtime,
		scheduleRefreshTick:  defaultScheduleRefreshTick,
		scheduleToastDismiss: defaultScheduleToastDismiss,
		scheduleSpinnerTick:  defaultScheduleSpinnerTick,
	}
	m.bindStore(services)
	if runtime.UnresolvedStore != "" {
		m.storeOpen = false
		m.active = mode.StorePicker
	}
	return m, nil
}

// bindStore makes services the active store. It cancels the previous store's
// context, derives a new one, and rebuilds every surface that reads a store —
// board, docs, search, detail — with the state the shell keeps about them.
//
// It is the one construction path for those surfaces, at startup and on every
// switch, so a switch cannot keep a field that a per-mode Reset() forgot to
// clear.
func (m *Model) bindStore(services Services) {
	if m.cancelStore != nil {
		m.cancelStore()
	}
	m.ctx, m.cancelStore = context.WithCancel(m.appCtx)
	m.storeEpoch++
	m.services = services
	m.storeOpen = true

	m.board = boardmode.NewModel(m.ctx, services.Repo, logging.WithComponent(services.Logger, "board"), m.keys)
	m.docs = docsmode.NewModel(m.ctx, services.Repo, logging.WithComponent(services.Logger, "docs"), m.keys)
	m.search = searchmode.NewModel(m.ctx, services.Repo, logging.WithComponent(services.Logger, "search"), m.keys)
	m.detail = detail.Model{Keys: m.keys}
	m.applyWorkspaceSizeToBrowseModes()

	m.active = mode.Board
	m.lastBrowse = mode.Board
	m.pickerReturn = mode.Board
	m.selectedByMode = make(map[mode.ID]*mode.Selection)
	m.drillSelection = nil
	m.pendingDialog = pendingDialogGuard{}
	m.showActionModal = false
	m.fatalErrTitle, m.fatalErrBody = "", ""
	// Board is initialised eagerly — by Init at startup, by switchStore after a
	// switch — so it starts marked done and a later switch back to it does not
	// re-fire a load.
	m.initDone = map[mode.ID]bool{mode.Board: true}

	now := modelNow()
	m.refreshStateBySurface = map[mode.ID]surfaceRefreshState{
		mode.Board:  {lastRefresh: now},
		mode.Docs:   {lastRefresh: now},
		mode.Search: {lastRefresh: now},
		mode.Detail: {},
	}

	m.storePicker.SetActiveStorePath(services.ActiveStorePath)
}

// switchStore makes an opened store the active one and loads its board.
func (m *Model) switchStore(opened storecatalog.Opened) tea.Cmd {
	services, err := m.services.ForStore(opened)
	if err != nil {
		m.logger().Error("failed to switch task-manager store", "store", opened.Name, "error", err.Error())
		return m.showToast(fmt.Sprintf("Failed to open store %s: %v", opened.Name, err), toaster.StyleError)
	}
	m.bindStore(services)
	return batchCmds(
		m.scoped(m.board.Init()),
		m.showToast(fmt.Sprintf("Opened store %s", opened.Name), toaster.StyleSuccess),
	)
}

// scopedMsg is a message produced by work issued against one store, tagged
// with that store's epoch.
type scopedMsg struct {
	epoch int
	msg   tea.Msg
}

// scoped tags every message cmd produces with the active store's epoch, so
// update drops it if the store has been switched by the time it arrives.
//
// Only store-bound work goes through it: browse-mode commands and the shell's
// repository reads. Timers stay unscoped — a refresh or spinner tick re-arms
// only from its own handler, so dropping one would stop the chain for good.
func (m Model) scoped(cmd tea.Cmd) tea.Cmd {
	return scopeCmd(m.storeEpoch, cmd)
}

func scopeCmd(epoch int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case nil:
			return nil
		case tea.BatchMsg:
			// A batch is a list of commands the runtime runs, not a result, so
			// the tag goes on each of them rather than on the list.
			out := make(tea.BatchMsg, 0, len(msg))
			for _, inner := range msg {
				out = append(out, scopeCmd(epoch, inner))
			}
			return out
		default:
			return scopedMsg{epoch: epoch, msg: msg}
		}
	}
}

// loadDetail loads one issue's detail from the active store.
func (m Model) loadDetail(issueID string) tea.Cmd {
	return m.scoped(loadDetailCmd(m.ctx, m.services, issueID))
}

// openStoreCmd opens a central store by registry name. It is not scoped: it is
// the work that decides which store is active, not work done inside one.
func openStoreCmd(ctx context.Context, catalog storecatalog.Catalog, name string) tea.Cmd {
	return func() tea.Msg {
		if catalog == nil {
			return storeOpenedMsg{name: name, err: errors.New("no store catalog is configured for this session")}
		}
		opened, err := catalog.Open(ctx, name)
		return storeOpenedMsg{name: name, opened: opened, err: err}
	}
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
// mode; see lazyInitActiveTabCmd.
func (m Model) Init() tea.Cmd {
	if !m.storeOpen {
		// Nothing to health-check and no board to load: the app opens on the
		// picker and says why it is there.
		reason := m.runtime.UnresolvedStore
		return tea.Batch(
			m.storePicker.Init(),
			func() tea.Msg { return unresolvedStoreMsg{reason: reason} },
			m.services.SweepStaleTempFiles(),
		)
	}

	m.applyWorkspaceSizeToBrowseModes()
	healthCheckCmd := m.scoped(func() tea.Msg {
		err := m.services.Repo.HealthCheck(m.ctx)
		return startupHealthCheckMsg{err: err}
	})
	sweepCmd := m.services.SweepStaleTempFiles()
	// The spinner tick is not armed here. Update arms it whenever something
	// starts loading and stops re-arming when nothing is, so an idle app draws
	// no frames; Init cannot set the armed flag anyway (value receiver).
	if m.runtime.DisableAutoRefresh {
		return tea.Batch(healthCheckCmd, sweepCmd)
	}
	return tea.Batch(healthCheckCmd, sweepCmd, m.scheduleRefreshTick())
}

// lazyInitActiveTabCmd fires the active browse tab's Init() exactly once — the
// first time that tab becomes active. Neither Docs nor Search is pre-loaded at
// startup, so the shell opens on Board with one repository read rather than
// three; the first switch pays for that tab and later switches reuse the
// already-loaded state until an explicit reload or the auto-refresh interval.
//
// Board is initialised eagerly by Init(), so it is absent from initDone and
// never re-fires here.
func (m *Model) lazyInitActiveTabCmd() tea.Cmd {
	if !mode.IsBrowse(m.active) || m.initDone[m.active] {
		return nil
	}
	tab := m.browseController(m.active)
	if tab == nil {
		return nil
	}
	if m.initDone == nil {
		m.initDone = make(map[mode.ID]bool, len(mode.BrowseModes))
	}
	m.initDone[m.active] = true
	m.markSurfaceRefreshed(m.active)
	return m.scoped(tab.Init())
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
// is already scheduled: long work renders the spinner (docs/DESIGN-GUIDE.md,
// Loading feedback), and nothing spins while nothing waits.
func (m *Model) ensureSpinnerTickCmd() tea.Cmd {
	if m.spinnerTicking || len(m.loadingStates()) == 0 {
		return nil
	}
	m.spinnerTicking = true
	return m.scheduleSpinnerTick()
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if scoped, ok := msg.(scopedMsg); ok {
		if scoped.epoch != m.storeEpoch {
			// Issued against a store that is no longer active. Rendering it
			// would put another project's issues on this store's surfaces.
			return m, nil
		}
		msg = scoped.msg
	}

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
		return m, m.scoped(m.board.Init())
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
		// Both overlays are sized here, open or not: handleOverlayMessage
		// passes a resize through rather than consuming it.
		m.help.SetSize(m.width, m.height)
		m.actionModal.SetSize(m.width, m.height)
		// The picker renders instead of the shell, so it takes the whole
		// terminal rather than the workspace the browse tabs share.
		m.storePicker.SetSize(m.width, m.height)
		m.detail.ClampScroll(m.detailViewportWidth(), m.detailViewportHeight())
		return m, modeCmd
	case storepickermode.StoresLoadedMsg:
		return m, batchCmds(modeCmd, m.storePicker.Update(msg))
	case unresolvedStoreMsg:
		return m, batchCmds(modeCmd, m.showToast(msg.reason, toaster.StyleWarn))
	case storepickermode.OpenMsg:
		entry := msg.Entry
		if !entry.Health.Usable() {
			return m, batchCmds(modeCmd, m.showToast(
				fmt.Sprintf("Store %s is %s and cannot be opened", entry.Name, entry.Health), toaster.StyleWarn))
		}
		// Already the active store: leave the picker and touch nothing, rather
		// than rebuilding every surface to show what is already there.
		if entry.StorePath == m.services.ActiveStorePath {
			m.active = m.pickerReturn
			return m, modeCmd
		}
		return m, batchCmds(modeCmd, openStoreCmd(m.appCtx, m.services.StoreCatalog, entry.Name))
	case storeOpenedMsg:
		if msg.err != nil {
			m.logger().Error("failed to open task-manager store", "store", msg.name, "error", msg.err.Error())
			return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Failed to open store %s: %v", msg.name, msg.err), toaster.StyleError))
		}
		return m, batchCmds(modeCmd, m.switchStore(msg.opened))
	case detailLoadedMsg:
		if msg.issueID != m.detail.TargetID() {
			return m, modeCmd
		}

		m.detail.FinishLoad(msg.err)
		m.markSurfaceRefreshed(mode.Detail)
		if msg.err != nil {
			return m, batchCmds(modeCmd, m.showToast("Failed to load selected issue details", toaster.StyleError))
		}

		if strings.TrimSpace(msg.issueID) == strings.TrimSpace(m.detail.SelectionID()) {
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
		// A browse tab moving its own selection supersedes any drill-in — but
		// only the tab the operator is actually on. A background load
		// completing in another tab used to clear the drill selection too,
		// silently retargeting every shell mutation at that tab's row while
		// Detail still showed the drilled-in issue.
		if msg.Mode == m.active {
			m.clearDrillSelection()
		}
		// The picker is not a browse tab and shows no detail, so a selection
		// landing under it must not start a detail load: currentSelection() would
		// answer from lastBrowse, retargeting the Detail surface the operator
		// returns to and raising its failure toast over the store list.
		if m.active == mode.StorePicker {
			return m, modeCmd
		}
		return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd())
	case mode.ActionRequestMsg:
		// The request travels as a Cmd, so the operator can leave the
		// requesting mode before it arrives. Answering it then resolves the
		// target from a surface that is no longer on screen — and an ESC that
		// left the mode is what cancels the request.
		if msg.Mode != m.active {
			return m, modeCmd
		}
		switch msg.Action {
		case mode.ActionOpenStatusDialog:
			issue, ok := m.dialogTargetIssue(msg.Mode)
			if !ok {
				return m, batchCmds(modeCmd, m.showToast("No selected issue to update status", toaster.StyleWarn))
			}
			m.pendingDialog = pendingDialogGuard{active: true, kind: mutationStatus}
			return m, batchCmds(modeCmd, m.scoped(loadMutationCatalogsCmd(m.ctx, m.services, mutationStatus, issue)))
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

// issueScopedShellActions are the shell actions that act on the currently
// selected issue. A surface that has no issue selection swallows them rather
// than letting currentSelection() answer from a tab that is not on screen.
var issueScopedShellActions = []string{
	config.ShellActionEditIssue,
	config.ShellActionCreateIssue,
	config.ShellActionUpdateIssue,
	config.ShellActionCloseIssue,
	config.ShellActionCommentIssue,
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

	// The picker gets first refusal on a key while it is up, then the shell
	// switch below still sees Escape, quit, help and the tab keys.
	if m.active == mode.StorePicker {
		if consumed, pickerCmd := m.storePicker.HandleKey(msg); consumed {
			return m, batchCmds(modeCmd, pickerCmd)
		}
		// Every action below acts on "the selected issue", which the picker does
		// not have: currentSelection() would answer with a browse row that is not
		// on screen, and the picker's help line names none of them. Editing,
		// closing or commenting on an issue the operator cannot see is the worst
		// available outcome, so the picker swallows them.
		for _, action := range issueScopedShellActions {
			if m.keys.Match(config.ShellContext, action, msg) {
				return m, modeCmd
			}
		}

		// With no store open there is nothing below the picker: no board to
		// return to and no tab to switch to. Escape leaves the app, quit and
		// help keep working, and every other key is inert.
		if !m.storeOpen {
			if m.keys.Match(config.ShellContext, config.ShellActionEscape, msg) {
				return m, batchCmds(modeCmd, tea.Quit)
			}
			if !m.keys.Match(config.ShellContext, config.ShellActionQuit, msg) &&
				!m.keys.Match(config.ShellContext, config.ShellActionHelp, msg) {
				return m, modeCmd
			}
		}
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
			m.drillSelection = &mode.Selection{Issue: domain.IssueSummary{
				ID:       issueID,
				Title:    intent.Ref.Title,
				Status:   intent.Ref.Status,
				Type:     intent.Ref.Type,
				Priority: intent.Ref.Priority,
			}}
			m.detail.BeginLoad(issueID, detail.BeginLoadOptions{Ref: &intent.Ref, Drill: true})
			return m, batchCmds(modeCmd, m.loadDetail(issueID))
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
	case m.keys.Match(config.ShellContext, config.ShellActionStorePicker, msg):
		if m.active == mode.StorePicker {
			return m, modeCmd
		}
		m.pickerReturn = m.active
		m.active = mode.StorePicker
		m.storePicker.SetSize(m.width, m.height)
		m.storePicker.SetActiveStorePath(m.services.ActiveStorePath)
		// Re-listed on every open, not cached: a store registered from another
		// terminal since the last look must appear without a restart.
		return m, batchCmds(modeCmd, m.storePicker.Init())
	case m.keys.Match(config.ShellContext, config.ShellActionModeBoard, msg):
		m.enterBrowseMode(mode.Board)
		return m, batchCmds(modeCmd, m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionModeDocs, msg):
		m.enterBrowseMode(mode.Docs)
		return m, batchCmds(modeCmd, m.lazyInitActiveTabCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionModeSearch, msg):
		m.enterBrowseMode(mode.Search)
		return m, batchCmds(modeCmd, m.lazyInitActiveTabCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
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
		return m, batchCmds(modeCmd, m.lazyInitActiveTabCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
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
		return m, batchCmds(modeCmd, m.lazyInitActiveTabCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionModeCyclePrev, msg):
		m.applyModeCycle(prevMode(m.active, m.lastBrowse))
		return m, batchCmds(modeCmd, m.lazyInitActiveTabCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
	case m.keys.Match(config.ShellContext, config.ShellActionEscape, msg):
		// If a dialog-open was in flight when ESC arrived, the guard has
		// already been cleared at the top of this branch. Consume ESC as
		// "cancel the pending open" and keep the current mode — do NOT pop
		// Detail → Board (or Search → Board) while the load is in progress.
		if hadPendingDialog {
			return m, modeCmd
		}
		// The picker is a surface above the shell, so leaving it restores the
		// mode it was opened from — including Detail — rather than popping to
		// the home tab. Nothing below it was touched, so its selection and
		// scroll position are still where the operator left them.
		if m.active == mode.StorePicker {
			m.active = m.pickerReturn
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
		return m, batchCmds(modeCmd, m.scoped(prepareEditCmd(m.ctx, m.services, issueID)))
	case m.keys.Match(config.ShellContext, config.ShellActionCreateIssue, msg):
		m.pendingDialog = pendingDialogGuard{active: true, kind: mutationCreate}
		return m, batchCmds(modeCmd, m.scoped(loadMutationCatalogsCmd(m.ctx, m.services, mutationCreate, domain.IssueSummary{})))
	case m.keys.Match(config.ShellContext, config.ShellActionUpdateIssue, msg):
		issue, ok := m.mutationTargetIssue()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue to update", toaster.StyleWarn))
		}
		m.pendingDialog = pendingDialogGuard{active: true, kind: mutationUpdate}
		return m, batchCmds(modeCmd, m.scoped(loadMutationCatalogsCmd(m.ctx, m.services, mutationUpdate, issue)))
	case m.keys.Match(config.ShellContext, config.ShellActionCloseIssue, msg):
		issue, ok := m.mutationTargetIssue()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue to close", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, m.openMutationModal(mutationDialogState{kind: mutationClose, issue: issue}))
	case m.keys.Match(config.ShellContext, config.ShellActionCommentIssue, msg):
		issue, ok := m.mutationTargetIssue()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue to comment on", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, m.openMutationModal(mutationDialogState{kind: mutationComment, issue: issue}))
	case m.keys.Match(config.ShellContext, config.ShellActionLaunchNvim, msg):
		if m.active != mode.Detail {
			return m, modeCmd
		}
		issueContext, ok := m.selectedIssueContext()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue for launcher", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, launchActionCmd(m.ctx, m.services, LaunchActionNvim, issueContext))
	case m.keys.Match(config.ShellContext, config.ShellActionLaunchOpencode, msg):
		if m.active != mode.Detail {
			return m, modeCmd
		}
		issueContext, ok := m.selectedIssueContext()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue for launcher", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, launchActionCmd(m.ctx, m.services, LaunchActionOpencode, issueContext))
	case m.keys.Match(config.ShellContext, config.ShellActionLaunchShell, msg):
		if m.active != mode.Detail {
			return m, modeCmd
		}
		issueContext, ok := m.selectedIssueContext()
		if !ok {
			return m, batchCmds(modeCmd, m.showToast("No selected issue for launcher", toaster.StyleWarn))
		}
		return m, batchCmds(modeCmd, launchActionCmd(m.ctx, m.services, LaunchActionShellCommand, issueContext))
	}

	return m, modeCmd
}

// handleOverlayMessage routes msg to whichever overlay is open. handled is
// false when none is, in which case the caller falls through to the message
// switch. An open overlay consumes the message: that is why this runs before
// routing and not inside it.
func (m Model) handleOverlayMessage(msg tea.Msg, modeCmd tea.Cmd) (tea.Model, tea.Cmd, bool) {
	// These message types are the shell's own and an overlay never consumes
	// them. Both tick chains re-arm only from their own handlers in update(),
	// so a swallowed tick froze the spinner and stopped auto-refresh for the
	// rest of the session; and the shell's resize case is the only caller of
	// applyWorkspaceSizeToBrowseModes and detail.ClampScroll, so a swallowed
	// resize left every browse tab sized to the raw terminal until the next
	// resize with no overlay open. That case sizes the overlays too.
	//
	// A swallowed store listing is the same shape of bug: the picker's in-flight
	// state is cleared only by its own result, so losing one leaves it reading
	// "Reading the central store registry…" and spinning for the rest of the
	// session. A swallowed open request or opened store silently drops a switch
	// the operator asked for.
	switch msg.(type) {
	case loading.TickMsg, refreshTickMsg, tea.WindowSizeMsg,
		storepickermode.StoresLoadedMsg, storepickermode.OpenMsg, storeOpenedMsg:
		return m, nil, false
	}

	if m.showActionModal {
		if _, ok := msg.(modal.CancelMsg); ok {
			m.showActionModal = false
			return m, modeCmd, true
		}

		if submit, ok := msg.(modal.SubmitMsg); ok {
			m.showActionModal = false
			return m, batchCmds(modeCmd, m.scoped(submitMutationCmd(m.services, m.actionState, submit.Values))), true
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
