package board

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
)

func resolvedBoardKeys(t *testing.T) config.ResolvedKeyBindings {
	t.Helper()
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	return keys
}

// optsCaptureRepo is a minimal recording stub that captures the DashboardOptions
// passed to each Dashboard call. Used by tests that assert the ClosedLimit /
// ClosedOffset windowing, where ErrorInjectingRepository (which records only
// Method, not args) is insufficient.
type optsCaptureRepo struct {
	mu            sync.Mutex
	dashboardOpts []repository.DashboardOptions
}

func (r *optsCaptureRepo) Dashboard(_ context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	r.mu.Lock()
	r.dashboardOpts = append(r.dashboardOpts, opts)
	r.mu.Unlock()
	return repository.DashboardData{}, nil
}

func (r *optsCaptureRepo) capturedOpts() []repository.DashboardOptions {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]repository.DashboardOptions, len(r.dashboardOpts))
	copy(out, r.dashboardOpts)
	return out
}

// Remaining Repository methods are no-ops.
func (r *optsCaptureRepo) Issue(_ context.Context, _ string) (domain.IssueDetail, error) {
	return domain.IssueDetail{}, nil
}
func (r *optsCaptureRepo) Search(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return domain.SearchResultPage{}, nil
}
func (r *optsCaptureRepo) CreateIssue(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return domain.CreateIssueResult{}, nil
}
func (r *optsCaptureRepo) UpdateIssue(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
	return nil
}
func (r *optsCaptureRepo) CloseIssue(_ context.Context, _ string, _ domain.CloseIssueInput) error {
	return nil
}
func (r *optsCaptureRepo) AddComment(_ context.Context, _ string, _ domain.AddCommentInput) error {
	return nil
}
func (r *optsCaptureRepo) HealthCheck(_ context.Context) error { return nil }
func (r *optsCaptureRepo) Catalogs(_ context.Context) (repository.Catalogs, error) {
	return repository.Catalogs{}, nil
}

