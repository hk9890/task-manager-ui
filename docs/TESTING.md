# Testing Strategy

This document defines the repository testing vocabulary, commands, and harness conventions for Beads Workbench.

**Automated tests are the primary proof of correctness.** For user-facing behavior, they must be complemented by a reproducible full-app verification run performed by the agent/operator. Do not rely on user confirmation for basic product validation that can be checked directly.

## Test Tiers

The repository uses a two-tier model.

### Tier 1 — Unit (`mise run test`, fast, no external processes)

- Fast, deterministic. No external processes. No `git` or `jq`.
- Uses a stub `repository.Repository` or the in-process `memory.Repository` for repository-backed assertions.
- Asserts app behavior: model logic, view rendering, key handling.
- Live in `*_test.go` files alongside the package under test (no build tag required).
- Examples: `internal/mode/*/model_test.go`, `internal/ui/*/*_test.go`, `internal/app/model_test.go`.

### Tier 2 — Integration (`mise run test:integration`, `//go:build integration`)

- Tagged with `//go:build integration` and built only under `mise run test:integration` (and the `quality` / `test:coverage` gates).
- These exercise real OS-level seams (for example launcher subprocess execution) that a synchronous fake cannot reach.
- No external `bd` binary is required: the repository backend is the in-process task-manager SDK, and integration tests construct their stores directly.
- Example: `internal/launcher/process_runner_integration_test.go`.

### Backend behavior tests

The active repository backend (`internal/repository/taskmgr`, built on the task-manager Go SDK) carries its own package-level behavior tests in `internal/repository/taskmgr/repository_test.go`. These build a fresh store with `tasks.Init(t.TempDir(), ...)` and assert dashboard sections, search, mutation effects, error codes, time-field semantics, catalogs, and context cancellation — directly against the in-process backend, no subprocess and no build tag. The in-repo `memory.Repository` (`internal/repository/memory`) is the unit-test fixture and has its own behavior tests in `internal/repository/memory/repository_test.go`.

## Where Does My New Test Go?

| What the test asserts | Where it goes | Tool |
|---|---|---|
| App behavior given any repository state (model logic, view rendering, key handling) | Tier 1 — unit | hand-rolled stub `repository.Repository` or `memory.New()` seeded via `Seed` / `SeedComments` / `SeedCatalogs` |
| `taskmgr` backend semantics (reads, mutations, error mapping) | `internal/repository/taskmgr/repository_test.go` | a real `tasks.Store` via `tasks.Init(t.TempDir(), ...)` wrapped by `taskmgr.New` |
| A real OS seam (subprocess execution, filesystem) | Tier 2 — a `//go:build integration` test | real process/filesystem, run under `mise run test:integration` |

Decision rule: if the test does not touch a real OS seam (subprocess, filesystem) and costs <100ms, it is a unit test; otherwise tag it `integration`.

## Commands

```bash
mise run test                # unit tests only (fast, no external deps)
mise run test:integration    # integration tests only (build tag: integration)
mise run test:all            # unit + integration tests
mise run test:verbose        # unit tests with -v
mise run test:coverage       # unit + integration tests with the coverage-threshold gate
mise run quality             # full pre-handoff gate: vet, lint, guardrails, unit + integration tests
mise run quality:fast        # fast pre-commit gate: vet, lint, guardrails, unit tests (skips integration only)
```

Run `mise tasks` to see the full list. CI additionally runs `fmt:check`,
`scripts:check`, and a `test:coverage` threshold gate — see
`docs/CODING.md` Quality Gates.

Harness-focused runs (package-scoped):

```bash
mise run test -- ./internal/testing/...
go test ./internal/repository/taskmgr/... -v
```

## When to Run Which Gate

- **Per-commit / pre-push (local dev):** `mise run quality:fast` is sufficient.
- **End-of-change validation (closing an epic, acceptance review, before declaring "done"):** `mise run quality` is required — it adds integration tests, which exercise real OS seams invisible to the unit suite. `quality:fast` is not a substitute.

## Runtime UI Verification Workflow (operator runbook)

Use `docs/RUNTIME_UI_VERIFICATION.md` for the concrete, command-oriented workflow.

- It covers the fast deterministic automated scenario loop and a built-binary manual run.
- It includes a short checklist for layout, navigation, search behavior, and external-tool flows.
- Keep this document as policy/strategy; keep step-by-step runtime commands in that runbook.

## Full-App Verification (required for user-facing changes)

Use the real app when a change affects layout, navigation, startup behavior, or operator-facing workflows.

Typical workflow — run the built binary against this project's own `.tasks` store via the `bwb` task:

```bash
mise run bwb
```

For a disposable seeded board, run against the JSONL-backed `memory` backend instead:

```bash
go build -o /tmp/bwb ./cmd/bwb
/tmp/bwb --repo memory --repo-file path/to/seed.jsonl
```

During the run, verify the changed behavior directly:

1. The app starts cleanly and renders a usable first screen.
2. The changed workflow works in the real app, not only in tests.
3. Core navigation still works for the touched area (for example board/detail/search transitions when relevant).
4. Layout changes behave correctly at representative terminal sizes.
5. You can state pass/fail yourself without asking the user to validate basics.

Notes:

- The repository backend is in-process (the task-manager SDK); there is no
  external tracker subprocess in the product path, so no prompt-suppression env
  var is needed for a scripted/captured run.
