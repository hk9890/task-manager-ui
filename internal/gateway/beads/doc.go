// Package beads defines the beads gateway seam and adapters.
//
// # Argv contract testing
//
// The gateway has two correctness surfaces that together define its full
// contract:
//
//  1. INCOMING — what bwb PARSES from bd output.
//     Covered by RunReadContract and RunWriteContract in
//     internal/gateway/beads/contract/ (shared with FakeBeadsGateway).
//
//  2. OUTGOING — what bwb SENDS to bd as subprocess argv.
//     Covered by per-call argv-cardinality tests in this package and in
//     internal/mode/board/model_test.go and internal/mode/search/.
//
// Both surfaces must hold for the gateway to be correct. A passing
// RunReadContract test proves nothing about whether the gateway issues the
// right bd verb in the first place; a passing argv-cardinality test proves
// nothing about whether the response is parsed correctly.
//
// # Argv inventory
//
// internal/gateway/beads/ARGV_CONTRACT.md is the single source of truth for
// every distinct bd argv shape bwb emits at runtime. When adding or modifying
// a bd call site:
//
//  1. Add (or update) the row in ARGV_CONTRACT.md.
//  2. Add a pinning test (see canonical pattern below).
//  3. For any dynamic flag (e.g. --limit driven by terminal height), pin
//     default + max + min + 1 boundary value — not just the common case.
//
// # When to use RecordingExecutor (or testRecordingExecutor)
//
// Use a RecordingExecutor (public: internal/testing/fakes.RecordingExecutor;
// package-internal: testRecordingExecutor in recording_executor_test.go)
// in every test that asserts a specific bd argv shape.
//
// Tests within this package (package beads) must use the package-internal
// testRecordingExecutor to avoid an import cycle: fakes imports beads, so
// beads tests cannot import fakes.
//
// Tests outside this package (e.g. internal/mode/board) use the public
// fakes.RecordingExecutor from internal/testing/fakes.
//
// Canonical pattern (package-internal, from TestStatusCatalogArgvShape):
//
//	func TestMyCallArgvShape(t *testing.T) {
//	    t.Parallel()
//
//	    wantArgv := []string{"myverb", "--flag", "value"}
//
//	    rec := newTestRecordingExecutor()
//	    rec.OnArgs(wantArgv).Return(ExecResult{Stdout: []byte(`...`)}, nil)
//
//	    gateway, rec := newTestGateway(rec)
//
//	    _, err := gateway.MyMethod(context.Background())
//	    if err != nil {
//	        t.Fatalf("MyMethod returned error: %v", err)
//	    }
//
//	    assertExactArgv(t, rec, wantArgv)
//	}
//
// For tests that drive a full model Init (multiple calls in a tea.Batch), use
// the multi-call pattern from TestBoardInitRealGatewaySubprocessArgvCardinality
// in internal/mode/board/model_test.go: register all expected argv shapes with
// rec.OnArgs(...).Return(...), drive the batch, then assert each shape with
// assertArgvPresent.
package beads
