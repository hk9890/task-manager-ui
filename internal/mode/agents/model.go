// Package agents is the Agents tab: every agent of the store's project, those
// linked to an issue and those that are not. The shell hands it the agents of
// each poll and the issues In Progress agents commented on; the tab reads the
// issues that carry a link label itself. Its keys act on the selected agent
// through the shell, which owns the agent host.
package agents

import (
	"cmp"
	"context"
	"log/slog"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	uiagents "github.com/hk9890/task-manager-ui/internal/ui/agents"
)

// linkedIssuesMsg carries the issues that carry a link label.
type linkedIssuesMsg struct {
	issues []domain.IssueSummary
	err    error
}

// Model is the Agents tab.
type Model struct {
	ctx    context.Context
	repo   repository.Repository
	logger *slog.Logger
	keys   config.ResolvedKeyBindings
	width  int
	height int

	// live is the agents of the last poll, liveKnown false before the first
	// answer and after a failed poll, and absent why there may be none.
	live      []agent.Agent
	liveKnown bool
	absent    string

	// issues is every issue with a link label, closed ones included;
	// commentLinks the In Progress issues by the session of an agent that
	// commented on them.
	issues       []domain.IssueSummary
	issuesKnown  bool
	commentLinks map[string]domain.IssueSummary
	loading      bool

	rows     []uiagents.Row
	selected int
	offset   int
	// chosen is the key of the agent the selection belongs on. It outlives
	// its row while the rows are not known, so a failed poll does not move
	// the selection for good.
	chosen string
	// pending is the agent Select asked for before its row was there.
	pending string

	pointer *mode.Pointer
	clicks  mode.ClickTracker
}

// NewModel builds the Agents tab.
func NewModel(ctx context.Context, repo repository.Repository, logger *slog.Logger, keys config.ResolvedKeyBindings) *Model {
	if logger == nil {
		logger = slog.Default()
	}
	return &Model{ctx: ctx, repo: repo, logger: logger, keys: keys}
}

// Init reads the issues that carry a link label.
func (m *Model) Init() tea.Cmd { return m.load() }

// Reload reads them again; nil while a read is in flight.
func (m *Model) Reload() tea.Cmd { return m.load() }

// AutoRefresh is Reload: the agents themselves come from the shell's poll.
func (m *Model) AutoRefresh() tea.Cmd { return m.load() }

func (m *Model) load() tea.Cmd {
	if m.loading {
		return nil
	}
	m.loading = true
	ctx, repo := m.ctx, m.repo
	return func() tea.Msg {
		page, err := repo.Search(ctx, domain.SearchIssuesQuery{LabelContains: agent.LinkLabelPrefix, IncludeClosed: true})
		issues := make([]domain.IssueSummary, 0, len(page.Results))
		for _, result := range page.Results {
			issues = append(issues, result.Issue)
		}
		return linkedIssuesMsg{issues: issues, err: err}
	}
}

// IsLoading reports whether the read is in flight.
func (m *Model) IsLoading() bool { return m.loading }

// SetSize gives the tab its workspace.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	m.offset = uiagents.EnsureVisible(m.rows, m.selected, m.offset, m.height)
}

// ClearQuery does nothing: the tab has no query.
func (m *Model) ClearQuery() (bool, tea.Cmd) { return false, nil }

// HasQuery is false: the tab has no query.
func (m *Model) HasQuery() bool { return false }

// TakesKey is false: every key of the tab is a binding.
func (m *Model) TakesKey(tea.KeyPressMsg) bool { return false }

// SetAgents replaces the live agents with those of a poll. known is false
// when the poll failed, and absent then, or when there is no host to ask,
// says why no agent is shown.
func (m *Model) SetAgents(live []agent.Agent, known bool, absent string) tea.Cmd {
	m.live, m.liveKnown, m.absent = live, known, absent
	return m.rebuild()
}

// SetCommentLinks replaces the In Progress issues by commenting session.
func (m *Model) SetCommentLinks(links map[string]domain.IssueSummary) tea.Cmd {
	m.commentLinks = links
	return m.rebuild()
}

// Select puts the selection on the agent of session, now when its row is
// there and otherwise once a poll or a read brings it.
func (m *Model) Select(session string) tea.Cmd {
	m.pending = session
	return m.rebuild()
}

