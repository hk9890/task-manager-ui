package app

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/launcher"
	"github.com/hk9890/task-manager-ui/internal/ui/selectmenu"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// openIntentMenu opens the menu of the configured intents for the issue open
// in Detail, in the middle of the screen. The issue is taken when the menu
// opens, so a refresh under it cannot move the agent to another issue.
func (m *Model) openIntentMenu() tea.Cmd {
	issue, ok := m.selectedIssueContext()
	if !ok {
		return m.showToast("No selected issue to start an agent on", toaster.StyleWarn)
	}
	if m.services.Agents == nil {
		return m.showToast("No agent host: agents are off", toaster.StyleWarn)
	}
	intents := m.services.Config.Agent.Intents
	if len(intents) == 0 {
		return m.showToast("No agent intents configured", toaster.StyleWarn)
	}
	m.projectRootMissing = projectRootMissing(m.services.ProjectRoot)
	if m.projectRootMissing {
		return m.showToast("Agents are off: project path not accessible: "+m.services.ProjectRoot, toaster.StyleWarn)
	}

	menu := &selectionMenu{
		title:       "Start an agent on " + displaytext.OneLine(issue.Summary.ID),
		hint:        "it opens in front",
		backdrop:    m.frozenScreen(),
		screenWidth: m.width,
	}
	for _, intent := range intents {
		menu.entries = append(menu.entries, selectionAction{
			entry: selectmenu.Entry{Label: displaytext.OneLine(intent.Name)},
			run:   func(m *Model) tea.Cmd { return m.startAgent(issue, intent) },
		})
	}
	menu.entries = append(menu.entries, m.closeMenuAction())
	width, height := selectmenu.Size(menu.state())
	menu.at.x, menu.at.y = max((m.width-width)/2, 0), max((m.height-height)/2, 0)
	m.selMenu = menu
	return nil
}

// startAgent starts an agent in front on the issue, prompted for intent, and
// links it to the issue with the label of its session. The host returns once
// the session is known, which takes as long as the agent takes to start.
func (m *Model) startAgent(issue domain.IssueDetail, intent config.Intent) tea.Cmd {
	host, repo, root := m.services.Agents, m.services.Repo, m.services.ProjectRoot
	spec := agent.Spec{Root: root, Prompt: launcher.Interpolate(intent.Prompt, issue, root), Focus: true}
	issueID := issue.Summary.ID
	starting := m.showToast(fmt.Sprintf("Starting an agent to %s %s…", displaytext.OneLine(intent.Name), displaytext.OneLine(issueID)), toaster.StyleInfo)
	// Not the store's context: a store switch must not stop an agent half
	// started. The host bounds every call itself.
	start := m.scoped(func() tea.Msg {
		started, err := host.Start(context.Background(), spec)
		if err != nil {
			return agentStartedMsg{issueID: issueID, err: err}
		}
		if started.Session == "" {
			return agentStartedMsg{issueID: issueID, agent: started, linkErr: errors.New("the agent host reported no session")}
		}
		linkErr := repo.UpdateIssue(context.Background(), issueID, domain.UpdateIssueInput{
			AddLabels: []string{agent.LinkLabel(started.Session)},
		})
		return agentStartedMsg{issueID: issueID, agent: started, linkErr: linkErr}
	})
	return batchCmds(starting, start)
}

// handleAgentStarted reports the start to the operator and, on a store that
// is not watched, reloads the issue the label was written to.
func (m *Model) handleAgentStarted(msg agentStartedMsg) tea.Cmd {
	issueID := displaytext.OneLine(msg.issueID)
	if msg.err != nil {
		return m.showToast(fmt.Sprintf("Could not start an agent on %s: %v", issueID, msg.err), toaster.StyleError)
	}
	label := displaytext.OneLine(msg.agent.Label)
	if msg.linkErr != nil {
		return m.showToast(fmt.Sprintf("Started %s, but could not link it to %s: %v", label, issueID, msg.linkErr), toaster.StyleError)
	}
	done := m.showToast(fmt.Sprintf("Started %s on %s", label, issueID), toaster.StyleSuccess)
	// On a watched store the watch reloads after a write.
	if m.storeWatched() {
		return done
	}
	m.markBrowseSurfacesDirty()
	return batchCmds(m.reloadDetailAfterMutationCmd(msg.issueID), m.maybeAutoRefreshActiveSurfaceCmd(), done)
}
