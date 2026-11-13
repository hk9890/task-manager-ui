package app

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
	"github.com/hk9890/task-manager-ui/internal/ui/selectmenu"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// armAgentPoll starts the agent poll timer, or returns nil when there is no
// agent host or no background work is wanted. The timer is not scoped: it
// re-arms only from its own handler, so a dropped tick would stop the chain
// for the rest of the session.
func (m Model) armAgentPoll() tea.Cmd {
	if m.services.Agents == nil || m.runtime.DisableAutoRefresh {
		return nil
	}
	return m.scheduleAgentPoll()
}

// pollAgentsCmd asks the agent host for the live agents of the store's
// project. It starts nothing while a poll is in flight, without a host, and
// without a project path to ask about. After the host said the root is no
// project of its own, the next poll runs only after projectRetryTicks ticks,
// or on the next store bind.
func (m *Model) pollAgentsCmd() tea.Cmd {
	host := m.services.Agents
	root := m.services.ProjectRoot
	if host == nil || m.agentPollInFlight || !m.storeOpen || root == "" || m.projectRootMissing {
		return nil
	}
	if m.projectLookupWait > 0 {
		m.projectLookupWait--
		return nil
	}
	m.agentPollInFlight = true
	ctx := m.ctx
	return m.scoped(func() tea.Msg {
		agents, err := host.List(ctx, root)
		return agentsListedMsg{agents: agents, err: err}
	})
}

// applyAgentList hands the listed agents to Detail and the Agents tab, and
// their statuses to the board, keyed by session. A failed poll clears them,
// and is logged once until a poll succeeds again: the poll runs every few
// seconds. The Agents tab takes the board's comment links with each poll.
func (m *Model) applyAgentList(msg agentsListedMsg) tea.Cmd {
	m.agentPollInFlight = false
	linksCmd := m.agents.SetCommentLinks(m.board.CommentLinks())
	if msg.err != nil {
		if errors.Is(msg.err, agent.ErrNoProject) {
			m.projectLookupWait = projectRetryTicks
		}
		m.detail.SetLiveAgents(nil)
		m.board.SetLiveAgents(nil)
		if !m.agentPollFailing {
			m.logAgentPollFailure(msg.err)
		}
		m.agentPollFailing = true
		return m.scoped(batchCmds(linksCmd, m.agents.SetAgents(nil, false, m.agentsAbsent(msg.err))))
	}
	m.agentPollFailing = false

	bySession := make(map[string]agent.Agent, len(msg.agents))
	statusBySession := make(map[string]agent.Status, len(msg.agents))
	for _, live := range msg.agents {
		if live.Session != "" {
			bySession[live.Session] = live
			statusBySession[live.Session] = live.Status
		}
	}
	m.detail.SetLiveAgents(bySession)
	m.board.SetLiveAgents(statusBySession)
	return m.scoped(batchCmds(linksCmd, m.agents.SetAgents(msg.agents, true, "")))
}

// agentsAbsent is why the Agents tab may show no agent: no host, no project
// path to ask about, or the error the last poll failed with; "" when nothing
// keeps the host from answering.
func (m Model) agentsAbsent(pollErr error) string {
	root := m.services.ProjectRoot
	switch {
	case m.services.Agents == nil:
		return "no agent host: agents are off"
	case root == "":
		return "the store has no project path to ask the agent host about"
	case m.projectRootMissing:
		return "the project path is not accessible: " + root
	case errors.Is(pollErr, agent.ErrNoProject):
		return root + " is no project of the agent host"
	case pollErr != nil:
		return "the agent host failed: " + pollErr.Error()
	}
	return ""
}

// agentsOff says why no agent can start in the store's project: no host, no
// project path, or a path that is not accessible; "" when one can.
func (m *Model) agentsOff() string {
	root := m.services.ProjectRoot
	m.projectRootMissing = projectRootMissing(root)
	switch {
	case m.services.Agents == nil:
		return "No agent host: agents are off"
	case root == "":
		return "Agents are off: the store has no project path"
	case m.projectRootMissing:
		return "Agents are off: project path not accessible: " + root
	}
	return ""
}

