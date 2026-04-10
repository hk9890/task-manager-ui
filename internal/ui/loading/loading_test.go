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

func TestViewScopeVariantsAreDistinct(t *testing.T) {
	t.Parallel()

	board := View(State{Scope: ScopeBoard})
	search := View(State{Scope: ScopeSearch})
	detail := View(State{Scope: ScopeDetail, Target: "bw-42"})

	if board != search {
		t.Fatalf("expected board/search to share issue-loading copy, got board=%q search=%q", board, search)
	}
	if detail == board {
		t.Fatalf("expected detail loading output to differ from board/search")
	}
}

func TestViewDetailScopeBlankTargetUsesFallback(t *testing.T) {
	t.Parallel()

	view := View(State{Scope: ScopeDetail, Target: "   "})
	if !strings.Contains(view, "Loading selected issue details") {
		t.Fatalf("expected detail fallback message for blank target, got %q", view)
	}
	if !strings.Contains(view, "⏳") {
		t.Fatalf("expected loading indicator in view, got %q", view)
	}
}

func TestSummaryBlankScopesReturnsIdle(t *testing.T) {
	t.Parallel()

	summary := Summary([]State{{Scope: Scope("   ")}, {Scope: Scope("")}})
	if !strings.Contains(summary, "Idle") {
		t.Fatalf("expected idle summary for blank scopes, got %q", summary)
	}
}
