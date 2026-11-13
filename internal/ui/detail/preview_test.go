package detail

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/domain"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func previewOf(description string) PreviewState {
	return PreviewState{
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-7", Title: "Auth redesign", Type: "doc", Status: "open"},
			Description: description,
		},
		Width:  60,
		Height: 12,
	}
}

func TestThePreviewDrawsTheHeaderAndTheBodyInItsBox(t *testing.T) {
	view := testui.Downsample(RenderPreview(previewOf("The body of the doc.")))

	testui.AssertContainsAll(t, view, "Preview", "tm-7", "Auth redesign", "The body of the doc.")
	testui.AssertNotContainsAny(t, view, "Description", "Comments", "edit")
	lines := strings.Split(view, "\n")
	if len(lines) != 12 {
		t.Fatalf("the pane is %d lines, want 12", len(lines))
	}
	for idx, line := range lines {
		if got := lipgloss.Width(line); got != 60 {
			t.Fatalf("line %d is %d cells, want 60:\n%s", idx, got, view)
		}
	}
}

func TestThePreviewScrollsAndMarksWhatItDoesNotShow(t *testing.T) {
	var paragraphs []string
	for i := 1; i <= 30; i++ {
		paragraphs = append(paragraphs, fmt.Sprintf("Line %02d.", i))
	}
	state := previewOf(strings.Join(paragraphs, "\n\n"))

	top := testui.Downsample(RenderPreview(state))
	testui.AssertContainsAll(t, top, "Line 01.", "more)")
	testui.AssertNotContainsAny(t, top, "earlier)")

	state.ScrollOffset = PreviewMaxOffset(state)
	bottom := testui.Downsample(RenderPreview(state))
	testui.AssertContainsAll(t, bottom, "Line 30.", "earlier)")
	testui.AssertNotContainsAny(t, bottom, "more)")

	if short := previewOf("One line."); PreviewMaxOffset(short) != 0 {
		t.Fatalf("a doc that fits scrolls to %d", PreviewMaxOffset(short))
	}
}

func TestThePreviewStatesInPlaceOfTheBody(t *testing.T) {
	empty := previewOf("")
	empty.Empty = true
	testui.AssertContainsAll(t, testui.Downsample(RenderPreview(empty)), "Nothing selected")

	failed := previewOf("The body of the doc.")
	failed.Error = "store is gone"
	view := testui.Downsample(RenderPreview(failed))
	testui.AssertContainsAll(t, view, "Auth redesign", "load failed: store is gone")
	testui.AssertNotContainsAny(t, view, "The body of the doc.")

	loading := previewOf("The body of the doc.")
	loading.Skeleton = true
	view = testui.Downsample(RenderPreview(loading))
	testui.AssertContainsAll(t, view, "Auth redesign", "░")
	testui.AssertNotContainsAny(t, view, "The body of the doc.")

	testui.AssertContainsAll(t, testui.Downsample(RenderPreview(previewOf(""))), "(no description)")
}