// newBoardModel builds a test board model with a no-op logger.
func newBoardModel(repo repository.Repository, keys config.ResolvedKeyBindings) *Model {
	return NewModel(context.Background(), repo, slog.Default(), keys)
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
	repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"})
	repo.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Ready second", Priority: 2, Status: "open", Type: "task"})
	repo.Seed(memoryrepo.Issue{ID: "tm-3", Title: "In progress", Priority: 2, Status: "in_progress", Type: "feature"})
	repo.Seed(memoryrepo.Issue{ID: "tm-4", Title: "Blocked now", Priority: 1, Status: "blocked", Type: "bug"})

	tm := testui.NewTestModel(t, testui.ControllerAdapter{Controller: newBoardModel(repo, resolvedBoardKeys(t))})
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})

	// At width=120, 3 of 4 columns are visible (Not Ready, Ready, In Progress).
	// Done is offscreen; only check visible sections and known issue IDs.
	testui.WaitForOutputContainsAll(t, tm.Output(),
		sectionTitleNotReady, sectionTitleReady, sectionTitleInProgress,
		"tm-1", "tm-4",
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

	if sel := finalModel.CurrentSelection(); sel == nil || sel.Issue.ID != "tm-4" {
		t.Fatalf("expected initial selection tm-4 from Not Ready lane (earliest non-empty), got %#v", sel)
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

	loadErr := errors.New("taskmgr unavailable")
	feedDashboardErr(m, loadErr)

	if m.IsLoading() {
		t.Fatal("expected IsLoading()=false after dashboard result arrived")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected 4 columns after composition, got %d", len(m.columns))
	}
	// All 4 columns carry the same error.
	for _, col := range m.columns {
		if col.err == nil || !strings.Contains(col.err.Error(), "taskmgr unavailable") {
			t.Errorf("expected load error on all columns, column %q got: %v", col.title, col.err)
		}
	}
}

// --- Navigation tests ---

func TestBoardModeNavigationEmitsSelectionChangedAndActionRequest(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.columns = []columnData{
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "tm-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "tm-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}, {ID: "tm-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"}}, total: 2, exact: true},
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
	if selChanged.Selection == nil || selChanged.Selection.Issue.ID != "tm-7" {
		t.Fatalf("expected selection tm-7 after moving right, got %#v", selChanged.Selection)
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
	if selChanged.Selection == nil || selChanged.Selection.Issue.ID != "tm-8" {
		t.Fatalf("expected selection tm-8 after moving down, got %#v", selChanged.Selection)
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
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "tm-1", Title: "Ready first", Priority: 1, Status: "open", Type: "task"}}, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "tm-7", Title: "Progress one", Priority: 2, Status: "in_progress", Type: "task"}, {ID: "tm-8", Title: "Progress two", Priority: 1, Status: "in_progress", Type: "bug"}}, total: 2, exact: true},
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
	if !ok || selChanged.Selection == nil || selChanged.Selection.Issue.ID != "tm-8" {
		t.Fatalf("expected configured move-down to select tm-8, got %#v", msg)
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
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "tm-1", Title: "Ready one"}}, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "tm-2", Title: "Progress one"}, {ID: "tm-3", Title: "Progress two"}}, total: 2, exact: true},
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
			Ready: []domain.IssueSummary{{ID: "tm-9", Title: "Ready refreshed"}},
		},
		[]domain.IssueSummary{
			{ID: "tm-8", Title: "Progress refreshed one"},
			{ID: "tm-3", Title: "Progress two still here"},
			{ID: "tm-10", Title: "Progress refreshed three"},
		},
		nil,
	)

	// After refresh: columns are [NotReady(0), Ready(1), InProgress(2), Done(3)].
	// tm-3 is in InProgress = column 2.
	if m.focusedColumn != 2 {
		t.Fatalf("expected focused column 2 (InProgress) to be restored via anchor, got %d", m.focusedColumn)
	}

	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "tm-3" {
		t.Fatalf("expected preserved selected issue tm-3, got %#v", sel)
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
			Ready: []domain.IssueSummary{{ID: "tm-11", Title: "Ready refreshed"}},
		},
		[]domain.IssueSummary{
			{ID: "tm-12", Title: "Progress replacement"},
		},
		nil,
	)

	// After refresh: columns are [NotReady(0), Ready(1), InProgress(2), Done(3)].
	// tm-3 (anchor issue) is gone. The anchor's prior focusedColumn was 1 (InProgress
	// in the 2-column model). Column 1 in the new 4-column model is Ready (has tm-11).
	// restoreFromAnchor clamps the prior focusedColumn (1) and selects it.
	if m.focusedColumn != 1 {
		t.Fatalf("expected fallback to clamped prior focused column 1 (Ready), got %d", m.focusedColumn)
	}
	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "tm-11" {
		t.Fatalf("expected deterministic row-clamp fallback selection tm-11, got %#v", sel)
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
			Ready: []domain.IssueSummary{{ID: "tm-21", Title: "Ready refreshed"}},
		},
		[]domain.IssueSummary{
			{ID: "tm-22", Title: "Progress refreshed"},
		},
		nil,
	)

	// Manual reload: focus settles to first available non-empty column.
	// Columns: [NotReady(empty,0), Ready(tm-21,1), InProgress(tm-22,2), Done(empty,3)]
	// First non-empty is col 1 (Ready, has tm-21).
	if m.focusedColumn != 1 {
		t.Fatalf("expected manual reload to reset focus to first non-empty column (Ready, col 1), got %d", m.focusedColumn)
	}
	sel := m.CurrentSelection()
	if sel == nil || sel.Issue.ID != "tm-21" {
		t.Fatalf("expected manual reload selection to be tm-21 (first issue in first non-empty col), got %#v", sel)
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
			Ready: []domain.IssueSummary{{ID: "tm-1", Title: "Ready one", Status: "open", Type: "task", Priority: 1}},
		},
		InProgress: []domain.IssueSummary{{ID: "tm-2", Title: "IP one", Status: "in_progress", Type: "task", Priority: 1}},
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
		{title: sectionTitleReady, issues: []domain.IssueSummary{{ID: "tm-1", Title: "Ready one"}}, loading: false, total: 1, exact: true},
		{title: sectionTitleInProgress, issues: []domain.IssueSummary{{ID: "tm-2", Title: "IP one"}}, loading: false, total: 1, exact: true},
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

	m := NewModel(context.Background(), memoryrepo.New(), logger, resolvedBoardKeys(t))

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

	m := NewModel(context.Background(), memoryrepo.New(), logger, resolvedBoardKeys(t))

	// Build 501 ready issues — enough to exceed the 500-item cardinality threshold
	// and trigger a "cardinality threshold exceeded" warning from dashboard.Compose.
	ready := make([]domain.IssueSummary, 501)
	for i := range ready {
		ready[i] = domain.IssueSummary{ID: fmt.Sprintf("tm-%d", i+1), Title: fmt.Sprintf("Issue %d", i+1)}
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

	m := NewModel(context.Background(), memoryrepo.New(), boardLogger, resolvedBoardKeys(t))

	// 501 ready issues exceeds the 500-item cardinality threshold.
	ready := make([]domain.IssueSummary, 501)
	for i := range ready {
		ready[i] = domain.IssueSummary{ID: fmt.Sprintf("tm-%d", i+1), Title: fmt.Sprintf("Issue %d", i+1)}
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
			{ID: "tm-1", Title: "Ready fix login prompt", Priority: 1, Status: "open", Type: "task"},
			{ID: "tm-5", Title: "Ready improve docs outline", Priority: 2, Status: "open", Type: "task"},
			{ID: "tm-6", Title: "Ready triage inbox", Priority: 2, Status: "open", Type: "chore"},
		},
		Blocked: []domain.BlockedIssueView{
			{Issue: domain.IssueSummary{ID: "tm-3", Title: "Blocked: API contract pending", Priority: 0, Status: "blocked", Type: "bug"}},
			{Issue: domain.IssueSummary{ID: "tm-9", Title: "Blocked: migration sequencing", Priority: 1, Status: "blocked", Type: "task"}},
		},
	}
	inProgress := []domain.IssueSummary{
		{ID: "tm-2", Title: "Implement board keyboard shortcuts", Priority: 1, Status: "in_progress", Type: "feature"},
		{ID: "tm-7", Title: "Wire detail reload behavior", Priority: 1, Status: "in_progress", Type: "task"},
		{ID: "tm-8", Title: "Polish header help copy", Priority: 2, Status: "in_progress", Type: "docs"},
	}

	tests := []struct {
		name     string
		width    int
		height   int
		golden   string
		mustShow []string
		minMeta  int
	}{
		{name: "w80", width: 80, height: 28, golden: "model_layout_w80.golden", mustShow: []string{sectionTitleNotReady, sectionTitleReady, "tm-1"}, minMeta: 4},
		{name: "w120", width: 120, height: 30, golden: "model_layout_w120.golden", mustShow: []string{sectionTitleNotReady, sectionTitleInProgress, "tm-1", "tm-3"}, minMeta: 6},
		{name: "w180", width: 180, height: 34, golden: "model_layout_w180.golden", mustShow: []string{sectionTitleNotReady, sectionTitleDone, "tm-1", "tm-2", "tm-3"}, minMeta: 8},
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

// TestBoardModeStoredBlockedNoDependencyVisibleInNotReadyColumn is the regression
// test for this scenario: an issue with status=blocked and no dependency
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

// --- AC: reload and auto-refresh dispatch a Dashboard call with a ClosedLimit ---

// TestStartReload_PassesClosedLimit verifies that startReload builds
// DashboardOptions with a positive ClosedLimit (the section item capacity) and
// dispatches exactly one Dashboard call, for both the full-reset reload mode
// and the background auto mode.
func TestStartReload_PassesClosedLimit(t *testing.T) {
	t.Parallel()

	for _, mode := range []refreshMode{refreshModeReload, refreshModeAuto} {
		stub := &optsCaptureRepo{}
		m := newBoardModel(stub, resolvedBoardKeys(t))

		cmd := m.startReload(mode)
		if cmd == nil {
			t.Fatalf("startReload(%v) must return a non-nil command", mode)
		}
		// Execute the command so it calls stub.Dashboard.
		_ = cmd()

		opts := stub.capturedOpts()
		if len(opts) != 1 {
			t.Fatalf("startReload(%v): expected exactly 1 Dashboard call, got %d", mode, len(opts))
		}
		if opts[0].ClosedLimit <= 0 {
			t.Errorf("startReload(%v): expected positive ClosedLimit, got %d", mode, opts[0].ClosedLimit)
		}
	}
}

// --- Scroll-window tests ---

// TestBoardModeScrollWindowAdvancesWithSelection verifies that pressing j×30
// on a column with 80 rows (height=25, sectionItemCapacity=22) advances
// the selection to row 30 and moves ScrollOffset so the selection stays
// within the visible window.
func TestBoardModeScrollWindowAdvancesWithSelection(t *testing.T) {
	t.Parallel()

	keys := resolvedBoardKeys(t)
	m := newBoardModel(memoryrepo.New(), keys)

	// Synthesize 80 Ready issues.
	const rowCount = 80
	issues := make([]domain.IssueSummary, rowCount)
	for i := range issues {
		issues[i] = domain.IssueSummary{
			ID:    fmt.Sprintf("tm-r%d", i),
			Title: fmt.Sprintf("Ready issue %d", i),
		}
	}
	m.columns = []columnData{
		{title: sectionTitleReady, issues: issues, total: rowCount, exact: true},
	}
	m.focusedColumn = 0
	m.selectedRow[0] = 0
	m.scrollOffset[0] = 0
	m.SetSize(120, 25) // sectionItemCapacity = 25-3 = 22

	capacity := m.sectionItemCapacity() // 22

	// Press j 30 times.
	const steps = 30
	for i := 0; i < steps; i++ {
		_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	sel := m.selectedRow[0]
	if sel != steps {
		t.Errorf("expected selected row %d after %d j presses, got %d", steps, steps, sel)
	}

	offset := m.scrollOffset[0]
	// Selection must be within the visible window.
	if sel < offset || sel >= offset+capacity {
		t.Errorf("selection %d not in window [%d, %d)", sel, offset, offset+capacity)
	}

	// Offset must have advanced (it can't stay at 0 when sel=30 and window=22).
	if offset == 0 {
		t.Errorf("expected scroll offset to advance from 0, got 0 with sel=%d window=%d", sel, capacity)
	}
}

// TestBoardModeScrollWindowRendererSlicesRows verifies that the board renderer
// slices the row list to the scroll window and shows "N of M" in the header
// when the window is smaller than the total row count.
func TestBoardModeScrollWindowRendererSlicesRows(t *testing.T) {
	t.Parallel()

	keys := resolvedBoardKeys(t)
	m := newBoardModel(memoryrepo.New(), keys)

	const rowCount = 80
	issues := make([]domain.IssueSummary, rowCount)
	for i := range issues {
		issues[i] = domain.IssueSummary{
			ID:    fmt.Sprintf("tm-r%d", i),
			Title: fmt.Sprintf("Ready issue %d", i),
		}
	}
	m.columns = []columnData{
		{title: sectionTitleReady, issues: issues, total: rowCount, exact: true},
	}
	m.focusedColumn = 0
	m.selectedRow[0] = 30
	m.scrollOffset[0] = 20 // window shows rows 20..41

	m.SetSize(120, 25) // capacity = 22

	view := m.View(0)

	// Header must show "N of 80" since window clips.
	if !strings.Contains(view, "of 80") {
		t.Errorf("expected 'of 80' in header when window clips, got:\n%s", view)
	}

	// Row tm-r20 should be visible (start of window).
	if !strings.Contains(view, "r20") {
		t.Errorf("expected tm-r20 to be visible at scroll offset 20, got:\n%s", view)
	}

	// Row tm-r0 should NOT be visible (before window).
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	if strings.Contains(plain, "Ready issue 0") {
		t.Errorf("expected row 0 to be hidden at scroll offset 20, got:\n%s", plain)
	}
}

// TestBoardModeScrollTeatestChevronVisible exercises the full EnsureVisible +
// renderer path end-to-end: Init triggers a real Dashboard load from an
// in-memory repo, then 30 j keypresses are applied synchronously via
// ApplyControllerKeySequence, and the final View output is asserted to contain
// the selection chevron (›) — proving the selected row is in the visible window.
func TestBoardModeScrollTeatestChevronVisible(t *testing.T) {
	t.Parallel()

	// Seed the memory repo with 80 ready issues so Init loads them.
	repo := memoryrepo.New()
	const rowCount = 80
	for i := 0; i < rowCount; i++ {
		repo.Seed(memoryrepo.Issue{
			ID:     fmt.Sprintf("tm-ready%02d", i),
			Title:  fmt.Sprintf("Ready issue %02d", i),
			Status: "open",
			Type:   "task",
		})
	}

	m := newBoardModel(repo, resolvedBoardKeys(t))
	m.SetSize(120, 25)

	// InitializeController runs Init and drains all resulting commands
	// synchronously. For the memory repo this resolves the Dashboard load
	// entirely before any keypress is applied.
	ctrl := testui.InitializeController(m)

	// Board must have loaded all 80 issues: IsLoading() must be false and at
	// least one Ready row visible.
	bm, ok := ctrl.(*Model)
	if !ok {
		t.Fatalf("expected *Model after InitializeController, got %T", ctrl)
	}
	if bm.IsLoading() {
		t.Fatal("board is still loading after InitializeController — memory repo must resolve synchronously")
	}

	// Apply a WindowSizeMsg so sectionItemCapacity is set.
	_ = bm.Update(tea.WindowSizeMsg{Width: 120, Height: 25})

	// Apply 30 j (down) keypresses synchronously.
	const steps = 30
	keys := make([]tea.KeyMsg, steps)
	for i := range keys {
		keys[i] = tea.KeyMsg{Type: tea.KeyDown}
	}
	final := testui.ApplyControllerKeySequence(bm, keys...)
	finalBoard, ok := final.(*Model)
	if !ok {
		t.Fatalf("expected *Model after ApplyControllerKeySequence, got %T", final)
	}

	// Selection index must equal exactly steps (board clamps at len-1, 30 < 80).
	sel := finalBoard.selectedRow[finalBoard.focusedColumn]
	if sel != steps {
		t.Errorf("expected selected row index %d after %d j presses, got %d", steps, steps, sel)
	}

	// Scroll offset must have advanced so the selection stays in the window.
	offset := finalBoard.ScrollOffsetForColumn(finalBoard.focusedColumn)
	capacity := finalBoard.sectionItemCapacity()
	if sel < offset || sel >= offset+capacity {
		t.Errorf("selection %d not in visible window [%d, %d)", sel, offset, offset+capacity)
	}
	if offset == 0 {
		t.Errorf("expected scroll offset > 0 after selection moved past viewport, got 0")
	}

	// The rendered view must contain the chevron character (›).
	view := finalBoard.View(0)
	plain := testui.AnsiEscapePattern.ReplaceAllString(view, "")
	if !strings.Contains(plain, "›") {
		t.Errorf("expected selection chevron '›' in rendered view after 30 j presses, got:\n%s", plain)
	}
}

// --- Done column load-more tests ---

// loadMoreCapture is a minimal stub repository that records all Dashboard opts
// and returns a configurable canned response for each call.
type loadMoreCapture struct {
	mu   sync.Mutex
	opts []repository.DashboardOptions
	resp repository.DashboardData // returned for every Dashboard call
}

func (r *loadMoreCapture) Dashboard(_ context.Context, opts repository.DashboardOptions) (repository.DashboardData, error) {
	r.mu.Lock()
	r.opts = append(r.opts, opts)
	r.mu.Unlock()
	return r.resp, nil
}

func (r *loadMoreCapture) capturedOpts() []repository.DashboardOptions {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]repository.DashboardOptions, len(r.opts))
	copy(out, r.opts)
	return out
}

