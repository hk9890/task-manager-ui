package docs

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
	// docType is the task-manager issue type this mode lists.
	docType = "doc"

	// columnTitle is the title of the single docs column.
	columnTitle = "Docs"

	// queryPlaceholder is what the query line says while nothing is typed.
	queryPlaceholder = "filter docs"

	// defaultItemCapacity is the row window used before the first
	// tea.WindowSizeMsg sets a real height.
	defaultItemCapacity = 20
)

// docsLoadedMsg carries the result of a docs Search repository call.
type docsLoadedMsg struct {
	page domain.SearchResultPage
	err  error
}

// Model is the standalone docs mode controller backed by repository calls.
type Model struct {
	ctx    context.Context
	repo   repository.Repository
	logger *slog.Logger
	keys   config.ResolvedKeyBindings
	width  int
	height int

	// now is the clock the age markers measure against. Tests replace it so a
	// golden pins which rows fall on which side of a divider.
	now func() time.Time

	// issues is every loaded doc; shown is the docs of it the query matches,
	// and is issues itself while the query is empty. Selection, scroll, hit
	// test, hover and View read shown.
	issues []domain.IssueSummary
	shown  []domain.IssueSummary
	total  int
	err    error

	// query is the filter over the column. It outlives a reload.
	query mode.Query

	// loading is the column's visual loading state; inflight guards against
	// concurrent reloads. Both start false: docs mode is lazily initialised on
	// the first switch into the tab (like search), so reporting "loading"
	// before that would keep the shell spinner on for a surface nobody opened.
	loading  bool
	inflight bool

	selectedRow  int
	scrollOffset int

	pointer *mode.Pointer
	clicks  mode.ClickTracker

	// anchorIssueID is the issue selected when an auto-refresh started. The
	// load handler restores the cursor onto it when it survives the refresh.
	anchorIssueID string
}

// NewModel builds the docs mode controller. Keybindings default to the
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
			panic(fmt.Sprintf("invalid default docs keybindings: %v", err))
		}
	}

	return &Model{
		ctx:    ctx,
		repo:   repo,
		logger: logger,
		keys:   keys,
		now:    time.Now,
	}
}

// Init loads the docs list from the repository.
func (m *Model) Init() tea.Cmd {
	return m.startReload(mode.RefreshReload)
}

// Reload is the manual refresh: a full reset, dropped while one is in flight.
func (m *Model) Reload() tea.Cmd {
	if m.inflight {
		m.logger.Debug("manual docs refresh suppressed; refresh already in flight",
			"trigger", "docs-manual")
		return nil
	}
	return m.startReload(mode.RefreshReload)
}

// Update processes docs-specific messages and keybindings. Row movement, open
// detail, and reload reuse the board keybinding context: the docs column is a
// board column, so the two surfaces must not drift apart.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return nil

	case docsLoadedMsg:
		return m.apply(msg)

	case mode.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		if consumed, changed := m.query.HandleKey(msg); consumed {
			if !changed {
				return nil
			}
			return m.queryChanged()
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
			return m.moveRow(-len(m.shown))
		case m.keys.Match(config.BoardContext, config.BoardActionMoveEnd, msg):
			return m.moveRow(len(m.shown))
		case m.keys.Match(config.BoardContext, config.BoardActionOpenDetail, msg):
			if m.currentSelection() == nil {
				return nil
			}
			return mode.RequestActionCmd(mode.Docs, mode.ActionOpenDetail)
		case m.keys.Match(config.BoardContext, config.BoardActionReload, msg):
			return m.Reload()
		}
	}

	return nil
}

// View renders the docs column.
func (m *Model) View(skeletonPhase int) string {
	state := m.viewState(skeletonPhase)
	state.Hover = m.hover(state)
	return uiboard.Render(state)
}

// uiColumn is the docs column as the renderer sees it. View and clampSelection
// build the same value, so the stored offset is computed against the rows
// View draws.
func (m *Model) uiColumn() uiboard.Column {
	errText := ""
	if m.err != nil {
		errText = m.err.Error()
	}
	return uiboard.Column{
		Title:        columnTitle,
		Rows:         m.shown,
		SelectedRow:  m.selectedRow,
		ScrollOffset: m.scrollOffset,
		Total:        m.total,
		TotalIsExact: true,
		Loaded:       len(m.issues),
		Loading:      m.loading,
		Error:        errText,
		AgeMarkers:   true,
	}
}

// SetSize updates render dimensions. The clamp is what board's SetSize does and
// for the same reason: itemCapacity() is derived from the height, so a resize
// leaves an offset that was valid for the old window.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.clampSelection()
}

// IsLoading reports whether the column is in its loading state.
func (m *Model) IsLoading() bool {
	return m.loading
}

// AutoRefresh reloads the docs list, preserving the selected issue when it
// survives the refresh.
func (m *Model) AutoRefresh() tea.Cmd {
	if m.inflight {
		return nil
	}
	return m.startReload(mode.RefreshAuto)
}

