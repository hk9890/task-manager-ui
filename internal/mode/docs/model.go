// Package docs is the docs-mode controller: a single-column browse surface over
// issues of type doc. A doc is not work — task-manager excludes it from the
// ready and blocked queues by construction (sdk/tasks Type.IsWork) — so an open
// doc never reaches a board column. This mode is where docs are browsed
// instead. Rows are drawn by internal/ui/board, the renderer the board uses.
package docs

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	uiboard "github.com/hk9890/task-manager-ui/internal/ui/board"
	"github.com/hk9890/task-manager-ui/internal/ui/scroll"
)

const (
	// docType is the task-manager issue type this mode lists.
	docType = "doc"

	// columnTitle is the title of the single docs column.
	columnTitle = "Docs"

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

	issues []domain.IssueSummary
	total  int
	err    error

	// loading is the column's visual loading state; inflight guards against
	// concurrent reloads. Both start false: docs mode is lazily initialised on
	// the first switch into the tab (like search), so reporting "loading"
	// before that would keep the shell spinner on for a surface nobody opened.
	loading  bool
	inflight bool

	selectedRow  int
	scrollOffset int

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
	}
}

// Init loads the docs list from the repository.
func (m *Model) Init() tea.Cmd {
	return m.startReload(true)
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

	case tea.KeyMsg:
		switch {
		case m.keys.Match(config.BoardContext, config.BoardActionMoveUp, msg):
			return m.moveRow(-1)
		case m.keys.Match(config.BoardContext, config.BoardActionMoveDown, msg):
			return m.moveRow(1)
		case m.keys.Match(config.BoardContext, config.BoardActionOpenDetail, msg):
			if m.currentSelection() == nil {
				return nil
			}
			return func() tea.Msg {
				return mode.ActionRequestMsg{Mode: mode.Docs, Action: mode.ActionOpenDetail}
			}
		case m.keys.Match(config.BoardContext, config.BoardActionReload, msg):
			if m.inflight {
				m.logger.Debug("manual docs refresh suppressed; refresh already in flight",
					"trigger", "docs-manual")
				return nil
			}
			return m.startReload(true)
		}
	}

	return nil
}

// View renders the docs column.
func (m *Model) View(skeletonPhase int) string {
	errText := ""
	if m.err != nil {
		errText = m.err.Error()
	}

	// No dashboard title: with a single column the tab chip and the column
	// header already name the surface, so the board's title line would only
	// repeat them. The renderer omits the line when the title is empty (it
	// still reserves the row, so the column height is unchanged).
	return uiboard.Render(uiboard.State{
		Columns: []uiboard.Column{{
			Title:        columnTitle,
			Rows:         m.issues,
			SelectedRow:  m.selectedRow,
			ScrollOffset: m.scrollOffset,
			Total:        m.total,
			TotalIsExact: true,
			Loading:      m.loading,
			Error:        errText,
		}},
		FocusedColumn: 0,
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
	return m.startReload(false)
}

// startReload dispatches the docs Search call. reset clears the cursor (cold
// start and manual reload); otherwise the current issue is anchored so an
// auto-refresh does not move the selection under the user.
func (m *Model) startReload(reset bool) tea.Cmd {
	if m.inflight {
		m.logger.Debug("startReload re-entry suppressed; docs refresh already in flight")
		return nil
	}
	m.inflight = true
	m.loading = true
	m.err = nil

	m.anchorIssueID = ""
	if reset {
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
	m.issues = issues
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

func (m *Model) findIssue(issueID string) (int, bool) {
	for idx, issue := range m.issues {
		if issue.ID == issueID {
			return idx, true
		}
	}
	return 0, false
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
	m.scrollOffset = scroll.EnsureVisible(m.scrollOffset, m.selectedRow, m.itemCapacity())
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

// itemCapacity returns the number of issue rows that fit in the column at the
// current terminal height. It mirrors the board's section capacity so the
// scroll window matches what internal/ui/board actually draws.
func (m *Model) itemCapacity() int {
	if m.height == 0 {
		return defaultItemCapacity
	}
	rows := m.height - 3
	if rows < 1 {
		rows = 1
	}
	// The renderer pins an inline error row above the issue rows, so one fewer
	// issue is visible while an error is shown.
	if m.err != nil && rows > 1 {
		rows--
	}
	return rows
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
		return mode.SelectionChangedMsg{Mode: mode.Docs, Selection: selection}
	}
}

func loadDocsCmd(ctx context.Context, repo repository.Repository, query domain.SearchIssuesQuery) tea.Cmd {
	return func() tea.Msg {
		page, err := repo.Search(ctx, query)
		return docsLoadedMsg{page: page, err: err}
	}
}
