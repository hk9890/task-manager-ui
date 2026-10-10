package board

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

func plain(s string) string {
	return testui.AnsiEscapePattern.ReplaceAllString(s, "")
}

func TestRenderQueryLine(t *testing.T) {
	t.Parallel()

	prompt := styles.Glyphs.Prompt + " "
	cases := []struct {
		name  string
		query string
		width int
		want  string
	}{
		{name: "empty shows the placeholder", width: 40, want: prompt + "filter issues"},
		{name: "empty and narrow cuts the placeholder", width: 9, want: prompt + "filter…"},
		// The trailing cell is the cursor block.
		{name: "typed text then the cursor", query: "login", width: 40, want: prompt + "login "},
		{name: "a trailing space is kept", query: "login ", width: 40, want: prompt + "login  "},
		{name: "text that just fits", query: "abcdefg", width: 10, want: prompt + "abcdefg "},
		{name: "long text is cut from the front", query: "abcdefgh", width: 10, want: prompt + "…cdefgh "},
	}
	for _, tc := range cases {
		line := renderQueryLine(tc.query, "filter issues", tc.width)
		if got := plain(line); got != tc.want {
			t.Errorf("%s: line %q, want %q", tc.name, got, tc.want)
		}
		if got := lipgloss.Width(line); got > tc.width {
			t.Errorf("%s: line is %d cells wide, over the %d it has", tc.name, got, tc.width)
		}
	}
}

// TestRenderQueryLineDrawsPromptCursorAndPlaceholderInTheirRoles is not
// parallel: it renders in colour.
func TestRenderQueryLineDrawsPromptCursorAndPlaceholderInTheirRoles(t *testing.T) {
	testui.ForceTrueColor(t)

	accent := lipgloss.NewStyle().Foreground(styles.QueryAccentColor)
	muted := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	prompt := accent.Render(styles.Glyphs.Prompt + " ")

	empty := renderQueryLine("", "filter issues", 40)
	if want := prompt + accent.Reverse(true).Render("f") + muted.Render("ilter issues"); empty != want {
		t.Errorf("empty line %q, want %q", empty, want)
	}

	typed := renderQueryLine("abc", "filter issues", 40)
	if !strings.HasPrefix(typed, prompt) || !strings.HasSuffix(typed, accent.Reverse(true).Render(" ")) {
		t.Errorf("typed line %q does not start with the prompt and end with the cursor block", typed)
	}
}

func queryState(query string) State {
	issue := func(id, title string) domain.IssueSummary {
		return domain.IssueSummary{ID: id, Title: title, Type: "task", Status: "open", Priority: 2}
	}
	return State{
		Query:       query,
		Placeholder: "filter issues",
		Width:       180,
		Height:      16,
		Columns: []Column{
			{Title: "Not Ready", SelectedRow: -1, Total: 4, TotalIsExact: true, Loaded: 4},
			{Title: "Ready", SelectedRow: 0, Total: 9, TotalIsExact: true, Loaded: 9, Rows: []domain.IssueSummary{
				issue("tm-login1", "Fix the login prompt"),
				issue("tm-7", "Login page loses the redirect target after a timeout"),
			}},
			{Title: "In Progress", SelectedRow: -1, TotalIsExact: true},
			// Done is paged: 30 of its 200 rows are in memory.
			{Title: "Done", SelectedRow: -1, Total: 200, Loaded: 30, Rows: []domain.IssueSummary{
				issue("tm-3", "Login audit"),
			}},
		},
		FocusedColumn: 1,
	}
}

// TestHeaderCountsMatchesOfLoadedWhileAQueryIsActive: with a query every
// header reads "N of M", the rows the query chose of the rows in memory.
func TestHeaderCountsMatchesOfLoadedWhileAQueryIsActive(t *testing.T) {
	t.Parallel()

	view := plain(Render(queryState("login")))
	for _, header := range []string{"Not Ready ", " 0 of 4 ─", " 2 of 9 ─", " 0 of 0 ─", " 1 of 30 ─"} {
		if !strings.Contains(view, header) {
			t.Errorf("header %q is not drawn:\n%s", header, view)
		}
	}
	// A column the query emptied says so; one that had no rows says that.
	if strings.Count(view, "(no matches)") != 1 || strings.Count(view, "(no issues)") != 1 {
		t.Errorf("want one (no matches) column and one (no issues) column:\n%s", view)
	}

	// Without a query the same columns count as they did.
	view = plain(Render(queryState("")))
	for _, header := range []string{" 4 ─", " 9 ─", " 0 ─", " 1 of 200 ─"} {
		if !strings.Contains(view, header) {
			t.Errorf("without a query, header %q is not drawn:\n%s", header, view)
		}
	}
	if strings.Contains(view, "(no matches)") {
		t.Errorf("without a query no column says (no matches):\n%s", view)
	}
}

