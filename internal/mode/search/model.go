package search

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/mode/rowlist"
	"github.com/hk9890/task-manager-ui/internal/repository"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
)

const (
	// resultLimit is the page a search asks the store for. The header says so
	// when the store holds more matches than that.
	resultLimit = 100

	// queryPlaceholder is what the query line says while nothing is typed.
	queryPlaceholder = "search the store"

	// The column title names the scope the scope key toggles.
	titleOpen = "Results · open"
	titleAll  = "Results · all"

	// editPause is how long an edit of the query waits before its search runs.
	// Keys typed inside it run one search, for the text as it is when it ends.
	editPause = 150 * time.Millisecond
)

// landing is where the selection goes when the result of a search is applied.
type landing int

const (
	// landFirst puts the selection on the first result: the opening search and
	// the reload key.
	landFirst landing = iota
	// landOnIssue keeps the selection on its issue while the result holds it,
	// and otherwise puts it on the first result: an edit of the query or of the
	// scope.
	landOnIssue
	// landInPlace keeps the selection on its issue too, and otherwise on its
	// row, with the list scrolled as it was: an auto refresh, which the
	// operator did not ask for.
	landInPlace
)

// loadedMsg carries the result of one Search repository call.
type loadedMsg struct {
	// generation is the search this result answers. Every edit starts a new
	// one, so only the result of the latest is applied.
	generation int
	landing    landing
	page       domain.SearchResultPage
	err        error
}

// editPauseEndedMsg says the pause after an edit of the query is over.
type editPauseEndedMsg struct {
	// generation is the edit the pause followed. A later edit, or any other
	// search, supersedes it, and its pause then runs nothing.
	generation int
}

// Model is the store search controller backed by repository calls.
type Model struct {
	ctx    context.Context
	repo   repository.Repository
	logger *slog.Logger
	keys   config.ResolvedKeyBindings
	width  int
	height int

	// query is the text the store is searched for. Unlike the filter of a tab
	// it is not matched here: every edit runs a search, after editPause.
	query mode.Query
	// includeClosed is the scope: open issues, or all of them.
	includeClosed bool

	// issues is the page the latest applied search returned, in the store's
	// order, and total every match the store holds for it.
	issues []domain.IssueSummary
	total  int
	err    error

	// generation counts the searches started, the pause before the search of
	// an edit included. loading is true until the result of the latest one
	// arrives; settled once any result has.
	generation int
	loading    bool
	settled    bool
	// pause is editPause. A test shortens it.
	pause time.Duration

	// list is the selection, the scroll window and the pointer over issues.
	list rowlist.List

	// heldOpen is an Enter that arrived while a search the operator asked for
	// was in flight. The rows on screen then answer an older query, so the
	// detail opens on the result of the newest one. Any later key, click or
	// wheel notch drops it.
	heldOpen bool
}

// isScopeKey reports whether msg is the key that toggles the scope between
// open issues and all of them. It is built in, as the query keys are.
func isScopeKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyCtrlT && !msg.Alt
}

// TakesKey reports whether msg is a key of the query or the scope key, which
// the search takes before any binding.
func (m *Model) TakesKey(msg tea.KeyMsg) bool {
	return mode.IsQueryKey(msg) || isScopeKey(msg)
}

// NewModel builds the store search controller. Keybindings default to the
// resolved defaults when no resolved set is supplied.
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
			panic(fmt.Sprintf("invalid default search keybindings: %v", err))
		}
	}

	return &Model{
		ctx:    ctx,
		repo:   repo,
		logger: logger,
		keys:   keys,
		pause:  editPause,
	}
}

// SetEditPause replaces the pause an edit of the query waits before its search
// runs. A test of the shell passes zero: it runs every command to its message,
// and would otherwise sleep through each pause.
func (m *Model) SetEditPause(pause time.Duration) {
	m.pause = pause
}

// Init runs the first search. With nothing typed it lists the open issues.
func (m *Model) Init() tea.Cmd {
	return m.search(landFirst)
}

