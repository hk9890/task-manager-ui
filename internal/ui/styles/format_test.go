package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestTruncateString(t *testing.T) {
	if got := TruncateString("hello", 10); got != "hello" {
		t.Fatalf("unexpected non-truncated value: %q", got)
	}
	if got := TruncateString("hello world", 5); got != "he..." {
		t.Fatalf("unexpected truncated value: %q", got)
	}
}

func TestSelectionPrefix(t *testing.T) {
	t.Run("idle prefix", func(t *testing.T) {
		plain, rendered := SelectionPrefix(false, true)
		if plain != "  " || rendered != "  " {
			t.Fatalf("expected idle gutter prefixes to be two spaces, got plain=%q rendered=%q", plain, rendered)
		}
	})

	t.Run("selected unstyled", func(t *testing.T) {
		plain, rendered := SelectionPrefix(true, false)
		if plain != "› " || rendered != "› " {
			t.Fatalf("expected unstyled selected prefix, got plain=%q rendered=%q", plain, rendered)
		}
	})

	t.Run("selected styled", func(t *testing.T) {
		previousProfile := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(termenv.TrueColor)
		t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

		plain, rendered := SelectionPrefix(true, true)
		if plain != "› " {
			t.Fatalf("expected plain selected prefix to stay canonical, got %q", plain)
		}
		if !strings.Contains(rendered, "\x1b[") {
			t.Fatalf("expected styled selected prefix to include ANSI, got %q", rendered)
		}
		if lipgloss.Width(rendered) != 2 {
			t.Fatalf("expected styled selected prefix display width 2, got %d (%q)", lipgloss.Width(rendered), rendered)
		}
	})
}
