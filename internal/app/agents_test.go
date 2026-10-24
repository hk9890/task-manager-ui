package app

// The revier agents of the issue, through the shell: the poll that finds them,
// the open key and button that bring one to the front, and the agent button
// that starts one. The linking itself is tested in internal/mode/detail.

import (
	"bytes"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/revier"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// agentShell is a shell in Detail on tm-1 at 100x60, over a fake launcher and
// a fake revier whose project is demo. The session s-1 wrote a comment on
// tm-1. The agents the fake lists are agents; log holds every record.
func agentShell(t *testing.T, agents ...revier.Agent) (Model, *fakes.FakeRevierClient, *fakes.FakeLauncher, *bytes.Buffer) {
	t.Helper()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	gw.Memory.SeedComments("tm-1", memoryrepo.Comment{
		ID: "c-1", Author: "claude", Session: "s-1", Body: "hello",
		CreatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
	})

	launcher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), launcher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	client := &fakes.FakeRevierClient{ProjectName: "demo", AgentList: agents}
	services.Revier = client
	services.ProjectRoot = t.TempDir()
	log := &bytes.Buffer{}
	services.Logger = slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 60})
	m = pressKey(t, m, "enter")
	if m.active != mode.Detail {
		t.Fatalf("fixture is on %q, want Detail", m.active)
	}
	return m, client, launcher, log
}

func runningOnTab3() revier.Agent {
	return revier.Agent{Panel: "3", Status: revier.StatusRunning, Session: "s-1"}
}

func callsOf(client *fakes.FakeRevierClient, method string) []string {
	var args []string
	for _, call := range client.Calls() {
		if call.Method == method {
			args = append(args, call.Arg)
		}
	}
	return args
}

// The first poll runs when the store is bound; a tick polls again. The project
// is resolved once, and an agent that comes up between two polls shows on the
// ID line and in the Metadata pane.
func TestAPollTickShowsTheAgentThatCommentedOnTheIssue(t *testing.T) {
	m, client, _, _ := agentShell(t)
	if view := plainShell(m); strings.Contains(view, "open alt+o") || strings.Contains(view, "Agents") {
		t.Fatalf("Detail draws an agent while revier lists none:\n%s", view)
	}

	client.AgentList = []revier.Agent{runningOnTab3(), {Panel: "4", Status: revier.StatusIdle, Session: "s-other"}}
	m = send(t, m, agentPollTickMsg{})

	testui.AssertContainsAll(t, plainShell(m), "agent alt+i", "open alt+o", "edit alt+u", "Agents", "Tab 3: running")
	if strings.Contains(plainShell(m), "Tab 4") {
		t.Errorf("an agent that wrote no comment on the issue is listed:\n%s", plainShell(m))
	}
	if got := callsOf(client, "Project"); !reflect.DeepEqual(got, []string{m.services.ProjectRoot}) {
		t.Errorf("Project calls = %v, want one for the project root", got)
	}
	if got := callsOf(client, "Agents"); !reflect.DeepEqual(got, []string{"demo", "demo"}) {
		t.Errorf("Agents calls = %v, want two for demo", got)
	}
}

// A tick while a poll is in flight starts no second one, and every tick
// re-arms the timer.
func TestATickDuringAPollStartsNoSecondPoll(t *testing.T) {
	m, client, _, _ := agentShell(t)
	armed := 0
	m.scheduleAgentPoll = func() tea.Cmd {
		armed++
		return nil
	}

	next, first := m.Update(agentPollTickMsg{})
	next, second := next.(Model).Update(agentPollTickMsg{})
	m = applyMessages(t, next.(Model), append(runBatch(first), runBatch(second)...))

	if got := callsOf(client, "Agents"); len(got) != 2 {
		t.Errorf("Agents calls = %v, want the bind's poll and one more", got)
	}
	if armed != 2 {
		t.Errorf("two ticks armed the timer %d times", armed)
	}
	if m.agentPollInFlight {
		t.Error("a poll is still in flight after its result")
	}
}

