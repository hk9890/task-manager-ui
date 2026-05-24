// Package fakes provides contract-conforming fake implementations of
// external dependencies used in tests across this module.
//
// After the 8pxi refactor, the fake gateway and its contract tests were
// removed. The lean beads.Repository (internal/repository/beads) is now used
// directly in all integration-tier tests via the embedded fixture.
//
// # Remaining fakes
//
//   - FakeEditor (editor.go) — fake for the $EDITOR launch path
//   - FakeLauncher (launcher.go) — fake for the configurable launcher
//   - FakeProcessRunner (process_runner.go) — fake for subprocess execution
//   - RecordingExecutor (recording_executor.go) — records bd subprocess argv
//     for argv-cardinality tests
//
// # RecordingExecutor
//
// RecordingExecutor implements gateway/beads.CommandExecutor and records each
// Run invocation (argv, workDir, envLen) so tests can assert exact bd argv
// shapes. Use it in tests outside internal/gateway/beads/ that need to verify
// the exact bd verb and flags bwb emits.
//
// Tests within internal/gateway/beads/ use package-internal stubs to avoid
// the import cycle: this package imports beads, so beads tests cannot import
// fakes.
//
// Canonical pattern:
//
//	rec := fakes.NewRecordingExecutor()
//	rec.OnArgs([]string{"list", "--json", "--all"}).Return(beads.ExecResult{...}, nil)
//	runner := beads.NewCommandRunner(beads.RunnerConfig{Command: "bd", Executor: rec})
//	// drive code under test ...
//	calls := rec.Calls()
//	// assert calls[0].Args == expected
//
// See also internal/gateway/beads/ARGV_CONTRACT.md for the full argv inventory.
package fakes
