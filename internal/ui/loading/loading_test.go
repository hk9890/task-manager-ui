package loading

import (
	"strings"
	"testing"
)

func TestSkeletonPhaseAdvancesEveryFourFrames(t *testing.T) {
	// Frames 0-3 must all be in phase 0.
	if SkeletonPhase(0) != SkeletonPhase(3) {
		t.Fatalf("SkeletonPhase(0)=%d, SkeletonPhase(3)=%d: expected equal (same phase)", SkeletonPhase(0), SkeletonPhase(3))
	}
	// Frame 4 must be in the next phase.
	if SkeletonPhase(0) == SkeletonPhase(4) {
		t.Fatalf("SkeletonPhase(0)=%d, SkeletonPhase(4)=%d: expected different (phase boundary at frame 4)", SkeletonPhase(0), SkeletonPhase(4))
	}
	// Spot-check the divisor: frame/4.
	for frame, want := range map[int]int{
		0: 0, 1: 0, 2: 0, 3: 0,
		4: 1, 5: 1, 6: 1, 7: 1,
		8: 2, 12: 3, 16: 4,
	} {
		if got := SkeletonPhase(frame); got != want {
			t.Errorf("SkeletonPhase(%d) = %d, want %d", frame, got, want)
		}
	}
}

func TestSkeletonPhaseNegativeReturnsZero(t *testing.T) {
	// Negative frame values return 0; callers must pass non-negative spinnerFrame.
	if got := SkeletonPhase(-1); got != 0 {
		t.Fatalf("SkeletonPhase(-1) = %d, want 0", got)
	}
	if got := SkeletonPhase(-100); got != 0 {
		t.Fatalf("SkeletonPhase(-100) = %d, want 0", got)
	}
}

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
