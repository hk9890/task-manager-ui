package agents

import (
	"context"
	"errors"
	"log/slog"
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
		{ID: "tm-1", Status: "open", Labels: []string{"agent-session:s-1", "agent-session:s-gone", "backend"}},
		{ID: "tm-2", Status: "closed", Labels: []string{"agent-session:s-2", "agent-session:s-closed-gone"}},
		{ID: "tm-3", Status: "open", Labels: []string{"agent-session:s-1"}},
	}
	comments := []mode.CommentLink{{Issue: domain.IssueSummary{ID: "tm-4"}, Sessions: []string{"s-3"}}, {Issue: domain.IssueSummary{ID: "tm-5"}, Sessions: []string{"s-1"}}}

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
	m := NewModel(context.Background(), memory.New(), nil, keys(t), nil)
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
	m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{{ID: "tm-1", Title: "Fix", Status: "open", Labels: []string{"agent-session:s-1"}}}})
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Running), live("s-2", "2", agent.Idle)}, true, "", nil)

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
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Running)}, true, "", nil)
	m.Select("s-gone")
	m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{{ID: "tm-1", Status: "open", Labels: []string{"agent-session:s-gone"}}}})
	if row, _ := m.selectedRow(); row.Agent.Session != "s-gone" {
		t.Fatalf("selected %q, want the gone agent", row.Agent.Session)
	}
	// A later poll keeps the selection on that agent.
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Running), live("s-0", "0", agent.NeedsYou)}, true, "", nil)
	if row, _ := m.selectedRow(); row.Agent.Session != "s-gone" {
		t.Errorf("after a poll selected %q, want the gone agent", row.Agent.Session)
	}
}

// A failed poll empties the tab but keeps the agent the selection was on:
// the next poll that answers puts the selection back on it.
func TestAFailedPollKeepsTheSelectedAgent(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{{ID: "tm-2", Status: "open", Labels: []string{"agent-session:s-2"}}}})
	agents := []agent.Agent{live("s-1", "1", agent.Running), live("s-2", "2", agent.Idle)}
	m.SetAgents(agents, true, "", nil)
	m.Select("s-2")
	if row, _ := m.selectedRow(); row.Agent.Session != "s-2" {
		t.Fatalf("selected %q, want s-2", row.Agent.Session)
	}

	m.SetAgents(nil, false, "the agent host failed: timeout", nil)
	if len(m.rows) != 0 {
		t.Fatalf("rows after a failed poll = %v, want none", titles(m.rows))
	}
	m.SetAgents([]agent.Agent{live("s-0", "0", agent.NeedsYou), live("s-1", "1", agent.Running), live("s-2", "2", agent.Idle)}, true, "", nil)
	if row, _ := m.selectedRow(); row.Agent.Session != "s-2" {
		t.Errorf("after the poll answered again selected %q, want s-2", row.Agent.Session)
	}
}

// Select of an agent with no row on a settled tab keeps the selection where
// it was, and the selection then follows that agent when the rows reorder.
func TestSelectOfAnAgentWithoutARowKeepsTheSelection(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	m.Update(linkedIssuesMsg{})
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Idle), live("s-2", "2", agent.Idle)}, true, "", nil)
	m.Select("s-closed-gone")
	if row, _ := m.selectedRow(); row.Agent.Session != "s-1" || m.pending != "" {
		t.Fatalf("selected %q with %q pending, want s-1 and nothing pending", row.Agent.Session, m.pending)
	}
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Idle), live("s-2", "2", agent.NeedsYou)}, true, "", nil)
	if row, _ := m.selectedRow(); row.Agent.Session != "s-1" {
		t.Errorf("after the rows reordered selected %q, want s-1", row.Agent.Session)
	}
}

