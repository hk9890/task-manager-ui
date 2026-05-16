package issuerow

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hk9890/beads-workbench/internal/domain"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
)

func TestRenderCompactSelectionAndMetadata(t *testing.T) {
	line := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "beads-workbench-u5s",
			Title:    "Row renderer metadata",
			Type:     "task",
			Status:   "open",
			Priority: 1,
		},
		Selected: true,
		Width:    72,
	})

	if !strings.HasPrefix(line, "› ") {
		t.Fatalf("expected selected row prefix, got: %q", line)
	}
	if !strings.Contains(line, "T P1 OPN") {
		t.Fatalf("expected compact metadata tokens, got: %q", line)
	}
}

func TestRenderCompactTruncatesMetadataWhenVeryNarrow(t *testing.T) {
	line := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "beads-workbench-very-long-id",
			Title:    "Long title that should not fit",
			Type:     "feature",
			Status:   "in_progress",
			Priority: 0,
		},
		Width: 15,
	})

	if strings.Contains(line, "Long title") {
		t.Fatalf("expected title to be omitted when metadata consumes width, got: %q", line)
	}
	if !strings.Contains(line, "F P0 IP") {
		t.Fatalf("expected metadata retained in narrow row, got: %q", line)
	}
}

func TestRenderCompactStyledIncludesANSI(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	line := RenderCompact(RenderConfig{
		Issue: domain.IssueSummary{
			ID:       "beads-workbench-u5s",
			Title:    "Styled row",
			Type:     "bug",
			Status:   "blocked",
			Priority: 0,
		},
		Selected: true,
		Width:    64,
		Styled:   true,
	})

	if !strings.Contains(line, "\x1b[") {
		t.Fatalf("expected ANSI styling when Styled is true, got: %q", line)
	}
	plain := testui.AnsiEscapePattern.ReplaceAllString(line, "")
	if !strings.Contains(plain, "› B P0 BLK u5s") {
		t.Fatalf("expected styled metadata to preserve token text, got: %q", plain)
	}
}

func TestCompactIDWidthUsesSharedBoundedRule(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{width: 10, want: 7},
		{width: 45, want: 9},
		{width: 60, want: 12},
		{width: 200, want: 12},
	}

	for _, tc := range tests {
		if got := CompactIDWidth(tc.width); got != tc.want {
			t.Fatalf("CompactIDWidth(%d) = %d, want %d", tc.width, got, tc.want)
		}
	}
}

func TestRenderReferenceCompactNarrowWidthsRemainReadable(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{name: "width 22", width: 22},
		{name: "width 24", width: 24},
		{name: "width 28", width: 28},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			line := RenderReferenceCompact(ReferenceRenderConfig{
				Issue: domain.IssueReference{
					ID:       "beads-workbench-syf.3",
					Title:    "Add narrow related issue row renderer for left rail",
					Type:     "task",
					Priority: 3,
					Status:   "in_progress",
				},
				Selected: true,
				Width:    tc.width,
			})

			if strings.Contains(line, "\n") {
				t.Fatalf("expected one-line row at width %d, got %q", tc.width, line)
			}
			if lipgloss.Width(line) > tc.width {
				t.Fatalf("expected row width <= %d, got %d: %q", tc.width, lipgloss.Width(line), line)
			}
			if !strings.Contains(line, "T P3 I") {
				t.Fatalf("expected type/priority/status compact tokens at width %d, got %q", tc.width, line)
			}
			if !strings.Contains(line, "syf.3") {
				t.Fatalf("expected compact issue id token at width %d, got %q", tc.width, line)
			}
		})
	}
}

func TestRenderReferenceCompactSelectionDistinct(t *testing.T) {
	issue := domain.IssueReference{
		ID:       "beads-workbench-9uk",
		Title:    "Selection contrast check",
		Type:     "bug",
		Priority: 1,
		Status:   "blocked",
	}

	selected := RenderReferenceCompact(ReferenceRenderConfig{Issue: issue, Selected: true, Styled: true, Width: 28})
	idle := RenderReferenceCompact(ReferenceRenderConfig{Issue: issue, Selected: false, Styled: true, Width: 28})

	if selected == idle {
		t.Fatalf("expected selected and unselected rows to differ, got selected=%q idle=%q", selected, idle)
	}

	selectedPlain := testui.AnsiEscapePattern.ReplaceAllString(selected, "")
	idlePlain := testui.AnsiEscapePattern.ReplaceAllString(idle, "")

	if !strings.HasPrefix(selectedPlain, "› ") {
		t.Fatalf("expected selected row indicator prefix, got %q", selectedPlain)
	}
	if !strings.HasPrefix(idlePlain, "  ") {
		t.Fatalf("expected unselected row idle prefix, got %q", idlePlain)
	}
}