// selectAgent switches to the Agents tab with the agent of session selected.
// The click on its row in the Details pane asks for it.
func (m *Model) selectAgent(session string) tea.Cmd {
	switchCmd := m.switchToTab(mode.Agents)
	return batchCmds(switchCmd, m.scoped(m.agents.Select(session)))
}

func (m Model) logAgentPollFailure(err error) {
	if errors.Is(err, agent.ErrNoProject) {
		// Not a fault: most projects are not revier projects.
		m.logger().Debug("project is not a revier project; agent status not shown",
			"project_root", m.services.ProjectRoot, "error", err.Error())
		return
	}
	m.logger().Warn("revier agent listing failed; agent status not shown",
		"project_root", m.services.ProjectRoot, "error", err.Error())
}

func (m *Model) focusOneAgent(a agent.Agent) tea.Cmd {
	ctx, host := m.ctx, m.services.Agents
	return func() tea.Msg {
		return agentFocusedMsg{err: host.Focus(ctx, a)}
	}
}

// openAgent brings a to the front: a live one at once, a gone one after its
// resume. Enter on its row of the Agents tab asks for it.
func (m *Model) openAgent(a agent.Agent) tea.Cmd {
	if m.services.Agents == nil {
		return m.showToast("No agent host: agents are off", toaster.StyleWarn)
	}
	if a.Status == agent.Gone {
		return m.resumeAgent(a)
	}
	return m.focusOneAgent(a)
}

// openAgentDialog opens the agent dialog of the issue open in Detail, in the
// middle of the screen: first the agents linked to it, focus or close each
// live one and resume or unlink each gone one, then one entry per configured
// intent that starts a new agent, then Cancel. The issue and its agents are taken
// when the dialog opens, so a refresh under it cannot move an agent to
// another issue.
func (m *Model) openAgentDialog() tea.Cmd {
	issue, ok := m.selectedIssueContext()
	if !ok {
		return m.showToast("No selected issue to start an agent on", toaster.StyleWarn)
	}
	if off := m.agentsOff(); off != "" {
		return m.showToast(off, toaster.StyleWarn)
	}
	linked := m.detail.LinkedAgents()
	intents := m.services.Config.Agent.Intents
	if len(linked) == 0 && len(intents) == 0 {
		return m.showToast("No agent on this issue and no agent intents configured", toaster.StyleWarn)
	}

	menu := &selectionMenu{
		title:       "Agents on " + displaytext.OneLine(issue.Summary.ID),
		backdrop:    m.frozenScreen(),
		screenWidth: m.width,
	}
	for _, a := range linked {
		menu.entries = append(menu.entries, linkedAgentActions(a)...)
	}
	if len(intents) > 0 {
		heading, off := "Start new", issue.Summary.Status == domain.StatusClosed
		if off {
			heading = "Start new: off, the issue is closed"
		}
		menu.entries = append(menu.entries, selectionAction{entry: selectmenu.Entry{Label: heading, Heading: true}})
		for _, intent := range intents {
			menu.entries = append(menu.entries, selectionAction{
				entry: selectmenu.Entry{Label: displaytext.OneLine(intent.Name), Disabled: off},
				run:   func(m *Model) tea.Cmd { return m.startAgent(issue, intent) },
			})
		}
	}
	menu.entries = append(menu.entries, m.closeMenuAction("Cancel"))
	m.openCentredMenu(menu)
	return nil
}

