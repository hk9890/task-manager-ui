package board

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/repository"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// filterData is a board with rows in all four columns. "login" matches one row
// of Not Ready, two of Ready, none of In Progress and one of Done.
func filterData() repository.DashboardData {
	issue := func(id, title, status string) domain.IssueSummary {
		return domain.IssueSummary{ID: id, Title: title, Status: status, Type: "task", Priority: 2}
	}
	return repository.DashboardData{
		ReadyExplain: domain.ReadyExplainResult{
			Ready: []domain.IssueSummary{
				issue("tm-20", "Fix login prompt", "open"),
				issue("tm-21", "Triage inbox", "open"),
				issue("tm-22", "Login audit", "open"),
			},
			Blocked: []domain.BlockedIssueView{
				{Issue: issue("tm-10", "Schema rework", "blocked")},
				{Issue: issue("tm-11", "Blocked login migration", "blocked")},
			},
		},
		InProgress:  []domain.IssueSummary{issue("tm-30", "Board shortcuts", "in_progress")},
		Closed:      []domain.IssueSummary{issue("tm-40", "Login page", "closed"), issue("tm-41", "Old chore", "closed")},
		ClosedTotal: 2,
	}
}

// filterBoard is a loaded board wide enough to draw all four columns.
// The returned repository is what a later load reads.
func filterBoard(t *testing.T) (*Model, *cannedDashboard) {
	t.Helper()
	repo := &cannedDashboard{resp: filterData()}
	m := newBoardModel(repo, resolvedBoardKeys(t))
	m.SetSize(180, 24)
	_ = m.Update(m.Init()())
	return m, repo
}

func typeText(m *Model, text string) tea.Cmd {
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
}

// shownIDs is the issue IDs each column draws.
func shownIDs(m *Model) string {
	columns := make([]string, len(m.columns))
	for i, col := range m.columns {
		ids := make([]string, len(col.shown))
		for j, issue := range col.shown {
			ids[j] = issue.ID
		}
		columns[i] = strings.Join(ids, ",")
	}
	return strings.Join(columns, " | ")
}

func plainView(m *Model) string {
	return testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
}

// TestFilterNarrowsAllFourColumnsAndCountsMatchesOfLoaded: one query filters
// every column, each header counts its matches of its loaded rows, and the
// rows keep their order.
func TestFilterNarrowsAllFourColumnsAndCountsMatchesOfLoaded(t *testing.T) {
	t.Parallel()

	m, _ := filterBoard(t)
	if got, want := shownIDs(m), "tm-10,tm-11 | tm-20,tm-21,tm-22 | tm-30 | tm-40,tm-41"; got != want {
		t.Fatalf("setup: the board shows %q, want %q", got, want)
	}

	typeText(m, "LOGIN")
	if got, want := shownIDs(m), "tm-11 | tm-20,tm-22 |  | tm-40"; got != want {
		t.Fatalf("after the query the board shows %q, want %q", got, want)
	}
	view := plainView(m)
	for _, want := range []string{"❯ LOGIN", " 1 of 2 ─", " 2 of 3 ─", " 0 of 1 ─", "(no matches)"} {
		if !strings.Contains(view, want) {
			t.Errorf("the filtered board does not draw %q:\n%s", want, view)
		}
	}
	if strings.Count(view, " 1 of 2 ─") != 2 {
		t.Errorf("Not Ready and Done must both count 1 of 2:\n%s", view)
	}
	for _, hidden := range []string{"Schema rework", "Triage inbox", "Board shortcuts", "Old chore"} {
		if strings.Contains(view, hidden) {
			t.Errorf("the filtered board still draws %q:\n%s", hidden, view)
		}
	}

	// A second word narrows further: every word must match, in title or ID.
	typeText(m, " tm-2")
	if got, want := shownIDs(m), " | tm-20,tm-22 |  | "; got != want {
		t.Fatalf("after two words the board shows %q, want %q", got, want)
	}

	_ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if got, want := shownIDs(m), "tm-10,tm-11 | tm-20,tm-21,tm-22 | tm-30 | tm-40,tm-41"; got != want {
		t.Fatalf("an empty query shows %q, want everything", got)
	}
	if view := plainView(m); strings.Contains(view, " of ") || !strings.Contains(view, "❯ filter issues") {
		t.Errorf("an empty query must draw plain counts and the placeholder:\n%s", view)
	}
}

