package app

// The agents of the issue in Detail: an agent the intent menu started shows in
// the Agents group through its link label, and the agent menu focuses, closes
// and resumes them.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// withLinkLabel writes the link label of session on tm-1 and reads Detail
// again, as a start on a store that is not watched does.
func withLinkLabel(t *testing.T, m Model, session string) Model {
	t.Helper()
	if err := m.services.Repo.UpdateIssue(m.ctx, "tm-1", domain.UpdateIssueInput{AddLabels: []string{"agent:" + session}}); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return pressKey(t, m, "alt+r")
}

func hostCalls(client *fakes.FakeAgentHost, methods ...string) []fakes.AgentHostCall {
	var calls []fakes.AgentHostCall
	for _, call := range client.Calls() {
		for _, method := range methods {
			if call.Method == method {
				calls = append(calls, call)
			}
		}
	}
	return calls
}

// An agent the intent menu starts is in the Agents group after the next
// poll, linked by its label alone: it wrote no comment. The label is not drawn.
func TestAStartedAgentShowsInTheAgentsGroupByItsLabel(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = agent.Agent{Ref: "demo:home:9", Session: "s-9", Label: "Tab 9", Status: agent.Running, Activity: "reading"}

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "enter")
	m = send(t, m, agentPollTickMsg{})

	view := plainShell(m)
	testui.AssertContainsAll(t, view, "Agents", "Tab 9: running · reading", "open alt+o")
	if strings.Contains(view, "agent:s-9") {
		t.Errorf("Detail draws the link label:\n%s", view)
	}
}

// A linked session no live agent holds is gone. The open key finds no live
// agent there, and the agent menu resumes the conversation and brings it to
// the front.
func TestTheAgentMenuResumesAGoneAgent(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Resumed = agent.Agent{Ref: "demo:home:5", Session: "s-gone-0123", Label: "Tab 5", Status: agent.Idle}
	m = withLinkLabel(t, m, "s-gone-0123")

	testui.AssertContainsAll(t, plainShell(m), "Session s-gone-0: gone")
	if strings.Contains(plainShell(m), "open alt+o") {
		t.Errorf("Detail draws open without a live agent:\n%s", plainShell(m))
	}
	m = pressKey(t, m, "alt+o")
	if !strings.Contains(m.toast.View(), "No live agent on this issue") || len(hostCalls(client, "Focus")) != 0 {
		t.Fatalf("the open key on a gone agent: toast %q, calls %+v", m.toast.View(), client.Calls())
	}

	m = pressKey(t, m, "alt+g")
	testui.AssertContainsAll(t, plainShell(m), "Agents on tm-1", "Resume Session s-gone-0", "Close")
	m = pressKey(t, m, "enter")

	want := []fakes.AgentHostCall{{Method: "Resume", Arg: "s-gone-0123"}, {Method: "Focus", Arg: "demo:home:5"}}
	if got := hostCalls(client, "Resume", "Focus"); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %+v, want %+v", got, want)
	}
	if !strings.Contains(m.toast.View(), "Resumed Session s-gone-0") {
		t.Errorf("toast = %q, want the resume reported", m.toast.View())
	}
}

// After a failed poll the live agents are not known: a linked session is not
// drawn gone and the agent menu offers no resume.
func TestAFailedPollReportsNoLinkedAgentGone(t *testing.T) {
	m, client, _, _ := agentShell(t)
	m = withLinkLabel(t, m, "s-gone-0123")
	testui.AssertContainsAll(t, plainShell(m), "Session s-gone-0: gone")
	client.ListErr = errors.New("revier is down")
	m = send(t, m, agentPollTickMsg{})

	if strings.Contains(plainShell(m), "gone") {
		t.Errorf("Detail draws a gone agent after a failed poll:\n%s", plainShell(m))
	}
	m = pressKey(t, m, "alt+g")
	if strings.Contains(plainShell(m), "Resume") || !strings.Contains(m.toast.View(), "No agent on this issue") {
		t.Errorf("the agent menu after a failed poll: toast %q\n%s", m.toast.View(), plainShell(m))
	}
}

// Close on an idle agent closes it at once, without force.
func TestTheAgentMenuClosesAnIdleAgentAtOnce(t *testing.T) {
	idle := agent.Agent{Ref: "demo:home:3", Label: "Tab 3", Status: agent.Idle, Session: "s-1"}
	m, client, _, _ := agentShell(t, idle)

	m = pressKey(t, m, "alt+g")
	testui.AssertContainsAll(t, plainShell(m), "Agents on tm-1", "Focus Tab 3", "Close Tab 3")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")

	if m.showActionModal {
		t.Fatal("closing an idle agent asked first")
	}
	want := []fakes.AgentHostCall{{Method: "Close", Arg: "demo:home:3"}}
	if got := hostCalls(client, "Close"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Close calls = %+v, want %+v", got, want)
	}
	if !strings.Contains(m.toast.View(), "Closed Tab 3") {
		t.Errorf("toast = %q, want the close reported", m.toast.View())
	}
}

