package styles_test

// normalizeIssueToken in colors.go and renderhelpers.NormalizeToken are kept
// as separate copies because styles → renderhelpers → styles would be a cycle.
// This test pins their behaviour as byte-for-byte identical so that a future
// divergence is caught immediately.

import (
	"testing"

	"github.com/hk9890/task-manager-ui/internal/ui/shared/renderhelpers"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

func TestNormalizeIssueTokenParityWithRenderHelpers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "whitespace only", input: "   "},
		{name: "uppercase", input: "IN_PROGRESS"},
		{name: "lowercase", input: "ready"},
		{name: "mixed with spaces", input: " In Progress "},
		{name: "hyphen separator", input: "In-Progress"},
		{name: "special chars unchanged", input: "feat@v2"},
		{name: "multiple hyphens", input: "in-progress-now"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want := renderhelpers.NormalizeToken(tc.input)
			got := styles.NormalizeIssueToken(tc.input)
			if got != want {
				t.Fatalf("parity failure for %q: styles.normalizeIssueToken=%q, renderhelpers.NormalizeToken=%q",
					tc.input, got, want)
			}
		})
	}
}
