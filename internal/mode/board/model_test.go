package board

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	beads "github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/repository"
	repositorybeads "github.com/hk9890/beads-workbench/internal/repository/beads"
	memoryrepo "github.com/hk9890/beads-workbench/internal/repository/memory"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
	"github.com/hk9890/beads-workbench/internal/ui/shared/issuerow"
)

func resolvedBoardKeys(t *testing.T) config.ResolvedKeyBindings {
	t.Helper()
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	return keys
}

// newBoardModel builds a test board model with a no-op logger.
func newBoardModel(repo repository.Repository, keys config.ResolvedKeyBindings) *Model {
	return NewModel(repo, slog.Default(), keys)
}

// feedDashboardData injects a dashboardLoadedMsg with the given data into the model.
func feedDashboardData(m *Model, data repository.DashboardData) {
	_ = m.Update(dashboardLoadedMsg{data: data, err: nil})
}

// feedDashboardErr injects a dashboardLoadedMsg with an error into the model.
func feedDashboardErr(m *Model, err error) {
	_ = m.Update(dashboardLoadedMsg{err: err})
}

// --- AC: exactly 1 repository call dispatched on Init ---

func TestBoardModeInitDispatchesSingleDashboardCall(t *testing.T) {
	t.Parallel()

	repo := repository.NewErrorInjecting(memoryrepo.New())
	m := newBoardModel(repo, resolvedBoardKeys(t))

	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("Init() must return a non-nil command")
	}
	// Execute the command — it calls repo.Dashboard().
	_ = cmd()

	calls := repo.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 repository call on Init, got %d: %v", len(calls), calls)
	}
	if calls[0].Method != repository.MethodDashboard {
		t.Errorf("expected Dashboard call, got %v", calls[0].Method)
	}
}

// --- AC: Init produces a single non-batch command (not a tea.Batch) ---

func TestBoardModeInitProducesNonBatchCmd(t *testing.T) {
	t.Parallel()

	repo := memoryrepo.New()
	m := newBoardModel(repo, resolvedBoardKeys(t))

	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("Init() must return a non-nil command")
	}
	// The command must NOT be a batch; it is a single loadDashboardCmd.
	msg := cmd()
	if _, ok := msg.(tea.BatchMsg); ok {
		t.Fatalf("Init() must not return a tea.Batch after the repo migration; got tea.BatchMsg")
	}
	if _, ok := msg.(dashboardLoadedMsg); !ok {
		t.Fatalf("Init() command must produce a dashboardLoadedMsg; got %T", msg)
	}
}

// --- AC: all-empty load ---

func TestBoardModeAllEmptyLoad(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	// Use a wide enough terminal so all 4 columns are visible.
	m.SetSize(200, 30)

	// Feed empty dashboard result.
	feedDashboardData(m, repository.DashboardData{})

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after dashboard result arrived")
	}
	for _, col := range m.columns {
		if col.err != nil {
			t.Fatalf("expected no column errors, got col %q err: %v", col.title, col.err)
		}
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}

	view := m.View(0)
	// All 4 section titles must appear in a wide render.
	for _, title := range []string{sectionTitleNotReady, sectionTitleReady, sectionTitleInProgress, sectionTitleDone} {
		if !strings.Contains(view, title) {
			t.Errorf("expected column title %q in view, got: %s", title, view)
		}
	}
}

// --- AC: all 3 groups populated ---

func TestBoardModeAllGroupsPopulatedRendersGolden(t *testing.T) {
	t.Parallel()

	repo := memoryrepo.New()
	repo.Seed(memoryrepo.Issue{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"})
	repo.Seed(memoryrepo.Issue{ID: "bw-2", Title: "Ready second", Priority: 2, Status: "open", Type: "task"})
	repo.Seed(memoryrepo.Issue{ID: "bw-3", Title: "In progress", Priority: 2, Status: "in_progress", Type: "feature"})
	repo.Seed(memoryrepo.Issue{ID: "bw-4", Title: "Blocked now", Priority: 1, Status: "blocked", Type: "bug"})

	tm := testui.NewTestModel(t, testui.ControllerAdapter{Controller: newBoardModel(repo, resolvedBoardKeys(t))})
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})

	// At width=120, 3 of 4 columns are visible (Not Ready, Ready, In Progress).
	// Done is offscreen; only check visible sections and known issue IDs.
	testui.WaitForOutputContainsAll(t, tm.Output(),
		sectionTitleNotReady, sectionTitleReady, sectionTitleInProgress,
		"bw-1", "bw-4",
	)

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}

	final, ok := tm.FinalModel(t).(testui.ControllerAdapter)
	if !ok {
		t.Fatalf("expected final model adapter")
	}
	finalModel, ok := final.Controller.(*Model)
	if !ok {
		t.Fatalf("expected wrapped board model, got %T", final.Controller)
	}

	if sel := finalModel.CurrentSelection(); sel == nil || sel.Issue.ID != "bw-4" {
		t.Fatalf("expected initial selection bw-4 from Not Ready lane (earliest non-empty), got %#v", sel)
	}

	testui.AssertMatchesGoldenNormalized(t, []byte(finalModel.View(0)), "model_loaded.golden")
}

// --- AC: load error path (single error on all columns) ---

