# Testing Strategy

The testing vocabulary, commands and harness conventions for `taskmgr-ui`. Automated tests are the
primary proof of correctness; a user-facing change also needs the built binary driven, which
[RUNNING.md](RUNNING.md) owns.

## Test Tiers

The repository uses a two-tier model.

### Tier 1 — Unit (`mise run test`, fast, no external processes)

- Fast, deterministic, in-process. A test that shells out to `git` belongs in Tier 2 — that is why
  the hygiene scans are tagged.
- Uses a stub `repository.Repository` or the in-process `memory.Repository` for repository-backed assertions.
- Asserts app behavior: model logic, view rendering, key handling.
- Live in `*_test.go` files alongside the package under test (no build tag required).
- Examples: `internal/mode/*/model_test.go`, `internal/ui/*/*_test.go`, `internal/app/model_test.go`.

### Tier 2 — Integration (`mise run test:integration`, `//go:build integration`)

- Tagged with `//go:build integration` and built only under `mise run test:integration` and the `test:coverage` gate that `ci` runs.
- These exercise real OS-level seams (for example launcher subprocess execution) that a synchronous fake cannot reach.
- No external tracker binary is required: the repository backend is the in-process task-manager SDK, and integration tests construct their stores directly.
- Example: `internal/launcher/process_runner_integration_test.go`.

### Backend behavior tests

- `internal/repository/taskmgr/repository_test.go` asserts the active backend directly: dashboard
  sections, search, mutation effects, write-path error codes, time-field semantics, catalogs and
  context cancellation. `internal/repository/conformance_test.go` asserts parity with
  `memory.Repository`. Both stay untagged.
- `internal/repository/memory/repository_test.go` covers the in-repo fixture backend.

## Where Does My New Test Go?

| What the test asserts | Where it goes | Tool |
|---|---|---|
| App behavior given any repository state (model logic, view rendering, key handling) | Tier 1 — unit | hand-rolled stub `repository.Repository` or `memory.New()` seeded via `Seed` / `SeedComments` / `SeedClosed` / `SeedCatalogs` |
| `taskmgr` backend semantics (reads, mutations, write-path error mapping) | `internal/repository/taskmgr/repository_test.go` | a real `tasks.Store` via `tasks.Init(t.TempDir(), ...)` wrapped by `taskmgr.New` |
| A real OS seam (subprocess execution, filesystem) | Tier 2 — a `//go:build integration` test | real process/filesystem, run under `mise run test:integration` |

- Build a `memory.Repository` with `memory.New(...)`; use `WithClock` / `WithIDGenerator` for
  deterministic timestamps and IDs.
- Keep seeded data small and focused so each test states one intent clearly.

Decision rule: if the test does not touch a real OS seam (subprocess, filesystem) and costs <100ms, it is a unit test; otherwise tag it `integration`. The in-process task-manager SDK store built via `tasks.Init(t.TempDir(), ...)` counts as a unit seam (in-process, <100ms), not an OS seam — so the backend behavior and conformance tests that use it stay untagged.

## Commands

```bash
mise run test                # unit tests only (fast, no external deps)
mise run test:integration    # integration tests only (build tag: integration)
mise run test:all            # unit + integration tests
mise run test:verbose        # unit tests with -v
mise run test:coverage       # unit + integration tests with the coverage-threshold gate
mise run test:coverage:report # per-file coverage, least covered first; no threshold, gates nothing
mise run ci                  # the merge gate, and exactly what the linux CI job runs
mise run quality:fast        # ~15s pre-commit subset; ci adds format, scripts, build, integration and coverage
```

Run `mise tasks` to see the full list.

Package-scoped `go test` runs without the race detector — the gates add `-race`. To reproduce a gate
race locally use `CGO_ENABLED=1 go test -race ./internal/<pkg>`; the repo default is
`CGO_ENABLED=0`, which makes a bare `-race` fail.

Harness-focused runs (package-scoped):

```bash
go test ./internal/testing/... -v
go test ./internal/repository/taskmgr/... -v
```

## When to Run Which Gate

- **Per-commit / pre-push (local dev):** `mise run quality:fast` is sufficient.
- **End-of-change validation (closing an epic, acceptance review, before declaring "done"):** `mise run ci` is required — it adds integration tests, which exercise real OS seams invisible to the unit suite, plus formatting, script and coverage checks. `quality:fast` is not a substitute.

## Render-regression guards

Four packages guard the artifacts a passing state assertion still ships — frame stacking, doubled
column headers, and log output bleeding into a rendered frame:

| Package | Guards against |
|---|---|
| `internal/mode/board/render_regression_test.go` | frame stacking on the board — asserts the border count per rendered frame |
| `internal/mode/search/render_regression_test.go` | frame stacking in search mode |
| `internal/logging/render_regression_test.go` | log output bleeding into a rendered frame |
| `internal/app/render_regression_test.go` | doubled column headers and frame stacking at app-composition level |

A failure in any of them is a release blocker: it means a visible artifact, not a style preference.
So is a failure in the `internal/repository/taskmgr` behavior tests — wrong counts, order or search
results reach the screen as wrong numbers.

A test that prints a `t.Logf` diagnostic and still ends in PASS is advisory. Read the message and
judge whether to follow up; do not hold a release for it.

## Runtime UI verification

A change to user-visible behavior — layout, navigation, search, startup shell, editor and launcher
flows — needs more than a green suite.

**First, the fast deterministic loop.** These are unit tests, no build tag, and they are the default
quick proof while implementing:

