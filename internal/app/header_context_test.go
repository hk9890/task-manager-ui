package app

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
)

// TestHeaderContextPicksTheLongestVariantInsideItsBudget pins the header
// context fit rule. headerContextVariants builds four to six alternatives and
// only the chosen one reaches the screen, so nothing observed which budget the
// choice was made against: a doubled budget lets the context string consume the
// whole header row and crowd out the title and the tab strip.
//
// Asserted as the rule rather than as a snapshot: no golden is added, so the
// app captures do not gain two more files that churn on every header change.
func TestHeaderContextPicksTheLongestVariantInsideItsBudget(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "A ready issue with a reasonably long title", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)

	// Widths spanning the point where each successive variant stops fitting.
	for _, width := range []int{40, 60, 80, 100, 120, 160, 200} {
		m.width = width
		variants := m.headerContextVariants()
		if len(variants) == 0 {
			t.Fatalf("width %d: no header context variants to choose from", width)
		}

		got := m.headerContext()
		budget := width / 2

		// The chosen variant is the first one inside the budget, or the last
		// variant when none fits.
		want := variants[len(variants)-1]
		for _, v := range variants {
			if lipgloss.Width(v) <= budget {
				want = v
				break
			}
		}
		if got != want {
			t.Errorf("width %d (budget %d): headerContext = %q (width %d), want %q (width %d)",
				width, budget, got, lipgloss.Width(got), want, lipgloss.Width(want))
		}

		// The budget itself: whenever some variant fits in half the header, the
		// chosen one must too. This is the half that a doubled budget breaks.
		anyFits := false
		for _, v := range variants {
			if lipgloss.Width(v) <= budget {
				anyFits = true
				break
			}
		}
		if anyFits && lipgloss.Width(got) > budget {
			t.Errorf("width %d: chosen context is %d cells wide, over the %d-cell budget: %q",
				width, lipgloss.Width(got), budget, got)
		}
	}
}

// TestHeaderContextFallsBackToTheFirstVariantAtAnUnknownWidth covers the
// pre-resize path, where no budget exists yet.
func TestHeaderContextFallsBackToTheFirstVariantAtAnUnknownWidth(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)
	m.width = 0

	variants := m.headerContextVariants()
	if got := m.headerContext(); got != variants[0] {
		t.Errorf("headerContext at width 0 = %q, want the longest variant %q", got, variants[0])
	}
}
