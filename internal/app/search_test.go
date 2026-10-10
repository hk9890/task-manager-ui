package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// searchSurfaces are the surfaces the store search opens from, each with the
// keys that take filterShell there from the Board.
var searchSurfaces = []struct {
	from mode.ID
	keys []string
}{
	{from: mode.Board},
	{from: mode.Docs, keys: []string{"tab"}},
	{from: mode.Detail, keys: []string{"enter"}},
}

// searchShell is filterShell with the store search open from the Board: the
// five open issues of the fixture, tm-1 selected.
func searchShell(t *testing.T, keys ...func(*config.Model)) (Model, *fakes.TrackedRepository) {
	t.Helper()
	m, gw := filterShell(t, keys...)
	m = press(t, m, "alt+f")
	if m.active != mode.Search || firstSelectionID(m, mode.Search) != "tm-1" {
		t.Fatalf("fixture: on %q with search selection %q, want search on tm-1", m.active, firstSelectionID(m, mode.Search))
	}
	return m, gw
}

// lastStoreSearch is the query of the latest Search call that was not the
// docs tab's.
func lastStoreSearch(t *testing.T, gw *fakes.TrackedRepository) domain.SearchIssuesQuery {
	t.Helper()
	calls := gw.CallsFor(fakes.MethodSearch)
	for idx := len(calls) - 1; idx >= 0; idx-- {
		if query := calls[idx].Args.(domain.SearchIssuesQuery); len(query.Types) == 0 {
			return query
		}
	}
	t.Fatal("the repository received no store search")
	return domain.SearchIssuesQuery{}
}

func storeSearchCount(gw *fakes.TrackedRepository) int {
	count := 0
	for _, call := range gw.CallsFor(fakes.MethodSearch) {
		if len(call.Args.(domain.SearchIssuesQuery).Types) == 0 {
			count++
		}
	}
	return count
}

// TestSearchOpensByKeyAndByButtonFromBoardDocsAndDetail: the key and a click
// on the menu-bar button both put the search on screen, inside the shell
// chrome, and it lists the open issues before anything is typed.
func TestSearchOpensByKeyAndByButtonFromBoardDocsAndDetail(t *testing.T) {
	for _, surface := range searchSurfaces {
		t.Run(string(surface.from), func(t *testing.T) {
			base, _ := filterShell(t)
			base = press(t, base, surface.keys...)
			if base.active != surface.from {
				t.Fatalf("fixture: on %q, want %q", base.active, surface.from)
			}
			x, text := barButton(t, base, "search", config.ShellActionOpenSearch)

			byKey := press(t, base, "alt+f")
			byClick := send(t, base, onScreen(base, leftClick(x+len(text)-1, headerMenuRow)))
			for name, m := range map[string]Model{"key": byKey, "click": byClick} {
				if m.active != mode.Search || m.searchFrom != surface.from {
					t.Fatalf("%s: on %q opened from %q, want search opened from %q", name, m.active, m.searchFrom, surface.from)
				}
				if m.lastBrowse == mode.Search || !mode.IsBrowse(m.lastBrowse) {
					t.Fatalf("%s: lastBrowse is %q, which is not a tab", name, m.lastBrowse)
				}
				view := plainShell(m)
				for _, want := range []string{"search alt+f", " Board ", " Docs ", "❯ search the store", "─ Results · open ", " 5 ─", "Fix login prompt", "Auth redesign", "type search"} {
					if !strings.Contains(view, want) {
						t.Errorf("%s: the search surface does not draw %q:\n%s", name, want, view)
					}
				}
				if got := lastStoreSearch(t, m.services.Repo.(*fakes.TrackedRepository)); got.Text != "" || got.IncludeClosed || got.Limit != 100 {
					t.Errorf("%s: the opening search asked for %#v", name, got)
				}
			}
		})
	}
}

