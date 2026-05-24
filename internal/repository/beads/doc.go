// Package beads implements [repository.Repository] directly on
// [bdrunner.CommandRunner] with no intermediate repository type.
//
// # File layout
//
//   - lean.go           — Repository type, New constructor, run chokepoint, small utilities
//   - lean_reads.go     — Dashboard, Issue, Search, HealthCheck, Catalogs
//   - lean_writes.go    — CreateIssue, UpdateIssue, CloseIssue, AddComment
//   - lean_payloads.go  — package-private JSON DTOs and scalar helpers
//   - options.go        — functional option types (WithCommandHook)
//   - errors.go         — sentinel and helper error types for this package
//
// # Runtime stack
//
// The app composes this package as the innermost layer:
//
//	App / cmd → repository.Repository
//	              │
//	              ├──> caching.Repository       (internal/repository/caching)
//	              ├──> repository.NewValidating  (internal/repository/validating.go)
//	              └──> beads.Repository          ← this package
//	                      └──> CommandRunner     (internal/repository/beads)
//
// Construct with [New]; do not create a [Repository] zero value directly.
//
// # Argv contract
//
// ARGV_CONTRACT.md in [internal/repository/beads] is the single source of truth
// for every distinct bd argv shape bwb emits at runtime. When adding or
// modifying a bd call site:
//
//  1. Add (or update) the row in ARGV_CONTRACT.md.
//  2. Add a pinning test in this package using [fakes.RecordingExecutor]
//     (see the canonical pattern in [internal/repository/beads/doc.go]).
//  3. For any dynamic flag (e.g. --limit driven by terminal height), pin
//     default + max + min + 1 boundary value — not just the common case.
//
// # Contract validation
//
// Domain-return-type validation is NOT performed inside this package.
// It is handled by the [repository.NewValidating] decorator one layer up,
// which warn-logs any contract violations (missing required fields, etc.)
// without interrupting the call.
package beads
