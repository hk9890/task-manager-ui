// Package beads provides the subprocess runner and argv-level read cache for
// the beads (bd) CLI.
//
// After the 8pxi refactor, the BeadsGateway interface and all gateway method
// implementations (read_gateway.go, writes.go, validating_gateway.go, etc.)
// live in internal/repository/beads. This package retains:
//
//   - CommandRunner / RunnerConfig / CommandRequest (runner.go)
//   - RunJSON generic helper (runner.go)
//   - DecodeJSONInto / ExecResult / CommandExecutor (runner.go)
//   - The read cache (cache.go) — deferred to 8pxi.7
//
// Consumers that need both runner and gateway types use the two-import pattern:
//
//	import (
//	    bdrunner "github.com/hk9890/beads-workbench/internal/gateway/beads"
//	    repobeads "github.com/hk9890/beads-workbench/internal/repository/beads"
//	)
//
// # Argv contract
//
// ARGV_CONTRACT.md is the single source of truth for every distinct bd argv
// shape bwb emits at runtime. When adding or modifying a bd call site:
//
//  1. Add (or update) the row in ARGV_CONTRACT.md.
//  2. Add a pinning test in internal/repository/beads/ (see canonical pattern
//     in recording_executor_test.go there) or in internal/mode/.
//  3. For any dynamic flag (e.g. --limit driven by terminal height), pin
//     default + max + min + 1 boundary value — not just the common case.
//
// # When to use RecordingExecutor
//
// Use fakes.RecordingExecutor (internal/testing/fakes) in tests outside this
// package that assert a specific bd argv shape.
//
// Tests within this package (package beads) must use package-internal stubs
// (stubExecutor, concurrencyGuardExecutor, etc. in runner_test.go) to avoid
// an import cycle: fakes imports beads, so beads tests cannot import fakes.
//
// Canonical pattern (from internal/repository/beads/ or internal/mode/):
//
//	func TestMyCallArgvShape(t *testing.T) {
//	    t.Parallel()
//
//	    wantArgv := []string{"myverb", "--flag", "value"}
//
//	    rec := fakes.NewRecordingExecutor()
//	    rec.OnArgs(wantArgv).Return(beads.ExecResult{Stdout: []byte(`...`)}, nil)
//
//	    runner := beads.NewCommandRunner(beads.RunnerConfig{Command: "bd", Executor: rec})
//	    gw := repobeads.NewCLIGateway(runner)
//
//	    _, err := gw.MyMethod(context.Background())
//	    if err != nil {
//	        t.Fatalf("MyMethod returned error: %v", err)
//	    }
//
//	    calls := rec.Calls()
//	    // assert calls[0].Args == wantArgv
//	}
package beads
