package detail

import "testing"

// TestADetailErrorWithALineBreakKeepsItsLines: the error text is the store's
// own and can embed a path. It is drawn on the one line after "Error:".
func TestADetailErrorWithALineBreakKeepsItsLines(t *testing.T) {
	t.Parallel()

	render := func(message string) string {
		return Render(State{SelectionID: "tm-1", Error: message, Width: 80, Height: 20})
	}
	if plain, broken := render("open /tmp/a b: no such file"), render("open /tmp/a\nb:\tno such file"); broken != plain {
		t.Errorf("control characters in a detail error change the text:\n%q\nwant\n%q", broken, plain)
	}
}