func TestBoardModeLoadErrorAffectsAllColumns(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.SetSize(200, 30)

	loadErr := errors.New("network timeout")
	feedDashboardErr(m, loadErr)

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after dashboard result arrived")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}
	// All 4 columns must carry the error.
	for _, col := range m.columns {
		if col.err == nil || !strings.Contains(col.err.Error(), "network timeout") {
			t.Errorf("expected load error on column %q, got: %v", col.title, col.err)
		}
	}

	// View must render 4-column layout (never the old loading.View).
	view := m.View(0)
	for _, title := range []string{sectionTitleNotReady, sectionTitleReady, sectionTitleInProgress, sectionTitleDone} {
		if !strings.Contains(view, title) {
			t.Errorf("expected column title %q in view even on error, got: %s", title, view)
		}
	}
}

// TestBoardModeLoadErrorSingleErrorOnAllColumns verifies that a single load
// error applies to all 4 columns (consistent with repository.Dashboard atomicity).
func TestBoardModeLoadErrorSingleErrorOnAllColumns(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.SetSize(200, 30)

	loadErr := errors.New("bd unavailable")
	feedDashboardErr(m, loadErr)

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after dashboard result arrived")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}
	// All 4 columns carry the same error.
	for _, col := range m.columns {
		if col.err == nil || !strings.Contains(col.err.Error(), "bd unavailable") {
			t.Errorf("expected load error on all columns, column %q got: %v", col.title, col.err)
		}
	}
}

// --- Navigation tests ---

func TestBoardModeNavigationEmitsSelectionChangedAndActionRequest(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.columns = []columnData{
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "bw-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}, {ID: "bw-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"}}, total: 2, exact: true},
	}
	m.focusedColumn = 0
	m.selectedRow[0] = 0
	m.selectedRow[1] = 0
	m.SetSize(100, 24)

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving right")
	}
	msg := cmd()
	selChanged, ok := msg.(mode.SelectionChangedMsg)
	if !ok {
		t.Fatalf("expected SelectionChangedMsg, got %T", msg)
	}
	if selChanged.Selection == nil || selChanged.Selection.Issue.ID != "bw-7" {
		t.Fatalf("expected selection bw-7 after moving right, got %#v", selChanged.Selection)
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatalf("expected selection changed command after moving down")
	}
	msg = cmd()
	selChanged, ok = msg.(mode.SelectionChangedMsg)
	if !ok {
		t.Fatalf("expected SelectionChangedMsg, got %T", msg)
	}
	if selChanged.Selection == nil || selChanged.Selection.Issue.ID != "bw-8" {
		t.Fatalf("expected selection bw-8 after moving down, got %#v", selChanged.Selection)
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected action request command on enter")
	}
	msg = cmd()
	action, ok := msg.(mode.ActionRequestMsg)
	if !ok {
		t.Fatalf("expected ActionRequestMsg, got %T", msg)
	}
	if action.Action != mode.ActionOpenDetail {
		t.Fatalf("expected action %q got %q", mode.ActionOpenDetail, action.Action)
	}

	testui.AssertMatchesGoldenNormalized(t, []byte(m.View(0)), "model_navigation.golden")
}

func TestBoardModeUsesConfiguredBindings(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.MergeKeyBindings(config.DefaultKeyBindings(), &config.KeyBindingOverride{
		Board: map[string][]string{
			config.BoardActionMoveLeft:   {"a"},
			config.BoardActionMoveRight:  {"d"},
			config.BoardActionMoveUp:     {"w"},
			config.BoardActionMoveDown:   {"s"},
			config.BoardActionOpenDetail: {"space"},
			config.BoardActionReload:     {"R"},
		},
	}))
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	m := newBoardModel(memoryrepo.New(), keys)
	m.columns = []columnData{
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "bw-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}, {ID: "bw-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"}}, total: 2, exact: true},
	}
	m.focusedColumn = 0
	m.selectedRow[0] = 0
	m.selectedRow[1] = 0
	m.SetSize(100, 24)

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd == nil {
		t.Fatal("expected selection change after configured move-right key")
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("expected selection change after configured move-down key")
	}
	msg := cmd()
	selChanged, ok := msg.(mode.SelectionChangedMsg)
	if !ok || selChanged.Selection == nil || selChanged.Selection.Issue.ID != "bw-8" {
		t.Fatalf("expected configured move-down to select bw-8, got %#v", msg)
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if cmd == nil {
		t.Fatal("expected action request from configured open key")
	}
	if action, ok := cmd().(mode.ActionRequestMsg); !ok || action.Action != mode.ActionOpenDetail {
		t.Fatalf("expected open detail action request, got %#v", cmd())
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil || !m.IsLoading() {
		t.Fatal("expected configured reload key to trigger board reload")
	}
}

// --- Auto-refresh anchor tests ---

func populatedModel(repo repository.Repository, keys config.ResolvedKeyBindings) *Model {
	m := newBoardModel(repo, keys)
	m.columns = []columnData{
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready one"}}, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "bw-2", Title: "Progress one"}, {ID: "bw-3", Title: "Progress two"}}, total: 2, exact: true},
	}
	m.focusedColumn = 1
	m.selectedRow[0] = 0
	m.selectedRow[1] = 1
	return m
}

func feedAllResults(m *Model, readyExplain domain.ReadyExplainResult, inProgress []domain.IssueSummary, closed []domain.IssueSummary) {
	feedDashboardData(m, repository.DashboardData{
		ReadyExplain: readyExplain,
		InProgress:   inProgress,
		Closed:       closed,
		ClosedTotal:  len(closed),
	})
}

func TestBoardModeAutoRefreshPreservesFocusedIssueSelectionWhenPresent(t *testing.T) {
	t.Parallel()

	m := populatedModel(memoryrepo.New(), resolvedBoardKeys(t))

	cmd := m.AutoRefresh()
	if cmd == nil {
		t.Fatalf("expected auto-refresh command")
	}

	feedAllResults(m,
		domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{{ID: "bw-9", Title: "Ready refreshed"}},
		},
		[]domain.IssueSummary{
			{ID: "bw-8", Title: "Progress refreshed one"},
			{ID: "bw-3", Title: "Progress two still here"},
			{ID: "bw-10", Title: "Progress refreshed three"},
		},
		nil,
	)

	// After refresh: columns are [NotReady(0), Ready(1), InProgress(2), Done(3)].
	// bw-3 is in InProgress = column 2.
	if m.focusedColumn != 2 {
		t.Fatalf("expected focused column 2 (InProgress) to be restored via anchor, got %d", m.focusedColumn)
	}

	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "bw-3" {
		t.Fatalf("expected preserved selected issue bw-3, got %#v", sel)
	}
}