// TestFilterMovesTheFocusToTheFirstColumnWithAMatch: a focused column the
// query emptied gives the focus away; a query nothing matches leaves it.
func TestFilterMovesTheFocusToTheFirstColumnWithAMatch(t *testing.T) {
	t.Parallel()

	m, _ := filterBoard(t)
	if m.focusedColumn != 0 || m.selectedIssueID() != "tm-10" {
		t.Fatalf("setup: focus on column %d, selection %q", m.focusedColumn, m.selectedIssueID())
	}

	cmd := typeText(m, "audit")
	if m.focusedColumn != 1 || m.selectedIssueID() != "tm-22" {
		t.Fatalf("focus on column %d with %q selected, want column 1 with tm-22", m.focusedColumn, m.selectedIssueID())
	}
	if got := selectionFrom(t, cmd); got != "tm-22" {
		t.Fatalf("the query reported selection %q, want tm-22", got)
	}
	assertSelectionDrawn(t, m)

	// The column the focus left has a match again: the focus goes back, to the
	// issue that was selected there.
	_ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.focusedColumn != 0 || m.selectedIssueID() != "tm-10" {
		t.Fatalf("after the clear: focus on column %d with %q selected, want column 0 with tm-10", m.focusedColumn, m.selectedIssueID())
	}

	// A focus the operator moved is theirs: it stays while its column has a
	// match, although an earlier column has one too, and a clear leaves it.
	typeText(m, "audit")
	_ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	typeText(m, "login")
	if m.focusedColumn != 1 {
		t.Fatalf("focus moved to column %d although the operator put it on column 1", m.focusedColumn)
	}

	cmd = typeText(m, "zzz")
	if m.focusedColumn != 1 || m.currentSelection() != nil {
		t.Fatalf("with no match anywhere: focus on column %d, selection %v; want column 1 and none", m.focusedColumn, m.currentSelection())
	}
	reported := false
	for _, msg := range testui.DrainCmd(cmd) {
		if changed, ok := msg.(mode.SelectionChangedMsg); ok && changed.Selection == nil {
			reported = true
		}
	}
	if !reported {
		t.Fatal("a query that leaves no selection must report it to the shell")
	}
}

// TestFilterKeepsEachColumnsSelectionOnItsIssue: a query change keeps a
// column's selection on the same issue while it matches, and takes the first
// match when it does not.
func TestFilterKeepsEachColumnsSelectionOnItsIssue(t *testing.T) {
	t.Parallel()

	m, _ := filterBoard(t)
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // Not Ready: tm-11
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	_ = m.Update(tea.KeyMsg{Type: tea.KeyEnd}) // Ready: tm-22
	if m.selectedIssueID() != "tm-22" {
		t.Fatalf("setup: selection %q, want tm-22", m.selectedIssueID())
	}

	if cmd := typeText(m, "login"); cmd != nil {
		t.Fatalf("the selection did not change, so the query must report none; got %v", testui.DrainCmd(cmd))
	}
	if m.selectedIssueID() != "tm-22" || m.selectedRow[1] != 1 {
		t.Fatalf("selection %q on row %d, want tm-22 on row 1 of the filtered column", m.selectedIssueID(), m.selectedRow[1])
	}
	if m.selectedRow[0] != 0 || m.columns[0].shown[0].ID != "tm-11" {
		t.Fatalf("the unfocused column lost its issue: row %d of %v", m.selectedRow[0], m.columns[0].shown)
	}

	// Back to everything: each selection stays on its issue, at its new row.
	cleared, cmd := m.ClearQuery()
	if !cleared || cmd != nil {
		t.Fatalf("ClearQuery() = %v with a selection report %v; want cleared and no report", cleared, cmd != nil)
	}
	if m.selectedIssueID() != "tm-22" || m.selectedRow[1] != 2 || m.selectedRow[0] != 1 {
		t.Fatalf("after the clear: selection %q on row %d, Not Ready row %d; want tm-22 on row 2 and tm-11 on row 1",
			m.selectedIssueID(), m.selectedRow[1], m.selectedRow[0])
	}
	assertSelectionDrawn(t, m)
	if cleared, _ := m.ClearQuery(); cleared {
		t.Fatal("ClearQuery() on an empty query reported a clear")
	}

	// tm-22 does not match: the first match takes the selection.
	cmd = typeText(m, "login fix")
	if m.selectedIssueID() != "tm-20" || selectionFrom(t, cmd) != "tm-20" {
		t.Fatalf("selection %q, want the first match tm-20", m.selectedIssueID())
	}
}