// startReload dispatches the docs Search call. reset clears the cursor (cold
// start and manual reload); otherwise the current issue is anchored so an
// auto-refresh does not move the selection under the user.
func (m *Model) startReload(rm mode.RefreshMode) tea.Cmd {
	if m.inflight {
		m.logger.Debug("startReload re-entry suppressed; docs refresh already in flight")
		return nil
	}
	m.inflight = true
	m.loading = true
	m.err = nil

	m.anchorIssueID = ""
	if rm == mode.RefreshReload {
		m.selectedRow = 0
		m.scrollOffset = 0
	} else if selection := m.currentSelection(); selection != nil {
		m.anchorIssueID = selection.Issue.ID
	}

	// Limit 0 means "no limit" in both backends: the doc set is small and the
	// column scrolls, so paging it would add state with nothing to show for it.
	// Docs deliberately span the closed history: a doc is reference material, and
	// closing one archives it rather than finishing work. The Search default is
	// open-only (see domain.SearchIssuesQuery.IncludeClosed), so this opts in.
	query := domain.SearchIssuesQuery{Types: []string{docType}, IncludeClosed: true}
	return loadDocsCmd(m.ctx, m.repo, query)
}

// apply settles the model from a completed docs load.
func (m *Model) apply(msg docsLoadedMsg) tea.Cmd {
	m.loading = false
	m.inflight = false
	m.err = msg.err

	if msg.err != nil {
		// Keep the stale rows on screen; the inline error row explains why they
		// may be out of date.
		m.anchorIssueID = ""
		return m.selectionChangedCmd()
	}

	issues := make([]domain.IssueSummary, 0, len(msg.page.Results))
	for _, result := range msg.page.Results {
		issues = append(issues, result.Issue)
	}
	domain.SortByLastChange(issues)
	m.issues = issues
	m.shown = m.query.Filter(issues)
	m.total = len(issues)

	if anchor := m.anchorIssueID; anchor != "" {
		if idx, ok := m.findIssue(anchor); ok {
			m.selectedRow = idx
		}
	}
	m.anchorIssueID = ""

	m.clampSelection()
	return m.selectionChangedCmd()
}

// findIssue is the row of the issue among the docs the query matches.
func (m *Model) findIssue(issueID string) (int, bool) {
	for idx, issue := range m.shown {
		if issue.ID == issueID {
			return idx, true
		}
	}
	return 0, false
}

// ClearQuery empties the query, which the shell asks for on Escape. cleared is
// false when there was no text, and Escape is then the shell's.
func (m *Model) ClearQuery() (cleared bool, cmd tea.Cmd) {
	if !m.query.Clear() {
		return false, nil
	}
	return true, m.queryChanged()
}

// queryChanged narrows the column to the new query. The selection stays on the
// same doc when that doc still matches, and otherwise takes the first match.
func (m *Model) queryChanged() tea.Cmd {
	previous := m.selectedIssueID()

	m.shown = m.query.Filter(m.issues)
	m.selectedRow, _ = m.findIssue(previous)
	m.clampSelection()

	if m.selectedIssueID() == previous {
		return nil
	}
	return m.selectionChangedCmd()
}

func (m *Model) clampSelection() {
	if len(m.shown) == 0 {
		m.selectedRow = 0
		m.scrollOffset = 0
		return
	}
	if m.selectedRow < 0 {
		m.selectedRow = 0
	}
	if m.selectedRow >= len(m.shown) {
		m.selectedRow = len(m.shown) - 1
	}
	// Pull the window back inside the list first, as board's clampScrollOffsets
	// does. EnsureVisible only slides far enough to reveal the selected row, so
	// on its own a list that shrank under a scrolled offset keeps the offset and
	// draws its last rows with the ones above unreachable until the operator
	// moves up. MaxOffset counts the lines the renderer draws, as capacity does.
	capacity := m.itemCapacity()
	m.scrollOffset = min(m.scrollOffset, uiboard.MaxOffset(m.uiColumn(), capacity, m.now()))
	m.scrollOffset = uiboard.EnsureVisible(m.uiColumn(), capacity, m.now())
}

func (m *Model) moveRow(delta int) tea.Cmd {
	if len(m.shown) == 0 {
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
// current terminal height. It mirrors the board's section capacity; which of
// those rows are issues is uiboard.EnsureVisible's business.
func (m *Model) itemCapacity() int {
	if m.height == 0 {
		return defaultItemCapacity
	}
	return uiboard.ContentRows(m.height)
}

// pageRows is the number of docs a page key moves the selection by: the docs
// the column shows at the current height.
func (m *Model) pageRows() int {
	return max(1, m.itemCapacity()/issuerow.Height)
}

func (m *Model) currentSelection() *mode.Selection {
	if len(m.shown) == 0 {
		return nil
	}
	row := m.selectedRow
	if row < 0 || row >= len(m.shown) {
		row = 0
	}
	selection := mode.Selection{Issue: m.shown[row]}
	return &selection
}

func (m *Model) selectionChangedCmd() tea.Cmd {
	selection := m.currentSelection()
	return func() tea.Msg {
		return mode.SelectionChangedMsg{Mode: mode.Docs, Selection: selection}
	}
}

func loadDocsCmd(ctx context.Context, repo repository.Repository, query domain.SearchIssuesQuery) tea.Cmd {
	return func() tea.Msg {
		page, err := repo.Search(ctx, query)
		return docsLoadedMsg{page: page, err: err}
	}
}
