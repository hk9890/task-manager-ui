package search

import (
	"context"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	uidetails "github.com/hk9890/task-manager-ui/internal/ui/details"
	uisearch "github.com/hk9890/task-manager-ui/internal/ui/search"
)

type searchLoadedMsg struct {
	appliedQuery string
	page         domain.SearchResultPage
	err          error
}

type selectionAnchor struct {
	issueID string
	row     int
}

// SessionState captures the current search session state for shell integration.
type SessionState struct {
	DraftQuery   string
	AppliedQuery string
	Page         domain.SearchResultPage
	Loading      bool
	Reloading    bool
	Error        string
}

// Model is the standalone search mode controller.
type Model struct {
	ctx    context.Context
	repo   repository.Repository
	logger *slog.Logger
	keys   config.ResolvedKeyBindings

	width  int
	height int

	loading       bool
	reloading     bool
	hasLoadedPage bool
	errText       string

	draftQuery   string
	appliedQuery string
	focus        uisearch.FocusPane

	page        domain.SearchResultPage
	selectedRow int
	typing      bool

	selectedDetail            domain.IssueDetail
	selectedDetailLoading     bool
	metadataSelectedField     uidetails.MetadataFieldKey
	pendingOpenStatusDialog   bool
	pendingOpenPriorityDialog bool

	pendingSelectionAnchor *selectionAnchor

	// pendingDraft holds a typed+submitted draft query that arrived while a
	// search was already in flight. When the in-flight search resolves, this
	// pending submit is automatically re-fired so the user's Enter intent is
	// never silently discarded.
	pendingDraft *string
}

// NewModel creates a search mode controller.
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
			panic(err)
		}
	}
	return &Model{
		ctx:                   ctx,
		repo:                  repo,
		logger:                logger,
		keys:                  keys,
		focus:                 uisearch.FocusQuery,
		metadataSelectedField: uidetails.MetadataFieldStatus,
	}
}

// Init loads default all-issues search results for empty query.
func (m *Model) Init() tea.Cmd {
	m.loading = true
	m.reloading = false
	m.errText = ""
	m.typing = false
	return loadSearchCmd(m.ctx, m.repo, domain.SearchIssuesQuery{Limit: m.searchItemCapacity(), Offset: 0})
}

