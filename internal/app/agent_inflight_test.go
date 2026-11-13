package app

// The live agents against the work the app does on them: a poll that began
// before a start or a close, the reload key of the Agents tab, a resume or a
// close that outlives a store switch, and where the question of an asking
// agent may open.

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// pollInFlight starts a poll and returns its result, which the shell has not
// seen yet: the host answered before what the test does next.
func pollInFlight(t *testing.T, m *Model) []tea.Msg {
	t.Helper()
	poll := m.pollAgentsCmd()
	if poll == nil {
		t.Fatal("fixture: no poll started")
	}
	return runBatch(poll)
}

// A poll that began before a start lands without the new agent. The started
// agent is live at once, so its link label does not draw it gone, and the
// result of the older poll does not take it away.
func TestAPollInFlightAtAStartDoesNotDrawTheNewAgentGone(t *testing.T) {
	m, client, _, _ := agentShell(t)
	stale := pollInFlight(t, &m)

	if err := m.services.Repo.UpdateIssue(m.ctx, "tm-1", domain.UpdateIssueInput{AddLabels: []string{"agent-session:s-9"}}); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	m = send(t, m, agentStartedMsg{epoch: m.storeEpoch, issueID: "tm-1", agent: startedOnTab9()})
	client.Agents = []agent.Agent{startedOnTab9()}
	for _, msg := range stale {
		m = updateOnly(m, msg)
	}

	if got := m.detail.LinkedAgents(); !reflect.DeepEqual(got, []agent.Agent{startedOnTab9()}) {
		t.Fatalf("Detail links %+v, want the started agent live", got)
	}
}

// A poll that began before a close still lists the closed agent. Its result
// is dropped, so the agent does not come back as live.
func TestAPollInFlightAtACloseDoesNotPutTheAgentBack(t *testing.T) {
	m, client := agentsTab(t)
	stale := pollInFlight(t, &m)

	client.Agents = []agent.Agent{idleOnTab4()}
	dropLinkLabel(t, m, "s-1")
	m = updateOnly(m, agentClosedMsg{epoch: m.storeEpoch, agent: runningOnTab3(), unlinked: []string{"tm-1"}})
	for _, msg := range stale {
		m = updateOnly(m, msg)
	}

	if view := plainShell(m); strings.Contains(view, "Tab 3  running") {
		t.Errorf("the Agents tab draws the closed agent again:\n%s", view)
	}
	if !m.agentPollInFlight {
		t.Error("the dropped poll started no other poll")
	}
}

// The reload key of the Agents tab asks the host for the agents too, also
// with no auto refresh and while a lookup that found no project waits.
func TestTheReloadKeyOfTheAgentsTabPollsTheHost(t *testing.T) {
	m, client := agentsTab(t)
	m.projectLookupWait = projectRetryTicks
	before := len(hostCalls(client, "List"))

	m = pressKey(t, m, "alt+r")

	if got := len(hostCalls(client, "List")) - before; got != 1 {
		t.Errorf("the reload key polled %d times, want 1", got)
	}
}

// With no auto refresh the agents are polled once at startup, so the Agents
// tab does not wait for the host for good.
func TestTheAgentsArePolledAtStartupWithNoAutoRefresh(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	services, err := NewServicesWithLauncher(gw, config.Default(), &fakes.FakeLauncher{})
	if err != nil {
		t.Fatal(err)
	}
	client := &fakes.FakeAgentHost{Agents: []agent.Agent{idleOnTab4()}}
	services.Agents = client
	services.ProjectRoot = t.TempDir()
	m := mustNewModelWithOptions(t, services, RuntimeOptions{DisableAutoRefresh: true})
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = pressKey(t, m, "shift+tab")

	if view := plainShell(m); strings.Contains(view, "waiting for the agent host") || !strings.Contains(view, "scratch  idle") {
		t.Errorf("the Agents tab with no auto refresh:\n%s", view)
	}
}

// contextHost is the fake host whose Resume and Close fail on a context
// that ended, as the real host's calls do.
type contextHost struct {
	*fakes.FakeAgentHost
}

func (h contextHost) Resume(ctx context.Context, root, session string) (agent.Agent, error) {
	if err := ctx.Err(); err != nil {
		return agent.Agent{}, err
	}
	return h.FakeAgentHost.Resume(ctx, root, session)
}

func (h contextHost) Close(ctx context.Context, a agent.Agent, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.FakeAgentHost.Close(ctx, a, force)
}

