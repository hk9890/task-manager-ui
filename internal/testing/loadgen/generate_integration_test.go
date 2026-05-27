//go:build integration

package loadgen

// Note: TestGenerate_EndToEnd and TestGenerate_Determinism have been converted
// to fake-runner unit tests in generate_test.go (TestGenerate_EndToEnd_Unit,
// TestGenerate_Determinism_Unit). The single real-bd E2E smoke test covering
// bd-version capture lives in measure_integration_test.go (TestMeasure_EndToEnd).
//
// TestGenerate_BlockerInvariant and TestGenerate_EdgeSortStability have been
// moved to generate_test.go (unit tier) since they use buildPlan only and fork
// no real bd subprocesses.
