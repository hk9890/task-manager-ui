package docs

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// docsRepo bundles the memory repo (for seeding) with the error-injecting
// wrapper (for call tracking and error injection), mirroring the search-mode
// harness.
type docsRepo struct {
	repo *memoryrepo.Repository
	*fakes.ErrorInjectingRepository
}

func newDocsRepo() *docsRepo {
	repo := memoryrepo.New()
	return &docsRepo{
		repo:                     repo,
		ErrorInjectingRepository: fakes.NewErrorInjecting(repo),
	}
}

// seedMixed seeds two docs and one task so every assertion also proves the
// type filter is doing its job.
func seedMixed(gw *docsRepo) {
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Auth redesign", Status: "open", Type: "doc", Priority: 2})
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Session notes", Status: "closed", Type: "doc", Priority: 2})
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-3", Title: "Wire the tab strip", Status: "open", Type: "task", Priority: 1})
}

func newModel(t *testing.T, gw *docsRepo) *Model {
	t.Helper()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	m := NewModel(context.Background(), gw, nil, keys)
	m.SetSize(120, 30)
	return m
}

// resolve runs cmd and feeds the resulting message back into the model,
// returning whatever the handler dispatched next.
func resolve(t *testing.T, m *Model, cmd tea.Cmd) tea.Cmd {
	t.Helper()

	if cmd == nil {
		t.Fatal("expected a command to resolve, got nil")
	}
	return m.Update(cmd())
}

func loadedModel(t *testing.T, gw *docsRepo) *Model {
	t.Helper()

	m := newModel(t, gw)
	resolve(t, m, m.Init())
	return m
}

func TestDocsModeListsOnlyDocsIncludingOpenAndClosed(t *testing.T) {
	gw := newDocsRepo()
	seedMixed(gw)

	m := loadedModel(t, gw)

	if m.IsLoading() {
		t.Fatal("expected loading to clear once the page lands")
	}
	if len(m.issues) != 2 {
		t.Fatalf("expected 2 docs, got %d: %#v", len(m.issues), m.issues)
	}
	for _, issue := range m.issues {
		if issue.Type != "doc" {
			t.Fatalf("expected only doc-type issues, got %#v", issue)
		}
	}
	if m.total != 2 {
		t.Fatalf("expected total 2, got %d", m.total)
	}

	view := m.View(0)
	// An open doc is the case the board cannot show at all: assert it renders
	// here, next to the closed one.
	testui.AssertContainsAll(t, view, "Docs", "tm-1", "Auth redesign", "tm-2", "Session notes")
	testui.AssertNotContainsAny(t, view, "tm-3", "Wire the tab strip")
}

// queryCaptureRepo records the SearchIssuesQuery each Search call receives.
// fakes.Call records the method but not its arguments, so the query contract —
// a type filter, not a text query — needs its own stub.
type queryCaptureRepo struct {
	repository.Repository
	queries []domain.SearchIssuesQuery
}

func (r *queryCaptureRepo) Search(_ context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error) {
	r.queries = append(r.queries, query)
	return domain.SearchResultPage{}, nil
}

func TestDocsModeQueriesTheRepositoryWithTheDocTypeFilter(t *testing.T) {
	repo := &queryCaptureRepo{}
	m := NewModel(context.Background(), repo, nil)
	m.SetSize(120, 30)

	resolve(t, m, m.Init())

	if len(repo.queries) != 1 {
		t.Fatalf("expected exactly one Search call, got %d", len(repo.queries))
	}
	query := repo.queries[0]
	if len(query.Types) != 1 || query.Types[0] != "doc" {
		t.Fatalf("expected a doc type filter, got %#v", query.Types)
	}
	if query.Text != "" {
		t.Fatalf("expected no text filter, got %q", query.Text)
	}
	if query.Limit != 0 {
		t.Fatalf("expected an unlimited page (Limit 0), got %d", query.Limit)
	}
}

func TestDocsModeMovementEmitsSelectionAndEnterOpensDetail(t *testing.T) {
	gw := newDocsRepo()
	seedMixed(gw)

	m := loadedModel(t, gw)

	first := m.currentSelection()
	if first == nil || first.Issue.ID != "tm-1" {
		t.Fatalf("expected initial selection tm-1, got %#v", first)
	}

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("expected a selection-changed command after moving down")
	}
	selection, ok := cmd().(mode.SelectionChangedMsg)
	if !ok {
		t.Fatalf("expected SelectionChangedMsg, got %T", cmd())
	}
	if selection.Mode != mode.Docs {
		t.Fatalf("expected selection tagged with mode.Docs, got %s", selection.Mode)
	}
	if selection.Selection == nil || selection.Selection.Issue.ID != "tm-2" {
		t.Fatalf("expected selection tm-2 after moving down, got %#v", selection.Selection)
	}

	// The bottom row is the last row: moving down again is a no-op, so no
	// redundant selection message is emitted.
	if cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown}); cmd != nil {
		t.Fatalf("expected no command when the cursor is already on the last row, got %#v", cmd())
	}

	cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an action-request command on enter")
	}
	testui.AssertActionRequest(t, cmd(), mode.Docs, mode.ActionOpenDetail)
}