func TestRenderBoardWithActiveQueryGolden(t *testing.T) {
	t.Parallel()

	view := Render(queryState("login"))
	assertEqualColumnHeights(t, view)
	testui.AssertMatchesGoldenStripANSI(t, []byte(view), "board_query_active_w180.golden")
}

// TestQueryLineKeepsTheColumnWindowBesideIt: a board too narrow for its
// columns still says which ones are drawn, after the query.
func TestQueryLineKeepsTheColumnWindowBesideIt(t *testing.T) {
	t.Parallel()

	state := queryState("login")
	state.Width = 80
	view := Render(state)
	head := strings.Split(plain(view), "\n")[1]
	if want := " " + styles.Glyphs.Prompt + " login  · cols 1-2/4"; head != want {
		t.Errorf("query line %q, want %q", head, want)
	}
	if got := lipgloss.Width(head); got > state.Width {
		t.Errorf("query line is %d cells wide, over %d", got, state.Width)
	}
}

// TestHeadIsARuleOverTheIndentedQueryLine pins the two lines above the
// columns, and that a click on either lands on no column.
func TestHeadIsARuleOverTheIndentedQueryLine(t *testing.T) {
	t.Parallel()

	state := queryState("login")
	view := Render(state)
	lines := strings.Split(plain(view), "\n")
	if want := plain(styles.Rule(state.Width)); lines[0] != want {
		t.Errorf("first line %q, want the rule %q", lines[0], want)
	}
	if want := " " + styles.Glyphs.Prompt + " login "; lines[1] != want {
		t.Errorf("second line %q, want the query line %q", lines[1], want)
	}
	if !strings.HasPrefix(lines[2], "╭─ Not Ready ") {
		t.Errorf("third line %q, want the top border of the first column", lines[2])
	}

	x, _ := testui.FindCell(t, view, "Fix the login prompt")
	_, ruleY := testui.FindCell(t, view, lines[0])
	_, queryY := testui.FindCell(t, view, lines[1])
	for _, y := range []int{ruleY, queryY} {
		if hit, ok := HitTest(state, x, y); ok {
			t.Errorf("HitTest on head line %d = %+v, want no hit", y, hit)
		}
	}
	if hit, ok := HitTest(state, x, queryY+1); !ok || hit.Column != 1 || hit.Row != -1 {
		t.Errorf("HitTest on the column border under the head = %+v, %v; want column 1 with no row", hit, ok)
	}
}

// TestEmptyColumnSaysWhetherTheQueryEmptiedIt: a filter that leaves a column
// empty is a miss only when the column had rows, and a store search that
// returns nothing for a typed query is always one.
func TestEmptyColumnSaysWhetherTheQueryEmptiedIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		col    Column
		query  string
		search bool
		want   string
	}{
		{name: "no query", want: "(no issues)"},
		{name: "filter on a column with no rows", query: "login", want: "(no issues)"},
		{name: "filter that emptied a column", col: Column{Loaded: 3}, query: "login", want: "(no matches)"},
		{name: "filter that emptied a column whose rows reload", col: Column{Loaded: 3, Loading: true}, query: "login", want: "(no matches)"},
		{name: "search with no query", search: true, want: "(no issues)"},
		{name: "search that found nothing", query: "login", search: true, want: "(no matches)"},
	}
	for _, tc := range cases {
		rows := renderColumnRows(tc.col, 40, 0, 0, time.Time{}, -1, tc.query, tc.search).rows
		if len(rows) != 1 || rows[0] != tc.want {
			t.Errorf("%s: rows %q, want [%q]", tc.name, rows, tc.want)
		}
	}
}
