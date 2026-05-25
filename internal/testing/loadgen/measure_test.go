package loadgen

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/hk9890/beads-workbench/internal/repository/memory"
)

// ── percentile correctness ──────────────────────────────────────────────────

// TestPercentile_KnownInputs verifies percentile against known reference
// values for a [1,2,...,10] sample using linear interpolation (NumPy/R type 7).
//
// Convention: h = (p/100)*(n-1); result = v[floor(h)] + frac*(v[floor(h)+1]-v[floor(h)])
// For [1..10]: p50 → h=4.5 → 5+0.5*(6-5)=5.5; p95 → h=8.55 → 9+0.55*(10-9)=9.55
func TestPercentile_KnownInputs(t *testing.T) {
	t.Parallel()

	input := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	cases := []struct {
		p    float64
		want float64
	}{
		{p: 0, want: 1.0},
		{p: 50, want: 5.5},
		{p: 95, want: 9.55},
		{p: 99, want: 9.91},
		{p: 100, want: 10.0},
	}

	const eps = 1e-9
	for _, tc := range cases {
		got := percentile(input, tc.p)
		diff := math.Abs(got - tc.want)
		if diff > eps {
			t.Errorf("percentile(%v, %g): got %.10f, want %.10f (diff=%.2e)", input, tc.p, got, tc.want, diff)
		}
	}
}

// TestPercentile_SingleElement verifies behaviour on a one-element slice.
func TestPercentile_SingleElement(t *testing.T) {
	t.Parallel()

	input := []float64{42.0}
	for _, p := range []float64{0, 50, 100} {
		if got := percentile(input, p); got != 42.0 {
			t.Errorf("percentile([42], %g): got %g, want 42.0", p, got)
		}
	}
}

// TestPercentile_Empty verifies that percentile on an empty slice returns 0.
func TestPercentile_Empty(t *testing.T) {
	t.Parallel()

	if got := percentile(nil, 50); got != 0 {
		t.Errorf("percentile(nil, 50): got %g, want 0", got)
	}
}

// TestPercentile_TwoElements verifies boundary and midpoint interpolation.
func TestPercentile_TwoElements(t *testing.T) {
	t.Parallel()

	// [0, 100]: h = 0.01*(p/100), scaled
	// p0 → h=0 → v[0]=0
	// p50 → h=0.5 → 0+0.5*(100-0)=50
	// p100 → h=1 → v[1]=100
	input := []float64{0, 100}
	cases := []struct {
		p    float64
		want float64
	}{
		{0, 0},
		{50, 50},
		{100, 100},
	}
	const eps = 1e-9
	for _, tc := range cases {
		got := percentile(input, tc.p)
		if math.Abs(got-tc.want) > eps {
			t.Errorf("percentile([0,100], %g): got %g, want %g", tc.p, got, tc.want)
		}
	}
}

// ── computeStats ────────────────────────────────────────────────────────────

// TestComputeStats_Basic verifies that computeStats produces correct p50 on
// the known [1..10] ms sample.
func TestComputeStats_Basic(t *testing.T) {
	t.Parallel()

	// Construct 10 durations of 1ms, 2ms, ..., 10ms.
	durations := make([]time.Duration, 10)
	for i := range durations {
		durations[i] = time.Duration(i+1) * time.Millisecond
	}

	stats := computeStats("test.op", durations)

	if stats.Operation != "test.op" {
		t.Errorf("Operation: got %q, want %q", stats.Operation, "test.op")
	}
	if stats.SampleCount != 10 {
		t.Errorf("SampleCount: got %d, want 10", stats.SampleCount)
	}
	if stats.Approximate {
		t.Error("Approximate should be false for n=10")
	}

	const eps = 0.001
	if math.Abs(stats.P50Ms-5.5) > eps {
		t.Errorf("P50Ms: got %g, want 5.5", stats.P50Ms)
	}
	if math.Abs(stats.P95Ms-9.55) > eps {
		t.Errorf("P95Ms: got %g, want 9.55", stats.P95Ms)
	}
	if stats.MaxMs != 10.0 {
		t.Errorf("MaxMs: got %g, want 10.0", stats.MaxMs)
	}
}

// TestComputeStats_Approximate verifies that n < 5 sets Approximate=true.
func TestComputeStats_Approximate(t *testing.T) {
	t.Parallel()

	durations := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
	}
	stats := computeStats("small.op", durations)

	if !stats.Approximate {
		t.Error("Approximate should be true for n=3")
	}
	if stats.SampleCount != 3 {
		t.Errorf("SampleCount: got %d, want 3", stats.SampleCount)
	}
}