// TestTabLineShowsBoardAndDocsAndNoneActiveWhileSearchIsUp: the search is not
// a tab.
func TestTabLineShowsBoardAndDocsAndNoneActiveWhileSearchIsUp(t *testing.T) {
	testui.ForceTrueColor(t)
	board, _ := filterShell(t)
	search := press(t, board, "alt+f")

	if len(mode.BrowseModes) != 2 || mode.IsBrowse(mode.Search) {
		t.Fatalf("the browse tabs are %v, want board and docs only", mode.BrowseModes)
	}
	tabs := tabLine(search)
	if got := strings.TrimRight(tabs[:headerTabsEnd()], " "); got != "   Board   Docs" {
		t.Fatalf("the tab line holds %q, want the two tabs", got)
	}
	// The active tab rides a background; with the search up none does.
	if !strings.Contains(board.renderTabs(), "48;2;") {
		t.Fatalf("fixture: the board tab draws no background:\n%q", board.renderTabs())
	}
	if strings.Contains(search.renderTabs(), "48;2;") {
		t.Fatalf("a tab is drawn active while the search is up:\n%q", search.renderTabs())
	}
}

// TestSearchTypingAndTheScopeKeyRunTheStoreSearch: every edit searches, and so
// does the scope key. Config refuses a binding on that key, so no shell action
// can be bound to it here.
func TestSearchTypingAndTheScopeKeyRunTheStoreSearch(t *testing.T) {
	m, gw := searchShell(t)

	m = typeInto(t, m, "login")
	if got := lastStoreSearch(t, gw); got.Text != "login" || got.IncludeClosed {
		t.Fatalf("typing searched for %#v, want login in open issues", got)
	}
	view := plainShell(m)
	if !strings.Contains(view, "❯ login") || !strings.Contains(view, " 2 ─") || strings.Contains(view, "Triage inbox") {
		t.Fatalf("the results do not follow the query:\n%s", view)
	}
	if got := firstSelectionID(m, mode.Search); got != "tm-1" {
		t.Fatalf("the shell holds search selection %q, want tm-1", got)
	}

	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-7", Title: "Old login page", Status: "closed", Type: "task"})
	m = press(t, m, "ctrl+t")
	if got := lastStoreSearch(t, gw); got.Text != "login" || !got.IncludeClosed {
		t.Fatalf("the scope key searched for %#v, want login in all issues", got)
	}
	if view := plainShell(m); !strings.Contains(view, "─ Results · all ") || !strings.Contains(view, "Old login page") {
		t.Fatalf("the scope key did not widen the results:\n%s", view)
	}
}

// TestSearchEnterOpensDetailAndEscapeReturnsWithStateKept: Detail opened from a
// result returns to the search, not to a tab, and nothing is searched again.
func TestSearchEnterOpensDetailAndEscapeReturnsWithStateKept(t *testing.T) {
	m, gw := searchShell(t)
	m = typeInto(t, m, "login")
	m = press(t, m, "down")
	if got := firstSelectionID(m, mode.Search); got != "tm-3" {
		t.Fatalf("setup: search selection %q, want tm-3", got)
	}
	searches := storeSearchCount(gw)

	m = press(t, m, "enter")
	if m.active != mode.Detail || m.detail.TargetID() != "tm-3" {
		t.Fatalf("enter left the shell on %q showing %q, want detail of tm-3", m.active, m.detail.TargetID())
	}
	if got, ok := m.selectedIssueID(); !ok || got != "tm-3" {
		t.Fatalf("under detail the shell acts on %q, want the search result tm-3", got)
	}

	m = press(t, m, "esc")
	if m.active != mode.Search || m.searchFrom != mode.Board {
		t.Fatalf("esc left detail for %q (opened from %q), want the search", m.active, m.searchFrom)
	}
	view := plainShell(m)
	if sel := m.currentSelection(); !strings.Contains(view, "❯ login") || sel == nil || sel.Issue.ID != "tm-3" {
		t.Fatalf("the search did not keep its query and selection:\n%s", view)
	}
	if got := storeSearchCount(gw); got != searches {
		t.Fatalf("the detail round trip ran %d more searches", got-searches)
	}
}