// A new store's poll does not see the previous store's project or agents.
func TestBindingAStoreClearsTheAgentsOfThePreviousOne(t *testing.T) {
	m, _, _, _ := agentShell(t, runningOnTab3())
	if m.revierProject != "demo" || len(m.detail.LinkedAgents()) != 1 {
		t.Fatalf("fixture: project %q, linked %v", m.revierProject, m.detail.LinkedAgents())
	}
	m.bindStore(m.services)
	if m.revierProject != "" || m.agentPollInFlight || len(m.detail.LinkedAgents()) != 0 {
		t.Errorf("after a bind: project %q, in flight %v, linked %v", m.revierProject, m.agentPollInFlight, m.detail.LinkedAgents())
	}
}

// The open key brings the linked agent to the front; a failure says so in a
// toast, and an issue with no live agent refuses with a toast and asks
// revier nothing.
func TestTheOpenKeyFocusesTheLinkedAgent(t *testing.T) {
	t.Run("linked", func(t *testing.T) {
		m, client, _, _ := agentShell(t, runningOnTab3())
		m = pressKey(t, m, "alt+o")
		if got := callsOf(client, "Focus"); !reflect.DeepEqual(got, []string{"demo:3"}) {
			t.Errorf("Focus calls = %v, want demo:3", got)
		}
		if m.toast.Visible() {
			t.Errorf("a focus that worked raised the toast %q", m.toast.View())
		}
	})

	t.Run("refused", func(t *testing.T) {
		m, client, _, _ := agentShell(t, runningOnTab3())
		client.FocusErr = errors.New("no agent at demo:3")
		m = pressKey(t, m, "alt+o")
		if view := m.toast.View(); !strings.Contains(view, "no agent at demo:3") {
			t.Errorf("toast = %q, want the focus error", view)
		}
	})

	t.Run("no linked agent", func(t *testing.T) {
		m, client, _, _ := agentShell(t)
		m = pressKey(t, m, "alt+o")
		if got := callsOf(client, "Focus"); len(got) != 0 {
			t.Errorf("Focus calls = %v, want none", got)
		}
		if view := m.toast.View(); !strings.Contains(view, "No live agent on this issue") {
			t.Errorf("toast = %q", view)
		}
	})
}

// The agent and open buttons do what alt+i and alt+o do.
func TestClicksOnTheAgentButtonsDoWhatTheirKeysDo(t *testing.T) {
	click := func(t *testing.T, m Model, text string) Model {
		t.Helper()
		x, y := testui.FindCell(t, plainShell(m), text)
		return send(t, m, testui.Click(x, y, tea.MouseLeft))
	}

	t.Run("agent", func(t *testing.T) {
		m, _, launcher, _ := agentShell(t)
		click(t, m, "agent alt+i")
		calls := launcher.Calls()
		if len(calls) != 1 || calls[0].Action != LaunchActionAgent || calls[0].Issue.Summary.ID != "tm-1" {
			t.Errorf("launcher calls = %+v, want the agent launcher on tm-1", calls)
		}
	})

	t.Run("open", func(t *testing.T) {
		m, client, _, _ := agentShell(t, runningOnTab3())
		click(t, m, "open alt+o")
		if got := callsOf(client, "Focus"); !reflect.DeepEqual(got, []string{"demo:3"}) {
			t.Errorf("Focus calls = %v, want demo:3", got)
		}
	})
}

