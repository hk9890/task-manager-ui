package styles

import "testing"

func TestFormatCommentIndicator(t *testing.T) {
	if got := FormatCommentIndicator(0); got != "" {
		t.Fatalf("expected empty for zero, got %q", got)
	}
	if got := FormatCommentIndicator(3); got != "3💬" {
		t.Fatalf("expected 3💬, got %q", got)
	}
}

func TestTruncateString(t *testing.T) {
	if got := TruncateString("hello", 10); got != "hello" {
		t.Fatalf("unexpected non-truncated value: %q", got)
	}
	if got := TruncateString("hello world", 5); got != "he..." {
		t.Fatalf("unexpected truncated value: %q", got)
	}
}
