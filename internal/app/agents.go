package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/revier"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// armAgentPoll starts the agent poll timer, or returns nil when there is no
// revier client or no background work is wanted. The timer is not scoped: it
// re-arms only from its own handler, so a dropped tick would stop the chain
// for the rest of the session.
func (m Model) armAgentPoll() tea.Cmd {
	if m.services.Revier == nil || m.runtime.DisableAutoRefresh {
		return nil
	}
	return m.scheduleAgentPoll()
}

// pollAgentsCmd asks revier for the live agents of the store's project. It
// starts nothing while a poll is in flight, without a revier client, and
// without a project path to ask about. The project name is resolved once per
// store and cached; a failed lookup runs again only after projectRetryTicks
// ticks, or on the next store bind.
func (m *Model) pollAgentsCmd() tea.Cmd {
	client := m.services.Revier
	root := m.services.ProjectRoot
	if client == nil || m.agentPollInFlight || !m.storeOpen || root == "" || m.projectRootMissing {
		return nil
	}
	if m.projectLookupWait > 0 {
		m.projectLookupWait--
		return nil
	}
	m.agentPollInFlight = true
	ctx, project := m.ctx, m.revierProject
	return m.scoped(func() tea.Msg {
		return listAgents(ctx, client, root, project)
	})
}

func listAgents(ctx context.Context, client revier.Client, root, project string) agentsListedMsg {
	if project == "" {
		resolved, err := client.Project(ctx, root)
		if err != nil {
			return agentsListedMsg{err: err}
		}
		project = resolved
	}
	agents, err := client.Agents(ctx, project)
	return agentsListedMsg{project: project, agents: agents, err: err}
}

// applyAgentList hands the listed agents to Detail and their statuses to the
// board, both keyed by session. A failed
// poll clears them, and is logged once until a poll succeeds again: the poll
// runs every few seconds.
func (m *Model) applyAgentList(msg agentsListedMsg) {
	m.agentPollInFlight = false
	m.revierProject = msg.project
	if msg.err != nil {
		if msg.project == "" {
			m.projectLookupWait = projectRetryTicks
		}
		m.detail.SetLiveAgents(nil)
		m.board.SetLiveAgents(nil)
		if !m.agentPollFailing {
			m.logAgentPollFailure(msg)
		}
		m.agentPollFailing = true
		return
	}
	m.agentPollFailing = false

	bySession := make(map[string]revier.Agent, len(msg.agents))
	statusBySession := make(map[string]string, len(msg.agents))
	for _, agent := range msg.agents {
		if agent.Session != "" {
			bySession[agent.Session] = agent
			statusBySession[agent.Session] = agent.Status
		}
	}
	m.detail.SetLiveAgents(bySession)
	m.board.SetLiveAgents(statusBySession)
}

func (m Model) logAgentPollFailure(msg agentsListedMsg) {
	if msg.project == "" {
		// Not a fault: most projects are not revier projects.
		m.logger().Debug("project is not a revier project; agent status not shown",
			"project_root", m.services.ProjectRoot, "error", msg.err.Error())
		return
	}
	m.logger().Warn("revier agent listing failed; agent status not shown",
		"project", msg.project, "error", msg.err.Error())
}

// focusAgent brings the first live agent linked to the issue open in Detail
// to the front: the one that needs the operator most.
func (m *Model) focusAgent() tea.Cmd {
	linked := m.detail.LinkedAgents()
	if len(linked) == 0 || m.services.Revier == nil {
		return m.showToast("No live agent on this issue", toaster.StyleWarn)
	}
	ctx, client := m.ctx, m.services.Revier
	addr := revier.Address(m.revierProject, linked[0].Panel)
	return func() tea.Msg {
		return agentFocusedMsg{err: client.Focus(ctx, addr)}
	}
}