// TestSearchEnterDuringASearchOpensTheDetailOfItsResult: the rows on screen
// answer the query before the last key, so Enter waits for the search in
// flight and the detail opens on what that search found.
func TestSearchEnterDuringASearchOpensTheDetailOfItsResult(t *testing.T) {
	m, _ := searchShell(t)

	// The key's search is held back: its command is not run yet.
	next, typed := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("triage")})
	next, entered := next.(Model).Update(testKey("enter"))
	m = applyMessages(t, next.(Model), runBatch(entered))
	if m.active != mode.Search {
		t.Fatalf("enter left the search for %q before the result arrived", m.active)
	}

	m = applyMessages(t, m, runBatch(typed))
	if m.active != mode.Detail || m.detail.TargetID() != "tm-2" {
		t.Fatalf("the result left the shell on %q showing %q, want detail of tm-2", m.active, m.detail.TargetID())
	}
	if got, ok := m.selectedIssueID(); !ok || got != "tm-2" {
		t.Fatalf("under detail the shell acts on %q, want the search result tm-2", got)
	}
}

// TestSearchHeldEnterOpensTheResultInEitherMessageOrder: the result of a held
// Enter sends the selection change and the open request as two commands, and
// the runtime delivers them in either order. The request carries the selected
// row of the result, so the detail opens on it both times, and the selection
// change that arrives under that Detail changes nothing.
func TestSearchHeldEnterOpensTheResultInEitherMessageOrder(t *testing.T) {
	for name, requestFirst := range map[string]bool{"the request first": true, "the selection change first": false} {
		t.Run(name, func(t *testing.T) {
			m, gw := searchShell(t)

			next, typed := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("triage")})
			next, entered := next.(Model).Update(testKey("enter"))
			m = applyMessages(t, next.(Model), runBatch(entered))

			// The pause of the edit ends, its search returns and the result is
			// applied; what the search sends for the result is held back.
			var request, change tea.Msg
			for _, pauseEnded := range runBatch(typed) {
				next, searching := m.Update(pauseEnded)
				m = next.(Model)
				for _, result := range runBatch(searching) {
					next, cmd := m.Update(result)
					m = next.(Model)
					for _, msg := range runBatch(cmd) {
						switch unscoped(msg).(type) {
						case mode.ActionRequestMsg:
							request = msg
						case mode.SelectionChangedMsg:
							change = msg
						}
					}
				}
			}
			if request == nil || change == nil {
				t.Fatalf("the result of a held enter sent request %#v and selection change %#v, want both", request, change)
			}
			if got := firstSelectionID(m, mode.Search); m.active != mode.Search || got != "tm-1" {
				t.Fatalf("setup: on %q with search selection %q, want the search still on tm-1", m.active, got)
			}

			first, second := change, request
			if requestFirst {
				first, second = request, change
			}
			m = applyMessages(t, m, []tea.Msg{first})
			if requestFirst && (m.active != mode.Detail || m.detail.TargetID() != "tm-2") {
				t.Fatalf("the request left the shell on %q showing %q, want detail of tm-2", m.active, m.detail.TargetID())
			}
			loads := len(gw.CallsFor(fakes.MethodIssue))
			m = applyMessages(t, m, []tea.Msg{second})

			if m.active != mode.Detail || m.searchFrom != mode.Board {
				t.Fatalf("the shell is on %q (search from %q), want the detail of a search result", m.active, m.searchFrom)
			}
			if m.detail.TargetID() != "tm-2" || m.detail.Detail.Summary.ID != "tm-2" || m.detail.IsLoading() {
				t.Fatalf("detail targets %q and shows %q (loading %v), want tm-2 loaded", m.detail.TargetID(), m.detail.Detail.Summary.ID, m.detail.IsLoading())
			}
			if got, ok := m.selectedIssueID(); !ok || got != "tm-2" {
				t.Fatalf("under detail the shell acts on %q, want the search result tm-2", got)
			}
			if got := len(gw.CallsFor(fakes.MethodIssue)) - loads; requestFirst && got != 0 {
				t.Fatalf("the selection change under detail loaded the issue %d more times", got)
			}

			m = press(t, m, "esc")
			if got := firstSelectionID(m, mode.Search); m.active != mode.Search || got != "tm-2" {
				t.Fatalf("esc went to %q with search selection %q, want the search on tm-2", m.active, got)
			}
		})
	}
}

