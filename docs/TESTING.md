# Testing Strategy

This document defines the repository testing vocabulary, commands, and harness conventions for Beads Workbench.

**Automated tests are the primary proof of correctness.** For user-facing behavior, they must be complemented by a reproducible full-app verification run performed by the agent/operator. Do not rely on user confirmation for basic product validation that can be checked directly.

## Test Tiers

The repository uses a three-tier model.

### Tier 1 — Unit (`mise run test`, fast, no external processes)

- Fast, deterministic. No external processes. No `bd`, `git`, or `jq`.
- Uses a stub `repository.Repository` or a `fakes.RecordingExecutor`-backed real `beads.Repository` for argv-level assertions.
- Asserts app behavior: model logic, view rendering, key handling.
- Live in `*_test.go` files alongside the package under test (no build tag required).
- Examples: `internal/mode/*/model_test.go`, `internal/ui/*/*_test.go`, `internal/app/model_test.go`.

### Tier 2a — Repository-integration READ contract (`mise run test:integration`, `//go:build integration`)

- Package: `internal/repository/beads/contract/`.
- Single parameterized function `RunReadContract(t, factory RepositoryFactory)` wired against a real CLI-backed `beads.Repository`.
- Uses `embeddedfixture.SharedFixtureRepoPath(t)` for the real factory: seeds once per process, copies the pre-seeded cache directory per test (~100ms after the first call).
- **Read-only.** Never mutate the shared fixture inside `RunReadContract`.
- Covers every read method on `repository.Repository` — `internal/repository/repository.go` is the source of truth for the method set.

### Tier 2b — Repository-integration MUTATING scenarios (`mise run test:integration`, `//go:build integration`)

- Same `contract` package, separate `*_scenario_integration_test.go` files.
- Two scenarios today:
  - `TestRealRepositoryIssueLifecycleScenario` — create → update → comment → close (all 4 write methods).
  - `TestRealRepositoryLinksAndDepsScenario` — `bd link`, `bd dep relate`, `bd dep add` with repository read verification.
- Use `embeddedfixture.TempRepoPath(t)` + `embeddedfixture.Seed(t, repoPath)` for a fresh per-test fixture (mutations OK).
- Sized to be debuggable: ~5–10 steps per scenario. Cap at ~3 scenarios total.

## Where Does My New Test Go?

| What the test asserts | Where it goes | Tool |
|---|---|---|
| App behavior given any repository state (model logic, view rendering, key handling) | Tier 1 — unit | hand-rolled stub `repository.Repository` or `fakes.RecordingExecutor`-backed `beads.Repository` |
| The bd CLI adapter's contract (a read method produces correct output) | Tier 2a — add a `t.Run` block to `RunReadContract` | real `beads.Repository` via `SharedFixtureRepoPath` |
| A multi-step bd write workflow (mutations + read verification) | Tier 2b — add to an existing scenario or create a new one | real `beads.Repository` via `TempRepoPath` + `Seed` |

Decision rule: if the test does not fork a real subprocess and costs <100ms, it is a unit test. If it forks `bd`, it is integration.

## Fixture Rules

**Read operations** — use the shared pre-seeded snapshot:

```go
repoPath := embeddedfixture.SharedFixtureRepoPath(t)
```

Seeds the fixture once per process (slow, ~10s on first call); subsequent calls copy the pre-seeded directory (~100ms). Never write to this repo.

**Mutating operations** — use a fresh per-test fixture:

```go
repoPath := embeddedfixture.TempRepoPath(t)
embeddedfixture.Seed(t, repoPath)
```

Creates and seeds a clean directory. Cleanup is automatic via `t.TempDir()`.

## The Fake/Real Contract

`RunReadContract` is the single function that bridges Tier 1 and Tier 2a.

- `TestRealRepositoryReadContract` wires it against the real `bd`-backed `beads.Repository` (integration, `//go:build integration`).

**Why it exists:** The contract pins the exact behavior of every read method against real `bd` output, so regressions in parsing are caught automatically.

## Commands

```bash
mise run test                # unit tests only (fast, no bd required)
mise run test:integration    # integration tests only (real bd + embedded fixture)
mise run test:all            # unit + integration tests
mise run test:verbose        # unit tests with -v
mise run quality             # full pre-handoff gate: vet, lint, guardrails, unit + integration tests
mise run quality:fast        # fast pre-commit gate: vet, lint, guardrails, unit tests (skips integration only)
mise run test:load           # load-test suite: generate workload, measure, emit ./load-test-report.json
```

### Load testing

`mise run test:load` generates a seeded workload, measures data-layer operation latencies (Dashboard cold/warm, cache, search, detail), and writes a JSON report plus a human summary table. Requires `bd` on PATH. Workload shape is configurable via `LOAD_*` env vars (see `scripts/load-test.sh` header). Default workload is 30 issues and takes ~90s due to sequential `bd create` subprocess calls; use larger profiles via env vars for stress testing. Full agent-runnable recipe in `docs/LOAD_TESTING.md`.

