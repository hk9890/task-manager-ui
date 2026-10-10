package search

import (
	"context"
	"errors"
	"fmt"
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

// seedStore seeds three open issues and one closed. "login" is in the title of
// tm-1 and tm-4 and only in the description of tm-3.
func seedStore(gw *fakes.TrackedRepository) {
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Fix login prompt", Status: "open", Type: "bug", Priority: 1})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Triage inbox", Status: "open", Type: "task", Priority: 2})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-3", Title: "Session notes", Description: "what the login audit found", Status: "open", Type: "doc", Priority: 2})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-4", Title: "Old login page", Status: "closed", Type: "task", Priority: 3})
}

func newModel(t *testing.T, repo repository.Repository) *Model {
	t.Helper()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	m := NewModel(context.Background(), repo, nil, keys)
	m.SetSize(100, 24)
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

func openedModel(t *testing.T, gw *fakes.TrackedRepository) *Model {
	t.Helper()

	m := newModel(t, gw)
	resolve(t, m, m.Init())
	return m
}

func typeText(m *Model, text string) tea.Cmd {
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
}

func resultIDs(m *Model) string {
	ids := make([]string, len(m.issues))
	for i, issue := range m.issues {
		ids[i] = issue.ID
	}
	return strings.Join(ids, ",")
}

func plainView(m *Model) string {
	return testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
}

// lastSearch is the query of the latest Search call the repository received.
func lastSearch(t *testing.T, gw *fakes.TrackedRepository) domain.SearchIssuesQuery {
	t.Helper()
	calls := gw.CallsFor(fakes.MethodSearch)
	if len(calls) == 0 {
		t.Fatal("the repository received no Search call")
	}
	return calls[len(calls)-1].Args.(domain.SearchIssuesQuery)
}

func selection(t *testing.T, cmd tea.Cmd) *mode.Selection {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a selection change, got no command")
	}
	msg, ok := cmd().(mode.SelectionChangedMsg)
	if !ok || msg.Mode != mode.Search {
		t.Fatalf("expected a search selection change, got %#v", msg)
	}
	return msg.Selection
}

func TestOpeningWithAnEmptyQueryListsTheOpenIssues(t *testing.T) {
	gw := fakes.NewTracked()
	seedStore(gw)

	m := newModel(t, gw)
	init := m.Init()
	if !m.IsLoading() {
		t.Fatal("the first search is not reported as loading")
	}
	if got := selection(t, resolve(t, m, init)); got == nil || got.Issue.ID != "tm-1" {
		t.Fatalf("the first result is not announced as the selection: %#v", got)
	}

	if m.IsLoading() {
		t.Fatal("loading did not clear once the page landed")
	}
	if got := lastSearch(t, gw); got.Text != "" || got.IncludeClosed || got.Limit != resultLimit {
		t.Fatalf("the opening search asked for %#v", got)
	}
	if got := resultIDs(m); got != "tm-1,tm-2,tm-3" {
		t.Fatalf("the opening search lists %q, want the open issues", got)
	}
	view := plainView(m)
	for _, want := range []string{"❯ search the store", "─ Results · open ", " 3 ─", "Fix login prompt"} {
		if !strings.Contains(view, want) {
			t.Errorf("the surface does not draw %q:\n%s", want, view)
		}
	}
}

