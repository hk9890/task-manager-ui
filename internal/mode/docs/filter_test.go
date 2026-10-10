package docs

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// filterDocs is three loaded docs; "notes" matches the last two.
func filterDocs(t *testing.T) (*Model, *fakes.TrackedRepository) {
	t.Helper()
	gw := fakes.NewTracked()
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-1", Title: "Auth redesign", Status: "open", Type: "doc", Priority: 2})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-2", Title: "Session notes", Status: "closed", Type: "doc", Priority: 2})
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-3", Title: "Release NOTES", Status: "open", Type: "doc", Priority: 2})
	return loadedModel(t, gw), gw
}

func typeText(m *Model, text string) tea.Cmd {
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
}

func shownIDs(m *Model) string {
	ids := make([]string, len(m.shown))
	for i, issue := range m.shown {
		ids[i] = issue.ID
	}
	return strings.Join(ids, ",")
}

func TestDocsFilterNarrowsTheColumnAndCountsMatchesOfLoaded(t *testing.T) {
	m, _ := filterDocs(t)
	if got := shownIDs(m); got != "tm-1,tm-2,tm-3" {
		t.Fatalf("setup: the column shows %q", got)
	}
	view := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	if lines := strings.Split(view, "\n"); !strings.HasPrefix(lines[1], " ❯ filter docs") {
		t.Fatalf("the docs tab does not open on its query line:\n%s", view)
	}

	typeText(m, "notes")
	if got := shownIDs(m); got != "tm-2,tm-3" {
		t.Fatalf("the filtered column shows %q, want tm-2,tm-3", got)
	}
	view = testui.AnsiEscapePattern.ReplaceAllString(m.View(0), "")
	for _, want := range []string{"❯ notes", " 2 of 3 ─", "Session notes", "Release NOTES"} {
		if !strings.Contains(view, want) {
			t.Errorf("the filtered column does not draw %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Auth redesign") {
		t.Errorf("the filtered column still draws a doc that does not match:\n%s", view)
	}

	// The ID is searched too, and every word must match.
	typeText(m, " tm-3")
	if got := shownIDs(m); got != "tm-3" {
		t.Fatalf("the filtered column shows %q, want tm-3", got)
	}
}

func TestDocsFilterKeepsTheSelectionOnItsDoc(t *testing.T) {
	m, _ := filterDocs(t)
	_ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.selectedIssueID() != "tm-3" {
		t.Fatalf("setup: selection %q, want tm-3", m.selectedIssueID())
	}

	if cmd := typeText(m, "notes"); cmd != nil {
		t.Fatal("the selection did not change, so the query must report none")
	}
	if m.selectedIssueID() != "tm-3" || m.list.SelectedRow != 1 {
		t.Fatalf("selection %q on row %d, want tm-3 on row 1", m.selectedIssueID(), m.list.SelectedRow)
	}
	assertSelectionDrawn(t, m)

	// tm-3 does not match: the first match takes the selection and the shell
	// is told.
	_ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	cmd := typeText(m, "sign")
	if m.selectedIssueID() != "tm-1" {
		t.Fatalf("selection %q, want the first match tm-1", m.selectedIssueID())
	}
	changed, ok := cmd().(mode.SelectionChangedMsg)
	if !ok || changed.Mode != mode.Docs || changed.Selection == nil || changed.Selection.Issue.ID != "tm-1" {
		t.Fatalf("the query reported %#v, want the docs selection tm-1", cmd())
	}

	// No match at all leaves no selection.
	typeText(m, "zzz")
	if m.currentSelection() != nil {
		t.Fatalf("a query nothing matches left the selection %#v", m.currentSelection())
	}
	if view := testui.AnsiEscapePattern.ReplaceAllString(m.View(0), ""); !strings.Contains(view, "(no matches)") || !strings.Contains(view, " 0 of 3 ─") {
		t.Errorf("an emptied column must say so:\n%s", view)
	}
}

func TestDocsClearQueryRestoresTheColumn(t *testing.T) {
	m, _ := filterDocs(t)
	if cleared, _ := m.ClearQuery(); cleared {
		t.Fatal("ClearQuery() on an empty query reported a clear")
	}

	typeText(m, "notes")
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	cleared, cmd := m.ClearQuery()
	if !cleared || cmd != nil {
		t.Fatalf("ClearQuery() = %v with a selection report %v; want cleared and no report", cleared, cmd != nil)
	}
	if got := shownIDs(m); got != "tm-1,tm-2,tm-3" {
		t.Fatalf("after the clear the column shows %q", got)
	}
	if m.selectedIssueID() != "tm-3" || m.list.SelectedRow != 2 {
		t.Fatalf("after the clear: selection %q on row %d, want tm-3 on row 2", m.selectedIssueID(), m.list.SelectedRow)
	}
}

func TestDocsFilterSurvivesAutoRefreshAndReload(t *testing.T) {
	m, gw := filterDocs(t)
	typeText(m, "notes")
	_ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// A new match sorts ahead of the selected doc.
	gw.Memory.Seed(memoryrepo.Issue{ID: "tm-0", Title: "Handover notes", Status: "open", Type: "doc", Priority: 2})
	resolve(t, m, m.AutoRefresh())
	if m.query.Text() != "notes" || shownIDs(m) != "tm-0,tm-2,tm-3" {
		t.Fatalf("after the auto refresh: query %q, column %q", m.query.Text(), shownIDs(m))
	}
	if m.selectedIssueID() != "tm-3" {
		t.Fatalf("the auto refresh moved the selection to %q", m.selectedIssueID())
	}

	resolve(t, m, m.Reload())
	if m.query.Text() != "notes" || shownIDs(m) != "tm-0,tm-2,tm-3" {
		t.Fatalf("after the reload: query %q, column %q", m.query.Text(), shownIDs(m))
	}
	if m.selectedIssueID() != "tm-0" {
		t.Fatalf("the reload left the selection on %q, want the first match", m.selectedIssueID())
	}
}

// TestDocsClickLandsOnTheFilteredRowDrawnUnderIt asks the renderer where a
// match is drawn and clicks there.
func TestDocsClickLandsOnTheFilteredRowDrawnUnderIt(t *testing.T) {
	m, _ := filterDocs(t)
	typeText(m, "notes")

	x, y := testui.FindCell(t, m.View(0), "Release NOTES")
	cmd := m.Update(mode.MouseMsg{Kind: mode.MouseClick, X: x, Y: y, At: mouseStart})
	if m.selectedIssueID() != "tm-3" || cmd == nil {
		t.Fatalf("the click selected %q (reported %v), want tm-3", m.selectedIssueID(), cmd != nil)
	}
}