// linkedAgentActions are the entries of one linked agent in the agent
// dialog: Focus and Close for a live one, Resume and Unlink for a gone one.
func linkedAgentActions(a agent.Agent) []selectionAction {
	label := displaytext.OneLine(a.Title())
	if a.Status == agent.Gone {
		return []selectionAction{
			{
				entry: selectmenu.Entry{Label: "Resume " + label},
				run:   func(m *Model) tea.Cmd { return m.resumeAgent(a) },
			},
			{
				entry: selectmenu.Entry{Label: "Unlink " + label},
				run:   func(m *Model) tea.Cmd { return m.unlinkGoneAgent(a) },
			},
		}
	}
	return []selectionAction{
		{
			entry: selectmenu.Entry{Label: "Focus " + label},
			run:   func(m *Model) tea.Cmd { return m.focusOneAgent(a) },
		},
		{
			entry: selectmenu.Entry{Label: "Close " + label},
			run:   func(m *Model) tea.Cmd { return m.requestAgentClose(a) },
		},
	}
}

// resumeAgent reopens the conversation of a gone agent in the store's
// project and brings it to the front. The harness of the new tab takes
// seconds to come up, so an info toast says the resume started, and a second
// resume of the session while one is in flight starts nothing: it would open
// a second tab on the same conversation.
func (m *Model) resumeAgent(gone agent.Agent) tea.Cmd {
	label := displaytext.OneLine(gone.Title())
	if m.agentResumes[gone.Session] {
		return m.showToast("Already resuming "+label+"…", toaster.StyleInfo)
	}
	if m.agentResumes == nil {
		m.agentResumes = make(map[string]bool)
	}
	m.agentResumes[gone.Session] = true
	ctx, host, root := m.ctx, m.services.Agents, m.services.ProjectRoot
	resume := func() tea.Msg {
		resumed, err := host.Resume(ctx, root, gone.Session)
		if err == nil {
			err = host.Focus(ctx, resumed)
		}
		return agentResumedMsg{agent: gone, err: err}
	}
	return batchCmds(m.showToast("Resuming "+label+"…", toaster.StyleInfo), resume)
}

// requestAgentClose closes an idle agent at once. It asks first before it
// closes one that is running or waits for the operator: the host refuses
// those unless forced, and the turn in them is lost. A gone agent has
// nothing to close, so it is unlinked.
func (m *Model) requestAgentClose(a agent.Agent) tea.Cmd {
	switch {
	case m.services.Agents == nil:
		return m.showToast("No agent host: agents are off", toaster.StyleWarn)
	case a.Status == agent.Gone:
		return m.unlinkGoneAgent(a)
	}
	if a.Status != agent.Running && a.Status != agent.NeedsYou {
		return m.closeAgent(a, false)
	}
	m.agentClose = &a
	m.actionModal = modal.NewWithKeys(modal.Config{
		Title:          "Close agent",
		Message:        displaytext.OneLine(fmt.Sprintf("%s is %s. Close it anyway?", a.Title(), agentStatusWord(a.Status))),
		ConfirmText:    "Close",
		ConfirmVariant: modal.ButtonDanger,
	}, modal.BindingsFromConfig(m.keys))
	m.actionModal.SetSize(m.width, m.height)
	m.showActionModal = true
	return m.actionModal.Init()
}

// confirmAgentClose closes the close question and force-closes the agent it
// was about.
func (m *Model) confirmAgentClose() tea.Cmd {
	a := *m.agentClose
	m.showActionModal = false
	m.agentClose = nil
	return m.closeAgent(a, true)
}

// closeAgent closes a, and then unlinks it: it removes the link label of its
// session from every open issue that carries it. The store refuses every
// change to a closed issue, so a closed one keeps its label.
func (m *Model) closeAgent(a agent.Agent, force bool) tea.Cmd {
	ctx, host, repo := m.ctx, m.services.Agents, m.services.Repo
	return m.scoped(func() tea.Msg {
		if err := host.Close(ctx, a, force); err != nil {
			return agentClosedMsg{agent: a, err: err}
		}
		unlinked, err := unlinkAgent(ctx, repo, a.Session)
		return agentClosedMsg{agent: a, unlinked: unlinked, unlinkErr: err}
	})
}

