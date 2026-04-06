package loading

import (
	"strings"
	"testing"
)

func TestViewBoardScope(t *testing.T) {
	view := View(State{Scope: ScopeBoard})
	if !strings.Contains(view, "Loading issues from gateway") {
		t.Fatalf("expected shared board loading message, got %q", view)
	}
}

func TestViewDetailScopeIncludesTarget(t *testing.T) {
	view := View(State{Scope: ScopeDetail, Target: "bw-23"})
	if !strings.Contains(view, "Loading details for bw-23") {
		t.Fatalf("expected shared detail loading message, got %q", view)
	}
}

func TestSummaryDeduplicatesScopes(t *testing.T) {
	summary := Summary([]State{{Scope: ScopeBoard}, {Scope: ScopeDetail, Target: "bw-1"}, {Scope: ScopeBoard}})
	if !strings.Contains(summary, "Loading: board, detail") {
		t.Fatalf("expected deduplicated loading summary, got %q", summary)
	}
}