// TestOpenDetailRequestWithoutASelectionOpensNothing: the request names its
// row, and one that names none says so instead of opening the stored row.
func TestOpenDetailRequestWithoutASelectionOpensNothing(t *testing.T) {
	m, _ := filterShell(t)
	if firstSelectionID(m, mode.Board) == "" {
		t.Fatal("fixture: the board holds no selection")
	}

	m = applyMessages(t, m, runBatch(mode.RequestOpenDetailCmd(mode.Board, nil)))
	if m.active != mode.Board {
		t.Fatalf("a request without a selection left the board for %q", m.active)
	}
	if view := plainShell(m); !strings.Contains(view, "No selected issue to open in detail mode") {
		t.Fatalf("the shell does not say why nothing opened:\n%s", view)
	}
}

// TestSearchEscapeClearsTheQueryThenReturnsToTheOpeningSurface covers the three
// surfaces the search opens from.
func TestSearchEscapeClearsTheQueryThenReturnsToTheOpeningSurface(t *testing.T) {
	for _, surface := range searchSurfaces {
		t.Run(string(surface.from), func(t *testing.T) {
			m, gw := filterShell(t)
			m = press(t, m, surface.keys...)
			before, _ := m.selectedIssueID()

			m = press(t, m, "alt+f")
			m = typeInto(t, m, "inbox")
			if got := firstSelectionID(m, mode.Search); got != "tm-2" {
				t.Fatalf("setup: search selection %q, want tm-2", got)
			}

			m = press(t, m, "esc")
			if m.active != mode.Search {
				t.Fatalf("the esc that cleared the query left the search for %q", m.active)
			}
			if got := lastStoreSearch(t, gw); got.Text != "" {
				t.Fatalf("the clear searched for %q, want the empty query", got.Text)
			}
			if view := plainShell(m); !strings.Contains(view, "❯ search the store") || !strings.Contains(view, " 5 ─") {
				t.Fatalf("esc did not clear the query:\n%s", view)
			}

			m = press(t, m, "esc")
			if m.active != surface.from || m.searchFrom != "" {
				t.Fatalf("with an empty query esc went to %q (search still from %q), want %q", m.active, m.searchFrom, surface.from)
			}
			if got, _ := m.selectedIssueID(); got != before {
				t.Fatalf("back on %q the shell acts on %q, want %q as before the search", surface.from, got, before)
			}
			if surface.from == mode.Detail && m.detail.TargetID() != before {
				t.Fatalf("detail shows %q after the search, want %q", m.detail.TargetID(), before)
			}
		})
	}
}

// TestSearchClosedFromADrilledDetailReturnsToTheDrilledIssue: the Detail the
// search returns to shows the issue it showed, not the browse row under it.
func TestSearchClosedFromADrilledDetailReturnsToTheDrilledIssue(t *testing.T) {
	m := drilledIntoChild(t, fakes.NewTracked())

	m = press(t, m, "alt+f")
	if m.active != mode.Search || m.searchFrom != mode.Detail {
		t.Fatalf("fixture: on %q opened from %q, want the search opened from detail", m.active, m.searchFrom)
	}

	m = press(t, m, "esc")
	if m.active != mode.Detail || m.detail.Detail.Summary.ID != "tm-child" {
		t.Fatalf("esc left the search for %q showing %q, want detail of tm-child", m.active, m.detail.Detail.Summary.ID)
	}
	if got, _ := m.selectedIssueID(); got != "tm-child" {
		t.Fatalf("back in detail the shell acts on %q, want the drilled issue tm-child", got)
	}

	m = press(t, m, "esc")
	if got, _ := m.selectedIssueID(); m.active != mode.Board || got != "tm-epic" {
		t.Fatalf("esc from detail went to %q acting on %q, want the board row tm-epic", m.active, got)
	}
}

