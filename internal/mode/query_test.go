package mode_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func TestQueryEditingKeys(t *testing.T) {
	t.Parallel()

	steps := []struct {
		name string
		key  tea.Msg
		want string
		// changed is false for a key the query takes without a new text.
		changed bool
	}{
		{name: "a space before any text is dropped", key: testui.Press("space"), want: ""},
		{name: "rune", key: testui.Runes("f"), want: "f", changed: true},
		{name: "paste of several runes", key: testui.Paste("ix\tthe\nlogin"), want: "fix the login", changed: true},
		{name: "space", key: testui.Press("space"), want: "fix the login ", changed: true},
		{name: "backspace drops the last rune", key: testui.Press("backspace"), want: "fix the login", changed: true},
		{name: "ctrl+w drops the last word", key: testui.Press("ctrl+w"), want: "fix the ", changed: true},
		{name: "ctrl+w drops the word before trailing spaces", key: testui.Press("ctrl+w"), want: "fix ", changed: true},
		{name: "a wide rune is one rune", key: testui.Runes("日本"), want: "fix 日本", changed: true},
		{name: "backspace drops one wide rune", key: testui.Press("backspace"), want: "fix 日", changed: true},
		{name: "ctrl+u clears", key: testui.Press("ctrl+u"), want: "", changed: true},
		{name: "backspace on an empty query", key: testui.Press("backspace"), want: ""},
		{name: "ctrl+w on an empty query", key: testui.Press("ctrl+w"), want: ""},
	}

	var q mode.Query
	for _, step := range steps {
		var consumed, changed bool
		switch msg := step.key.(type) {
		case tea.KeyPressMsg:
			consumed, changed = q.HandleKey(msg)
		case tea.PasteMsg:
			consumed, changed = true, q.Paste(msg.Content)
		}
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

	var q mode.Query
	q.HandleKey(testui.Runes("abc"))

	keys := []tea.KeyPressMsg{
		testui.Press("left"), testui.Press("right"), testui.Press("up"), testui.Press("down"),
		testui.Press("home"), testui.Press("end"), testui.Press("pgup"), testui.Press("pgdown"),
		testui.Press("enter"), testui.Press("esc"), testui.Press("tab"), testui.Press("shift+tab"),
		testui.Press("delete"), testui.Press("ctrl+c"), testui.Press("ctrl+t"),
		testui.Press("alt+r"),
		testui.Press("alt+space"),
		testui.Press("alt+backspace"),
	}
	for _, key := range keys {
		if mode.IsQueryKey(key) {
			t.Errorf("IsQueryKey(%q) = true", key.String())
		}
		if consumed, changed := q.HandleKey(key); consumed || changed || q.Text() != "abc" {
			t.Errorf("%q: consumed %v, changed %v, text %q; the query must leave it", key.String(), consumed, changed, q.Text())
		}
	}
}

func TestQueryHoldsAtMostQueryLimitRunes(t *testing.T) {
	t.Parallel()

	var q mode.Query
	q.HandleKey(testui.Runes(strings.Repeat("é", mode.QueryLimit-1)))
	q.HandleKey(testui.Runes("xyz"))
	if got := []rune(q.Text()); len(got) != mode.QueryLimit || got[mode.QueryLimit-1] != 'x' {
		t.Fatalf("text is %d runes ending in %q, want %d ending in x", len(got), string(got[len(got)-1]), mode.QueryLimit)
	}
	if consumed, changed := q.HandleKey(testui.Runes("z")); !consumed || changed {
		t.Fatalf("a rune past the limit: consumed %v, changed %v; want consumed and unchanged", consumed, changed)
	}
}

func TestQueryClearReportsWhetherItHeldText(t *testing.T) {
	t.Parallel()

	var q mode.Query
	if q.Clear() {
		t.Fatal("Clear() on an empty query reported text")
	}
	q.HandleKey(testui.Runes("abc"))
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
		var q mode.Query
		q.HandleKey(testui.Runes(tc.query))
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

	var q mode.Query
	if got := q.Filter(issues); len(got) != len(issues) {
		t.Fatalf("an empty query kept %d of %d issues", len(got), len(issues))
	}

	q.HandleKey(testui.Runes("filter"))
	got := q.Filter(issues)
	if len(got) != 2 || got[0].ID != "tm-3" || got[1].ID != "tm-2" {
		t.Fatalf("Filter = %v, want tm-3 then tm-2", got)
	}
}
