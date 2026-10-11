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

// TestFormSectionActionCellsAreWhereTheBorderDrawsTheAction reads the cells
// back from the drawn border, with and without a right title.
func TestFormSectionActionCellsAreWhereTheBorderDrawsTheAction(t *testing.T) {
	for _, cfg := range []FormSectionConfig{
		{Width: 60, TopLeft: "Content", TopAction: " edit alt+u ", TopRight: "2 comments"},
		{Width: 40, TopLeft: "Content", TopAction: " edit alt+u "},
	} {
		top := []rune(strings.Split(FormSection(cfg), "\n")[0])
		if len(top) != cfg.Width {
			t.Fatalf("the top border is %d cells wide, want %d: %q", len(top), cfg.Width, string(top))
		}
		x0, x1, ok := FormSectionActionCells(cfg)
		if !ok {
			t.Fatalf("no action cells for %q", string(top))
		}
		if got := string(top[x0:x1]); got != cfg.TopAction {
			t.Errorf("cells %d to %d hold %q, want the action %q in %q", x0, x1, got, cfg.TopAction, string(top))
		}
		if top[x0-1] != '─' || top[x1] != '─' {
			t.Errorf("the action does not stand between two dashes: %q", string(top))
		}
	}
}

// TestFormSectionDropsTheActionWhenTheBorderHasNoRoom: a dash must stay
// between the action and what follows it.
func TestFormSectionDropsTheActionWhenTheBorderHasNoRoom(t *testing.T) {
	cfg := FormSectionConfig{TopLeft: "Content", TopAction: " edit alt+u ", TopRight: "2 comments"}
	// The corner, "─ Content ", a dash, the action, a dash, " 2 comments ─", the corner.
	fits := 1 + 10 + 1 + len(cfg.TopAction) + 1 + 13 + 1

	cfg.Width = fits
	if _, _, ok := FormSectionActionCells(cfg); !ok || !strings.Contains(FormSection(cfg), "edit") {
		t.Errorf("at width %d the action has room and is not drawn:\n%s", fits, FormSection(cfg))
	}

	cfg.Width = fits - 1
	if _, _, ok := FormSectionActionCells(cfg); ok || strings.Contains(FormSection(cfg), "edit") {
		t.Errorf("at width %d the action is drawn without a dash after it:\n%s", fits-1, FormSection(cfg))
	}
	if got := len([]rune(strings.Split(FormSection(cfg), "\n")[0])); got != cfg.Width {
		t.Errorf("without the action the border is %d cells wide, want %d", got, cfg.Width)
	}

	cfg.TopLeft, cfg.Width = "", 60
	if _, _, ok := FormSectionActionCells(cfg); ok {
		t.Error("an action without a left title reports cells")
	}
}
