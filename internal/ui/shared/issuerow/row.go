package issuerow

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/ui/shared/renderhelpers"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

const (
	minTitleWidth       = 8
	minNarrowTitleWidth = 4
	minCompactIDWidth   = 7
	maxCompactIDWidth   = 12

	// SkeletonGlyph is the canonical placeholder character for the loading-bar
	// segments (id + title) of a skeleton row. It is U+2591 LIGHT SHADE — light
	// enough to read as a placeholder rather than uniform noise.
	// Test assertions reference this constant so no caller hard-codes the literal rune.
	SkeletonGlyph = "░"

	// SkeletonMetaGlyph is the placeholder character for the type/priority/state
	// metadata slots of a skeleton row. An "X" so the left edge reads like a real
	// row's metadata columns (e.g. "T P1 OPN") rather than a featureless bar.
	SkeletonMetaGlyph = "X"
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
func skeletonSegment(glyph string, width int, styled bool, color lipgloss.AdaptiveColor) string {
	block := strings.Repeat(glyph, width)
	if styled {
		return lipgloss.NewStyle().Foreground(color).Render(block)
	}
	return block
}

// skeletonColor returns the lipgloss.AdaptiveColor for the given phase index.
// N = len(styles.SkeletonShades); safe-modulo handles any integer phase.
func skeletonColor(phase int) lipgloss.AdaptiveColor {
	n := len(styles.SkeletonShades)
	idx := ((phase % n) + n) % n
	return styles.SkeletonShades[idx]
}

// RenderCompactSkeleton renders a placeholder row shaped like RenderCompact.
// The type/priority/state slots are filled with SkeletonMetaGlyph ("X") so the
// left edge reads like a real row's metadata columns; the id and title slots are
// SkeletonGlyph ("░") loading bars, the title bar's width varying by Seed so a
// column of rows does not read as a uniform block. Segments are separated by
// single spaces so the structure mirrors a real issue row, and lipgloss.Width of
// the result equals opts.Width.
func RenderCompactSkeleton(opts SkeletonOpts) string {
	width := opts.Width
	if width <= 0 {
		return ""
	}

	// Fixed segment widths matching the real compact row slot layout.
	// type=1, priority=2, state=3 — same as CompactIssueType/Priority/State plain text widths.
	typeWidth := 1
	prioWidth := 2
	stateWidth := 3
	idWidth := CompactIDWidth(width)

	// Gaps: four single spaces between the five segments.
	const gaps = 4
	titleWidth := width - typeWidth - prioWidth - stateWidth - idWidth - gaps
	color := skeletonColor(opts.Phase)
	if titleWidth < 1 {
		// Terminal too narrow: fall back to a single full-width bar.
		return skeletonSegment(SkeletonGlyph, width, opts.Styled, color)
	}

	// Select the title bar fill fraction from the normative table.
	idx := ((opts.Seed % 6) + 6) % 6
	fillWidth := int(float64(titleWidth) * skeletonTitleFractions[idx])
	if fillWidth < 1 {
		fillWidth = 1
	}

	// Build segments left-to-right: type | priority | state | id | title.
	// type/priority/state use "X" metadata placeholders; id and title are bars.
	typeSeg := skeletonSegment(SkeletonMetaGlyph, typeWidth, opts.Styled, color)
	prioSeg := skeletonSegment(SkeletonMetaGlyph, prioWidth, opts.Styled, color)
	stateSeg := skeletonSegment(SkeletonMetaGlyph, stateWidth, opts.Styled, color)
	idSeg := skeletonSegment(SkeletonGlyph, idWidth, opts.Styled, color)
	titleFill := skeletonSegment(SkeletonGlyph, fillWidth, opts.Styled, color)
	titlePad := strings.Repeat(" ", titleWidth-fillWidth)

	return typeSeg + " " + prioSeg + " " + stateSeg + " " + idSeg + " " + titleFill + titlePad
}

// RenderConfig configures compact issue row rendering.
type RenderConfig struct {
	// Issue uses domain.IssueSummary directly because compact rows need only
	// canonical summary fields (id/title/type/status/priority). This keeps board
	// and search on one data shape and removes adapter-only row structs.
	Issue    domain.IssueSummary
	Selected bool
	Width    int
	Styled   bool

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
	Width    int
	Styled   bool
}

// RenderCompact renders one compact issue row with shared metadata semantics.
func RenderCompact(config RenderConfig) string {
	prefixPlain, prefixStyled := styles.SelectionPrefix(config.Selected, config.Styled)

	title := strings.TrimSpace(config.Issue.Title)
	if title == "" {
		title = "(untitled)"
	}

	idWidth := CompactIDWidth(config.Width)
	metaPlain := strings.Join([]string{
		renderhelpers.CompactIssueType(config.Issue.Type),
		renderhelpers.CompactPriority(config.Issue.Priority),
		renderhelpers.CompactIssueState(config.Issue.Status),
		renderhelpers.CompactIssueID(config.Issue.ID, idWidth),
	}, " ")
	metaStyled := metaPlain
	if config.Styled {
		metaStyled = strings.Join([]string{
			renderhelpers.CompactIssueTypeStyled(config.Issue.Type),
			renderhelpers.CompactPriorityStyled(config.Issue.Priority),
			renderhelpers.CompactIssueStateStyled(config.Issue.Status),
			renderhelpers.CompactIssueIDMuted(config.Issue.ID, idWidth),
		}, " ")
	}

	titlePrefix := prefixPlain + metaPlain + " "
	titleWidth := config.Width - lipgloss.Width(titlePrefix)
	if titleWidth < minTitleWidth {
		return styles.TruncateString(prefixPlain+metaPlain, config.Width)
	}

	content := metaStyled + " " + styles.TruncateString(title, titleWidth)
	if config.Dim && config.Styled {
		// Apply a SkeletonShades foreground tint to the row content only.
		// Selection-conflict rule: the prefix (prefixStyled) is left unchanged so
		// the selection indicator remains visually dominant.
		content = lipgloss.NewStyle().Foreground(skeletonColor(config.Phase)).Render(content)
	}
	return prefixStyled + content
}

// RenderReferenceCompact renders a one-line compact row for related issues.
func RenderReferenceCompact(config ReferenceRenderConfig) string {
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
		return styles.TruncateString(prefixPlain+metaPlain, config.Width)
	}

	return prefixStyled + metaStyled + " " + styles.TruncateString(title, titleWidth)
}

// CompactIDWidth returns the shared max width for compact issue IDs.
func CompactIDWidth(width int) int {
	return min(maxCompactIDWidth, max(minCompactIDWidth, width/5))
}
