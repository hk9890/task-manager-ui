package app

// A close or an unlink that succeeds takes the agent off the screen at once,
// before the reload drops its link label.

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/domain"
)

// dropLinkLabel removes the link label of session from tm-1 in the store, as
// a close or an unlink does before its result reaches the shell.
func dropLinkLabel(t *testing.T, m Model, session string) {
	t.Helper()
	if err := m.services.Repo.UpdateIssue(m.ctx, "tm-1", domain.UpdateIssueInput{RemoveLabels: []string{"agent-session:" + session}}); err != nil {
		t.Fatalf("fixture: %v", err)
	}
}

// A close that succeeds takes the agent off the Agents tab at once: the poll
// that no longer lists it draws no gone row before the reload, which Enter
// would resume.
func TestAClosedAgentIsNotDrawnGoneOnTheAgentsTab(t *testing.T) {
	m, client := agentsTab(t)
	client.Agents = []agent.Agent{idleOnTab4()}
	dropLinkLabel(t, m, "s-1")

	m = updateOnly(m, agentClosedMsg{epoch: m.storeEpoch, agent: runningOnTab3(), unlinked: []string{"tm-1"}})
	m = updateOnly(m, agentsListedMsg{agents: client.Agents})
	if view := plainShell(m); strings.Contains(view, "  gone") || strings.Contains(view, "Tab 3  running") {
		t.Errorf("the Agents tab draws the closed agent:\n%s", view)
	}
	if row, _ := m.agents.SelectedRow(); row.Agent.Session == "s-1" {
		t.Errorf("the selection is on the closed agent %+v", row.Agent)
	}
}

// A close that succeeds takes the agent off the Details pane at once.
func TestAClosedAgentIsNotDrawnGoneInTheDetailsPane(t *testing.T) {
	idle := agent.Agent{Ref: "demo:home:3", Label: "Tab 3", Status: agent.Idle, Session: "s-3"}
	m, client, _, _ := agentShell(t, idle)
	m = withLinkLabel(t, m, "s-3")
	client.Agents = nil
	dropLinkLabel(t, m, "s-3")

	m = updateOnly(m, agentClosedMsg{epoch: m.storeEpoch, agent: idle, unlinked: []string{"tm-1"}})
	if view := plainShell(m); strings.Contains(view, "idle  Tab 3") {
		t.Errorf("the Details pane draws the closed agent:\n%s", view)
	}
	m = updateOnly(m, agentsListedMsg{})
	if view := plainShell(m); strings.Contains(view, "gone  ") || strings.Contains(view, "  gone") {
		t.Errorf("the Details pane draws the closed agent gone:\n%s", view)
	}
}

// An unlink of a gone agent takes its row off the Agents tab at once.
func TestAnUnlinkedAgentLeavesTheAgentsTabBeforeTheReload(t *testing.T) {
	m, _ := withGoneRow(t)
	gone := agent.GoneAgent("s-gone-0123")
	dropLinkLabel(t, m, gone.Session)

	m = updateOnly(m, agentUnlinkedMsg{agent: gone, unlinked: []string{"tm-1"}})
	if view := plainShell(m); strings.Contains(view, "gone  ") || strings.Contains(view, "  gone") {
		t.Errorf("the Agents tab draws the unlinked agent:\n%s", view)
	}
}
