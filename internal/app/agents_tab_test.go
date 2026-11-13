package app

// The Agents tab: every agent of the project, linked to an issue or not, and
// the keys that act on the selected one through the agent host.

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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

// requestFrom presses key on the Agents tab, hands the shell the request the
// tab sends, and returns the command the shell answers with, not run: the
// host call in it stays in flight.
func requestFrom(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(testui.Press(key))
	m = next.(Model)
	for _, msg := range runBatch(cmd) {
		if request, ok := unscoped(msg).(mode.ActionRequestMsg); ok {
			next, cmd = m.Update(request)
			return next.(Model), cmd
		}
	}
	t.Fatalf("%s sent no request", key)
	return m, nil
}

// withGoneRow is the Agents tab with a gone agent, s-gone-0123 on tm-1, as
// its second row, selected.
func withGoneRow(t *testing.T) (Model, *fakes.FakeAgentHost) {
	t.Helper()
	m, client := agentsTab(t)
	m = withLinkLabel(t, m, "s-gone-0123")
	m = send(t, m, agentPollTickMsg{})
	m = pressKey(t, m, "down")
	if row, _ := m.agents.SelectedRow(); row.Agent.Status != agent.Gone {
		t.Fatalf("fixture: selected %+v, want the gone agent", row.Agent)
	}
	return m, client
}

// A resume says it started and turns the spinner until it ends: a harness
// takes seconds to come up. A second Enter meanwhile opens no second tab on
// the same conversation.
func TestAResumeInFlightIsShownAndNotStartedTwice(t *testing.T) {
	m, client := withGoneRow(t)
	client.Resumed = agent.Agent{Ref: "demo:home:5", Session: "s-gone-0123", Label: "Tab 5", Status: agent.Idle}

	m, resume := requestFrom(t, m, "enter")
	if !strings.Contains(m.toast.View(), "Resuming Session s-gone-0…") {
		t.Errorf("toast = %q, want the resume reported as started", m.toast.View())
	}
	if !m.workInFlight() {
		t.Error("the spinner does not turn while the resume is in flight")
	}
	m, again := requestFrom(t, m, "enter")
	if !strings.Contains(m.toast.View(), "Already resuming Session s-gone-0") {
		t.Errorf("toast = %q, want the second resume refused", m.toast.View())
	}

	m = applyMessages(t, m, append(runBatch(resume), runBatch(again)...))
	if got := hostCalls(client, "Resume"); len(got) != 1 {
		t.Fatalf("Resume calls = %+v, want one", got)
	}
	if m.workInFlight() {
		t.Error("the spinner still turns after the resume ended")
	}
	if !strings.Contains(m.toast.View(), "Resumed Session s-gone-0") {
		t.Errorf("toast = %q, want the resume reported", m.toast.View())
	}
	// Once it ended, the session can be resumed again.
	m, resume = requestFrom(t, m, "enter")
	applyMessages(t, m, runBatch(resume))
	if got := hostCalls(client, "Resume"); len(got) != 2 {
		t.Errorf("Resume calls after the first ended = %+v, want two", got)
	}
}

// While the Agent pane is on screen the legend names the keys that scroll it;
// on a screen too narrow for the pane it does not.
func TestTheAgentsTabLegendNamesThePaneKeysWhileThePaneIsShown(t *testing.T) {
	m, _ := agentsTab(t)
	footer := func(m Model) string { return testui.Downsample(m.renderFooter()) }

	m = send(t, m, tea.WindowSizeMsg{Width: 200, Height: 40})
	if legend := footer(m); !strings.Contains(legend, "pgdown/pgup message") {
		t.Errorf("legend beside the pane = %q, want pgdown/pgup message", legend)
	}
	m = send(t, m, tea.WindowSizeMsg{Width: 90, Height: 40})
	if legend := footer(m); strings.Contains(legend, "message") {
		t.Errorf("legend without the pane = %q, names the pane keys", legend)
	}
}

// The legend names the keys that act on the selected row as they act on it:
// a gone agent is resumed and unlinked, an agent linked to no issue has no
// issue to open, and without a row no key acts on one.
func TestTheAgentsTabLegendFollowsTheSelectedRow(t *testing.T) {
	m, _ := withGoneRow(t)
	footer := func(m Model) string { return testui.Downsample(m.renderFooter()) }

	m = pressKey(t, m, "up")
	testui.AssertContainsAll(t, footer(m), "enter focus", "alt+x close", "alt+o issue")
	m = pressKey(t, m, "down")
	testui.AssertContainsAll(t, footer(m), "enter resume", "alt+x unlink", "alt+o issue")
	m = pressKey(t, m, "down")
	if legend := footer(m); !strings.Contains(legend, "enter focus") || strings.Contains(legend, "alt+o") {
		t.Errorf("legend on an agent linked to none = %q, want enter focus and no alt+o", legend)
	}

	empty, _, _, _ := agentShellAt(t, "")
	empty = pressKey(t, empty, "esc")
	empty = pressKey(t, empty, "shift+tab")
	legend := footer(empty)
	for _, key := range []string{"enter", "alt+x", "alt+o"} {
		if strings.Contains(legend, key) {
			t.Errorf("legend of an empty tab = %q, names %s", legend, key)
		}
	}
}

