//go:build integration

package loadgen

import (
	"sort"
	"testing"
)

// Note: TestGenerate_EndToEnd and TestGenerate_Determinism have been converted
// to fake-runner unit tests in generate_test.go (TestGenerate_EndToEnd_Unit,
// TestGenerate_Determinism_Unit). The single real-bd E2E smoke test covering
// bd-version capture lives in measure_integration_test.go (TestMeasure_EndToEnd).

// TestGenerate_BlockerInvariant verifies that every blocked issue in a
// Generated repo has at least one incoming dep via the plan (checked against
// the plan, since inspecting bd's dep state requires bd dep list per issue).
// This test uses buildPlan only and forks no real bd subprocesses.
func TestGenerate_BlockerInvariant(t *testing.T) {
	spec := makeSpec(10, 0, 4, 0, 1.0, 17)
	p := buildPlan(spec)

	incoming := make(map[int][]int)
	for _, e := range p.edges {
		incoming[e.blockedIdx] = append(incoming[e.blockedIdx], e.blockerIdx)
	}

	for i, iss := range p.issues {
		if iss.status != "blocked" {
			continue
		}
		if len(incoming[i]) == 0 {
			t.Errorf("blocked issue at plan index %d has no blocker edge", i)
		}
	}
}

// TestGenerate_EdgeSortStability verifies that two plan runs with the same
// seed produce sorted-identical edge key sets (structural edge determinism).
// This test uses buildPlan only and forks no real bd subprocesses.
func TestGenerate_EdgeSortStability(t *testing.T) {
	spec := makeSpec(15, 3, 4, 3, 1.5, 55)

	e1 := planEdgeKeys(buildPlan(spec))
	e2 := planEdgeKeys(buildPlan(spec))

	sort.Strings(e1)
	sort.Strings(e2)

	if len(e1) != len(e2) {
		t.Fatalf("edge count: %d vs %d", len(e1), len(e2))
	}
	for i := range e1 {
		if e1[i] != e2[i] {
			t.Errorf("edge[%d] differs: %q vs %q", i, e1[i], e2[i])
		}
	}
}