// Update processes search-specific messages and keybindings.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return nil
	case searchLoadedMsg:
		m.loading = false
		m.reloading = false
		m.typing = false
		anchor := m.pendingSelectionAnchor
		m.pendingSelectionAnchor = nil
		if msg.err != nil {
			m.errText = msg.err.Error()
			// A queued Enter-submit must fire even when the in-flight search errored
			// — the user's submit intent is never silently dropped. Force the re-fire
			// (the failed search may have applied a different query, and retrying the
			// same text is still desirable since the prior attempt failed).
			if cmd := m.consumePendingDraft(m.appliedQuery, true); cmd != nil {
				return cmd
			}
			if !m.hasResults() {
				m.selectedRow = 0
				m.selectedDetail = domain.IssueDetail{}
				m.selectedDetailLoading = false
				return m.selectionChangedCmd()
			}
			return nil
		}

		m.errText = ""
		m.appliedQuery = msg.appliedQuery
		m.page = msg.page
		m.hasLoadedPage = true
		m.selectedDetailLoading = false
		if anchor != nil {
			m.restoreSelectionFromAnchor(anchor)
		} else {
			m.normalizeSelection()
		}
		m.selectedDetail = domain.IssueDetail{}

		// If the user pressed Enter while this search was in flight, consume the
		// queued intent and fire the pending search now that we're no longer loading.
		// Only re-fire when the pending query actually differs from what just landed;
		// if they match, the result set is already correct.
		if cmd := m.consumePendingDraft(m.appliedQuery, false); cmd != nil {
			return cmd
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
		// Only consume Esc when the query input is focused; Esc has a
		// specific local meaning there (clear / unfocus). For all other
		// focus states, let the key fall through so the shell-level
		// escape action can fire (CapturesShellKey already returns false
		// for Esc in non-query focus states, so the shell handler runs).
		return nil
	case tea.KeyBackspace:
		if m.focus != uisearch.FocusQuery {
			return nil
		}
		if m.draftQuery == "" {
			return nil
		}
		runes := []rune(m.draftQuery)
		m.draftQuery = string(runes[:len(runes)-1])
		m.typing = true
		return nil
	case tea.KeyCtrlU:
		if m.focus != uisearch.FocusQuery {
			return nil
		}
		if m.draftQuery == "" {
			return nil
		}
		m.draftQuery = ""
		m.typing = false
		return nil
	}

	switch {
	case (msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace) && m.focus == uisearch.FocusQuery:
		// Bubble Tea delivers a lone space as KeySpace (not KeyRunes) with
		// Runes==[]rune{' '}; without accepting it the query box would silently
		// drop spaces, making multi-word (AND-of-words) search impossible to type.
		m.draftQuery += string(msg.Runes)
		m.typing = true
		return nil
	case msg.Type == tea.KeyEnter && m.focus == uisearch.FocusQuery:
		if m.loading {
			// A search is already in flight (often the Init empty-query load).
			// Queue this submit so it fires once the in-flight search resolves.
			// The searchLoadedMsg handler will consume pendingDraft and re-fire.
			draft := strings.TrimSpace(m.draftQuery)
			m.pendingDraft = &draft
			return nil
		}
		return m.triggerSearch()
	case msg.Type == tea.KeyEnter && m.focus == uisearch.FocusMetadata:
		switch m.metadataSelectedField {
		case uidetails.MetadataFieldStatus:
			m.pendingOpenStatusDialog = true
		case uidetails.MetadataFieldPriority:
			m.pendingOpenPriorityDialog = true
		}
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionOpenDetail, msg):
		if m.focus == uisearch.FocusResults && m.currentSelection() != nil {
			return func() tea.Msg {
				return mode.ActionRequestMsg{Mode: mode.Search, Action: mode.ActionOpenDetail}
			}
		}
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionMoveUp, msg):
		if m.focus == uisearch.FocusResults && m.selectedRow <= 0 {
			m.focus = uisearch.FocusQuery
			return nil
		}
		if m.focus == uisearch.FocusResults && m.moveSelection(-1) {
			m.selectedDetailLoading = true
			m.selectedDetail = domain.IssueDetail{}
			return m.selectionChangedCmd()
		}
		if m.focus == uisearch.FocusMetadata {
			m.moveMetadataSelection(-1)
			return nil
		}
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionMoveDown, msg):
		if m.focus == uisearch.FocusQuery {
			if m.hasResults() {
				m.focus = uisearch.FocusResults
			}
			return nil
		}
		if m.focus == uisearch.FocusResults && m.moveSelection(1) {
			m.selectedDetailLoading = true
			m.selectedDetail = domain.IssueDetail{}
			return m.selectionChangedCmd()
		}
		if m.focus == uisearch.FocusMetadata {
			m.moveMetadataSelection(1)
			return nil
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
	case m.keys.Match(config.SearchContext, config.SearchActionFocusQuery, msg):
		m.focus = uisearch.FocusQuery
		return nil
	case m.keys.Match(config.SearchContext, config.SearchActionReload, msg):
		return m.Reload()
	case msg.Type == tea.KeyRunes:
		return nil
	default:
		return nil
	}
}

func (m *Model) moveFocusLeft() {
	switch m.focus {
	case uisearch.FocusMetadata:
		m.focus = uisearch.FocusContent
	case uisearch.FocusContent:
		m.focus = uisearch.FocusResults
	}
}

func (m *Model) moveFocusRight() {
	switch m.focus {
	case uisearch.FocusResults:
		m.focus = uisearch.FocusContent
	case uisearch.FocusContent:
		m.focus = uisearch.FocusMetadata
		m.ensureMetadataSelection()
	}
}

func (m *Model) cycleFocus(delta int) {
	order := []uisearch.FocusPane{uisearch.FocusQuery, uisearch.FocusResults, uisearch.FocusContent, uisearch.FocusMetadata}
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
	if !m.hasResults() && order[idx] != uisearch.FocusQuery {
		m.focus = uisearch.FocusQuery
		return
	}
	m.focus = order[idx]
	if m.focus == uisearch.FocusMetadata {
		m.ensureMetadataSelection()
	}
}

// consumePendingDraft fires a queued Enter-submit that arrived while a search
// was in flight, then clears it. It returns the search command to run, or nil
// when there is no queued submit (or, with forceRefire=false, when the queued
// query already matches appliedQuery so the result set is already correct).
// forceRefire is set on the error path so a queued submit still runs even if the
// failed search left appliedQuery equal to the pending text.
func (m *Model) consumePendingDraft(appliedQuery string, forceRefire bool) tea.Cmd {
	if m.pendingDraft == nil {
		return nil
	}
	pending := *m.pendingDraft
	m.pendingDraft = nil
	if forceRefire || pending != appliedQuery {
		return m.triggerSearchWithAnchor(pending, nil)
	}
	return nil
}