// A resume outlives a store switch: it runs to its end and is reported.
func TestAResumeIsReportedAfterAStoreSwitch(t *testing.T) {
	m, client := withGoneRow(t)
	client.Resumed = agent.Agent{Ref: "demo:home:5", Session: "s-gone-0123", Label: "Tab 5", Status: agent.Idle}
	m.services.Agents = contextHost{client}
	m, resume := requestFrom(t, m, "enter")

	switchToOtherStore(t, &m)
	m = applyMessages(t, m, runBatch(resume))

	if view := m.toast.View(); !strings.Contains(view, "Resumed Session s-gone-0") {
		t.Fatalf("toast = %q, want the resume reported", view)
	}
}

// A close outlives a store switch: the agent is closed, unlinked in the
// store the close began on, and reported.
func TestACloseIsReportedAfterAStoreSwitch(t *testing.T) {
	m, client := agentsTab(t)
	m.services.Agents = contextHost{client}
	first := m.services.Repo
	closeCmd := m.closeAgent(runningOnTab3(), true)

	switchToOtherStore(t, &m)
	m = applyMessages(t, m, runBatch(closeCmd))

	if view := m.toast.View(); !strings.Contains(view, "Closed Tab 3") {
		t.Fatalf("toast = %q, want the close reported", view)
	}
	if labels := labelsIn(t, first, "tm-1"); len(labels) != 0 {
		t.Errorf("labels of tm-1 in the store the close began on = %v, want none", labels)
	}
}

// On the board a typed key is query text: the question of an asking agent
// would take it, so a toast names it there instead.
func TestTheFocusOfferIsAToastOnTheBoard(t *testing.T) {
	m, _ := askedShell(t, `{"status":"question","summary":"which file?"}`)
	for range 2 {
		m = pressKey(t, m, "esc")
	}
	if m.active != mode.Board || m.overlayOpen() {
		t.Fatalf("fixture is on %q with an overlay %v, want the board", m.active, m.overlayOpen())
	}

	m = updateOnly(m, agentAnsweredMsg{issueID: "tm-1", agent: startedOnTab9(), stoppedOn: agent.NeedsYou})

	if m.showActionModal {
		t.Fatal("the focus offer opened a question on the board")
	}
	if view := m.toast.View(); !strings.Contains(view, "Agents tab") {
		t.Errorf("toast = %q, want it to point to the Agents tab", view)
	}
}

// Agents of one rank are listed by the name the operator reads.
func TestDetailListsTheAgentsOfARankByTitle(t *testing.T) {
	zeta := agent.Agent{Ref: "demo:home:1", Session: "s-a", Label: "Tab 1", Name: "zeta", Status: agent.Idle}
	alpha := agent.Agent{Ref: "demo:home:2", Session: "s-b", Label: "Tab 2", Name: "alpha", Status: agent.Idle}
	m, _, _, _ := agentShell(t, zeta, alpha)
	m = withLinkLabel(t, m, "s-a")
	m = withLinkLabel(t, m, "s-b")

	if got := m.detail.LinkedAgents(); !reflect.DeepEqual(got, []agent.Agent{alpha, zeta}) {
		t.Errorf("Detail links %+v, want alpha first", got)
	}
}

// An agent that commented on two In Progress issues is linked to the first
// of the column on the Agents tab.
func TestAnAgentThatCommentedTwiceIsLinkedToTheFirstIssue(t *testing.T) {
	gw := fakes.NewTracked()
	seedInProgress(gw, "tm-1", "First", "task", 0)
	seedInProgress(gw, "tm-2", "Second", "task", 2)
	for _, id := range []string{"tm-1", "tm-2"} {
		gw.Memory.SeedComments(id, memoryrepo.Comment{
			ID: "c-" + id, Author: "claude", Session: "s-1", Body: "hello",
			CreatedAt: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		})
	}
	services, err := NewServicesWithLauncher(gw, config.Default(), &fakes.FakeLauncher{})
	if err != nil {
		t.Fatal(err)
	}
	services.Agents = &fakes.FakeAgentHost{Agents: []agent.Agent{runningOnTab3()}}
	services.ProjectRoot = t.TempDir()
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = pressKey(t, m, "shift+tab")
	m = send(t, m, agentPollTickMsg{})

	row, ok := m.agents.SelectedRow()
	if !ok || row.Issue == nil || row.Issue.ID != "tm-1" {
		t.Fatalf("the agent row is %+v, want it linked to tm-1", row)
	}
}
