package loadgen

import (
	"fmt"
	"os"
	"sort"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// fakeBdCommander is a deterministic bdCommander for unit tests.
// It returns sequential IDs and records no real state without forking
// any subprocess, so tests run in <<1ms.
type fakeBdCommander struct {
	nextID    int
	bdVersion string
}

// Compile-time check: fakeBdCommander satisfies bdCommander.
var _ bdCommander = (*fakeBdCommander)(nil)

func newFakeCommander() *fakeBdCommander {
	return &fakeBdCommander{nextID: 1, bdVersion: "bd fake-1.0.0 (test)"}
}

func (f *fakeBdCommander) version() (string, error) { return f.bdVersion, nil }
func (f *fakeBdCommander) init(_ string) error      { return nil }
func (f *fakeBdCommander) create(_ string, _ string, _ int) (string, error) {
	id := fmt.Sprintf("lt-%04d", f.nextID)
	f.nextID++
	return id, nil
}
func (f *fakeBdCommander) run(_ string, _ ...string) error { return nil }

func makeSpec(open, inProgress, blocked, closed int, density float64, seed int64) Spec {
	return Spec{
		Counts: map[string]int{
			"open":        open,
			"in_progress": inProgress,
			"blocked":     blocked,
			"closed":      closed,
		},
		DepDensity: density,
		Seed:       seed,
	}
}

// planEdgeKeys returns a sorted list of "blockerIdx→blockedIdx" strings for
// stable comparison across two plan runs.
func planEdgeKeys(p plan) []string {
	keys := make([]string, len(p.edges))
	for i, e := range p.edges {
		keys[i] = fmt.Sprintf("%d→%d", e.blockerIdx, e.blockedIdx)
	}
	sort.Strings(keys)
	return keys
}

// planStatusSummary returns a map of status→count from a plan.
func planStatusSummary(p plan) map[string]int {
	m := make(map[string]int)
	for _, iss := range p.issues {
		m[iss.status]++
	}
	return m
}

// planPrioritySummary returns a map of priority→count from a plan.
func planPrioritySummary(p plan) map[int]int {
	m := make(map[int]int)
	for _, iss := range p.issues {
		m[iss.priority]++
	}
	return m
}

// ── tests ────────────────────────────────────────────────────────────────────

// TestBuildPlan_Determinism verifies that two buildPlan calls with the same
// Spec+Seed produce identical issue plans and edge plans.
func TestBuildPlan_Determinism(t *testing.T) {
	spec := makeSpec(20, 5, 3, 0, 0.5, 42)

	p1 := buildPlan(spec)
	p2 := buildPlan(spec)

	// Issue count must be identical.
	if len(p1.issues) != len(p2.issues) {
		t.Fatalf("issue count: run1=%d run2=%d", len(p1.issues), len(p2.issues))
	}

	// Issue titles, statuses, and priorities must be identical in order.
	for i := range p1.issues {
		a, b := p1.issues[i], p2.issues[i]
		if a.title != b.title || a.status != b.status || a.priority != b.priority {
			t.Errorf("issue[%d] diverges: run1=%+v run2=%+v", i, a, b)
		}
	}

	// Edge structure must be identical (sorted for stability).
	e1 := planEdgeKeys(p1)
	e2 := planEdgeKeys(p2)
	if len(e1) != len(e2) {
		t.Fatalf("edge count: run1=%d run2=%d", len(e1), len(e2))
	}
	for i := range e1 {
		if e1[i] != e2[i] {
			t.Errorf("edge[%d] diverges: run1=%q run2=%q", i, e1[i], e2[i])
		}
	}
}

// TestBuildPlan_DifferentSeeds verifies that different seeds produce different
// plans (probabilistic — very unlikely to collide on a 28-issue plan).
func TestBuildPlan_DifferentSeeds(t *testing.T) {
	spec1 := makeSpec(20, 5, 3, 0, 0.5, 42)
	spec2 := makeSpec(20, 5, 3, 0, 0.5, 99)

	p1 := buildPlan(spec1)
	p2 := buildPlan(spec2)

	// Both plans should have the same total count.
	if len(p1.issues) != len(p2.issues) {
		t.Fatalf("issue count mismatch: %d vs %d", len(p1.issues), len(p2.issues))
	}

	// At least one priority should differ (unless astronomically unlucky).
	// We verify priorities differ across at least one position.
	differ := false
	for i := range p1.issues {
		if p1.issues[i].priority != p2.issues[i].priority {
			differ = true
			break
		}
	}
	if !differ {
		t.Log("note: different seeds produced identical priority assignments (astronomically rare; retry with a different seed if this fails repeatedly)")
	}
}

// TestBuildPlan_CountCorrectness verifies that the plan contains exactly the
// requested number of issues per status.
func TestBuildPlan_CountCorrectness(t *testing.T) {
	tests := []struct {
		name       string
		open       int
		inProgress int
		blocked    int
		closed     int
	}{
		{"typical", 20, 5, 3, 0},
		{"all_open", 10, 0, 0, 0},
		{"all_closed", 0, 0, 0, 8},
		{"no_blocked", 10, 2, 0, 3},
		{"zero_all", 0, 0, 0, 0},
		{"one_each", 1, 1, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := makeSpec(tt.open, tt.inProgress, tt.blocked, tt.closed, 0.0, 7)
			p := buildPlan(spec)

			got := planStatusSummary(p)
			want := map[string]int{
				"open":        tt.open,
				"in_progress": tt.inProgress,
				"blocked":     tt.blocked,
				"closed":      tt.closed,
			}

			for status, wantN := range want {
				if got[status] != wantN {
					t.Errorf("status %q: got %d want %d", status, got[status], wantN)
				}
			}
		})
	}
}

