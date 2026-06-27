package overlay

import (
	"strings"
	"testing"
)

func TestPlace_MainRenderScenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		fg   string
		bg   string
		want string
	}{
		{
			name: "plain text overlay on plain text background",
			cfg:  Config{Width: 10, Height: 3, Position: Center},
			fg:   "OK",
			bg:   "..........\n..........\n..........",
			want: "..........\n....OK....\n..........",
		},
		{
			name: "overlay wider than background handled gracefully",
			cfg:  Config{Width: 5, Height: 1, Position: Center},
			fg:   "LONGER",
			bg:   "abcde",
			want: "LONGER",
		},
		{
			name: "overlay taller than background handled gracefully",
			cfg:  Config{Width: 5, Height: 2, Position: Center},
			fg:   "12\n34\n56",
			bg:   "aaaaa",
			want: "a12aa\n 34  ",
		},
		{
			name: "empty overlay on empty background",
			cfg:  Config{Width: 0, Height: 0, Position: Center},
			fg:   "",
			bg:   "",
			want: "",
		},
		{
			name: "single-line overlay on multi-line background",
			cfg:  Config{Width: 8, Height: 4, Position: Bottom},
			fg:   "HI",
			bg:   "........\n........\n........\n........",
			want: "........\n........\n........\n...HI...",
		},
		{
			name: "multi-line overlay with varying line widths",
			cfg:  Config{Width: 8, Height: 4, Position: Center},
			fg:   "A\nBBBB\nCC",
			bg:   "........\n........\n........\n........",
			want: "..A.....\n..BBBB..\n..CC....\n........",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Place(tc.cfg, tc.fg, tc.bg)
			if got != tc.want {
				t.Fatalf("Place() mismatch\nwant:\n%q\ngot:\n%q", tc.want, got)
			}
		})
	}
}

func TestPlace_PositionVariantsAcrossBackgroundSizes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		bg   string
		want string
	}{
		{
			name: "center on 10x5",
			cfg:  Config{Width: 10, Height: 5, Position: Center},
			bg:   rect(10, 5, "."),
			want: "..........\n..........\n....XX....\n..........\n..........",
		},
		{
			name: "center on 7x3",
			cfg:  Config{Width: 7, Height: 3, Position: Center},
			bg:   rect(7, 3, "."),
			want: ".......\n..XX...\n.......",
		},
		{
			name: "bottom on 10x5",
			cfg:  Config{Width: 10, Height: 5, Position: Bottom},
			bg:   rect(10, 5, "."),
			want: "..........\n..........\n..........\n..........\n....XX....",
		},
		{
			name: "bottom on 7x3",
			cfg:  Config{Width: 7, Height: 3, Position: Bottom},
			bg:   rect(7, 3, "."),
			want: ".......\n.......\n..XX...",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Place(tc.cfg, "XX", tc.bg)
			if got != tc.want {
				t.Fatalf("Place() position mismatch\nwant:\n%q\ngot:\n%q", tc.want, got)
			}
		})
	}
}

func TestPlace_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		fg   string
		bg   string
		want string
	}{
		{
			name: "zero-width background",
			cfg:  Config{Width: 0, Height: 3, Position: Center},
			fg:   "X",
			bg:   "\n\n",
			want: "\nX\n",
		},
		{
			name: "zero-height background",
			cfg:  Config{Width: 5, Height: 0, Position: Center},
			fg:   "X",
			bg:   "",
			want: "  X",
		},
		{
			name: "overlay same size as background",
			cfg:  Config{Width: 4, Height: 2, Position: Center},
			fg:   "1111\n2222",
			bg:   "aaaa\nbbbb",
			want: "1111\n2222",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Place(tc.cfg, tc.fg, tc.bg)
			if got != tc.want {
				t.Fatalf("Place() edge-case mismatch\nwant:\n%q\ngot:\n%q", tc.want, got)
			}
		})
	}
}

func rect(width, height int, fill string) string {
	if height <= 0 {
		return ""
	}

	line := strings.Repeat(fill, width)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = line
	}

	return strings.Join(lines, "\n")
}