// Reload is the manual refresh: the query runs again and the selection goes
// back to the first result. It is dropped while a search is in flight.
func (m *Model) Reload() tea.Cmd {
	if m.loading {
		m.logger.Debug("manual search refresh suppressed; search already in flight",
			"trigger", "search-manual")
		return nil
	}
	return m.search(landFirst)
}

// AutoRefresh runs the query again and keeps the selection on the same issue
// when the store still returns it, and on the same row when it does not.
func (m *Model) AutoRefresh() tea.Cmd {
	if m.loading {
		return nil
	}
	return m.search(landInPlace)
}

// Update processes search messages and keys. Row movement, open detail and
// reload reuse the board keybinding context, as the docs tab does.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return nil

	case loadedMsg:
		return m.apply(msg)

	case editPauseEndedMsg:
		if msg.generation != m.generation {
			return nil
		}
		return m.searchCmd(msg.generation, landOnIssue)

	case mode.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		m.heldOpen = false
		if consumed, changed := m.query.HandleKey(msg); consumed {
			if !changed {
				return nil
			}
			return m.searchAfterPause()
		}
		if isScopeKey(msg) {
			m.includeClosed = !m.includeClosed
			return m.search(landOnIssue)
		}
		if moved, handled := m.list.MoveKey(m.keys, msg, m.viewState(0)); handled {
			return m.selectionMovedCmd(moved)
		}
		switch {
		case m.keys.Match(config.BoardContext, config.BoardActionOpenDetail, msg):
			return m.openDetail()
		case m.keys.Match(config.BoardContext, config.BoardActionReload, msg):
			return m.Reload()
		}
	}

	return nil
}

// openDetail is Enter and the second click on a row. While a search is in
// flight, or the pause before the search of an edit runs, it holds the open
// for that result: an edit answers another query, and an auto refresh moves
// the selection when its issue left the store's answer.
// The opening search has no rows the operator pressed Enter on, so it holds
// nothing. Text typed before its result arrives is what the operator chose:
// the pause of that edit supersedes the opening search, so no result has
// settled yet, and the open is held for the result of the text.
func (m *Model) openDetail() tea.Cmd {
	if m.loading && (m.settled || !m.query.Empty()) {
		m.heldOpen = true
		return nil
	}
	selection := m.currentSelection()
	if selection == nil {
		return nil
	}
	return mode.RequestOpenDetailCmd(mode.Search, selection)
}

// View renders the results column under the query line.
func (m *Model) View(skeletonPhase int) string {
	state := m.viewState(skeletonPhase)
	state.Hover = m.list.Hover(state)
	return uiboard.Render(state)
}

// uiColumn is the results column as the renderer sees it. View and
// clampSelection build the same value, so the stored offset is computed
// against the rows View draws.
func (m *Model) uiColumn() uiboard.Column {
	errText := ""
	if m.err != nil {
		errText = m.err.Error()
	}
	title := titleOpen
	if m.includeClosed {
		title = titleAll
	}
	return uiboard.Column{
		Title:        title,
		Rows:         m.issues,
		SelectedRow:  m.list.SelectedRow,
		ScrollOffset: m.list.ScrollOffset,
		Total:        m.total,
		TotalIsExact: m.total <= len(m.issues),
		// Only the first search draws the column as loading. A search runs after
		// every edit, and dimming the rows for each one would flicker them; the
		// header spinner says a search is in flight.
		Loading: m.loading && !m.settled,
		Error:   errText,
	}
}

// SetSize updates render dimensions and clamps the scroll offset to the new
// window, as the docs tab does.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.clampSelection()
}

// IsLoading reports whether a search is in flight, or the pause before the
// search of an edit is running.
func (m *Model) IsLoading() bool {
	return m.loading
}

// search starts a search for the query and scope as they are now. It
// supersedes the one in flight: that result is dropped when it arrives. land
// says where the selection goes when this one does.
func (m *Model) search(land landing) tea.Cmd {
	return m.searchCmd(m.nextGeneration(), land)
}

