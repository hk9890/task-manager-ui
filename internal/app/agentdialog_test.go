package app

// The agent dialog of the issue in Detail: launch_agent and the agent button
// open it. It focuses, closes and resumes the agents linked to the issue, and
// an intent in it starts a new agent without focus and links it to the issue.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// withLinkLabel writes the link label of session on tm-1 and reads Detail
// again, as a start on a store that is not watched does.
func withLinkLabel(t *testing.T, m Model, session string) Model {
	t.Helper()
	if err := m.services.Repo.UpdateIssue(m.ctx, "tm-1", domain.UpdateIssueInput{AddLabels: []string{"agent-session:" + session}}); err != nil {
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

// An agent the agent dialog starts is in the Agents group after the next
// poll, linked by its label alone: it wrote no comment. The label is not drawn.
func TestAStartedAgentShowsInTheAgentsGroupByItsLabel(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = agent.Agent{Ref: "demo:home:9", Session: "s-9", Label: "Tab 9", Status: agent.Running, Activity: "reading"}

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "enter")
	m = send(t, m, agentPollTickMsg{})

	view := plainShell(m)
	testui.AssertContainsAll(t, view, "Agents", "running  Tab 9")
	if strings.Contains(view, "agent-session:s-9") {
		t.Errorf("Detail draws the link label:\n%s", view)
	}
}

// A linked session no live agent holds is gone. The agent dialog resumes the
// conversation and brings it to the front.
func TestTheAgentDialogResumesAGoneAgent(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Resumed = agent.Agent{Ref: "demo:home:5", Session: "s-gone-0123", Label: "Tab 5", Status: agent.Idle}
	m = withLinkLabel(t, m, "s-gone-0123")

	testui.AssertContainsAll(t, plainShell(m), "gone  Session s-gone-0")

	m = pressKey(t, m, "alt+i")
	testui.AssertContainsAll(t, plainShell(m), "Agents on tm-1", "Resume Session s-gone-0", "Unlink Session s-gone-0", "Start new", "implement", "Cancel")
	m = pressKey(t, m, "enter")

	want := []fakes.AgentHostCall{{Method: "Resume", Arg: "s-gone-0123"}, {Method: "Focus", Arg: "demo:home:5"}}
	if got := hostCalls(client, "Resume", "Focus"); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %+v, want %+v", got, want)
	}
	if !strings.Contains(m.toast.View(), "Resumed Session s-gone-0") {
		t.Errorf("toast = %q, want the resume reported", m.toast.View())
	}
}

// The agent dialog unlinks a gone agent without a host call: its link label
// leaves the issue. On a closed issue the store refuses that, so the gone
// agent is left out of the Details pane and of the dialog, as on the Agents tab.
func TestTheAgentDialogUnlinksAGoneAgent(t *testing.T) {
	m, client, _, _ := agentShell(t)
	m = withLinkLabel(t, m, "s-gone-0123")
	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")

	if got := hostCalls(client, "Close", "Resume"); len(got) != 0 {
		t.Fatalf("the unlink asked the host %+v", got)
	}
	if got := labelsOf(t, m, "tm-1"); len(got) != 0 {
		t.Errorf("labels of tm-1 = %v, want the link label removed", got)
	}
	if !strings.Contains(m.toast.View(), "Unlinked Session s-gone-0") {
		t.Errorf("toast = %q, want the unlink reported", m.toast.View())
	}

	m, _, _, _ = agentShell(t)
	m = withLinkLabel(t, m, "s-gone-0123")
	m.detail.Detail.Summary.Status = domain.StatusClosed
	if view := plainShell(m); strings.Contains(view, "Session s-gone-0") {
		t.Errorf("the Details pane of a closed issue shows the gone agent:\n%s", view)
	}
	m = pressKey(t, m, "alt+i")
	for _, entry := range m.selMenu.entries {
		if strings.HasSuffix(entry.entry.Label, "Session s-gone-0") {
			t.Errorf("the dialog of a closed issue offers %q", entry.entry.Label)
		}
	}
}

// After a failed poll the live agents are not known: a linked session is not
// drawn gone and the agent dialog offers no resume.
func TestAFailedPollReportsNoLinkedAgentGone(t *testing.T) {
	m, client, _, _ := agentShell(t)
	m = withLinkLabel(t, m, "s-gone-0123")
	testui.AssertContainsAll(t, plainShell(m), "gone  Session s-gone-0")
	client.ListErr = errors.New("revier is down")
	m = send(t, m, agentPollTickMsg{})

	if strings.Contains(plainShell(m), "gone") {
		t.Errorf("Detail draws a gone agent after a failed poll:\n%s", plainShell(m))
	}
	m = pressKey(t, m, "alt+i")
	if view := plainShell(m); strings.Contains(view, "Resume") || !strings.Contains(view, "Start new") {
		t.Errorf("the agent dialog after a failed poll:\n%s", view)
	}
}

