package displaytext_test

import (
	"testing"

	"github.com/hk9890/task-manager-ui/internal/displaytext"
)

// The characters under test are built from their code points: written into a
// string literal they are invisible to whoever reads the file.
var (
	rightToLeftOverride = string(rune(0x202e))
	leftToRightIsolate  = string(rune(0x2066))
	byteOrderMark       = string(rune(0xfeff))
	lineSeparator       = string(rune(0x2028))
	paragraphSeparator  = string(rune(0x2029))
	zeroWidthSpace      = string(rune(0x200b))
	zeroWidthNonJoiner  = string(rune(0x200c))
	zeroWidthJoiner     = string(rune(0x200d))
	variationSelector16 = string(rune(0xfe0f))
	combiningAcute      = string(rune(0x0301))
	hangulFiller        = string(rune(0x3164))
	technologist        = string(rune(0x1f468)) + zeroWidthJoiner + string(rune(0x1f4bb))
	// The flag of England: a black flag and a tag sequence.
	taggedFlag = string([]rune{0x1f3f4, 0xe0067, 0xe0062, 0xe0065, 0xe006e, 0xe0067, 0xe007f})
)

func TestOneLine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, in, want string }{
		{"plain text keeps its spaces", "plain  text", "plain  text"},
		{"control characters become spaces", "a\nb\tc\x1b[2Jd", "a b c [2Jd"},
		{"separators become spaces", "a" + lineSeparator + "b" + paragraphSeparator + "c", "a b c"},
		{"reordering characters are dropped", "a" + rightToLeftOverride + "b" + leftToRightIsolate + "c" + byteOrderMark, "abc"},
		{"a joiner in an emoji sequence stays", technologist, technologist},
		{"a tag sequence stays", taggedFlag, taggedFlag},
		{"a non-joiner stays", "a" + zeroWidthNonJoiner + "b", "a" + zeroWidthNonJoiner + "b"},
	} {
		if got := displaytext.OneLine(tc.in); got != tc.want {
			t.Errorf("%s: OneLine(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestLinesKeepsTheLineFeeds: a toast may hold two lines the app wrote, each
// with a text from outside.
func TestLinesKeepsTheLineFeeds(t *testing.T) {
	t.Parallel()

	in := "Store a\tb is not usable\ncause: x\x1b[2Jy\r"
	if got, want := displaytext.Lines(in), "Store a b is not usable\ncause: x [2Jy "; got != want {
		t.Errorf("Lines(%q) = %q, want %q", in, got, want)
	}
}

func TestVisible(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]bool{
		"":                               false,
		" \t":                            false,
		zeroWidthJoiner + zeroWidthSpace: false,
		variationSelector16:              false,
		combiningAcute:                   false,
		hangulFiller:                     false,
		"a":                              true,
		"e" + combiningAcute:             true,
		technologist:                     true,
	} {
		if got := displaytext.Visible(in); got != want {
			t.Errorf("Visible(%q) = %v, want %v", in, got, want)
		}
	}
}