Run `mise tasks` to see the full list. CI additionally runs `fmt:check`,
`scripts:check`, and a `test:coverage` threshold gate — see
`docs/CODING.md` Quality Gates.

Harness-focused runs (package-scoped):

```bash
mise run test -- ./internal/testing/...
mise run test -- ./internal/repository/beads/contract/... -v
```

## When to Run Which Gate

- **Per-commit / pre-push (local dev):** `mise run quality:fast` is sufficient.
- **End-of-change validation (closing an epic, acceptance review, before declaring "done"):** `mise run quality` is required — it adds integration tests, which routinely catch parity regressions invisible to the unit suite. `quality:fast` is not a substitute.

## Runtime UI Verification Workflow (operator runbook)

Use `docs/RUNTIME_UI_VERIFICATION.md` for the concrete, command-oriented workflow.

- It covers the fast deterministic automated scenario loop and a built-binary manual run.
- It includes a short checklist for layout, navigation, search behavior, and external-tool flows.
- Keep this document as policy/strategy; keep step-by-step runtime commands in that runbook.

## Full-App Verification (required for user-facing changes)

Use the real app with the embedded fixture when a change affects layout, navigation, startup behavior, or operator-facing workflows.

Typical workflow:

```bash
go build -o /tmp/bwb ./cmd/bwb
repoPath="$(mktemp -d)"
sh internal/testing/e2e/embeddedfixture/setup.sh "$repoPath" internal/testing/e2e/embeddedfixture/seed.json
(cd "$repoPath" && BD_NON_INTERACTIVE=1 /tmp/bwb)
```

During the run, verify the changed behavior directly:

1. The app starts cleanly and renders a usable first screen.
2. The changed workflow works in the real app, not only in tests.
3. Core navigation still works for the touched area (for example board/detail/search transitions when relevant).
4. Layout changes behave correctly at representative terminal sizes.
5. You can state pass/fail yourself without asking the user to validate basics.

Notes:

- `BD_NON_INTERACTIVE=1` suppresses interactive `bd` prompts so the app and
  harness run deterministically; keep it set for any scripted/captured run.
- Prefer the embedded fixture for repeatable verification.
- If terminal capture is needed, use a method that records the visible rendered screen. Alt-screen TUIs may not be proven by raw stdout/transcript output alone.
- For a repo-local reproducible capture path, use `scripts/capture_bwb_screen.py` with `pyte`; see `docs/RUNTIME_UI_VERIFICATION.md`.
- Full-app verification complements automated tests; it does not replace them.

### Process-level capture policy

Current decision: **no new default process-level capture harness is added**.

Reasoning:

- Existing in-process embedded-fixture + teatest + golden/state assertions already cover the primary runtime UI risk surface quickly and deterministically.
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
2. Include at least one realistic full-board capture using embedded fixture data when practical.
3. Include a board → detail → board runtime round-trip test that verifies rendered layout/focus behavior after returning.
4. Add at least one density/chrome assertion to prevent regressions that technically pass state checks but degrade visible issue density.
5. Run the built app against the embedded fixture and verify the dashboard in a real terminal session.

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

- Store JSON payload fixtures in `internal/repository/beads/testdata/`.
- Prefer realistic JSON copied from official command output shape (with sensitive data removed).
- Keep fixtures small and focused so each test states one intent clearly.

## Embedded-Mode Fixture Repository

Deterministic fixture harness lives under `internal/testing/e2e/embeddedfixture`:

- `seed.json`: source-of-truth seed data (issues, comments, labels, dependencies; prefix `bwf`, 3 issues).
- `setup.sh`: reproducible script that initializes an embedded-mode beads repo and seeds `seed.json`.
- `Seed(tb, repoPath)`: test helper that invokes `setup.sh` against a caller-supplied directory.
- `TempRepoPath(tb)`: returns a `tb.TempDir()`-backed path suitable for a fresh mutable fixture.
- `SharedFixtureRepoPath(tb)`: seeds once per process, returns a per-test copy (read-only use).
- `ReadSeedSpec(tb)`: loads `seed.json` as a typed `Spec` struct — used by the scale fixture smoke tests to locate edge-case issues.

## Official `bd` command-surface limitations (known)

The `beads.Repository` implementation intentionally compensates for limitations in official command flags:

1. `bd search` does not expose ready-state semantics directly. Ready filtering is implemented by loading `bd ready --json` and applying additional structured filters in-memory.
2. `bd search` does not expose dependency-blocked semantics directly; `bd blocked` has a narrow filter surface. Blocked-state search is implemented by loading `bd blocked --json` and applying richer filtering in-memory.

These limitations are expected and tested. If `bd` adds first-class flags later, the `beads.Repository` implementation should be simplified to prefer direct command filtering.