// TestSearchIsLeftForATabByTheTabKeysAndAClick: the tab keys step from the tab
// the operator was last on.
func TestSearchIsLeftForATabByTheTabKeysAndAClick(t *testing.T) {
	base, _ := searchShell(t)

	byKey := press(t, base, "tab")
	if byKey.active != mode.Docs || byKey.searchFrom != "" || byKey.lastBrowse != mode.Docs {
		t.Fatalf("tab went to %q (search from %q), want docs", byKey.active, byKey.searchFrom)
	}
	if got := press(t, base, "shift+tab"); got.active != mode.Docs || got.searchFrom != "" {
		t.Fatalf("shift+tab went to %q, want docs", got.active)
	}

	x, _ := testui.FindCell(t, tabLine(base), " Board ")
	byClick := send(t, base, onScreen(base, leftClick(x+1, headerTabsRow)))
	if byClick.active != mode.Board || byClick.searchFrom != "" {
		t.Fatalf("a click on the board tab went to %q (search from %q)", byClick.active, byClick.searchFrom)
	}
	if got, _ := byClick.selectedIssueID(); got != firstSelectionID(byClick, mode.Board) {
		t.Fatalf("back on the board the shell acts on %q, not on the board row", got)
	}

	// From a Detail opened from the search, a tab key leaves the search too.
	detail := press(t, base, "enter", "tab")
	if detail.active != mode.Docs || detail.searchFrom != "" {
		t.Fatalf("tab from the detail of a result went to %q (search from %q)", detail.active, detail.searchFrom)
	}
}

// TestIssueActionsActOnTheSelectedSearchResult: edit, update, comment and close
// take the search selection, on the search and under a Detail opened from it,
// while the Board holds another row.
func TestIssueActionsActOnTheSelectedSearchResult(t *testing.T) {
	surfaces := map[string][]string{"search": nil, "detail of a result": {"enter"}}
	for name, keys := range surfaces {
		t.Run(name, func(t *testing.T) {
			m, _ := searchShell(t)
			m = press(t, m, "down", "down")
			m = press(t, m, keys...)
			if board, search := firstSelectionID(m, mode.Board), firstSelectionID(m, mode.Search); board != "tm-1" || search != "tm-3" {
				t.Fatalf("setup: board on %q and search on %q, want tm-1 and tm-3", board, search)
			}

			_, cmd := m.Update(testKey("alt+e"))
			var edited string
			for _, msg := range runBatch(cmd) {
				if prepared, ok := unscoped(msg).(editIssuePreparedMsg); ok {
					edited = prepared.issueID
				}
			}
			if edited != "tm-3" {
				t.Errorf("edit prepared %q, want tm-3", edited)
			}

			if got := openDialog(t, m, "alt+u"); got.actionState.kind != mutationUpdate || got.actionState.issue.ID != "tm-3" {
				t.Errorf("update opened on %q, want tm-3", got.actionState.issue.ID)
			}
			if got := openDialog(t, m, "alt+a"); got.actionState.kind != mutationComment || got.actionState.issue.ID != "tm-3" {
				t.Errorf("comment opened on %q, want tm-3", got.actionState.issue.ID)
			}
			if got := openDialog(t, m, "delete"); got.actionState.kind != mutationClose || got.actionState.issue.ID != "tm-3" {
				t.Errorf("close opened on %q, want tm-3", got.actionState.issue.ID)
			}
		})
	}
}

// TestSearchReloadsOnAStoreChangeAndKeepsTheSelectedIssue: the search takes the
// auto refresh a tab gets, with the query it holds.
func TestSearchReloadsOnAStoreChangeAndKeepsTheSelectedIssue(t *testing.T) {
	m, gw := searchShell(t)
	m = typeInto(t, m, "login")
	m = press(t, m, "down")
	if got := firstSelectionID(m, mode.Search); got != "tm-3" {
		t.Fatalf("setup: search selection %q, want tm-3", got)
	}

	// A new match sorts ahead of the selected issue.
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-0", Title: "Login banner", Status: "open", Type: "task"})
	searches := storeSearchCount(gw)
	m = applyMessages(t, m, []tea.Msg{storeChangedMsg{}})

	if got := storeSearchCount(gw) - searches; got != 1 {
		t.Fatalf("the store change ran %d searches, want 1", got)
	}
	if got := lastStoreSearch(t, gw); got.Text != "login" {
		t.Fatalf("the reload searched for %q, want the query login", got.Text)
	}
	if view := plainShell(m); !strings.Contains(view, "Login banner") || !strings.Contains(view, " 3 ─") {
		t.Fatalf("the results did not reload:\n%s", view)
	}
	if got := firstSelectionID(m, mode.Search); got != "tm-3" {
		t.Fatalf("the reload moved the selection to %q, want tm-3", got)
	}
}

