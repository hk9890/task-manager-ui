package loading

import (
	"strings"
	"testing"
)

func TestSummaryDeduplicatesScopes(t *testing.T) {
	summary := Summary([]State{{Scope: ScopeBoard}, {Scope: ScopeDetail, Target: "bw-1"}, {Scope: ScopeBoard}})
	if !strings.Contains(summary, "Loading: board, detail") {
		t.Fatalf("expected deduplicated loading summary, got %q", summary)
	}
}

func TestSummaryBlankScopesReturnsIdle(t *testing.T) {
	t.Parallel()

	summary := Summary([]State{{Scope: Scope("   ")}, {Scope: Scope("")}})
	if !strings.Contains(summary, "Idle") {
		t.Fatalf("expected idle summary for blank scopes, got %q", summary)
	}
}