```bash
go test ./internal/testing/ui ./internal/mode/search ./internal/app -run 'TestAssertionHelpersCoverStartupErrorsSearchAndActions|TestSearchModeReusableScenarioHelpersCoverTypingFragileAndClear|TestModelReusableBoardSearchDetailScenarioCoversTypingClearScrollAndBack|TestModelStartupBoardLayoutSanityAndNoRuntimeErrors' -v
```

**Then the real app.** A user-facing change is not verified until it has been driven in the built
binary. [RUNNING.md](RUNNING.md) owns launching it, the PTY capture harness,
and what to check; run it and state pass or fail yourself rather than asking the operator to validate
basics.

### Process-level capture policy

Process-level capture stays manual — the in-process fixtures plus the built-binary run cover the
risk. Automate one only for a bug class that cannot be proven in-process, and only with a readiness
signal, a hard timeout, and guaranteed child-process cleanup.

## Bubble Tea UI Testing Strategy (default)

Default repository strategy for Bubble Tea surfaces:

1. **`teatest` program-driven tests** for real Bubble Tea runtime behavior (message flow, keyboard input, program wiring).
2. **Golden output verification** for `View()` rendering stability.
3. **Model/message-driven state-machine tests** where behavior needs direct state verification in addition to rendered output.

Shared helpers live under `internal/testing/ui`:

- `NewTestModel`: starts `teatest` with deterministic terminal size.
- `NewTestModelWithSize`: starts `teatest` at explicit terminal width/height.
- `AssertMatchesGoldenNormalized`: **the default.** Compares with trailing-space normalization, colour codes kept.
- `AssertMatchesGoldenStripANSI`: same, with colour escapes removed. Use only where the golden pins column geometry and colour would be noise.
- `AssertMatchesGolden`: byte-for-byte comparison. Trailing spaces are invisible in a diff and this helper fails on them, so prefer the normalized variant for rendered layout.
- `AssertModelViewMatchesGolden`: `tea.Model.View()` through the byte-for-byte comparator — for
  rendered layout call `AssertMatchesGoldenNormalized(tb, []byte(m.View()), name)` instead.
- `WaitForOutputContainsAll`: waits for real runtime output containing required UI snippets before assertions.

Flow helpers (`internal/testing/ui/scenarios.go`): `ApplyKeySequence` with `BoardToSearchKeys` /
`OpenDetailKeys` / `DetailBackKeys` / `SearchTypeTextKeys` instead of literal key structs.

Screen assertions (`internal/testing/ui/assertions.go`): `AssertContainsAll`,
`AssertStartupBoardLayoutSanity`, `AssertNoObviousRuntimeErrorPanels`, `AssertActionRequest`.

Golden file convention:

- Store golden files under the tested package's `testdata/` directory.
- Keep one scenario per golden for readable diffs.
- Name a golden that pins a specific terminal width with a `_w<width>` suffix (e.g. `search_results_w120.golden`). Width decides which layout branch the snapshot exercises and is the one attribute a reader cannot recover without opening the test.
- When the width comes from a layout constant rather than a literal, use a symbolic suffix — `_w3col`, `_w2col`, `_w3col_less1` — so the filename cannot go stale if the constant moves.

### Regenerating goldens

```bash
TASKMGR_UI_UPDATE_GOLDEN=1 go test ./internal/...   # rewrite every golden from current output
```

Every assertion helper above regenerates with exactly the bytes it then compares, so a regeneration run against unchanged code produces **no** diff — if `git status` shows a golden after one, the rendering really changed. Review that diff; do not commit it blind.

Goldens are stored with no trailing newline (that is what the normalizing helpers emit). Do not add one.

Use one idiom per package and prefer the shared helpers: three packages once carried their own regeneration wrappers writing three different byte streams, so which directory you were in decided what got committed.

### Dashboard UX verification workflow (required for redesign work)

Dashboard UX changes must prove real layout behavior, not only internal model state.

Required checks:

1. Run parameterized board layout goldens at representative widths (minimum 80/120/180 columns).
2. Include at least one realistic full-board capture using seeded `memory` fixture data when practical.
3. Include a board → detail → board runtime round-trip test that verifies rendered layout/focus behavior after returning.
4. Add at least one density/chrome assertion to prevent regressions that technically pass state checks but degrade visible issue density.
5. Drive the built app against a seeded board in a real terminal session ([RUNNING.md](RUNNING.md)).

Example focused runs:

```bash
go test ./internal/mode/board -run TestBoardModeDashboardLayoutGoldensAcrossWidths -v
go test ./internal/app -run 'TestModelFixtureShapedBoardCaptureGolden|TestModelStartupBoardLayoutSanityAndNoRuntimeErrors' -v
```

### Controller-async contract tests

Add one to `TestSearchControllerAsyncContracts` whenever a bug's root cause is a key
arriving while a prior async Cmd is still in flight. Drive the controller against
`fakes.DelayingRepository` (`internal/testing/fakes/delaying.go`); it wraps any
`repository.Repository`, so the pattern carries to detail-mode follow-ups.

The ordinary harness cannot see these races: `ApplyControllerKeySequence` drains
every Cmd before the next key arrives, so `m.loading` is always `false` by the time
the next message is processed.

### Exceptions

If a surface is not practical for teatest+golden (for example, highly volatile ANSI animation timing), document the exception in the package test file and use the narrowest deterministic alternative (typically message/state assertions).

## Shared fake seams

An editor, a launcher and a subprocess are the only external things you may fake:
`FakeEditor`, `FakeLauncher`, `FakeProcessRunner` (`internal/testing/fakes`). Each returns what you
configure and records its calls. A test that touches an editor, a launcher, or a subprocess is
required to use them.

- Failure-path tests wrap any `repository.Repository` in `fakes.NewErrorInjecting`
  (`internal/testing/fakes/error_injecting.go`); do not hand-roll an error-returning stub.
