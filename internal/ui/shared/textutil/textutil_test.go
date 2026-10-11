package textutil_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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

func TestTruncateString(t *testing.T) {
	if got := textutil.TruncateString("hello", 10); got != "hello" {
		t.Fatalf("unexpected non-truncated value: %q", got)
	}
	// Unicode ellipsis (…, width 1) replaces the old ASCII "..." tail so one
	// more character of content fits in the same rendered width.
	if got := textutil.TruncateString("hello world", 5); got != "hell…" {
		t.Fatalf("unexpected truncated value: %q", got)
	}
}

func TestTruncateStringPreservesWellFormedANSI(t *testing.T) {
	t.Parallel()

	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(strings.Repeat("x", 64))
	truncated := textutil.TruncateString(styled, 18)

	if got := lipgloss.Width(truncated); got != 18 {
		t.Fatalf("expected display width 18, got %d", got)
	}

	stripped := ansi.Strip(truncated)
	if strings.Contains(stripped, "\x1b") {
		t.Fatalf("expected no dangling/incomplete ANSI escapes after truncation, got %q", truncated)
	}

	if !strings.HasSuffix(stripped, "…") {
		t.Fatalf("expected ellipsis suffix after truncation, got %q", stripped)
	}
}

func TestTruncateStringFront(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		s        string
		maxWidth int
		want     string
	}{
		{name: "fits", s: "hello", maxWidth: 10, want: "hello"},
		{name: "exact fit", s: "hello", maxWidth: 5, want: "hello"},
		{name: "cut keeps the tail", s: "hello world", maxWidth: 5, want: "…orld"},
		{name: "one cell over", s: "abcdef", maxWidth: 5, want: "…cdef"},
		{name: "width 2", s: "abcdef", maxWidth: 2, want: "…f"},
		{name: "width 1 is the ellipsis", s: "abcdef", maxWidth: 1, want: "…"},
		{name: "width 1 that fits", s: "a", maxWidth: 1, want: "a"},
		{name: "width 0", s: "abcdef", maxWidth: 0, want: ""},
		{name: "negative width", s: "abcdef", maxWidth: -3, want: ""},
		{name: "empty", s: "", maxWidth: 4, want: ""},
		{name: "wide runes that fit", s: "世界", maxWidth: 4, want: "世界"},
		{name: "wide runes cut on a rune edge", s: "世界世界", maxWidth: 5, want: "…世界"},
		{name: "wide rune that straddles the cut is dropped", s: "世界世界", maxWidth: 4, want: "…界"},
		{name: "wide rune with no room beside the ellipsis", s: "a世", maxWidth: 2, want: "…"},
		{name: "wide rune after narrow ones", s: "abc世", maxWidth: 3, want: "…世"},
	}
	for _, tc := range cases {
		got := textutil.TruncateStringFront(tc.s, tc.maxWidth)
		if got != tc.want {
			t.Errorf("%s: TruncateStringFront(%q, %d) = %q, want %q", tc.name, tc.s, tc.maxWidth, got, tc.want)
		}
		if width := lipgloss.Width(got); tc.maxWidth >= 0 && width > tc.maxWidth {
			t.Errorf("%s: result %q is %d cells wide, over %d", tc.name, got, width, tc.maxWidth)
		}
	}
}

func TestTruncateStringFrontPreservesWellFormedANSI(t *testing.T) {
	t.Parallel()

	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(strings.Repeat("x", 63) + "y")
	truncated := textutil.TruncateStringFront(styled, 18)

	if got := lipgloss.Width(truncated); got != 18 {
		t.Fatalf("expected display width 18, got %d", got)
	}
	if got, want := ansi.Strip(truncated), "…"+strings.Repeat("x", 16)+"y"; got != want {
		t.Fatalf("stripped value %q, want %q", got, want)
	}
}

func TestWrapLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		maxWidth int
		wantLen  int    // expected number of returned lines
		wantLine string // substring that must appear in the first line (empty = no check)
	}{
		{
			name:     "short string fits in one line",
			input:    "hello",
			maxWidth: 20,
			wantLen:  1,
			wantLine: "hello",
		},
		{
			name:     "empty string returns one line",
			input:    "",
			maxWidth: 20,
			wantLen:  1,
			wantLine: "",
		},
		{
			name:     "maxWidth below 1 returns single empty line",
			input:    "hello",
			maxWidth: 0,
			wantLen:  1,
			wantLine: "",
		},
		{
			name:     "maxWidth of -1 returns single empty line",
			input:    "something long",
			maxWidth: -1,
			wantLen:  1,
			wantLine: "",
		},
		{
			name:     "long string wraps to multiple lines",
			input:    "the quick brown fox jumps over the lazy dog",
			maxWidth: 15,
			wantLen:  3, // "the quick brown", "fox jumps over", "the lazy dog" (actual split depends on ansi.Wrap)
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := textutil.WrapLines(tc.input, tc.maxWidth)
			if len(got) == 0 {
				t.Fatalf("textutil.WrapLines(%q, %d) returned empty slice", tc.input, tc.maxWidth)
			}
			if tc.wantLen > 0 && len(got) != tc.wantLen {
				t.Errorf("textutil.WrapLines(%q, %d) returned %d lines, want %d: %v",
					tc.input, tc.maxWidth, len(got), tc.wantLen, got)
			}
			if tc.wantLine != "" && got[0] != tc.wantLine {
				t.Errorf("textutil.WrapLines(%q, %d) first line = %q, want %q",
					tc.input, tc.maxWidth, got[0], tc.wantLine)
			}
		})
	}
}
