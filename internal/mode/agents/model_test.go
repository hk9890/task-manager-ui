package agents

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository/memory"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uiagents "github.com/hk9890/task-manager-ui/internal/ui/agents"
)

func keys(t *testing.T) config.ResolvedKeyBindings {
	t.Helper()
	resolved, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func live(session, label string, status agent.Status) agent.Agent {
	return agent.Agent{Ref: "demo:home:" + label, Session: session, Label: "Tab " + label, Status: status}
}

func titles(rows []uiagents.Row) []string {
	var out []string
	for _, row := range rows {
		id := ""
		if row.Issue != nil {
			id = row.Issue.ID
		}
		out = append(out, row.Agent.Title()+"@"+id)
	}
	return out
}

// An agent is linked by a link label on any issue or by a comment on an In
// Progress issue, the label first; a label no live agent holds is gone on an
// open issue only, and only while the live agents are known. Every other live
// agent is not linked. Each group puts the agent that needs the operator most
// first.
func TestBuildRowsGroupsTheAgentsByTheirLink(t *testing.T) {
	t.Parallel()
	agents := []agent.Agent{
		live("s-1", "1", agent.Idle),
		live("s-2", "2", agent.NeedsYou),
		live("s-3", "3", agent.Running),
		live("s-4", "4", agent.Idle),
		{Ref: "demo:home:9", Label: "Tab 9", Status: agent.Idle},
	}
	issues := []domain.IssueSummary{
		{ID: "tm-1", Status: "open", Labels: []string{"agent:s-1", "agent:s-gone", "backend"}},
		{ID: "tm-2", Status: "closed", Labels: []string{"agent:s-2", "agent:s-closed-gone"}},
		{ID: "tm-3", Status: "open", Labels: []string{"agent:s-1"}},
	}
	comments := map[string]domain.IssueSummary{"s-3": {ID: "tm-4"}, "s-1": {ID: "tm-5"}}

	got := titles(buildRows(agents, true, issues, comments))
	want := []string{"Tab 2@tm-2", "Tab 3@tm-4", "Tab 1@tm-1", "Session s-gone@tm-1", "Tab 4@", "Tab 9@"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v\nwant %v", got, want)
	}

	if got := titles(buildRows(nil, false, issues, comments)); len(got) != 0 {
		t.Errorf("rows without the live agents = %v, want none", got)
	}
}

func newTab(t *testing.T) *Model {
	t.Helper()
	m := NewModel(context.Background(), memory.New(), nil, keys(t))
	m.SetSize(80, 24)
	return m
}

func request(t *testing.T, m *Model, key string) mode.ActionRequestMsg {
	t.Helper()
	cmd := m.Update(testui.Press(key))
	if cmd == nil {
		t.Fatalf("%s asks for nothing", key)
	}
	msg, ok := cmd().(mode.ActionRequestMsg)
	if !ok {
		t.Fatalf("%s sent %T, want an ActionRequestMsg", key, msg)
	}
	return msg
}

// Enter, the close key and the issue key ask the shell for the selected
// agent's action; the issue key opens the issue the agent is linked to.
func TestTheKeysActOnTheSelectedAgent(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{{ID: "tm-1", Title: "Fix", Status: "open", Labels: []string{"agent:s-1"}}}})
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Running), live("s-2", "2", agent.Idle)}, true, "")

	if got := request(t, m, "enter"); got.Action != mode.ActionOpenAgent || got.Agent.Session != "s-1" {
		t.Errorf("enter sent %#v", got)
	}
	if got := request(t, m, "alt+x"); got.Action != mode.ActionCloseAgent || got.Agent.Session != "s-1" {
		t.Errorf("alt+x sent %#v", got)
	}
	if got := request(t, m, "alt+o"); got.Action != mode.ActionOpenDetail || got.Selection == nil || got.Selection.Issue.ID != "tm-1" {
		t.Errorf("alt+o sent %#v", got)
	}

	cmd := m.Update(testui.Press("down"))
	if msg, ok := cmd().(mode.SelectionChangedMsg); !ok || msg.Mode != mode.Agents || msg.Selection != nil {
		t.Errorf("moving to the agent linked to none announced %#v", msg)
	}
	if got := request(t, m, "alt+o"); got.Action != mode.ActionOpenDetail || got.Selection != nil {
		t.Errorf("alt+o on an agent linked to none sent %#v", got)
	}
}

// Select puts the selection on the agent, also when its row comes later:
// a gone agent's row needs the read of the linked issues.
func TestSelectWaitsForTheRowOfTheAgent(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Running)}, true, "")
	m.Select("s-gone")
	m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{{ID: "tm-1", Status: "open", Labels: []string{"agent:s-gone"}}}})
	if row, _ := m.selectedRow(); row.Agent.Session != "s-gone" {
		t.Fatalf("selected %q, want the gone agent", row.Agent.Session)
	}
	// A later poll keeps the selection on that agent.
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Running), live("s-0", "0", agent.NeedsYou)}, true, "")
	if row, _ := m.selectedRow(); row.Agent.Session != "s-gone" {
		t.Errorf("after a poll selected %q, want the gone agent", row.Agent.Session)
	}
}

// One click selects a row and a second brings its agent to the front.
func TestASecondClickBringsTheAgentToTheFront(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Running), live("s-2", "2", agent.Idle)}, true, "")
	x, y := testui.FindCell(t, m.View(0), "Tab 2")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.Update(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: at})
	if row, _ := m.selectedRow(); row.Agent.Session != "s-2" {
		t.Fatalf("the click selected %q, want s-2", row.Agent.Session)
	}
	cmd := m.Update(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: at.Add(100 * time.Millisecond)})
	if cmd == nil {
		t.Fatal("the second click asks for nothing")
	}
	if got, ok := cmd().(mode.ActionRequestMsg); !ok || got.Action != mode.ActionOpenAgent || got.Agent.Session != "s-2" {
		t.Errorf("the second click sent %#v", got)
	}
}

// Without an agent the tab says why: the reason the shell gave, or that the
// host has not answered yet.
func TestAnEmptyTabSaysWhy(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	testui.AssertContainsAll(t, testui.Downsample(m.View(0)), "No agents", "waiting for the agent host")
	m.SetAgents(nil, false, "no agent host: agents are off")
	testui.AssertContainsAll(t, testui.Downsample(m.View(0)), "No agents", "no agent host: agents are off")
}

// The read keeps the issues with a link label, closed ones included.
func TestInitReadsTheIssuesWithALinkLabel(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	repo.Seed(memory.Issue{ID: "tm-1", Title: "linked", Status: "open", Labels: []string{"agent:s-1"}})
	repo.Seed(memory.Issue{ID: "tm-2", Title: "plain", Status: "open"})
	m := NewModel(context.Background(), repo, nil, keys(t))
	cmd := m.Init()
	if !m.IsLoading() || m.Reload() != nil {
		t.Fatal("Init does not hold the read in flight")
	}
	msg := cmd().(linkedIssuesMsg)
	if len(msg.issues) != 1 || msg.issues[0].ID != "tm-1" {
		t.Errorf("the read returned %+v, want tm-1 alone", msg.issues)
	}
	m.Update(msg)
	if m.IsLoading() {
		t.Error("the read is still in flight after its result")
	}
}
