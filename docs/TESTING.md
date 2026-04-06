# Testing Strategy

This document defines the repository testing vocabulary, commands, and harness conventions for Beads Workbench.

**Automated tests are the primary proof of correctness.** For user-facing behavior, they must be complemented by a reproducible full-app verification run performed by the agent/operator. Do not rely on user confirmation for basic product validation that can be checked directly.

## Test Vocabulary

- **Unit tests**
  - Fast, deterministic tests that isolate a package or function with test doubles.
  - No dependency on a live beads project or shell state.
  - Examples: `internal/gateway/beads/*_test.go`, `internal/ui/*/*_test.go`, `internal/testing/fakes/*_test.go`.

- **Integration tests**
  - Tests that execute the real `bd` CLI against a controlled beads fixture repo/workspace.
  - Verify command availability, real output shape, and behavior across package seams.
  - Use the embedded-mode fixture harness in `internal/testing/e2e/embeddedfixture`.

- **End-to-end / smoke tests**
  - Broad checks of `bwb` startup and critical user workflows using real process wiring.
  - Lower depth than full scenario testing; intended to catch obvious regressions quickly.
  - Use the same deterministic embedded fixture seed data as integration tests.

- **Full-app verification**
  - A real run of the built `bwb` binary against a deterministic fixture repo in a terminal session.
  - Required for user-visible flows and major UX/layout changes.
  - Confirms the app works as a product, not only as isolated render/test units.
  - Must be performed by the agent/operator; do not hand this responsibility to the user.

## Commands

Use Go tooling from the repository root:

```bash
go test ./...
```

Recommended local quality checks:

```bash
go test ./cmd/bwb -run TestArchitectureGuardrails
go build ./cmd/bwb
go vet ./...
go test ./...
```

Harness-focused checks:

```bash
go test ./internal/testing/...
go test ./internal/testing/ui -v
```

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

- Prefer the embedded fixture for repeatable verification.
- If terminal capture is needed, use a method that records the visible rendered screen. Alt-screen TUIs may not be proven by raw stdout/transcript output alone.
- Full-app verification complements automated tests; it does not replace them.

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

- Store golden files under the tested package’s `testdata/` directory.
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
go test ./internal/app -run 'TestModelEmbeddedFixtureFullBoardCaptureGolden|TestModelBoardDetailBoardRoundTripPreservesLayoutAndFocus' -v
```

### Exceptions

If a surface is not practical for teatest+golden (for example, highly volatile ANSI animation timing), document the exception in the package test file and use the narrowest deterministic alternative (typically message/state assertions).

## Shared Fake Seams for UI/Integration Tests

The shared deterministic seams live in `internal/testing/fakes`:

- **`FakeBeadsGateway`**
  - Implements `beads.BeadsGateway`.
  - Supports deterministic per-method responses.
  - Supports configurable per-method error injection via `SetError(method, err)` for error-path tests.
  - Records calls for interaction assertions.

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

## Gateway Test Strategy (Phase 1)

Gateway coverage is centered on deterministic unit tests with two seams:

1. **Command seam**: `CommandExecutor` test doubles verify exact `bd` command construction and flag mapping.
2. **Payload seam**: JSON fixtures under `internal/gateway/beads/testdata/` verify decoding against representative official `bd` payload shapes.

### Coverage goals

- Representative **read flow** command construction and decoding (`list`, `ready`, `blocked`, `show`, `search`, catalog commands).
- Representative **write flow** command construction (`create`, `update`, `close`, `comments add`).
- Failure behavior and error normalization for:
  - process execution failures,
  - non-zero exit codes,
  - invalid JSON output,
  - missing required fields,
  - stderr propagation in gateway error messages.

## Embedded-Mode Fixture Repository (integration/e2e)

Deterministic fixture harness lives under `internal/testing/e2e/embeddedfixture`:

- `seed.json`: source-of-truth seed data (issues, comments, labels, dependencies).
- `setup.sh`: reproducible script that initializes an embedded-mode beads repo and seeds `seed.json`.
- `Seed(...)` helper: test helper that invokes `setup.sh`.

Typical usage in integration/e2e tests:

1. `repoPath := embeddedfixture.TempRepoPath(t)`
2. `embeddedfixture.Seed(t, repoPath)`
3. Run real `bd`/gateway/app flow against `repoPath`

Fixture expectations:

- Seed data is deterministic and version-controlled.
- Setup is idempotent enough for repeated test execution.
- Tests should treat fixture data as read-only unless a scenario explicitly validates writes.

## Gateway Fixture Conventions (unit)

- Store gateway JSON payload fixtures in `internal/gateway/beads/testdata/`.
- Prefer realistic JSON copied from official command output shape (with sensitive data removed).
- Keep fixtures small and focused so each test states one intent clearly.

## Official `bd` command-surface limitations (known)

The current gateway design intentionally compensates for limitations in official command flags:

1. `bd search` does not expose ready-state semantics directly. Ready filtering is implemented by loading `bd ready --json` and applying additional structured filters in-memory.
2. `bd search` does not expose dependency-blocked semantics directly; `bd blocked` has a narrow filter surface. Blocked-state search is implemented by loading `bd blocked --json` and applying richer filtering in-memory.

These limitations are expected and tested. If `bd` adds first-class flags later, gateway behavior should be simplified to prefer direct command filtering.
