package details

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uidetails "github.com/hk9890/task-manager-ui/internal/ui/details"
)

// issueWithLongContent returns an IssueDetail with enough content lines for
// content pane scrolling and enough dependency entries for deps pane scrolling.
// At 160x24 terminal: content max ~62, deps max ~15 with 30 blockers.
func issueWithLongContent(id, title string) domain.IssueDetail {
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = "Content line"
	}

	blockedBy := make([]domain.IssueReference, 30)
	for i := range blockedBy {
		blockedBy[i] = domain.IssueReference{
			ID:    fmt.Sprintf("tm-dep-%02d", i+1),
			Title: fmt.Sprintf("Dependency %d", i+1),
		}
	}

	return domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: id, Title: title, Status: "open", Type: "task", Priority: 1},
		Description: strings.Join(lines, "\n"),
		BlockedBy:   blockedBy,
	}
}

// TestDetailsTeatestFocusSwitchRetainsScrollOffsets drives a details.Model
// through the real Bubble Tea runtime to assert that per-pane scroll offsets
// are retained when the operator switches focus between content, dependencies,
// and metadata panes (AC 1).
func TestDetailsTeatestFocusSwitchRetainsScrollOffsets(t *testing.T) {
	t.Parallel()

	m := &Model{}
	m.SelectionID = "tm-1"
	m.TargetID = "tm-1"
	m.ApplyLoadedDetail("tm-1", issueWithLongContent("tm-1", "Issue A"))

	const (
		testWidth  = 160
		testHeight = 24
	)

	wrapper := newDetailsTestWrapper(m, testWidth, testHeight)
	tm := testui.NewTestModelWithSize(t, wrapper, testWidth, testHeight)
	tm.Send(tea.WindowSizeMsg{Width: testWidth, Height: testHeight})

	// Scroll content pane down several times (content pane is default focus).
	for i := 0; i < 5; i++ {
		tm.Send(tea.KeyMsg{Type: tea.KeyPgDown})
	}

	// Switch focus left to dependencies pane.
	tm.Send(tea.KeyMsg{Type: tea.KeyLeft})

	// Scroll dependencies via page-down (page-down on deps pane scrolls the
	// rail offset).
	for i := 0; i < 3; i++ {
		tm.Send(tea.KeyMsg{Type: tea.KeyPgDown})
	}

	// Switch focus back to content pane (right key from dependencies).
	tm.Send(tea.KeyMsg{Type: tea.KeyRight})

	// Switch focus to metadata pane (right from content).
	tm.Send(tea.KeyMsg{Type: tea.KeyRight})

	// Metadata pane: scroll down moves the field selector, not a raw offset.
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})

	// Return focus to content pane (left from metadata).
	tm.Send(tea.KeyMsg{Type: tea.KeyLeft})

	// Wait for the metadata-pane content to appear, confirming all sends
	// have been processed before we inspect final model state.
	testui.WaitForOutputContainsAll(t, tm.Output(), "Metadata")

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}

	finalAdapter, ok := tm.FinalModel(t).(detailsTestWrapper)
	if !ok {
		t.Fatalf("expected detailsTestWrapper as final model, got %T", tm.FinalModel(t))
	}
	final := finalAdapter.m

	// Content offset must be > 0: we scrolled content before switching panes,
	// and returning focus should not have reset it.
	if final.ContentScrollOffset == 0 {
		t.Errorf("expected ContentScrollOffset > 0 after focus switch and return, got 0")
	}

	// Dependencies offset must be > 0: page-down while focused on deps pane
	// should have advanced the dependencies scroll rail.
	if final.DependenciesScrollOffset == 0 {
		t.Errorf("expected DependenciesScrollOffset > 0 after scrolling deps pane, got 0")
	}

	// Final focus should be back on content.
	if final.focusPane() != uidetails.FocusPaneContent {
		t.Errorf("expected final focus on content pane, got %v", final.focusPane())
	}
}

// TestDetailsTeatestSelectionChangeResetsAllScrollOffsets drives a details.Model
// through the real Bubble Tea runtime to assert that switching to a different
// issue resets all three pane scroll offsets to zero (AC 2).
// This overlaps intentionally with the scroll-reset fix — defense in depth.
func TestDetailsTeatestSelectionChangeResetsAllScrollOffsets(t *testing.T) {
	t.Parallel()

	m := &Model{}
	m.SelectionID = "tm-1"
	m.TargetID = "tm-1"
	issueA := issueWithLongContent("tm-1", "Issue A")
	m.ApplyLoadedDetail("tm-1", issueA)

	const (
		testWidth  = 160
		testHeight = 24
	)

	wrapper := newDetailsTestWrapper(m, testWidth, testHeight)
	tm := testui.NewTestModelWithSize(t, wrapper, testWidth, testHeight)
	tm.Send(tea.WindowSizeMsg{Width: testWidth, Height: testHeight})

	// Scroll content pane.
	for i := 0; i < 4; i++ {
		tm.Send(tea.KeyMsg{Type: tea.KeyPgDown})
	}

	// Switch to dependencies and scroll.
	tm.Send(tea.KeyMsg{Type: tea.KeyLeft})
	for i := 0; i < 2; i++ {
		tm.Send(tea.KeyMsg{Type: tea.KeyPgDown})
	}

	// Switch to metadata (two right presses: deps→content→metadata).
	tm.Send(tea.KeyMsg{Type: tea.KeyRight})
	tm.Send(tea.KeyMsg{Type: tea.KeyRight})

	// Wait for the metadata pane to appear in the output, which confirms all
	// sends have been processed before inspecting final state.
	testui.WaitForOutputContainsAll(t, tm.Output(), "Metadata")

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}

	// Extract final model state.
	mid, ok := tm.FinalModel(t).(detailsTestWrapper)
	if !ok {
		t.Fatalf("expected detailsTestWrapper as final model, got %T", tm.FinalModel(t))
	}

	// Pre-condition: content and deps offsets should be > 0 from scrolling.
	if mid.m.ContentScrollOffset == 0 {
		t.Fatalf("pre-condition failed: ContentScrollOffset should be > 0 before issue switch, got 0")
	}
	if mid.m.DependenciesScrollOffset == 0 {
		t.Fatalf("pre-condition failed: DependenciesScrollOffset should be > 0 before issue switch, got 0")
	}

	// Simulate selection change: load a different issue.
	issueB := issueWithLongContent("tm-2", "Issue B")
	mid.m.ApplyLoadedDetail("tm-2", issueB)

	// All three offsets must be reset.
	if mid.m.ContentScrollOffset != 0 {
		t.Errorf("expected ContentScrollOffset=0 after issue switch, got %d", mid.m.ContentScrollOffset)
	}
	if mid.m.DependenciesScrollOffset != 0 {
		t.Errorf("expected DependenciesScrollOffset=0 after issue switch, got %d", mid.m.DependenciesScrollOffset)
	}
	if mid.m.MetadataScrollOffset != 0 {
		t.Errorf("expected MetadataScrollOffset=0 after issue switch, got %d", mid.m.MetadataScrollOffset)
	}
}
