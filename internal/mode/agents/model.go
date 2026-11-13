// Package agents is the Agents tab: every agent of the store's project, those
// linked to an issue and those that are not, filtered by the query, and on a
// wide screen the Agent pane of the selected one. The shell hands it the
// agents of each poll and the issues In Progress agents commented on; the tab
// reads the issues that carry a link label itself, and the last message of the
// selected agent through the read the shell gives it. Its keys act on the
// selected agent through the shell, which owns the agent host.
package agents

import (
	"context"
	"log/slog"
	"reflect"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	uiagents "github.com/hk9890/task-manager-ui/internal/ui/agents"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	queryPlaceholder = "filter agents"
	// readPause is how long the selection rests on an agent before its last
	// message is read: moving through the list reads nothing.
	readPause = 250 * time.Millisecond
	// paneWheelLines is the lines one wheel notch scrolls the pane.
	paneWheelLines = 3
)

// Read is the last thing agent a said: agent.Host.Read.
type Read func(ctx context.Context, a agent.Agent) (string, error)

// readDueMsg ends the pause before the read of the agent of key.
type readDueMsg struct {
	key        string
	generation int
}

// messageReadMsg carries the last message of the agent of key, read while it
// did what activity and status say.
type messageReadMsg struct {
	key      string
	activity string
	status   agent.Status
	message  string
	err      error
}

// lastMessage is the last read message of one agent, and what the agent did
// when it was read: a poll that reports another activity or status makes it
// stale.
type lastMessage struct {
	activity string
	status   agent.Status
	message  string
	err      error
}

// linkedIssuesMsg carries the issues that carry a link label.
type linkedIssuesMsg struct {
	issues []domain.IssueSummary
	err    error
}

// Model is the Agents tab.
type Model struct {
	ctx    context.Context
	repo   repository.Repository
	read   Read
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
	// commentLinks the In Progress issues agents commented on.
	issues       []domain.IssueSummary
	issuesKnown  bool
	commentLinks []mode.CommentLink
	loading      bool
	// dropped is the sessions the app closed or unlinked whose link label a
	// read may still carry on an open issue: a read begun before the unlink.
	dropped map[string]bool

	// all is every row; rows is the rows the query matches, and is all
	// itself while the query is empty. Selection, scroll, hit test and View
	// read rows.
	all      []uiagents.Row
	rows     []uiagents.Row
	query    mode.Query
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

	// paneShown is whether the Agent pane is on screen: the tab is the one
	// shown, on a screen wide enough to split. Nothing is read while it is not.
	paneShown bool
	// messages is the last read message of each agent, by key; reading the
	// keys of the reads in flight, due the key whose pause runs, and
	// readGeneration that pause. paneOffset is the scroll of the Agent pane.
	messages       map[string]lastMessage
	reading        map[string]bool
	due            string
	readGeneration int
	paneOffset     int
}

// NewModel builds the Agents tab. read is nil when there is no agent host:
// the pane then reads no message.
func NewModel(ctx context.Context, repo repository.Repository, logger *slog.Logger, keys config.ResolvedKeyBindings, read Read) *Model {
	if logger == nil {
		logger = slog.Default()
	}
	return &Model{
		ctx: ctx, repo: repo, read: read, logger: logger, keys: keys,
		messages: make(map[string]lastMessage), reading: make(map[string]bool),
	}
}

// Init reads the issues that carry a link label.
func (m *Model) Init() tea.Cmd { return m.load() }

// Reload reads them again, nil while a read is in flight, and forgets every
// last message, so the selected agent's is read again.
func (m *Model) Reload() tea.Cmd {
	clear(m.messages)
	return tea.Batch(m.load(), m.readSoon())
}

// SetShown tells the tab whether it is the surface on screen. The shell calls
// it after every message: it is how a tab switch or a resize that puts the
// Agent pane on screen starts the read of the selected agent's message.
func (m *Model) SetShown(shown bool) tea.Cmd {
	wasShown := m.paneShown
	m.paneShown = shown && m.split()
	if m.paneShown && !wasShown {
		return m.readSoon()
	}
	return nil
}

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
	m.paneOffset = min(m.paneOffset, uiagents.PaneMaxOffset(m.viewState()))
}

