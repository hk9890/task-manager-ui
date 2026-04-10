package issuerow

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/muesli/termenv"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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
	plain := ansiEscapePattern.ReplaceAllString(line, "")
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
