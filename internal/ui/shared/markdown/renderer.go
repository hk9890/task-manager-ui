// Package markdown renders issue descriptions and comment bodies for terminal
// display.
package markdown

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	defaultWidth         = 80
	DefaultEmptyFallback = "(no content)"
)

// renderMarkdownANSIMemoized is the production markdown renderer. Both the
// glamour renderer and its output are memoized, keyed by everything that can
// change the result.
//
// Nothing above this function is memoized on purpose: a test that sets
// Renderer.renderANSI must see its own function, never a cached frame.
//
// glamour styles the text and wraps none of it: it wraps a block twice, with
// two measures, so a line with a hyphen came out a cell too long from the
// first pass and lost its last word to the second, which also cut a code line
// as prose and put a quote line back without its bar. Neither pass breaks a
// word wider than the pane. wrapRenderedLine wraps each line instead. Only a
// table wider than the pane is taken from a render at the width, because
// glamour sizes its columns.
func renderMarkdownANSIMemoized(content string, width int) (string, error) {
	dark := styles.Dark()

	if cached, ok := lookupRenderedMarkdown(content, width, dark); ok {
		return cached, nil
	}

	limit := max(1, width-rightMargin)
	source := joinSoftBreaks(content)
	lines, err := renderLines(source, 0, dark)
	if err != nil {
		return "", err
	}
	if slices.ContainsFunc(tableBlocks(lines), func(table lineRange) bool { return !fits(lines[table.start:table.end], limit) }) {
		fitted, err := renderLines(source, width, dark)
		if err != nil {
			return "", err
		}
		lines = spliceTables(lines, fitted, limit)
	}

	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapRenderedLine(line, limit)...)
	}
	rendered := strings.Join(wrapped, "\n")

	storeRenderedMarkdown(content, width, dark, rendered)
	return rendered, nil
}

// rightMargin is the blank cells a rendered line keeps before the pane edge:
// the margin glamour's document style has on the left.
const rightMargin = 2

// renderLines returns the lines glamour renders source as, wrapped at wrap
// cells, or not wrapped when wrap is 0.
func renderLines(source string, wrap int, dark bool) ([]string, error) {
	renderer, err := termRendererFor(wrap, dark)
	if err != nil {
		return nil, err
	}

	rendered, err := renderer.Render(source)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(rendered, "\n"), "\n"), nil
}

// markdownCacheKey identifies one rendered result. dark is part of the key
// because the style is chosen from the applied theme (styles.Dark).
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

// termRendererFor returns the glamour renderer for one wrap width and
// background, building it once. A fresh renderer was constructed on every call: 7.7ms and
// 2.9MB for a ~20-section document, 77.5ms and 30MB for a 21KB description.
//
// The style follows the applied theme the way every colour role in this app
// does. It was pinned to "dark", so under a light theme every description,
// note and comment rendered in dark-theme colours while the chrome around
// them did not (docs/DESIGN-GUIDE.md, Colour roles).
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

// glamourStyleName maps the background the applied theme draws on to a glamour
// standard style.
func glamourStyleName(dark bool) string {
	if dark {
		return "dark"
	}
	return "light"
}

// Renderer renders markdown for read-only terminal viewing surfaces.
//
// Fallback behavior is deterministic:
//   - empty/whitespace input -> EmptyFallback
//   - effectively plain text -> plain deterministic wrapping (no ANSI styling)
//   - glamour renderer init/render failure -> same plain deterministic wrapping
type Renderer struct {
	EmptyFallback string

	// renderANSI renders markdown to ANSI. Nil means
	// renderMarkdownANSIMemoized; only a test sets it.
	renderANSI func(string, int) (string, error)
}

// NewRenderer returns a renderer configured for read-only markdown viewing.
func NewRenderer() Renderer {
	return Renderer{EmptyFallback: DefaultEmptyFallback}
}