// Remaining Repository methods are no-ops.
func (r *loadMoreCapture) Issue(_ context.Context, _ string) (domain.IssueDetail, error) {
	return domain.IssueDetail{}, nil
}
func (r *loadMoreCapture) Search(_ context.Context, _ domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	return domain.SearchResultPage{}, nil
}
func (r *loadMoreCapture) CreateIssue(_ context.Context, _ domain.CreateIssueInput) (domain.CreateIssueResult, error) {
	return domain.CreateIssueResult{}, nil
}
func (r *loadMoreCapture) UpdateIssue(_ context.Context, _ string, _ domain.UpdateIssueInput) error {
	return nil
}
func (r *loadMoreCapture) CloseIssue(_ context.Context, _ string, _ domain.CloseIssueInput) error {
	return nil
}
func (r *loadMoreCapture) AddComment(_ context.Context, _ string, _ domain.AddCommentInput) error {
	return nil
}
func (r *loadMoreCapture) HealthCheck(_ context.Context) error { return nil }
func (r *loadMoreCapture) Catalogs(_ context.Context) (repository.Catalogs, error) {
	return repository.Catalogs{}, nil
}

// makeClosedIssues returns n synthesised closed IssueSummary values.
func makeClosedIssues(n int) []domain.IssueSummary {
	issues := make([]domain.IssueSummary, n)
	for i := range issues {
		issues[i] = domain.IssueSummary{
			ID:    fmt.Sprintf("closed-%d", i),
			Title: fmt.Sprintf("Closed issue %d", i),
		}
	}
	return issues
}