// TestComputeStats_Empty verifies that computeStats on empty input returns
// a zero-valued stats with the correct operation name.
func TestComputeStats_Empty(t *testing.T) {
	t.Parallel()

	stats := computeStats("empty.op", nil)
	if stats.Operation != "empty.op" {
		t.Errorf("Operation: got %q, want %q", stats.Operation, "empty.op")
	}
	if stats.SampleCount != 0 {
		t.Errorf("SampleCount: got %d, want 0", stats.SampleCount)
	}
	if stats.P50Ms != 0 || stats.P95Ms != 0 || stats.P99Ms != 0 || stats.MaxMs != 0 {
		t.Errorf("all percentiles should be 0 for empty input; got p50=%g p95=%g p99=%g max=%g",
			stats.P50Ms, stats.P95Ms, stats.P99Ms, stats.MaxMs)
	}
}

// ── JSON roundtrip ───────────────────────────────────────────────────────────

// TestOperationStats_JSONRoundtrip verifies that OperationStats marshals to
// and unmarshals from JSON with identical values.
func TestOperationStats_JSONRoundtrip(t *testing.T) {
	t.Parallel()

	original := OperationStats{
		Operation:   "dashboard.cold",
		SampleCount: 10,
		Approximate: false,
		P50Ms:       5.5,
		P95Ms:       9.55,
		P99Ms:       9.91,
		MaxMs:       12.34567,
	}

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got OperationStats
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.Operation != original.Operation {
		t.Errorf("Operation: got %q, want %q", got.Operation, original.Operation)
	}
	if got.SampleCount != original.SampleCount {
		t.Errorf("SampleCount: got %d, want %d", got.SampleCount, original.SampleCount)
	}
	if got.Approximate != original.Approximate {
		t.Errorf("Approximate: got %v, want %v", got.Approximate, original.Approximate)
	}
	if got.P50Ms != original.P50Ms {
		t.Errorf("P50Ms: got %g, want %g", got.P50Ms, original.P50Ms)
	}
	if got.P95Ms != original.P95Ms {
		t.Errorf("P95Ms: got %g, want %g", got.P95Ms, original.P95Ms)
	}
	if got.P99Ms != original.P99Ms {
		t.Errorf("P99Ms: got %g, want %g", got.P99Ms, original.P99Ms)
	}
	if got.MaxMs != original.MaxMs {
		t.Errorf("MaxMs: got %g, want %g", got.MaxMs, original.MaxMs)
	}
}

// TestReport_JSONRoundtrip verifies that a full Report round-trips through JSON.
func TestReport_JSONRoundtrip(t *testing.T) {
	t.Parallel()

	original := Report{
		Header: ReportHeader{
			GeneratedAt: time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC),
			BdVersion:   "bd 1.0.4",
			SamplesCold: 5,
			SamplesWarm: 20,
		},
		Operations: []OperationStats{
			{Operation: "dashboard.cold", SampleCount: 5, P50Ms: 100.0, P95Ms: 120.0, P99Ms: 130.0, MaxMs: 135.0},
			{Operation: "dashboard.warm", SampleCount: 20, P50Ms: 0.5, P95Ms: 0.9, P99Ms: 1.0, MaxMs: 1.2},
		},
	}

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got Report
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(got.Operations) != len(original.Operations) {
		t.Fatalf("Operations count: got %d, want %d", len(got.Operations), len(original.Operations))
	}

	if got.Header.BdVersion != original.Header.BdVersion {
		t.Errorf("BdVersion: got %q, want %q", got.Header.BdVersion, original.Header.BdVersion)
	}
	if got.Header.SamplesCold != original.Header.SamplesCold {
		t.Errorf("SamplesCold: got %d, want %d", got.Header.SamplesCold, original.Header.SamplesCold)
	}
	if !got.Header.GeneratedAt.Equal(original.Header.GeneratedAt) {
		t.Errorf("GeneratedAt: got %v, want %v", got.Header.GeneratedAt, original.Header.GeneratedAt)
	}
	for i, op := range got.Operations {
		want := original.Operations[i]
		if op.Operation != want.Operation {
			t.Errorf("Operations[%d].Operation: got %q, want %q", i, op.Operation, want.Operation)
		}
		if op.P50Ms != want.P50Ms {
			t.Errorf("Operations[%d].P50Ms: got %g, want %g", i, op.P50Ms, want.P50Ms)
		}
	}
}

// ── durationsToMs ───────────────────────────────────────────────────────────

// TestDurationsToMs verifies the millisecond conversion.
func TestDurationsToMs(t *testing.T) {
	t.Parallel()

	durations := []time.Duration{
		500 * time.Microsecond, // 0.5ms
		1 * time.Millisecond,
		2500 * time.Microsecond, // 2.5ms
	}
	want := []float64{0.5, 1.0, 2.5}
	got := durationsToMs(durations)

	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	const eps = 1e-9
	for i := range want {
		if math.Abs(got[i]-want[i]) > eps {
			t.Errorf("[%d]: got %g, want %g", i, got[i], want[i])
		}
	}
}

