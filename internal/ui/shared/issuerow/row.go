// Package issuerow renders one issue as a compact row. It is the shared row
// primitive behind the board columns and the search results, so both surfaces
// read identically.
package issuerow

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/renderhelpers"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	// Height is the number of lines RenderCompact and RenderCompactSkeleton
	// draw for one issue: the type and the title, then the priority, the
	// status and the ID under the title. A list that scrolls, counts or
	// hit-tests issue rows reads it instead of assuming a line.
	Height = 2

	minNarrowTitleWidth = 4
	minCompactIDWidth   = 7
	maxCompactIDWidth   = 12

	// SkeletonGlyph is the canonical placeholder character for the loading-bar
	// segments (id + title) of a skeleton row. It is U+2591 LIGHT SHADE — light
	// enough to read as a placeholder rather than uniform noise.
	// Test assertions reference this constant so no caller hard-codes the literal rune.
	SkeletonGlyph = "░"

	// SkeletonMetaGlyph is the placeholder character for the type/priority/state
	// metadata slots of a skeleton row. An "X" so the row reads like a real
	// row's metadata (e.g. "T" over "P1 OPN") rather than a featureless bar.
	SkeletonMetaGlyph = "X"

	// skeletonIDWidth is the bar a skeleton row draws where the ID goes.
	skeletonIDWidth = 8
)

// skeletonTitleFractions is the normative table of title fill widths for
// RenderCompactSkeleton. Six values hand-picked so successive rows show visibly
// different bar lengths and the column does not read as a uniform block.
// Indexed by ((Seed % 6) + 6) % 6 to handle negative seeds safely.
var skeletonTitleFractions = [6]float64{0.70, 0.45, 0.85, 0.55, 0.80, 0.65}

// SkeletonOpts configures skeleton row rendering.
type SkeletonOpts struct {
	Width  int
	Seed   int  // selects title fill width from the normative table
	Phase  int  // styles.SkeletonShades index; modulo applied internally
	Styled bool // when true, apply lipgloss muted foreground colour
}

// skeletonSegment renders one fixed-width segment by repeating glyph.
// When styled is true it applies the given foreground color via lipgloss.
func skeletonSegment(glyph string, width int, styled bool, color lipgloss.Color) string {
	block := strings.Repeat(glyph, width)
	if styled {
		return lipgloss.NewStyle().Foreground(color).Render(block)
	}
	return block
}

// skeletonColor returns the shade for the given phase index.
// N = len(styles.SkeletonShades); safe-modulo handles any integer phase.
func skeletonColor(phase int) lipgloss.Color {
	n := len(styles.SkeletonShades)
	idx := ((phase % n) + n) % n
	return styles.SkeletonShades[idx]
}

// RenderCompactSkeleton renders a placeholder shaped like RenderCompact: Height
// lines, each opts.Width cells wide. The type, priority and state slots are
// filled with SkeletonMetaGlyph ("X"); the title and the ID are SkeletonGlyph
// ("░") loading bars, the title bar's width varying by Seed so a column of rows
// does not read as a uniform block.
func RenderCompactSkeleton(opts SkeletonOpts) []string {
	width := opts.Width
	if width <= 0 {
		return make([]string, Height)
	}

	color := skeletonColor(opts.Phase)
	seg := func(glyph string, n int) string {
		return skeletonSegment(glyph, n, opts.Styled, color)
	}

	// Both lines: the gutter, then the type slot and a space; the title starts
	// after them and the second line is indented to it.
	const indent = 4
	titleWidth := width - indent
	if titleWidth < 1 {
		return []string{seg(SkeletonGlyph, width), seg(SkeletonGlyph, width)}
	}

	idx := ((opts.Seed % 6) + 6) % 6
	fillWidth := max(1, int(float64(titleWidth)*skeletonTitleFractions[idx]))
	first := "  " + seg(SkeletonMetaGlyph, 1) + " " + seg(SkeletonGlyph, fillWidth) +
		strings.Repeat(" ", titleWidth-fillWidth)

	// priority=2, state=3, one space after each.
	const metaWidth = 2 + 1 + 3 + 1
	idWidth := min(skeletonIDWidth, titleWidth-metaWidth)
	if idWidth < 1 {
		return []string{first, strings.Repeat(" ", indent) + seg(SkeletonMetaGlyph, titleWidth)}
	}
	second := strings.Repeat(" ", indent) + seg(SkeletonMetaGlyph, 2) + " " + seg(SkeletonMetaGlyph, 3) + " " +
		seg(SkeletonGlyph, idWidth) + strings.Repeat(" ", titleWidth-metaWidth-idWidth)

	return []string{first, second}
}

// RenderConfig configures compact issue row rendering.
type RenderConfig struct {
	// Issue uses domain.IssueSummary directly because compact rows need only
	// canonical summary fields (id/title/type/status/priority). This keeps board
	// and search on one data shape and removes adapter-only row structs.
	Issue    domain.IssueSummary
	Selected bool
	// Hovered marks the row under the pointer.
	Hovered bool
	Width   int
	Styled  bool

	// Dim, when true, applies a SkeletonShades foreground tint to the rendered
	// row text — used to signal that the surface is refreshing stale data.
	// Phase selects the shade index (modulo applied internally).
	// Selection-conflict rule: when Selected==true && Dim==true, the dim shade
	// is applied to the foreground text only; the selection indicator is
	// preserved unchanged so the selection highlight remains visually dominant.
	Dim   bool
	Phase int
}