// Update handles the read's result, the keys and the mouse.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case linkedIssuesMsg:
		m.loading = false
		if msg.err != nil {
			m.logger.Warn("reading the issues linked to agents failed", "error", msg.err.Error())
			return nil
		}
		m.issues, m.issuesKnown = msg.issues, true
		return m.rebuild()
	case mode.MouseMsg:
		return m.mouse(msg)
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return nil
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	match := func(action string) bool { return m.keys.Match(config.BoardContext, action, msg) }
	page := max(1, uiagents.InnerHeight(m.height)/uiagents.RowLines)
	switch {
	case match(config.BoardActionMoveUp):
		return m.move(-1)
	case match(config.BoardActionMoveDown):
		return m.move(1)
	case match(config.BoardActionPageUp):
		return m.move(-page)
	case match(config.BoardActionPageDown):
		return m.move(page)
	case match(config.BoardActionMoveHome):
		return m.move(-len(m.rows))
	case match(config.BoardActionMoveEnd):
		return m.move(len(m.rows))
	case match(config.BoardActionReload):
		return m.Reload()
	}
	row, ok := m.selectedRow()
	if !ok {
		return nil
	}
	switch {
	case match(config.BoardActionOpenDetail):
		return mode.RequestAgentCmd(mode.Agents, mode.ActionOpenAgent, row.Agent)
	case match(config.BoardActionCloseAgent):
		return mode.RequestAgentCmd(mode.Agents, mode.ActionCloseAgent, row.Agent)
	case match(config.BoardActionOpenIssue):
		return mode.RequestOpenDetailCmd(mode.Agents, selectionOf(row))
	}
	return nil
}

// mouse is the list's mouse: the wheel moves the selection, one click
// selects a row and a second brings its agent to the front.
func (m *Model) mouse(msg mode.MouseMsg) tea.Cmd {
	m.pointer = msg.Pointer()
	if m.pointer == nil {
		return nil
	}
	switch msg.Kind {
	case mode.MouseWheelUp:
		return m.move(-1)
	case mode.MouseWheelDown:
		return m.move(1)
	case mode.MouseClick:
		target := ""
		row, ok := uiagents.HitTest(m.viewState(), msg.X, msg.Y)
		if ok {
			target = m.rows[row].Agent.Key()
		}
		if m.clicks.Double(target, m.selectedKey(), msg) {
			if selected, ok := m.selectedRow(); ok {
				return mode.RequestAgentCmd(mode.Agents, mode.ActionOpenAgent, selected.Agent)
			}
			return nil
		}
		if ok {
			return m.move(row - m.selected)
		}
	}
	return nil
}

// View draws the tab.
func (m *Model) View(int) string {
	state := m.viewState()
	if m.pointer != nil {
		if row, ok := uiagents.HitTest(state, m.pointer.X, m.pointer.Y); ok {
			state.Hover = row
		}
	}
	return uiagents.Render(state)
}

func (m *Model) viewState() uiagents.State {
	empty := m.absent
	if empty == "" && !m.liveKnown {
		empty = "waiting for the agent host"
	}
	return uiagents.State{
		Rows:         m.rows,
		SelectedRow:  m.selected,
		ScrollOffset: m.offset,
		Width:        m.width,
		Height:       m.height,
		Empty:        empty,
		Hover:        -1,
	}
}

func (m *Model) move(delta int) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	previous := m.selected
	m.selected = min(max(m.selected+delta, 0), len(m.rows)-1)
	m.offset = uiagents.EnsureVisible(m.rows, m.selected, m.offset, m.height)
	m.chosen, m.pending = m.selectedKey(), ""
	if m.selected == previous {
		return nil
	}
	return m.selectionChangedCmd()
}

