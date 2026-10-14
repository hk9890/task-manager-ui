package markdown

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestPlainWrappingCountsATabAsCells pins that a tab-bearing line is wrapped.
//
// ansi.StringWidth measures a TAB as zero cells, so a line of tabs measured as
// narrow, was never wrapped, and bled past the Content pane border while
// lipgloss.Width agreed it fitted — which is also why TruncateString downstream
// could not rescue it. drawable expands tabs in the source, before any measure.
func TestPlainWrappingCountsATabAsCells(t *testing.T) {
	t.Parallel()

	const width = 8
	r := NewRenderer()
	got := r.RenderReadOnly("a\tb\tc\td\te\tf\tg\th", width)

	if strings.ContainsRune(got, '\t') {
		t.Errorf("expected tabs expanded to spaces, got %q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("line %q is %d cells wide, want at most %d", line, w, width)
		}
	}
}

// TestPlainWrappingKeepsTheIndentOfTheSourceLine pins indentation across a
// wrap: an indented line that wraps keeps its indent on every chunk, and a
// line with no indent gets none. A hard wrap at the cell started a chunk with
// the space that stood between two words, which read as a stray indent.
func TestPlainWrappingKeepsTheIndentOfTheSourceLine(t *testing.T) {
	t.Parallel()

	const width = 24
	r := NewRenderer()

	const indented = "    indented paragraph that is long enough to wrap twice at this width ok"
	lines := strings.Split(r.RenderReadOnly(indented, width), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected the line to wrap twice, got %q", lines)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "     ") {
			t.Errorf("chunk does not have the 4-cell indent of its source line: %q", line)
		}
		if w := lipgloss.Width(line); w > width {
			t.Errorf("chunk %q is %d cells wide, want at most %d", line, w, width)
		}
	}
	if got, want := strings.Fields(strings.Join(lines, " ")), strings.Fields(indented); !slices.Equal(got, want) {
		t.Errorf("wrapping changed the words: got %q, want %q", got, want)
	}

	const flush = "This comment goes on and on and on and on and on and on and on and on and on"
	for _, line := range strings.Split(r.RenderReadOnly(flush, width), "\n") {
		if strings.HasPrefix(line, " ") {
			t.Errorf("chunk of a line with no indent starts with a space: %q", line)
		}
	}
}

// TestPlainWrappingCutsOnlyAWordWiderThanTheLine: a word that fits moves to
// the next line whole; one that cannot fit is cut at the cell, and the text
// after it goes on behind its last piece.
func TestPlainWrappingCutsOnlyAWordWiderThanTheLine(t *testing.T) {
	t.Parallel()

	whole := wrapLine("internationalization standardization characterization", 20)
	if want := []string{"internationalization", "standardization", "characterization"}; !slices.Equal(whole, want) {
		t.Errorf("words that fit were cut: got %q, want %q", whole, want)
	}

	long := strings.Repeat("y", 50)
	cut := wrapLine("word "+long+" tail", 24)
	if want := []string{"word", long[:24], long[:24], "yy tail"}; !slices.Equal(cut, want) {
		t.Errorf("a word wider than the line: got %q, want %q", cut, want)
	}
}

// TestRenderReadOnlyIsSafeUnderConcurrentUse pins that the memoized glamour
// renderer is serialised.
//
// glamour's ANSIRenderer carries a mutable RenderContext — a block stack and a
// table accumulator — that every Render mutates. Memoizing one renderer per
// width removed the per-call isolation a fresh renderer used to provide, so two
// goroutines rendering different bodies at one width interleaved pushes and pops
// on that stack. Under -race this test reports the write; without it the failure
// is interleaved output or a panic in the walk.
func TestRenderReadOnlyIsSafeUnderConcurrentUse(t *testing.T) {
	clearMarkdownCaches()
	t.Cleanup(clearMarkdownCaches)

	const width = 60
	bodies := []string{
		"# One\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n- alpha\n- beta",
		"## Two\n\n1. first\n2. second\n\n> quoted\n\n```go\nfunc main() {}\n```",
		"### Three\n\n**bold** and *italic* and `code`\n\n- [ ] task\n- [x] done",
	}

	r := NewRenderer()
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// Distinct bodies at one width: a repeated body would be answered
			// from the memo and never reach the renderer.
			body := bodies[n%len(bodies)] + strings.Repeat("\n\nparagraph ", n%5+1)
			if out := r.RenderReadOnly(body, width); strings.TrimSpace(out) == "" {
				t.Errorf("goroutine %d rendered nothing", n)
			}
		}(i)
	}
	wg.Wait()
}