// searchAfterPause is the search of an edit of the query. The pause counts as
// that search in flight, so an Enter pressed inside it is held for the result
// of the typed text, and a reload or an auto refresh is dropped. Another edit
// inside the pause starts the pause again; only the last one runs a search.
func (m *Model) searchAfterPause() tea.Cmd {
	generation := m.nextGeneration()
	return tea.Tick(m.pause, func(time.Time) tea.Msg {
		return editPauseEndedMsg{generation: generation}
	})
}

// nextGeneration starts a generation, which supersedes the search in flight
// and a pause that has not ended.
func (m *Model) nextGeneration() int {
	m.generation++
	m.loading = true
	return m.generation
}

// searchCmd runs the search of generation for the query and scope as they are
// now.
func (m *Model) searchCmd(generation int, land landing) tea.Cmd {
	ctx, repo := m.ctx, m.repo
	query := domain.SearchIssuesQuery{
		Text:          m.query.Text(),
		IncludeClosed: m.includeClosed,
		Limit:         resultLimit,
	}
	return func() tea.Msg {
		page, err := repo.Search(ctx, query)
		return loadedMsg{generation: generation, landing: land, page: page, err: err}
	}
}

// apply settles the model from a completed search. A held Enter opens the
// detail of the row the result selects: the request carries that row, so it
// does not wait for the shell to hold the selection change sent with it.
func (m *Model) apply(msg loadedMsg) tea.Cmd {
	if msg.generation != m.generation {
		m.logger.Debug("search result dropped; a later search superseded it",
			"generation", msg.generation, "latest", m.generation)
		return nil
	}
	m.loading = false
	m.settled = true
	m.err = msg.err
	held := m.heldOpen
	m.heldOpen = false

	if msg.err != nil {
		// Keep the stale rows on screen; the inline error row explains why they
		// may be out of date. A held Enter opens nothing on them.
		return m.selectionChangedCmd()
	}

	// The selection is read now, not when the search started: the rows stay on
	// screen while it is in flight, and the operator can move on them.
	selectedIssueID := m.selectedIssueID()

	m.issues = make([]domain.IssueSummary, 0, len(msg.page.Results))
	for _, result := range msg.page.Results {
		m.issues = append(m.issues, result.Issue)
	}
	m.total = msg.page.Metadata.Total

	if msg.landing != landInPlace {
		m.list.SelectedRow, m.list.ScrollOffset = 0, 0
	}
	if msg.landing != landFirst {
		if row := slices.IndexFunc(m.issues, func(issue domain.IssueSummary) bool {
			return issue.ID == selectedIssueID
		}); row >= 0 {
			m.list.SelectedRow = row
		}
	}
	m.clampSelection()

	selection := m.currentSelection()
	if !held || selection == nil {
		return m.selectionChangedCmd()
	}
	return tea.Batch(m.selectionChangedCmd(), mode.RequestOpenDetailCmd(mode.Search, selection))
}

// ClearQuery empties the query and searches again, which the shell asks for
// on Escape. cleared is false when there was no text, and Escape is then the
// shell's.
func (m *Model) ClearQuery() (cleared bool, cmd tea.Cmd) {
	if !m.query.Clear() {
		return false, nil
	}
	return true, m.search(landOnIssue)
}

func (m *Model) clampSelection() {
	m.list.Clamp(m.viewState(0))
}

func (m *Model) currentSelection() *mode.Selection {
	return m.list.Selection(m.issues)
}

func (m *Model) selectedIssueID() string {
	return m.list.SelectedID(m.issues)
}

// selectionMovedCmd announces the selection after a key or the mouse moved it.
func (m *Model) selectionMovedCmd(moved bool) tea.Cmd {
	if !moved {
		return nil
	}
	return m.selectionChangedCmd()
}

func (m *Model) selectionChangedCmd() tea.Cmd {
	selection := m.currentSelection()
	return func() tea.Msg {
		return mode.SelectionChangedMsg{Mode: mode.Search, Selection: selection}
	}
}
