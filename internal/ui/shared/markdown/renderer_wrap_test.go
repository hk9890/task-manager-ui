package markdown

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
)

// hardBody holds what glamour's own wrap could not keep inside a pane: words
// with hyphens, a word, a CJK run and a URL wider than the pane, a TAB in
// code, in a list item, in a quote and in a paragraph, and a nested list.
const hardBody = "## Context\n\n" +
	"`install/wctl-install.sh`, functions `extension_needs_upgrade` and `install_release_extension`. " +
	"Read the list of pinned versions the way scripts/setup/install-lib.sh, as install/gtile-install.sh does.\n\n" +
	"CJK: 日本語のテキストが長い行で折り返されるかどうかを確認するためのとても長い文章です。日本語のテキストが長い行で折り返されるかどうかを確認するためのとても長い文章です。\n\n" +
	"A token " + "very-long-" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa end, " +
	"and a link https://example.com/a/very/long/path/that/goes/on/and/on/and/on/and/on/and/on/and/on/and/on/and/on/and/on/end.\n\n" +
	"    $ curl -s 'https://extensions.gnome.org/extension-info/?uuid=window-control@carlo9890.github.io&shell_version=50' | jq .version\n" +
	"    GNOME Window Control v12\tLatest\tv12\t2026-10-02T15:34:07Z\n\n" +
	"```go\nfunc main() {\n\tif true {\n\t\tfmt.Println(\"tab indented\")\n\t}\n}\n```\n\n" +
	"- [ ] one criterion with a\ttab\n- [x] a done criterion\n\n" +
	"> A quote with a\ttab inside and a long text that must wrap inside the quote block without breaking the bar on the left side of it at all.\n\n" +
	"1. first\n   - nested item with `code` and a long text that must wrap under the nested bullet and keep its indent on the next line too\n2. second\n\n" +
	"Line with a\ttab in a paragraph.\n"

func renderedLines(t *testing.T, body string, width int) []string {
	t.Helper()
	return strings.Split(textutil.StripANSI(NewRenderer().RenderReadOnly(body, width)), "\n")
}

// ink is the text of lines without the cells a wrap may add or take away:
// spaces and the bar that starts each line of a quote.
func ink(lines []string) string {
	return strings.NewReplacer(" ", "", "│", "").Replace(strings.Join(lines, ""))
}

// TestRenderedMarkdownStaysInsideTheWidthAndLosesNoText is the contract of the
// markdown path at every pane width: no line is wider than the pane, no TAB
// reaches the terminal, and the text is the same text at every width — a line
// the pane had to cut lost its end for good, because a pane scrolls down only.
func TestRenderedMarkdownStaysInsideTheWidthAndLosesNoText(t *testing.T) {
	t.Parallel()

	want := ink(renderedLines(t, hardBody, 2000))
	for width := 20; width <= 240; width++ {
		lines := renderedLines(t, hardBody, width)
		for _, line := range lines {
			if strings.ContainsRune(line, '\t') {
				t.Fatalf("width %d: a TAB reaches the terminal: %q", width, line)
			}
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width %d: line is %d cells wide: %q", width, got, line)
			}
		}
		if got := ink(lines); got != want {
			t.Fatalf("width %d: the text changed:\n%s\nwant\n%s", width, got, want)
		}
	}
}

// TestAParagraphLineEndsOnlyWhenTheNextWordDoesNotFit: glamour wrapped a block
// twice, and a line with a hyphen came out of the first pass a cell too long,
// so the second pass moved its last word to a line of its own.
func TestAParagraphLineEndsOnlyWhenTheNextWordDoesNotFit(t *testing.T) {
	t.Parallel()

	const body = "## T\n\nRead the list of pinned versions the way scripts/setup/install-lib.sh, as install/gtile-install.sh does. " +
		"Keep the rule in one place, so that the next change of a pinned-version-number is a one-line change."
	for width := 30; width <= 160; width++ {
		lines := renderedLines(t, body, width)
		for i := 3; i+1 < len(lines); i++ {
			line, next := strings.TrimRight(lines[i], " "), strings.Fields(lines[i+1])
			if len(next) == 0 {
				continue
			}
			if lipgloss.Width(line)+1+lipgloss.Width(next[0]) <= width-rightMargin {
				t.Fatalf("width %d: %q ends before %q, which fits", width, line, next[0])
			}
		}
	}
}