// TestDoneLoadMore_DispatchesOnThreshold verifies that pressing j while the
// cursor in the Done column is within loadMoreThreshold rows of the loaded
// end dispatches exactly one loadMoreClosedCmd with the correct ClosedOffset
// and ClosedLimit.
//
// Setup: doneClosedTotal=736, 35 issues loaded, cursor at row 31 → remaining=4 < 5.
func TestDoneLoadMore_DispatchesOnThreshold(t *testing.T) {
	t.Parallel()

	stub := &loadMoreCapture{}
	m := newBoardModel(stub, resolvedBoardKeys(t))
	m.SetSize(120, 25) // sectionItemCapacity = 22; closedPageSize = max(44,50) = 50

	// Pre-populate the 4 fixed columns; Done has 35 loaded issues.
	const loaded = 35
	closedIssues := makeClosedIssues(loaded)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone, issues: closedIssues, total: 736, exact: false},
	}

	// Initialise load-more state as if compose() already ran.
	m.doneLoadedCount = loaded
	m.doneClosedTotal = 736

	// Focus Done column; place cursor at row 31 (remaining = 35-31 = 4 < 5).
	m.focusedColumn = doneColumnIndex
	m.selectedRow[doneColumnIndex] = 31

	// Press j: cursor moves to row 32 (remaining = 35-32 = 3 < 5 → still triggers).
	// The threshold check fires because remaining < loadMoreThreshold.
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("expected non-nil Cmd after j press near Done column end")
	}

	// Execute the cmd batch; exactly one Dashboard call must have been issued
	// (the load-more dispatch). The selectionChangedCmd returns a tea.Msg in
	// the same batch; only the Dashboard call touches the stub.
	//
	// Drain the cmd: if it is a BatchMsg, execute each sub-cmd.
	switch c := cmd().(type) {
	case tea.BatchMsg:
		for _, sub := range c {
			if sub != nil {
				_ = sub()
			}
		}
	default:
		// Single cmd already executed by cmd() above.
	}

	// After execution the in-flight guard must be set.
	if !m.doneLoadInFlight {
		t.Error("expected doneLoadInFlight=true after dispatching load-more")
	}

	opts := stub.capturedOpts()
	if len(opts) != 1 {
		t.Fatalf("expected exactly 1 Dashboard call from load-more dispatch, got %d: %v", len(opts), opts)
	}
	if opts[0].ClosedOffset != loaded {
		t.Errorf("expected ClosedOffset=%d, got %d", loaded, opts[0].ClosedOffset)
	}
	wantLimit := m.closedPageSize()
	if opts[0].ClosedLimit != wantLimit {
		t.Errorf("expected ClosedLimit=%d, got %d", wantLimit, opts[0].ClosedLimit)
	}
}

