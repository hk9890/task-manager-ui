// Package markdown renders issue descriptions and comment bodies for terminal
// display.
package markdown

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
)

const (
	defaultWidth         = 80
	DefaultEmptyFallback = "(no content)"
)

// renderMarkdownANSIMu guards renderMarkdownANSI. Production code never
// reassigns the variable after init; the mutex exists so that tests running in
// parallel can safely swap and restore the seam without triggering the race
// detector.
var renderMarkdownANSIMu sync.Mutex

// renderMarkdownANSI is a test seam for deterministic fallback testing.
var renderMarkdownANSI = renderMarkdownANSIMemoized

// renderMarkdownANSIMemoized is the production markdown renderer. Both the
// glamour renderer and its output are memoized, keyed by everything that can
// change the result.
//
// Nothing above this seam is memoized on purpose: a test that swaps
// renderMarkdownANSI must see its own function, never a cached frame.
func renderMarkdownANSIMemoized(content string, width int) (string, error) {
	dark := lipgloss.HasDarkBackground()

	if cached, ok := lookupRenderedMarkdown(content, width, dark); ok {
		return cached, nil
	}

	renderer, err := termRendererFor(width, dark)
	if err != nil {
		return "", err
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		return "", err
	}

	storeRenderedMarkdown(content, width, dark, rendered)
	return rendered, nil
}

// markdownCacheKey identifies one rendered result. dark is part of the key
// because the style is chosen from the terminal background.
type markdownCacheKey struct {
	content string
	width   int
	dark    bool
}

// markdownCacheMaxEntries caps the memo. Detail mode re-renders a handful of
// bodies at one or two widths, so a small cap holds the whole working set; the
// map is dropped wholesale when it overflows rather than carrying an eviction
// policy no measurement asked for.
const markdownCacheMaxEntries = 64

var (
	markdownCacheMu sync.Mutex
	markdownCache   = map[markdownCacheKey]string{}
	termRenderers   = map[markdownCacheKey]*lockedRenderer{}
)

// lockedRenderer pairs a memoized glamour renderer with the mutex that
// serialises it.
//
// glamour's ANSIRenderer carries a mutable RenderContext — a block stack and a
// table accumulator — that every Render mutates, so one shared renderer is not
// safe for concurrent use: two goroutines rendering different bodies interleave
// pushes and pops on that stack, which surfaces as output interleaved between
// two documents, or a panic in the walk. Building a renderer per call was what
// used to keep them apart; memoizing it removed that isolation.
//
// The mutex is per (width, background), so renders at different widths still
// run in parallel, and the memo above means a repeated body does not reach here
// at all.
type lockedRenderer struct {
	mu       sync.Mutex
	renderer *glamour.TermRenderer
}

func (l *lockedRenderer) Render(content string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.renderer.Render(content)
}

func lookupRenderedMarkdown(content string, width int, dark bool) (string, bool) {
	markdownCacheMu.Lock()
	defer markdownCacheMu.Unlock()
	rendered, ok := markdownCache[markdownCacheKey{content: content, width: width, dark: dark}]
	return rendered, ok
}

func storeRenderedMarkdown(content string, width int, dark bool, rendered string) {
	markdownCacheMu.Lock()
	defer markdownCacheMu.Unlock()
	if len(markdownCache) >= markdownCacheMaxEntries {
		markdownCache = map[markdownCacheKey]string{}
	}
	markdownCache[markdownCacheKey{content: content, width: width, dark: dark}] = rendered
}

// termRendererFor returns the glamour renderer for one width and background,
// building it once. A fresh renderer was constructed on every call: 7.7ms and
// 2.9MB for a ~20-section document, 77.5ms and 30MB for a 21KB description.
//
// The style follows the terminal background the way every lipgloss.
// AdaptiveColor in this app does. It was pinned to "dark", so on a light
// terminal every description, note and comment rendered in dark-theme colours
// while the chrome around them adapted (docs/DESIGN-GUIDE.md, Colour roles).
func termRendererFor(width int, dark bool) (*lockedRenderer, error) {
	key := markdownCacheKey{width: width, dark: dark}

	markdownCacheMu.Lock()
	defer markdownCacheMu.Unlock()
	if renderer, ok := termRenderers[key]; ok {
		return renderer, nil
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(glamourStyleName(dark)),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}

	if len(termRenderers) >= markdownCacheMaxEntries {
		termRenderers = map[markdownCacheKey]*lockedRenderer{}
	}
	locked := &lockedRenderer{renderer: renderer}
	termRenderers[key] = locked
	return locked, nil
}

// glamourStyleName maps the terminal background to a glamour standard style,
// the same split lipgloss.AdaptiveColor makes for every other colour here.
func glamourStyleName(dark bool) string {
	if dark {
		return "dark"
	}
	return "light"
}