func (m *Model) triggerSearch() tea.Cmd {
	return m.triggerSearchWithAnchor(strings.TrimSpace(m.draftQuery), nil)
}

func (m *Model) triggerSearchPreservingSelection() tea.Cmd {
	anchor := m.captureSelectionAnchor()
	return m.triggerSearchWithAnchor(strings.TrimSpace(m.appliedQuery), anchor)
}

func (m *Model) triggerSearchWithAnchor(queryText string, anchor *selectionAnchor) tea.Cmd {
	// Defense-in-depth: guard against re-entrant calls from future callers that
	// may not check m.loading at the call site. The call-site guard in Reload()
	// is the primary protection; this guard is a second line of defense
	// so the invariant is maintained regardless of how triggerSearchWithAnchor is called.
	if m.loading {
		m.logger.Debug("triggerSearchWithAnchor re-entry suppressed; search already in flight",
			"query", queryText)
		return nil
	}
	query := domain.SearchIssuesQuery{
		Text:   queryText,
		Limit:  m.searchItemCapacity(),
		Offset: 0,
	}
	m.loading = true
	m.reloading = m.hasLoadedPage
	m.errText = ""
	m.pendingSelectionAnchor = anchor
	return loadSearchCmd(m.ctx, m.repo, query)
}

// View renders the standalone search surface.
func (m *Model) View(skeletonPhase int) string {
	return uisearch.Render(uisearch.State{
		Loading:               m.loading,
		Reloading:             m.reloading,
		Error:                 m.errText,
		Query:                 m.draftQuery,
		AppliedQuery:          m.appliedQuery,
		Focus:                 m.focus,
		Typing:                m.typing,
		Results:               m.results(),
		Metadata:              m.page.Metadata,
		SelectedID:            m.selectedIssueID(),
		SelectedDetail:        m.selectedDetail,
		DetailLoading:         m.selectedDetailLoading,
		MetadataSelectedField: m.metadataSelectedField,
		QuickActions: uidetails.QuickActionLabels{
			EditIssue:    m.keys.DisplayLabel(config.ShellContext, config.ShellActionEditIssue),
			UpdateIssue:  m.keys.DisplayLabel(config.ShellContext, config.ShellActionUpdateIssue),
			AddComment:   m.keys.DisplayLabel(config.ShellContext, config.ShellActionCommentIssue),
			CloseIssue:   m.keys.DisplayLabel(config.ShellContext, config.ShellActionCloseIssue),
			ReloadDetail: m.keys.DisplayLabel(config.ShellContext, config.ShellActionReloadDetail),
		},
		Width:         m.width,
		Height:        m.height,
		SkeletonPhase: skeletonPhase,
	})
}

// SetSize updates render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// searchItemCapacity returns the number of result rows that fit in the results
// pane at the current terminal height.
//
// Chrome breakdown: the query FormSection occupies searchQueryHeight (5) rows
// (1 top border + 3 content lines + 1 bottom border), and the results
// FormSection adds 2 border rows (1 top + 1 bottom). Total chrome = 7.
// Formula: max(1, height-7).
//
// When height is 0 (before the first tea.WindowSizeMsg), a safe default of 20
// is returned so that Init() fires queries with a reasonable limit.
func (m *Model) searchItemCapacity() int {
	if m.height == 0 {
		return 20 // safe default before first WindowSizeMsg
	}
	rows := m.height - 7
	if rows < 1 {
		rows = 1
	}
	return rows
}

// IsLoading reports whether a repository search is active.
func (m *Model) IsLoading() bool {
	return m.loading
}

