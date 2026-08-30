package markdown

import (
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestPlainWrappingCountsATabAsCells pins that a tab-bearing line is wrapped.
//
// ansi.StringWidth measures a TAB as zero cells, so a line of tabs measured as
// narrow, was never wrapped, and bled past the Content pane border while
// lipgloss.Width agreed it fitted — which is also why TruncateString downstream
// could not rescue it. renderPlain expands tabs before measuring.
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

// TestPlainWrappingKeepsLeadingSpaceOnWrappedChunks pins indentation across a
// wrap. ansi.Hardwrap with preserveSpace=false dropped the leading spaces of
// every wrapped chunk, so an indented paragraph lost its indent on exactly the
// lines that wrapped, which reads as corrupted output rather than as wrapping.
func TestPlainWrappingKeepsLeadingSpaceOnWrappedChunks(t *testing.T) {
	t.Parallel()

	r := NewRenderer()
	got := r.RenderReadOnly("aaaaaaaaaaaaaaaaaaaa    bbbb", 20)

	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected the line to wrap into 2, got %d: %q", len(lines), got)
	}
	if !strings.HasPrefix(lines[1], "    ") {
		t.Errorf("wrapped chunk lost its leading space: %q", lines[1])
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