// TestAQuoteKeepsItsBarOnEveryLine: glamour's second pass knew no prefix, so
// the word it moved to a new line stood there without the quote bar.
func TestAQuoteKeepsItsBarOnEveryLine(t *testing.T) {
	t.Parallel()

	const body = "> A quote with a long text that must wrap inside the quote block without breaking the bar on the left side of it at all."
	for width := 20; width <= 120; width++ {
		for _, line := range renderedLines(t, body, width) {
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "  │ ") {
				t.Fatalf("width %d: a quote line has no bar: %q", width, line)
			}
		}
	}
}

// TestAWrappedLineStartsUnderTheTextOfItsFirstLine: a code line keeps its
// indent, and a list item continues under its text, not under its marker.
func TestAWrappedLineStartsUnderTheTextOfItsFirstLine(t *testing.T) {
	t.Parallel()

	code := renderedLines(t, "text\n\n    $ curl -s 'https://extensions.gnome.org/extension-info/' | jq .version | sort | uniq | head", 40)
	for _, line := range code[3:] {
		if !strings.HasPrefix(line, "    ") {
			t.Errorf("a code line lost its indent: %q", line)
		}
	}

	item := renderedLines(t, "1. first\n   - nested item with a long text that must wrap under the nested bullet and keep its indent", 50)
	nested := slices.IndexFunc(item, func(line string) bool { return strings.HasPrefix(line, "    • nested") })
	if nested < 0 || nested+1 >= len(item) {
		t.Fatalf("expected the nested item to wrap, got %q", item)
	}
	for _, line := range item[nested+1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "      ") || strings.HasPrefix(line, "       ") {
			t.Errorf("the item does not continue under its text: %q", line)
		}
	}
}

// TestAParagraphThatStartsWithInlineCodeHangsAtTheMargin: glamour pads inline
// code with a space on the code's background. That space is text.
func TestAParagraphThatStartsWithInlineCodeHangsAtTheMargin(t *testing.T) {
	t.Parallel()

	lines := renderedLines(t, "## T\n\n`code` and then enough words that this paragraph has to wrap at this width", 40)
	if len(lines) < 5 {
		t.Fatalf("expected the paragraph to wrap, got %q", lines)
	}
	if !strings.HasPrefix(lines[4], "  ") || strings.HasPrefix(lines[4], "   ") {
		t.Errorf("the second line does not start at the 2-cell margin: %q", lines[4])
	}
}

// TestATableKeepsItsColumns: a table is the one block taken from glamour at
// the pane width, because glamour sizes the columns and wraps inside a cell.
func TestATableKeepsItsColumns(t *testing.T) {
	t.Parallel()

	const body = "## T\n\n| Name | Value | Note |\n|---|---|---|\n| alpha | 1 | a short note |\n" +
		"| beta | 2 | a much longer note that runs on and on to see what the table does when a cell is wider than the pane |\n\nafter"
	const width = 60

	lines := renderedLines(t, body, width)
	rule := -1
	for i, line := range lines {
		if strings.Contains(line, "┼") {
			rule = i
		}
	}
	if rule < 1 || strings.Count(lines[rule], "┼") != 2 {
		t.Fatalf("expected one rule with two column joints, got %q", lines)
	}
	for i := rule + 1; i < len(lines) && strings.TrimSpace(lines[i]) != ""; i++ {
		if strings.Count(lines[i], "│") != 2 {
			t.Errorf("a table row lost a column: %q", lines[i])
		}
		if got := lipgloss.Width(lines[i]); got > width {
			t.Errorf("a table row is %d cells wide: %q", got, lines[i])
		}
	}
}

