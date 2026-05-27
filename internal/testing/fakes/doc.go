// Package fakes provides contract-conforming fake implementations of
// external dependencies used in tests across this module.
//
// After the 8pxi refactor, the fake repository and its contract tests were
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
// RecordingExecutor implements bd.CommandExecutor and records each Run
// invocation (argv, workDir, envLen) so tests can assert exact bd argv shapes.
// Use it in any test — including tests within internal/repository/beads/ — that
// needs to verify the exact bd verb and flags bwb emits.
//
// There is no import cycle between this package and internal/repository/beads:
// fakes imports internal/bd (the runner primitive), not internal/repository/beads
// itself, so both package-internal (package beads) and external (package
// beads_test) test files in internal/repository/beads/ may freely import fakes.
// Choose the style based on access needs: use package beads when you need
// unexported symbols; use package beads_test (the default) for pure black-box
// coverage.
//
// Canonical pattern:
//
//	import (
//	    bdrunner "github.com/hk9890/beads-workbench/internal/bd"
//	    "github.com/hk9890/beads-workbench/internal/testing/fakes"
//	)
//
//	rec := fakes.NewRecordingExecutor()
//	rec.OnArgs([]string{"list", "--json", "--all"}).Return(bdrunner.ExecResult{...}, nil)
//	runner := bdrunner.NewCommandRunner(bdrunner.RunnerConfig{Command: "bd", Executor: rec})
//	// drive code under test ...
//	calls := rec.Calls()
//	// assert calls[0].Args == expected
//
// See also internal/repository/beads/ARGV_CONTRACT.md for the full argv inventory.
package fakes
