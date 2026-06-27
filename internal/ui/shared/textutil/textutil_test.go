package textutil_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
)

func TestClamp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		value, low, high, want int
	}{
		{5, 0, 10, 5},   // in range
		{-3, 0, 10, 0},  // below low
		{42, 0, 10, 10}, // above high
		{0, 0, 10, 0},   // at low
		{10, 0, 10, 10}, // at high
	}
	for _, tc := range cases {
		if got := textutil.Clamp(tc.value, tc.low, tc.high); got != tc.want {
			t.Errorf("Clamp(%d, %d, %d) = %d, want %d", tc.value, tc.low, tc.high, got, tc.want)
		}
	}
}

func TestStripANSI(t *testing.T) {
	t.Parallel()
	// Literal SGR sequences so the test does not depend on lipgloss emitting
	// color codes (it suppresses them without a TTY/color profile).
	styled := "\x1b[1m\x1b[31mhello\x1b[0m world\x1b[0m"
	if got := textutil.StripANSI(styled); got != "hello world" {
		t.Errorf("StripANSI(%q) = %q, want %q", styled, got, "hello world")
	}
	if got := textutil.StripANSI("plain"); got != "plain" {
		t.Errorf("StripANSI(plain) = %q, want plain", got)
	}
}

func TestPadToWidth(t *testing.T) {
	t.Parallel()
	// Shorter value is right-padded to the target rendered width.
	got := textutil.PadToWidth("ab", 5)
	if lipgloss.Width(got) != 5 {
		t.Errorf("PadToWidth(ab,5) width = %d, want 5 (%q)", lipgloss.Width(got), got)
	}
	if !strings.HasPrefix(got, "ab") {
		t.Errorf("PadToWidth(ab,5) = %q, want prefix ab", got)
	}
	// Already-wide value is truncated to the target width.
	wide := textutil.PadToWidth("abcdef", 3)
	if lipgloss.Width(wide) > 3 {
		t.Errorf("PadToWidth(abcdef,3) width = %d, want <= 3 (%q)", lipgloss.Width(wide), wide)
	}
}
