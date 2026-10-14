package app

// Detail-mode behaviour driven through the shell: content scrolling, the
// dependency browser and drill-in/return navigation. Detail *rendering* lives
// in internal/ui/detail; this file asserts what the shell does with it.

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/mode/detail"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uidetail "github.com/hk9890/task-manager-ui/internal/ui/detail"
)

func TestModelDetailViewShowsTheConfiguredCommentKeyOnTheAddButton(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-2", Title: "In progress", Status: "in_progress", Type: "task", Priority: 2}, Description: "detail"})
	seedIssueDetail(gw, domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1}})

	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{
			config.ShellActionCommentIssue: {"ctrl+a"},
		},
	})

	services, err := NewServices(gw, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.setTerminalSize(tea.WindowSizeMsg{Width: 120, Height: 34})
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	view := testui.AnsiEscapePattern.ReplaceAllString(testui.View(m), "")
	if !strings.Contains(view, "add ctrl+a") {
		t.Fatalf("expected the add button to show the configured comment binding, got:\n%s", view)
	}
	if strings.Contains(view, "add alt+a") {
		t.Fatalf("expected the default comment key to be absent, got:\n%s", view)
	}
}

func TestModelDetailModeSupportsScrollingLongContent(t *testing.T) {
	t.Parallel()

	longLines := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		longLines = append(longLines, "Line "+strconv.Itoa(i))
	}

	gw := fakes.NewTracked()
	seedReady(gw, "tm-9", "Ninth", "task", 2)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-9", Title: "Ninth", Status: "open", Type: "task", Priority: 2},
		Description: strings.Join(longLines, "\n"),
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.setTerminalSize(tea.WindowSizeMsg{Width: 90, Height: 16})
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	viewTop := testui.View(m)
	if !strings.Contains(viewTop, "Metadata") || !strings.Contains(viewTop, "Core") || !strings.Contains(viewTop, "Type    : task") {
		t.Fatalf("expected metadata section in initial detail view, got:\n%s", viewTop)
	}

	next, cmd = m.Update(testui.Press("pgdown"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	viewPaged := testui.View(m)
	if viewPaged == viewTop {
		t.Fatalf("expected detail view to change after page down")
	}
	if !strings.Contains(viewPaged, "Line 1") {
		t.Fatalf("expected first description lines after paging, got:\n%s", viewPaged)
	}

	next, cmd = m.Update(testui.Press("end"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	viewEnd := testui.View(m)
	if !strings.Contains(viewEnd, "Line 80") {
		t.Fatalf("expected to reach bottom content after end key, got:\n%s", viewEnd)
	}
}

// TestModelDetailModeLeftBrowserUpDownMovesCursorOnlyThenEnterLoads verifies
// the decoupled navigation flow for an issue with a parent group (the parent
// shows as the last row of the dependency browser).
// After decoupling (Q5): ↑/↓ only moves the cursor highlight (no load cmd);
// Enter emits OpenRelatedIssueMsg → loadDetailCmd (non-nil cmd).
func TestModelDetailModeLeftBrowserUpDownMovesCursorOnlyThenEnterLoads(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Root", "task", 1)
	seedInProgress(gw, "tm-9", "Other", "task", 2)
	// tm-1 (viewed) has a blocker, a downstream issue, and a parent. The
	// dependency browser lists those deps followed by the parent row — a stable
	// 3 rows. Parent-only: the parent's other children (siblings) are not
	// surfaced. Pressing Enter on tm-6 (same shape) keeps the panel at 3 rows.
	parent := domain.IssueReference{ID: "tm-0", Title: "Parent epic"}
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:            domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 1},
		BlockedBy:          []domain.IssueReference{{ID: "tm-5", Title: "Upstream"}},
		Blocks:             []domain.IssueReference{{ID: "tm-6", Title: "Downstream"}},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{Parent: parent},
	})
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:            domain.IssueSummary{ID: "tm-6", Title: "Downstream", Status: "in_progress", Type: "bug", Priority: 2},
		BlockedBy:          []domain.IssueReference{{ID: "tm-7", Title: "Upstream two"}},
		Blocks:             []domain.IssueReference{{ID: "tm-8", Title: "Downstream two"}},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{Parent: parent},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.setTerminalSize(tea.WindowSizeMsg{Width: 160, Height: 34})
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	m.detail.ContentScrollOffset = 5

	next, cmd = m.Update(testui.Press("left"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// (Q6a) Down only moves cursor — no preview load command (nil cmd).
	prevIndex := m.detail.BrowserSelectedIndex
	next, cmd = m.Update(testui.Press("down"))
	m = next.(Model)
	// cmd may be nil or a no-op batch; it must NOT trigger a detail reload.
	mark := gw.CallCount()
	m = applyMessages(t, m, runBatch(cmd))
	if gw.HasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected down on browser to NOT trigger repository.Issue call, got calls=%#v", gw.Calls())
	}
	if m.detail.BrowserSelectedIndex == prevIndex {
		t.Errorf("expected BrowserSelectedIndex to advance after down, still at %d", prevIndex)
	}
	// Selection stays anchored; no TargetID change from the arrow alone.
	if m.detail.SelectionID() != "tm-1" {
		t.Errorf("expected SelectionID to remain tm-1 after arrow, got %q", m.detail.SelectionID())
	}
	if got := firstSelectionID(m, mode.Board); got != "tm-1" {
		t.Errorf("expected board selection to stay anchored on tm-1, got %q", got)
	}

	// (Q6b) Enter emits OpenRelatedIssueMsg → loadDetailCmd (non-nil cmd).
	mark = gw.CallCount()
	next, cmd = m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Errorf("expected app to remain in detail mode after Enter on browser panel, got %s", m.active)
	}
	if !gw.HasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected Enter on browser to trigger repository.Issue call (loadDetailCmd), calls=%#v", gw.Calls())
	}
	// Scroll must be reset and Loading must have been set (may now be false after applyMessages resolves the load).
	if m.detail.ContentScrollOffset != 0 {
		t.Errorf("expected ContentScrollOffset=0 after Enter-reload, got %d", m.detail.ContentScrollOffset)
	}
	// Full navigation: the Dependencies pane now reflects the DRILLED issue
	// (tm-6: its blocker tm-7, downstream tm-8, and parent tm-0) — not the
	// issue we came from.
	if got := browserIDs(m.detail.BrowserItems); strings.Join(got, ",") != "tm-7,tm-8,tm-0" {
		t.Errorf("expected drilled issue's deps+parent in browser, got %v", got)
	}
	if m.detail.SelectionID() != "tm-6" {
		t.Errorf("expected SelectionID to follow the drill to tm-6, got %q", m.detail.SelectionID())
	}
}