// Close on an idle agent closes it at once, without force.
func TestTheAgentDialogClosesAnIdleAgentAtOnce(t *testing.T) {
	idle := agent.Agent{Ref: "demo:home:3", Label: "Tab 3", Status: agent.Idle, Session: "s-1"}
	m, client, _, _ := agentShell(t, idle)

	m = pressKey(t, m, "alt+i")
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

// A close from the dialog unlinks the agent: its link label leaves every
// open issue that carries it, and every other label stays. A closed issue
// keeps it: the store refuses every change to one.
func TestClosingAnAgentRemovesItsLinkLabelFromTheOpenIssues(t *testing.T) {
	idle := agent.Agent{Ref: "demo:home:3", Label: "Tab 3", Status: agent.Idle, Session: "s-3"}
	m, _, _, _ := agentShell(t, idle)
	gw := m.services.Repo.(*fakes.TrackedRepository)
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Also linked", Labels: []string{"agent-session:s-3"}})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-3", Title: "Done", Status: "closed", Labels: []string{"agent-session:s-3"}})
	if err := m.services.Repo.UpdateIssue(m.ctx, "tm-1", domain.UpdateIssueInput{AddLabels: []string{"backend", "agent-session:s-30"}}); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	m = withLinkLabel(t, m, "s-3")

	m = pressKey(t, m, "alt+i")
	testui.AssertContainsAll(t, plainShell(m), "Focus Tab 3", "Close Tab 3")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")

	if got, want := labelsOf(t, m, "tm-1"), []string{"backend", "agent-session:s-30"}; !reflect.DeepEqual(got, want) {
		t.Errorf("labels of tm-1 = %v, want %v", got, want)
	}
	if got := labelsOf(t, m, "tm-2"); len(got) != 0 {
		t.Errorf("labels of tm-2 = %v, want none", got)
	}
	if got, want := labelsOf(t, m, "tm-3"), []string{"agent-session:s-3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("labels of the closed tm-3 = %v, want %v", got, want)
	}
	if !strings.Contains(m.toast.View(), "Closed Tab 3") {
		t.Errorf("toast = %q, want the close reported", m.toast.View())
	}
	if strings.Contains(plainShell(m), "idle  Tab 3") {
		t.Errorf("Detail still lists the unlinked agent:\n%s", plainShell(m))
	}
}

// A close that fails unlinks nothing; a removal that fails says the agent is
// closed but still linked.
func TestAFailedCloseOrUnlinkKeepsTheLinkLabel(t *testing.T) {
	idle := agent.Agent{Ref: "demo:home:3", Label: "Tab 3", Status: agent.Idle, Session: "s-3"}
	m, client, _, _ := agentShell(t, idle)
	m = withLinkLabel(t, m, "s-3")
	client.CloseErr = errors.New("revier is down")

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")
	if got := labelsOf(t, m, "tm-1"); !reflect.DeepEqual(got, []string{"agent-session:s-3"}) {
		t.Errorf("labels after a failed close = %v, want the link label kept", got)
	}
	if !strings.Contains(m.toast.View(), "Could not close Tab 3") {
		t.Errorf("toast = %q, want the failed close reported", m.toast.View())
	}

	client.CloseErr = nil
	failing := fakes.NewErrorInjecting(m.services.Repo)
	failing.SetError(fakes.MethodUpdateIssue, errors.New("store is locked"))
	m.services.Repo = failing
	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")
	if !strings.Contains(m.toast.View(), "Closed Tab 3, but could not unlink it from tm-1: store is locked") {
		t.Errorf("toast = %q, want the failed unlink reported", m.toast.View())
	}
}

// Close on a running agent asks first: no writes nothing, yes closes it with
// force. Focus brings it to the front.
func TestTheAgentDialogAsksBeforeItClosesARunningAgent(t *testing.T) {
	m, client, _, _ := agentShell(t, runningOnTab3())

	m = pressKey(t, m, "alt+i")
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

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")
	m = pressKey(t, m, "y")
	want := []fakes.AgentHostCall{{Method: "Close", Arg: "demo:home:3", Force: true}}
	if got := hostCalls(client, "Close"); !reflect.DeepEqual(got, want) {
		t.Fatalf("Close calls = %+v, want %+v", got, want)
	}

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "enter")
	if got := hostCalls(client, "Focus"); len(got) != 1 || got[0].Arg != "demo:home:3" {
		t.Errorf("Focus calls = %+v, want one on Tab 3", got)
	}
}