// unlinkGoneAgent removes the link label of a gone agent from the open
// issues that carry it, so it is no gone row afterwards.
func (m *Model) unlinkGoneAgent(a agent.Agent) tea.Cmd {
	ctx, repo := m.ctx, m.services.Repo
	return m.scoped(func() tea.Msg {
		unlinked, err := unlinkAgent(ctx, repo, a.Session)
		return agentUnlinkedMsg{agent: a, unlinked: unlinked, err: err}
	})
}

// handleAgentUnlinked reports an unlink of a gone agent.
func (m *Model) handleAgentUnlinked(msg agentUnlinkedMsg) tea.Cmd {
	var reload tea.Cmd
	if len(msg.unlinked) > 0 {
		reload = m.reloadAfterLink(msg.unlinked...)
	}
	label := displaytext.OneLine(msg.agent.Title())
	if msg.err != nil {
		return batchCmds(reload, m.showToast(fmt.Sprintf("Could not unlink %s: %s", label, displaytext.OneLine(msg.err.Error())), toaster.StyleError))
	}
	return batchCmds(reload, m.dropAgent(msg.agent), m.showToast("Unlinked "+label, toaster.StyleSuccess))
}

// dropAgent takes an agent the app closed and unlinked off the Details pane
// and the Agents tab at once: until the reload lands, its link label would
// draw it gone, and Enter on that row would resume it.
func (m *Model) dropAgent(a agent.Agent) tea.Cmd {
	m.detail.DropAgent(a)
	return m.scoped(m.agents.DropAgent(a))
}

// unlinkAgent removes the link label of session from the open issues that
// carry it, and returns the IDs of the issues it was removed from.
func unlinkAgent(ctx context.Context, repo repository.Repository, session string) ([]string, error) {
	label, err := agent.LinkLabel(session)
	if err != nil {
		// No issue carries a label the store refuses: the agent was never linked.
		return nil, nil
	}
	page, err := repo.Search(ctx, domain.SearchIssuesQuery{Labels: []string{label}})
	if err != nil {
		return nil, err
	}
	var unlinked []string
	for _, result := range page.Results {
		id := result.Issue.ID
		if err := repo.UpdateIssue(ctx, id, domain.UpdateIssueInput{RemoveLabels: []string{label}}); err != nil {
			return unlinked, fmt.Errorf("%s: %w", id, err)
		}
		unlinked = append(unlinked, id)
	}
	return unlinked, nil
}

// handleAgentClosed reports a close. An agent closed but not unlinked is
// reported as an error: its issue shows it gone.
func (m *Model) handleAgentClosed(msg agentClosedMsg) tea.Cmd {
	var reload tea.Cmd
	if len(msg.unlinked) > 0 {
		reload = m.reloadAfterLink(msg.unlinked...)
	}
	if msg.unlinkErr != nil {
		return batchCmds(reload, m.pollAgentsCmd(), m.showToast(fmt.Sprintf("Closed %s, but could not unlink it from %s", displaytext.OneLine(msg.agent.Title()), displaytext.OneLine(msg.unlinkErr.Error())), toaster.StyleError))
	}
	if msg.err != nil {
		return batchCmds(reload, m.reportAgentAction("close", "Closed", msg.agent, msg.err))
	}
	return batchCmds(reload, m.dropAgent(msg.agent), m.reportAgentAction("close", "Closed", msg.agent, nil))
}

// reportAgentAction reports a resume or a close of a, and on success polls
// at once, so the Agents group shows the change before the next tick.
func (m *Model) reportAgentAction(verb, past string, a agent.Agent, err error) tea.Cmd {
	label := displaytext.OneLine(a.Title())
	if err != nil {
		return m.showToast(fmt.Sprintf("Could not %s %s: %v", verb, label, err), toaster.StyleError)
	}
	return batchCmds(m.showToast(past+" "+label, toaster.StyleSuccess), m.pollAgentsCmd())
}

// agentStatusWord is what the close question says for an agent status.
func agentStatusWord(status agent.Status) string {
	if status == agent.NeedsYou {
		return "waiting for you"
	}
	return string(status)
}