// TestSearchInFlightSpinsTheHeader: the search is no tab, and its work in
// flight still draws the header spinner.
func TestSearchInFlightSpinsTheHeader(t *testing.T) {
	m, _ := filterShell(t)
	next, _ := m.Update(testKey("alt+f"))
	opened := next.(Model)
	if !opened.search.IsLoading() || !opened.workInFlight() {
		t.Fatalf("the opening search: search loading %v, work in flight %v; want both", opened.search.IsLoading(), opened.workInFlight())
	}
}

// TestStoreSwitchRebuildsTheSearch: the query and the results of the previous
// store end with it.
func TestStoreSwitchRebuildsTheSearch(t *testing.T) {
	s := newTwoStores(t)
	m := applyMessages(t, s.m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 30}})
	m = press(t, m, "alt+f")
	m = typeInto(t, m, "alpha")
	if view := plainShell(m); !strings.Contains(view, "❯ alpha") || !strings.Contains(view, "Alpha store issue") {
		t.Fatalf("fixture: the search does not show alpha's issue:\n%s", view)
	}

	m = switchToBravo(t, m)
	if m.active != mode.Board || m.searchFrom != "" || firstSelectionID(m, mode.Search) != "" {
		t.Fatalf("after the switch: on %q, search from %q, search selection %q; want a fresh board", m.active, m.searchFrom, firstSelectionID(m, mode.Search))
	}

	m = press(t, m, "alt+f")
	view := plainShell(m)
	if !strings.Contains(view, "❯ search the store") || !strings.Contains(view, "Bravo store issue") || strings.Contains(view, "Alpha store issue") {
		t.Fatalf("the search was not rebuilt for the new store:\n%s", view)
	}
}

// TestStoreSwitchDropsAPendingEditPause: the pause of an edit is work of its
// store, so one that ends after a switch starts no search in the new store,
// also when the new search stands at the generation the pause carries.
func TestStoreSwitchDropsAPendingEditPause(t *testing.T) {
	s := newTwoStores(t)
	m := applyMessages(t, s.m, []tea.Msg{tea.WindowSizeMsg{Width: 160, Height: 30}})
	m = press(t, m, "alt+f")

	next, typed := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alpha")})
	pauseEnded := runBatch(typed)
	if len(pauseEnded) == 0 {
		t.Fatal("fixture: the edit started no pause")
	}

	// The opening search and the scope key are two generations, as the opening
	// search and the edit were in the previous store.
	m = switchToBravo(t, next.(Model))
	m = press(t, m, "alt+f", "ctrl+t")
	bravo := s.bravo.Repo.(*fakes.TrackedRepository)
	searches := storeSearchCount(bravo)

	m = applyMessages(t, m, pauseEnded)
	if got := storeSearchCount(bravo) - searches; got != 0 {
		t.Fatalf("the pause of the previous store ran %d searches in the new one", got)
	}
	if view := plainShell(m); !strings.Contains(view, "❯ search the store") || !strings.Contains(view, "Bravo store issue") {
		t.Fatalf("the new store's search did not stay as it was:\n%s", view)
	}
}

// TestSearchSurfaceGolden pins the search inside the shell chrome.
func TestSearchSurfaceGolden(t *testing.T) {
	m, _ := searchShell(t)
	m = typeInto(t, m, "login")
	testui.AssertMatchesGoldenNormalized(t, []byte(m.View()), "model_search_results_w160.golden")
}
