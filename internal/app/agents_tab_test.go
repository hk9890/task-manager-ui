package app

// The Agents tab: every agent of the project, linked to an issue or not, and
// the keys that act on the selected one through the agent host.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func idleOnTab4() agent.Agent {
	return agent.Agent{Ref: "demo:home:4", Label: "Tab 4", Name: "scratch", Status: agent.Idle, Session: "s-4", Activity: "waiting"}
}

// agentsTab is the agent shell on the Agents tab, with s-1 on Tab 3 linked
// to tm-1 by its label and s-4 on Tab 4 linked to nothing.
func agentsTab(t *testing.T) (Model, *fakes.FakeAgentHost) {
	t.Helper()
	m, client, _, _ := agentShell(t, runningOnTab3(), idleOnTab4())
	m = withLinkLabel(t, m, "s-1")
	m = pressKey(t, m, "esc")
	m = pressKey(t, m, "shift+tab")
	if m.active != mode.Agents {
		t.Fatalf("fixture is on %q, want the Agents tab", m.active)
	}
	return m, client
}

// The tab lists the linked agents with their issue and the others apart,
// each with its status and what it does; the legend names its keys.
func TestTheAgentsTabListsTheLinkedAgentsAndTheOthers(t *testing.T) {
	m, _ := agentsTab(t)
	testui.AssertContainsAll(t, plainShell(m),
		"Board   Docs   Agents",
		"Linked (1)", "Tab 3  running", "tm-1  Ready first",
		"Not linked (1)", "scratch  idle · waiting", "Tab 4",
		"enter focus", "alt+x close", "alt+o issue",
	)
}

// Enter brings the selected agent to the front, and the issue key opens the
// issue it is linked to in Detail; Escape there returns to the tab.
func TestTheAgentsTabFocusesAnAgentAndOpensItsIssue(t *testing.T) {
	m, client := agentsTab(t)
	m = pressKey(t, m, "enter")
	if got := hostCalls(client, "Focus"); len(got) != 1 || got[0].Arg != "demo:home:3" {
		t.Fatalf("Focus calls = %+v, want one on Tab 3", got)
	}

	m = pressKey(t, m, "alt+o")
	if m.active != mode.Detail || m.detail.Detail.Summary.ID != "tm-1" {
		t.Fatalf("alt+o left the shell on %q showing %q, want Detail of tm-1", m.active, m.detail.Detail.Summary.ID)
	}
	m = pressKey(t, m, "esc")
	if m.active != mode.Agents {
		t.Errorf("esc from Detail went to %q, want the Agents tab", m.active)
	}

	m = pressKey(t, m, "down")
	m = pressKey(t, m, "alt+o")
	if m.active != mode.Agents || !strings.Contains(m.toast.View(), "No selected issue") {
		t.Errorf("alt+o on an agent linked to none: on %q, toast %q", m.active, m.toast.View())
	}
}

// The close key asks before it closes a running agent, and closes an idle
// one at once.
func TestTheAgentsTabClosesTheSelectedAgent(t *testing.T) {
	m, client := agentsTab(t)
	m = pressKey(t, m, "alt+x")
	testui.AssertContainsAll(t, plainShell(m), "Close agent", "Tab 3 is running. Close it anyway?")
	m = pressKey(t, m, "y")
	if got := labelsOf(t, m, "tm-1"); len(got) != 0 {
		t.Errorf("labels of tm-1 after the close = %v, want the link label removed", got)
	}

	m = pressKey(t, m, "down")
	m = pressKey(t, m, "alt+x")
	want := []fakes.AgentHostCall{{Method: "Close", Arg: "demo:home:3", Force: true}, {Method: "Close", Arg: "demo:home:4"}}
	if got := hostCalls(client, "Close"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Close calls = %+v, want %+v", got, want)
	}
	if !strings.Contains(m.toast.View(), "Closed scratch") {
		t.Errorf("toast = %q, want the close reported by the agent's name", m.toast.View())
	}
}