func TestBoardModeAutoRefreshDeterministicFallbackWhenSelectedIssueDisappears(t *testing.T) {
	t.Parallel()

	m := populatedModel(memoryrepo.New(), resolvedBoardKeys(t))

	cmd := m.AutoRefresh()
	if cmd == nil {
		t.Fatalf("expected auto-refresh command")
	}

	feedAllResults(m,
		domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{{ID: "bw-11", Title: "Ready refreshed"}},
		},
		[]domain.IssueSummary{
			{ID: "bw-12", Title: "Progress replacement"},
		},
		nil,
	)

	// After refresh: columns are [NotReady(0), Ready(1), InProgress(2), Done(3)].
	// bw-3 (anchor issue) is gone. The anchor's prior focusedColumn was 1 (InProgress
	// in the 2-column model). Column 1 in the new 4-column model is Ready (has bw-11).
	// restoreFromAnchor clamps the prior focusedColumn (1) and selects it.
	if m.focusedColumn != 1 {
		t.Fatalf("expected fallback to clamped prior focused column 1 (Ready), got %d", m.focusedColumn)
	}
	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "bw-11" {
		t.Fatalf("expected deterministic row-clamp fallback selection bw-11, got %#v", sel)
	}
}

func TestBoardModeManualReloadRemainsFullResetBehavior(t *testing.T) {
	t.Parallel()

	m := populatedModel(memoryrepo.New(), resolvedBoardKeys(t))

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatalf("expected manual reload command")
	}

	feedAllResults(m,
		domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{{ID: "bw-21", Title: "Ready refreshed"}},
		},
		[]domain.IssueSummary{
			{ID: "bw-22", Title: "Progress refreshed"},
		},
		nil,
	)

	// Manual reload: focus settles to first available non-empty column.
	// Columns: [NotReady(empty,0), Ready(bw-21,1), InProgress(bw-22,2), Done(empty,3)]
	// First non-empty is col 1 (Ready, has bw-21).
	if m.focusedColumn != 1 {
		t.Fatalf("expected manual reload to reset focus to first non-empty column (Ready, col 1), got %d", m.focusedColumn)
	}
	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "bw-21" {
		t.Fatalf("expected manual reload selection to be bw-21 (first issue in first non-empty col), got %#v", sel)
	}
}

// --- Per-column loading state ---

func TestBoardModePerColumnLoadingState(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.SetSize(200, 30)

	// Phase 1: initial loading state — all 4 columns are loading.
	if !m.IsLoading() {
		t.Fatal("expected IsLoading()=true before any results")
	}
	for i, col := range m.columns {
		if !col.loading {
			t.Errorf("expected column %d (%q) loading=true in cold start", i, col.title)
		}
	}
	// View must render 4-column layout with skeleton rows, not a full-screen loading message.
	view := m.View(0)
	for _, title := range []string{sectionTitleNotReady, sectionTitleReady, sectionTitleInProgress, sectionTitleDone} {
		if !strings.Contains(view, title) {
			t.Errorf("expected column title %q during cold-start, got: %s", title, view)
		}
	}
	if !strings.Contains(view, issuerow.SkeletonGlyph) {
		t.Fatalf("expected skeleton glyph %q during cold-start loading, got: %s", issuerow.SkeletonGlyph, view)
	}

	// Phase 2: dashboard result arrives — loading clears.
	feedDashboardData(m, repository.DashboardData{})

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after dashboard result arrived")
	}
	for i, col := range m.columns {
		if col.loading {
			t.Errorf("expected column %d (%q) loading=false after composition", i, col.title)
		}
	}
}

// --- New tests ---

// TestBoardModeColdStartAllColumnsLoading verifies that after NewModel,
// all 4 columns have loading=true and IsLoading() returns true.
func TestBoardModeColdStartAllColumnsLoading(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))

	if !m.IsLoading() {
		t.Fatal("expected IsLoading()=true immediately after NewModel")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns in cold start, got %d", len(m.columns))
	}
	for i, col := range m.columns {
		if !col.loading {
			t.Errorf("expected column %d (%q) loading=true in cold start", i, col.title)
		}
		if col.err != nil {
			t.Errorf("expected column %d (%q) err=nil in cold start, got: %v", i, col.title, col.err)
		}
		if len(col.issues) != 0 {
			t.Errorf("expected column %d (%q) empty in cold start, got %d issues", i, col.title, len(col.issues))
		}
	}
}

