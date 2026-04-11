package markdown

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
)

const (
	defaultWidth         = 80
	DefaultEmptyFallback = "(no content)"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// renderMarkdownANSI is a test seam for deterministic fallback testing.
var renderMarkdownANSI = func(content string, width int) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", err
	}

	return renderer.Render(content)
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

	rendered, err := renderMarkdownANSI(content, width)
	if err != nil {
		return renderPlain(content, width)
	}

	if strings.TrimSpace(stripANSI(rendered)) == "" {
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

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
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
		trimmedRight := strings.TrimRight(line, " \t")
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

func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}

	runes := []rune(line)
	if len(runes) <= width {
		return []string{line}
	}

	wrapped := make([]string, 0, len(runes)/width+1)
	for len(runes) > width {
		chunk := strings.TrimRight(string(runes[:width]), " \t")
		wrapped = append(wrapped, chunk)
		runes = runes[width:]
	}
	wrapped = append(wrapped, strings.TrimRight(string(runes), " \t"))

	return wrapped
}
