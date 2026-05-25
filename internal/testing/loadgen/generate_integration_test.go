//go:build integration

package loadgen

import (
	"os"
	"sort"
	"testing"
)

// TestGenerate_EndToEnd calls Generate with a small spec and verifies the
// manifest matches expected counts. This test forks real bd subprocesses.
func TestGenerate_EndToEnd(t *testing.T) {
	spec := makeSpec(20, 5, 3, 0, 0.5, 42)

	dir := t.TempDir()
	m, err := Generate(spec, dir)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify counts.
	for status, want := range spec.Counts {
		got := m.ActualCounts[status]
		if got != want {
			t.Errorf("status %q: got %d want %d", status, got, want)
		}
	}

	// Verify IssuesPath exists.
	if _, err := os.Stat(m.IssuesPath); err != nil {
		t.Errorf("IssuesPath %q not accessible: %v", m.IssuesPath, err)
	}

	// Verify bd version is captured.
	if m.BdVersion == "" {
		t.Error("BdVersion is empty")
	}

	// Verify actual edges are non-negative.
	if m.ActualEdges < 0 {
		t.Errorf("ActualEdges is negative: %d", m.ActualEdges)
	}
}

// TestGenerate_Determinism calls Generate twice with the same spec and seed,
// and verifies the resulting manifests have identical shape (counts + edge count).
// Note: bd uses hash-based IDs, so actual ID strings will differ between runs.
// Determinism covers structural shape, not byte-level identity.
func TestGenerate_Determinism(t *testing.T) {
	spec := makeSpec(10, 2, 2, 1, 0.5, 999)

	dir1 := t.TempDir()
	m1, err := Generate(spec, dir1)
	if err != nil {
		t.Fatalf("Generate run1: %v", err)
	}

	dir2 := t.TempDir()
	m2, err := Generate(spec, dir2)
	if err != nil {
		t.Fatalf("Generate run2: %v", err)
	}

	// Counts must be identical.
	for status := range spec.Counts {
		if m1.ActualCounts[status] != m2.ActualCounts[status] {
			t.Errorf("status %q count differs: run1=%d run2=%d",
				status, m1.ActualCounts[status], m2.ActualCounts[status])
		}
	}

	// Edge count must be identical.
	if m1.ActualEdges != m2.ActualEdges {
		t.Errorf("ActualEdges differs: run1=%d run2=%d", m1.ActualEdges, m2.ActualEdges)
	}
}

// TestGenerate_BlockerInvariant verifies that every blocked issue in a
// Generated repo has at least one incoming dep via the plan (checked against
// the plan, since inspecting bd's dep state requires bd dep list per issue).
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