// TestATableKeepsItsTextAtEveryWidth: glamour cuts a cell of a table it
// cannot make narrow enough, and one character at the width its table just
// misses. Such a table keeps its text and its rows wrap as text.
func TestATableKeepsItsTextAtEveryWidth(t *testing.T) {
	t.Parallel()

	const body = "## T\n\n| Name | Value | Note |\n|---|---|---|\n| alpha | 1 | a short note |\n" +
		"| beta | 2 | a much longer note that runs on and on to see what the table does when a cell is wider than the pane |\n"
	want := tableText(renderedLines(t, body, 2000))
	for width := 20; width <= 140; width++ {
		lines := renderedLines(t, body, width)
		if got := tableText(lines); got != want {
			t.Fatalf("width %d: the table lost text:\n%s", width, strings.Join(lines, "\n"))
		}
		for _, line := range lines {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width %d: a table row is %d cells wide: %q", width, got, line)
			}
		}
	}
}

// TestAHandWrappedParagraphFlowsToTheWidth: glamour joined the lines of a
// paragraph only while it wrapped. A hard break still ends its line.
func TestAHandWrappedParagraphFlowsToTheWidth(t *testing.T) {
	t.Parallel()

	const body = "# H\n\nA paragraph that the author wrapped by hand\nat about forty five cells in the source file\nso every line ends with a soft break.\n\n" +
		"Hard break here\\\nnext line."
	text := strings.Join(renderedLines(t, body, 78), "\n")
	if !strings.Contains(text, "by hand at about forty five cells") {
		t.Errorf("the hand-wrapped paragraph kept its line ends:\n%s", text)
	}
	if strings.Contains(text, "here next") || strings.Contains(text, "herenext") {
		t.Errorf("the hard break is gone:\n%s", text)
	}
}

// TestATabInCodeReachesItsTabStop: the TAB is expanded in the source, where
// its column is the column the author saw.
func TestATabInCodeReachesItsTabStop(t *testing.T) {
	t.Parallel()

	text := strings.Join(renderedLines(t, "```\na\tb\nabc\td\nabcd\te\n```\n", 60), "\n")
	for _, want := range []string{"a   b", "abc d", "abcd    e"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in:\n%s", want, text)
		}
	}
}

// TestNoControlCharacterOfTheSourceReachesTheTerminal: a body is text from
// outside. An escape sequence in it would run in the terminal, and a carriage
// return would draw the rest of its line over the pane border.
func TestNoControlCharacterOfTheSourceReachesTheTerminal(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		"plain\x1b[2Jtext\rmore\vend\r\nsecond line",
		"# Heading\r\n\r\npara\x1b[2Jgraph\rmore\vend",
	} {
		got := textutil.StripANSI(NewRenderer().RenderReadOnly(body, 40))
		for _, r := range got {
			if r != '\n' && r < ' ' {
				t.Errorf("control character %q reaches the terminal: %q", r, got)
			}
		}
	}
}

// TestAWrappedLineCarriesItsOwnStyles: a pane draws each line alone, so a
// style that is open where a line ends must end there and start again.
func TestAWrappedLineCarriesItsOwnStyles(t *testing.T) {
	t.Parallel()

	lines := wrapRenderedLine("\x1b[38;5;203maaa bbb ccc ddd eee fff\x1b[0m", 8)
	if len(lines) < 3 {
		t.Fatalf("expected the line to wrap, got %q", lines)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "\x1b[38;5;203m") || !strings.HasSuffix(line, "\x1b[m") {
			t.Errorf("line does not open and close its style: %q", line)
		}
	}
	if got := strings.Join(strings.Fields(textutil.StripANSI(strings.Join(lines, " "))), " "); got != "aaa bbb ccc ddd eee fff" {
		t.Errorf("the words changed: %q", got)
	}
}
