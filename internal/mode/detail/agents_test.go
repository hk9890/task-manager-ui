package detail

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// commentsBySessions is a loaded detail whose comments were written by these
// sessions, in order; "" is a comment no agent wrote.
func commentsBySessions(t *testing.T, sessions ...string) *Model {
	t.Helper()
	m := commentsDetail(t, len(sessions))
	for idx, session := range sessions {
		m.Detail.Comments[idx].Session = session
	}
	return m
}

// The linked agents are the live agents whose session wrote a comment, each
// once, ordered attention, running, idle, then by panel. A live agent that
// wrote no comment and a session with no live agent are not linked.
func TestLinkedAgentsAreTheLiveAgentsThatCommentedInOrderOfNeed(t *testing.T) {
	t.Parallel()

	m := commentsBySessions(t, "s-idle", "s-run-b", "", "s-gone", "s-run-a", "s-attn", "s-idle")
	m.SetLiveAgents(map[string]agent.Agent{
		"s-idle":   {Label: "Tab 1", Status: agent.Idle, Session: "s-idle"},
		"s-run-b":  {Label: "Tab 5", Status: agent.Running, Session: "s-run-b"},
		"s-run-a":  {Label: "Tab 4", Status: agent.Running, Session: "s-run-a"},
		"s-attn":   {Label: "Tab 9", Status: agent.NeedsYou, Session: "s-attn"},
		"s-silent": {Label: "Tab 2", Status: agent.NeedsYou, Session: "s-silent"},
	})

	var labels []string
	for _, linked := range m.LinkedAgents() {
		labels = append(labels, linked.Label)
	}
	if want := []string{"Tab 9", "Tab 4", "Tab 5", "Tab 1"}; !reflect.DeepEqual(labels, want) {
		t.Errorf("linked labels = %v, want %v", labels, want)
	}

	m.Reset()
	m.ApplyLoadedDetail("tm-1", issueWithComments(1))
	m.Detail.Comments[0].Session = "s-attn"
	if linked := m.LinkedAgents(); len(linked) != 1 || linked[0].Label != "Tab 9" {
		t.Errorf("after Reset the linked agents are %+v, want the live agents kept", linked)
	}
}

// The agent button is drawn on every loaded issue, and a click on it sends
// the request its key sends.
func TestAClickOnTheAgentButtonSendsItsRequest(t *testing.T) {
	t.Parallel()

	m := commentsBySessions(t, "s-1")
	m.SetLiveAgents(map[string]agent.Agent{"s-1": {Label: "Tab 3", Status: agent.Running, Session: "s-1"}})
	x, y := testui.FindCell(t, m.View(commentsWidth, commentsHeight, false, 0), "agent alt+i")
	request := m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: mouseStart}, commentsWidth, commentsHeight)()
	testui.AssertActionRequest(t, request, mode.Detail, mode.ActionLaunchAgent)
	if view := testui.Downsample(m.View(commentsWidth, commentsHeight, false, 0)); !strings.Contains(view, "Tab 3: running") {
		t.Errorf("the Details pane does not list the linked agent:\n%s", view)
	}
}

// A click on the row of an agent in the Details pane asks the shell for that
// agent, and gives the pane no focus; the wheel there scrolls nothing. The
// row under the pointer is lit.
func TestAClickOnAnAgentRowAsksForThatAgent(t *testing.T) {
	testui.ForceTrueColor(t)

	m := commentsBySessions(t, "s-1", "s-2")
	m.SetLiveAgents(map[string]agent.Agent{
		"s-1": {Label: "Tab 3", Status: agent.Running, Session: "s-1"},
		"s-2": {Label: "Tab 4", Status: agent.Idle, Session: "s-2"},
	})
	x, y := testui.FindCell(t, m.View(commentsWidth, commentsHeight, false, 0), "Tab 4: idle")

	m.HandleMouse(mode.MouseMsg{Kind: mode.MouseMove, X: x, Y: y, At: mouseStart}, commentsWidth, commentsHeight)
	view := m.View(commentsWidth, commentsHeight, false, 0)
	if testui.RowBand(t, view, "Tab 4: idle") == "" || testui.RowBand(t, view, "Tab 3: running") != "" {
		t.Errorf("the pointer on Tab 4 does not light that row alone:\n%s", view)
	}

	focus := m.FocusPane
	cmd := m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: mouseStart}, commentsWidth, commentsHeight)
	if cmd == nil {
		t.Fatal("a click on an agent row asks for nothing")
	}
	request, ok := cmd().(mode.ActionRequestMsg)
	if !ok || request.Action != mode.ActionSelectAgent || request.Agent.Session != "s-2" {
		t.Errorf("a click on Tab 4 sent %#v, want ActionSelectAgent for s-2", request)
	}
	if m.FocusPane != focus {
		t.Errorf("a click on an agent row moved the focus to %d", m.FocusPane)
	}

	if cmd := m.HandleMouse(mode.MouseMsg{Kind: mode.MouseWheelDown, X: x, Y: y, At: mouseStart}, commentsWidth, commentsHeight); cmd != nil || m.ContentScrollOffset != 0 {
		t.Errorf("the wheel on an agent row: cmd %v, content offset %d", cmd != nil, m.ContentScrollOffset)
	}
}