// TestBoardModeAtomicSwapAllColumnsAfterDashboard verifies that after the
// dashboard result arrives, all 4 columns have loading=false.
func TestBoardModeAtomicSwapAllColumnsAfterDashboard(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))

	feedDashboardData(m, repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{{ID: "bw-1", Title: "Ready one", Status: "open", Type: "task", Priority: 1}},
		},
		InProgress: []domain.IssueSummary{{ID: "bw-2", Title: "IP one", Status: "in_progress", Type: "task", Priority: 1}},
	})

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after dashboard result arrived")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}
	for i, col := range m.columns {
		if col.loading {
			t.Errorf("expected column %d (%q) loading=false after atomic swap", i, col.title)
		}
	}
}

// TestBoardModeKeyboardNavigationNoopWhenAllColumnsEmpty verifies that
// keyboard navigation during full cold-start (all columns empty) is a no-op
// and does not panic.
func TestBoardModeKeyboardNavigationNoopWhenAllColumnsEmpty(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.SetSize(200, 30)

	// All columns are in cold-start loading state with no issues.
	// Navigation key presses must not panic.
	keyTests := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyLeft},
		tea.KeyMsg{Type: tea.KeyRight},
		tea.KeyMsg{Type: tea.KeyUp},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
	}

	for _, k := range keyTests {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("keyboard navigation panicked during cold-start: %v", r)
				}
			}()
			_ = m.Update(k)
		}()
	}

	if m.CurrentSelection() != nil {
		t.Fatalf("expected nil selection when all columns empty, got: %#v", m.CurrentSelection())
	}
}

// TestBoardModeRefreshKeepsStaleIssuesVisible verifies that on auto-refresh,
// columns with existing issues keep them visible while loading=true.
func TestBoardModeRefreshKeepsStaleIssuesVisible(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))

	// Seed the model with loaded data as if a prior load already completed.
	m.columns = []columnData{
		{title: sectionTitleNotReady, issues: nil, loading: false},
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "bw-1", Title: "Ready one"}}, loading: false, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "bw-2", Title: "IP one"}}, loading: false, total: 1, exact: true},
		{title: sectionTitleDone, issues: nil, loading: false},
	}

	// Trigger auto-refresh (marks columns loading while preserving issues).
	_ = m.AutoRefresh()

	// All columns must now be loading.
	if !m.IsLoading() {
		t.Fatal("expected IsLoading()=true during auto-refresh")
	}
	// Columns with prior issues must still have them visible (stale rendering).
	if len(m.columns[1].issues) == 0 {
		t.Errorf("expected Ready column to retain stale issues during refresh, got empty")
	}
	if !m.columns[1].loading {
		t.Errorf("expected Ready column loading=true during auto-refresh")
	}
	if len(m.columns[2].issues) == 0 {
		t.Errorf("expected InProgress column to retain stale issues during refresh, got empty")
	}
}

// --- closedLimit ---

func TestClosedLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		height int
		want   int
	}{
		{height: 0, want: 50},   // default: max(50, 20)
		{height: 10, want: 50},  // 10-3=7; max(50,7)=50
		{height: 53, want: 50},  // 53-3=50; max(50,50)=50
		{height: 60, want: 57},  // 60-3=57; max(50,57)=57
		{height: 100, want: 97}, // 100-3=97; max(50,97)=97
	}

	for _, tc := range tests {
		m := &Model{height: tc.height}
		got := m.closedLimit()
		if got != tc.want {
			t.Errorf("closedLimit() with height=%d: got %d, want %d", tc.height, got, tc.want)
		}
	}
}

// TestClosedLimitWiredThroughDashboardOpts asserts that loadDashboardCmd passes
// m.closedLimit() as DashboardOptions.ClosedLimit to the Repository. This is
// iwvm Success Criterion #1: the wiring is verified in CI without terminal resize.
func TestClosedLimitWiredThroughDashboardOpts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		height      int
		wantedLimit int
	}{
		{name: "default (height=0)", height: 0, wantedLimit: 50},   // max(50, 20) = 50
		{name: "small terminal", height: 10, wantedLimit: 50},      // max(50, 7) = 50
		{name: "medium terminal", height: 53, wantedLimit: 50},     // max(50, 50) = 50
		{name: "tall terminal", height: 60, wantedLimit: 57},       // max(50, 57) = 57
		{name: "very tall terminal", height: 100, wantedLimit: 97}, // max(50, 97) = 97
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &optRecordingRepo{inner: memoryrepo.New()}
			m := newBoardModel(rec, resolvedBoardKeys(t))
			m.SetSize(200, tc.height)

			cmd := m.Init()
			if cmd == nil {
				t.Fatalf("Init() must return a non-nil command")
			}
			_ = cmd()

			opts := rec.lastOpts()
			if opts.ClosedLimit != tc.wantedLimit {
				t.Errorf("height=%d: DashboardOptions.ClosedLimit=%d; want %d (closedLimit())",
					tc.height, opts.ClosedLimit, tc.wantedLimit)
			}
		})
	}
}

