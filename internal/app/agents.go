package app

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
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

// applyAgentList hands the listed agents to Detail and their statuses to the
// board, both keyed by session. A failed poll clears them, and is logged once
// until a poll succeeds again: the poll runs every few seconds.
func (m *Model) applyAgentList(msg agentsListedMsg) {
	m.agentPollInFlight = false
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
		return
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

// focusAgent brings the first live agent linked to the issue open in Detail
// to the front: the one that needs the operator most.
func (m *Model) focusAgent() tea.Cmd {
	linked := m.detail.LinkedAgents()
	if len(linked) == 0 || m.services.Agents == nil {
		return m.showToast("No live agent on this issue", toaster.StyleWarn)
	}
	ctx, host := m.ctx, m.services.Agents
	return func() tea.Msg {
		return agentFocusedMsg{err: host.Focus(ctx, linked[0])}
	}
}
