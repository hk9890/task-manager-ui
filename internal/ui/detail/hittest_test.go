package detail

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func hitTestRefs(prefix string, count int) []domain.IssueReference {
	refs := make([]domain.IssueReference, count)
	for idx := range refs {
		refs[idx] = domain.IssueReference{
			ID:       fmt.Sprintf("tm-%s%02d", prefix, idx),
			Title:    fmt.Sprintf("%s-ref-%02d", prefix, idx),
			Type:     "task",
			Status:   "open",
			Priority: idx % 4,
		}
	}
	return refs
}

func hitTestState(width, height int) State {
	detail := domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-root", Title: "root-title", Type: "epic", Status: "open", Priority: 1},
		Description: "body-text",
		BlockedBy:   hitTestRefs("bb", 2),
		Blocks:      hitTestRefs("bk", 1),
		Children:    hitTestRefs("ch", 12),
	}
	detail.ParentGroupBrowser.Parent = hitTestRefs("pa", 1)[0]
	return State{
		SelectionID: "tm-root",
		TargetID:    "tm-root",
		Detail:      detail,
		Width:       width,
		Height:      height,
	}
}

// drawnRefs is every reference of the state's dependency groups.
func drawnRefs(state State) []domain.IssueReference {
	var refs []domain.IssueReference
	for _, group := range dependencyGroups(state.Detail, state.BrowserItems) {
		refs = append(refs, orderedReferences(group.Refs)...)
	}
	return refs
}

// TestHitTestFindsEveryReferenceWhereRenderDrewIt asks the rendered frame
// where each reference row is, in both layouts.
func TestHitTestFindsEveryReferenceWhereRenderDrewIt(t *testing.T) {
	t.Parallel()

	for _, width := range []int{160, 100} {
		// Tall enough that the Dependencies pane clips nothing in either layout.
		state := hitTestState(width, 70)
		view := Render(state)

		refs := drawnRefs(state)
		if len(refs) != 16 {
			t.Fatalf("fixture drew %d references, want 16", len(refs))
		}
		for _, ref := range refs {
			x, y := testui.FindCell(t, view, ref.Title)
			hit, ok := HitTest(state, x, y)
			if !ok || hit.Pane != FocusPaneDependencies || hit.RefID != ref.ID {
				t.Errorf("width %d: HitTest at %q (%d,%d) = %+v, %v; want %s", width, ref.Title, x, y, hit, ok, ref.ID)
			}
		}
	}
}

// TestHitTestFollowsTheScrolledDependenciesWindow scrolls the pane and checks
// that the rows still map to what is drawn, and that the two rows the window
// spends on its "… (N earlier)" and "… (N more)" indicators hold no reference.
func TestHitTestFollowsTheScrolledDependenciesWindow(t *testing.T) {
	t.Parallel()

	state := hitTestState(160, 14)
	state.DependenciesScrollOffset = 6
	view := Render(state)

	found := 0
	for _, ref := range drawnRefs(state) {
		if !strings.Contains(view, ref.Title) {
			continue
		}
		found++
		x, y := testui.FindCell(t, view, ref.Title)
		if hit, ok := HitTest(state, x, y); !ok || hit.RefID != ref.ID {
			t.Errorf("HitTest at %q (%d,%d) = %+v, %v; want %s", ref.Title, x, y, hit, ok, ref.ID)
		}
	}
	if found == 0 {
		t.Fatalf("the scrolled window drew no reference row:\n%s", view)
	}

	for _, indicator := range []string{"earlier)", "(8 more)"} {
		x, y := testui.FindCell(t, view, indicator)
		if hit, ok := HitTest(state, x, y); !ok || hit.Pane != FocusPaneDependencies || hit.RefID != "" {
			t.Errorf("HitTest on the %q indicator = %+v, %v; want the pane with no reference", indicator, hit, ok)
		}
	}
}

func TestHitTestNamesThePaneUnderThePointer(t *testing.T) {
	t.Parallel()

	for _, width := range []int{160, 100} {
		state := hitTestState(width, 70)
		view := Render(state)

		for text, want := range map[string]FocusPane{
			"body-text":      FocusPaneContent,
			"Dependencies ─": FocusPaneDependencies,
			"Blocked by (2)": FocusPaneDependencies,
		} {
			x, y := testui.FindCell(t, view, text)
			hit, ok := HitTest(state, x, y)
			if !ok || hit.Pane != want || hit.RefID != "" {
				t.Errorf("width %d: HitTest on %q (%d,%d) = %+v, %v; want pane %d with no reference", width, text, x, y, hit, ok, want)
			}
		}

		// Off the row of an agent the Details pane holds nothing to click.
		for _, text := range []string{"Details ─", "Priority"} {
			x, y := testui.FindCell(t, view, text)
			if hit, ok := HitTest(state, x, y); ok {
				t.Errorf("width %d: HitTest on %q of the Details pane = %+v, want no hit", width, text, hit)
			}
		}
	}

	// The gap between two panes.
	state := hitTestState(160, 30)
	x, y := testui.FindCell(t, Render(state), "Content ─")
	if hit, ok := HitTest(state, x-4, y); ok {
		t.Errorf("HitTest in the column gap = %+v, want no hit", hit)
	}
}