// optRecordingRepo records the DashboardOptions passed on each Dashboard call.
// It delegates to inner for the actual result.
type optRecordingRepo struct {
	mu    sync.Mutex
	inner repository.Repository
	opts  []repository.DashboardOptions
}

func (r *optRecordingRepo) lastOpts() repository.DashboardOptions {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.opts) == 0 {
		return repository.DashboardOptions{}
	}
	return r.opts[len(r.opts)-1]
}

func (r *optRecordingRepo) Dashboard(ctx context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	r.mu.Lock()
	r.opts = append(r.opts, opts)
	r.mu.Unlock()
	return r.inner.Dashboard(ctx, opts)
}
func (r *optRecordingRepo) Issue(ctx context.Context, id string) (domain.IssueDetail, error) {
	return r.inner.Issue(ctx, id)
}
func (r *optRecordingRepo) Search(ctx context.Context, q domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return r.inner.Search(ctx, q)
}
func (r *optRecordingRepo) CreateIssue(ctx context.Context, inp domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return r.inner.CreateIssue(ctx, inp)
}
func (r *optRecordingRepo) UpdateIssue(ctx context.Context, id string, inp domain.UpdateIssueInput) error {
	return r.inner.UpdateIssue(ctx, id, inp)
}
func (r *optRecordingRepo) CloseIssue(ctx context.Context, id string, inp domain.CloseIssueInput) error {
	return r.inner.CloseIssue(ctx, id, inp)
}
func (r *optRecordingRepo) AddComment(ctx context.Context, id string, inp domain.AddCommentInput) error {
	return r.inner.AddComment(ctx, id, inp)
}
func (r *optRecordingRepo) HealthCheck(ctx context.Context) error {
	return r.inner.HealthCheck(ctx)
}
func (r *optRecordingRepo) Catalogs(ctx context.Context) (repository.Catalogs, error) {
	return r.inner.Catalogs(ctx)
}

// --- sectionItemCapacity ---

func TestSectionItemCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		height int
		want   int
	}{
		{height: 0, want: 20},  // safe default before first WindowSizeMsg
		{height: 1, want: 1},   // clamp: 1-3=-2, clamped to 1
		{height: 3, want: 1},   // clamp: 3-3=0, clamped to 1
		{height: 4, want: 1},   // 4-3=1
		{height: 24, want: 21}, // 24-3=21
		{height: 30, want: 27}, // 30-3=27
		{height: 34, want: 31}, // 34-3=31
	}

	for _, tc := range tests {
		m := &Model{height: tc.height}
		got := m.sectionItemCapacity()
		if got != tc.want {
			t.Errorf("sectionItemCapacity() with height=%d: got %d, want %d", tc.height, got, tc.want)
		}
	}
}

// --- slog warning capture ---

func TestBoardModeComposerWarningsEmittedToSlog(t *testing.T) {
	t.Parallel()

	// Capture slog output by using a custom handler.
	var capturedMessages []string
	handler := &captureHandler{capture: &capturedMessages}
	logger := slog.New(handler)

	m := NewModel(memoryrepo.New(), logger, resolvedBoardKeys(t))

	// Feed empty dashboard result. No warnings expected from empty inputs.
	feedDashboardData(m, repository.DashboardData{})

	// With empty inputs no cardinality warning is expected.
	if len(capturedMessages) != 0 {
		t.Fatalf("expected no slog warnings for empty inputs, got %v", capturedMessages)
	}
}

// TestBoardModeWarningLogNoDuplicateComponentKey asserts that warning records
// emitted via compose contain exactly one "component" JSON key.
func TestBoardModeWarningLogNoDuplicateComponentKey(t *testing.T) {
	t.Parallel()

	// Use a real JSON handler writing to a buffer so we can inspect raw output.
	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	// Simulate what main.go does: attach component=dashboard to the parent logger.
	logger := slog.New(jsonHandler).With("component", "dashboard")

	m := NewModel(memoryrepo.New(), logger, resolvedBoardKeys(t))

	// Build 501 ready issues — enough to exceed the 500-item cardinality threshold
	// and trigger a "cardinality threshold exceeded" warning from dashboard.Compose.
	ready := make([]domain.IssueSummary, 501)
	for i := range ready {
		ready[i] = domain.IssueSummary{ID: fmt.Sprintf("bw-%d", i+1), Title: fmt.Sprintf("Issue %d", i+1)}
	}

	feedDashboardData(m, repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{Ready: ready},
	})

	output := buf.String()
	if output == "" {
		t.Fatal("expected at least one slog warning record, got empty output")
	}

	// Count occurrences of `"component":` in each JSON line.
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		count := strings.Count(line, `"component":`)
		if count != 1 {
			t.Errorf("expected exactly 1 occurrence of \"component\": in log line, got %d\nline: %s", count, line)
		}
	}
}

