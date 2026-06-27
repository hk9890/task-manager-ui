package styles

import (
	"strings"
	"testing"
)

func TestFormSectionRendersBordersAndContent(t *testing.T) {
	got := FormSection(FormSectionConfig{
		Content:            []string{"Body"},
		Width:              20,
		TopLeft:            "Title",
		Focused:            true,
		FocusedBorderColor: BorderHighlightFocusColor,
	})

	for _, needle := range []string{"╭", "╮", "╰", "╯", "Title", "Body"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("expected output to contain %q", needle)
		}
	}
}

func TestFormSectionReturnsTooNarrowLiteralBelowWidth6(t *testing.T) {
	for _, w := range []int{0, 1, 3, 5} {
		got := FormSection(FormSectionConfig{
			Content: []string{"Body"},
			Width:   w,
			TopLeft: "Title",
		})
		if got != "too narrow" {
			t.Fatalf("width=%d: expected literal \"too narrow\", got %q", w, got)
		}
	}
}

func TestFormSectionRendersNormallyAtWidth6(t *testing.T) {
	got := FormSection(FormSectionConfig{
		Content: []string{"x"},
		Width:   6,
	})
	if got == "too narrow" {
		t.Fatalf("width=6: expected normal render, got \"too narrow\"")
	}
	if !strings.Contains(got, "╭") {
		t.Fatalf("width=6: expected border characters, got %q", got)
	}
}

func TestFormSectionPadsToRequestedHeight(t *testing.T) {
	got := FormSection(FormSectionConfig{
		Content: []string{"Body"},
		Width:   20,
		Height:  6,
		TopLeft: "Title",
	})

	lines := strings.Split(got, "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines, got %d:\n%s", len(lines), got)
	}
}
