package search

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
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

	// defaultItemCapacity is the row window used before the first
	// tea.WindowSizeMsg sets a real height.
	defaultItemCapacity = 20
)

// loadedMsg carries the result of one Search repository call.
type loadedMsg struct {
	// generation is the search this result answers. Every edit starts a new
	// one, so only the result of the latest is applied.
	generation int
	// anchorIssueID is the issue selected when the search started, or "" for
	// a search that resets the selection.
	anchorIssueID string
	page          domain.SearchResultPage
	err           error
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
	// it is not matched here: every edit runs a search.
	query mode.Query
	// includeClosed is the scope: open issues, or all of them.
	includeClosed bool

	// issues is the page the latest applied search returned, in the store's
	// order, and total every match the store holds for it.
	issues []domain.IssueSummary
	total  int
	err    error

	// generation counts the searches started. loading is true until the
	// result of the latest one arrives; settled once any result has.
	generation int
	loading    bool
	settled    bool

	selectedRow  int
	scrollOffset int

	pointer *mode.Pointer
	clicks  mode.ClickTracker
}

// IsScopeKey reports whether msg is the key that toggles the scope between
// open issues and all of them. It is built in, as the query keys are, and the
// shell asks so that a key the search took runs no shell action.
func IsScopeKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyCtrlT && !msg.Alt
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
	}
}

// Init runs the first search. With nothing typed it lists the open issues.
func (m *Model) Init() tea.Cmd {
	return m.search("")
}

// Reload is the manual refresh: the query runs again and the selection goes
// back to the first result. It is dropped while a search is in flight.
func (m *Model) Reload() tea.Cmd {
	if m.loading {
		m.logger.Debug("manual search refresh suppressed; search already in flight",
			"trigger", "search-manual")
		return nil
	}
	return m.search("")
}

// AutoRefresh runs the query again and keeps the selection on the same issue
// when the store still returns it.
func (m *Model) AutoRefresh() tea.Cmd {
	if m.loading {
		return nil
	}
	return m.search(m.selectedIssueID())
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

	case mode.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		if consumed, changed := m.query.HandleKey(msg); consumed {
			if !changed {
				return nil
			}
			return m.search(m.selectedIssueID())
		}
		if IsScopeKey(msg) {
			m.includeClosed = !m.includeClosed
			return m.search(m.selectedIssueID())
		}
		switch {
		case m.keys.Match(config.BoardContext, config.BoardActionMoveUp, msg):
			return m.moveRow(-1)
		case m.keys.Match(config.BoardContext, config.BoardActionMoveDown, msg):
			return m.moveRow(1)
		case m.keys.Match(config.BoardContext, config.BoardActionPageUp, msg):
			return m.moveRow(-m.pageRows())
		case m.keys.Match(config.BoardContext, config.BoardActionPageDown, msg):
			return m.moveRow(m.pageRows())
		case m.keys.Match(config.BoardContext, config.BoardActionMoveHome, msg):
			return m.moveRow(-len(m.issues))
		case m.keys.Match(config.BoardContext, config.BoardActionMoveEnd, msg):
			return m.moveRow(len(m.issues))
		case m.keys.Match(config.BoardContext, config.BoardActionOpenDetail, msg):
			if m.currentSelection() == nil {
				return nil
			}
			return mode.RequestActionCmd(mode.Search, mode.ActionOpenDetail)
		case m.keys.Match(config.BoardContext, config.BoardActionReload, msg):
			return m.Reload()
		}
	}

	return nil
}

