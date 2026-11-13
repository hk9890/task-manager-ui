package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/launcher"
	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/ui/selectmenu"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// openCentredMenu puts menu on screen, in the middle of it, with the cursor
// on its first entry that is on.
func (m *Model) openCentredMenu(menu *selectionMenu) {
	menu.cursorToFirst()
	width, height := selectmenu.Size(menu.state())
	menu.at.x, menu.at.y = max((m.width-width)/2, 0), max((m.height-height)/2, 0)
	m.selMenu = menu
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
		return agentStartedMsg(startAndLink(host, repo, issueID, spec))
	})
	return batchCmds(starting, start)
}

// startAndLink starts an agent on spec and writes the label of its session on
// the issue. It runs on no store's context: a store switch must not stop an
// agent half started, and the host bounds every call itself. An agent the
// host returns with a session is linked even when a later step of its start
// failed: it is open and may work on the issue.
func startAndLink(host agent.Host, repo repository.Repository, issueID string, spec agent.Spec) agentStart {
	started, err := host.Start(context.Background(), spec)
	result := agentStart{issueID: issueID, agent: started, err: err}
	if started.Session == "" {
		if err == nil {
			result.linkErr = errors.New("the agent host reported no session")
		}
		return result
	}
	label, err := agent.LinkLabel(started.Session)
	if err != nil {
		result.linkErr = err
		return result
	}
	result.linkErr = repo.UpdateIssue(context.Background(), issueID, domain.UpdateIssueInput{
		AddLabels: []string{label},
	})
	return result
}

// handleAgentStarted reports the start to the operator in one toast, a
// failed start and a failed link both, and reloads the issue the label was
// written to.
func (m *Model) handleAgentStarted(msg agentStartedMsg) tea.Cmd {
	issueID := displaytext.OneLine(msg.issueID)
	if msg.err != nil && msg.agent.Session == "" {
		return m.showToast(fmt.Sprintf("Could not start an agent on %s: %v", issueID, msg.err), toaster.StyleError)
	}
	label := displaytext.OneLine(msg.agent.Label)
	switch {
	case msg.linkErr != nil && msg.err != nil:
		return m.showToast(fmt.Sprintf("Started %s on %s, but: %v; and could not link it: %v", label, issueID, msg.err, msg.linkErr), toaster.StyleError)
	case msg.linkErr != nil:
		return m.showToast(fmt.Sprintf("Started %s, but could not link it to %s: %v", label, issueID, msg.linkErr), toaster.StyleError)
	case msg.err != nil:
		return batchCmds(m.reloadAfterLink(msg.issueID), m.showToast(fmt.Sprintf("Started %s on %s, but: %v", label, issueID, msg.err), toaster.StyleError))
	default:
		return batchCmds(m.reloadAfterLink(msg.issueID), m.showToast(fmt.Sprintf("Started %s on %s", label, issueID), toaster.StyleSuccess))
	}
}

// reloadAfterLink reloads the browse surfaces, and Detail when it shows an
// issue a link label was written to or removed from, on a store that is not
// watched: on a watched one the watch reloads after a write.
func (m *Model) reloadAfterLink(issueIDs ...string) tea.Cmd {
	if m.storeWatched() {
		return nil
	}
	m.markBrowseSurfacesDirty()
	reload := m.maybeAutoRefreshActiveSurfaceCmd()
	if issueID := m.detail.Detail.Summary.ID; slices.Contains(issueIDs, issueID) {
		reload = batchCmds(reload, m.reloadDetailAfterMutationCmd(issueID))
	}
	return reload
}
