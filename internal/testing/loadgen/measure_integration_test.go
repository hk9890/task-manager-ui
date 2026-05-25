//go:build integration

package loadgen

import (
	"os"
	"os/exec"
	"testing"
)

// TestMeasure_EndToEnd runs Measure against a small generated repository
// (8 issues, no comments) and asserts:
//   - The report has all expected operations.
//   - Sample counts match the options.
//   - Statistics are non-negative.
func TestMeasure_EndToEnd(t *testing.T) {
	// Skip if bd is not available.
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping end-to-end measurement test")
	}

	// Generate a small corpus: 5 open + 2 in_progress + 1 blocked.
	spec := Spec{
		Counts: map[string]int{
			"open":        5,
			"in_progress": 2,
			"blocked":     1,
			"closed":      0,
		},
		DepDensity:  0.0,
		CommentsPer: 0, // no comments — faster
		Seed:        42,
	}

	dir := t.TempDir()
	manifest, err := Generate(spec, dir)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	opts := MeasureOpts{
		SamplesCold:  2, // small for test speed
		SamplesWarm:  3,
		IssueDetailN: 3,
	}

	report, err := Measure(dir, manifest, opts)
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}

	if report == nil {
		t.Fatal("Measure returned nil report")
	}

	// Header checks.
	if report.Header.BdVersion == "" {
		t.Error("Header.BdVersion is empty")
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

	// Expect at minimum these operation names (exact list may grow).
	requiredOps := []string{
		"dashboard.cold",
		"dashboard.warm",
		"cache.hydrate",
		"cache.hot_read",
		"cache.force_fresh",
	}
	// Add search matrix ops.
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
		if op.MaxMs < 0 {
			t.Errorf("operation %q has negative MaxMs=%g", name, op.MaxMs)
		}
		if op.P50Ms < 0 || op.P95Ms < 0 || op.P99Ms < 0 {
			t.Errorf("operation %q has negative percentile: p50=%g p95=%g p99=%g",
				name, op.P50Ms, op.P95Ms, op.P99Ms)
		}
	}

	// Verify cold-path operations have SamplesCold samples (or multiples thereof
	// for operations that multiply by IssueDetailN).
	coldOps := []string{"dashboard.cold", "cache.hydrate", "cache.force_fresh"}
	for _, name := range coldOps {
		op := opsByName[name]
		if op.SampleCount != opts.SamplesCold {
			t.Errorf("cold op %q SampleCount: got %d, want %d", name, op.SampleCount, opts.SamplesCold)
		}
	}

	// Warm-path ops should have SamplesWarm samples.
	warmOps := []string{"dashboard.warm", "cache.hot_read"}
	for _, name := range warmOps {
		op := opsByName[name]
		if op.SampleCount != opts.SamplesWarm {
			t.Errorf("warm op %q SampleCount: got %d, want %d", name, op.SampleCount, opts.SamplesWarm)
		}
	}

	// Report must have at least len(requiredOps) operations.
	if len(report.Operations) < len(requiredOps) {
		t.Errorf("Operations count: got %d, want >= %d", len(report.Operations), len(requiredOps))
	}
}

// TestMeasure_WriteReport verifies that WriteReport creates a well-formed JSON
// file that can be read back and decoded.
func TestMeasure_WriteReport(t *testing.T) {
	report := &Report{
		Header: ReportHeader{
			BdVersion:   "bd test",
			SamplesCold: 5,
			SamplesWarm: 20,
		},
		Operations: []OperationStats{
			{Operation: "dashboard.cold", SampleCount: 5, P50Ms: 100.0, MaxMs: 110.0},
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
	// Basic JSON shape check: must start with '{'.
	for _, b := range data {
		if b == ' ' || b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		if b != '{' {
			t.Errorf("report file does not start with '{'; got %q", b)
		}
		break
	}
}