// ClearQuery empties the query, and reports whether it held text.
func (m *Model) ClearQuery() (bool, tea.Cmd) {
	if !m.query.Clear() {
		return false, nil
	}
	return true, m.queryChanged()
}

// HasQuery reports whether the query holds text.
func (m *Model) HasQuery() bool { return !m.query.Empty() }

// TakesKey reports whether msg is typed into the query.
func (m *Model) TakesKey(msg tea.KeyPressMsg) bool { return mode.IsQueryKey(msg) }

// SetAgents replaces the live agents with those of a poll, and the In
// Progress issues agents commented on with the board's. known is false when
// the poll failed, and absent then, or when there is no host to ask, says
// why no agent is shown.
func (m *Model) SetAgents(live []agent.Agent, known bool, absent string, commentLinks []mode.CommentLink) tea.Cmd {
	m.live, m.liveKnown, m.absent, m.commentLinks = live, known, absent, commentLinks
	return m.rebuild()
}

// DropAgent takes the agent a off the tab at once, after the app closed or
// unlinked it: until a read without its link label lands, the label would
// draw it gone.
func (m *Model) DropAgent(a agent.Agent) tea.Cmd {
	m.live = slices.DeleteFunc(slices.Clone(m.live), func(live agent.Agent) bool { return live.Key() == a.Key() })
	if a.Session != "" {
		if m.dropped == nil {
			m.dropped = make(map[string]bool)
		}
		m.dropped[a.Session] = true
		m.issues = m.withoutDropped(m.issues)
	}
	return m.rebuild()
}

// withoutDropped is issues without the link labels of the dropped sessions on
// the open ones. A session no open issue carries is no longer dropped: the
// store's unlink has landed.
func (m *Model) withoutDropped(issues []domain.IssueSummary) []domain.IssueSummary {
	issues = slices.Clone(issues)
	for session := range m.dropped {
		label := agent.LinkLabelPrefix + session
		carried := false
		for i, issue := range issues {
			if issue.Status == domain.StatusClosed || !slices.Contains(issue.Labels, label) {
				continue
			}
			carried = true
			issues[i].Labels = slices.DeleteFunc(slices.Clone(issue.Labels), func(l string) bool { return l == label })
		}
		if !carried {
			delete(m.dropped, session)
		}
	}
	return issues
}

// Select puts the selection on the agent of session, now when its row is
// there and otherwise once a poll or a read brings it. It clears the query,
// which could hide that row.
func (m *Model) Select(session string) tea.Cmd {
	m.query.Clear()
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
		m.issues, m.issuesKnown = m.withoutDropped(msg.issues), true
		return m.rebuild()
	case readDueMsg:
		return m.readDue(msg)
	case messageReadMsg:
		return m.messageRead(msg)
	case mode.MouseMsg:
		return m.mouse(msg)
	case tea.PasteMsg:
		if !m.query.Paste(msg.Content) {
			return nil
		}
		return m.queryChanged()
	case tea.KeyPressMsg:
		if consumed, changed := m.query.HandleKey(msg); consumed {
			if !changed {
				return nil
			}
			return m.queryChanged()
		}
		return m.key(msg)
	}
	return nil
}

// queryChanged filters the rows again. The selection stays on its agent while
// it still matches, and otherwise takes the first match.
func (m *Model) queryChanged() tea.Cmd {
	return m.rebuildWith(true)
}