// TestDoneLoadMore_NoDispatchAtSliceEnd verifies that no load-more is dispatched
// when doneLoadedCount==doneClosedTotal (all pages already loaded).
func TestDoneLoadMore_NoDispatchAtSliceEnd(t *testing.T) {
	t.Parallel()

	stub := &loadMoreCapture{}
	m := newBoardModel(stub, resolvedBoardKeys(t))
	m.SetSize(120, 25)

	const total = 35
	closedIssues := makeClosedIssues(total)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone, issues: closedIssues, total: total, exact: true},
	}

	// All issues loaded: doneLoadedCount == doneClosedTotal.
	m.doneLoadedCount = total
	m.doneClosedTotal = total

	// Place cursor near the very end.
	m.focusedColumn = doneColumnIndex
	m.selectedRow[doneColumnIndex] = total - 2

	// Press j: cursor advances to total-1. No load-more should fire.
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Execute any cmd to drain sub-commands.
	if cmd != nil {
		switch c := cmd().(type) {
		case tea.BatchMsg:
			for _, sub := range c {
				if sub != nil {
					_ = sub()
				}
			}
		}
	}

	opts := stub.capturedOpts()
	if len(opts) != 0 {
		t.Fatalf("expected 0 Dashboard calls when all issues loaded, got %d: %v", len(opts), opts)
	}
	if m.doneLoadInFlight {
		t.Error("expected doneLoadInFlight=false when all issues are loaded")
	}
}

// TestDoneLoadMore_MergesIncomingPage verifies that feeding a
// loadMoreClosedDoneMsg with 50 new issues into a model that already has 35
// Done issues produces a merged Done column with 85 issues, doneLoadedCount=85,
// and doneLoadInFlight=false.
func TestDoneLoadMore_MergesIncomingPage(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.SetSize(120, 25)

	const priorCount = 35
	const incomingCount = 50

	priorIssues := makeClosedIssues(priorCount)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone, issues: priorIssues, total: 736, exact: false},
	}
	m.doneLoadedCount = priorCount
	m.doneClosedTotal = 736
	m.doneLoadInFlight = true // simulates the in-flight state before response arrives

	// Build the incoming page with IDs that don't overlap the prior slice.
	incomingIssues := make([]domain.IssueSummary, incomingCount)
	for i := range incomingIssues {
		incomingIssues[i] = domain.IssueSummary{
			ID:    fmt.Sprintf("incoming-%d", i),
			Title: fmt.Sprintf("Incoming closed %d", i),
		}
	}

	// Feed the load-more response.
	_ = m.Update(loadMoreClosedDoneMsg{
		data: repository.DashboardData{
			Closed:      incomingIssues,
			ClosedTotal: 736,
		},
		opts: repository.DashboardOptions{ClosedOffset: priorCount, ClosedLimit: 50},
	})

	// AC 1: Done column must have prior+incoming (no ID overlap → 35+50=85).
	wantCount := priorCount + incomingCount
	gotCount := len(m.columns[doneColumnIndex].issues)
	if gotCount != wantCount {
		t.Errorf("expected Done.Issues count=%d after merge, got %d", wantCount, gotCount)
	}

	// AC 2: doneLoadedCount must match the merged slice length.
	if m.doneLoadedCount != wantCount {
		t.Errorf("expected doneLoadedCount=%d, got %d", wantCount, m.doneLoadedCount)
	}

	// AC 3: in-flight guard must be cleared.
	if m.doneLoadInFlight {
		t.Error("expected doneLoadInFlight=false after load-more response processed")
	}
}

// TestDoneLoadMore_ExplicitKey verifies that pressing the > key while focused
// on the Done column dispatches a load-more even when the cursor is not near
// the end of the loaded slice.
func TestDoneLoadMore_ExplicitKey(t *testing.T) {
	t.Parallel()

	stub := &loadMoreCapture{}
	m := newBoardModel(stub, resolvedBoardKeys(t))
	m.SetSize(120, 25)

	const loaded = 35
	closedIssues := makeClosedIssues(loaded)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone, issues: closedIssues, total: 736, exact: false},
	}
	m.doneLoadedCount = loaded
	m.doneClosedTotal = 736

	// Focus Done at row 0 — cursor is far from the end (remaining=35 >> threshold=5).
	m.focusedColumn = doneColumnIndex
	m.selectedRow[doneColumnIndex] = 0

	// Press >: explicit load-more key.
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})
	if cmd == nil {
		t.Fatal("expected non-nil Cmd after > press on Done column")
	}

	// Execute the cmd.
	_ = cmd()

	opts := stub.capturedOpts()
	if len(opts) != 1 {
		t.Fatalf("expected exactly 1 Dashboard call from explicit > key, got %d: %v", len(opts), opts)
	}
	if opts[0].ClosedOffset != loaded {
		t.Errorf("expected ClosedOffset=%d, got %d", loaded, opts[0].ClosedOffset)
	}
	if !m.doneLoadInFlight {
		t.Error("expected doneLoadInFlight=true after explicit load-more dispatch")
	}
}

// --- Done column load-more reset tests ---

