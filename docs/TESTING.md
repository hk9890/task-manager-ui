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

### Tier 2 — Repository parity contract (`mise run test:integration`, `//go:build integration`)

- Package: `internal/repository/contract/`.
- File: `internal/repository/contract/parity_integration_test.go`.
- Entry point: `TestRepositoryContract` runs all 13 scenarios against each factory in parallel sub-tests.
- Requires a real `bd` binary on PATH; the test self-skips when `bd` is not found.

**Factories wired in** (two today; the legacy repository-backed adapter was removed):

| Factory name | Backing implementation |
|---|---|
| `memory` | `memory.Repository` seeded via `memory.Seed` / `memory.SeedComments` / `memory.SeedCatalogs` |
| `beads` | `repobeads.New(runner)` backed by a real `bd` subprocess; each test case gets a fresh temp dir with `bd init` |

**Scenarios** (13 total, each is a `t.Run` sub-test inside `runAllScenarios`):

| # | Sub-test name | What it covers |
|---|---|---|
| 1 | `EmptyStore` | Dashboard / Issue / Search all behave correctly on an empty store |
| 2 | `SingleOpenIssue` | Open issue appears in `Dashboard.ReadyExplain.Ready`; `Issue(id)` returns fields |
| 3 | `DepChainClosedToOpen` | Issue with a closed blocker appears in Ready |
| 4 | `DepChainOpenToOpen` | Issue with an open blocker appears in `ReadyExplain.Blocked` |
| 5 | `StoredStatusBlocked` | Issue with `status=blocked` appears in `Dashboard.Blocked` |
| 6 | `SortDirection` | `Dashboard.Closed` is sorted DESC by `ClosedAt` (most-recently-closed first) |
| 7 | `SearchHitShape` | Title match returns issue; impl divergence for description/notes matching is carve-outed to `impl.name == "memory"` |
| 8 | `MutationEffects` | `CreateIssue` / `UpdateIssue` / `CloseIssue` / `AddComment` mutations are visible in subsequent reads |
| 9 | `UnknownIDErrorCodes` | `UpdateIssue`, `CloseIssue`, `AddComment` on a missing ID return `domain.RepositoryError{Code: ErrorCodeCommandFailed}` |
| 10 | `PartialDashboardFailure` | When one fan-out branch fails, `Dashboard` returns an error (skipped for `memory` impl) |
| 11 | `TimeFieldSemantics` | `CreatedAt` is non-zero; `UpdatedAt` does not regress after mutation; `ClosedAt` is set after `CloseIssue` |
| 12 | `HealthCheckEmptyStore` | `HealthCheck` returns nil for a healthy empty store |
| 13 | `CatalogsShape` | `Catalogs` returns non-nil, non-empty `Statuses` and `Types` including core values |

**Adding a new scenario:**

1. Open `internal/repository/contract/parity_integration_test.go`.
2. Add a new `t.Run("YourScenarioName", func(t *testing.T) { ... })` block inside `runAllScenarios`.
3. Build the repository under test with `impl.build(t, seed)` where `seed` is a `scenarioSeed` (fields: `issues []seedIssue`, `deps []seedDep`).
4. Use `impl.name == "memory"` guards for assertions that cannot hold for both implementations (documented divergences). For scenarios that are entirely inapplicable to one impl, use `t.Skip(...)` — see `PartialDashboardFailure` (Scenario 10) as a template.

## Where Does My New Test Go?

| What the test asserts | Where it goes | Tool |
|---|---|---|
| App behavior given any repository state (model logic, view rendering, key handling) | Tier 1 — unit | hand-rolled stub `repository.Repository` or `fakes.RecordingExecutor`-backed `beads.Repository` |
| Repository parity (read or mutating): both impls must agree | Tier 2 — add a `t.Run` block inside `runAllScenarios` in `internal/repository/contract/parity_integration_test.go` | `impl.build(t, seed)` — runs against both `memory` and `beads` factories |

Decision rule: if the test does not fork a real subprocess and costs <100ms, it is a unit test. If it forks `bd`, it is integration.

## The Fake/Real Contract

`TestRepositoryContract` in `internal/repository/contract/parity_integration_test.go` is the structural answer to the fake/real divergence discipline described in `internal/testing/fakes/doc.go`.

- It runs the same 13 scenarios against both the `memory.Repository` (used as the in-process fake for unit tests) and the `beads.Repository` backed by a real `bd` subprocess.
- Any divergence between the two that is not explicitly carved out in the test source is a parity bug.

