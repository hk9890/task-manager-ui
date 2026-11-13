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

// OnPane is the pane's cells only, and nothing while the screen is not split.
func TestOnPaneIsThePaneOnly(t *testing.T) {
	t.Parallel()
	state := splitState()
	list, _, _ := styles.SplitWidths(state.Width)
	if OnPane(state, list-1, 5) || !OnPane(state, list+styles.SplitGap, 5) || OnPane(state, list+styles.SplitGap, 1) {
		t.Errorf("OnPane does not follow the split at list width %d", list)
	}
	if _, ok := HitTest(state, list+styles.SplitGap+3, 4); ok {
		t.Errorf("a click on the pane hits a row")
	}
	state.Width = styles.SplitMinWidth - 1
	if OnPane(state, state.Width-2, 5) {
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