// A failing poll clears the agents and logs once, at WARN, until a poll
// succeeds; the next failure logs again. A project root that is no revier
// project logs once at DEBUG. No poll raises a toast.
func TestAFailingPollLogsOncePerFailure(t *testing.T) {
	m, client, _, log := agentShell(t, runningOnTab3())

	client.AgentsErr = errors.New("revier list: exit status 1")
	m = send(t, send(t, m, agentPollTickMsg{}), agentPollTickMsg{})
	if len(m.detail.LinkedAgents()) != 0 || strings.Contains(plainShell(m), "open alt+o") {
		t.Errorf("a failed poll left the agents on screen:\n%s", plainShell(m))
	}
	if got := strings.Count(log.String(), "level=WARN msg=\"revier agent listing failed"); got != 1 {
		t.Errorf("two failed polls logged %d WARN records:\n%s", got, log)
	}
	if !strings.Contains(log.String(), "project=demo error=\"revier list: exit status 1\"") {
		t.Errorf("the record does not name the project and the error:\n%s", log)
	}

	client.AgentsErr = nil
	m = send(t, m, agentPollTickMsg{})
	if len(m.detail.LinkedAgents()) != 1 {
		t.Errorf("a poll that succeeded again links %v", m.detail.LinkedAgents())
	}
	client.AgentsErr = errors.New("revier list: exit status 1")
	m = send(t, m, agentPollTickMsg{})
	if got := strings.Count(log.String(), "level=WARN"); got != 2 {
		t.Errorf("a failure after a success logged %d WARN records in all, want 2", got)
	}

	m, client, _, log = agentShell(t)
	log.Reset()
	m.revierProject = ""
	client.ProjectErr = errors.New("not a revier project")
	m = send(t, send(t, m, agentPollTickMsg{}), agentPollTickMsg{})
	if got := strings.Count(log.String(), "level=DEBUG msg=\"project is not a revier project"); got != 1 || strings.Contains(log.String(), "level=WARN") {
		t.Errorf("two polls outside a revier project logged:\n%s", log)
	}
	if m.toast.Visible() {
		t.Errorf("a failed poll raised the toast %q", m.toast.View())
	}
}

// Outside a revier project every lookup fails. A failed lookup runs again
// only after projectRetryTicks ticks, and at once after a store bind, so the
// poll does not run revier every tick for the whole session.
func TestAFailedProjectLookupWaitsBeforeItRunsAgain(t *testing.T) {
	m, client, _, _ := agentShell(t)
	m.revierProject = ""
	client.ProjectErr = errors.New("not a revier project")
	before := len(callsOf(client, "Project"))

	m = send(t, m, agentPollTickMsg{})
	for range projectRetryTicks {
		m = send(t, m, agentPollTickMsg{})
	}
	if got := len(callsOf(client, "Project")) - before; got != 1 {
		t.Errorf("a failed lookup and %d ticks ran %d lookups, want 1", projectRetryTicks, got)
	}
	m = send(t, m, agentPollTickMsg{})
	if got := len(callsOf(client, "Project")) - before; got != 2 {
		t.Errorf("the tick after the wait ran %d lookups in all, want 2", got)
	}

	m.bindStore(m.services)
	m = send(t, m, agentPollTickMsg{})
	if got := len(callsOf(client, "Project")) - before; got != 3 {
		t.Errorf("a tick after a store bind ran %d lookups in all, want 3", got)
	}
}

// A poll hands the board the live agents: the In Progress card of an issue the
// agent commented on draws the agent mark, and a failed poll takes it away.
func TestAPollMarksTheInProgressCardOfTheLinkedAgent(t *testing.T) {
	gw := fakes.NewTracked()
	seedInProgress(gw, "tm-2", "Agent at work", "task", 1)
	gw.Memory.SeedComments("tm-2", memoryrepo.Comment{
		ID: "c-2", Author: "claude", Session: "s-2", Body: "hello",
		CreatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
	})
	services, err := NewServicesWithLauncher(gw, config.Default(), &fakes.FakeLauncher{})
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	client := &fakes.FakeRevierClient{ProjectName: "demo", AgentList: []revier.Agent{{Panel: "2", Status: revier.StatusRunning, Session: "s-2"}}}
	services.Revier = client
	services.ProjectRoot = t.TempDir()

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: 160, Height: 30})

	marked := "IP " + styles.Glyphs.Agent + " tm-2"
	testui.AssertContainsAll(t, plainShell(m), marked)

	client.AgentsErr = errors.New("revier list: exit status 1")
	m = send(t, m, agentPollTickMsg{})
	if strings.Contains(plainShell(m), marked) {
		t.Errorf("a failed poll left the agent mark on the board:\n%s", plainShell(m))
	}
}
