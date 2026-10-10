package search

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	uisearch "github.com/hk9890/task-manager-ui/internal/ui/search"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// The shell gives the search surface 20 of an 80x24 terminal's lines.
const (
	scrollWidth  = 80
	scrollHeight = 20
	scrollIssues = 13
)

func scrollTitle(idx int) string { return fmt.Sprintf("reach-%02d", idx) }

func scrollID(idx int) string { return fmt.Sprintf("tm-%02d", idx) }

// scrollSearch is the Search tab on a store of 13 open issues, loaded through
// the repository, with the results focused.
func scrollSearch(t *testing.T) (*Model, *fakes.TrackedRepository) {
	t.Helper()
	return scrollSearchOf(t, scrollIssues)
}

// scrollSearchOf is scrollSearch on a store of issues open issues.
func scrollSearchOf(t *testing.T, issues int) (*Model, *fakes.TrackedRepository) {
	t.Helper()

	repo := fakes.NewTracked()
	for idx := 0; idx < issues; idx++ {
		repo.Memory.Seed(memoryrepo.Issue{ID: scrollID(idx), Title: scrollTitle(idx), Status: "open", Type: "task", Priority: 2})
	}
	m := NewModel(context.Background(), repo, nil)
	m.SetSize(scrollWidth, scrollHeight)
	applyMessages(m, testui.DrainCmd(m.Init()))
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.focus != uisearch.FocusResults {
		t.Fatalf("fixture focus = %d, want the results pane", m.focus)
	}
	return m, repo
}