// TestDoneLoadMore_ManualReloadResetsToPage1 verifies that pressing r (manual
// reload) while doneLoadedCount=85 resets the state and dispatches a Dashboard
// call with ClosedOffset=0 and ClosedLimit=sectionItemCapacity(). After the
// resulting dashboard response arrives, doneLoadedCount reflects only the new
// page (not the stale 85).
//
// Audit note: the r key handler calls startReload(refreshModeReload) which
// already resets doneLoadedCount=0 and doneLoadInFlight=false (lines 383-384
// of model.go). This test is the explicit regression guard for that
// path.
func TestDoneLoadMore_ManualReloadResetsToPage1(t *testing.T) {
	t.Parallel()

	// Return a fresh page of 20 closed issues on reload so we can verify
	// doneLoadedCount is set from the new page, not the stale 85.
	const freshPageSize = 20
	freshClosed := makeClosedIssues(freshPageSize)
	stub := &loadMoreCapture{
		resp: repository.DashboardData{
			Closed:      freshClosed,
			ClosedTotal: 736,
		},
	}

	m := newBoardModel(stub, resolvedBoardKeys(t))
	// height=23 → sectionItemCapacity()=20.
	m.SetSize(120, 23)

	// Arrange: 85 Done issues loaded (as if two load-more pages have been fetched).
	const staleLoaded = 85
	staleClosed := makeClosedIssues(staleLoaded)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone, issues: staleClosed, total: 736, exact: false},
	}
	m.doneLoadedCount = staleLoaded
	m.doneClosedTotal = 736
	m.doneLoadInFlight = false
	m.focusedColumn = doneColumnIndex
	m.selectedRow[doneColumnIndex] = 50

	// AC: pressing r dispatches a reload command.
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Fatal("expected non-nil Cmd after r press (manual reload)")
	}

	// AC: doneLoadedCount must be reset to 0 immediately after the key is handled
	// (startReload resets before the cmd runs).
	if m.doneLoadedCount != 0 {
		t.Errorf("expected doneLoadedCount=0 immediately after r press, got %d", m.doneLoadedCount)
	}
	if m.doneLoadInFlight {
		t.Error("expected doneLoadInFlight=false immediately after r press (reset by startReload)")
	}

	// Execute the command to call stub.Dashboard and capture opts.
	msg := cmd()

	// The reload cmd returns a dashboardLoadedMsg. Feed it back into the model.
	loaded, ok := msg.(dashboardLoadedMsg)
	if !ok {
		t.Fatalf("expected dashboardLoadedMsg from reload cmd, got %T", msg)
	}

	opts := stub.capturedOpts()
	if len(opts) != 1 {
		t.Fatalf("expected exactly 1 Dashboard call from manual reload, got %d: %v", len(opts), opts)
	}

	// AC: ClosedOffset=0 (page-1 reset).
	if opts[0].ClosedOffset != 0 {
		t.Errorf("expected ClosedOffset=0 on reload, got %d", opts[0].ClosedOffset)
	}
	// AC: ClosedLimit=sectionItemCapacity() (not the load-more page size).
	wantLimit := m.sectionItemCapacity()
	if opts[0].ClosedLimit != wantLimit {
		t.Errorf("expected ClosedLimit=%d on reload, got %d", wantLimit, opts[0].ClosedLimit)
	}

	// Feed the dashboard result back so compose() runs and sets doneLoadedCount.
	_ = m.Update(loaded)

	// AC: doneLoadedCount must equal the new page size (freshPageSize), not staleLoaded.
	if m.doneLoadedCount != freshPageSize {
		t.Errorf("expected doneLoadedCount=%d after reload response, got %d", freshPageSize, m.doneLoadedCount)
	}
}

// TestDoneLoadMore_FocusRegainResetsToPage1 verifies that the focus-regain
// auto-refresh path resets Done pagination to page 1.
//
// Architecture note: focus-regain is handled in internal/app/model.go
// (tea.FocusMsg → maybeAutoRefreshActiveSurfaceCmdOnFocusRegain →
// refreshActiveSurfaceCmd → m.board.AutoRefresh). AutoRefresh() calls
// startReload(refreshModeAuto), which resets doneLoadedCount and
// doneLoadInFlight via the shared counter-reset block in startReload (lines
// 383-384 of model.go). This test covers the board.AutoRefresh() entry point
// directly — the app-level wiring is covered by existing app model tests; the
// board-level counter reset is what we pin here.
func TestDoneLoadMore_FocusRegainResetsToPage1(t *testing.T) {
	t.Parallel()

	const freshPageSize = 20
	freshClosed := makeClosedIssues(freshPageSize)
	stub := &loadMoreCapture{
		resp: repository.DashboardData{
			Closed:      freshClosed,
			ClosedTotal: 736,
		},
	}

	m := newBoardModel(stub, resolvedBoardKeys(t))
	// height=23 → sectionItemCapacity()=20.
	m.SetSize(120, 23)

	// Arrange: 85 Done issues loaded (deep into pagination).
	const staleLoaded = 85
	staleClosed := makeClosedIssues(staleLoaded)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone, issues: staleClosed, total: 736, exact: false},
	}
	m.doneLoadedCount = staleLoaded
	m.doneClosedTotal = 736
	m.doneLoadInFlight = false

	// Simulate focus-regain auto-refresh by calling AutoRefresh() directly —
	// this is the same method the app shell calls from refreshActiveSurfaceCmd.
	cmd := m.AutoRefresh()
	if cmd == nil {
		t.Fatal("expected non-nil Cmd from AutoRefresh() (focus-regain path)")
	}

	// AC: counters reset immediately when AutoRefresh/startReload runs.
	if m.doneLoadedCount != 0 {
		t.Errorf("expected doneLoadedCount=0 after AutoRefresh, got %d", m.doneLoadedCount)
	}
	if m.doneLoadInFlight {
		t.Error("expected doneLoadInFlight=false after AutoRefresh reset")
	}

	// Execute cmd to capture opts.
	msg := cmd()

	opts := stub.capturedOpts()
	if len(opts) != 1 {
		t.Fatalf("expected exactly 1 Dashboard call from AutoRefresh, got %d: %v", len(opts), opts)
	}

	// AC: ClosedOffset=0 (page-1 reset).
	if opts[0].ClosedOffset != 0 {
		t.Errorf("expected ClosedOffset=0 on focus-regain reload, got %d", opts[0].ClosedOffset)
	}
	// AC: ClosedLimit=sectionItemCapacity().
	wantLimit := m.sectionItemCapacity()
	if opts[0].ClosedLimit != wantLimit {
		t.Errorf("expected ClosedLimit=%d on focus-regain reload, got %d", wantLimit, opts[0].ClosedLimit)
	}

	// Feed the dashboard result so compose() runs.
	loaded, ok := msg.(dashboardLoadedMsg)
	if !ok {
		t.Fatalf("expected dashboardLoadedMsg from AutoRefresh cmd, got %T", msg)
	}
	_ = m.Update(loaded)

	// AC: doneLoadedCount set from fresh page.
	if m.doneLoadedCount != freshPageSize {
		t.Errorf("expected doneLoadedCount=%d after focus-regain reload response, got %d", freshPageSize, m.doneLoadedCount)
	}
}