func TestEveryEditSearchesTheStoreForTheQuery(t *testing.T) {
	gw := fakes.NewTracked()
	seedStore(gw)
	m := openedModel(t, gw)

	before := len(gw.CallsFor(fakes.MethodSearch))
	resolve(t, m, typeText(m, "log"))
	resolve(t, m, typeText(m, "in"))
	if got := len(gw.CallsFor(fakes.MethodSearch)) - before; got != 2 {
		t.Fatalf("two edits ran %d searches", got)
	}
	if got := lastSearch(t, gw); got.Text != "login" || got.IncludeClosed || got.Limit != resultLimit {
		t.Fatalf("the search asked for %#v, want the text login in open issues", got)
	}
	// tm-3 holds the word in its description only.
	if got := resultIDs(m); got != "tm-1,tm-3" {
		t.Fatalf("the results are %q, want tm-1,tm-3", got)
	}
	view := plainView(m)
	for _, want := range []string{"❯ login", " 2 ─", "Session notes"} {
		if !strings.Contains(view, want) {
			t.Errorf("the surface does not draw %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, " of ") {
		t.Errorf("a complete result is counted against something:\n%s", view)
	}

	resolve(t, m, m.Update(tea.KeyMsg{Type: tea.KeyBackspace}))
	if got := lastSearch(t, gw); got.Text != "logi" {
		t.Fatalf("backspace searched for %q, want logi", got.Text)
	}
	resolve(t, m, m.Update(tea.KeyMsg{Type: tea.KeyCtrlU}))
	if got := lastSearch(t, gw); got.Text != "" {
		t.Fatalf("ctrl+u searched for %q, want the empty query", got.Text)
	}

	// A key that leaves the text as it is searches nothing.
	before = len(gw.CallsFor(fakes.MethodSearch))
	if cmd := m.Update(tea.KeyMsg{Type: tea.KeyBackspace}); cmd != nil {
		t.Fatal("backspace on an empty query returned a command")
	}
	if got := len(gw.CallsFor(fakes.MethodSearch)); got != before {
		t.Fatal("backspace on an empty query ran a search")
	}
}

func TestMatchedWordsAreMarkedInTitleAndIDOnly(t *testing.T) {
	testui.ForceTrueColor(t)
	gw := fakes.NewTracked()
	seedStore(gw)
	m := openedModel(t, gw)
	resolve(t, m, typeText(m, "login"))

	lines := strings.Split(m.View(0), "\n")
	_, titleRow := testui.FindCell(t, m.View(0), "Fix login prompt")
	_, descriptionRow := testui.FindCell(t, m.View(0), "Session notes")
	// A marked word is drawn apart from the text around it.
	if strings.Contains(lines[titleRow], "Fix login prompt") {
		t.Errorf("the matched word is not marked in the title:\n%q", lines[titleRow])
	}
	if !strings.Contains(lines[descriptionRow], "Session notes") {
		t.Errorf("a row found through its description carries a mark:\n%q", lines[descriptionRow])
	}
}

func TestScopeKeyTogglesBetweenOpenAndAllAndSearchesAgain(t *testing.T) {
	gw := fakes.NewTracked()
	seedStore(gw)
	m := openedModel(t, gw)
	resolve(t, m, typeText(m, "login"))

	resolve(t, m, m.Update(tea.KeyMsg{Type: tea.KeyCtrlT}))
	if got := lastSearch(t, gw); got.Text != "login" || !got.IncludeClosed {
		t.Fatalf("ctrl+t searched for %#v, want login in all issues", got)
	}
	if got := resultIDs(m); got != "tm-1,tm-3,tm-4" {
		t.Fatalf("the results are %q, want the closed tm-4 too", got)
	}
	if view := plainView(m); !strings.Contains(view, "─ Results · all ") {
		t.Fatalf("the column title does not name the scope:\n%s", view)
	}

	resolve(t, m, m.Update(tea.KeyMsg{Type: tea.KeyCtrlT}))
	if got := lastSearch(t, gw); got.IncludeClosed {
		t.Fatal("a second ctrl+t did not return to open issues")
	}
	if view := plainView(m); !strings.Contains(view, "─ Results · open ") {
		t.Fatalf("the column title does not name the scope:\n%s", view)
	}
}

func TestHeaderCountsThePageAgainstEveryMatchWhenTheStoreHoldsMore(t *testing.T) {
	gw := fakes.NewTracked()
	for i := 0; i < resultLimit+5; i++ {
		gw.Memory.Seed(memoryrepo.Issue{ID: fmt.Sprintf("tm-%03d", i), Title: "Issue", Status: "open", Type: "task"})
	}
	m := openedModel(t, gw)

	if len(m.issues) != resultLimit {
		t.Fatalf("the page holds %d results, want %d", len(m.issues), resultLimit)
	}
	if view := plainView(m); !strings.Contains(view, fmt.Sprintf(" %d of %d ─", resultLimit, resultLimit+5)) {
		t.Fatalf("the header does not count the page against every match:\n%s", view)
	}
}

func TestMovementOpenDetailAndReloadReadTheBoardContext(t *testing.T) {
	gw := fakes.NewTracked()
	seedStore(gw)
	m := openedModel(t, gw)

	if got := selection(t, m.Update(tea.KeyMsg{Type: tea.KeyDown})); got.Issue.ID != "tm-2" {
		t.Fatalf("down selected %q, want tm-2", got.Issue.ID)
	}
	if got := selection(t, m.Update(tea.KeyMsg{Type: tea.KeyEnd})); got.Issue.ID != "tm-3" {
		t.Fatalf("end selected %q, want tm-3", got.Issue.ID)
	}
	if cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown}); cmd != nil {
		t.Fatal("down on the last result returned a command")
	}
	testui.AssertActionRequest(t, m.Update(tea.KeyMsg{Type: tea.KeyEnter})(), mode.Search, mode.ActionOpenDetail)

	// The reload key runs the query again and returns to the first result.
	before := len(gw.CallsFor(fakes.MethodSearch))
	reload := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r"), Alt: true})
	if cmd := m.Reload(); cmd != nil {
		t.Fatal("a reload during a search in flight was not dropped")
	}
	if got := selection(t, resolve(t, m, reload)); got.Issue.ID != "tm-1" {
		t.Fatalf("reload left the selection on %q, want the first result", got.Issue.ID)
	}
	if got := len(gw.CallsFor(fakes.MethodSearch)) - before; got != 1 {
		t.Fatalf("reload ran %d searches, want 1", got)
	}
}