// RenderReadOnly renders markdown as ANSI output when markdown structure is
// present, otherwise returns deterministic plain text fallback output.
func (r Renderer) RenderReadOnly(input string, width int) string {
	content := strings.Trim(drawable(input), "\n")
	if strings.TrimSpace(content) == "" {
		return emptyFallback(r.EmptyFallback)
	}

	width = normalizeWidth(width)
	if isEffectivelyPlainText(content) {
		return renderPlain(content, width)
	}

	renderANSI := r.renderANSI
	if renderANSI == nil {
		renderANSI = renderMarkdownANSIMemoized
	}

	rendered, err := renderANSI(content, width)
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

// RenderPlain wraps text to width with no markdown rendering. The line breaks
// of the source stay.
func RenderPlain(value string, width int) string {
	return renderPlain(drawable(value), normalizeWidth(width))
}

func renderPlain(value string, width int) string {
	lines := strings.Split(strings.Trim(value, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " ")
		if trimmedRight == "" {
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

// drawable returns source with every character one a terminal draws as text: a
// TAB becomes the spaces that reach its tab stop, and every other control or
// format character what displaytext.OneLine makes of it. The line feeds stay.
//
// It runs on the source, before any wrap. ansi.StringWidth measures a TAB and
// a control character as no cell, so a line that held one measured as narrow,
// was not wrapped, and then ran past the pane border — and past the edge of
// the terminal, which moved every row below it down a line.
func drawable(source string) string {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lines[i] = displaytext.OneLine(expandTabs(line))
	}
	return strings.Join(lines, "\n")
}

// tabWidth is the column stop expandTabs lays a TAB out on. Four rather than
// eight because the panes these lines render into are narrow, and an eight-cell
// indent spends most of a column on a nested code block. Four is also the stop
// markdown gives a TAB, so expanding the source changes no block structure.
const tabWidth = 4

// expandTabs replaces every TAB with the spaces that reach the next tab stop.
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

// wrapLine wraps one line of plain text to width terminal cells at its spaces.
// A word wider than the room is cut on a cell boundary.
//
// Cells, not runes: a CJK ideograph or an emoji covers two cells, so counting
// runes produced lines up to twice the requested width, which the width-correct
// renderer downstream then truncated — losing text instead of wrapping it
// (docs/DESIGN-GUIDE.md, Width and height).
//
// Each chunk takes the indent of the source line, so a continuation line
// starts with a space only when the source line did. A run of spaces inside a
// line that wraps becomes one space; a line that fits is returned as it is.
func wrapLine(line string, width int) []string {
	if width <= 0 || ansi.StringWidth(line) <= width {
		return []string{line}
	}

	text := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(text)]
	if ansi.StringWidth(indent) > width/2 {
		indent = ""
	}
	room := width - ansi.StringWidth(indent)

	var out []string
	current := ""
	for _, word := range strings.Fields(text) {
		if current != "" && ansi.StringWidth(current)+1+ansi.StringWidth(word) <= room {
			current += " " + word
			continue
		}
		if current != "" {
			out = append(out, indent+current)
		}
		pieces := strings.Split(ansi.Hardwrap(word, room, true), "\n")
		for _, piece := range pieces[:len(pieces)-1] {
			out = append(out, indent+piece)
		}
		current = pieces[len(pieces)-1]
	}
	return append(out, indent+current)
}

// markdownParser parses a source the way glamour.NewTermRenderer does.
var markdownParser = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList)).Parser()

// joinSoftBreaks replaces the line end inside each top-level paragraph with a
// space, so a paragraph its author wrapped by hand flows to the pane width.
// glamour joins those lines only while it wraps, and it wraps nothing here.
func joinSoftBreaks(content string) string {
	source := []byte(content)
	document := markdownParser.Parse(text.NewReader(source))
	for block := document.FirstChild(); block != nil; block = block.NextSibling() {
		if block.Kind() != ast.KindParagraph {
			continue
		}
		_ = ast.Walk(block, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if inline, ok := node.(*ast.Text); ok && entering && inline.SoftLineBreak() {
				if at := bytes.IndexByte(source[inline.Segment.Stop:], '\n'); at >= 0 {
					source[inline.Segment.Stop+at] = ' '
				}
			}
			return ast.WalkContinue, nil
		})
	}
	return string(source)
}

// lineRange is the lines [start, end) of a rendered document.
type lineRange struct{ start, end int }

// tableBlocks finds the tables glamour drew: a header line, a rule of ─ and ┼,
// then rows down to the next blank line.
func tableBlocks(lines []string) []lineRange {
	var blocks []lineRange
	for i := 1; i < len(lines); i++ {
		rule := strings.TrimSpace(ansi.Strip(lines[i]))
		if rule == "" || strings.Trim(rule, "─┼") != "" {
			continue
		}
		end := i + 1
		for end < len(lines) && strings.TrimSpace(ansi.Strip(lines[end])) != "" {
			end++
		}
		blocks = append(blocks, lineRange{start: i - 1, end: end})
		i = end
	}
	return blocks
}

// fits reports whether no line is wider than limit cells.
func fits(lines []string, limit int) bool {
	return !slices.ContainsFunc(lines, func(line string) bool { return ansi.StringWidth(line) > limit })
}

// spliceTables returns flow with each of its tables that is wider than limit
// replaced by the same table of fitted, the render glamour sized to the pane.
// A table stays as it is, and its rows wrap as text, when glamour cut a cell
// to make it fit, or when the two renders disagree on the tables.
func spliceTables(flow, fitted []string, limit int) []string {
	from, to := tableBlocks(fitted), tableBlocks(flow)
	if len(from) != len(to) {
		return flow
	}

	var out []string
	next := 0
	for i, table := range to {
		natural, sized := flow[table.start:table.end], fitted[from[i].start:from[i].end]
		if fits(natural, limit) || tableText(natural) != tableText(sized) {
			continue
		}
		out = append(out, flow[next:table.start]...)
		out = append(out, sized...)
		next = table.end
	}
	return append(out, flow[next:]...)
}

// tableText is the characters of the cells of a table, sorted: glamour wraps
// inside a cell, so two renders of one table hold them in another order.
func tableText(lines []string) string {
	text := []rune(strings.NewReplacer(" ", "", "│", "", "─", "", "┼", "").Replace(ansi.Strip(strings.Join(lines, ""))))
	slices.Sort(text)
	return string(text)
}