// TestDoneLoadMore_ReloadResetsState verifies that a manual reload (r key)
// resets doneLoadedCount and doneLoadInFlight to zero and dispatches with
// ClosedOffset=0 — the page-1 reload contract.
//
// This test is complementary to TestDoneLoadMore_ManualReloadResetsToPage1 and
// focuses specifically on the reset interaction rather than the post-compose
// doneLoadedCount value.
func TestDoneLoadMore_ReloadResetsState(t *testing.T) {
	t.Parallel()

	stub := &loadMoreCapture{}
	m := newBoardModel(stub, resolvedBoardKeys(t))
	m.SetSize(120, 25) // sectionItemCapacity=22

	// Arrange: model has fetched two load-more pages and has a stale in-flight.
	m.doneLoadedCount = 120
	m.doneClosedTotal = 500
	m.doneLoadInFlight = true // leave stale in-flight to confirm reset

	// Trigger manual reload directly (same code path as r key handler).
	// We bypass the inflight guard by resetting it first — the r key handler
	// itself guards on m.inflight (not doneLoadInFlight), so this matches the
	// real code path where doneLoadInFlight is left over from a prior session.
	m.inflight = false
	cmd := m.startReload(refreshModeReload)
	if cmd == nil {
		t.Fatal("expected non-nil Cmd from startReload(refreshModeReload)")
	}

	// AC 1: doneLoadedCount reset to 0 synchronously.
	if m.doneLoadedCount != 0 {
		t.Errorf("expected doneLoadedCount=0 after startReload(Reload), got %d", m.doneLoadedCount)
	}

	// AC 2: doneLoadInFlight cleared (stale flag reset).
	if m.doneLoadInFlight {
		t.Error("expected doneLoadInFlight=false after startReload(Reload) reset")
	}

	// Execute cmd and capture opts.
	_ = cmd()

	opts := stub.capturedOpts()
	if len(opts) != 1 {
		t.Fatalf("expected exactly 1 Dashboard call, got %d: %v", len(opts), opts)
	}

	// AC 3: ClosedOffset=0 (no leftover offset from previous load-more pages).
	if opts[0].ClosedOffset != 0 {
		t.Errorf("expected ClosedOffset=0 (page-1 reset), got %d", opts[0].ClosedOffset)
	}
}

// TestDoneLoadMore_EmptyDoneColumnNoDispatch is the regression test for the
// empty-Done load-more guard (FIX #14). When the Done column is empty after a
// Dashboard load (ClosedTotal==0, no closed issues), triggering load-more while
// focused on Done must NOT dispatch a wasted backend Dashboard fetch.
//
// The previous all-loaded guard read `loaded >= total && total > 0`, so the
// empty case (loaded==0, total==0) failed the `total > 0` clause, fell through,
// and dispatched a fresh Dashboard call on every cursor move / load-more
// keypress. With that clause removed, `loaded(0) >= total(0)` short-circuits and
// returns nil. Reverting the fix makes this test fail: dispatchLoadMoreClosed
// would return a non-nil Cmd that calls repo.Dashboard.
func TestDoneLoadMore_EmptyDoneColumnNoDispatch(t *testing.T) {
	t.Parallel()

	stub := &loadMoreCapture{}
	m := newBoardModel(stub, resolvedBoardKeys(t))
	m.SetSize(120, 25)

	// Simulate a completed Dashboard load with no closed issues. compose() sets
	// doneLoadedCount=0 and doneClosedTotal=0 from this empty result. Feeding the
	// dashboardLoadedMsg directly does not touch the repo stub, so capturedOpts
	// stays empty until (and unless) a load-more is dispatched.
	feedDashboardData(m, repository.DashboardData{})

	if m.doneClosedTotal != 0 {
		t.Fatalf("precondition: expected doneClosedTotal=0 for empty Done, got %d", m.doneClosedTotal)
	}
	if m.doneLoadedCount != 0 {
		t.Fatalf("precondition: expected doneLoadedCount=0 for empty Done, got %d", m.doneLoadedCount)
	}

	// Focus the (empty) Done column and trigger an explicit load-more (>).
	m.focusedColumn = doneColumnIndex
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})

	// FIX #14: empty Done must short-circuit — no Cmd, no in-flight flag.
	if cmd != nil {
		// Drain in case the buggy code returned a Cmd (single or batch) so any
		// leaked Dashboard call is recorded before we assert on capturedOpts.
		if c, ok := cmd().(tea.BatchMsg); ok {
			for _, sub := range c {
				if sub != nil {
					_ = sub()
				}
			}
		}
		t.Error("expected nil Cmd for empty Done column load-more, got non-nil")
	}
	if m.doneLoadInFlight {
		t.Error("expected doneLoadInFlight=false for empty Done column (no dispatch)")
	}

	// No Dashboard fetch may have been dispatched for the wasted load-more.
	if opts := stub.capturedOpts(); len(opts) != 0 {
		t.Fatalf("expected 0 Dashboard calls for empty Done load-more, got %d: %v", len(opts), opts)
	}
}