- Prefer the seeded `memory` backend for repeatable, disposable verification.
- If terminal capture is needed, use a method that records the visible rendered screen. Alt-screen TUIs may not be proven by raw stdout/transcript output alone.
- For a repo-local reproducible capture path, use `scripts/capture_bwb_screen.py` with `pyte`; see `docs/RUNTIME_UI_VERIFICATION.md`.
- Full-app verification complements automated tests; it does not replace them.

### Process-level capture policy

Current decision: **no new default process-level capture harness is added**.

Reasoning:

- Existing in-process `memory`-backed fixtures + teatest + golden/state assertions already cover the primary runtime UI risk surface quickly and deterministically.
- Existing built-binary full-app verification already covers the remaining entrypoint/product run check for manual review without adding fragile transcript-only automation.

Process-level capture stays optional and narrow. Add process-level automation only when a concrete bug class cannot be verified in-process. Any such path must define all of the following up front:

1. **Readiness signal** (what visible state means the app is ready for assertion/capture).
2. **Hard timeout** (must fail explicitly rather than hang; startup-to-capture budget under 2s for the seeded fixture path unless documented otherwise).
3. **Cleanup behavior** (guaranteed child-process termination on success, timeout, and failure).

Do not rely on raw stdout transcript capture alone for alt-screen rendering proof.

## Bubble Tea UI Testing Strategy (default)

Default repository strategy for Bubble Tea surfaces:

1. **`teatest` program-driven tests** for real Bubble Tea runtime behavior (message flow, keyboard input, program wiring).
2. **Golden output verification** for `View()` rendering stability.
3. **Model/message-driven state-machine tests** where behavior needs direct state verification in addition to rendered output.

Shared helpers live under `internal/testing/ui`:

- `NewTestModel`: starts `teatest` with deterministic terminal size.
- `NewTestModelWithSize`: starts `teatest` at explicit terminal width/height.
- `AssertMatchesGolden`: compares rendered output to package-local `testdata/*.golden` files.
- `AssertMatchesGoldenNormalized`: compares output with trailing-space normalization for stable layout snapshots.
- `AssertModelViewMatchesGolden`: convenience for comparing `tea.Model.View()` output.
- `WaitForOutputContainsAll`: waits for real runtime output containing required UI snippets before assertions.

Golden file convention:

- Store golden files under the tested package's `testdata/` directory.
- Keep one scenario per golden for readable diffs.

### Dashboard UX verification workflow (required for redesign work)

Dashboard UX changes must prove real layout behavior, not only internal model state.

Required checks:

1. Run parameterized board layout goldens at representative widths (minimum 80/120/180 columns).
2. Include at least one realistic full-board capture using seeded `memory` fixture data when practical.
3. Include a board → detail → board runtime round-trip test that verifies rendered layout/focus behavior after returning.
4. Add at least one density/chrome assertion to prevent regressions that technically pass state checks but degrade visible issue density.
5. Run the built app against a seeded board and verify the dashboard in a real terminal session.

Example focused runs:

```bash
go test ./internal/mode/board -run TestBoardModeDashboardLayoutGoldensAcrossWidths -v
go test ./internal/app -run 'TestModelFixtureShapedBoardCaptureGolden|TestModelStartupBoardLayoutSanityAndNoRuntimeErrors' -v
```

### Controller-async contract tests

A fourth pattern, *controller-async contract*, drives the search controller against a `DelayedFakeRepository` (defined in `internal/mode/search/model_async_test.go`) to exercise overlapping-Cmd cadence that the synchronous-drain harness (`pressAndResolve` → `ApplyControllerKeySequence`) cannot reach. The gap exists because `ApplyControllerKeySequence` drains every Cmd to completion before the next key arrives, so `m.loading` is always `false` when the next message is processed — making async race windows (keys arriving before a prior search Cmd returns its Msg) completely invisible to those tests. Add controller-async contract tests in `TestSearchControllerAsyncContracts` whenever a bug's root cause involves a user event arriving while a prior async Cmd is still in flight (the czkq.4 / znri.6 / czkq.2 shape). The `DelayedFakeRepository` wraps any `repository.Repository`, so the same pattern applies to detail-mode follow-ups.

### Exceptions

If a surface is not practical for teatest+golden (for example, highly volatile ANSI animation timing), document the exception in the package test file and use the narrowest deterministic alternative (typically message/state assertions).

## Shared Fake Seams for UI Tests

The shared deterministic seams live in `internal/testing/fakes`:

- **`FakeEditor`**
  - Deterministic non-interactive editor seam.
  - Returns configured edit result or error.
  - Records calls.

- **`FakeLauncher`**
  - Deterministic `launcher.Service` seam.
  - Returns configured error and records launch calls.

- **`FakeProcessRunner`**
  - Deterministic seam for launcher process execution.
  - Never spawns real interactive tools.
  - Records command/args/dir/env for assertions.

These seams are required for tests that must not launch real editors or subprocesses.

## Repository Fixture Conventions (unit)

- The in-process `memory.Repository` (`internal/repository/memory`) is the in-repo fixture for unit tests. Build one with `memory.New(...)` and populate it via `Seed` / `SeedComments` / `SeedClosed` / `SeedCatalogs`. Use `WithClock` / `WithIDGenerator` options for deterministic timestamps and IDs.
- The `taskmgr` backend is exercised directly in `internal/repository/taskmgr/repository_test.go` against a real `tasks.Store` from `tasks.Init(t.TempDir(), ...)` — no fixture files, the store is built per-test.
- Keep seeded data small and focused so each test states one intent clearly.