// TestModelDetailModeDependenciesWithoutParentGroupUpDownMovesCursorOnlyThenEnterLoads
// verifies the decoupled navigation flow for deps-only (no parent group). After
// decoupling (Q5): ↑/↓ only moves the cursor (no load); Enter triggers reload.
func TestModelDetailModeDependenciesWithoutParentGroupUpDownMovesCursorOnlyThenEnterLoads(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Root", "task", 1)
	seedInProgress(gw, "tm-9", "Other", "task", 2)
	seedIssueDetail(gw, domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 1},
		BlockedBy: []domain.IssueReference{
			{ID: "tm-3", Title: "Blocker"},
		},
		Blocks: []domain.IssueReference{
			{ID: "tm-5", Title: "Downstream"},
		},
		Related: []domain.IssueReference{
			{ID: "tm-4", Title: "Related"},
		},
	})
	seedIssueDetail(gw, domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-5", Title: "Downstream", Status: "in_progress", Type: "task", Priority: 2},
	})
	seedIssueDetail(gw, domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-4", Title: "Related", Status: "in_progress", Type: "bug", Priority: 2},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.setTerminalSize(tea.WindowSizeMsg{Width: 160, Height: 34})
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(m.detail.BrowserItems) != 3 {
		t.Fatalf("expected dependencies to populate browser items without parent-group, got %d", len(m.detail.BrowserItems))
	}

	next, cmd = m.Update(testui.Press("left"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// Down twice: moves cursor to index 2 (tm-4 in the Related group). No load occurs.
	mark := gw.CallCount()
	next, cmd = m.Update(testui.Press("down"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.HasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected first down to NOT trigger Issue call, calls=%#v", gw.Calls())
	}

	mark = gw.CallCount()
	next, cmd = m.Update(testui.Press("down"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if gw.HasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected second down to NOT trigger Issue call, calls=%#v", gw.Calls())
	}

	// Cursor is now on tm-4 (index 2). Enter triggers reload.
	mark = gw.CallCount()
	next, cmd = m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Errorf("expected app to remain in detail mode after Enter on dependencies pane, got %s", m.active)
	}
	if !gw.HasCallSince(mark, fakes.MethodIssue) {
		t.Errorf("expected Enter on dependencies pane to trigger repository.Issue call, calls=%#v", gw.Calls())
	}
	// TargetID must point to the cursor row (tm-4).
	if m.detail.TargetID() != "tm-4" {
		t.Errorf("expected Enter to set TargetID=tm-4 (cursor row), got %q", m.detail.TargetID())
	}
	// Drilling is a full navigation: SelectionID now follows the target so the
	// Dependencies pane (and all panes) reflect tm-4, not the issue we came from.
	if m.detail.SelectionID() != "tm-4" {
		t.Errorf("expected SelectionID to follow the drill to tm-4, got %q", m.detail.SelectionID())
	}
}

// TestModelDetailRoundTripEpicToChildAndBackViaParent is the end-to-end proof of
// the user-requested flow: open an epic's detail, drill into one of its children
// from the Children group, and then jump back to the epic via the child's own
// Parent row. The key property is that drilling is a FULL navigation — every
// pane, including the Dependencies rail, reflects the issue you land on — so the
// child shows its Parent group (which is what lets you go back up).
func TestModelDetailRoundTripEpicToChildAndBackViaParent(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-epic", "Auth epic", "epic", 1)
	// Epic's detail lists its child in the Children group.
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:  domain.IssueSummary{ID: "tm-epic", Title: "Auth epic", Status: "open", Type: "epic", Priority: 1},
		Children: []domain.IssueReference{{ID: "tm-child", Title: "Login crash", Type: "bug", Priority: 0, Status: "open"}},
	})
	// Child's detail lists the epic in its Parent group. Seeded in_progress (not
	// "open" with no deps) so it does not also land in the Ready column — the
	// epic stays the sole ready issue and thus the default board selection.
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:            domain.IssueSummary{ID: "tm-child", Title: "Login crash", Status: "in_progress", Type: "bug", Priority: 0},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{Parent: domain.IssueReference{ID: "tm-epic", Title: "Auth epic", Type: "epic", Priority: 1, Status: "open"}},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.setTerminalSize(tea.WindowSizeMsg{Width: 160, Height: 34})
	m = applyMessages(t, m, runBatch(m.Init()))

	// Open the epic's detail (the ready selection).
	next, cmd := m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.detail.SelectionID() != "tm-epic" || m.detail.Detail.Summary.ID != "tm-epic" {
		t.Fatalf("setup: expected epic detail, got selection=%q detail=%q", m.detail.SelectionID(), m.detail.Detail.Summary.ID)
	}
	if got := browserIDs(m.detail.BrowserItems); strings.Join(got, ",") != "tm-child" {
		t.Fatalf("expected epic deps pane to list its child, got %v", got)
	}

	// Focus the Dependencies pane and Enter on the child → drill down.
	next, cmd = m.Update(testui.Press("left"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.detail.SelectionID() != "tm-child" || m.detail.Detail.Summary.ID != "tm-child" {
		t.Fatalf("expected to land on the child, got selection=%q detail=%q", m.detail.SelectionID(), m.detail.Detail.Summary.ID)
	}
	// The fix: the child's Dependencies pane now shows its OWN Parent group.
	if got := browserIDs(m.detail.BrowserItems); strings.Join(got, ",") != "tm-epic" {
		t.Fatalf("expected child deps pane to show its parent epic (drill must update the rail), got %v", got)
	}

	// Focus the Dependencies pane and Enter on the Parent row → jump back up.
	next, cmd = m.Update(testui.Press("left"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.detail.SelectionID() != "tm-epic" || m.detail.Detail.Summary.ID != "tm-epic" {
		t.Fatalf("expected round-trip back to the epic, got selection=%q detail=%q", m.detail.SelectionID(), m.detail.Detail.Summary.ID)
	}
	if got := browserIDs(m.detail.BrowserItems); strings.Join(got, ",") != "tm-child" {
		t.Fatalf("expected epic deps pane to list its child again after round-trip, got %v", got)
	}
}

// deliverDrill hands the shell the OpenRelatedIssueMsg that cmd produces and
// stops there: the returned command is the detail load, not yet run.
func deliverDrill(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()

	for _, msg := range runBatch(cmd) {
		if drill, ok := msg.(detail.OpenRelatedIssueMsg); ok {
			next, loadCmd := m.Update(drill)
			return next.(Model), loadCmd
		}
	}
	t.Fatal("the command produced no detail.OpenRelatedIssueMsg")
	return m, nil
}

// TestAppHandlerOpenRelatedIssueMsgPerformsReloadFocusMoveAndScrollReset
// verifies that when the Details mode emits OpenRelatedIssueMsg (via Enter on
// a Dependencies pane row), the app shell handler performs the reload + focus
// move + scroll reset it already does (Q6c). A synthetic KeyMsg drives the
// model through the production code path.
func TestAppHandlerOpenRelatedIssueMsgPerformsReloadFocusMoveAndScrollReset(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Main issue", "epic", 1)
	seedIssueSummary(gw, domain.IssueSummary{ID: "tm-child", Title: "Child issue", Status: "open", Type: "task", Priority: 2})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)

	// Put the model into Detail mode with tm-1 loaded and non-zero scroll offsets.
	m.active = mode.Detail
	m.detail = detail.Model{
		FocusPane: uidetail.FocusPaneDependencies,
		Detail: domain.IssueDetail{
			Summary:  domain.IssueSummary{ID: "tm-1", Title: "Main issue", Status: "open", Type: "epic", Priority: 1},
			Children: []domain.IssueReference{{ID: "tm-child", Title: "Child issue"}},
		},
		BrowserItems: []domain.IssueReference{
			{ID: "tm-child", Title: "Child issue"},
		},
		BrowserSelectedIndex:     0, // cursor on tm-child
		ContentScrollOffset:      5,
		DependenciesScrollOffset: 1,
		Keys:                     m.keys,
	}
	// Stage "tm-1 is loaded" through the real protocol. SelectBrowserIssue does
	// not find tm-1 among the browser items, so the cursor normalises back to
	// tm-child, which is what this test drives Enter on.
	m.detail.BeginLoad("tm-1", detail.BeginLoadOptions{})
	m.detail.FinishLoad(nil)
	m.sizeKnown = true
	m.setTerminalSize(tea.WindowSizeMsg{Width: 160, Height: 34})

	mark := gw.CallCount()

	// Send Enter: drives HandleKey which should emit OpenRelatedIssueMsg for tm-child.
	next, cmd := m.Update(testui.Press("enter"))
	m, cmd = deliverDrill(t, next.(Model), cmd)

	// (Q6c) App handler must: set TargetID, Loading=true, reset all scroll offsets.
	if m.detail.TargetID() != "tm-child" {
		t.Errorf("expected TargetID=tm-child after Enter on dep pane, got %q", m.detail.TargetID())
	}
	if !m.detail.IsLoading() {
		t.Error("expected detail.Loading=true after Enter on dep pane")
	}
	if m.detail.ContentScrollOffset != 0 {
		t.Errorf("expected ContentScrollOffset=0 after Enter-reload, got %d", m.detail.ContentScrollOffset)
	}
	if m.detail.DependenciesScrollOffset != 0 {
		t.Errorf("expected DependenciesScrollOffset=0 after Enter-reload, got %d", m.detail.DependenciesScrollOffset)
	}
	if m.active != mode.Detail {
		t.Errorf("expected mode.Detail to stay active after Enter on dep pane, got %s", m.active)
	}

	// App must have issued a detail load command (Issue call).
	m = applyMessages(t, m, runBatch(cmd))
	if !gw.HasCallSince(mark, fakes.MethodIssue) {
		t.Error("expected repository.Issue call after Enter-reload; handler must dispatch loadDetailCmd")
	}
}

// TestAppHandlerDrillIntoDepWithDepsKeepsFocusOnDependenciesRail verifies that
// when the user presses Enter on a row in the Dependencies pane to drill into an
// issue that itself has dependencies:
//   - Focus is NOT flipped to Content during the optimistic placeholder phase.
//   - After the real detailLoadedMsg arrives, focus stays on the Dependencies rail.
func TestAppHandlerDrillIntoDepWithDepsKeepsFocusOnDependenciesRail(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Main issue", "epic", 1)
	// tm-child has its own blockers so it is not a leaf.
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "tm-child", Title: "Child issue", Status: "open", Type: "task", Priority: 2},
		BlockedBy: []domain.IssueReference{{ID: "tm-blocker", Title: "Blocker"}},
	})
	seedIssueSummary(gw, domain.IssueSummary{ID: "tm-blocker", Title: "Blocker", Status: "open"})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m.active = mode.Detail
	m.detail = detail.Model{
		FocusPane: uidetail.FocusPaneDependencies,
		Detail: domain.IssueDetail{
			Summary:  domain.IssueSummary{ID: "tm-1", Title: "Main issue", Status: "open", Type: "epic", Priority: 1},
			Children: []domain.IssueReference{{ID: "tm-child", Title: "Child issue"}},
		},
		BrowserItems:         []domain.IssueReference{{ID: "tm-child", Title: "Child issue"}},
		BrowserSelectedIndex: 0,
		Keys:                 m.keys,
	}
	m.sizeKnown = true
	m.setTerminalSize(tea.WindowSizeMsg{Width: 160, Height: 34})

	// Send Enter to drill into tm-child (has deps → non-leaf).
	next, cmd := m.Update(testui.Press("enter"))
	m, _ = deliverDrill(t, next.(Model), cmd)
	if m.detail.TargetID() != "tm-child" || !m.detail.IsLoading() {
		t.Fatalf("placeholder phase: target %q loading %v, want an in-flight load of tm-child", m.detail.TargetID(), m.detail.IsLoading())
	}

	// Placeholder phase: focus must NOT have been flipped to Content.
	if m.detail.FocusPane != uidetail.FocusPaneDependencies {
		t.Errorf("placeholder phase: expected FocusPane=Dependencies, got %v", m.detail.FocusPane)
	}

	// Deliver the real detail (tm-child has BlockedBy so its rail is non-empty).
	realDetail := domain.IssueDetail{
		Summary:   domain.IssueSummary{ID: "tm-child", Title: "Child issue", Status: "open", Type: "task", Priority: 2},
		BlockedBy: []domain.IssueReference{{ID: "tm-blocker", Title: "Blocker"}},
	}
	next, _ = m.Update(detailLoadedMsg{issueID: "tm-child", detail: realDetail})
	m = next.(Model)

	// After real load: non-empty rail → focus must stay on Dependencies.
	if m.detail.FocusPane != uidetail.FocusPaneDependencies {
		t.Errorf("after real load with deps: expected FocusPane=Dependencies, got %v", m.detail.FocusPane)
	}
}

// TestAppHandlerDrillIntoLeafDepMovesFocusToContent verifies that when the user
// presses Enter on a Dependencies pane row to drill into a leaf issue (no deps),
// focus moves to the Content pane after the real detail loads.
func TestAppHandlerDrillIntoLeafDepMovesFocusToContent(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Main issue", "epic", 1)
	// tm-leaf has no dependencies.
	seedIssueSummary(gw, domain.IssueSummary{ID: "tm-leaf", Title: "Leaf issue", Status: "open", Type: "task", Priority: 2})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m.active = mode.Detail
	m.detail = detail.Model{
		FocusPane: uidetail.FocusPaneDependencies,
		Detail: domain.IssueDetail{
			Summary:  domain.IssueSummary{ID: "tm-1", Title: "Main issue", Status: "open", Type: "epic", Priority: 1},
			Children: []domain.IssueReference{{ID: "tm-leaf", Title: "Leaf issue"}},
		},
		BrowserItems:         []domain.IssueReference{{ID: "tm-leaf", Title: "Leaf issue"}},
		BrowserSelectedIndex: 0,
		Keys:                 m.keys,
	}
	m.sizeKnown = true
	m.setTerminalSize(tea.WindowSizeMsg{Width: 160, Height: 34})

	// Send Enter to drill into tm-leaf (no deps → leaf).
	next, cmd := m.Update(testui.Press("enter"))
	m, _ = deliverDrill(t, next.(Model), cmd)
	if m.detail.TargetID() != "tm-leaf" || !m.detail.IsLoading() {
		t.Fatalf("placeholder phase: target %q loading %v, want an in-flight load of tm-leaf", m.detail.TargetID(), m.detail.IsLoading())
	}

	// Placeholder phase: focus must NOT have been flipped to Content yet
	// (the focus decision is deferred to real load, not triggered by the empty placeholder).
	if m.detail.FocusPane != uidetail.FocusPaneDependencies {
		t.Errorf("placeholder phase: expected FocusPane=Dependencies (deferred), got %v", m.detail.FocusPane)
	}

	// Deliver the real detail (tm-leaf has no deps → empty rail).
	realDetail := domain.IssueDetail{
		Summary: domain.IssueSummary{ID: "tm-leaf", Title: "Leaf issue", Status: "open", Type: "task", Priority: 2},
	}
	next, _ = m.Update(detailLoadedMsg{issueID: "tm-leaf", detail: realDetail})
	m = next.(Model)

	// After real load: empty rail → focus must move to Content.
	if m.detail.FocusPane != uidetail.FocusPaneContent {
		t.Errorf("after real load with no deps: expected FocusPane=Content, got %v", m.detail.FocusPane)
	}
}

// TestAppHandlerOpenRelatedIssueMsgIsDroppedOutsideDetail: the drill travels
// as a command, so the operator can be on another surface when it arrives.
func TestAppHandlerOpenRelatedIssueMsgIsDroppedOutsideDetail(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Main issue", "epic", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	m := mustNewModel(t, services)
	m.active = mode.Board

	next, cmd := m.Update(detail.OpenRelatedIssueMsg{Ref: domain.IssueReference{ID: "tm-child", Title: "Child issue"}})
	m = next.(Model)

	if cmd != nil {
		t.Error("a drill that arrived on the board started a command")
	}
	if m.active != mode.Board || m.drillSelection != nil || m.detail.IsLoading() {
		t.Errorf("a drill that arrived on the board navigated: surface %q, drill %v, loading %v", m.active, m.drillSelection, m.detail.IsLoading())
	}
}

// drilledIntoChild opens the epic's detail, drills into its child, and returns
// the model sitting on the child.
func drilledIntoChild(t *testing.T, gw *fakes.TrackedRepository) Model {
	t.Helper()

	seedReady(gw, "tm-epic", "Auth epic", "epic", 1)
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:  domain.IssueSummary{ID: "tm-epic", Title: "Auth epic", Status: "open", Type: "epic", Priority: 1},
		Children: []domain.IssueReference{{ID: "tm-child", Title: "Login crash", Type: "bug", Priority: 0, Status: "open"}},
	})
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:            domain.IssueSummary{ID: "tm-child", Title: "Login crash", Status: "in_progress", Type: "bug", Priority: 0},
		ParentGroupBrowser: domain.ParentGroupBrowserContext{Parent: domain.IssueReference{ID: "tm-epic", Title: "Auth epic", Type: "epic", Priority: 1, Status: "open"}},
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.setTerminalSize(tea.WindowSizeMsg{Width: 160, Height: 34})
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(testui.Press("left"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.detail.Detail.Summary.ID != "tm-child" {
		t.Fatalf("setup: expected to land on the child, got %q", m.detail.Detail.Summary.ID)
	}
	return m
}

// TestModelShellActionsTargetTheDrilledIssue pins that every shell action acts
// on the issue Detail is showing.
//
// The drill-in handler set detail.SelectionID and TargetID but never the
// shell's own selection, which only the browse SelectionChangedMsg handler
// wrote. currentSelection() therefore still returned the board's row, so the
// close key closed the epic, the update and comment keys acted on it, and
// the launchers interpolated its ID — with nothing on screen to say so, because
// the follow-up detail load was dropped by the TargetID guard.
func TestModelShellActionsTargetTheDrilledIssue(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	m := drilledIntoChild(t, gw)

	if got := firstSelectionID(m, mode.Board); got != "tm-epic" {
		t.Fatalf("setup: expected the board row to stay on the epic, got %q", got)
	}

	id, ok := m.selectedIssueID()
	if !ok || id != "tm-child" {
		t.Errorf("selectedIssueID() = %q (ok=%v), want tm-child — this is the id the issue keys act on", id, ok)
	}

	issueContext, ok := m.selectedIssueContext()
	if !ok || issueContext.Summary.ID != "tm-child" {
		t.Errorf("selectedIssueContext() = %q, want tm-child — this is what the launchers interpolate", issueContext.Summary.ID)
	}

	for _, key := range []string{"delete", "alt+u", "alt+a"} {
		editing := pressKey(t, m, key)
		if editing.active != mode.IssueEdit || editing.editorTarget.issueID != "tm-child" {
			t.Errorf("%q opened the editor on %q (surface %q), want tm-child", key, editing.editorTarget.issueID, editing.active)
		}
	}
}

// TestModelAutoRefreshKeepsTheDrilledIssueOnScreen pins the same selection
// against the 60s tick and the focus-regain refresh, which reloaded detail from
// currentSelection() and so navigated back to the parent on their own.
func TestModelAutoRefreshKeepsTheDrilledIssueOnScreen(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	m := drilledIntoChild(t, gw)

	cmd := m.refreshActiveSurfaceCmd()
	if cmd == nil {
		t.Fatal("expected the detail surface to refresh")
	}
	if m.detail.TargetID() != "tm-child" {
		t.Errorf("auto-refresh retargeted detail to %q, want tm-child", m.detail.TargetID())
	}

	m = applyMessages(t, m, runBatch(cmd))
	if m.detail.Detail.Summary.ID != "tm-child" {
		t.Errorf("after the refresh the screen shows %q, want tm-child", m.detail.Detail.Summary.ID)
	}
}

// TestModelEscapeFromDrillReturnsTheSelectionToTheBrowseRow is the other half
// of the contract: once Detail is left, the browse tab owns the selection again.
func TestModelEscapeFromDrillReturnsTheSelectionToTheBrowseRow(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	m := drilledIntoChild(t, gw)

	next, cmd := m.Update(testui.Press("esc"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Board {
		t.Fatalf("Escape from detail left the app in %s", m.active)
	}
	id, ok := m.selectedIssueID()
	if !ok || id != "tm-epic" {
		t.Errorf("selectedIssueID() after Escape = %q (ok=%v), want the board row tm-epic", id, ok)
	}
}

// TestModelReloadDetailKeyIssuesALoad pins that the reload key reloads.
//
// It set detail.Loading and then called ensureDetailForCurrentSelectionCmd,
// whose first guard returns nil for a target that is already loading: no load
// was issued, and the header claimed "Loading: detail" for the rest of the
// session — which, with the spinner armed by the loading state, also meant a
// frame every 100ms forever.
func TestModelReloadDetailKeyIssuesALoad(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	m := drilledIntoChild(t, gw)

	mark := gw.CallCount()
	next, cmd := m.Update(testui.Press("alt+r"))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("the reload key produced no command")
	}
	m = applyMessages(t, m, runBatch(cmd))

	if !gw.HasCallSince(mark, fakes.MethodIssue) {
		t.Error("the reload key issued no repository.Issue call")
	}
	if m.detail.IsLoading() {
		t.Error("detail is still loading after the reload resolved")
	}
	if m.detail.Detail.Summary.ID != "tm-child" {
		t.Errorf("reload landed on %q, want tm-child", m.detail.Detail.Summary.ID)
	}
}