func pressRune(m *Model, key string, times int) {
	for i := 0; i < times; i++ {
		pressAndResolve(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}
}

// assertSelectionDrawn fails unless the selected result is result want, drawn
// whole, with the selection bar on its title line.
func assertSelectionDrawn(t *testing.T, m *Model, want int) {
	t.Helper()

	if m.selectedRow != want {
		t.Fatalf("selected row = %d, want %d", m.selectedRow, want)
	}
	view := m.View(0)
	bar, _ := styles.SelectionPrefix(true, false)
	_, y := testui.FindCell(t, view, bar+"T "+scrollTitle(want))
	lines := strings.Split(testui.AnsiEscapePattern.ReplaceAllString(view, ""), "\n")
	if !strings.Contains(lines[y+1], "P2 OPN "+scrollID(want)) {
		t.Fatalf("result %d is cut by the pane: the line under its title is %q", want, lines[y+1])
	}
}

// TestSearchReachesEveryResultOfAStoreLargerThanThePane is the regression:
// the query limit was the number of rows the pane draws, so at 80x24 a store
// of 13 open issues showed six and no key reached the rest.
func TestSearchReachesEveryResultOfAStoreLargerThanThePane(t *testing.T) {
	t.Parallel()

	m, repo := scrollSearch(t)

	calls := repo.CallsFor(fakes.MethodSearch)
	if len(calls) != 1 {
		t.Fatalf("Search calls = %d, want 1", len(calls))
	}
	if query := calls[0].Args.(domain.SearchIssuesQuery); query.Limit != searchPageSize || query.Offset != 0 {
		t.Fatalf("query limit %d offset %d, want the fixed page %d from 0", query.Limit, query.Offset, searchPageSize)
	}
	if len(m.page.Results) != scrollIssues {
		t.Fatalf("loaded %d results, want all %d", len(m.page.Results), scrollIssues)
	}

	assertSelectionDrawn(t, m, 0)
	for row := 1; row < scrollIssues; row++ {
		pressRune(m, "j", 1)
		assertSelectionDrawn(t, m, row)
	}
	pressRune(m, "j", 1)
	assertSelectionDrawn(t, m, scrollIssues-1)

	for row := scrollIssues - 2; row >= 0; row-- {
		pressRune(m, "k", 1)
		assertSelectionDrawn(t, m, row)
	}
}

func TestResultsHeaderCountsTheLoadedSetAndSaysWhenTheWindowClipsIt(t *testing.T) {
	t.Parallel()

	m, _ := scrollSearch(t)
	view := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	if !strings.Contains(view, " 7/13 exact · open ─╮") {
		t.Fatalf("header of a clipped list in the 80-column rail does not read `7/13 exact · open`:\n%s", view)
	}

	m.SetSize(160, scrollHeight)
	view = testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	if !strings.Contains(view, " 7 of 13 exact · open ") {
		t.Fatalf("header of a clipped list does not read `7 of 13 exact · open`:\n%s", view)
	}

	m.SetSize(scrollWidth, 40)
	view = testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	if !strings.Contains(view, " 13 exact · open") || strings.Contains(view, " of 13") {
		t.Fatalf("header of a list that fits does not read a plain `13 exact · open`:\n%s", view)
	}
}

// TestClickAfterScrollingSelectsTheResultUnderThePointer finds a result where
// the renderer drew it in a scrolled pane and clicks there.
func TestClickAfterScrollingSelectsTheResultUnderThePointer(t *testing.T) {
	t.Parallel()

	m, _ := scrollSearch(t)
	pressRune(m, "j", scrollIssues-1)
	if m.scrollOffset == 0 {
		t.Fatal("fixture: moving to the last result did not scroll the pane")
	}

	target := m.scrollOffset + 1
	cmd := m.Update(mouseAt(t, m, mode.MouseClick, "T "+scrollTitle(target), 0))
	if cmd == nil || m.selectedRow != target {
		t.Fatalf("click on %q selected row %d (offset %d), want row %d", scrollTitle(target), m.selectedRow, m.scrollOffset, target)
	}
	assertSelectionDrawn(t, m, target)

	// The second click, on the result's other line, opens what the first selected.
	second := mouseAt(t, m, mode.MouseClick, "T "+scrollTitle(target), 200)
	second.Y++
	if !opensDetail(m.Update(second)) || m.selectedIssueID() != scrollID(target) {
		t.Fatalf("second click did not open result %d; the selection is %q", target, m.selectedIssueID())
	}
}

func TestWheelScrollsTheResultsWithTheSelection(t *testing.T) {
	t.Parallel()

	m, _ := scrollSearch(t)
	wheel := mouseAt(t, m, mode.MouseWheelDown, "Results ─", 0)
	for row := 1; row < scrollIssues; row++ {
		_ = m.Update(wheel)
		assertSelectionDrawn(t, m, row)
	}

	wheel.Kind = mode.MouseWheelUp
	for row := scrollIssues - 2; row >= 0; row-- {
		_ = m.Update(wheel)
		assertSelectionDrawn(t, m, row)
	}
}

func TestReloadKeepsTheScrolledWindowAndANewQueryResetsIt(t *testing.T) {
	t.Parallel()

	m, _ := scrollSearch(t)
	pressRune(m, "j", scrollIssues-1)
	pressRune(m, "k", 1)
	offset := m.scrollOffset
	if offset == 0 {
		t.Fatal("fixture: the pane is not scrolled")
	}

	applyMessages(m, testui.DrainCmd(m.Reload()))
	if m.selectedRow != scrollIssues-2 || m.scrollOffset != offset {
		t.Fatalf("reload moved the window: row %d offset %d, want row %d offset %d", m.selectedRow, m.scrollOffset, scrollIssues-2, offset)
	}
	assertSelectionDrawn(t, m, scrollIssues-2)

	// A new query, typed and submitted from the scrolled pane.
	pressRune(m, "/", 1)
	pressRune(m, "reach", 1)
	pressAndResolve(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.appliedQuery != "reach" || len(m.page.Results) != scrollIssues {
		t.Fatalf("fixture: query %q loaded %d results, want %q and %d", m.appliedQuery, len(m.page.Results), "reach", scrollIssues)
	}
	if m.scrollOffset != 0 {
		t.Fatalf("a new result set kept scroll offset %d, want 0", m.scrollOffset)
	}
	assertSelectionDrawn(t, m, 0)
}

// TestAListThatFitsThePaneDoesNotScroll: the window reserved the two banner
// lines also with no banner up, so the last of 7 results in a pane of 7 rows
// scrolled the first away and left a blank row.
func TestAListThatFitsThePaneDoesNotScroll(t *testing.T) {
	t.Parallel()

	const fitting = 7
	m, _ := scrollSearchOf(t, fitting)
	pressRune(m, "j", fitting-1)
	assertSelectionDrawn(t, m, fitting-1)
	if m.scrollOffset != 0 {
		t.Fatalf("scroll offset = %d in a pane that draws the whole list", m.scrollOffset)
	}

	view := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	for idx := 0; idx < fitting; idx++ {
		for _, want := range []string{"T " + scrollTitle(idx), "P2 OPN " + scrollID(idx)} {
			if !strings.Contains(view, want) {
				t.Errorf("%q is not drawn:\n%s", want, view)
			}
		}
	}
	if !strings.Contains(view, "─ 7 exact · open ─╮") {
		t.Fatalf("header of a list that fits does not read a plain `7 exact · open`:\n%s", view)
	}
}

// TestTheBannerTakesARowFromTheScrollWindow: a typed draft puts the
// stale-results banner over the rows with no selection move. The selection was
// on the last row drawn and stays on a drawn row.
func TestTheBannerTakesARowFromTheScrollWindow(t *testing.T) {
	t.Parallel()

	m, _ := scrollSearch(t)
	withoutBanner := m.searchItemCapacity()
	pressRune(m, "j", withoutBanner-1)
	if m.scrollOffset != 0 {
		t.Fatalf("fixture: the last row of the first window scrolled the pane to %d", m.scrollOffset)
	}

	pressRune(m, "/", 1)
	pressRune(m, "x", 1)
	if view := m.View(0); !strings.Contains(view, "are stale") {
		t.Fatalf("fixture: a typed draft drew no stale-results banner:\n%s", view)
	}
	if got := m.searchItemCapacity(); got != withoutBanner-1 {
		t.Fatalf("scroll window under the banner = %d rows, want %d", got, withoutBanner-1)
	}
	assertSelectionDrawn(t, m, withoutBanner-1)
}

func TestResizeKeepsTheSelectionInTheScrollWindow(t *testing.T) {
	t.Parallel()

	m, _ := scrollSearch(t)
	m.SetSize(scrollWidth, 40)
	pressRune(m, "j", scrollIssues-1)
	if m.scrollOffset != 0 {
		t.Fatalf("a pane that fits the list scrolled to %d", m.scrollOffset)
	}

	m.SetSize(scrollWidth, scrollHeight)
	assertSelectionDrawn(t, m, scrollIssues-1)

	m.SetSize(scrollWidth, 40)
	if m.scrollOffset != 0 {
		t.Fatalf("growing the pane left scroll offset %d with room for the whole list", m.scrollOffset)
	}
}