// Reload refreshes current search results without mutating query input state.
func (m *Model) Reload() tea.Cmd {
	if m.loading {
		m.logger.Debug("manual search refresh suppressed; refresh already in flight",
			"trigger", "search-manual")
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
	if m.page.Metadata.ReturnedCount > 0 {
		return m.page.Metadata.ReturnedCount
	}
	return len(m.page.Results)
}

// SessionState returns the current search session snapshot.
func (m *Model) SessionState() SessionState {
	return SessionState{
		DraftQuery:   m.draftQuery,
		AppliedQuery: m.appliedQuery,
		Page:         cloneSearchResultPage(m.page),
		Loading:      m.loading,
		Reloading:    m.reloading,
		Error:        m.errText,
	}
}

func (m *Model) moveSelection(delta int) bool {
	if !m.hasResults() {
		return false
	}
	previous := m.selectedRow
	m.selectedRow += delta
	m.normalizeSelection()
	return m.selectedRow != previous
}

func (m *Model) normalizeSelection() {
	if !m.hasResults() {
		m.selectedRow = 0
		m.selectedDetail = domain.IssueDetail{}
		m.selectedDetailLoading = false
		return
	}
	if m.selectedRow < 0 {
		m.selectedRow = 0
	}
	if m.selectedRow >= len(m.page.Results) {
		m.selectedRow = len(m.page.Results) - 1
	}
}

func (m *Model) ensureMetadataSelection() {
	if m.metadataSelectedField != uidetails.MetadataFieldStatus && m.metadataSelectedField != uidetails.MetadataFieldPriority {
		m.metadataSelectedField = uidetails.MetadataFieldStatus
	}
}

func (m *Model) moveMetadataSelection(delta int) {
	fields := []uidetails.MetadataFieldKey{uidetails.MetadataFieldStatus, uidetails.MetadataFieldPriority}
	m.ensureMetadataSelection()
	idx := 0
	if m.metadataSelectedField == uidetails.MetadataFieldPriority {
		idx = 1
	}
	next := idx + delta
	if next < 0 {
		next = 0
	}
	if next >= len(fields) {
		next = len(fields) - 1
	}
	m.metadataSelectedField = fields[next]
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
		for idx, result := range m.page.Results {
			if result.Issue.ID == anchor.issueID {
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
	if !m.hasResults() || m.selectedRow < 0 || m.selectedRow >= len(m.page.Results) {
		return nil
	}
	selection := mode.Selection{Issue: m.page.Results[m.selectedRow].Issue}
	return &selection
}

func (m *Model) selectionChangedCmd() tea.Cmd {
	selection := m.currentSelection()
	if selection == nil {
		m.selectedDetailLoading = false
	}
	return func() tea.Msg {
		return mode.SelectionChangedMsg{Mode: mode.Search, Selection: selection}
	}
}

// loadSearchCmd fires the Search repository call and wraps the result in a
// searchLoadedMsg. ctx is the model's lifetime context set at construction;
// it does not change after NewModel returns, so reading it inside the closure
// at BubbleTea-execute time is safe.
func loadSearchCmd(ctx context.Context, repo repository.Repository, query domain.SearchIssuesQuery) tea.Cmd {
	return func() tea.Msg {
		appliedQuery := strings.TrimSpace(query.Text)
		page, err := repo.Search(ctx, query)
		if err != nil {
			return searchLoadedMsg{appliedQuery: appliedQuery, err: err}
		}

		return searchLoadedMsg{appliedQuery: appliedQuery, page: page}
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

// SetSelectedDetail updates shell-owned loaded detail for the current selection.
func (m *Model) SetSelectedDetail(detail domain.IssueDetail, loading bool) {
	m.selectedDetail = detail
	m.selectedDetailLoading = loading
}

func (m *Model) results() []domain.IssueSummary {
	issues := make([]domain.IssueSummary, 0, len(m.page.Results))
	for _, result := range m.page.Results {
		issues = append(issues, result.Issue)
	}
	return issues
}

func (m *Model) hasResults() bool {
	return len(m.page.Results) > 0
}

func cloneSearchResultPage(page domain.SearchResultPage) domain.SearchResultPage {
	results := append([]domain.SearchResult(nil), page.Results...)
	return domain.SearchResultPage{Results: results, Metadata: page.Metadata}
}

// ConsumeOpenStatusDialogIntent reports and clears pending status-dialog intent.
func (m *Model) ConsumeOpenStatusDialogIntent() bool {
	if !m.pendingOpenStatusDialog {
		return false
	}
	m.pendingOpenStatusDialog = false
	return true
}

// ConsumeOpenPriorityDialogIntent reports and clears pending priority-dialog intent.
func (m *Model) ConsumeOpenPriorityDialogIntent() bool {
	if !m.pendingOpenPriorityDialog {
		return false
	}
	m.pendingOpenPriorityDialog = false
	return true
}
