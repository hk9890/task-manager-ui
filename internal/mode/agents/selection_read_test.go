package agents

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository/memory"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// A query that hides the selected agent moves the selection to the first
// match and announces that agent's issue: the shell's issue keys act on it.
func TestAQueryThatMovesTheSelectionAnnouncesIt(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	m.Update(linkedIssuesMsg{issues: []domain.IssueSummary{
		{ID: "tm-1", Title: "Fix the board", Status: "open", Labels: []string{"agent-session:s-1"}},
		{ID: "tm-2", Title: "Write the docs", Status: "open", Labels: []string{"agent-session:s-2"}},
	}})
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Idle), live("s-2", "2", agent.Idle)}, true, "", nil)
	m.move(1) // the agent of tm-2

	var announced *mode.Selection
	found := false
	for _, msg := range testui.DrainCmd(m.Update(tea.PasteMsg{Content: "board"})) {
		if changed, ok := msg.(mode.SelectionChangedMsg); ok {
			announced, found = changed.Selection, true
		}
	}
	if !found || announced == nil || announced.Issue.ID != "tm-1" {
		t.Errorf("the query moved the selection to tm-1 but announced %v (sent %v)", announced, found)
	}
}

// Select clears a query that hides the agent asked for.
func TestSelectClearsAQueryThatHidesTheAgent(t *testing.T) {
	t.Parallel()
	m := newTab(t)
	m.SetAgents([]agent.Agent{live("s-1", "1", agent.Idle), live("s-2", "2", agent.Idle)}, true, "", nil)
	m.Update(linkedIssuesMsg{})
	m.Update(tea.PasteMsg{Content: "tab 1"})
	m.Select("s-2")
	if row, _ := m.selectedRow(); row.Agent.Session != "s-2" || m.HasQuery() {
		t.Errorf("Select selected %q with the query %q, want s-2 and no query", row.Agent.Session, m.query.Text())
	}
}

// A failed read is tried again on the next poll, not at once; the reload
// forgets every message and reads the selected agent's again.
func TestAFailedReadIsTriedAgainOnTheNextPoll(t *testing.T) {
	t.Parallel()
	calls, fail := 0, true
	m := NewModel(context.Background(), memory.New(), nil, keys(t), func(context.Context, agent.Agent) (string, error) {
		calls++
		if fail {
			return "", errors.New("timed out")
		}
		return "done", nil
	})
	m.SetSize(140, 30)
	m.SetShown(true)
	agents := []agent.Agent{live("s-1", "1", agent.Idle)}
	settle(t, m, m.SetAgents(agents, true, "", nil))
	if calls != 1 || m.pane().Error == "" {
		t.Fatalf("calls %d, pane %+v; want one failed read", calls, m.pane())
	}
	fail = false
	settle(t, m, m.SetAgents(agents, true, "", nil))
	if calls != 2 || m.pane().Message != "done" {
		t.Errorf("the next poll did not read again: calls %d, pane %+v", calls, m.pane())
	}
	settle(t, m, m.Reload())
	if calls != 3 {
		t.Errorf("the reload did not read again: calls %d", calls)
	}
}

// Nothing is read while the tab is not on screen, and the tab coming on
// screen, or growing wide enough for the pane, starts the read.
func TestThePaneReadsOnlyWhileItIsOnScreen(t *testing.T) {
	t.Parallel()
	reads := map[string]int{}
	m := splitTab(t, reads)
	m.SetShown(false)
	settle(t, m, m.SetAgents([]agent.Agent{live("s-1", "1", agent.Idle)}, true, "", nil))
	if reads["s-1"] != 0 || m.pane().Reading {
		t.Fatalf("a tab off screen read %v, or says reading: %+v", reads, m.pane())
	}
	settle(t, m, m.SetShown(true))
	if reads["s-1"] != 1 {
		t.Errorf("coming on screen did not read: %v", reads)
	}

	narrow := splitTab(t, reads)
	narrow.SetSize(90, 30)
	narrow.SetShown(true)
	settle(t, narrow, narrow.SetAgents([]agent.Agent{live("s-2", "2", agent.Idle)}, true, "", nil))
	narrow.SetSize(140, 30)
	settle(t, narrow, narrow.SetShown(true))
	if reads["s-2"] != 1 {
		t.Errorf("growing wide enough for the pane did not read: %v", reads)
	}
}
