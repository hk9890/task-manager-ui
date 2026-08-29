package detail

// The focus guard in RenderMetadataPane: a selected field is only drawn as
// selected while the pane holds focus. Both halves were unasserted, so
// inverting the guard left the whole repository green.

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
)

func metadataPaneFixture() domain.IssueDetail {
	return domain.IssueDetail{
		Summary: domain.IssueSummary{
			ID:       "tm-5",
			Title:    "Focus fixture",
			Status:   "open",
			Type:     "task",
			Priority: 1,
		},
	}
}

// countSelectionChevrons reports how many rows carry the selection gutter.
// The chevron is styled, so the rendered pane is stripped before counting.
func countSelectionChevrons(pane string) int {
	return strings.Count(textutil.StripANSI(pane), "› ")
}

// TestRenderMetadataPaneShowsTheSelectedFieldOnlyWhileFocused pins both
// directions of the guard at once. Search mode passes a live
// MetadataSelectedField with Focused=false on every render, so the
// unfocused-but-selected case is reachable in production, not hypothetical:
// inverting the guard puts the cursor on a pane the keys do not drive.
func TestRenderMetadataPaneShowsTheSelectedFieldOnlyWhileFocused(t *testing.T) {
	t.Parallel()

	base := MetadataPaneState{
		Detail:        metadataPaneFixture(),
		Width:         44,
		Height:        16,
		SelectedField: MetadataFieldStatus,
	}

	focused := base
	focused.Focused = true
	unfocused := base
	unfocused.Focused = false

	focusedPane := RenderMetadataPane(focused)
	unfocusedPane := RenderMetadataPane(unfocused)

	if got := countSelectionChevrons(focusedPane); got != 1 {
		t.Errorf("focused pane with a selected field drew %d selection chevrons, want 1:\n%s", got, focusedPane)
	}
	if got := countSelectionChevrons(unfocusedPane); got != 0 {
		t.Errorf("unfocused pane drew %d selection chevrons, want 0:\n%s", got, unfocusedPane)
	}
}

// A focused pane with no field selected must also draw no chevron, so the
// assertion above cannot be satisfied by a pane that marks a row whenever it
// holds focus.
func TestRenderMetadataPaneDrawsNoChevronWithoutASelectedField(t *testing.T) {
	t.Parallel()

	pane := RenderMetadataPane(MetadataPaneState{
		Detail:        metadataPaneFixture(),
		Width:         44,
		Height:        16,
		Focused:       true,
		SelectedField: MetadataFieldNone,
	})

	if got := countSelectionChevrons(pane); got != 0 {
		t.Errorf("focused pane with no selected field drew %d chevrons, want 0:\n%s", got, pane)
	}
}