// Before the linked issues are read the rows are not in their last order:
// the selection stays on the top row, so once they come it is on the agent
// that needs the operator most, not on the one that was first before.
func TestTheFirstSelectionIsTheTopRowOnceTheRowsSettle(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	links := []mode.CommentLink{{Issue: domain.IssueSummary{ID: "tm-2", Status: "in_progress"}, Sessions: []string{"s-work"}}}
	m.SetAgents([]agent.Agent{live("s-7", "7", agent.NeedsYou), live("s-work", "work", agent.Running)}, true, "", links)
	if row, _ := m.selectedRow(); row.Agent.Session != "s-work" {
		t.Fatalf("fixture: before the read selected %q, want s-work on top", row.Agent.Session)
	}

	m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{{ID: "tm-1", Status: "open", Labels: []string{"agent-session:s-7"}}}})
	if row, _ := m.selectedRow(); row.Agent.Session != "s-7" {
		t.Fatalf("after the read selected %q, want s-7, the top row", row.Agent.Session)
	}
	// From here the selection follows its agent.
	m.SetAgents([]agent.Agent{live("s-7", "7", agent.Running), live("s-work", "work", agent.NeedsYou)}, true, "", links)
	if row, _ := m.selectedRow(); row.Agent.Session != "s-7" {
		t.Errorf("after the rows reordered selected %q, want s-7", row.Agent.Session)
	}
}

// A read of the linked issues that fails keeps the rows of the last one that
// succeeded and ends the read, so the next reload reads again.
func TestAFailedReadKeepsTheRowsAndReadsAgain(t *testing.T) {
	t.Parallel()
	m := NewModel(context.Background(), memory.New(), slog.New(slog.DiscardHandler), keys(t), nil)
	m.SetSize(80, 24)
	m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{{ID: "tm-1", Status: "open", Labels: []string{"agent-session:s-1"}}}})
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Running), live("s-2", "2", agent.Idle)}, true, "", nil)
	before := titles(m.rows)

	if m.Reload() == nil {
		t.Fatal("fixture: Reload started no read")
	}
	m.Update(linkedIssuesMsg{err: errors.New("store locked")})
	if got := titles(m.rows); !reflect.DeepEqual(got, before) {
		t.Errorf("rows after a failed read = %v, want %v", got, before)
	}
	if m.IsLoading() {
		t.Error("the failed read is still in flight")
	}
	if m.Reload() == nil {
		t.Error("Reload after a failed read starts no read")
	}
}

// One click selects a row and a second brings its agent to the front.
func TestASecondClickBringsTheAgentToTheFront(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Running), live("s-2", "2", agent.Idle)}, true, "", nil)
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
	m.SetAgents(nil, false, "no agent host: agents are off", nil)
	testui.AssertContainsAll(t, testui.Downsample(m.View(0)), "No agents", "no agent host: agents are off")
}

// The read keeps the issues with a link label, closed ones included.
func TestInitReadsTheIssuesWithALinkLabel(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	repo.Seed(memory.Issue{ID: "tm-1", Title: "linked", Status: "open", Labels: []string{"agent-session:s-1"}})
	repo.Seed(memory.Issue{ID: "tm-2", Title: "plain", Status: "open"})
	m := NewModel(context.Background(), repo, nil, keys(t), nil)
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

// A dropped agent leaves the tab at once, and a read begun before its unlink
// that still carries its link label draws it no gone row. A read without the
// label ends the drop: a later label of the session links it again.
func TestADroppedAgentIsNoGoneRowUntilItsUnlinkLands(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	linked := []domain.IssueSummary{{ID: "tm-1", Status: "open", Labels: []string{"agent-session:s-1"}}}
	m.Update(linkedIssuesMsg{issues: linked})
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Idle), live("s-2", "2", agent.Idle)}, true, "", nil)

	m.DropAgent(live("s-1", "1", agent.Idle))
	if got, want := titles(m.rows), []string{"Tab 2@"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows after the drop = %v, want %v", got, want)
	}
	m.SetAgents([]agent.Agent{live("s-2", "2", agent.Idle)}, true, "", nil)
	m.Update(linkedIssuesMsg{issues: linked})
	if got, want := titles(m.rows), []string{"Tab 2@"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rows after a stale read = %v, want %v", got, want)
	}

	m.Update(linkedIssuesMsg{})
	m.Update(linkedIssuesMsg{issues: linked})
	if got, want := titles(m.rows), []string{"Session s-1@tm-1", "Tab 2@"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rows after the label came back = %v, want %v", got, want)
	}
}