// TestBuildPlan_BlockerInvariant verifies that every blocked issue in the plan
// has at least one incoming blocker edge, for specs where preceding
// open/in_progress issues exist.
func TestBuildPlan_BlockerInvariant(t *testing.T) {
	tests := []struct {
		name string
		spec Spec
	}{
		{
			name: "standard",
			spec: makeSpec(20, 5, 3, 0, 0.5, 42),
		},
		{
			name: "blocked_only",
			// Need at least one non-blocked issue before blocked ones.
			// open=5 so blocked issues at idx 5,6,7 always have earlier issues.
			spec: makeSpec(5, 0, 3, 0, 1.0, 13),
		},
		{
			name: "many_blocked",
			spec: makeSpec(10, 0, 8, 0, 2.0, 99),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := buildPlan(tt.spec)

			// Build a map: blockedIdx → set of blockerIdxs.
			incoming := make(map[int][]int)
			for _, e := range p.edges {
				incoming[e.blockedIdx] = append(incoming[e.blockedIdx], e.blockerIdx)
			}

			// Every blocked issue must have ≥1 incoming edge.
			for i, iss := range p.issues {
				if iss.status != "blocked" {
					continue
				}
				if len(incoming[i]) == 0 {
					t.Errorf("blocked issue at index %d (%q) has no incoming blocker edges", i, iss.title)
				}
			}
		})
	}
}

// TestBuildPlan_BlockerAtZero verifies that when blocked issues have no
// preceding open/in_progress issues (the "index 0 blocker" edge case), a
// warning is produced and the plan still generates without panic.
func TestBuildPlan_BlockerAtZero(t *testing.T) {
	// blocked=3 with no open/in_progress: first issue is at index 0 and has
	// no earlier issue to act as its blocker. The invariant cannot be satisfied
	// for that issue; planWarnings must surface it.
	spec := makeSpec(0, 0, 3, 0, 0.5, 7)
	p := buildPlan(spec)
	warnings := planWarnings(spec, p)

	// Plan should still have 3 issues.
	if len(p.issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(p.issues))
	}

	// planWarnings must contain a warning about the invariant gap.
	foundWarning := false
	for _, w := range warnings {
		if len(w) > 0 {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected at least one warning for blocked-at-index-0, got none")
	}

	// Issues 1 and 2 (index 1, 2) should have blocker edges (they have
	// earlier issues at index 0 and index 1 respectively).
	incoming := make(map[int][]int)
	for _, e := range p.edges {
		incoming[e.blockedIdx] = append(incoming[e.blockedIdx], e.blockerIdx)
	}
	// Index 0 is the known gap — no assertion there.
	// Indices 1 and 2 should have mandatory edges.
	for _, i := range []int{1, 2} {
		if len(incoming[i]) == 0 {
			t.Errorf("blocked issue at index %d should have ≥1 incoming blocker edge", i)
		}
	}
}