// matching is the rows the query matches: on the agent's title, label and
// activity, and the ID and title of its issue.
func (m *Model) matching(rows []uiagents.Row) []uiagents.Row {
	if m.query.Empty() {
		return rows
	}
	return slices.DeleteFunc(slices.Clone(rows), func(r uiagents.Row) bool {
		texts := []string{r.Agent.Title(), r.Agent.Label, r.Agent.Activity}
		if r.Issue != nil {
			texts = append(texts, r.Issue.ID, r.Issue.Title)
		}
		return !m.query.MatchesAny(texts...)
	})
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	match := func(action string) bool { return m.keys.Match(config.BoardContext, action, msg) }
	page := max(1, uiagents.InnerHeight(m.height)/uiagents.RowLines)
	if m.split() {
		// A page step keeps the lines the two scroll markers cover on screen.
		step := max(1, uiagents.InnerHeight(m.height)-2)
		switch {
		case m.keys.Match(config.DetailContext, config.DetailActionPageUp, msg):
			m.scrollPane(-step)
			return nil
		case m.keys.Match(config.DetailContext, config.DetailActionPageDown, msg):
			m.scrollPane(step)
			return nil
		}
	}
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
		// The shell owns the agent host, so it asks the host for the agents.
		return tea.Batch(m.Reload(), mode.RequestActionCmd(mode.Agents, mode.ActionPollAgents))
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
	onPane := uiagents.OnPane(m.viewState(), msg.X, msg.Y)
	switch msg.Kind {
	case mode.MouseWheelUp:
		if onPane {
			m.scrollPane(-paneWheelLines)
			return nil
		}
		return m.move(-1)
	case mode.MouseWheelDown:
		if onPane {
			m.scrollPane(paneWheelLines)
			return nil
		}
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
		Query:        m.query.Text(),
		Placeholder:  queryPlaceholder,
		Loaded:       len(m.all),
		Pane:         m.pane(),
	}
}

// pane is what the Agent pane shows of the selected agent's last message.
// A stale message stays on screen while the next one is read.
func (m *Model) pane() uiagents.Pane {
	pane := uiagents.Pane{ScrollOffset: m.paneOffset}
	row, ok := m.selectedRow()
	if !ok {
		return pane
	}
	key := row.Agent.Key()
	last, known := m.messages[key]
	switch {
	case !known && (m.due == key || m.reading[key]):
		pane.Reading = true
	case known && last.err != nil:
		pane.Error = last.err.Error()
	case known:
		pane.Message = last.message
	}
	return pane
}

func (m *Model) split() bool {
	_, _, split := styles.SplitWidths(m.width)
	return split
}

func (m *Model) scrollPane(delta int) {
	m.paneOffset = max(0, min(m.paneOffset+delta, uiagents.PaneMaxOffset(m.viewState())))
}

// readSoon starts the pause before the read of the selected agent's last
// message, when the pane is drawn and that message is not known or stale. A
// later call ends the pause before it.
func (m *Model) readSoon() tea.Cmd {
	m.readGeneration++
	m.due = ""
	row, ok := m.selectedRow()
	if !ok || !m.wantsRead(row.Agent) {
		return nil
	}
	m.due = row.Agent.Key()
	due := readDueMsg{key: m.due, generation: m.readGeneration}
	return tea.Tick(readPause, func(time.Time) tea.Msg { return due })
}

// wantsRead reports whether the last message of a is to be read: the pane is
// on screen, a is live, no read of it is in flight, and its message is not
// known, failed to read, or was read while a did something else.
func (m *Model) wantsRead(a agent.Agent) bool {
	if m.read == nil || !m.paneShown || a.Status == agent.Gone || m.reading[a.Key()] {
		return false
	}
	last, known := m.messages[a.Key()]
	return !known || last.err != nil || last.activity != a.Activity || last.status != a.Status
}

// readDue reads the selected agent's last message when the selection rested
// on it for the whole pause.
func (m *Model) readDue(msg readDueMsg) tea.Cmd {
	row, ok := m.selectedRow()
	if msg.generation != m.readGeneration {
		return nil
	}
	m.due = ""
	if !ok || row.Agent.Key() != msg.key || !m.wantsRead(row.Agent) {
		return nil
	}
	a, ctx, read := row.Agent, m.ctx, m.read
	m.reading[a.Key()] = true
	return func() tea.Msg {
		message, err := read(ctx, a)
		return messageReadMsg{key: a.Key(), activity: a.Activity, status: a.Status, message: message, err: err}
	}
}

func (m *Model) messageRead(msg messageReadMsg) tea.Cmd {
	delete(m.reading, msg.key)
	m.messages[msg.key] = lastMessage{activity: msg.activity, status: msg.status, message: msg.message, err: msg.err}
	m.paneOffset = min(m.paneOffset, uiagents.PaneMaxOffset(m.viewState()))
	if msg.err != nil {
		// The next poll or reload tries again: a retry from here would loop
		// on a host that keeps failing.
		m.logger.Debug("reading the last message of an agent failed", "error", msg.err.Error())
		return nil
	}
	// The agent may have moved on while it was read.
	return m.readSoon()
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
	m.paneOffset = 0
	return tea.Batch(m.selectionChangedCmd(), m.readSoon())
}