// TestBoardModeLogCarriesComponentBoard asserts that warning records emitted by
// the board model carry component=board.
func TestBoardModeLogCarriesComponentBoard(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	rootLogger := slog.New(jsonHandler)
	boardLogger := rootLogger.With("component", "board")

	m := NewModel(memoryrepo.New(), boardLogger, resolvedBoardKeys(t))

	// 501 ready issues exceeds the 500-item cardinality threshold.
	ready := make([]domain.IssueSummary, 501)
	for i := range ready {
		ready[i] = domain.IssueSummary{ID: fmt.Sprintf("bw-%d", i+1), Title: fmt.Sprintf("Issue %d", i+1)}
	}

	feedDashboardData(m, repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{Ready: ready},
	})

	output := buf.String()
	if output == "" {
		t.Fatal("expected at least one slog warning record, got empty output")
	}

	// Every emitted record must carry exactly one "component" key with value "board".
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		count := strings.Count(line, `"component":`)
		if count != 1 {
			t.Errorf("expected exactly 1 \"component\" key, got %d\nline: %s", count, line)
		}
		if !strings.Contains(line, `"component":"board"`) {
			t.Errorf("expected component=board in log line, got:\n%s", line)
		}
	}
}

// captureHandler is a minimal slog.Handler that captures log messages for assertions.
type captureHandler struct {
	capture *[]string
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	*h.capture = append(*h.capture, record.Message)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

// --- layout goldens ---

func TestBoardModeDashboardLayoutGoldensAcrossWidths(t *testing.T) {
	t.Parallel()

	readyExplain := domain.ReadyExplainResult{
		Ready: []domain.IssueSummary{
			{ID: "bw-1", Title: "Ready fix login prompt", Priority: 1, Status: "open", Type: "task"},
			{ID: "bw-5", Title: "Ready improve docs outline", Priority: 2, Status: "open", Type: "task"},
			{ID: "bw-6", Title: "Ready triage inbox", Priority: 2, Status: "open", Type: "chore"},
		},
		Blocked: []domain.BlockedIssueView{
			{Issue: domain.IssueSummary{ID: "bw-3", Title: "Blocked: API contract pending", Priority: 0, Status: "blocked", Type: "bug"}},
			{Issue: domain.IssueSummary{ID: "bw-9", Title: "Blocked: migration sequencing", Priority: 1, Status: "blocked", Type: "task"}},
		},
	}
	inProgress := []domain.IssueSummary{
		{ID: "bw-2", Title: "Implement board keyboard shortcuts", Priority: 1, Status: "in_progress", Type: "feature"},
		{ID: "bw-7", Title: "Wire detail reload behavior", Priority: 1, Status: "in_progress", Type: "task"},
		{ID: "bw-8", Title: "Polish header help copy", Priority: 2, Status: "in_progress", Type: "docs"},
	}

	tests := []struct {
		name     string
		width    int
		height   int
		golden   string
		mustShow []string
		minMeta  int
	}{
		{name: "w80", width: 80, height: 28, golden: "model_layout_w80.golden", mustShow: []string{sectionTitleNotReady, sectionTitleReady, "bw-1"}, minMeta: 4},
		{name: "w120", width: 120, height: 30, golden: "model_layout_w120.golden", mustShow: []string{sectionTitleNotReady, sectionTitleInProgress, "bw-1", "bw-3"}, minMeta: 6},
		{name: "w180", width: 180, height: 34, golden: "model_layout_w180.golden", mustShow: []string{sectionTitleNotReady, sectionTitleDone, "bw-1", "bw-2", "bw-3"}, minMeta: 8},
	}

	data := repository.DashboardData{
		ReadyExplain: readyExplain,
		InProgress:   inProgress,
		Closed:       inProgress, // use same slice to match original golden
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
			m.SetSize(tc.width, tc.height)
			_ = m.Update(dashboardLoadedMsg{data: data})

			view := m.View(0)
			for _, snippet := range tc.mustShow {
				if !strings.Contains(view, snippet) {
					t.Fatalf("expected view to contain %q\nview:\n%s", snippet, view)
				}
			}
			testui.AssertMatchesGoldenNormalized(t, []byte(view), tc.golden)
			assertCompactIssueRows(t, view, tc.minMeta)
		})
	}
}

// --- Real Repository + RecordingExecutor subprocess-argv scenario ---