// ReferenceRenderConfig configures compact related-issue row rendering.
type ReferenceRenderConfig struct {
	Issue    domain.IssueReference
	Selected bool
	// Hovered marks the row under the pointer.
	Hovered bool
	Width   int
	Styled  bool
}

// RenderCompact renders one issue as Height lines: the type and the title,
// then the priority, the status and the ID under the title. A styled row that
// is selected or under the pointer carries its band across the whole width of
// both lines, and the selection bar runs down both.
func RenderCompact(config RenderConfig) []string {
	prefixPlain, prefixStyled := styles.SelectionPrefix(config.Selected, config.Styled)
	textWidth := config.Width - lipgloss.Width(prefixPlain)

	title := strings.TrimSpace(config.Issue.Title)
	if title == "" {
		title = "(untitled)"
	}

	typePlain := renderhelpers.CompactIssueType(config.Issue.Type)
	indent := strings.Repeat(" ", lipgloss.Width(typePlain)+1)
	metaWidth := textWidth - lipgloss.Width(indent)
	meta := []string{
		renderhelpers.CompactPriority(config.Issue.Priority),
		renderhelpers.CompactIssueState(config.Issue.Status),
	}
	idWidth := metaWidth - lipgloss.Width(strings.Join(meta, " ")) - 1

	first := textutil.TruncateString(typePlain+" "+title, textWidth)
	second := textutil.TruncateString(indent+strings.Join(meta, " "), textWidth)
	if idWidth >= 1 {
		second = indent + strings.Join(append(meta, renderhelpers.CompactIssueID(config.Issue.ID, idWidth)), " ")
	}

	if config.Styled && textWidth > lipgloss.Width(indent) {
		first = renderhelpers.CompactIssueTypeStyled(config.Issue.Type) + " " +
			textutil.TruncateString(title, metaWidth)
		if idWidth >= 1 {
			second = indent + strings.Join([]string{
				renderhelpers.CompactPriorityStyled(config.Issue.Priority),
				renderhelpers.CompactIssueStateStyled(config.Issue.Status),
				renderhelpers.CompactIssueIDMuted(config.Issue.ID, idWidth),
			}, " ")
		}
	}

	lines := []string{first, second}
	for i, content := range lines {
		if config.Dim && config.Styled {
			// The tint goes on the content only: the selection bar stays as it
			// is, so the selection remains visually dominant.
			content = lipgloss.NewStyle().Foreground(skeletonColor(config.Phase)).Render(content)
		}
		lines[i] = prefixStyled + content
		if config.Styled {
			lines[i] = styles.RowHighlight(lines[i], config.Width, config.Selected, config.Hovered)
		}
	}
	return lines
}

// RenderReferenceCompact renders a one-line compact row for related issues,
// with the same band as RenderCompact. The detail panes that list relations
// are a few rows tall, so a relation keeps to one line.
func RenderReferenceCompact(config ReferenceRenderConfig) string {
	row := renderReferenceCompact(config)
	if !config.Styled {
		return row
	}
	return styles.RowHighlight(row, config.Width, config.Selected, config.Hovered)
}

func renderReferenceCompact(config ReferenceRenderConfig) string {
	prefixPlain, prefixStyled := styles.SelectionPrefix(config.Selected, config.Styled)

	title := strings.TrimSpace(config.Issue.Title)
	if title == "" {
		title = "(untitled)"
	}

	idWidth := CompactIDWidth(config.Width)
	metaPlain := strings.Join([]string{
		renderhelpers.CompactIssueType(config.Issue.Type),
		renderhelpers.CompactPriority(config.Issue.Priority),
		renderhelpers.CompactIssueStateNarrow(config.Issue.Status),
		renderhelpers.CompactIssueID(config.Issue.ID, idWidth),
	}, " ")
	metaStyled := metaPlain
	if config.Styled {
		metaStyled = strings.Join([]string{
			renderhelpers.CompactIssueTypeStyled(config.Issue.Type),
			renderhelpers.CompactPriorityStyled(config.Issue.Priority),
			renderhelpers.CompactIssueStateNarrowStyled(config.Issue.Status),
			renderhelpers.CompactIssueIDMuted(config.Issue.ID, idWidth),
		}, " ")
	}

	titlePrefix := prefixPlain + metaPlain + " "
	titleWidth := config.Width - lipgloss.Width(titlePrefix)
	if titleWidth < minNarrowTitleWidth {
		return textutil.TruncateString(prefixPlain+metaPlain, config.Width)
	}

	return prefixStyled + metaStyled + " " + textutil.TruncateString(title, titleWidth)
}

// CompactIDWidth returns the shared max width for compact issue IDs on a
// one-line row.
func CompactIDWidth(width int) int {
	return min(maxCompactIDWidth, max(minCompactIDWidth, width/5))
}