func TestDocsModeAutoRefreshKeepsTheSelectedDocUnderTheCursor(t *testing.T) {
	gw := newDocsRepo()
	seedMixed(gw)

	m := loadedModel(t, gw)
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-2" {
		t.Fatalf("expected selection tm-2 before refresh, got %#v", got)
	}

	// A doc sorting ahead of the selected one arrives between refreshes: the
	// cursor must follow tm-2 rather than stay on row 1.
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-0", Title: "Handover", Status: "open", Type: "doc", Priority: 2})
	resolve(t, m, m.AutoRefresh())

	if len(m.issues) != 3 {
		t.Fatalf("expected 3 docs after refresh, got %d", len(m.issues))
	}
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-2" {
		t.Fatalf("expected the cursor to stay on tm-2 across the refresh, got %#v", got)
	}
}

func TestDocsModeReloadKeyResetsTheCursorToTheTop(t *testing.T) {
	gw := newDocsRepo()
	seedMixed(gw)

	m := loadedModel(t, gw)
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	resolve(t, m, m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")}))
	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-1" {
		t.Fatalf("expected manual reload to reset the cursor to tm-1, got %#v", got)
	}
}

func TestDocsModeShowsTheRepositoryErrorAndKeepsStaleRows(t *testing.T) {
	gw := newDocsRepo()
	seedMixed(gw)

	m := loadedModel(t, gw)

	gw.SetError(fakes.MethodSearch, errors.New("search boom"))
	resolve(t, m, m.AutoRefresh())

	if m.err == nil {
		t.Fatal("expected the load error to be retained on the column")
	}
	view := m.View(0)
	if !strings.Contains(view, "search boom") {
		t.Fatalf("expected the error to render in the column, got:\n%s", view)
	}
	// Stale rows stay on screen so the surface does not blank out on a failed
	// background refresh.
	testui.AssertContainsAll(t, view, "tm-1", "Auth redesign")
}

func TestDocsModeEmptyStateHasNoSelection(t *testing.T) {
	gw := newDocsRepo()
	gw.repo.Seed(memoryrepo.Issue{ID: "tm-3", Title: "Wire the tab strip", Status: "open", Type: "task", Priority: 1})

	m := loadedModel(t, gw)

	if got := m.currentSelection(); got != nil {
		t.Fatalf("expected no selection when there are no docs, got %#v", got)
	}
	if cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatalf("expected enter to be inert with no docs, got %#v", cmd())
	}

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("expected movement to be inert with no docs, got %#v", cmd())
	}
}

func TestDocsModeIsLazyUntilInit(t *testing.T) {
	gw := newDocsRepo()
	seedMixed(gw)

	m := newModel(t, gw)
	// Before Init the shell must not report the surface as loading: docs mode
	// is initialised on the first switch into the tab, like search.
	if m.IsLoading() {
		t.Fatal("expected a freshly constructed docs model not to report loading")
	}
	if len(gw.Calls()) != 0 {
		t.Fatalf("expected no repository calls before Init, got %#v", gw.Calls())
	}

	cmd := m.Init()
	if !m.IsLoading() {
		t.Fatal("expected Init to mark the surface loading")
	}
	resolve(t, m, cmd)
}

func TestDocsModeSelectionSurvivesADocDisappearing(t *testing.T) {
	gw := newDocsRepo()
	seedMixed(gw)

	m := loadedModel(t, gw)
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// tm-2 stops being a doc, so it leaves this surface: the cursor must clamp
	// into range rather than point past the end of the slice.
	taskType := "task"
	if err := gw.UpdateIssue(context.Background(), "tm-2", domain.UpdateIssueInput{Type: &taskType}); err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}
	resolve(t, m, m.AutoRefresh())

	if got := m.currentSelection(); got == nil || got.Issue.ID != "tm-1" {
		t.Fatalf("expected the cursor to clamp onto tm-1, got %#v", got)
	}
}

// TestItemCapacityReservesOneRowForAnInlineError is the docs twin of the
// board's TestMoveRow_ErrorColumnReservesPrefixRowInScrollWindow. The renderer
// pins an inline error row above the issue rows, so the scroll window must lose
// exactly one row while an error is shown — and must lose it only then.
//
// Reserving unconditionally scrolls the docs viewport one row early on every
// render; dropping the reservation clips the selected row off-screen while an
// error banner is up. Both pass every other assertion in this package.
func TestItemCapacityReservesOneRowForAnInlineError(t *testing.T) {
	t.Parallel()

	gw := newDocsRepo()
	seedMixed(gw)

	cases := []struct {
		name   string
		height int
		// wantWithoutErr is the row window with no error shown; the window with
		// an error must be exactly one smaller, until the floor of one row.
		wantWithoutErr int
		wantWithErr    int
	}{
		{"roomy column", 30, 27, 26},
		{"two rows leaves one for the error", 5, 2, 1},
		{"single row cannot reserve and keeps its row", 4, 1, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := newModel(t, gw)
			m.SetSize(120, tc.height)

			m.err = nil
			if got := m.itemCapacity(); got != tc.wantWithoutErr {
				t.Errorf("itemCapacity with no error = %d, want %d", got, tc.wantWithoutErr)
			}

			m.err = errors.New("load failed")
			if got := m.itemCapacity(); got != tc.wantWithErr {
				t.Errorf("itemCapacity with an inline error = %d, want %d", got, tc.wantWithErr)
			}
		})
	}
}
