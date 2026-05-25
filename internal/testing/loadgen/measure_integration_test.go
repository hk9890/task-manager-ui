//go:build integration

package loadgen

import (
	"os/exec"
	"testing"
)

// TestMeasure_EndToEnd is the single real-bd E2E smoke test for the measurement
// harness. It generates a small corpus (5 issues, no comments), runs Measure
// against it, and exercises the bd-version capture path in both Generate and
// Measure. All other former E2E tests have been converted to fake-runner unit
// tests in measure_test.go / generate_test.go.
//
// Wall-time budget: ≤15s.
func TestMeasure_EndToEnd(t *testing.T) {
	// Skip if bd is not available.
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping end-to-end measurement test")
	}

	// Generate a minimal corpus: 2 open + 1 in_progress.
	// No comments, no deps (DepDensity=0) to stay well within the 15s budget.
	// 3 issues total keeps bd-subprocess overhead low while still exercising
	// the full Generate→Measure pipeline including bd-version capture.
	spec := Spec{
		Counts: map[string]int{
			"open":        2,
			"in_progress": 1,
			"blocked":     0,
			"closed":      0,
		},
		DepDensity:  0.0,
		CommentsPer: 0,
		Seed:        42,
	}

	dir := t.TempDir()
	manifest, err := Generate(spec, dir)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	opts := MeasureOpts{
		SamplesCold:  1, // minimal — we only need to verify the pipeline
		SamplesWarm:  1,
		IssueDetailN: 1,
	}

	report, err := Measure(dir, manifest, opts)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}

	if report == nil {
		t.Fatal("Measure returned nil report")
	}

	// Header checks: bd-version capture must work (this is the E2E smoke reason).
	if report.Header.BdVersion == "" {
		t.Error("Header.BdVersion is empty — bd-version capture failed")
	}
	if report.Header.SamplesCold != opts.SamplesCold {
		t.Errorf("SamplesCold: got %d, want %d", report.Header.SamplesCold, opts.SamplesCold)
	}
	if report.Header.SamplesWarm != opts.SamplesWarm {
		t.Errorf("SamplesWarm: got %d, want %d", report.Header.SamplesWarm, opts.SamplesWarm)
	}
	if report.Header.Manifest == nil {
		t.Error("Header.Manifest is nil")
	}

	// Manifest BdVersion must also be captured (Generate path).
	if manifest.BdVersion == "" {
		t.Error("manifest.BdVersion is empty — bd-version capture in Generate failed")
	}

	// Expect at minimum these operation names.
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

	opsByName := make(map[string]OperationStats, len(report.Operations))
	for _, op := range report.Operations {
		opsByName[op.Operation] = op
	}

	for _, name := range requiredOps {
		op, ok := opsByName[name]
		if !ok {
			t.Errorf("missing operation %q in report", name)
			continue
		}
		if op.SampleCount <= 0 {
			t.Errorf("operation %q has SampleCount=%d (expected > 0)", name, op.SampleCount)
		}
		// Percentile ordering invariant: P50 ≤ P95 ≤ P99 ≤ Max.
		if op.P50Ms > op.P95Ms {
			t.Errorf("operation %q: P50Ms (%g) > P95Ms (%g)", name, op.P50Ms, op.P95Ms)
		}
		if op.P95Ms > op.P99Ms {
			t.Errorf("operation %q: P95Ms (%g) > P99Ms (%g)", name, op.P95Ms, op.P99Ms)
		}
		if op.P99Ms > op.MaxMs {
			t.Errorf("operation %q: P99Ms (%g) > MaxMs (%g)", name, op.P99Ms, op.MaxMs)
		}
	}

	// Cold-path operations: SampleCount == SamplesCold exactly.
	for _, name := range []string{"dashboard.cold", "cache.hydrate", "cache.force_fresh"} {
		op := opsByName[name]
		if op.SampleCount != opts.SamplesCold {
			t.Errorf("cold op %q SampleCount: got %d, want %d", name, op.SampleCount, opts.SamplesCold)
		}
	}

	// Warm-path operations: SampleCount == SamplesWarm exactly.
	for _, name := range []string{"dashboard.warm", "cache.hot_read"} {
		op := opsByName[name]
		if op.SampleCount != opts.SamplesWarm {
			t.Errorf("warm op %q SampleCount: got %d, want %d", name, op.SampleCount, opts.SamplesWarm)
		}
	}

	// issue.detail.* operations: SampleCount == Samples{Cold,Warm} * detailN.
	detailN := len(report.Header.IssueDetailIDs)
	if detailN > 0 {
		// issue.detail.warm SampleCount = SamplesWarm * detailN
		wantWarm := opts.SamplesWarm * detailN
		if op, ok := opsByName["issue.detail.warm"]; ok {
			if op.SampleCount != wantWarm {
				t.Errorf("issue.detail.warm SampleCount: got %d, want %d (SamplesWarm=%d × detailN=%d)",
					op.SampleCount, wantWarm, opts.SamplesWarm, detailN)
			}
		}
		// issue.detail.cold SampleCount = SamplesCold * detailN
		wantCold := opts.SamplesCold * detailN
		if op, ok := opsByName["issue.detail.cold"]; ok {
			if op.SampleCount != wantCold {
				t.Errorf("issue.detail.cold SampleCount: got %d, want %d (SamplesCold=%d × detailN=%d)",
					op.SampleCount, wantCold, opts.SamplesCold, detailN)
			}
		}
	}

	// Report must have at least len(requiredOps) operations.
	if len(report.Operations) < len(requiredOps) {
		t.Errorf("Operations count: got %d, want >= %d", len(report.Operations), len(requiredOps))
	}
}