**Why it exists:** The suite pins the exact behavior of every `repository.Repository` method against real `bd` output, so regressions in parsing are caught automatically and the unit-test fake stays honest.

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
go test -tags integration ./internal/repository/contract/... -v -run TestRepositoryContract
```

## When to Run Which Gate

- **Per-commit / pre-push (local dev):** `mise run quality:fast` is sufficient.
- **End-of-change validation (closing an epic, acceptance review, before declaring "done"):** `mise run quality` is required — it adds integration tests, which routinely catch parity regressions invisible to the unit suite. `quality:fast` is not a substitute.

## Slow Integration Packages — Risk Justifications

The following packages carry most of the integration wall-time cost (per the test-suite review). Each entry names one concrete regression that a fast in-process or unit test would miss — the reason the subprocess cost is justified.

| Package | ~Time (review) | Regression a fast/in-process test would miss |
|---|---|---|
| `cmd/bwb-smoke` | ~62.8s | See dedicated answer below. |
| `internal/dashboard/parity` | ~51.8s | `bd ready` silently caps at 100 issues without `--limit 0` — a fake repository returns seeded counts and never exercises real bd pagination, so the cap-induced count divergence (and the `TotalIsExact` Done-badge bug class) goes undetected until this parity test runs against a real bd subprocess. |
| `internal/mode/search/parity` | ~27.3s | The search repository routes different query shapes to different bd verbs (`bd list`, `bd search`, `bd ready`, `bd blocked`) with distinct argv; a fake returns whatever it was seeded with regardless of verb, so misrouting or a wrong flag is invisible. Additionally, `TestSearchParityExternalMtimeUnchanged` catches a read-only write-leak to a real `.beads/` directory — a bug that only manifests against a real filesystem. |
| `internal/mode/search` (integration) | ~18.4s | `TestSearchModeEmbeddedFixtureInitUsesEmptyQueryFallback` drives the real search Model via `teatest` against a live bd-backed repository; a synchronous fake never reaches the real subprocess spawn, async Cmd dispatch, or real JSON output parsing path — a broken empty-query argv wiring would only fail here. |
| `internal/testing/datasets` | ~20s | The parity harness self-tests that `NewRepository` enforces `--readonly` via real bd rejection — a fake would accept any call. A regression here means `ThisRepo`/`External` datasets could silently mutate live tracker data during parity runs; only a real bd subprocess returns the enforcement error. |
| `internal/testing/loadgen` | ~20s | The single integration test (`TestMeasure_EndToEnd`) exercises real bd-version capture through the full `Generate`→`Measure` pipeline; the remaining tests have been migrated to fake-runner unit tests. A regression where `manifest.BdVersion` or `report.Header.BdVersion` returns empty — because `bd version --json` changed shape or discovery broke — is invisible to the unit tier. |
| `internal/testing/e2e/embeddedfixture` | ~20s | `TestSharedFixtureRepoPathRoundTrip` verifies that `setup.sh` produces a real-bd-usable repository (`.beads/embeddeddolt` present, `bd ready` succeeds) and enforces a ≥10x speedup on the second call — if the per-process cache regresses to re-seeding on every call, the whole integration suite balloons; this is unfakeable without a real bd subprocess and real filesystem. |

### `cmd/bwb-smoke` 62s challenge: what does a real-binary run catch?

`bwb-smoke` is built by `TestSmokeIntegration` using `go build ./cmd/bwb-smoke` — it is a separate binary compilation step. The unit tests in `main_test.go` already cover flag parsing (`parseChecks`), JSON emission (`emitJSON`), and the in-process render check (`runRenderCheck`) with no subprocess and no binary build.

The 62s integration run's unique catches are:

1. **Build and link of the standalone binary.** A broken import, removed export, or `cmd/bwb-smoke`-local compilation error surfaces here and nowhere else in the test suite.
2. **Exit-code contract.** The binary must exit 0 on PASS and 1 on FAIL. In-process unit tests call functions directly; they never validate `os.Exit` behavior, which is only observable by running the compiled process.
3. **Real bd binary discovery and invocation.** `bwb-smoke` constructs a `beads.CommandRunner` at runtime and calls `bd count`, `bd ready`, `bd blocked`, `bd list`, `bd search`, and `bd query` against a seeded database. A wrong argv assembly or a missing `--json` flag fails silently in-process (the unit `runCountCheck`/`runSortCheck`/`runSearchCheck` helpers are called in-process with a pre-built `repo`); only the built binary run exercises the full subprocess invocation path from the main entry point.
4. **Stdout JSON pipeability.** `TestSmokeIntegrationJSONPipeable` runs the binary and confirms the output is parseable JSON regardless of exit code — testing the `--format json` output contract end-to-end with a real process stdout.

**There are no trim candidates.** Every package above names a real-bd-only regression. The scale-fixture variants (`BWB_SCALE_FIXTURE=1`, `BWB_SCALE_FIXTURE_SMOKE=1`) are already env-gated and skipped by default; their cost does not appear in the measured baseline times above.

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
