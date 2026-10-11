package toaster

import (
	"fmt"
	"strings"
	"testing"
)

// TestToastDrawsAtMostItsLineLimit: a message carries text from outside, and
// each line feed in it is a line of the box. A message over the limit ends
// with a mark on a line of its own; one at the limit is drawn whole.
func TestToastDrawsAtMostItsLineLimit(t *testing.T) {
	t.Parallel()

	message := func(count int) string {
		lines := make([]string, count)
		for idx := range lines {
			lines[idx] = fmt.Sprintf("line-%02d", idx+1)
		}
		return strings.Join(lines, "\n")
	}
	const borderLines = 2

	for _, tc := range []struct {
		lines, drawn int
		cut          bool
	}{
		{lines: 1, drawn: 1},
		{lines: maxMessageLines, drawn: maxMessageLines},
		{lines: maxMessageLines + 1, drawn: maxMessageLines + 1, cut: true},
		{lines: 40, drawn: maxMessageLines + 1, cut: true},
	} {
		view := New().Show(message(tc.lines), StyleError).View()
		rows := strings.Split(view, "\n")
		if got := len(rows) - borderLines; got != tc.drawn {
			t.Errorf("a message of %d lines draws %d, want %d:\n%s", tc.lines, got, tc.drawn, view)
		}
		kept := min(tc.lines, maxMessageLines)
		if !strings.Contains(view, fmt.Sprintf("line-%02d", kept)) || strings.Contains(view, fmt.Sprintf("line-%02d", kept+1)) {
			t.Errorf("a message of %d lines does not end at line %d:\n%s", tc.lines, kept, view)
		}
		if got := strings.Contains(rows[len(rows)-2], "…"); got != tc.cut {
			t.Errorf("a message of %d lines: cut mark drawn %v, want %v:\n%s", tc.lines, got, tc.cut, view)
		}
	}
}
