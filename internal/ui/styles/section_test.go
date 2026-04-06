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
