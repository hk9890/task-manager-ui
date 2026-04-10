package styles

import "testing"

func TestTruncateString(t *testing.T) {
	if got := TruncateString("hello", 10); got != "hello" {
		t.Fatalf("unexpected non-truncated value: %q", got)
	}
	if got := TruncateString("hello world", 5); got != "he..." {
		t.Fatalf("unexpected truncated value: %q", got)
	}
}
