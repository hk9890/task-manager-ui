package app

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
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
	first, live := agent.FirstLive(m.detail.LinkedAgents())
	if !live || m.services.Agents == nil {
		return m.showToast("No live agent on this issue", toaster.StyleWarn)
	}
	return m.focusOneAgent(first)
}

func (m *Model) focusOneAgent(a agent.Agent) tea.Cmd {
	ctx, host := m.ctx, m.services.Agents
	return func() tea.Msg {
		return agentFocusedMsg{err: host.Focus(ctx, a)}
	}
}

// openAgentMenu opens the menu of the agents linked to the issue open in
// Detail, in the middle of the screen: focus or close each live one, resume
// each gone one. The agents are taken when the menu opens.
func (m *Model) openAgentMenu() tea.Cmd {
	if m.services.Agents == nil {
		return m.showToast("No agent host: agents are off", toaster.StyleWarn)
	}
	linked := m.detail.LinkedAgents()
	if len(linked) == 0 {
		return m.showToast("No agent on this issue", toaster.StyleWarn)
	}
	menu := &selectionMenu{
		title:       "Agents on " + displaytext.OneLine(m.detail.Detail.Summary.ID),
		backdrop:    m.frozenScreen(),
		screenWidth: m.width,
	}
	for _, a := range linked {
		label := displaytext.OneLine(a.Label)
		if a.Status == agent.Gone {
			menu.entries = append(menu.entries, selectionAction{
				entry: selectmenu.Entry{Label: "Resume " + label},
				run:   func(m *Model) tea.Cmd { return m.resumeAgent(a) },
			})
			continue
		}
		menu.entries = append(menu.entries,
			selectionAction{
				entry: selectmenu.Entry{Label: "Focus " + label},
				run:   func(m *Model) tea.Cmd { return m.focusOneAgent(a) },
			},
			selectionAction{
				entry: selectmenu.Entry{Label: "Close " + label},
				run:   func(m *Model) tea.Cmd { return m.requestAgentClose(a) },
			})
	}
	menu.entries = append(menu.entries, m.closeMenuAction())
	m.openCentredMenu(menu)
	return nil
}

// resumeAgent reopens the conversation of a gone agent in the store's
// project and brings it to the front.
func (m *Model) resumeAgent(gone agent.Agent) tea.Cmd {
	ctx, host, root := m.ctx, m.services.Agents, m.services.ProjectRoot
	return func() tea.Msg {
		resumed, err := host.Resume(ctx, root, gone.Session)
		if err == nil {
			err = host.Focus(ctx, resumed)
		}
		return agentResumedMsg{agent: gone, err: err}
	}
}

// requestAgentClose closes an idle agent at once. It asks first before it
// closes one that is running or waits for the operator: the host refuses
// those unless forced, and the turn in them is lost.
func (m *Model) requestAgentClose(a agent.Agent) tea.Cmd {
	if a.Status != agent.Running && a.Status != agent.NeedsYou {
		return m.closeAgent(a, false)
	}
	m.agentClose = &a
	m.actionModal = modal.NewWithKeys(modal.Config{
		Title:          "Close agent",
		Message:        displaytext.OneLine(fmt.Sprintf("%s is %s. Close it anyway?", a.Label, agentStatusWord(a.Status))),
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

func (m *Model) closeAgent(a agent.Agent, force bool) tea.Cmd {
	ctx, host := m.ctx, m.services.Agents
	return func() tea.Msg {
		return agentClosedMsg{agent: a, err: host.Close(ctx, a, force)}
	}
}

// reportAgentAction reports a resume or a close of a, and on success polls
// at once, so the Agents group shows the change before the next tick.
func (m *Model) reportAgentAction(verb, past string, a agent.Agent, err error) tea.Cmd {
	label := displaytext.OneLine(a.Label)
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
