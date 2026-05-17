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

	// SkeletonGlyph is the canonical placeholder character used in skeleton rows.
	// Test assertions reference this constant so no caller hard-codes the literal rune.
	SkeletonGlyph = "▓"
)

// skeletonTitleFractions is the normative table of title fill widths for
// RenderCompactSkeleton.  Six values hand-picked so the average (≈ 0.66) matches
// the median real-title length / available width in a 4-column board layout.
// Indexed by ((Seed % 6) + 6) % 6 to handle negative seeds safely.
var skeletonTitleFractions = [6]float64{0.70, 0.45, 0.85, 0.55, 0.80, 0.65}

// SkeletonOpts configures skeleton row rendering.
type SkeletonOpts struct {
	Width  int
	Seed   int  // selects title fill width from the normative table
	Phase  int  // styles.SkeletonShades index; modulo applied internally
	Styled bool // when true, apply lipgloss muted foreground colour
}

// skeletonSegment renders one fixed-width segment of SkeletonGlyph characters.
// When styled is true it applies the given foreground color via lipgloss.
func skeletonSegment(width int, styled bool, color lipgloss.AdaptiveColor) string {
	block := strings.Repeat(SkeletonGlyph, width)
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
// It emits five ▓-filled segments (type=1, priority=2, state=3, id=CompactIDWidth,
// title=variable) separated by single spaces so the visual structure mirrors a
// real issue row.  lipgloss.Width of the result equals opts.Width.
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
		// Terminal too narrow: fall back to a single full-width block.
		return skeletonSegment(width, opts.Styled, color)
	}

	// Select title fill fraction from the normative table.
	idx := ((opts.Seed % 6) + 6) % 6
	fraction := skeletonTitleFractions[idx]
	fillWidth := int(float64(titleWidth) * fraction)
	if fillWidth < 1 {
		fillWidth = 1
	}

	// Build segments left-to-right: type | priority | state | id | title.
	typeSeg := skeletonSegment(typeWidth, opts.Styled, color)
	prioSeg := skeletonSegment(prioWidth, opts.Styled, color)
	stateSeg := skeletonSegment(stateWidth, opts.Styled, color)
	idSeg := skeletonSegment(idWidth, opts.Styled, color)
	titleFill := skeletonSegment(fillWidth, opts.Styled, color)
	titlePad := strings.Repeat(" ", titleWidth-fillWidth)

	return typeSeg + " " + prioSeg + " " + stateSeg + " " + idSeg + " " + titleFill + titlePad
}

// RenderReferenceCompactSkeleton renders a placeholder row shaped like
// RenderReferenceCompact.  It uses the same slot arithmetic and normative
// title-fill table as RenderCompactSkeleton.
func RenderReferenceCompactSkeleton(opts SkeletonOpts) string {
	// ReferenceCompact uses the same slot widths as the regular compact row
	// (type=1, priority=2, state=3, id=CompactIDWidth) so the implementation
	// is identical.
	return RenderCompactSkeleton(opts)
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

	return prefixStyled + metaStyled + " " + styles.TruncateString(title, titleWidth)
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