// Without a linked agent and without an intent the agent key raises a
// warning, not a dialog.
func TestTheAgentKeyWithoutAnAgentOrAnIntentWarns(t *testing.T) {
	m, _, _, _ := agentShell(t)
	m.services.Config.Agent.Intents = nil
	m = pressKey(t, m, "alt+i")
	if m.selMenu != nil || !strings.Contains(m.toast.View(), "No agent on this issue and no agent intents configured") {
		t.Errorf("menu %v, toast %q", m.selMenu != nil, m.toast.View())
	}
}

// Without an intent the dialog lists the linked agents and has no Start new
// part.
func TestTheAgentDialogWithoutAnIntentHasNoStartPart(t *testing.T) {
	m, _, _, _ := agentShell(t, runningOnTab3())
	m.services.Config.Agent.Intents = nil
	m = pressKey(t, m, "alt+i")
	view := plainShell(m)
	testui.AssertContainsAll(t, view, "Agents on tm-1", "Focus Tab 3", "Close Tab 3")
	if strings.Contains(view, "Start new") {
		t.Errorf("the dialog has a Start new part without an intent:\n%s", view)
	}
}

// A click on the row of an agent in the Details pane switches to the Agents
// tab with that agent selected: Enter there does what its entry in the agent
// dialog does, a live one comes to the front and a gone one is resumed first.
func TestAClickOnAnAgentRowSelectsItOnTheAgentsTab(t *testing.T) {
	click := func(t *testing.T, m Model, text string) Model {
		t.Helper()
		x, y := testui.FindCell(t, plainShell(m), text)
		return send(t, m, testui.Click(x, y, tea.MouseLeft))
	}

	t.Run("live", func(t *testing.T) {
		m, client, _, _ := agentShell(t, runningOnTab3(), agent.Agent{Ref: "demo:home:4", Label: "Tab 4", Status: agent.Idle, Session: "s-4"})
		m = click(t, m, "running  Tab 3")
		if m.active != mode.Agents {
			t.Fatalf("the click left the shell on %q, want the Agents tab", m.active)
		}
		if got := hostCalls(client, "Focus", "Resume"); len(got) != 0 {
			t.Fatalf("the click asked the host %+v, want nothing", got)
		}
		m = pressKey(t, m, "enter")
		want := []fakes.AgentHostCall{{Method: "Focus", Arg: "demo:home:3"}}
		if got := hostCalls(client, "Focus", "Resume"); !reflect.DeepEqual(got, want) {
			t.Errorf("calls = %+v, want %+v", got, want)
		}
	})

	t.Run("gone", func(t *testing.T) {
		m, client, _, _ := agentShell(t, agent.Agent{Ref: "demo:home:4", Label: "Tab 4", Status: agent.Idle, Session: "s-4"})
		client.Resumed = agent.Agent{Ref: "demo:home:5", Session: "s-gone-0123", Label: "Tab 5", Status: agent.Idle}
		m = withLinkLabel(t, m, "s-gone-0123")
		m = click(t, m, "gone  Session s-gone-0")
		m = pressKey(t, m, "enter")
		want := []fakes.AgentHostCall{{Method: "Resume", Arg: "s-gone-0123"}, {Method: "Focus", Arg: "demo:home:5"}}
		if got := hostCalls(client, "Resume", "Focus"); !reflect.DeepEqual(got, want) {
			t.Errorf("calls = %+v, want %+v", got, want)
		}
		if !strings.Contains(m.toast.View(), "Resumed Session s-gone-0") {
			t.Errorf("toast = %q, want the resume reported", m.toast.View())
		}
	})
}

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

// The agent key opens the dialog on the issue: the linked agents first, then
// Start new with one entry per intent, then Cancel. The cursor starts on the
// first entry that is on.
func TestTheAgentKeyOpensTheAgentDialogOnTheIssue(t *testing.T) {
	m, client, _, _ := agentShell(t, runningOnTab3())
	m = pressKey(t, m, "alt+i")

	var labels []string
	for _, entry := range m.selMenu.entries {
		labels = append(labels, entry.entry.Label)
	}
	if want := []string{"Focus Tab 3", "Close Tab 3", "Start new", "implement", "review", "Cancel"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("entries = %v, want %v", labels, want)
	}
	testui.AssertContainsAll(t, plainShell(m), "Agents on tm-1", "Focus Tab 3", "Start new", "implement", "review")
	if m.selMenu.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.selMenu.cursor)
	}
	if specs := startCalls(client); len(specs) != 0 {
		t.Fatalf("the menu started an agent before a choice: %+v", specs)
	}

	m = pressKey(t, m, "esc")
	if m.selMenu != nil || strings.Contains(plainShell(m), "Agents on") {
		t.Fatal("Escape left the agent dialog open")
	}
	if specs := startCalls(client); len(specs) != 0 {
		t.Fatalf("Escape started an agent: %+v", specs)
	}
}