// ── measureWith unit tests (fake repository, no real bd) ────────────────────

// newSeededdMemRepo returns a memory.Repository seeded with n open issues
// so Search returns IDs for detail-measurement loops.
func newSeededMemRepo(n int) *memory.Repository {
	repo := memory.New()
	for i := 1; i <= n; i++ {
		repo.Seed(memory.Issue{
			ID:     fmt.Sprintf("test-%04d", i),
			Title:  fmt.Sprintf("test issue %d", i),
			Status: "open",
		})
	}
	return repo
}

// assertPercentileOrder verifies P50 ≤ P95 ≤ P99 ≤ Max for an operation.
func assertPercentileOrder(t *testing.T, op OperationStats) {
	t.Helper()
	if op.P50Ms > op.P95Ms {
		t.Errorf("operation %q: P50Ms (%g) > P95Ms (%g)", op.Operation, op.P50Ms, op.P95Ms)
	}
	if op.P95Ms > op.P99Ms {
		t.Errorf("operation %q: P95Ms (%g) > P99Ms (%g)", op.Operation, op.P95Ms, op.P99Ms)
	}
	if op.P99Ms > op.MaxMs {
		t.Errorf("operation %q: P99Ms (%g) > MaxMs (%g)", op.Operation, op.P99Ms, op.MaxMs)
	}
}

// TestMeasure_EndToEnd_Unit is the unit-test equivalent of TestMeasure_EndToEnd.
// It uses a memory repository and a static bd-version string so no real bd
// subprocess is forked. This verifies the measureWith pipeline: operation names,
// sample counts (including the SamplesWarm*detailN and SamplesCold*detailN
// formulas for issue.detail.*), and the percentile ordering invariant.
func TestMeasure_EndToEnd_Unit(t *testing.T) {
	t.Parallel()

	const (
		nIssues     = 5
		samplesCold = 2
		samplesWarm = 3
		issueDetailN = 3
	)
	const fakeBdVersion = "bd fake-1.0.0 (unit-test)"

	repo := newSeededMemRepo(nIssues)
	manifest := &Manifest{
		Spec:         Spec{Counts: map[string]int{"open": nIssues}, Seed: 1},
		ActualCounts: map[string]int{"open": nIssues},
		BdVersion:    fakeBdVersion,
	}
	opts := MeasureOpts{
		SamplesCold:  samplesCold,
		SamplesWarm:  samplesWarm,
		IssueDetailN: issueDetailN,
	}

	report, err := measureWith(context.Background(), repo, fakeBdVersion, manifest, opts)
	if err != nil {
		t.Fatalf("measureWith: %v", err)
	}
	if report == nil {
		t.Fatal("measureWith returned nil report")
	}

	// Header checks.
	if report.Header.BdVersion != fakeBdVersion {
		t.Errorf("Header.BdVersion: got %q, want %q", report.Header.BdVersion, fakeBdVersion)
	}
	if report.Header.SamplesCold != samplesCold {
		t.Errorf("Header.SamplesCold: got %d, want %d", report.Header.SamplesCold, samplesCold)
	}
	if report.Header.SamplesWarm != samplesWarm {
		t.Errorf("Header.SamplesWarm: got %d, want %d", report.Header.SamplesWarm, samplesWarm)
	}
	if report.Header.Manifest == nil {
		t.Error("Header.Manifest is nil")
	}

	// Build a name → stats map for assertions.
	opsByName := make(map[string]OperationStats, len(report.Operations))
	for _, op := range report.Operations {
		opsByName[op.Operation] = op
	}

	// Required operations.
	requiredOps := []string{
		"dashboard.cold",
		"dashboard.warm",
		"cache.hydrate",
		"cache.hot_read",
		"cache.force_fresh",
	}
	for _, sc := range searchMatrix {
		requiredOps = append(requiredOps, "search."+sc.name)
	}

	for _, name := range requiredOps {
		op, ok := opsByName[name]
		if !ok {
			t.Errorf("missing operation %q in report", name)
			continue
		}
		if op.SampleCount <= 0 {
			t.Errorf("operation %q: SampleCount=%d, want > 0", name, op.SampleCount)
		}
		assertPercentileOrder(t, op)
	}

	// Cold-path ops: SampleCount == SamplesCold exactly.
	for _, name := range []string{"dashboard.cold", "cache.hydrate", "cache.force_fresh"} {
		op := opsByName[name]
		if op.SampleCount != samplesCold {
			t.Errorf("cold op %q SampleCount: got %d, want %d", name, op.SampleCount, samplesCold)
		}
	}

	// Warm-path ops: SampleCount == SamplesWarm exactly.
	for _, name := range []string{"dashboard.warm", "cache.hot_read"} {
		op := opsByName[name]
		if op.SampleCount != samplesWarm {
			t.Errorf("warm op %q SampleCount: got %d, want %d", name, op.SampleCount, samplesWarm)
		}
	}

	// Collect how many IDs collectIssueIDs actually returned (may be < issueDetailN
	// if the repo has fewer issues).
	detailN := len(report.Header.IssueDetailIDs)

	if detailN > 0 {
		// issue.detail.warm: SampleCount = SamplesWarm * len(detailIDs)
		wantWarmDetail := samplesWarm * detailN
		if op, ok := opsByName["issue.detail.warm"]; ok {
			if op.SampleCount != wantWarmDetail {
				t.Errorf("issue.detail.warm SampleCount: got %d, want %d (SamplesWarm=%d × detailN=%d)",
					op.SampleCount, wantWarmDetail, samplesWarm, detailN)
			}
			assertPercentileOrder(t, op)
		} else {
			t.Error("missing operation issue.detail.warm")
		}

		// issue.detail.cold: SampleCount = SamplesCold * len(detailIDs)
		wantColdDetail := samplesCold * detailN
		if op, ok := opsByName["issue.detail.cold"]; ok {
			if op.SampleCount != wantColdDetail {
				t.Errorf("issue.detail.cold SampleCount: got %d, want %d (SamplesCold=%d × detailN=%d)",
					op.SampleCount, wantColdDetail, samplesCold, detailN)
			}
			assertPercentileOrder(t, op)
		} else {
			t.Error("missing operation issue.detail.cold")
		}
	}

	// Total operations count must be at least len(requiredOps).
	if len(report.Operations) < len(requiredOps) {
		t.Errorf("Operations count: got %d, want >= %d", len(report.Operations), len(requiredOps))
	}
}

