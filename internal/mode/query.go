package mode

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// QueryLimit is the longest query a surface holds, in runes.
const QueryLimit = 64

// Query is the always-live filter text of a list surface. No key enters it:
// every printable key the surface receives is typed into it, which is why no
// action outside a modal is bound to one.
//
// It has no cursor. The text is edited at its end only, so the arrow keys stay
// with the list.
type Query struct {
	text string
	// words is text split on whitespace and lower-cased, kept so a filter pass
	// does not split the text again for every row.
	words []string
}

// Text is the query as typed. It never starts with whitespace, so a non-empty
// text holds at least one word.
func (q Query) Text() string {
	return q.text
}

// Empty reports whether there is nothing to filter by and nothing to clear.
func (q Query) Empty() bool {
	return q.text == ""
}

// IsQueryKey reports whether msg edits a query: a key that types text, space
// included, without alt, and backspace, ctrl+w and ctrl+u. A browse surface
// answers Browse.TakesKey with it, so a key the query took runs no shell
// action.
func IsQueryKey(msg tea.KeyPressMsg) bool {
	if msg.Mod.Contains(tea.ModAlt) {
		return false
	}
	if msg.Text != "" {
		return true
	}
	switch msg.Keystroke() {
	case "backspace", "ctrl+w", "ctrl+u":
		return true
	}
	return false
}

// HandleKey applies msg when it is a query key. consumed reports that the key
// was the query's, changed that the text is different afterwards.
func (q *Query) HandleKey(msg tea.KeyPressMsg) (consumed, changed bool) {
	if !IsQueryKey(msg) {
		return false, false
	}

	before := q.text
	switch msg.Keystroke() {
	case "backspace":
		if runes := []rune(q.text); len(runes) > 0 {
			q.set(string(runes[:len(runes)-1]))
		}
	case "ctrl+w":
		word := strings.TrimRight(q.text, " ")
		q.set(word[:strings.LastIndex(word, " ")+1])
	case "ctrl+u":
		q.set("")
	default:
		q.set(q.text + msg.Text)
	}
	return true, q.text != before
}

// Paste types pasted text at the end of the query, and reports whether the
// text is different afterwards.
func (q *Query) Paste(text string) (changed bool) {
	before := q.text
	q.set(q.text + text)
	return q.text != before
}

// PlainKey reports whether msg is the key code with no modifier but alt, as a
// surface that matches a key itself rather than through a binding means it.
func PlainKey(msg tea.KeyPressMsg, code rune) bool {
	return msg.Code == code && msg.Mod&^tea.ModAlt == 0
}

// Clear empties the query and reports whether it held text.
func (q *Query) Clear() bool {
	if q.Empty() {
		return false
	}
	q.set("")
	return true
}

// set stores text as one line of at most QueryLimit runes. A paste can carry
// line breaks and tabs; each becomes a space.
func (q *Query) set(text string) {
	text = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, text)
	text = strings.TrimLeft(text, " ")
	if runes := []rune(text); len(runes) > QueryLimit {
		text = string(runes[:QueryLimit])
	}
	q.text = text
	q.words = strings.Fields(strings.ToLower(text))
}

// Matches reports whether every word of the query is in the issue's title or
// its ID, whatever the case. An empty query matches every issue.
func (q Query) Matches(issue domain.IssueSummary) bool {
	title, id := strings.ToLower(issue.Title), strings.ToLower(issue.ID)
	for _, word := range q.words {
		if !strings.Contains(title, word) && !strings.Contains(id, word) {
			return false
		}
	}
	return true
}

// Filter returns the issues the query matches, in the order they came. It is
// issues itself for an empty query.
func (q Query) Filter(issues []domain.IssueSummary) []domain.IssueSummary {
	if q.Empty() {
		return issues
	}
	matched := make([]domain.IssueSummary, 0, len(issues))
	for _, issue := range issues {
		if q.Matches(issue) {
			matched = append(matched, issue)
		}
	}
	return matched
}
