package detail

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// proseSkeletonBarFractions is the normative table of bar-fill widths used by
// renderProseContentSkeleton.  Seven values produce visibly different bar
// lengths so the column does not read as a uniform block.
var proseSkeletonBarFractions = [7]float64{0.85, 0.70, 0.55, 0.80, 0.65, 0.75, 0.90}

// renderProseContentSkeleton renders n prose-style placeholder lines for the
// Content pane description body during the loading skeleton.  The shape is:
//
//	heading bar (≈55 % width) ← shorter bar signals a section heading
//	blank line                ← visual gap before the first paragraph block
//	paragraph lines (varying widths)
//	blank line                ← gap between paragraph blocks
//	… (pattern repeats until n lines are filled)
//
// The ░ glyphs are styled with styles.SkeletonShades[phase] (same animation
// language as the Dependencies rail and board skeletons).
// Total line count is always n regardless of width or phase.
func renderProseContentSkeleton(width, n, phase int) []string {
	if n <= 0 {
		return nil
	}

	// Resolve the phase-keyed skeleton colour from the shared shades table.
	numShades := len(styles.SkeletonShades)
	idx := ((phase % numShades) + numShades) % numShades
	color := styles.SkeletonShades[idx]
	barStyle := lipgloss.NewStyle().Foreground(color)

	barLine := func(fraction float64) string {
		barWidth := int(float64(width) * fraction)
		if barWidth < 1 {
			barWidth = 1
		}
		if barWidth > width {
			barWidth = width
		}
		return barStyle.Render(strings.Repeat(issuerow.SkeletonGlyph, barWidth))
	}

	// Cycle through: heading, blank, para, para, para, blank, para, para, para, blank, …
	// "heading" uses fraction 0.55; subsequent para lines cycle through proseSkeletonBarFractions.
	//
	// The pattern is encoded as a flat []float64 sentinel slice where -1 = blank
	// and -2 = heading (fraction 0.55).
	const headingFraction = 0.55
	const blankSentinel = -1.0

	// Fixed leading segment: heading then blank.
	pattern := make([]float64, 0, 8)
	pattern = append(pattern, headingFraction, blankSentinel)
	// Then cycles of three para lines followed by a blank.
	for i := 0; i < 3; i++ {
		pattern = append(pattern,
			proseSkeletonBarFractions[i*3%7],
			proseSkeletonBarFractions[(i*3+1)%7],
			proseSkeletonBarFractions[(i*3+2)%7],
			blankSentinel,
		)
	}
	// pattern now has 2 + 4*3 = 14 entries; we'll cycle it below.

	lines := make([]string, 0, n)
	pi := 0
	for len(lines) < n {
		entry := pattern[pi%len(pattern)]
		pi++
		if entry == blankSentinel {
			lines = append(lines, "")
		} else {
			lines = append(lines, barLine(entry))
		}
	}
	return lines[:n]
}

// skeletonDetail returns a synthetic IssueDetail for cold-start skeleton
// rendering.  Only Summary.ID is set; everything else is empty/zero so the
// Dependencies and Metadata panes render natural empty frames.
func skeletonDetail(targetID string) domain.IssueDetail {
	return domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       targetID,
			Title:    "",
			Status:   "",
			Priority: -1,
			Type:     "",
		},
	}
}

// renderColdStartSkeleton renders the full 3-pane layout for the cold-start
// case (no prior detail loaded).  It routes through renderResponsiveLayout /
// renderThreePane with Skeleton=true so the layout is identical to a loaded
// detail render and there is no visible jump when data arrives.
func renderColdStartSkeleton(targetID string, width, height, skeletonPhase int) string {
	if width <= 0 {
		width = defaultDetailWidth
	}
	if height <= 0 {
		height = defaultDetailHeight
	}
	detail := skeletonDetail(targetID)
	skeletonState := State{
		Loading:       true,
		Skeleton:      true,
		SkeletonPhase: skeletonPhase,
		Detail:        detail,
		Width:         width,
		Height:        height,
	}
	if usesResponsiveDetailLayout(width) {
		return renderResponsiveLayout(detail, skeletonState, width, height)
	}
	return renderThreePane(detail, skeletonState, width, height)
}