// Enter on an intent starts an agent without focus, prompted with the intent's
// prompt for the issue, and writes the label of its session on the issue.
func TestAnIntentStartsAnAgentWithoutFocusAndLinksItToTheIssue(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "down")
	m = pressKey(t, m, "enter")

	want := []agent.Spec{{
		Root:   m.services.ProjectRoot,
		Prompt: "Review the work done on taskmgr issue tm-1 and report what is missing.",
	}}
	if got := startCalls(client); !reflect.DeepEqual(got, want) {
		t.Fatalf("Start calls = %+v, want %+v", got, want)
	}
	if labels := labelsOf(t, m, "tm-1"); !reflect.DeepEqual(labels, []string{"agent-session:s-9"}) {
		t.Fatalf("labels of tm-1 = %v, want [agent-session:s-9]", labels)
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

// A session the store refuses in a label is not written: the toast says why.
// With the start failed too, one toast carries both errors.
func TestASessionTheStoreRefusesIsReportedWithTheStartError(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = agent.Agent{Ref: "demo:home:9", Session: "Not A Label", Label: "Tab 9"}
	client.StartErr = errors.New("the focus was refused")

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "enter")

	if labels := labelsOf(t, m, "tm-1"); len(labels) != 0 {
		t.Fatalf("labels of tm-1 = %v, want none", labels)
	}
	view := m.toast.View()
	for _, part := range []string{"the focus was refused", "could not link it", "makes no valid issue label"} {
		if !strings.Contains(view, part) {
			t.Errorf("toast = %q, want it to hold %q", view, part)
		}
	}
}

// A link written to an issue Detail does not show reloads the board, not
// Detail.
func TestALinkOnAnotherIssueDoesNotReloadDetail(t *testing.T) {
	m, _, _, _ := agentShell(t)
	m.handleAgentStarted(agentStartedMsg{issueID: "tm-other", agent: startedOnTab9()})
	if m.detail.IsLoading() {
		t.Fatal("a link on another issue reloaded Detail")
	}
}

func TestTheAgentDialogNeedsAnAgentHost(t *testing.T) {
	m, _, _, _ := agentShell(t)
	m.services.Agents = nil

	m = pressKey(t, m, "alt+i")
	if m.selMenu != nil {
		t.Fatal("the agent dialog opened with no agent host")
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

	if labels := labelsOf(t, m, "tm-1"); !reflect.DeepEqual(labels, []string{"agent-session:s-9"}) {
		t.Fatalf("labels of tm-1 = %v, want [agent-session:s-9]", labels)
	}
	if view := m.toast.View(); !strings.Contains(view, "Started Tab 9 on tm-1, but: the focus was refused") {
		t.Fatalf("toast = %q, want the partial failure", view)
	}
}

// On a closed issue the store refuses the link label: the Start new part says
// so, and its intents are drawn off and start nothing.
func TestTheAgentDialogOnAClosedIssueStartsNothing(t *testing.T) {
	m, client, _, _ := agentShell(t)
	m.detail.Detail.Summary.Status = domain.StatusClosed

	m = pressKey(t, m, "alt+i")
	testui.AssertContainsAll(t, plainShell(m), "Start new: off, the issue is closed", "implement", "review")
	for _, entry := range m.selMenu.entries {
		if entry.entry.Label == "implement" && !entry.entry.Disabled {
			t.Fatal("the implement intent is on for a closed issue")
		}
	}
	if label := m.selMenu.entries[m.selMenu.cursor].entry.Label; label != "Cancel" {
		t.Errorf("the cursor is on %q, want Cancel", label)
	}

	x, y := testui.FindCell(t, plainShell(m), "implement")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	if specs := startCalls(client); len(specs) != 0 {
		t.Fatalf("a click on an intent that is off started %+v", specs)
	}
}

// A started agent's row is running at once, not gone until the next poll
// tick: the start lists the agents again after its link label is written.
func TestAStartedAgentIsNotDrawnGoneBeforeTheNextPoll(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "enter")

	view := plainShell(m)
	if strings.Contains(view, "gone  Session s-9") {
		t.Errorf("Detail draws the started agent gone:\n%s", view)
	}
	testui.AssertContainsAll(t, view, "idle  Tab 9")
}
