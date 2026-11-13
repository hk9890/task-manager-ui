package agents

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func sampleRows() []Row {
	issue := &domain.IssueSummary{ID: "tm-1", Title: "Fix the board"}
	other := &domain.IssueSummary{ID: "tm-2", Title: "Write the docs"}
	return []Row{
		{Agent: agent.Agent{Ref: "demo:home:3", Session: "s-3", Label: "Tab 3", Name: "fix-board", Status: agent.NeedsYou}, Issue: issue},
		{Agent: agent.Agent{Ref: "demo:home:4", Session: "s-4", Label: "Tab 4", Status: agent.Running, Activity: "Editing model.go"}, Issue: other},
		{Agent: agent.GoneAgent("s-gone-0123"), Issue: issue},
		{Agent: agent.Agent{Ref: "demo:home:7", Session: "s-7", Label: "Tab 7", Name: "scratch", Status: agent.Idle}},
	}
}

func sampleState() State {
	return State{Rows: sampleRows(), SelectedRow: 1, Width: 80, Height: 18, Hover: -1}
}

// The list draws the linked agents under one heading and the others under
// a second: each agent its name, status and activity over where it is and
// the issue it is linked to.
func TestTheListGroupsTheAgentsByTheirLink(t *testing.T) {
	t.Parallel()
	testui.AssertMatchesGoldenNormalized(t, []byte(testui.Downsample(Render(sampleState()))), "agents_w80.golden")

	view := testui.Downsample(Render(sampleState()))
	for _, want := range []string{
		"Linked (3)", "Not linked (1)",
		"fix-board  needs you", "Tab 3 · tm-1  Fix the board",
		"Tab 4  running · Editing model.go", "tm-2  Write the docs",
		"Session s-gone-0  gone",
		"scratch  idle",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the list does not draw %q:\n%s", want, view)
		}
	}
}

// Without a row the list says there is no agent, and why when it knows.
func TestAnEmptyListSaysWhy(t *testing.T) {
	t.Parallel()
	view := testui.Downsample(Render(State{Width: 60, Height: 10, Hover: -1, Empty: "no agent host: agents are off"}))
	testui.AssertContainsAll(t, view, "No agents", "no agent host: agents are off")
	if _, ok := HitTest(State{Width: 60, Height: 10}, 3, 2); ok {
		t.Error("an empty list reports a row")
	}
}

// HitTest reports the row Render drew at a cell, on both of its lines, and
// none on the rule, a border, a heading or a gap.
func TestHitTestReportsTheRowDrawnThere(t *testing.T) {
	t.Parallel()
	state := sampleState()
	view := Render(state)
	for row, text := range map[int]string{0: "fix-board", 1: "tm-2  Write the docs", 2: "Session s-gone-0", 3: "scratch"} {
		x, y := testui.FindCell(t, view, text)
		if got, ok := HitTest(state, x, y); !ok || got != row {
			t.Errorf("HitTest on %q = %d, %v; want row %d", text, got, ok, row)
		}
	}
	for _, text := range []string{"Linked (3)", "Not linked (1)", "Agents"} {
		x, y := testui.FindCell(t, view, text)
		if got, ok := HitTest(state, x, y); ok {
			t.Errorf("HitTest on %q = row %d, want none", text, got)
		}
	}
	x, y := testui.FindCell(t, view, "scratch")
	if _, ok := HitTest(state, 0, y); ok {
		t.Error("HitTest on the left border reports a row")
	}
	if _, ok := HitTest(state, x, 0); ok {
		t.Error("HitTest on the rule reports a row")
	}
}

// The selected row and the hovered one carry a band on both their lines.
func TestTheSelectedAndTheHoveredRowCarryABand(t *testing.T) {
	testui.ForceTrueColor(t)
	state := sampleState()
	state.Hover = 3
	view := Render(state)
	for _, text := range []string{"Tab 4", "tm-2  Write the docs", "scratch", "Tab 7"} {
		if testui.RowBand(t, view, text) == "" {
			t.Errorf("the line of %q carries no band:\n%s", text, view)
		}
	}
	if testui.RowBand(t, view, "fix-board") != "" {
		t.Errorf("a row neither selected nor hovered carries a band")
	}
}

// EnsureVisible scrolls the whole selected row into a short window, with its
// heading when that fits, and the window starts no later than it must.
func TestEnsureVisibleShowsTheWholeSelectedRow(t *testing.T) {
	t.Parallel()
	rows := sampleRows()
	const height = 7 // a window of four lines: two rows
	for _, tc := range []struct {
		selected, offset, want int
	}{
		{0, 0, 0},
		{1, 0, 1},
		{3, 0, 7},
		{0, 6, 0},
	} {
		got := EnsureVisible(rows, tc.selected, tc.offset, height)
		if got != tc.want {
			t.Errorf("EnsureVisible(row %d, offset %d) = %d, want %d", tc.selected, tc.offset, got, tc.want)
		}
		view := testui.Downsample(Render(State{Rows: rows, SelectedRow: tc.selected, ScrollOffset: got, Width: 60, Height: height, Hover: -1}))
		title := rows[tc.selected].Agent.Title()
		if !strings.Contains(view, title) {
			t.Errorf("row %d at offset %d is not drawn:\n%s", tc.selected, got, view)
		}
	}
}

// A name that fills the row is cut so the status stays: the tab sorts by it,
// and `needs you` is what the operator must act on.
func TestALongNameKeepsTheStatus(t *testing.T) {
	t.Parallel()
	long := agent.Agent{
		Ref: "demo:home:3", Session: "s-3", Label: "Tab 3", Status: agent.NeedsYou, Activity: "waiting",
		Name: "Implement the agent dialog for long running background tasks in detail view",
	}
	view := testui.Downsample(Render(State{Rows: []Row{{Agent: long}}, Width: 60, Height: 10, Hover: -1}))
	if !strings.Contains(view, "…  needs you") {
		t.Errorf("the row of a long name is not cut before its status:\n%s", view)
	}
}
