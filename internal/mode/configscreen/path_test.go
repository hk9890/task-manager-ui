package configscreen

import "testing"

// TestAConfigPathWithALineBreakKeepsTheFrame: the path comes from a flag or
// from the home directory. A newline in it must not add a line to the screen.
func TestAConfigPathWithALineBreakKeepsTheFrame(t *testing.T) {
	t.Parallel()

	view := func(path string) string {
		m := newModel(t, "catppuccin-mocha", "unicode")
		m.Open(path, "catppuccin-mocha", "unicode")
		return m.View("")
	}
	plain, broken := view("/tmp/a b/config.yaml"), view("/tmp/a\nb/config.yaml")
	if broken != plain {
		t.Errorf("a newline in the config path changes the frame:\n%s\nwant\n%s", broken, plain)
	}
}
