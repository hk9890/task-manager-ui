package agents

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository/memory"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// splitTab is a tab wide enough for the Agent pane, entered, whose reads
// count into reads and answer "said <key>".
func splitTab(t *testing.T, reads map[string]int) *Model {
	t.Helper()
	read := func(_ context.Context, a agent.Agent) (string, error) {
		reads[a.Key()]++
		return "said " + a.Key(), nil
	}
	m := NewModel(context.Background(), memory.New(), nil, keys(t), read)
	m.SetSize(140, 30)
	m.SetShown(true)
	return m
}

// settle runs cmd and every command it leads to: the pause of a read ends at
// once, and the read answers.
func settle(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	for _, msg := range testui.DrainCmd(cmd) {
		switch msg.(type) {
		case readDueMsg, messageReadMsg:
			settle(t, m, m.Update(msg))
		}
	}
}

// Typing filters the agents on their title, label and activity and the ID
// and title of their issue; the selection stays on its agent while it
// matches, else takes the first match; ClearQuery shows every agent again.
func TestTheQueryFiltersTheAgents(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{{ID: "tm-7", Title: "Fix the board", Status: "open", Labels: []string{"agent-session:s-1"}}}})
	editing := live("s-2", "2", agent.Running)
	editing.Activity = "Editing docs"
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Idle), editing, live("s-3", "3", agent.Idle)}, true, "", nil)

	for _, tc := range []struct{ query, want string }{
		{"board", "Tab 1@tm-7"},
		{"TM-7", "Tab 1@tm-7"},
		{"docs", "Tab 2@"},
		{"tab 3", "Tab 3@"},
	} {
		m.ClearQuery()
		m.Update(tea.PasteMsg{Content: tc.query})
		if got := titles(m.rows); len(got) != 1 || got[0] != tc.want {
			t.Errorf("%q matches %v, want [%s]", tc.query, got, tc.want)
		}
	}

	m.ClearQuery()
	m.move(2) // Tab 3
	m.Update(tea.PasteMsg{Content: "tab"})
	if row, _ := m.selectedRow(); row.Agent.Session != "s-3" {
		t.Errorf("a query the selected agent matches moved the selection to %q", row.Agent.Session)
	}
	m.Update(tea.PasteMsg{Content: " 2"})
	if row, _ := m.selectedRow(); row.Agent.Session != "s-2" {
		t.Errorf("a query the selected agent misses selected %q, want the first match", row.Agent.Session)
	}
	if cleared, _ := m.ClearQuery(); !cleared || len(m.rows) != 3 || m.HasQuery() {
		t.Errorf("ClearQuery left %d rows, query %q", len(m.rows), m.query.Text())
	}
	if !m.TakesKey(testui.Runes("j")) || m.TakesKey(testui.Press("alt+x")) {
		t.Errorf("TakesKey does not follow the query keys")
	}
}

// The last message is read once the selection rests on an agent, once per
// activity: a poll with the same activity reads nothing, another activity
// reads again, and a gone agent is never read.
func TestTheLastMessageIsReadOncePerActivity(t *testing.T) {
	t.Parallel()
	reads := map[string]int{}
	m := splitTab(t, reads)
	running := live("s-1", "1", agent.Running)
	running.Activity = "Editing"
	settle(t, m, m.SetAgents([]agent.Agent{running, live("s-2", "2", agent.Idle)}, true, "", nil))
	if reads["s-1"] != 1 || m.pane().Message != "said s-1" {
		t.Fatalf("reads %v, pane %+v; want s-1 read once", reads, m.pane())
	}

	settle(t, m, m.SetAgents([]agent.Agent{running, live("s-2", "2", agent.Idle)}, true, "", nil))
	if reads["s-1"] != 1 {
		t.Errorf("a poll with the same activity read again: %v", reads)
	}
	running.Activity = "Testing"
	settle(t, m, m.SetAgents([]agent.Agent{running, live("s-2", "2", agent.Idle)}, true, "", nil))
	if reads["s-1"] != 2 {
		t.Errorf("a new activity did not read again: %v", reads)
	}

	settle(t, m, m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{{ID: "tm-1", Status: "open", Labels: []string{"agent-session:s-gone"}}}}))
	settle(t, m, m.Select("s-gone"))
	if reads["s-gone"] != 0 {
		t.Errorf("a gone agent was read: %v", reads)
	}
}