// A gone agent is resumed by Enter.
func TestTheAgentsTabResumesAGoneAgent(t *testing.T) {
	m, client := agentsTab(t)
	client.Resumed = agent.Agent{Ref: "demo:home:5", Session: "s-gone-0123", Label: "Tab 5", Status: agent.Idle}
	m = withLinkLabel(t, m, "s-gone-0123")
	m = send(t, m, agentPollTickMsg{})
	testui.AssertContainsAll(t, plainShell(m), "Linked (2)", "Session s-gone-0  gone")

	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")
	want := []fakes.AgentHostCall{{Method: "Resume", Arg: "s-gone-0123"}, {Method: "Focus", Arg: "demo:home:5"}}
	if got := hostCalls(client, "Resume", "Focus"); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %+v, want %+v", got, want)
	}
}

// Without an agent host the tab says so.
func TestTheAgentsTabWithoutAHostSaysWhy(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = pressKey(t, m, "shift+tab")
	testui.AssertContainsAll(t, plainShell(m), "No agents", "no agent host: agents are off")
}

// The close key on a gone agent unlinks it: no host call, the link label
// leaves the issue, and the row leaves the tab. The legend names the key
// close on a live agent and unlink on a gone one.
func TestTheAgentsTabUnlinksAGoneAgent(t *testing.T) {
	m, client := agentsTab(t)
	m = withLinkLabel(t, m, "s-gone-0123")
	m = send(t, m, agentPollTickMsg{})
	if footer := testui.Downsample(m.renderFooter()); !strings.Contains(footer, "alt+x close") {
		t.Errorf("legend on a live agent = %q, want alt+x close", footer)
	}
	m = pressKey(t, m, "down")
	if footer := testui.Downsample(m.renderFooter()); !strings.Contains(footer, "alt+x unlink") {
		t.Errorf("legend on a gone agent = %q, want alt+x unlink", footer)
	}
	m = pressKey(t, m, "alt+x")

	if got := hostCalls(client, "Close"); len(got) != 0 {
		t.Fatalf("the unlink asked the host to close %+v", got)
	}
	if got, want := labelsOf(t, m, "tm-1"), []string{"agent-session:s-1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("labels of tm-1 = %v, want %v", got, want)
	}
	if !strings.Contains(m.toast.View(), "Unlinked Session s-gone-0") {
		t.Errorf("toast = %q, want the unlink reported", m.toast.View())
	}
	m = pressKey(t, m, "alt+r")
	if view := plainShell(m); strings.Contains(view, "s-gone-0  gone") || !strings.Contains(view, "Linked (1)") {
		t.Errorf("the tab after the unlink:\n%s", view)
	}
}

// A poll under a Detail opened from the tab does not retarget it: not one
// that fails and empties the tab, and not one that moves its rows.
func TestAPollDoesNotRetargetTheDetailOpenedFromTheAgentsTab(t *testing.T) {
	m, client := agentsTab(t)
	m = pressKey(t, m, "alt+o")
	if m.active != mode.Detail || m.detail.Detail.Summary.ID != "tm-1" {
		t.Fatalf("alt+o left the shell on %q showing %q, want Detail of tm-1", m.active, m.detail.Detail.Summary.ID)
	}

	client.ListErr = errors.New("revier timed out")
	m = send(t, m, agentPollTickMsg{})
	if got := m.detail.Detail.Summary.ID; got != "tm-1" {
		t.Fatalf("Detail shows %q after a failed poll, want tm-1", got)
	}
	if sel := m.currentSelection(); sel == nil || sel.Issue.ID != "tm-1" {
		t.Fatalf("the selection after a failed poll is %+v, want tm-1", sel)
	}

	client.ListErr = nil
	client.Agents = []agent.Agent{idleOnTab4()}
	m = send(t, m, agentPollTickMsg{})
	if got := m.detail.Detail.Summary.ID; got != "tm-1" {
		t.Fatalf("Detail shows %q after the agent left, want tm-1", got)
	}
	m = pressKey(t, m, "esc")
	if m.active != mode.Agents {
		t.Errorf("esc from Detail went to %q, want the Agents tab", m.active)
	}
}
