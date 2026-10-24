package board

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/revier"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// agentBoard is a board with TM-1 Ready and TM-2, TM-3 and TM-4 In Progress.
// Sessions s-1 to s-4 commented on the issue of the same number; s-2b
// commented on TM-2 too. TM-1 has a session although it is not In Progress,
// so a test sees that only the In Progress column draws the mark.
func agentBoard(t *testing.T) *Model {
	t.Helper()

	m := newBoardModel(memoryrepo.New(fakes.FrozenClock()), resolvedBoardKeys(t))
	m.SetSize(200, 30)
	feedDashboardData(m, repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{Ready: []domain.IssueSummary{
			{ID: "TM-1", Title: "Ready work", Type: "task", Status: "open", Priority: 2},
		}},
		InProgress: []domain.IssueSummary{
			{ID: "TM-2", Title: "Two agents", Type: "task", Status: "in_progress", Priority: 1},
			{ID: "TM-3", Title: "One agent", Type: "task", Status: "in_progress", Priority: 1},
			{ID: "TM-4", Title: "A gone agent", Type: "task", Status: "in_progress", Priority: 1},
		},
		InProgressSessions: map[string][]string{
			"TM-1": {"s-1"},
			"TM-2": {"s-2", "s-2b"},
			"TM-3": {"s-3"},
			"TM-4": {"s-4"},
		},
	})
	return m
}

// cardMeta is the second line of the card of id in the plain view: the
// priority, the status, the agent mark and the ID.
func cardMeta(t *testing.T, m *Model, id string) string {
	t.Helper()
	plain := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	for _, line := range strings.Split(plain, "\n") {
		for _, cell := range strings.Split(line, "│") {
			if strings.Contains(cell, " "+id) {
				return strings.TrimSpace(strings.TrimPrefix(cell, selectedGutter))
			}
		}
	}
	t.Fatalf("the board draws no card for %s:\n%s", id, plain)
	return ""
}

// A live agent that commented on an In Progress issue marks its card; the
// agent that needs the operator most gives the mark its status. A Ready issue
// draws no mark, and neither does an issue whose agent is not live.
func TestLiveAgentsMarkOnlyTheInProgressCardsTheyCommentedOn(t *testing.T) {
	testui.ForceTrueColor(t)

	m := agentBoard(t)
	m.SetLiveAgents(map[string]string{
		"s-1":  revier.StatusRunning,
		"s-2":  revier.StatusIdle,
		"s-2b": revier.StatusAttention,
		"s-3":  revier.StatusRunning,
	})

	mark := styles.Glyphs.Agent
	for id, want := range map[string]string{
		"TM-1": "P2 OPN TM-1",
		"TM-2": "P1 IP " + mark + " TM-2",
		"TM-3": "P1 IP " + mark + " TM-3",
		"TM-4": "P1 IP TM-4",
	} {
		if got := cardMeta(t, m, id); got != want {
			t.Errorf("card %s reads %q, want %q", id, got, want)
		}
	}

	col := m.uiColumn(inProgressColumnIndex, -1)
	if got := col.Agents["TM-2"]; got != revier.StatusAttention {
		t.Errorf("TM-2 draws the status %q, want attention over idle", got)
	}
	if got := col.Agents["TM-3"]; got != revier.StatusRunning {
		t.Errorf("TM-3 draws the status %q, want running", got)
	}
	for idx := range m.columns {
		if idx != inProgressColumnIndex && m.uiColumn(idx, -1).Agents != nil {
			t.Errorf("column %q carries agents", m.columns[idx].title)
		}
	}
}

// A poll that finds no agent clears the marks, and the next one sets them
// again. A reload keeps the live agents and links them to the sessions it
// read.
func TestTheAgentMarksFollowThePollAndTheReload(t *testing.T) {
	m := agentBoard(t)
	mark := styles.Glyphs.Agent

	m.SetLiveAgents(map[string]string{"s-3": revier.StatusRunning})
	if got := cardMeta(t, m, "TM-3"); !strings.Contains(got, mark) {
		t.Fatalf("card TM-3 reads %q, want the mark", got)
	}

	m.SetLiveAgents(nil)
	if got := cardMeta(t, m, "TM-3"); strings.Contains(got, mark) {
		t.Errorf("after a failed poll card TM-3 reads %q, want no mark", got)
	}

	m.SetLiveAgents(map[string]string{"s-3": revier.StatusRunning, "s-new": revier.StatusRunning})
	feedDashboardData(m, repository.DashboardData{
		InProgress: []domain.IssueSummary{
			{ID: "TM-3", Title: "One agent", Type: "task", Status: "in_progress", Priority: 1},
			{ID: "TM-5", Title: "A new agent", Type: "task", Status: "in_progress", Priority: 1},
		},
		InProgressSessions: map[string][]string{"TM-5": {"s-new"}},
	})
	if got := cardMeta(t, m, "TM-3"); strings.Contains(got, mark) {
		t.Errorf("card TM-3 lost its session in the reload but reads %q", got)
	}
	if got := cardMeta(t, m, "TM-5"); !strings.Contains(got, mark) {
		t.Errorf("card TM-5 reads %q, want the mark of the agent the reload linked", got)
	}
}