// Without a project path, or with one that is not accessible, the tab says
// why it shows no agent, the host is not asked, and the agent dialog and Ask
// start nothing.
func TestAgentsAreOffWithoutAnAccessibleProjectPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "removed")
	for _, tc := range []struct {
		name, root, empty, toast string
	}{
		{"no project path", "", "the store has no project path", "Agents are off: the store has no project path"},
		{"path not accessible", missing, "the project path is not accessible", "Agents are off: project path not accessible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, client, _, _ := agentShellAt(t, tc.root, runningOnTab3())

			m = pressKey(t, m, "alt+i")
			if m.selMenu != nil || !strings.Contains(m.toast.View(), tc.toast) {
				t.Errorf("alt+i opened the dialog %v, toast %q; want the toast %q", m.selMenu != nil, m.toast.View(), tc.toast)
			}
			m = applyMessages(t, m, runBatch(m.askAgent(agentAsk{issue: m.detail.Detail.Summary, text: "Ready first"}, "shorter")))
			if !strings.Contains(m.toast.View(), tc.toast) {
				t.Errorf("Ask: toast %q, want %q", m.toast.View(), tc.toast)
			}

			m = pressKey(t, m, "esc")
			m = pressKey(t, m, "shift+tab")
			testui.AssertContainsAll(t, plainShell(m), "No agents", tc.empty)
			if calls := client.Calls(); len(calls) != 0 {
				t.Errorf("the host was called: %+v", calls)
			}
		})
	}
}

// Ask without an agent host starts nothing and says so: the menu offers no
// Ask then, so this is the guard behind it.
func TestAskWithoutAHostSaysAgentsAreOff(t *testing.T) {
	m, _, _, _ := agentShell(t)
	m.services.Agents = nil
	m = applyMessages(t, m, runBatch(m.askAgent(agentAsk{issue: m.detail.Detail.Summary, text: "Ready first"}, "shorter")))
	if !strings.Contains(m.toast.View(), "No agent host: agents are off") {
		t.Errorf("toast = %q, want agents off", m.toast.View())
	}
	if m.agentStarts != 0 {
		t.Errorf("agent starts in flight = %d, want none", m.agentStarts)
	}
}

// An unlink whose search or write fails is reported as an error, and the
// link label stays: the gone row comes back on the next read.
func TestAFailedUnlinkIsReportedAndKeepsTheLabel(t *testing.T) {
	for _, tc := range []struct {
		method fakes.Method
		want   string
	}{
		{fakes.MethodUpdateIssue, "Could not unlink Session s-gone-0: tm-1: store locked"},
		{fakes.MethodSearch, "Could not unlink Session s-gone-0: store locked"},
	} {
		t.Run(string(tc.method), func(t *testing.T) {
			m, _ := withGoneRow(t)
			failing := fakes.NewErrorInjecting(m.services.Repo)
			failing.SetError(tc.method, errors.New("store locked"))
			m.services.Repo = failing

			m = pressKey(t, m, "alt+x")
			if view := m.toast.View(); !strings.Contains(view, tc.want) {
				t.Errorf("toast = %q, want %q", view, tc.want)
			}
			if got, want := labelsOf(t, m, "tm-1"), []string{"agent-session:s-1", "agent-session:s-gone-0123"}; !reflect.DeepEqual(got, want) {
				t.Errorf("labels of tm-1 = %v, want %v", got, want)
			}
		})
	}
}

// A session that makes no valid label was never linked: the unlink searches
// and writes nothing.
func TestTheUnlinkOfASessionThatMakesNoLabelWritesNothing(t *testing.T) {
	repo := fakes.NewTracked()
	unlinked, err := unlinkAgent(context.Background(), repo, "Not A Session!")
	if unlinked != nil || err != nil {
		t.Errorf("unlinkAgent = %v, %v; want nothing", unlinked, err)
	}
	if calls := repo.Calls(); len(calls) != 0 {
		t.Errorf("repository calls = %+v, want none", calls)
	}
}
