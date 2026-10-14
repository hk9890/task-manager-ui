package detail

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestADetailErrorWithALineBreakKeepsItsLines: the error text is the store's
// own and can embed a path. Its control characters add no line.
func TestADetailErrorWithALineBreakKeepsItsLines(t *testing.T) {
	t.Parallel()

	render := func(message string) string {
		return Render(State{SelectionID: "tm-1", Error: message, Width: 80, Height: 20})
	}
	if plain, broken := render("open /tmp/a b: no such file"), render("open /tmp/a\nb:\tno such file"); broken != plain {
		t.Errorf("control characters in a detail error change the text:\n%q\nwant\n%q", broken, plain)
	}
}

// TestALongDetailErrorWrapsToTheView: nothing below the renderer wraps, so an
// error longer than the view would lose its end at the terminal edge.
func TestALongDetailErrorWrapsToTheView(t *testing.T) {
	t.Parallel()

	const width = 80
	words := make([]string, 60)
	for idx := range words {
		words[idx] = "word" + string(rune('a'+idx%26))
	}
	message := strings.Join(words, " ")

	view := Render(State{SelectionID: "tm-1", Error: message, Width: width, Height: 20})
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("a line of the error is %d cells, want at most %d: %q", got, width, line)
		}
	}
	if got := strings.Join(strings.Fields(view), " "); !strings.HasSuffix(got, "Error: "+message) {
		t.Errorf("the wrapped error lost text:\n%s", view)
	}

	// The line that names the issue is as long as the issue's ID.
	const narrow = 30
	view = Render(State{SelectionID: "a-long-prefix-of-an-id-x1y2z3", Error: "no such file", Width: narrow, Height: 20})
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > narrow {
			t.Errorf("a line of the error is %d cells, want at most %d: %q", got, narrow, line)
		}
	}
}
