package board

import "testing"

// TestAColumnErrorWithALineBreakKeepsTheFrame: the error text is the store's
// own and can embed a path. It is drawn on the one pinned row.
func TestAColumnErrorWithALineBreakKeepsTheFrame(t *testing.T) {
	t.Parallel()

	render := func(message string) string {
		return Render(State{
			Width: 80, Height: 11,
			Columns: []Column{{Title: "Ready", Error: message}},
		})
	}
	if plain, broken := render("open /tmp/a b: no such file"), render("open /tmp/a\nb:\tno such file"); broken != plain {
		t.Errorf("control characters in a column error change the frame:\n%s\nwant\n%s", broken, plain)
	}
}