func TestEnterWithoutAResultOpensNothing(t *testing.T) {
	m := openedModel(t, fakes.NewTracked())
	if cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("enter on an empty result list returned a command")
	}
}

func TestAutoRefreshRunsTheQueryAgainAndKeepsTheSelectedIssue(t *testing.T) {
	gw := fakes.NewTracked()
	seedStore(gw)
	m := openedModel(t, gw)
	resolve(t, m, typeText(m, "login"))
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.selectedIssueID(); got != "tm-3" {
		t.Fatalf("setup: selected %q, want tm-3", got)
	}

	// A new match sorts ahead of the selected issue.
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-0", Title: "Login banner", Status: "open", Type: "task"})
	refresh := m.AutoRefresh()
	if m.AutoRefresh() != nil {
		t.Fatal("an auto refresh during a search in flight was not dropped")
	}
	resolve(t, m, refresh)

	if got := lastSearch(t, gw); got.Text != "login" {
		t.Fatalf("the refresh searched for %q, want the query login", got.Text)
	}
	if got := resultIDs(m); got != "tm-0,tm-1,tm-3" {
		t.Fatalf("the refreshed results are %q", got)
	}
	if got := m.selectedIssueID(); got != "tm-3" {
		t.Fatalf("the refresh moved the selection to %q, want tm-3", got)
	}
}

func TestClearQueryEmptiesTheQueryAndSearchesAgain(t *testing.T) {
	gw := fakes.NewTracked()
	seedStore(gw)
	m := openedModel(t, gw)

	if cleared, cmd := m.ClearQuery(); cleared || cmd != nil {
		t.Fatal("an empty query reported a clear")
	}

	resolve(t, m, typeText(m, "login"))
	cleared, cmd := m.ClearQuery()
	if !cleared {
		t.Fatal("a query with text did not report a clear")
	}
	resolve(t, m, cmd)
	if got := lastSearch(t, gw); got.Text != "" {
		t.Fatalf("the clear searched for %q, want the empty query", got.Text)
	}
	if got := resultIDs(m); got != "tm-1,tm-2,tm-3" {
		t.Fatalf("the cleared search lists %q, want the open issues", got)
	}
}

func TestFailedSearchKeepsTheRowsAndSaysSo(t *testing.T) {
	gw := fakes.NewTracked()
	seedStore(gw)
	m := openedModel(t, gw)

	gw.SetError(fakes.MethodSearch, errors.New("store unreadable"))
	resolve(t, m, typeText(m, "x"))

	if m.IsLoading() {
		t.Fatal("a failed search left the surface loading")
	}
	view := plainView(m)
	for _, want := range []string{"load failed: store unreadable", "Fix login prompt"} {
		if !strings.Contains(view, want) {
			t.Errorf("the surface does not draw %q:\n%s", want, view)
		}
	}
}

func TestSurfaceGolden(t *testing.T) {
	gw := fakes.NewTracked()
	seedStore(gw)
	m := openedModel(t, gw)
	resolve(t, m, typeText(m, "login"))
	resolve(t, m, m.Update(tea.KeyMsg{Type: tea.KeyCtrlT}))

	testui.AssertMatchesGoldenNormalized(t, []byte(m.View(0)), "model_results_all_w100.golden")
}