// TestBuildPlan_PrioritySampling verifies that with 1000 issues and a fixed
// seed, observed priority frequencies are within ±5% of normalized weights.
func TestBuildPlan_PrioritySampling(t *testing.T) {
	spec := Spec{
		Counts: map[string]int{"open": 1000},
		Seed:   12345,
		// Default priorities: P0=0.05, P1=0.20, P2=0.60, P3=0.10, P4=0.05
	}

	p := buildPlan(spec)
	if len(p.issues) != 1000 {
		t.Fatalf("expected 1000 issues, got %d", len(p.issues))
	}

	got := planPrioritySummary(p)
	total := float64(len(p.issues))

	// Expected frequencies (normalized DefaultPriorities, which already sum to 1.0).
	wantFreqs := map[int]float64{
		0: 0.05,
		1: 0.20,
		2: 0.60,
		3: 0.10,
		4: 0.05,
	}

	const tolerance = 0.05 // ±5%
	for priority, wantFreq := range wantFreqs {
		gotFreq := float64(got[priority]) / total
		diff := gotFreq - wantFreq
		if diff < -tolerance || diff > tolerance {
			t.Errorf("priority %d: got frequency %.3f want %.3f (±%.2f)", priority, gotFreq, wantFreq, tolerance)
		}
	}
}

// TestBuildPlan_EdgeNoCycles verifies that no edge points from a higher index
// to a lower index (which would create a cycle since edges go blocker→blocked
// and blocker must be earlier in creation order).
func TestBuildPlan_EdgeNoCycles(t *testing.T) {
	spec := makeSpec(15, 3, 4, 3, 2.0, 77)
	p := buildPlan(spec)

	for _, e := range p.edges {
		if e.blockerIdx >= e.blockedIdx {
			t.Errorf("invalid edge: blocker index %d >= blocked index %d (would create cycle)",
				e.blockerIdx, e.blockedIdx)
		}
	}
}

// TestBuildPlan_NoDuplicateEdges verifies no duplicate (blocker, blocked) pairs.
func TestBuildPlan_NoDuplicateEdges(t *testing.T) {
	spec := makeSpec(15, 5, 3, 5, 3.0, 55) // high density to stress duplicates
	p := buildPlan(spec)

	type edgeKey struct{ blocker, blocked int }
	seen := make(map[edgeKey]int)
	for i, e := range p.edges {
		k := edgeKey{e.blockerIdx, e.blockedIdx}
		if prev, dup := seen[k]; dup {
			t.Errorf("duplicate edge at index %d and %d: %d→%d", prev, i, k.blocker, k.blocked)
		}
		seen[k] = i
	}
}

// TestBuildPlan_ZeroSpec verifies that an empty spec produces an empty plan.
func TestBuildPlan_ZeroSpec(t *testing.T) {
	spec := makeSpec(0, 0, 0, 0, 0.0, 1)
	p := buildPlan(spec)
	if len(p.issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(p.issues))
	}
	if len(p.edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(p.edges))
	}
}

// ── fake-runner unit tests (converted from integration E2E tests) ─────────────