// TestFilterClampsTheScrollOfANarrowedColumn: a column scrolled deep draws its
// few matches from the top.
func TestFilterClampsTheScrollOfANarrowedColumn(t *testing.T) {
	t.Parallel()

	m := newBoardModel(newDashboardStub(repository.DashboardData{}), resolvedBoardKeys(t))
	m.SetSize(120, 14)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady, issues: makeClosedIssues(40), total: 40, exact: true},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone},
	}
	m.filterColumns()
	m.focusedColumn = 1
	_ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.scrollOffset[1] == 0 {
		t.Fatal("setup: expected the column to be scrolled")
	}

	typeText(m, "issue 3") // closed-3 and closed-30 to closed-39
	if m.selectedIssueID() != "closed-39" {
		t.Fatalf("selection %q, want it kept on closed-39", m.selectedIssueID())
	}
	assertSelectionDrawn(t, m)

	typeText(m, "9")
	if got := shownIDs(m); got != " | closed-39 |  | " {
		t.Fatalf("the board shows %q, want only closed-39", got)
	}
	if m.scrollOffset[1] != 0 {
		t.Fatalf("a one-row column is scrolled to %d", m.scrollOffset[1])
	}
	assertSelectionDrawn(t, m)
}

// TestFilterSurvivesAutoRefreshAndManualReload: the query is the operator's,
// not the load's.
func TestFilterSurvivesAutoRefreshAndManualReload(t *testing.T) {
	t.Parallel()

	m, repo := filterBoard(t)
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	typeText(m, "login")
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selectedIssueID() != "tm-22" {
		t.Fatalf("setup: selection %q, want tm-22", m.selectedIssueID())
	}

	// The refresh brings a new match ahead of the selected row.
	data := filterData()
	data.ReadyExplain.Ready = append([]domain.IssueSummary{{ID: "tm-19", Title: "Login throttle", Status: "open", Type: "task"}}, data.ReadyExplain.Ready...)
	repo.resp = data

	_ = m.Update(m.AutoRefresh()())
	if m.query.Text() != "login" {
		t.Fatalf("the auto refresh left the query %q", m.query.Text())
	}
	if got, want := shownIDs(m), "tm-11 | tm-19,tm-20,tm-22 |  | tm-40"; got != want {
		t.Fatalf("after the auto refresh the board shows %q, want %q", got, want)
	}
	if m.selectedIssueID() != "tm-22" {
		t.Fatalf("the auto refresh moved the selection to %q", m.selectedIssueID())
	}

	// The reload key resets focus and selection and keeps the query. Not Ready
	// is the first column with a match.
	_ = m.Update(m.Reload()())
	if m.query.Text() != "login" {
		t.Fatalf("the reload left the query %q", m.query.Text())
	}
	if got, want := shownIDs(m), "tm-11 | tm-19,tm-20,tm-22 |  | tm-40"; got != want {
		t.Fatalf("after the reload the board shows %q, want %q", got, want)
	}
	if m.focusedColumn != 0 || m.selectedIssueID() != "tm-11" {
		t.Fatalf("after the reload: column %d, selection %q; want column 0 and tm-11", m.focusedColumn, m.selectedIssueID())
	}
}