// getRenderMarkdownANSI returns the current renderMarkdownANSI function under lock.
func getRenderMarkdownANSI() func(string, int) (string, error) {
	renderMarkdownANSIMu.Lock()
	defer renderMarkdownANSIMu.Unlock()
	return renderMarkdownANSI
}

// Renderer renders markdown for read-only terminal viewing surfaces.
//
// Fallback behavior is deterministic:
//   - empty/whitespace input -> EmptyFallback
//   - effectively plain text -> plain deterministic wrapping (no ANSI styling)
//   - glamour renderer init/render failure -> same plain deterministic wrapping
type Renderer struct {
	EmptyFallback string
}

// NewRenderer returns a renderer configured for read-only markdown viewing.
func NewRenderer() Renderer {
	return Renderer{EmptyFallback: DefaultEmptyFallback}
}

// RenderReadOnly renders markdown as ANSI output when markdown structure is
// present, otherwise returns deterministic plain text fallback output.
func (r Renderer) RenderReadOnly(input string, width int) string {
	content := strings.Trim(input, "\n")
	if strings.TrimSpace(content) == "" {
		return emptyFallback(r.EmptyFallback)
	}

	width = normalizeWidth(width)
	if isEffectivelyPlainText(content) {
		return renderPlain(content, width)
	}

	rendered, err := getRenderMarkdownANSI()(content, width)
	if err != nil {
		return renderPlain(content, width)
	}

	if strings.TrimSpace(textutil.StripANSI(rendered)) == "" {
		return renderPlain(content, width)
	}

	return strings.TrimRight(rendered, "\n")
}

func emptyFallback(value string) string {
	if strings.TrimSpace(value) == "" {
		return DefaultEmptyFallback
	}
	return value
}

func normalizeWidth(width int) int {
	if width <= 0 {
		return defaultWidth
	}
	return width
}

func isEffectivelyPlainText(value string) bool {
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "# "):
			return false
		case strings.HasPrefix(trimmed, "## "):
			return false
		case strings.HasPrefix(trimmed, "### "):
			return false
		case strings.HasPrefix(trimmed, "- "):
			return false
		case strings.HasPrefix(trimmed, "* "):
			return false
		case strings.HasPrefix(trimmed, "> "):
			return false
		case strings.HasPrefix(trimmed, "```"):
			return false
		case strings.Contains(trimmed, "[") && strings.Contains(trimmed, "](") && strings.Contains(trimmed, ")"):
			return false
		}
	}

	return true
}

func renderPlain(value string, width int) string {
	lines := strings.Split(strings.Trim(value, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmedRight := expandTabs(strings.TrimRight(line, " \t"))
		if strings.TrimSpace(trimmedRight) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrapLine(trimmedRight, width)...)
	}

	if len(out) == 0 {
		return DefaultEmptyFallback
	}

	return strings.Join(out, "\n")
}

// tabWidth is the column stop expandTabs lays a TAB out on. Four rather than
// eight because the panes these lines render into are narrow, and an eight-cell
// indent spends most of a column on a nested code block.
const tabWidth = 4

// expandTabs replaces every TAB with the spaces that reach the next tab stop.
//
// ansi.StringWidth measures a TAB as zero cells, so a tab-bearing line measured
// as narrow, was never wrapped, and then bled past the pane border while
// lipgloss.Width agreed it fitted — pasted Go source, a Makefile and a TSV table
// all hit it. Expanding first makes the measurement and what the terminal draws
// the same thing, because what is drawn is now spaces.
func expandTabs(line string) string {
	if !strings.ContainsRune(line, '\t') {
		return line
	}

	var b strings.Builder
	b.Grow(len(line) + tabWidth)
	column := 0
	for _, r := range line {
		if r == '\t' {
			pad := tabWidth - column%tabWidth
			b.WriteString(strings.Repeat(" ", pad))
			column += pad
			continue
		}
		b.WriteRune(r)
		column += ansi.StringWidth(string(r))
	}
	return b.String()
}

// wrapLine hard-wraps one line to width terminal cells.
//
// Cells, not runes: a CJK ideograph or an emoji covers two cells, so counting
// runes produced lines up to twice the requested width, which the width-correct
// renderer downstream then truncated — losing text instead of wrapping it
// (docs/DESIGN-GUIDE.md, Width and height).
//
// Leading space is preserved on every wrapped chunk. Dropping it re-indented
// only the lines that happened to wrap, which reads as corrupted output rather
// than as wrapping.
func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	if ansi.StringWidth(line) <= width {
		return []string{line}
	}

	wrapped := strings.Split(ansi.Hardwrap(line, width, true), "\n")
	for i, chunk := range wrapped {
		wrapped[i] = strings.TrimRight(chunk, " \t")
	}

	return wrapped
}
