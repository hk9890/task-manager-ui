package app

// An agent start outlives a store switch: its result is reported, its link
// label goes to the store it began on, and nothing of it reaches the new
// store's surfaces. The end of an asked agent's turn leaves the reload to the
// watch on a watched store.

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/repository"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// switchToOtherStore binds m to a second store with the same agent host and
// returns that store's repository.
func switchToOtherStore(t *testing.T, m *Model) *fakes.TrackedRepository {
	t.Helper()
	other := fakes.NewTracked()
	seedReady(other, "tm-1", "Other store", "task", 1)
	services, err := NewServicesWithLauncher(other, config.Default(), &fakes.FakeLauncher{})
	if err != nil {
		t.Fatalf("NewServicesWithLauncher: %v", err)
	}
	services.Agents = m.services.Agents
	services.ProjectRoot = t.TempDir()
	m.bindStore(services)
	return other
}

func labelsIn(t *testing.T, repo repository.Repository, id string) []string {
	t.Helper()
	issue, err := repo.Issue(context.Background(), id)
	if err != nil {
		t.Fatalf("Issue(%s): %v", id, err)
	}
	return issue.Summary.Labels
}

func TestAnAgentStartIsReportedAfterAStoreSwitch(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	first := m.services.Repo
	m = pressKey(t, m, "alt+i")
	next, start := m.Update(testui.Press("enter"))
	m = next.(Model)

	other := switchToOtherStore(t, &m)
	mark := other.CallCount()
	m = applyMessages(t, m, runBatch(start))

	if view := m.toast.View(); !strings.Contains(view, "Started Tab 9 on tm-1") {
		t.Fatalf("toast = %q, want the start reported", view)
	}
	for _, method := range []fakes.Method{fakes.MethodUpdateIssue, fakes.MethodDashboard, fakes.MethodIssue} {
		if got := other.CallCountSince(mark, method); got != 0 {
			t.Errorf("the start called %s %d times on the new store, want 0", method, got)
		}
	}
	if labels := labelsIn(t, first, "tm-1"); !reflect.DeepEqual(labels, []string{"agent-session:s-9"}) {
		t.Errorf("labels of tm-1 in the store the start began on = %v, want [agent-session:s-9]", labels)
	}
	if labels := labelsIn(t, other, "tm-1"); len(labels) != 0 {
		t.Errorf("labels of tm-1 in the new store = %v, want none", labels)
	}
}

func TestAnAskedAgentIsReportedAndNotWaitedOnAfterAStoreSwitch(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	first := m.services.Repo
	m, submit := askCmdOnTitle(t, m, "make the title shorter")
	next, ask := m.Update(submit())
	m = next.(Model)

	other := switchToOtherStore(t, &m)
	mark := other.CallCount()
	m = applyMessages(t, m, runBatch(ask))

	if view := m.toast.View(); !strings.Contains(view, "Tab 9 works on tm-1 in the store you left; its answer is not reported") {
		t.Fatalf("toast = %q, want the start reported", view)
	}
	if got := hostCalls(client, "Wait", "Read"); len(got) != 0 {
		t.Errorf("calls = %+v, want no wait and no read", got)
	}
	if labels := labelsIn(t, first, "tm-1"); !reflect.DeepEqual(labels, []string{"agent-session:s-9"}) {
		t.Errorf("labels of tm-1 in the store the ask began on = %v, want [agent-session:s-9]", labels)
	}
	if got := other.CallCountSince(mark, fakes.MethodUpdateIssue); got != 0 {
		t.Errorf("the ask wrote %d times to the new store, want 0", got)
	}
}

// On a watched store the watch reloads after the agent's writes: the end of
// the turn polls the agents and reads no issue itself.
func TestTheEndOfTheTurnLeavesTheReloadToTheWatch(t *testing.T) {
	repo := newWatchedRepository()
	m, _ := watchedModel(t, repo, RuntimeOptions{})
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = pressKey(t, m, "enter")
	if !m.storeWatched() || m.detail.Detail.Summary.ID != "tm-1" {
		t.Fatalf("fixture: watched %v, Detail on %q", m.storeWatched(), m.detail.Detail.Summary.ID)
	}
	client := &fakes.FakeAgentHost{}
	m.services.Agents = client

	mark := repo.CallCount()
	m = send(t, m, agentAnsweredMsg{issueID: "tm-1", agent: startedOnTab9(), stoppedOn: agent.Idle, reply: `{"status":"done","summary":"ok"}`})

	if got := storeReads(repo, mark); got != 0 {
		t.Errorf("the end of the turn read the store %d times on a watched store, want 0", got)
	}
	if got := hostCalls(client, "List"); len(got) != 1 {
		t.Errorf("List calls = %+v, want one poll", got)
	}
	if view := m.toast.View(); !strings.Contains(view, "Tab 9 on tm-1 is done: ok") {
		t.Errorf("toast = %q, want the summary", view)
	}
}
