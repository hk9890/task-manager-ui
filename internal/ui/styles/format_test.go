package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

func TestTruncateStringPreservesWellFormedANSI(t *testing.T) {
	t.Parallel()

	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(strings.Repeat("x", 64))
	truncated := TruncateString(styled, 18)

	if got := lipgloss.Width(truncated); got != 18 {
		t.Fatalf("expected display width 18, got %d", got)
	}

	stripped := ansi.Strip(truncated)
	if strings.Contains(stripped, "\x1b") {
		t.Fatalf("expected no dangling/incomplete ANSI escapes after truncation, got %q", truncated)
	}

	if !strings.HasSuffix(stripped, "...") {
		t.Fatalf("expected ellipsis suffix after truncation, got %q", stripped)
	}
}

func TestWrapLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		maxWidth int
		wantLen  int    // expected number of returned lines
		wantLine string // substring that must appear in the first line (empty = no check)
	}{
		{
			name:     "short string fits in one line",
			input:    "hello",
			maxWidth: 20,
			wantLen:  1,
			wantLine: "hello",
		},
		{
			name:     "empty string returns one line",
			input:    "",
			maxWidth: 20,
			wantLen:  1,
			wantLine: "",
		},
		{
			name:     "maxWidth below 1 returns single empty line",
			input:    "hello",
			maxWidth: 0,
			wantLen:  1,
			wantLine: "",
		},
		{
			name:     "maxWidth of -1 returns single empty line",
			input:    "something long",
			maxWidth: -1,
			wantLen:  1,
			wantLine: "",
		},
		{
			name:     "long string wraps to multiple lines",
			input:    "the quick brown fox jumps over the lazy dog",
			maxWidth: 15,
			wantLen:  3, // "the quick brown", "fox jumps over", "the lazy dog" (actual split depends on ansi.Wrap)
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := WrapLines(tc.input, tc.maxWidth)
			if len(got) == 0 {
				t.Fatalf("WrapLines(%q, %d) returned empty slice", tc.input, tc.maxWidth)
			}
			if tc.wantLen > 0 && len(got) != tc.wantLen {
				t.Errorf("WrapLines(%q, %d) returned %d lines, want %d: %v",
					tc.input, tc.maxWidth, len(got), tc.wantLen, got)
			}
			if tc.wantLine != "" && got[0] != tc.wantLine {
				t.Errorf("WrapLines(%q, %d) first line = %q, want %q",
					tc.input, tc.maxWidth, got[0], tc.wantLine)
			}
		})
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