// TestBoardInitRealRepositorySubprocessArgvCardinality wires the board model against
// a real *beads.Repository + *beads.CommandRunner backed by a *fakes.RecordingExecutor.
// It asserts that exactly 5 subprocess invocations occur when the single Init cmd
// is executed (Dashboard fans out ReadyExplain + 3 Query + 1 Count inside repo.Dashboard).
func TestBoardInitRealRepositorySubprocessArgvCardinality(t *testing.T) {
	t.Parallel()

	// Expected argv shapes for the 5 subprocess invocations the board fires.
	// Closed query uses the fixed defaultClosedLimit=50 from repository/beads.
	argvReadyExplain := []string{"ready", "--explain", "--json"}
	argvQueryInProgress := []string{"query", "status=in_progress", "--json"}
	argvQueryClosed := []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "50"}
	argvCountClosed := []string{"count", "--by-status", "--json", "--status", "closed"}
	argvQueryStoredBlocked := []string{"query", "status=blocked", "--json"}

	rec := fakes.NewRecordingExecutor()

	// Pre-register canned responses so the repository parse path succeeds.
	rec.OnArgs(argvReadyExplain).Return(beads.ExecResult{Stdout: []byte(`{
		"ready": [
			{"id":"bw-r1","title":"Ready one","status":"open","issue_type":"task","priority":1,"owner":"alice","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
		],
		"blocked": [],
		"summary": {"total_ready": 1, "total_blocked": 0, "cycle_count": 0}
	}`)}, nil)

	rec.OnArgs(argvQueryInProgress).Return(beads.ExecResult{Stdout: []byte(`[
		{"id":"bw-p1","title":"In progress one","status":"in_progress","issue_type":"task","priority":2,"owner":"bob","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
	]`)}, nil)

	rec.OnArgs(argvQueryClosed).Return(beads.ExecResult{Stdout: []byte(`[
		{"id":"bw-c1","title":"Closed one","status":"closed","issue_type":"task","priority":1,"owner":"carol","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
	]`)}, nil)

	rec.OnArgs(argvCountClosed).Return(beads.ExecResult{Stdout: []byte(`{
		"groups": [{"group": "closed", "count": 452}],
		"total": 452,
		"schema_version": 1
	}`)}, nil)

	rec.OnArgs(argvQueryStoredBlocked).Return(beads.ExecResult{Stdout: []byte(`[
		{"id":"bw-b1","title":"Stored blocked one","status":"blocked","issue_type":"task","priority":2,"owner":"eve","created_at":"2026-04-05T09:00:00Z","updated_at":"2026-04-05T10:00:00Z"}
	]`)}, nil)

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		Command:  "bd",
		Executor: rec,
	})
	repo := repositorybeads.New(runner)

	m := NewModel(repo, slog.Default(), resolvedBoardKeys(t))

	// Drive Init: board.Init() now returns a single loadDashboardCmd (not a tea.Batch).
	initCmd := m.Init()
	if initCmd == nil {
		t.Fatalf("Init() must return a non-nil command")
	}

	// Execute the command — this calls repo.Dashboard() which fans out 5 subprocess calls.
	_ = initCmd()

	calls := rec.Calls()

	// AC: exactly 5 subprocess invocations (bd ping is dispatched at app level, not here).
	if len(calls) != 5 {
		t.Fatalf("expected exactly 5 subprocess invocations on board Init, got %d: %v",
			len(calls), formatArgvList(calls))
	}

	// AC: argv for each matches the expected shape.
	assertArgvPresent(t, calls, argvReadyExplain)
	assertArgvPresent(t, calls, argvQueryInProgress)
	assertArgvPresent(t, calls, argvQueryClosed)
	assertArgvPresent(t, calls, argvCountClosed)
	assertArgvPresent(t, calls, argvQueryStoredBlocked)

	// AC: regression guard — no "list --status" argv (old data layer).
	for _, c := range calls {
		if hasArg(c.Args, "list") && hasArg(c.Args, "--status") {
			t.Errorf("forbidden 'list --status' pattern observed in call %v (old data layer regression)", c.Args)
		}
	}
}

// TestBoardModeStoredBlockedNoDependencyVisibleInNotReadyColumn is the regression
// test for beads-workbench-2ev4.2: an issue with status=blocked and no dependency
// blocker must appear in the Not Ready column.
func TestBoardModeStoredBlockedNoDependencyVisibleInNotReadyColumn(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.SetSize(200, 30)

	data := repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{
				{ID: "bwf-1", Title: "Seed fixture root task", Status: "open", Type: "task", Priority: 1},
			},
			Blocked: []domain.BlockedIssueView{
				// bwf-2 is dep-blocked (bwf-1 blocks bwf-2) — it should appear in Not Ready exactly once.
				{Issue: domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0}},
			},
		},
		Blocked: []domain.IssueSummary{
			// bwf-5: stored-blocked with no dependency — must appear in Not Ready.
			{ID: "bwf-5", Title: "Stored-blocked task with no dependency", Status: "blocked", Type: "task", Priority: 2},
			// bwf-2: also in ReadyExplain.Blocked above — must appear only once after dedup.
			{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0},
		},
	}
	feedDashboardData(m, data)

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after dashboard result")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(m.columns))
	}

	notReady := m.columns[0]
	if notReady.title != sectionTitleNotReady {
		t.Fatalf("expected column 0 to be Not Ready, got %q", notReady.title)
	}

	// AC 1: bwf-5 (stored-blocked, no dependency) must appear in Not Ready.
	bwf5Found := false
	for _, issue := range notReady.issues {
		if issue.ID == "bwf-5" {
			bwf5Found = true
			break
		}
	}
	if !bwf5Found {
		t.Errorf("bwf-5 (stored-blocked, no dep) not found in Not Ready column; got IDs: %v",
			issueIDs(notReady.issues))
	}

	// AC 2: bwf-2 appears exactly once (dedup between dep-blocked and stored-blocked).
	bwf2Count := 0
	for _, issue := range notReady.issues {
		if issue.ID == "bwf-2" {
			bwf2Count++
		}
	}
	if bwf2Count != 1 {
		t.Errorf("bwf-2 should appear exactly once in Not Ready (dedup), got %d; IDs: %v",
			bwf2Count, issueIDs(notReady.issues))
	}

	// AC 3: Not Ready total matches actual count (bwf-2 + bwf-5 = 2).
	wantTotal := 2
	if notReady.total != wantTotal {
		t.Errorf("Not Ready total = %d, want %d", notReady.total, wantTotal)
	}

	// AC 4: Ready column has bwf-1, In Progress / Done are empty.
	ready := m.columns[1]
	if len(ready.issues) != 1 || ready.issues[0].ID != "bwf-1" {
		t.Errorf("Ready column: expected [bwf-1], got %v", issueIDs(ready.issues))
	}
	if len(m.columns[2].issues) != 0 {
		t.Errorf("In Progress column: expected empty, got %v", issueIDs(m.columns[2].issues))
	}
	if len(m.columns[3].issues) != 0 {
		t.Errorf("Done column: expected empty, got %v", issueIDs(m.columns[3].issues))
	}
}

