package mode

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

func runes(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

func TestQueryEditingKeys(t *testing.T) {
	t.Parallel()

	steps := []struct {
		name string
		key  tea.KeyMsg
		want string
		// changed is false for a key the query takes without a new text.
		changed bool
	}{
		{name: "a space before any text is dropped", key: tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}, want: ""},
		{name: "rune", key: runes("f"), want: "f", changed: true},
		{name: "paste of several runes", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ix\tthe\nlogin"), Paste: true}, want: "fix the login", changed: true},
		{name: "space", key: tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}, want: "fix the login ", changed: true},
		{name: "backspace drops the last rune", key: tea.KeyMsg{Type: tea.KeyBackspace}, want: "fix the login", changed: true},
		{name: "ctrl+w drops the last word", key: tea.KeyMsg{Type: tea.KeyCtrlW}, want: "fix the ", changed: true},
		{name: "ctrl+w drops the word before trailing spaces", key: tea.KeyMsg{Type: tea.KeyCtrlW}, want: "fix ", changed: true},
		{name: "a wide rune is one rune", key: runes("日本"), want: "fix 日本", changed: true},
		{name: "backspace drops one wide rune", key: tea.KeyMsg{Type: tea.KeyBackspace}, want: "fix 日", changed: true},
		{name: "ctrl+u clears", key: tea.KeyMsg{Type: tea.KeyCtrlU}, want: "", changed: true},
		{name: "backspace on an empty query", key: tea.KeyMsg{Type: tea.KeyBackspace}, want: ""},
		{name: "ctrl+w on an empty query", key: tea.KeyMsg{Type: tea.KeyCtrlW}, want: ""},
	}

	var q Query
	for _, step := range steps {
		consumed, changed := q.HandleKey(step.key)
		if !consumed || changed != step.changed || q.Text() != step.want {
			t.Fatalf("%s: consumed %v, changed %v, text %q; want consumed, changed %v, text %q",
				step.name, consumed, changed, q.Text(), step.changed, step.want)
		}
		if q.Empty() != (step.want == "") {
			t.Fatalf("%s: Empty() = %v with text %q", step.name, q.Empty(), q.Text())
		}
	}
}

// TestQueryLeavesEveryOtherKeyToTheSurface: the arrows move the list, enter
// opens, esc is the shell's, and an alt chord is an action.
func TestQueryLeavesEveryOtherKeyToTheSurface(t *testing.T) {
	t.Parallel()

	var q Query
	q.HandleKey(runes("abc"))

	keys := []tea.KeyMsg{
		{Type: tea.KeyLeft}, {Type: tea.KeyRight}, {Type: tea.KeyUp}, {Type: tea.KeyDown},
		{Type: tea.KeyHome}, {Type: tea.KeyEnd}, {Type: tea.KeyPgUp}, {Type: tea.KeyPgDown},
		{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyTab}, {Type: tea.KeyShiftTab},
		{Type: tea.KeyDelete}, {Type: tea.KeyCtrlC}, {Type: tea.KeyCtrlT},
		{Type: tea.KeyRunes, Runes: []rune("r"), Alt: true},
		{Type: tea.KeySpace, Runes: []rune{' '}, Alt: true},
		{Type: tea.KeyBackspace, Alt: true},
	}
	for _, key := range keys {
		if IsQueryKey(key) {
			t.Errorf("IsQueryKey(%q) = true", key.String())
		}
		if consumed, changed := q.HandleKey(key); consumed || changed || q.Text() != "abc" {
			t.Errorf("%q: consumed %v, changed %v, text %q; the query must leave it", key.String(), consumed, changed, q.Text())
		}
	}
}

func TestQueryHoldsAtMostQueryLimitRunes(t *testing.T) {
	t.Parallel()

	var q Query
	q.HandleKey(runes(strings.Repeat("é", QueryLimit-1)))
	q.HandleKey(runes("xyz"))
	if got := []rune(q.Text()); len(got) != QueryLimit || got[QueryLimit-1] != 'x' {
		t.Fatalf("text is %d runes ending in %q, want %d ending in x", len(got), string(got[len(got)-1]), QueryLimit)
	}
	if consumed, changed := q.HandleKey(runes("z")); !consumed || changed {
		t.Fatalf("a rune past the limit: consumed %v, changed %v; want consumed and unchanged", consumed, changed)
	}
}

func TestQueryClearReportsWhetherItHeldText(t *testing.T) {
	t.Parallel()

	var q Query
	if q.Clear() {
		t.Fatal("Clear() on an empty query reported text")
	}
	q.HandleKey(runes("abc"))
	if !q.Clear() || !q.Empty() {
		t.Fatalf("Clear() left %q", q.Text())
	}
}

func TestQueryMatchesEveryWordInTitleOrID(t *testing.T) {
	t.Parallel()

	issue := domain.IssueSummary{ID: "TM-42x", Title: "Fix the Login prompt"}
	cases := []struct {
		query string
		want  bool
	}{
		{"", true},
		{"login", true},
		{"LOGIN", true},
		{"gin pro", true},       // substrings, in any order
		{"prompt fix", true},    // every word, order free
		{"tm-42", true},         // the ID, whatever the case
		{"login 42X", true},     // one word in the title, one in the ID
		{"login logout", false}, // every word must match
		{"fixthe", false},       // a word does not span a space
		{"bug", false},          // the type is not searched
	}
	for _, tc := range cases {
		var q Query
		q.HandleKey(runes(tc.query))
		if got := q.Matches(issue); got != tc.want {
			t.Errorf("query %q: Matches = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestQueryFilterKeepsTheOrder(t *testing.T) {
	t.Parallel()

	issues := []domain.IssueSummary{
		{ID: "tm-3", Title: "Board filter"},
		{ID: "tm-1", Title: "Docs"},
		{ID: "tm-2", Title: "Filter docs"},
	}

	var q Query
	if got := q.Filter(issues); len(got) != len(issues) {
		t.Fatalf("an empty query kept %d of %d issues", len(got), len(issues))
	}

	q.HandleKey(runes("filter"))
	got := q.Filter(issues)
	if len(got) != 2 || got[0].ID != "tm-3" || got[1].ID != "tm-2" {
		t.Fatalf("Filter = %v, want tm-3 then tm-2", got)
	}
}