// Moving on before the pause ends reads only the agent the selection rests
// on; a narrow screen, which draws no pane, reads nothing.
func TestOnlyTheRestingSelectionIsRead(t *testing.T) {
	t.Parallel()
	reads := map[string]int{}
	m := splitTab(t, reads)
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Idle), live("s-2", "2", agent.Idle), live("s-3", "3", agent.Idle)}, true, "", nil)
	first := m.move(1)
	settle(t, m, m.move(1))
	settle(t, m, first)
	if reads["s-2"] != 0 || reads["s-3"] != 1 {
		t.Errorf("reads %v, want s-3 only", reads)
	}

	narrow := splitTab(t, reads)
	narrow.SetSize(90, 30)
	narrow.SetShown(true)
	settle(t, narrow, narrow.SetAgents([]agent.Agent{live("s-9", "9", agent.Idle)}, true, "", nil))
	if reads["s-9"] != 0 {
		t.Errorf("a narrow screen read %v", reads)
	}
}

// The page keys scroll the pane while it is drawn, and the wheel over it
// scrolls it; a new selection starts it at the top.
func TestThePageKeysScrollThePane(t *testing.T) {
	t.Parallel()
	m := NewModel(context.Background(), memory.New(), nil, keys(t), func(context.Context, agent.Agent) (string, error) {
		return strings.Repeat("line\n\n", 60), nil
	})
	m.SetSize(140, 30)
	m.SetShown(true)
	settle(t, m, m.SetAgents([]agent.Agent{live("s-1", "1", agent.Idle), live("s-2", "2", agent.Idle)}, true, "", nil))

	m.Update(testui.Press("pgdown"))
	if m.paneOffset == 0 || m.selected != 0 {
		t.Fatalf("pgdown: pane offset %d, selected %d; want the pane scrolled", m.paneOffset, m.selected)
	}
	scrolled := m.paneOffset
	m.Update(mode.MouseMsg{Kind: mode.MouseWheelDown, X: 100, Y: 10})
	if m.paneOffset != scrolled+paneWheelLines {
		t.Errorf("the wheel over the pane scrolled to %d, want %d", m.paneOffset, scrolled+paneWheelLines)
	}
	m.move(1)
	if m.paneOffset != 0 {
		t.Errorf("a new selection kept the pane offset %d", m.paneOffset)
	}
}

// The pane scrolls with the board page keys, as the Docs preview does: a
// rebound board page key scrolls it, a rebound detail page key does not, and
// without the pane the board key pages the list.
func TestThePaneScrollsWithTheBoardPageKeys(t *testing.T) {
	t.Parallel()
	bindings := config.DefaultKeyBindings()
	bindings.Board[config.BoardActionPageDown] = []string{"alt+j"}
	bindings.Board[config.BoardActionPageUp] = []string{"alt+u"}
	bindings.Detail[config.DetailActionPageDown] = []string{"alt+k"}
	bindings.Detail[config.DetailActionPageUp] = []string{"alt+i"}
	rebound, err := config.ResolveKeyBindings(bindings)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(context.Background(), memory.New(), nil, rebound, func(context.Context, agent.Agent) (string, error) {
		return strings.Repeat("line\n\n", 60), nil
	})
	m.SetSize(140, 30)
	m.SetShown(true)
	agents := make([]agent.Agent, 0, 30)
	for i := range 30 {
		agents = append(agents, live(fmt.Sprintf("s-%02d", i), fmt.Sprintf("%02d", i), agent.Idle))
	}
	settle(t, m, m.SetAgents(agents, true, "", nil))

	m.Update(testui.Press("alt+k"))
	if m.paneOffset != 0 {
		t.Errorf("the detail page_down scrolled the pane to %d", m.paneOffset)
	}
	m.Update(testui.Press("alt+j"))
	m.Update(testui.Press("alt+j"))
	if m.paneOffset == 0 || m.selected != 0 {
		t.Fatalf("the board page_down: pane offset %d, selected %d; want the pane scrolled", m.paneOffset, m.selected)
	}
	scrolled := m.paneOffset
	m.Update(testui.Press("alt+i"))
	if m.paneOffset != scrolled {
		t.Errorf("the detail page_up scrolled the pane from %d to %d", scrolled, m.paneOffset)
	}
	m.Update(testui.Press("alt+u"))
	if m.paneOffset >= scrolled {
		t.Errorf("the board page_up left the pane at %d, want above %d", m.paneOffset, scrolled)
	}

	m.SetSize(90, 30)
	m.Update(testui.Press("alt+j"))
	if m.selected == 0 {
		t.Errorf("without the pane the board page_down did not page the list")
	}
}
