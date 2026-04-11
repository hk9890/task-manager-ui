package markdown

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

var ansiStripPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Test strategy note:
// These tests avoid fragile full-screen ANSI goldens. We assert focused behavior:
// width-awareness, deterministic fallbacks, and readable output after ANSI strip.
func TestRenderReadOnlyReturnsEmptyFallbackForBlankInput(t *testing.T) {
	t.Parallel()

	r := NewRenderer()
	got := r.RenderReadOnly(" \n\t\n", 80)
	if got != DefaultEmptyFallback {
		t.Fatalf("expected empty fallback %q, got %q", DefaultEmptyFallback, got)
	}
}

func TestRenderReadOnlyRespectsCustomEmptyFallback(t *testing.T) {
	t.Parallel()

	r := Renderer{EmptyFallback: "(nothing here)"}
	got := r.RenderReadOnly("\n\n", 80)
	if got != "(nothing here)" {
		t.Fatalf("expected custom empty fallback, got %q", got)
	}
}

func TestRenderReadOnlyUsesPlainFallbackForEffectivelyPlainText(t *testing.T) {
	t.Parallel()

	r := NewRenderer()
	got := r.RenderReadOnly("alpha beta gamma delta", 6)
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected plain fallback without ANSI, got %q", got)
	}
	if got != "alpha\nbeta g\namma d\nelta" {
		t.Fatalf("expected deterministic plain wrapping, got %q", got)
	}
}

func TestRenderReadOnlyFallsBackToPlainWhenRendererErrors(t *testing.T) {
	t.Parallel()

	original := renderMarkdownANSI
	renderMarkdownANSI = func(_ string, _ int) (string, error) {
		return "", errors.New("boom")
	}
	t.Cleanup(func() {
		renderMarkdownANSI = original
	})

	r := NewRenderer()
	got := r.RenderReadOnly("# Header", 10)
	if got != "# Header" {
		t.Fatalf("expected plain fallback when renderer errors, got %q", got)
	}
}

func TestRenderReadOnlyUsesMarkdownRendererWhenMarkdownSyntaxPresent(t *testing.T) {
	t.Parallel()

	r := NewRenderer()
	got := r.RenderReadOnly("# Header\n\n- one\n- two", 40)
	plain := ansiStripPattern.ReplaceAllString(got, "")

	if !strings.Contains(plain, "Header") {
		t.Fatalf("expected header text in rendered markdown, got %q", plain)
	}
	if !strings.Contains(plain, "•") && !strings.Contains(plain, "-") {
		t.Fatalf("expected list marker in rendered markdown, got %q", plain)
	}
}

func TestRenderReadOnlyNormalizesInvalidWidthToDefault(t *testing.T) {
	t.Parallel()

	r := NewRenderer()
	content := strings.Repeat("a", defaultWidth+3)
	got := r.RenderReadOnly(content, 0)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected wrapped output to two lines at default width, got %d (%q)", len(lines), got)
	}
	if len(lines[0]) != defaultWidth {
		t.Fatalf("expected first line width %d, got %d", defaultWidth, len(lines[0]))
	}
}

func TestIsEffectivelyPlainText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "plain sentence", input: "just plain text", want: true},
		{name: "markdown heading", input: "# Heading", want: false},
		{name: "markdown list", input: "- item", want: false},
		{name: "markdown blockquote", input: "> quote", want: false},
		{name: "markdown fenced code", input: "```go", want: false},
		{name: "markdown link", input: "[x](https://example.com)", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isEffectivelyPlainText(tc.input); got != tc.want {
				t.Fatalf("isEffectivelyPlainText(%q) = %t, want %t", tc.input, got, tc.want)
			}
		})
	}
}