// cell is one grapheme of a rendered line with the SGR style it is drawn in.
type cell struct {
	text  string
	width int
	style string
}

func (c cell) isSpace() bool { return c.text == " " }

// hasBackground reports whether the cell is drawn on a colour of its own, as
// the space glamour pads inline code with is: such a space is text, not indent.
func (c cell) hasBackground() bool { return strings.Contains(c.style, ";48;") }

// styledCells splits a rendered line into its cells. A sequence that is not a
// style and draws nothing is dropped; glamour writes none.
func styledCells(line string) []cell {
	parser := ansi.GetParser()
	defer ansi.PutParser(parser)

	var cells []cell
	style, state := "", byte(0)
	for line != "" {
		seq, width, n, next := ansi.DecodeSequence(line, state, parser)
		switch {
		case width > 0:
			cells = append(cells, cell{text: seq, width: width, style: style})
		case seq == "\x1b[0m" || seq == ansi.ResetStyle:
			style = ""
		case ansi.HasCsiPrefix(seq) && parser.Command() == 'm':
			style += seq
		}
		line, state = line[n:], next
	}
	return cells
}

// renderCells draws cells as one line that carries its own styles and ends
// with none open, so the line can be drawn alone.
func renderCells(cells []cell) string {
	var b strings.Builder
	style := ""
	for _, c := range cells {
		if c.style != style {
			if style != "" {
				b.WriteString(ansi.ResetStyle)
			}
			b.WriteString(c.style)
			style = c.style
		}
		b.WriteString(c.text)
	}
	if style != "" {
		b.WriteString(ansi.ResetStyle)
	}
	return b.String()
}

func cellsText(cells []cell) string {
	var b strings.Builder
	for _, c := range cells {
		b.WriteString(c.text)
	}
	return b.String()
}

// hangingPrefix is what glamour puts in front of the text of a line: the
// margin, the quote bars, and one list marker. Each of its characters is one
// cell wide.
var hangingPrefix = regexp.MustCompile(`^[ │]*(?:(?:•|\d+\.|\[[ ✓]\]) )?`)

// wrapRenderedLine wraps one line glamour rendered to width cells. A
// continuation line starts under the text of the first line and keeps the
// quote bars, so a list item, a quote and a code line each keep their indent.
func wrapRenderedLine(line string, width int) []string {
	if ansi.StringWidth(line) <= width {
		return []string{line}
	}

	cells := styledCells(line)
	indent := min(len([]rune(hangingPrefix.FindString(cellsText(cells)))), width/2)
	for i, c := range cells[:indent] {
		if c.hasBackground() {
			indent = i
			break
		}
	}
	prefix := cells[:indent:indent]
	hanging := make([]cell, indent)
	for i, c := range prefix {
		hanging[i] = cell{text: " ", width: 1}
		if c.text == "│" {
			hanging[i] = c
		}
	}

	var lines []string
	for _, chunk := range wrapCells(cells[indent:], width-indent) {
		lines = append(lines, renderCells(append(prefix, chunk...)))
		prefix = hanging[:indent:indent]
	}
	return lines
}

// wrapCells breaks cells into lines of at most limit cells. It breaks at a
// space, and inside a word only when the word is wider than limit.
func wrapCells(cells []cell, limit int) [][]cell {
	var lines [][]cell
	for len(cells) > 0 {
		end := lineEnd(cells, limit)
		line := cells[:end]
		for len(line) > 0 && line[len(line)-1].isSpace() {
			line = line[:len(line)-1]
		}
		if len(line) > 0 {
			lines = append(lines, line)
		}
		cells = cells[end:]
		for len(cells) > 0 && cells[0].isSpace() {
			cells = cells[1:]
		}
	}
	return lines
}

// lineEnd returns how many of cells go on a line of limit cells.
func lineEnd(cells []cell, limit int) int {
	width, lastBreak := 0, -1
	for i, c := range cells {
		if width+c.width > limit {
			switch {
			case breakable(cells, i):
				return i
			case lastBreak >= 0 && wordWidth(cells, lastBreak+1) <= limit:
				return lastBreak
			default:
				return max(i, 1)
			}
		}
		if breakable(cells, i) {
			lastBreak = i
		}
		width += c.width
	}
	return len(cells)
}

// breakable reports whether a line may end at cell i: at a space, unless the
// next line would then start with closing punctuation. glamour pads inline
// code with a space, so "`code`, and" reaches here as "code , and".
func breakable(cells []cell, i int) bool {
	if !cells[i].isSpace() {
		return false
	}
	return i+1 == len(cells) || !strings.Contains(",.;:!?)", cells[i+1].text)
}

// wordWidth is the width of the cells from start up to the next break.
func wordWidth(cells []cell, start int) int {
	width := 0
	for i := start; i < len(cells) && !breakable(cells, i); i++ {
		width += cells[i].width
	}
	return width
}