// TestDoneLoadMore_MergeReSyncsSelectionWhenDoneFocused is the regression test
// for the post-merge selection re-sync (FIX #7). After a successful load-more
// replaces the Done column, applyLoadMoreClosed must re-emit a
// SelectionChangedMsg (when the Done column is focused) so the shell's stored
// selection, header, and any open detail pane stay in sync with the highlighted
// row rather than referencing a stale issue.
//
// Previously applyLoadMoreClosed returned nil after merging, desyncing the
// selection from the highlighted row. Reverting the fix makes this test fail at
// the non-nil Cmd assertion.
func TestDoneLoadMore_MergeReSyncsSelectionWhenDoneFocused(t *testing.T) {
	t.Parallel()

	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.SetSize(120, 25)

	const priorCount = 35
	const incomingCount = 50
	priorIssues := makeClosedIssues(priorCount)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone, issues: priorIssues, total: 736, exact: false},
	}
	m.doneLoadedCount = priorCount
	m.doneClosedTotal = 736
	m.doneLoadInFlight = true // simulates the in-flight state before the response

	// Done column focused, cursor parked on a prior issue (row 5 → "closed-5").
	m.focusedColumn = doneColumnIndex
	m.selectedRow[doneColumnIndex] = 5

	// Incoming continuation page with non-overlapping IDs.
	incomingIssues := make([]domain.IssueSummary, incomingCount)
	for i := range incomingIssues {
		incomingIssues[i] = domain.IssueSummary{
			ID:    fmt.Sprintf("incoming-%d", i),
			Title: fmt.Sprintf("Incoming closed %d", i),
		}
	}

	cmd := m.Update(loadMoreClosedDoneMsg{
		data: repository.DashboardData{Closed: incomingIssues, ClosedTotal: 736},
		opts: repository.DashboardOptions{ClosedOffset: priorCount, ClosedLimit: 50},
	})

	// FIX #7: a non-nil re-sync Cmd must be returned when Done is focused.
	if cmd == nil {
		t.Fatal("expected non-nil re-sync Cmd from applyLoadMoreClosed when Done is focused")
	}

	// The Cmd must produce a board SelectionChangedMsg reflecting the issue at
	// the highlighted row in the merged Done column.
	msg := cmd()
	selChanged, ok := msg.(mode.SelectionChangedMsg)
	if !ok {
		t.Fatalf("expected mode.SelectionChangedMsg from re-sync Cmd, got %T", msg)
	}
	if selChanged.Mode != mode.Board {
		t.Errorf("expected SelectionChangedMsg.Mode=%v, got %v", mode.Board, selChanged.Mode)
	}
	if selChanged.Selection == nil {
		t.Fatal("expected non-nil Selection in re-sync SelectionChangedMsg")
	}

	// The emitted selection must match the issue under the cursor in the merged
	// Done column — proving selection re-syncs to the highlighted row.
	row := m.selectedRow[doneColumnIndex]
	wantID := m.columns[doneColumnIndex].issues[row].ID
	if selChanged.Selection.Issue.ID != wantID {
		t.Errorf("re-sync selection ID = %q, want %q (issue at selected row %d)",
			selChanged.Selection.Issue.ID, wantID, row)
	}
	// Sanity: the merge preserves prior order, so row 5 is still "closed-5".
	if wantID != "closed-5" {
		t.Errorf("expected merged row 5 to be %q, got %q", "closed-5", wantID)
	}
}

// TestMoveRow_ErrorColumnReservesPrefixRowInScrollWindow is the model-side
// regression guard for FIX #6. When the focused column shows an inline error
// row, the renderer pins that row at the top and shows one fewer issue row, so
// moveRow must reserve it in the scroll-window size (sectionItemCapacity()-1).
// Without that reservation the selected bottom row lands at offset+capacity-1 —
// exactly the row the renderer drops — and clips off-screen.
func TestMoveRow_ErrorColumnReservesPrefixRowInScrollWindow(t *testing.T) {
	m := newBoardModel(memoryrepo.New(), resolvedBoardKeys(t))
	m.SetSize(40, 13) // sectionItemCapacity() == 13-3 == 10

	const n = 20
	issues := make([]domain.IssueSummary, n)
	for i := range issues {
		issues[i] = domain.IssueSummary{ID: fmt.Sprintf("c%d", i), Title: fmt.Sprintf("closed %d", i), Status: "closed"}
	}
	// Focused Done column carrying an inline error alongside loaded rows
	// (the failed-load-more shape that keeps its issues).
	m.columns[doneColumnIndex] = columnData{title: sectionTitleDone, issues: issues, total: n, exact: true, err: errors.New("load failed")}
	m.focusedColumn = doneColumnIndex
	m.selectedRow[doneColumnIndex] = 0
	m.scrollOffset[doneColumnIndex] = 0

	for i := 0; i < n-1; i++ {
		m.moveRow(1)
	}

	idx := m.selectedRow[doneColumnIndex]
	off := m.scrollOffset[doneColumnIndex]
	capacity := m.sectionItemCapacity()
	// The renderer's visible issue window with an error prefix is
	// [off, off+capacity-1); the selected row must fall inside it.
	if idx-off > capacity-2 {
		t.Errorf("selected row clips below the error-reserved window: idx=%d offset=%d capacity=%d (idx-off=%d must be <= %d)",
			idx, off, capacity, idx-off, capacity-2)
	}
	if idx != n-1 {
		t.Fatalf("expected selection at last row %d, got %d", n-1, idx)
	}
	if off == 0 {
		t.Fatalf("expected scroll offset to advance for a clipped column, got 0")
	}
}
