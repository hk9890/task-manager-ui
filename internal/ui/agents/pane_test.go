package agents

import (
	"strings"
	"testing"

	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

func splitState() State {
	state := sampleState()
	state.Width, state.Height = 140, 24
	state.SelectedRow = 1
	state.Placeholder = "filter agents"
	state.Pane = Pane{Message: "## Done\n\nI moved the query line into `board.go`.\n\n- the list keeps its rows\n- the pane scrolls"}
	return state
}

// From styles.SplitMinWidth the Agent pane stands beside the list: the
// selected agent, what it does, its issue, label and session, and its last
// message as markdown.
func TestTheAgentPaneStandsBesideTheList(t *testing.T) {
	t.Parallel()
	view := testui.Downsample(Render(splitState()))
	testui.AssertMatchesGoldenNormalized(t, []byte(view), "agents_split_w140.golden")
	for _, want := range []string{
		"╭─ Agent ", "Tab 4  running", "Editing model.go",
		"Issue", "tm-2", "Write the docs", "Session  s-4",
		"Last message", "I moved the query line",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the pane does not draw %q:\n%s", want, view)
		}
	}
}

// Below styles.SplitMinWidth the list takes the whole width, as before.
func TestANarrowScreenDrawsTheListAlone(t *testing.T) {
	t.Parallel()
	state := splitState()
	state.Width = styles.SplitMinWidth - 1
	if view := testui.Downsample(Render(state)); strings.Contains(view, "Agent ─") || strings.Contains(view, "Last message") {
		t.Errorf("the pane is drawn below the split width:\n%s", view)
	}
}

// The pane says how far the read of the message is, or why it failed; a gone
// agent has no message to read.
func TestThePaneSaysWhereTheMessageIs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		selected int
		pane     Pane
		want     string
	}{
		{"reading", 1, Pane{Reading: true}, "reading…"},
		{"failed", 1, Pane{Error: "revier timed out"}, "read failed: revier timed out"},
		{"empty", 1, Pane{}, "(nothing yet)"},
		{"gone", 2, Pane{}, "The host no longer runs this agent"},
		{"not linked", 3, Pane{}, "none"},
	} {
		state := splitState()
		state.SelectedRow, state.Pane = tc.selected, tc.pane
		if view := testui.Downsample(Render(state)); !strings.Contains(view, tc.want) {
			t.Errorf("%s: the pane does not say %q:\n%s", tc.name, tc.want, view)
		}
	}
}

// A message longer than the pane scrolls: the window says how many lines are
// above and below it, and PaneMaxOffset leaves no blank line at the bottom.
func TestTheMessageScrolls(t *testing.T) {
	t.Parallel()
	state := splitState()
	state.Pane.Message = strings.Repeat("line\n\n", 40)
	if view := testui.Downsample(Render(state)); !strings.Contains(view, "more)") {
		t.Errorf("a long message does not say there is more:\n%s", view)
	}
	state.Pane.ScrollOffset = PaneMaxOffset(state)
	view := testui.Downsample(Render(state))
	if !strings.Contains(view, "earlier)") || strings.Contains(view, "more)") {
		t.Errorf("at the last offset the window is wrong:\n%s", view)
	}
}

// The query line stands above the list, and a query that matches nothing
// says so rather than "No agents".
func TestTheQueryLineFiltersTheList(t *testing.T) {
	t.Parallel()
	state := sampleState()
	state.Placeholder = "filter agents"
	if view := testui.Downsample(Render(state)); !strings.Contains(view, "filter agents") {
		t.Errorf("the empty query line shows no placeholder:\n%s", view)
	}
	state.Query, state.Rows, state.Loaded = "nothing", nil, 4
	if view := testui.Downsample(Render(state)); !strings.Contains(view, "No agent matches the filter") || strings.Contains(view, "No agents") {
		t.Errorf("an empty match is not told apart:\n%s", view)
	}
}

// OnPane is true on the cells Render draws the pane on and false on the gap
// and the list beside it; it is false everywhere while the screen is not split.
func TestOnPaneIsWhereThePaneIsDrawn(t *testing.T) {
	t.Parallel()
	state := splitState()
	view := Render(state)

	x, y := testui.FindCell(t, view, "╭─ Agent ")
	if !OnPane(state, x, y) || OnPane(state, x-1, y) {
		t.Errorf("OnPane does not start at the pane's left border (%d, %d)", x, y)
	}
	if x, y := testui.FindCell(t, view, "Last message"); !OnPane(state, x, y) {
		t.Errorf("OnPane is false inside the pane at (%d, %d)", x, y)
	}
	if x, y := testui.FindCell(t, view, "Session s-gone-0"); OnPane(state, x, y) {
		t.Errorf("OnPane is true on a list row at (%d, %d)", x, y)
	} else if _, ok := HitTest(state, x, y); !ok {
		t.Errorf("the list row at (%d, %d) is no hit", x, y)
	}
	x, y = testui.FindCell(t, view, "Session  s-4")
	if _, ok := HitTest(state, x, y); ok || !OnPane(state, x, y) {
		t.Errorf("the pane line at (%d, %d) is a row hit, or not on the pane", x, y)
	}

	state.Width = styles.SplitMinWidth - 1
	narrow := Render(state)
	if x, y := testui.FindCell(t, narrow, "Not linked"); OnPane(state, x+20, y) {
		t.Errorf("OnPane is true while the screen is not split")
	}
}

// The query marks its words in the rows in the match style, and the pane's
// rule is the content rule, in the border colour.
func TestTheMatchMarksAndThePaneRuleCarryTheirRoles(t *testing.T) {
	testui.ForceTrueColor(t)
	state := splitState()
	state.Query = "board"
	view := Render(state)
	if !strings.Contains(view, styles.MatchTextStyle.Render("board")) {
		t.Errorf("the query word is not marked in the match style:\n%q", view)
	}
	_, pane, _ := styles.SplitWidths(state.Width)
	if !strings.Contains(view, styles.ContentRule(pane-2-2*panePad)) {
		t.Errorf("the pane rule is not the content rule:\n%q", view)
	}
}

// Under a filter the header counts the matches of the loaded agents.
func TestTheHeaderCountsTheMatchesOfTheLoadedAgents(t *testing.T) {
	t.Parallel()
	state := sampleState()
	state.Query, state.Loaded, state.Rows = "board", 4, state.Rows[:1]
	if view := testui.Downsample(Render(state)); !strings.Contains(view, " 1 of 4 ") {
		t.Errorf("the header does not read 1 of 4:\n%s", view)
	}
}
