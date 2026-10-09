package detail

import (
	"fmt"
	"strings"
	"testing"

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
			"Metadata ─":     FocusPaneMetadata,
		} {
			x, y := testui.FindCell(t, view, text)
			hit, ok := HitTest(state, x, y)
			if !ok || hit.Pane != want || hit.RefID != "" {
				t.Errorf("width %d: HitTest on %q (%d,%d) = %+v, %v; want pane %d with no reference", width, text, x, y, hit, ok, want)
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

func TestRenderMarksTheHoveredReference(t *testing.T) {
	t.Parallel()

	state := hitTestState(160, 70)
	_, y := testui.FindCell(t, Render(state), "ch-ref-04")
	state.Hover = &Hit{Pane: FocusPaneDependencies, RefID: "tm-ch04"}
	if _, hoverY := testui.FindCell(t, Render(state), "›"); hoverY != y {
		t.Fatalf("hover chevron drawn on row %d, want row %d", hoverY, y)
	}
}