// Close on a running agent asks first: no writes nothing, yes closes it with
// force. Focus brings it to the front.
func TestTheAgentMenuAsksBeforeItClosesARunningAgent(t *testing.T) {
	m, client, _, _ := agentShell(t, runningOnTab3())

	m = pressKey(t, m, "alt+g")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")
	if !m.showActionModal {
		t.Fatal("closing a running agent did not ask")
	}
	testui.AssertContainsAll(t, plainShell(m), "Close agent", "Tab 3 is running. Close it anyway?")
	m = pressKey(t, m, "n")
	if m.showActionModal || m.agentClose != nil || len(hostCalls(client, "Close")) != 0 {
		t.Fatalf("no: question up %v, calls %+v", m.showActionModal, client.Calls())
	}

	m = pressKey(t, m, "alt+g")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")
	m = pressKey(t, m, "y")
	want := []fakes.AgentHostCall{{Method: "Close", Arg: "demo:home:3", Force: true}}
	if got := hostCalls(client, "Close"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Close calls = %+v, want %+v", got, want)
	}

	m = pressKey(t, m, "alt+g")
	m = pressKey(t, m, "enter")
	if got := hostCalls(client, "Focus"); len(got) != 1 || got[0].Arg != "demo:home:3" {
		t.Errorf("Focus calls = %+v, want one on Tab 3", got)
	}
}

// Without a linked agent the agent menu key raises a warning, not a menu.
func TestTheAgentMenuKeyWithoutAnAgentWarns(t *testing.T) {
	m, _, _, _ := agentShell(t)
	m = pressKey(t, m, "alt+g")
	if m.selMenu != nil || !strings.Contains(m.toast.View(), "No agent on this issue") {
		t.Errorf("menu %v, toast %q", m.selMenu != nil, m.toast.View())
	}
}

// A click on the row of an agent in the Details pane does what its entry in
// the agent menu does: a live one comes to the front, a gone one is resumed
// first. A request for an agent no longer linked warns and asks the host
// nothing.
func TestAClickOnAnAgentRowFocusesOrResumesThatAgent(t *testing.T) {
	click := func(t *testing.T, m Model, text string) Model {
		t.Helper()
		x, y := testui.FindCell(t, plainShell(m), text)
		return send(t, m, testui.Click(x, y, tea.MouseLeft))
	}

	t.Run("live", func(t *testing.T) {
		m, client, _, _ := agentShell(t, runningOnTab3())
		click(t, m, "Tab 3: running")
		want := []fakes.AgentHostCall{{Method: "Focus", Arg: "demo:home:3"}}
		if got := hostCalls(client, "Focus", "Resume"); !reflect.DeepEqual(got, want) {
			t.Errorf("calls = %+v, want %+v", got, want)
		}
	})

	t.Run("gone", func(t *testing.T) {
		m, client, _, _ := agentShell(t)
		client.Resumed = agent.Agent{Ref: "demo:home:5", Session: "s-gone-0123", Label: "Tab 5", Status: agent.Idle}
		m = withLinkLabel(t, m, "s-gone-0123")
		m = click(t, m, "Session s-gone-0: gone")
		want := []fakes.AgentHostCall{{Method: "Resume", Arg: "s-gone-0123"}, {Method: "Focus", Arg: "demo:home:5"}}
		if got := hostCalls(client, "Resume", "Focus"); !reflect.DeepEqual(got, want) {
			t.Errorf("calls = %+v, want %+v", got, want)
		}
		if !strings.Contains(m.toast.View(), "Resumed Session s-gone-0") {
			t.Errorf("toast = %q, want the resume reported", m.toast.View())
		}
	})

	t.Run("no longer linked", func(t *testing.T) {
		m, client, _, _ := agentShell(t, runningOnTab3())
		m = send(t, m, mode.ActionRequestMsg{Mode: mode.Detail, Action: mode.ActionOpenAgent, Agent: "s-other"})
		if got := hostCalls(client, "Focus", "Resume"); len(got) != 0 {
			t.Errorf("calls = %+v, want none", got)
		}
		if !strings.Contains(m.toast.View(), "no longer linked") {
			t.Errorf("toast = %q, want a warning", m.toast.View())
		}
	})
}