// TestMeasure_WriteReport_Unit verifies that WriteReport creates a well-formed
// JSON file whose decoded payload matches the in-memory Report field-by-field.
func TestMeasure_WriteReport_Unit(t *testing.T) {
	t.Parallel()

	report := &Report{
		Header: ReportHeader{
			BdVersion:   "bd test-1.0.0",
			SamplesCold: 5,
			SamplesWarm: 20,
		},
		Operations: []OperationStats{
			{Operation: "dashboard.cold", SampleCount: 5, P50Ms: 100.0, P95Ms: 120.0, P99Ms: 130.0, MaxMs: 135.0},
			{Operation: "dashboard.warm", SampleCount: 20, P50Ms: 0.5, P95Ms: 0.9, P99Ms: 1.0, MaxMs: 1.2},
		},
	}

	dir := t.TempDir()
	outPath := dir + "/test-report.json"

	if err := WriteReport(report, outPath); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("report file is empty")
	}

	// Decode the JSON and verify the payload matches the in-memory report.
	var got Report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Header field-by-field.
	if got.Header.BdVersion != report.Header.BdVersion {
		t.Errorf("Header.BdVersion: got %q, want %q", got.Header.BdVersion, report.Header.BdVersion)
	}
	if got.Header.SamplesCold != report.Header.SamplesCold {
		t.Errorf("Header.SamplesCold: got %d, want %d", got.Header.SamplesCold, report.Header.SamplesCold)
	}
	if got.Header.SamplesWarm != report.Header.SamplesWarm {
		t.Errorf("Header.SamplesWarm: got %d, want %d", got.Header.SamplesWarm, report.Header.SamplesWarm)
	}

	// Operations array length and field-by-field comparison.
	if len(got.Operations) != len(report.Operations) {
		t.Fatalf("Operations count: got %d, want %d", len(got.Operations), len(report.Operations))
	}
	for i, want := range report.Operations {
		g := got.Operations[i]
		if g.Operation != want.Operation {
			t.Errorf("Operations[%d].Operation: got %q, want %q", i, g.Operation, want.Operation)
		}
		if g.SampleCount != want.SampleCount {
			t.Errorf("Operations[%d].SampleCount: got %d, want %d", i, g.SampleCount, want.SampleCount)
		}
		if g.P50Ms != want.P50Ms {
			t.Errorf("Operations[%d].P50Ms: got %g, want %g", i, g.P50Ms, want.P50Ms)
		}
		if g.P95Ms != want.P95Ms {
			t.Errorf("Operations[%d].P95Ms: got %g, want %g", i, g.P95Ms, want.P95Ms)
		}
		if g.P99Ms != want.P99Ms {
			t.Errorf("Operations[%d].P99Ms: got %g, want %g", i, g.P99Ms, want.P99Ms)
		}
		if g.MaxMs != want.MaxMs {
			t.Errorf("Operations[%d].MaxMs: got %g, want %g", i, g.MaxMs, want.MaxMs)
		}
		assertPercentileOrder(t, g)
	}
}