// rebuild works out the rows again and keeps the selection on the agent it
// was on: the one Select asked for, else the one chosen before. While the
// rows are not settled an agent without a row may still come, so neither
// key is given up; once they are settled, an agent without a row is gone
// from the tab and the selection stays on the row at its place.
func (m *Model) rebuild() tea.Cmd {
	return m.rebuildWith(false)
}

// rebuildWith is rebuild; with toFirst, an agent the rows no longer hold, as
// after a query change, gives the selection to the first row instead.
func (m *Model) rebuildWith(toFirst bool) tea.Cmd {
	before, beforeKey := m.selectedIssue(), m.selectedKey()
	m.all = buildRows(m.live, m.liveKnown, m.issues, m.commentLinks)
	m.rows = m.matching(m.all)
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
	} else if toFirst {
		m.selected = 0
	}
	m.selected = min(max(m.selected, 0), max(len(m.rows)-1, 0))
	// Before the rows settle their order is not final: the selection stays on
	// the first row, whichever agent comes there, unless the operator or
	// Select chose one.
	if m.settled() && (m.chosen == "" || rowOf(m.chosen) < 0) {
		m.chosen = m.selectedKey()
	}
	m.offset = uiagents.EnsureVisible(m.rows, m.selected, m.offset, m.height)
	if m.selectedKey() != beforeKey {
		m.paneOffset = 0
	}
	read := m.readSoon()
	// A read that changed the selected issue, its status say, announces it
	// too: the shell's issue keys and Ask read the selection.
	if reflect.DeepEqual(m.selectedIssue(), before) {
		return read
	}
	return tea.Batch(m.selectionChangedCmd(), read)
}

// settled reports whether the rows are final until the next poll or read:
// the live agents and the linked issues are both known, and no read is in
// flight.
func (m *Model) settled() bool {
	return m.liveKnown && m.issuesKnown && !m.loading
}

// SelectedRow is the selected row, for the legend: ok is false while the tab
// has no row.
func (m *Model) SelectedRow() (uiagents.Row, bool) {
	return m.selectedRow()
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

func (m *Model) selectedIssue() *domain.IssueSummary {
	if row, ok := m.selectedRow(); ok {
		return row.Issue
	}
	return nil
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

// buildRows is the list: first each agent linked to an issue, by the link
// labels of issues and the comments of commentLinks (agent.Link), then each
// live agent linked to none. Each group puts the agent that needs the
// operator most first (agent.ByNeed).
func buildRows(live []agent.Agent, liveKnown bool, issues []domain.IssueSummary, commentLinks []mode.CommentLink) []uiagents.Row {
	var bySession map[string]agent.Agent
	if liveKnown {
		bySession = make(map[string]agent.Agent, len(live))
		for _, a := range live {
			if a.Session != "" {
				bySession[a.Session] = a
			}
		}
	}
	targets := make([]domain.IssueSummary, 0, len(issues)+len(commentLinks))
	links := make([]agent.IssueLinks, 0, cap(targets))
	for _, issue := range issues {
		targets = append(targets, issue)
		links = append(links, agent.IssueLinks{Labels: issue.Labels, Closed: issue.Status == domain.StatusClosed})
	}
	for _, link := range commentLinks {
		targets = append(targets, link.Issue)
		links = append(links, agent.IssueLinks{CommentSessions: link.Sessions})
	}
	var rows []uiagents.Row
	seen := make(map[string]bool)
	for _, linked := range agent.Link(bySession, links) {
		seen[linked.Agent.Key()] = true
		issue := targets[linked.Issue]
		rows = append(rows, uiagents.Row{Agent: linked.Agent, Issue: &issue})
	}
	var unlinked []uiagents.Row
	for _, a := range live {
		if !seen[a.Key()] {
			unlinked = append(unlinked, uiagents.Row{Agent: a})
		}
	}
	slices.SortStableFunc(unlinked, func(x, y uiagents.Row) int { return agent.ByNeed(x.Agent, y.Agent) })
	return append(rows, unlinked...)
}
