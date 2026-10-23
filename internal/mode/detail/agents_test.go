package detail

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/revier"
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
	m.SetLiveAgents(map[string]revier.Agent{
		"s-idle":   {Panel: "1", Status: revier.StatusIdle, Session: "s-idle"},
		"s-run-b":  {Panel: "5", Status: revier.StatusRunning, Session: "s-run-b"},
		"s-run-a":  {Panel: "4", Status: revier.StatusRunning, Session: "s-run-a"},
		"s-attn":   {Panel: "9", Status: revier.StatusAttention, Session: "s-attn"},
		"s-silent": {Panel: "2", Status: revier.StatusAttention, Session: "s-silent"},
	})

	var panels []string
	for _, agent := range m.LinkedAgents() {
		panels = append(panels, agent.Panel)
	}
	if want := []string{"9", "4", "5", "1"}; !reflect.DeepEqual(panels, want) {
		t.Errorf("linked panels = %v, want %v", panels, want)
	}

	m.Reset()
	m.ApplyLoadedDetail("tm-1", issueWithComments(1))
	m.Detail.Comments[0].Session = "s-attn"
	if linked := m.LinkedAgents(); len(linked) != 1 || linked[0].Panel != "9" {
		t.Errorf("after Reset the linked agents are %+v, want the live agents kept", linked)
	}
}

// The agent button is drawn on every loaded issue, the open button only with a
// linked agent, and a click on each sends the request its key sends.
func TestClicksOnTheAgentButtonsSendTheirRequests(t *testing.T) {
	t.Parallel()

	m := commentsBySessions(t, "s-1")
	view := testui.Downsample(m.View(commentsWidth, commentsHeight, false, 0))
	if !strings.Contains(view, "agent alt+i") || strings.Contains(view, "open alt+o") {
		t.Fatalf("without a live agent the ID line is not agent and edit:\n%s", view)
	}

	m.SetLiveAgents(map[string]revier.Agent{"s-1": {Panel: "3", Status: revier.StatusRunning, Session: "s-1"}})
	click := func(text string) tea.Msg {
		t.Helper()
		x, y := testui.FindCell(t, m.View(commentsWidth, commentsHeight, false, 0), text)
		return m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: mouseStart}, commentsWidth, commentsHeight)()
	}
	testui.AssertActionRequest(t, click("agent alt+i"), mode.Detail, mode.ActionLaunchAgent)
	testui.AssertActionRequest(t, click("open alt+o"), mode.Detail, mode.ActionFocusAgent)
	if view := testui.Downsample(m.View(commentsWidth, commentsHeight, false, 0)); !strings.Contains(view, "Tab 3: running") {
		t.Errorf("the Metadata pane does not list the linked agent:\n%s", view)
	}
}

// A click aimed at open after the linked agent is gone starts no agent: agent
// does not move into the cells open held, in the stacked and the three-pane
// layouts.
func TestAClickWhereOpenWasStartsNoAgent(t *testing.T) {
	t.Parallel()

	for _, width := range []int{commentsWidth, 124, 160} {
		m := commentsBySessions(t, "s-1")
		m.SetLiveAgents(map[string]revier.Agent{"s-1": {Panel: "3", Status: revier.StatusRunning, Session: "s-1"}})
		x, y := testui.FindCell(t, m.View(width, commentsHeight, false, 0), "open alt+o")

		m.SetLiveAgents(nil)
		for _, column := range []int{x - 1, x, x + len("open alt+o")} {
			cmd := m.HandleMouse(mode.MouseMsg{Kind: mode.MouseClick, X: column, Y: y, At: mouseStart}, width, commentsHeight)
			if cmd == nil {
				continue
			}
			if request, ok := cmd().(mode.ActionRequestMsg); ok && request.Action == mode.ActionLaunchAgent {
				t.Errorf("width %d: a click at column %d, where open was, starts an agent", width, column)
			}
		}
	}
}