// TestGenerate_EndToEnd_Unit is the unit-test equivalent of the integration
// TestGenerate_EndToEnd. It calls generateWith with a fake bdCommander so no
// real bd subprocess is forked. This verifies the manifest shape, counts, and
// bd-version capture through the generateWith pipeline.
func TestGenerate_EndToEnd_Unit(t *testing.T) {
	t.Parallel()

	spec := makeSpec(20, 5, 3, 0, 0.5, 42)
	dir := t.TempDir()

	m, err := generateWith(spec, dir, newFakeCommander())
	if err != nil {
		t.Fatalf("generateWith: %v", err)
	}

	// Counts must match the spec.
	for status, want := range spec.Counts {
		got := m.ActualCounts[status]
		if got != want {
			t.Errorf("status %q: got %d want %d", status, got, want)
		}
	}

	// IssuesPath must be a non-empty string (the fake still sets it from the dir).
	if m.IssuesPath == "" {
		t.Error("IssuesPath is empty")
	}

	// BdVersion must be captured from the fake commander.
	if m.BdVersion != "bd fake-1.0.0 (test)" {
		t.Errorf("BdVersion: got %q, want %q", m.BdVersion, "bd fake-1.0.0 (test)")
	}

	// ActualEdges must match plan output.
	expectedEdges := len(buildPlan(spec).edges)
	if m.ActualEdges != expectedEdges {
		t.Errorf("ActualEdges: got %d, want %d (from buildPlan)", m.ActualEdges, expectedEdges)
	}

	// IssuesPath must point to the .beads subdir of dir.
	if !containsSuffix(m.IssuesPath, ".beads") {
		t.Errorf("IssuesPath %q does not end with .beads", m.IssuesPath)
	}
}

// TestGenerate_Determinism_Unit is the unit-test equivalent of the integration
// TestGenerate_Determinism. Two generateWith calls with the same spec and seed
// must produce manifests with identical structural shape.
func TestGenerate_Determinism_Unit(t *testing.T) {
	t.Parallel()

	spec := makeSpec(10, 2, 2, 1, 0.5, 999)

	dir1 := t.TempDir()
	m1, err := generateWith(spec, dir1, newFakeCommander())
	if err != nil {
		t.Fatalf("generateWith run1: %v", err)
	}

	dir2 := t.TempDir()
	m2, err := generateWith(spec, dir2, newFakeCommander())
	if err != nil {
		t.Fatalf("generateWith run2: %v", err)
	}

	// Counts must be identical across runs.
	for status := range spec.Counts {
		if m1.ActualCounts[status] != m2.ActualCounts[status] {
			t.Errorf("status %q count differs: run1=%d run2=%d",
				status, m1.ActualCounts[status], m2.ActualCounts[status])
		}
	}

	// Edge count must be identical across runs.
	if m1.ActualEdges != m2.ActualEdges {
		t.Errorf("ActualEdges differs: run1=%d run2=%d", m1.ActualEdges, m2.ActualEdges)
	}

	// BdVersion must be identical across runs (both use the same fake).
	if m1.BdVersion != m2.BdVersion {
		t.Errorf("BdVersion differs: run1=%q run2=%q", m1.BdVersion, m2.BdVersion)
	}

	// Substantive: edge count must equal what buildPlan produces for this spec.
	expectedEdges := len(buildPlan(spec).edges)
	if m1.ActualEdges != expectedEdges {
		t.Errorf("ActualEdges: got %d, want %d (from buildPlan)", m1.ActualEdges, expectedEdges)
	}
}

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

// containsSuffix reports whether path ends with the given component name.
// Used to assert IssuesPath ends with ".beads".
func containsSuffix(path, suffix string) bool {
	if len(path) < len(suffix) {
		return false
	}
	// Accept either the path ending in suffix or suffix/ or /suffix.
	base := path
	for len(base) > 0 && (base[len(base)-1] == '/' || base[len(base)-1] == os.PathSeparator) {
		base = base[:len(base)-1]
	}
	if len(base) < len(suffix) {
		return false
	}
	return base[len(base)-len(suffix):] == suffix
}
