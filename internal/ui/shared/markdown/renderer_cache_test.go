package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func clearMarkdownCaches() {
	markdownCacheMu.Lock()
	defer markdownCacheMu.Unlock()
	markdownCache = map[markdownCacheKey]string{}
	termRenderers = map[markdownCacheKey]*lockedRenderer{}
}

// TestRenderReadOnlyMemoizesRenderedMarkdown proves the memo is actually
// consulted: a poisoned cache entry must come back instead of a fresh render.
//
// Without it a fresh glamour renderer was built and the whole document
// re-rendered on every call — measured at 7.7ms / 2.9MB / 66k allocs for a
// ~20-section document — and detail mode called it twice per frame, once for
// the visible pane and once for MaxScrollOffsets counting lines.
func TestRenderReadOnlyMemoizesRenderedMarkdown(t *testing.T) {
	const content = "# Cached\n\n- one\n- two"
	const width = 40

	clearMarkdownCaches()
	t.Cleanup(clearMarkdownCaches)

	r := NewRenderer()
	first := r.RenderReadOnly(content, width)
	if strings.TrimSpace(first) == "" {
		t.Fatal("expected rendered markdown")
	}

	dark := lipgloss.HasDarkBackground()
	markdownCacheMu.Lock()
	cached, ok := markdownCache[markdownCacheKey{content: content, width: width, dark: dark}]
	markdownCacheMu.Unlock()
	if !ok {
		t.Fatal("the rendered document was not memoized")
	}
	if strings.TrimRight(cached, "\n") != first {
		t.Fatal("the memoized entry does not match what was returned")
	}

	// Poison the entry: a second call that re-renders would not return it.
	markdownCacheMu.Lock()
	markdownCache[markdownCacheKey{content: content, width: width, dark: dark}] = "SENTINEL"
	markdownCacheMu.Unlock()

	if got := r.RenderReadOnly(content, width); got != "SENTINEL" {
		t.Errorf("second render did not use the memo: got %q", got)
	}

	// A different width is a different entry, so it renders for real.
	if got := r.RenderReadOnly(content, width+10); got == "SENTINEL" {
		t.Error("the memo is not keyed by width")
	}
}

// TestGlamourStyleFollowsTerminalBackground pins the colour rule
// docs/DESIGN-GUIDE.md states: a surface carries a light and a dark value.
// The glamour style was hard-pinned to "dark", so on a light terminal every
// description, note and comment rendered in dark-theme colours.
func TestGlamourStyleFollowsTerminalBackground(t *testing.T) {
	if got := glamourStyleName(true); got != "dark" {
		t.Errorf("glamourStyleName(dark) = %q, want dark", got)
	}
	if got := glamourStyleName(false); got != "light" {
		t.Errorf("glamourStyleName(light) = %q, want light", got)
	}

	clearMarkdownCaches()
	t.Cleanup(clearMarkdownCaches)

	lightRenderer, err := termRendererFor(40, false)
	if err != nil {
		t.Fatalf("termRendererFor(light): %v", err)
	}
	darkRenderer, err := termRendererFor(40, true)
	if err != nil {
		t.Fatalf("termRendererFor(dark): %v", err)
	}
	if lightRenderer == darkRenderer {
		t.Error("the light and dark backgrounds share one renderer")
	}

	// The renderer is built once per width and background.
	again, err := termRendererFor(40, true)
	if err != nil {
		t.Fatalf("termRendererFor(dark, again): %v", err)
	}
	if again != darkRenderer {
		t.Error("the renderer was rebuilt for a width and background already seen")
	}
}

// TestWrapLineWrapsOnCellsNotRunes pins the plain-text fallback's width math.
// A CJK ideograph covers two cells, so counting runes produced lines twice the
// requested width, which the width-correct pass downstream truncated — losing
// text rather than wrapping it.
func TestWrapLineWrapsOnCellsNotRunes(t *testing.T) {
	t.Parallel()

	const line = "日本語のテキストはここにあります"
	const width = 6

	got := wrapLine(line, width)

	joined := strings.Join(got, "")
	if joined != line {
		t.Errorf("wrapping dropped content: got %q, want %q", joined, line)
	}
	for i, chunk := range got {
		if w := ansi.StringWidth(chunk); w > width {
			t.Errorf("line %d is %d cells wide, want at most %d: %q", i, w, width, chunk)
		}
	}

	// ASCII wrapping is unchanged.
	if got := wrapLine("alpha beta gamma delta", 6); strings.Join(got, "\n") != "alpha\nbeta g\namma d\nelta" {
		t.Errorf("ASCII wrapping changed: %q", got)
	}
}