// View renders the results column under the query line.
func (m *Model) View(skeletonPhase int) string {
	state := m.viewState(skeletonPhase)
	state.Hover = m.hover(state)
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
		SelectedRow:  m.selectedRow,
		ScrollOffset: m.scrollOffset,
		Total:        m.total,
		TotalIsExact: m.total <= len(m.issues),
		// Only the first search draws the column as loading. A search runs on
		// every key, and dimming the rows for each one would flicker them; the
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

// IsLoading reports whether a search is in flight.
func (m *Model) IsLoading() bool {
	return m.loading
}

// search starts a search for the query and scope as they are now. It
// supersedes the one in flight: that result is dropped when it arrives. The
// selection returns to anchorIssueID when the result holds it, and to the
// first row otherwise.
func (m *Model) search(anchorIssueID string) tea.Cmd {
	m.generation++
	m.loading = true

	generation := m.generation
	ctx, repo := m.ctx, m.repo
	query := domain.SearchIssuesQuery{
		Text:          m.query.Text(),
		IncludeClosed: m.includeClosed,
		Limit:         resultLimit,
	}
	return func() tea.Msg {
		page, err := repo.Search(ctx, query)
		return loadedMsg{generation: generation, anchorIssueID: anchorIssueID, page: page, err: err}
	}
}

// apply settles the model from a completed search.
func (m *Model) apply(msg loadedMsg) tea.Cmd {
	if msg.generation != m.generation {
		m.logger.Debug("search result dropped; a later search superseded it",
			"generation", msg.generation, "latest", m.generation)
		return nil
	}
	m.loading = false
	m.settled = true
	m.err = msg.err

	if msg.err != nil {
		// Keep the stale rows on screen; the inline error row explains why they
		// may be out of date.
		return m.selectionChangedCmd()
	}

	m.issues = make([]domain.IssueSummary, 0, len(msg.page.Results))
	for _, result := range msg.page.Results {
		m.issues = append(m.issues, result.Issue)
	}
	m.total = msg.page.Metadata.Total

	m.selectedRow, m.scrollOffset = 0, 0
	for idx, issue := range m.issues {
		if issue.ID == msg.anchorIssueID {
			m.selectedRow = idx
			break
		}
	}
	m.clampSelection()
	return m.selectionChangedCmd()
}

// ClearQuery empties the query and searches again, which the shell asks for
// on Escape. cleared is false when there was no text, and Escape is then the
// shell's.
func (m *Model) ClearQuery() (cleared bool, cmd tea.Cmd) {
	if !m.query.Clear() {
		return false, nil
	}
	return true, m.search(m.selectedIssueID())
}

func (m *Model) clampSelection() {
	if len(m.issues) == 0 {
		m.selectedRow = 0
		m.scrollOffset = 0
		return
	}
	if m.selectedRow < 0 {
		m.selectedRow = 0
	}
	if m.selectedRow >= len(m.issues) {
		m.selectedRow = len(m.issues) - 1
	}
	// Pull the window back inside the list before sliding it to the selection,
	// as the docs tab does: a list that shrank under a scrolled offset would
	// otherwise keep the rows above its last ones out of reach. The column
	// draws no age markers, so the instant they measure against is not read.
	capacity := m.itemCapacity()
	m.scrollOffset = min(m.scrollOffset, uiboard.MaxOffset(m.uiColumn(), capacity, time.Time{}))
	m.scrollOffset = uiboard.EnsureVisible(m.uiColumn(), capacity, time.Time{})
}

func (m *Model) moveRow(delta int) tea.Cmd {
	if len(m.issues) == 0 {
		m.selectedRow = 0
		return nil
	}

	previous := m.selectedRow
	m.selectedRow += delta
	m.clampSelection()
	if m.selectedRow == previous {
		return nil
	}
	return m.selectionChangedCmd()
}

// itemCapacity returns the number of content rows the column holds at the
// current terminal height, as the docs tab counts them.
func (m *Model) itemCapacity() int {
	if m.height == 0 {
		return defaultItemCapacity
	}
	return uiboard.ContentRows(m.height)
}

// pageRows is the number of results a page key moves the selection by.
func (m *Model) pageRows() int {
	return max(1, m.itemCapacity()/issuerow.Height)
}

func (m *Model) currentSelection() *mode.Selection {
	if len(m.issues) == 0 {
		return nil
	}
	row := m.selectedRow
	if row < 0 || row >= len(m.issues) {
		row = 0
	}
	selection := mode.Selection{Issue: m.issues[row]}
	return &selection
}

func (m *Model) selectionChangedCmd() tea.Cmd {
	selection := m.currentSelection()
	return func() tea.Msg {
		return mode.SelectionChangedMsg{Mode: mode.Search, Selection: selection}
	}
}
