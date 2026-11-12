package app

// The intent menu: launch_agent and the agent button open it on the issue in
// Detail, and an intent starts an agent in front and links it to the issue.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func startedOnTab9() agent.Agent {
	return agent.Agent{Ref: "demo:home:9", Session: "s-9", Label: "Tab 9", Status: agent.Idle}
}

func startCalls(client *fakes.FakeAgentHost) []agent.Spec {
	var specs []agent.Spec
	for _, call := range client.Calls() {
		if call.Method == "Start" {
			specs = append(specs, call.Spec)
		}
	}
	return specs
}

func labelsOf(t *testing.T, m Model, id string) []string {
	t.Helper()
	issue, err := m.services.Repo.Issue(m.ctx, id)
	if err != nil {
		t.Fatalf("Issue(%s): %v", id, err)
	}
	return issue.Summary.Labels
}

func TestTheAgentKeyOpensTheIntentMenuOnTheIssue(t *testing.T) {
	m, client, _, _ := agentShell(t)
	m = pressKey(t, m, "alt+i")

	testui.AssertContainsAll(t, plainShell(m), "Start an agent on tm-1", "implement", "review", "Close")
	if specs := startCalls(client); len(specs) != 0 {
		t.Fatalf("the menu started an agent before a choice: %+v", specs)
	}

	m = pressKey(t, m, "esc")
	if m.selMenu != nil || strings.Contains(plainShell(m), "Start an agent on") {
		t.Fatal("Escape left the intent menu open")
	}
	if specs := startCalls(client); len(specs) != 0 {
		t.Fatalf("Escape started an agent: %+v", specs)
	}
}

// Enter on an intent starts an agent in front, prompted with the intent's
// prompt for the issue, and writes the label of its session on the issue.
func TestAnIntentStartsAnAgentInFrontAndLinksItToTheIssue(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")

	want := []agent.Spec{{
		Root:   m.services.ProjectRoot,
		Prompt: "Review the work done on taskmgr issue tm-1 and report what is missing.",
		Focus:  true,
	}}
	if got := startCalls(client); !reflect.DeepEqual(got, want) {
		t.Fatalf("Start calls = %+v, want %+v", got, want)
	}
	if labels := labelsOf(t, m, "tm-1"); !reflect.DeepEqual(labels, []string{"agent:s-9"}) {
		t.Fatalf("labels of tm-1 = %v, want [agent:s-9]", labels)
	}
	if m.selMenu != nil {
		t.Fatal("the menu stayed open after the start")
	}
	if view := m.toast.View(); !strings.Contains(view, "Started Tab 9 on tm-1") {
		t.Fatalf("toast = %q, want the start reported", view)
	}
}

func TestAClickOnAnIntentStartsIt(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()

	m = pressKey(t, m, "alt+i")
	x, y := testui.FindCell(t, plainShell(m), "implement")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))

	specs := startCalls(client)
	if len(specs) != 1 || !strings.HasPrefix(specs[0].Prompt, "Implement taskmgr issue tm-1") {
		t.Fatalf("Start calls = %+v, want the implement intent on tm-1", specs)
	}
}

// A start that fails, and one whose link is not written, reach the operator
// as an error toast. A failed start writes no label.
func TestAFailedStartIsShownToTheOperator(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		m, client, _, _ := agentShell(t)
		client.StartErr = errors.New("revier is not running")

		m = pressKey(t, m, "alt+i")
		m = pressKey(t, m, "enter")

		if view := m.toast.View(); !strings.Contains(view, "Could not start an agent on tm-1: revier is not running") {
			t.Fatalf("toast = %q, want the failure", view)
		}
		if labels := labelsOf(t, m, "tm-1"); len(labels) != 0 {
			t.Fatalf("labels of tm-1 = %v, want none", labels)
		}
	})

	t.Run("link", func(t *testing.T) {
		m, client, _, _ := agentShell(t)
		client.Started = agent.Agent{Ref: "demo:home:9", Label: "Tab 9"}

		m = pressKey(t, m, "alt+i")
		m = pressKey(t, m, "enter")

		if view := m.toast.View(); !strings.Contains(view, "Started Tab 9, but could not link it to tm-1") {
			t.Fatalf("toast = %q, want the link failure", view)
		}
	})
}

func TestTheIntentMenuNeedsAnAgentHost(t *testing.T) {
	m, _, _, _ := agentShell(t)
	m.services.Agents = nil

	m = pressKey(t, m, "alt+i")
	if m.selMenu != nil {
		t.Fatal("the intent menu opened with no agent host")
	}
	if view := m.toast.View(); !strings.Contains(view, "agents are off") {
		t.Fatalf("toast = %q, want the reason", view)
	}
}

// A start that fails after the agent opened with its session, at the prompt
// or the focus, still links the agent: it is open and may work on the issue.
func TestAStartThatFailsAfterTheSessionIsKnownStillLinksTheAgent(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	client.StartErr = errors.New("the focus was refused")

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "enter")

	if labels := labelsOf(t, m, "tm-1"); !reflect.DeepEqual(labels, []string{"agent:s-9"}) {
		t.Fatalf("labels of tm-1 = %v, want [agent:s-9]", labels)
	}
	if view := m.toast.View(); !strings.Contains(view, "Started Tab 9 on tm-1, but: the focus was refused") {
		t.Fatalf("toast = %q, want the partial failure", view)
	}
}