// TestHitTestIsOffWhereRenderDrawsNoPanes covers every state Render answers
// with a placeholder, the compact form, or skeleton rows.
func TestHitTestIsOffWhereRenderDrawsNoPanes(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*State){
		"no selection":   func(s *State) { s.SelectionID = "" },
		"load failed":    func(s *State) { s.Error = "boom" },
		"nothing loaded": func(s *State) { s.Detail = domain.IssueDetail{} },
		"cold start":     func(s *State) { s.Detail = domain.IssueDetail{}; s.Loading = true },
		"too narrow":     func(s *State) { s.Width = minDetailWidth - 1 },
		"compact":        func(s *State) { s.Compact = true },
	}
	for name, mutate := range cases {
		state := hitTestState(160, 30)
		mutate(&state)
		if hit, ok := HitTest(state, 3, 3); ok {
			t.Errorf("%s: HitTest = %+v, want no hit", name, hit)
		}
	}

	state := hitTestState(160, 30)
	state.Skeleton = true
	if hit, ok := HitTest(state, 3, 3); !ok || hit.RefID != "" {
		t.Errorf("skeleton: HitTest = %+v, %v; want the pane with no reference", hit, ok)
	}
}

func TestRenderBandsTheCursorAndTheHoveredReference(t *testing.T) {
	testui.ForceTrueColor(t)

	state := hitTestState(160, 70)
	state.BrowserSelectedIssueID = "tm-ch01"
	state.Hover = &Hit{Pane: FocusPaneDependencies, RefID: "tm-ch04"}
	view := Render(state)

	cursor, hovered := testui.RowBand(t, view, "ch-ref-01"), testui.RowBand(t, view, "ch-ref-04")
	if cursor == "" || hovered == "" || cursor == hovered {
		t.Fatalf("bands: cursor %q, hovered %q; want two different backgrounds", cursor, hovered)
	}
	if plain := testui.RowBand(t, view, "ch-ref-02"); plain != "" {
		t.Fatalf("a plain reference carries the band %q", plain)
	}
}

// agentsState is hitTestState with a live and a gone agent linked.
func agentsState(width, height int) State {
	state := hitTestState(width, height)
	state.Agents = []agent.Agent{
		{Label: "Tab 3", Status: agent.Running, Session: "s-3"},
		{Label: "Tab 5", Status: agent.Gone, Session: "s-5"},
	}
	return state
}

// The row of each linked agent in the Details pane is where Render drew it,
// in both layouts; the rest of the pane holds nothing to click.
func TestHitTestFindsEveryAgentRowWhereRenderDrewIt(t *testing.T) {
	t.Parallel()

	for _, width := range []int{160, 100} {
		state := agentsState(width, 70)
		view := Render(state)
		for text, session := range map[string]string{"Tab 3: running": "s-3", "Tab 5: gone": "s-5"} {
			x, y := testui.FindCell(t, view, text)
			for _, column := range []int{x - 2, x, x + len(text)} {
				hit, ok := HitTest(state, column, y)
				if !ok || hit.Pane != FocusPaneDetails || hit.Agent != session {
					t.Errorf("width %d: HitTest at %q column %d = %+v, %v; want %s", width, text, column, hit, ok, session)
				}
			}
		}
		for _, text := range []string{"Agents", "Priority", "Details ─"} {
			x, y := testui.FindCell(t, view, text)
			if hit, ok := HitTest(state, x, y); ok {
				t.Errorf("width %d: HitTest on %q = %+v, want no hit", width, text, hit)
			}
		}

		state.Skeleton = true
		x, y := testui.FindCell(t, Render(state), "Tab 3: running")
		if hit, ok := HitTest(state, x, y); ok {
			t.Errorf("width %d: skeleton: HitTest on an agent row = %+v, want no hit", width, hit)
		}
	}
}

// A pane too short for its lines ends on "… (N more)", and that row is no
// agent, though an agent row stands under it in the full list.
func TestHitTestFindsNoAgentUnderTheMoreIndicator(t *testing.T) {
	t.Parallel()

	// Core takes four lines, the divider and the Agents title two more, so
	// a pane of eight inner lines draws Tab 3 and turns Tab 5 into the
	// indicator.
	state := agentsState(160, 10)
	view := Render(state)
	if strings.Contains(view, "Tab 5") {
		t.Fatalf("fixture draws Tab 5:\n%s", view)
	}
	x, y := testui.FindCell(t, view, "Tab 3: running")
	if hit, ok := HitTest(state, x, y); !ok || hit.Agent != "s-3" {
		t.Errorf("HitTest on Tab 3 = %+v, %v; want s-3", hit, ok)
	}
	if hit, ok := HitTest(state, x, y+1); ok {
		t.Errorf("HitTest on the indicator = %+v, want no hit", hit)
	}
}

// The agent row under the pointer draws the hover band, as a hovered
// reference does; the other agent row draws none.
func TestRenderBandsTheHoveredAgentRow(t *testing.T) {
	testui.ForceTrueColor(t)

	state := agentsState(160, 70)
	state.Hover = &Hit{Pane: FocusPaneDetails, Agent: "s-3"}
	view := Render(state)

	if band := testui.RowBand(t, view, "Tab 3: running"); band == "" {
		t.Errorf("the hovered agent row draws no band:\n%s", view)
	}
	if band := testui.RowBand(t, view, "Tab 5: gone"); band != "" {
		t.Errorf("an agent row off the pointer draws the band %q", band)
	}
}

// The Details pane with a live and a gone agent, the live one under the
// pointer.
func TestDetailsPaneWithAHoveredAgentGolden(t *testing.T) {
	testui.ForceTrueColor(t)

	state := agentsState(160, 20)
	state.Hover = &Hit{Pane: FocusPaneDetails, Agent: "s-3"}
	_, _, width := splitThreePaneWidths(state.Width)
	pane := RenderDetailsPane(state.detailsPane(width, state.Height))
	testui.AssertMatchesGoldenNormalized(t, []byte(pane), "details_pane_hovered_agent_w160.golden")
}
