package issuerow

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// matchedText returns, in order, every run a line drew in the match style.
func matchedText(t *testing.T, line string) []string {
	t.Helper()

	open, _, _ := strings.Cut(styles.MatchTextStyle.Render("x"), "x")
	if !strings.HasPrefix(open, "\x1b[") {
		t.Fatalf("the match style draws no escape: %q", styles.MatchTextStyle.Render("x"))
	}
	var runs []string
	for _, part := range strings.Split(line, open)[1:] {
		run, _, _ := strings.Cut(part, "\x1b[")
		runs = append(runs, run)
	}
	return runs
}

// TestRenderCompactMarksEveryOccurrenceOfAQueryWord: a row marks the words
// that chose it in its title and its ID, whatever the case, and the marking
// moves no cell.
func TestRenderCompactMarksEveryOccurrenceOfAQueryWord(t *testing.T) {
	testui.ForceTrueColor(t)

	issue := domain.IssueSummary{ID: "tm-log7", Title: "Login dialog logs the LOGIN twice", Type: "task", Status: "open", Priority: 1}
	config := RenderConfig{Issue: issue, Width: 60, Styled: true}
	unmarked := RenderCompact(config)
	plain := plainLines(unmarked)

	config.Match = []string{"LOG", "tm-"}
	marked := RenderCompact(config)

	if got := plainLines(marked); got[0] != plain[0] || got[1] != plain[1] {
		t.Fatalf("marking changed the text of the row:\n got %q\nwant %q", got, plain)
	}
	for i, line := range marked {
		if lipgloss.Width(line) != lipgloss.Width(unmarked[i]) {
			t.Errorf("line %d is %d cells wide marked and %d unmarked", i, lipgloss.Width(line), lipgloss.Width(unmarked[i]))
		}
	}
	if got := matchedText(t, marked[0]); strings.Join(got, "|") != "Log|log|log|LOG" {
		t.Errorf("title marks %q, want Log, the log of dialog, log and LOG", got)
	}
	if got := matchedText(t, marked[1]); strings.Join(got, "|") != "tm-log" {
		t.Errorf("ID marks %q, want the adjoining tm- and log as one run", got)
	}
	if got := matchedText(t, strings.Join(unmarked, "\n")); len(got) != 0 {
		t.Errorf("a row with no query marks %q", got)
	}
}

// TestRenderCompactMarksATruncatedTitleInWhatIsDrawn: the title is cut first
// and marked after, so the ellipsis stays where it was and only a word that is
// drawn whole is marked.
func TestRenderCompactMarksATruncatedTitleInWhatIsDrawn(t *testing.T) {
	testui.ForceTrueColor(t)

	issue := domain.IssueSummary{ID: "tm-1", Title: "Board filter keeps the filter text", Type: "task", Status: "open", Priority: 2}
	// 2 cells of gutter, the type and a space, then 16 cells of title.
	config := RenderConfig{Issue: issue, Width: 20, Styled: true}
	unmarked := RenderCompact(config)
	plain := plainLines(unmarked)
	if !strings.Contains(plain[0], "Board filter ke…") {
		t.Fatalf("setup: the title is not cut where the test expects: %q", plain[0])
	}

	for _, tc := range []struct {
		word string
		want string
	}{
		{word: "filter", want: "filter"}, // the second occurrence is cut off
		{word: "keeps", want: ""},        // cut through the match
		{word: "text", want: ""},         // wholly cut off
	} {
		config.Match = []string{tc.word}
		marked := RenderCompact(config)
		if got := plainLines(marked); got[0] != plain[0] {
			t.Errorf("%q: marking changed the cut title: %q, want %q", tc.word, got[0], plain[0])
		}
		if lipgloss.Width(marked[0]) != lipgloss.Width(unmarked[0]) {
			t.Errorf("%q: the first line is %d cells wide, want %d", tc.word, lipgloss.Width(marked[0]), lipgloss.Width(unmarked[0]))
		}
		if got := strings.Join(matchedText(t, marked[0]), "|"); got != tc.want {
			t.Errorf("%q: title marks %q, want %q", tc.word, got, tc.want)
		}
	}
}

// TestRenderCompactPlainRowIgnoresTheQuery: the plain row is what width math
// reads, so it carries no escape with or without a query.
func TestRenderCompactPlainRowIgnoresTheQuery(t *testing.T) {
	testui.ForceTrueColor(t)

	issue := domain.IssueSummary{ID: "tm-1", Title: "Board filter", Type: "task", Status: "open", Priority: 2}
	for _, line := range RenderCompact(RenderConfig{Issue: issue, Width: 40, Match: []string{"filter"}}) {
		if strings.Contains(line, "\x1b[") {
			t.Errorf("a plain row carries an escape: %q", line)
		}
	}
}