// issueIDs is a test helper that extracts IDs from a slice of IssueSummary.
func issueIDs(issues []domain.IssueSummary) []string {
	ids := make([]string, len(issues))
	for i, s := range issues {
		ids[i] = s.ID
	}
	return ids
}

// assertArgvPresent fails the test if none of the recorded calls has args
// that exactly match want.
func assertArgvPresent(t *testing.T, calls []fakes.RecordedCall, want []string) {
	t.Helper()
	for _, c := range calls {
		if reflect.DeepEqual(c.Args, want) {
			return
		}
	}
	t.Errorf("expected subprocess call with argv %v; got calls: %v", want, formatArgvList(calls))
}

// hasArg reports whether args contains the given token.
func hasArg(args []string, token string) bool {
	for _, a := range args {
		if a == token {
			return true
		}
	}
	return false
}

// formatArgvList returns a readable list of all recorded argv slices.
func formatArgvList(calls []fakes.RecordedCall) [][]string {
	out := make([][]string, len(calls))
	for i, c := range calls {
		out[i] = c.Args
	}
	return out
}

func assertCompactIssueRows(t *testing.T, view string, minIssueMetaLines int) {
	t.Helper()

	lines := strings.Split(view, "\n")
	issueRows := 0
	for _, line := range lines {
		issueRows += strings.Count(line, " P0 ")
		issueRows += strings.Count(line, " P1 ")
		issueRows += strings.Count(line, " P2 ")
		issueRows += strings.Count(line, " P3 ")
	}

	if issueRows < minIssueMetaLines {
		t.Fatalf("expected at least %d rendered issue rows, got %d\nview:\n%s", minIssueMetaLines, issueRows, view)
	}

	for _, forbidden := range []string{"Title:", "Description:", "Assignee:", "Labels:"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("expected board layout to keep compact one-line issue rows without detail-field chrome %q\nview:\n%s", forbidden, view)
		}
	}
}

// --- bd argv pinning for variable --limit (iwvm.7) ---

// TestBoardClosedQueryArgvLimitVariants pins the exact bd query argv that the
// board model emits for a range of ClosedLimit values: ClosedLimit=0 (falls back
// to default 50 in beads.Repository), ClosedLimit=50, and ClosedLimit=200.
//
// Each case wires the board model against a real *beads.Repository backed by a
// *fakes.RecordingExecutor so the exact subprocess argv is observable.
func TestBoardClosedQueryArgvLimitVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		height         int // controls m.closedLimit(); 0 = default (floor 50)
		wantClosedArgv []string
	}{
		{
			name:           "ClosedLimit_0_default_floor_50",
			height:         0, // closedLimit() returns 50 (floor)
			wantClosedArgv: []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "50"},
		},
		{
			name:           "ClosedLimit_50_exact",
			height:         53, // closedLimit() returns 50 (53-3=50, max(50,50)=50)
			wantClosedArgv: []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "50"},
		},
		{
			name:           "ClosedLimit_200",
			height:         203, // closedLimit() returns 200 (203-3=200, max(50,200)=200)
			wantClosedArgv: []string{"query", "status=closed", "--json", "-a", "--sort", "closed", "--limit", "200"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := fakes.NewRecordingExecutor()

			// Pre-register canned responses for all five Dashboard fan-out calls.
			rec.OnArgs([]string{"ready", "--explain", "--json"}).Return(beads.ExecResult{Stdout: []byte(`{
				"ready": [], "blocked": [],
				"summary": {"total_ready": 0, "total_blocked": 0, "cycle_count": 0}
			}`)}, nil)
			rec.OnArgs([]string{"query", "status=in_progress", "--json"}).Return(beads.ExecResult{Stdout: []byte(`[]`)}, nil)
			rec.OnArgs(tc.wantClosedArgv).Return(beads.ExecResult{Stdout: []byte(`[]`)}, nil)
			rec.OnArgs([]string{"count", "--by-status", "--json", "--status", "closed"}).Return(beads.ExecResult{Stdout: []byte(`{
				"groups": [{"group": "closed", "count": 0}], "total": 0, "schema_version": 1
			}`)}, nil)
			rec.OnArgs([]string{"query", "status=blocked", "--json"}).Return(beads.ExecResult{Stdout: []byte(`[]`)}, nil)

			runner := beads.NewCommandRunner(beads.RunnerConfig{
				Command:  "bd",
				Executor: rec,
			})
			repo := repositorybeads.New(runner)

			m := NewModel(repo, slog.Default(), resolvedBoardKeys(t))
			m.SetSize(200, tc.height)

			initCmd := m.Init()
			if initCmd == nil {
				t.Fatalf("Init() must return a non-nil command")
			}
			_ = initCmd()

			calls := rec.Calls()

			// Assert the exact closed-query argv is present.
			assertArgvPresent(t, calls, tc.wantClosedArgv)
		})
	}
}