// rebuild works out the rows again and keeps the selection on the agent it
// was on: the one Select asked for, else the one chosen before. While the
// rows are not settled an agent without a row may still come, so neither
// key is given up; once they are settled, an agent without a row is gone
// from the tab and the selection stays on the row at its place.
func (m *Model) rebuild() tea.Cmd {
	before := m.selectedIssueID()
	m.rows = buildRows(m.live, m.liveKnown, m.issues, m.commentLinks)
	rowOf := func(key string) int {
		if key == "" {
			return -1
		}
		return slices.IndexFunc(m.rows, func(r uiagents.Row) bool { return r.Agent.Key() == key })
	}
	if at := rowOf(m.pending); at >= 0 {
		m.chosen, m.pending = m.pending, ""
	} else if m.settled() {
		m.pending = ""
	}
	if at := rowOf(m.chosen); at >= 0 {
		m.selected = at
	}
	m.selected = min(max(m.selected, 0), max(len(m.rows)-1, 0))
	if m.chosen == "" || (m.settled() && rowOf(m.chosen) < 0) {
		m.chosen = m.selectedKey()
	}
	m.offset = uiagents.EnsureVisible(m.rows, m.selected, m.offset, m.height)
	if m.selectedIssueID() == before {
		return nil
	}
	return m.selectionChangedCmd()
}

// settled reports whether the rows are final until the next poll or read:
// the live agents and the linked issues are both known, and no read is in
// flight.
func (m *Model) settled() bool {
	return m.liveKnown && m.issuesKnown && !m.loading
}

func (m *Model) selectedRow() (uiagents.Row, bool) {
	if m.selected < 0 || m.selected >= len(m.rows) {
		return uiagents.Row{}, false
	}
	return m.rows[m.selected], true
}

func (m *Model) selectedKey() string {
	if row, ok := m.selectedRow(); ok {
		return row.Agent.Key()
	}
	return ""
}

func (m *Model) selectedIssueID() string {
	if row, ok := m.selectedRow(); ok && row.Issue != nil {
		return row.Issue.ID
	}
	return ""
}

// selectionChangedCmd announces the issue of the selected row, nil for an
// agent linked to none: the shell's issue keys act on that issue.
func (m *Model) selectionChangedCmd() tea.Cmd {
	var selection *mode.Selection
	if row, ok := m.selectedRow(); ok {
		selection = selectionOf(row)
	}
	return func() tea.Msg {
		return mode.SelectionChangedMsg{Mode: mode.Agents, Selection: selection}
	}
}

func selectionOf(row uiagents.Row) *mode.Selection {
	if row.Issue == nil {
		return nil
	}
	return &mode.Selection{Issue: *row.Issue}
}

// buildRows is the list: first each agent linked to an issue, then each live
// agent linked to none.
//
// An agent is linked by a link label on any issue, or by a comment on an In
// Progress issue; a label wins over a comment, and the first issue over a
// later one. A link label that no live agent holds is a gone row, but only
// on an open issue and while the live agents are known: a closed issue's
// agent finished, and an unknown list must not report a live agent gone.
// Each group puts the agent that needs the operator most first, by
// agent.Rank, then by title.
func buildRows(live []agent.Agent, liveKnown bool, issues []domain.IssueSummary, commentLinks map[string]domain.IssueSummary) []uiagents.Row {
	bySession := make(map[string]agent.Agent, len(live))
	for _, a := range live {
		if a.Session != "" {
			bySession[a.Session] = a
		}
	}
	var linked []uiagents.Row
	seen := make(map[string]bool)
	link := func(a agent.Agent, issue domain.IssueSummary) {
		seen[a.Key()] = true
		linked = append(linked, uiagents.Row{Agent: a, Issue: &issue})
	}
	for _, issue := range issues {
		for _, session := range agent.LinkSessions(issue.Labels) {
			a, isLive := bySession[session]
			switch {
			case seen[session]:
			case isLive:
				link(a, issue)
			case liveKnown && issue.Status != domain.StatusClosed:
				link(agent.GoneAgent(session), issue)
			}
		}
	}
	for _, session := range slices.Sorted(maps.Keys(commentLinks)) {
		if a, isLive := bySession[session]; isLive && !seen[session] {
			link(a, commentLinks[session])
		}
	}
	var unlinked []uiagents.Row
	for _, a := range live {
		if !seen[a.Key()] {
			unlinked = append(unlinked, uiagents.Row{Agent: a})
		}
	}
	byNeed := func(x, y uiagents.Row) int {
		return cmp.Or(
			agent.Rank(x.Agent.Status)-agent.Rank(y.Agent.Status),
			strings.Compare(x.Agent.Title(), y.Agent.Title()),
		)
	}
	slices.SortStableFunc(linked, byNeed)
	slices.SortStableFunc(unlinked, byNeed)
	return append(linked, unlinked...)
}