// TestDoneLoadMoreCountsInTheFilteredList: with a query the next Done page
// loads when the selection nears the end of the matches, the page is read from
// where the loaded rows end, and its matches join the list.
func TestDoneLoadMoreCountsInTheFilteredList(t *testing.T) {
	t.Parallel()

	const loaded = 35
	page := make([]domain.IssueSummary, 10)
	for i := range page {
		page[i] = domain.IssueSummary{ID: fmt.Sprintf("closed-%d", 100+i), Title: fmt.Sprintf("Closed issue %d", 100+i)}
	}
	stub := newDashboardStub(repository.DashboardData{Closed: page, ClosedTotal: 736})
	m := newBoardModel(stub, resolvedBoardKeys(t))
	m.SetSize(120, 25)
	m.columns = []columnData{
		{title: sectionTitleNotReady},
		{title: sectionTitleReady},
		{title: sectionTitleInProgress},
		{title: sectionTitleDone, issues: makeClosedIssues(loaded), total: 736, exact: false},
	}
	m.filterColumns()
	m.doneLoadedCount = loaded
	m.doneClosedTotal = 736
	m.focusedColumn = doneColumnIndex

	// closed-1, closed-10 to closed-19, closed-21 and closed-31: thirteen
	// matches of the 35 loaded.
	typeText(m, "issue 1")
	const matches = 13
	if got := len(m.columns[doneColumnIndex].shown); got != matches {
		t.Fatalf("setup: %d matches, want %d", got, matches)
	}

	// Five matches are left from the cursor's own down: outside the threshold.
	for range matches - loadMoreThreshold {
		testui.DrainCmd(m.Update(tea.KeyMsg{Type: tea.KeyDown}))
	}
	if got := stub.capturedOpts(); len(got) != 0 {
		t.Fatalf("row %d of %d matches loaded a page: %v", m.selectedRow[doneColumnIndex], matches, got)
	}

	// One more row is inside it. Unfiltered, row 9 of 35 would load nothing.
	msgs := testui.DrainCmd(m.Update(tea.KeyMsg{Type: tea.KeyDown}))
	opts := stub.capturedOpts()
	if len(opts) != 1 || opts[0].ClosedOffset != loaded {
		t.Fatalf("Dashboard calls = %v, want one page at offset %d, the end of the loaded rows", opts, loaded)
	}
	selected := m.selectedIssueID()

	for _, msg := range msgs {
		_ = m.Update(msg)
	}
	if got := len(m.columns[doneColumnIndex].issues); got != loaded+len(page) {
		t.Fatalf("%d rows loaded after the page, want %d", got, loaded+len(page))
	}
	if got := len(m.columns[doneColumnIndex].shown); got != matches+len(page) {
		t.Fatalf("%d matches after the page, want %d", got, matches+len(page))
	}
	if m.selectedIssueID() != selected {
		t.Fatalf("the page moved the selection from %q to %q", selected, m.selectedIssueID())
	}
	if view := plainView(m); !strings.Contains(view, fmt.Sprintf(" %d of %d ─", matches+len(page), loaded+len(page))) {
		t.Errorf("the Done header does not count the matches of the loaded rows:\n%s", view)
	}
}

// TestClickLandsOnTheFilteredRowDrawnUnderIt asks the renderer where a match
// is drawn and clicks there: the hit test and the selection read the same
// filtered rows the view does.
func TestClickLandsOnTheFilteredRowDrawnUnderIt(t *testing.T) {
	t.Parallel()

	m, _ := filterBoard(t)
	typeText(m, "login")

	cases := []struct {
		text   string
		column int
		row    int
		id     string
	}{
		{"Login audit", 1, 1, "tm-22"},
		{"tm-40", 3, 0, "tm-40"},
		{"Blocked login migration", 0, 0, "tm-11"},
	}
	for i, tc := range cases {
		cmd := m.Update(mouseAt(t, m, mode.MouseClick, tc.text, i*1000))
		if m.focusedColumn != tc.column || m.selectedRow[tc.column] != tc.row || m.selectedIssueID() != tc.id {
			t.Fatalf("click on %q: column %d row %d selection %q; want column %d row %d %s",
				tc.text, m.focusedColumn, m.selectedRow[m.focusedColumn], m.selectedIssueID(), tc.column, tc.row, tc.id)
		}
		if got := selectionFrom(t, cmd); got != tc.id {
			t.Fatalf("click on %q reported %q, want %s", tc.text, got, tc.id)
		}
	}

	// The pointer on a match marks that row, and no row where a hidden one was.
	_ = m.Update(mouseAt(t, m, mode.MouseMove, "Fix login prompt", 5000))
	if hit := m.hover(m.viewState(0)); hit == nil || hit.Column != 1 || hit.Row != 0 {
		t.Fatalf("hover = %+v, want column 1 row 0", hit)
	}
}

func TestBoardModeActiveQueryGolden(t *testing.T) {
	t.Parallel()

	m, _ := filterBoard(t)
	m.SetSize(120, 20)
	_ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	typeText(m, "login tm-2")

	testui.AssertMatchesGoldenNormalized(t, []byte(m.View(0)), "model_query_active_w120.golden")
}
