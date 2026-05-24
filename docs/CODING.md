# Coding

## Project Identity

- **Module:** `github.com/hk9890/beads-workbench`
- **Binary:** `bwb` (`cmd/bwb`)
- **Language:** Go
- **TUI framework:** Bubble Tea

## Build and Test

Use standard Go tooling from the repository root for build, vet, and test work.
See `docs/CHANGE-WORKFLOW.md` for the authoritative pre-handoff landing
workflow. See [Quality Gates](#quality-gates) for the code-change verification
commands used by that workflow.

For testing strategy, vocabulary, and harness conventions (teatest, golden files,
fake seams, embedded fixture usage), see `docs/TESTING.md`.

## CLI startup semantics (v1)

`cmd/bwb/main.go` intentionally keeps a minimal pre-TUI CLI surface before
starting Bubble Tea.

Supported flags:

- `-h`, `--help`
- `-v`, `--version`
- `-c`, `--config <path>`
- `--cwd <path>`
- `-d`, `--debug`
- `--no-auto-refresh`
- `--print-config`
- `--check-config`
- `--repo <backend>` — repository backend: `beads | memory | caching` (default: `caching`)
  - `caching` (default): wraps the beads backend with an in-memory read cache,
    per-session JSONL persistence, and background VCS-hash polling for
    invalidation. Startup prints `"Using caching repository backend; --repo beads disables"`.
  - `beads`: live `bd` subprocess calls on every read; no caching or persistence.
  - `memory`: loads the full repository from a JSONL file on startup; all reads
    are served from memory; requires `--repo-file`.
- `--repo-file <path>` — path to the JSONL repository file:
  - `caching` mode: session cache file; default
    `~/.cache/bwb/<project-hash>-<session-id>/repo.jsonl` (written on shutdown
    and periodically; hydrated at startup from a previous session file if it
    exists). Each session has its own file — multi-session cache consolidation
    is out of scope.
  - `beads` mode: informational only (not read or written); default
    `~/.cache/bwb/<project-hash>/repo.jsonl`.
  - `memory` mode: required; the file is the sole source of truth.

Non-interactive flags (`--help`, `--version`, `--print-config`,
`--check-config`) return without booting the Bubble Tea program.

### Path resolution and examples

- `--config` sets an explicit config file path. Relative paths resolve against
  the process start cwd.
- `--cwd` sets the target beads project directory used by repository commands.
  Relative paths also resolve against process start cwd.
- `--print-config` loads config, prints the resolved source comment and YAML,
  then exits.
- `--check-config` loads config, emits warnings, prints `config OK`, then exits.

Examples:

```bash
bwb --config "$HOME/.config/bwb/config.yaml"
bwb --cwd ../another-project
bwb --config "$HOME/.config/bwb/config.yaml" --print-config
bwb --check-config
```

### Exit-code contract for non-interactive paths

| Condition | Exit code |
| --- | --- |
| Successful `--help`, `--version`, `--print-config`, `--check-config` | `0` |
| Runtime/config failures (cwd/config load, config marshal, etc.) | `1` |
| CLI usage failures (unknown flag, unexpected positional args) | `2` |

### Version/build metadata behavior

- `internal/version.Version`, `Commit`, and `Date` default to `dev` /
  `unknown` / `unknown` for local builds (see `internal/version/version.go`).
- Release/snapshot builds inject version metadata via GoReleaser ldflags
  (see `.goreleaser.yaml`):
  - `-X github.com/hk9890/beads-workbench/internal/version.Version={{ .Version }}`
  - `-X github.com/hk9890/beads-workbench/internal/version.Commit={{ .ShortCommit }}`
  - `-X github.com/hk9890/beads-workbench/internal/version.Date={{ .Date }}`
- The `mise run build` task also injects the same three symbols using
  `git describe` / `git rev-parse` / `date -u` for local dev builds.

### Debug diagnostics contract

`--debug` mirrors startup-resolution and `bd`-execution diagnostics to stderr
(prefixed `[bwb-debug]`); every config-loading startup path also writes
structured JSON Lines records to a persistent per-session log. `docs/MONITORING.md`
owns the full contract — event categories, log paths, `session_id` correlation,
fallback behavior, and capture commands.

## Package Layout

Current bootstrapped layout:

```
cmd/
  bwb/               # primary TUI binary entrypoint
  bwb-smoke/         # release-smoke data-consistency check binary (mise run smoke)
internal/
  app/               # Bubble Tea root shell: mode ownership, routing, selection/detail coordination
  config/            # runtime configuration model + defaults
  domain/            # Beads Workbench issue and dashboard models
  repository/beads/     # bd subprocess runner and argv-level types (CommandRunner, RunnerConfig, ExecResult)
  repository/beads/  # lean repository.Repository built directly on CommandRunner; typed bd payload decoding
  logging/           # central slog logging package used by runtime startup and repository tracing
  launcher/          # external editor and command launch actions
  dashboard/         # dashboard metadata catalog (section IDs/titles) + provider interface + validation guardrails
  mode/              # board/search/details feature models + shell message contracts
  ui/                # reusable rendering components (board, search, details, modal, toaster, loading, overlay, fatalerror, shared, styles)
  testing/           # fakes, ui harness, datasets, embedded-fixture helpers
  version/           # build-time injected Version/Commit/Date symbols
project-plan/        # product, architecture, and execution planning docs
```

## Core Architectural Rules

1. **No direct SQL.** All issue reads and writes go through `repository.Repository`. No `database/sql`, no Dolt server client, no BQL executor in the primary product path.

2. **Official beads surfaces only.** The `beads.Repository` implementation talks to `bd` CLI commands. Do not read beads internals directly.

3. **Repository is source-specific.** A `beads.Repository` instance is bound to one beads project. Federation is a future layer above repositories, not a change to the core interface.

4. **Dashboard renderer and dashboard provider are separate.** The provider (`internal/dashboard`) is a metadata-only catalog: it returns section IDs and titles only. The board model owns repository query routing for each section (three parallel `Query` / `ReadyExplain` calls, fanned out after the provider responds). A file-backed provider can be added later by supplying section IDs and titles without touching the renderer or the board model's query logic.

5. **Editor handoff is a first-class flow.** Rich issue editing opens `$EDITOR` rather than building complex inline forms.

   **Issue edit document contract (v1):**
   - Editable fields map directly to repository update capabilities: `title`, `description`, `status`, `type`, `priority`, `assignee`, and `labels`.
   - Read-only context (issue id, timestamps, notes, dependencies, related items, comments) is rendered for operator context and ignored by parser/diff logic.
   - Round-trip behavior is marker-based (`BWB:EDITABLE` / `BWB:FIELD:*`) so parser changes are deterministic and testable.
   - The external editor launch is behind a replaceable seam (`internal/launcher/editor.Opener`) so tests never spawn a real interactive editor.

6. **Launchers are thin.** Launchers receive issue context and produce a subprocess. They must not become an orchestration engine.

   **Launcher behavior contract (v1):**
   - `internal/launcher.Service` resolves an action name to one configured command template.
   - Interpolation is simple placeholder replacement (no scripting/conditionals).
   - Launchers start a subprocess and return immediately (no process supervision/retry).
   - Launch success/failure is surfaced in shell toast feedback.

   **Shell-launcher security rule:** Launcher templates that use `sh -c` or
   `sh -lc` MUST NOT interpolate issue fields into the shell body argument.
   Issue fields (title, assignee, labels, etc.) are operator-untrusted input;
   embedding them in the body allows shell injection. Instead, pass issue field
   placeholders as additional positional arguments after the body, and reference
   them via `$0`, `$1`, `$2` … inside the script. Example:

   ```yaml
   # SAFE — issue fields are positional args, never re-parsed as code
   command: sh
   args:
     - "-lc"
     - "printf 'id=%s title=%s\n' \"$0\" \"$1\""
     - "{{issue.id}}"
     - "{{issue.title}}"

   # UNSAFE — do not do this
   args:
     - "-lc"
     - "printf 'id=%s title=%s\n' \"{{issue.id}}\" \"{{issue.title}}\""
   ```

7. **Create vs edit ownership boundary is explicit.** The rich marker-based document flow currently owns **issue editing** (`e` in detail context). Issue creation remains on the existing create/update task boundary and is not coupled to this editor document contract.

8. **App shell owns mode lifecycle and cross-mode coordination.** `internal/app` owns active-mode switching, selection ownership by mode, and detail loading/reloading decisions. `internal/mode/*` packages own feature-local state and emit shell contracts (`SelectionChangedMsg`, `ActionRequestMsg`) instead of reaching across package boundaries.

9. **Selection/detail sync is event-driven, not polled.** Browse modes emit `SelectionChangedMsg` when selection changes; app reacts by updating shared selection state and (when needed) issuing detail loads. Do not reintroduce polling-based synchronization loops.

10. **Repository decoding is typed and operation-scoped.** `internal/repository/beads` decodes command output through typed payload structs and explicit mappers (for example `RunJSON[T]` + `bd*Payload` types). Avoid `map[string]any`/generic map decoding paths for primary read flows.

11. **Dashboard provider output must validate before rendering.** Board rendering consumes `dashboard.Definition` values only after `dashboard.ValidateDefinitions` checks. Validation enforces non-empty IDs, titles, and sections. Query payload validation is no longer enforced at the provider boundary; the board model owns repository query routing and validates query types internally.

## UI Rendering Conventions

**Comment ordering:** The detail view renders comments newest-first (sorted by
`CreatedAt` descending; ties broken by `ID` descending). This diverges from
`bd comments` default order (oldest-first) and is intentional — surfacing the
most recent activity at the top makes triage faster when an issue has many
comments. The section header reads "Comments (N · newest first)" to make the
ordering obvious to the reader.

## Runtime Configuration

Runtime config loading, the config model, keybindings, and the launcher
interpolation reference live in `docs/CONFIGURATION.md`. The shell-launcher
security rule and the editor/launcher handoff rules are in the Core
Architectural Rules above; UI component placement is in `docs/OVERVIEW.md`
under Architectural boundaries.

## Donor Migration Rules (Perles → Beads Workbench)

This section applies only to a maintainer performing the Perles migration with a
local checkout of the donor repo; the paths below are maintainer-local and not
part of this repository.

When adapting code from the donor repo (Perles, checked out locally — paths
below assume `/home/hans/dev/github/perles`), prefer **small, isolated UI
primitives** and keep imports local to rendering concerns.

### Allowed donor paths (UI primitive scope)

- `/home/hans/dev/github/perles/internal/ui/shared/modal/`
- `/home/hans/dev/github/perles/internal/ui/shared/toaster/`
- `/home/hans/dev/github/perles/internal/ui/styles/`
- `/home/hans/dev/github/perles/internal/ui/shared/overlay/` (only as a rendering helper used by UI primitives)

Typical adapted local targets in this repo are `internal/ui/modal/`,
`internal/ui/toaster/`, `internal/ui/styles/`, and `internal/ui/overlay/`.

### Forbidden donor paths (do not copy into standalone shell)

- `internal/bql/**`
- `internal/orchestration/**`
- `internal/control-plane/**`, `internal/control_plane/**`
- `internal/store/**`, `internal/sql/**`, or any direct `database/sql` usage
- Any package that requires Perles service containers, session orchestration, or donor runtime wiring

### Adaptation requirements

1. Keep APIs small and shell-focused (modal prompts, toast feedback, shared style/render helpers).
2. Remove donor-specific assumptions, including SQL/BQL/orchestration/service-container dependencies.
3. Prefer value-oriented, reusable helpers with explicit inputs/outputs over hidden global state.
4. Keep package boundaries under `internal/ui/*` aligned to standalone ownership.

## Enforced Architecture Guardrails

Automated guardrails are enforced in `cmd/bwb/architecture_guardrails_test.go` by checking the full dependency graph for `./cmd/bwb` (`go list -deps ./cmd/bwb`).

The checks fail if any dependency in the active product path violates these boundaries:

1. **No direct SQL in the active product path.**
   - Forbidden at minimum: `database/sql`, `database/sql/driver`

2. **No `internal/bql` dependency in the standalone app.**
   - Any import path containing `/internal/bql` is forbidden.

3. **No orchestration/control-plane subsystem in the active product path.**
   - Any import path segment matching `orchestration`, `control-plane`, or `control_plane` is forbidden.

These checks are intentionally lightweight and local-friendly: they run as a normal Go test and require no external services.

## Quality Gates

The repository uses `.mise.toml` tasks as the execution layer. Run `mise tasks` to see all available tasks.

Key tasks:

| Task | What it runs |
|---|---|
| `mise run build` | `go build ./cmd/bwb` |
| `mise run vet` | `go vet ./...` |
| `mise run test` | unit tests only (no `//go:build integration` tests) |
| `mise run test:integration` | integration tests (real `bd` + embedded fixture) |
| `mise run test:all` | unit + integration |
| `mise run test:verbose` | unit tests with `-v` |
| `mise run lint` | pinned `golangci-lint` (version from `.mise.toml` `[tools]`) |
| `mise run guardrails` | `go test ./cmd/bwb -run TestArchitectureGuardrails` |
| `mise run fmt:check` | `goimports` formatting + `go mod tidy` cleanliness check (CI-enforced) |
| `mise run scripts:check` | shell + Python script syntax validation (CI-enforced) |
| `mise run test:coverage` | unit+integration tests with a coverage-threshold gate (CI-enforced) |
| `mise run quality` | full pre-handoff gate: `vet`, `lint`, `guardrails`, unit `test`, `test:integration` |
| `mise run quality:fast` | fast pre-commit gate: `vet`, `lint`, `guardrails`, unit `test` (skips `test:integration` only) |
| `mise run smoke` | build + run the `bwb-smoke` release data-consistency check |
| `mise run vuln` | `govulncheck ./...` — CVE scan against deps + stdlib (CI-enforced; needs network) |
| `mise run hooks:install` | `git config core.hooksPath scripts/git-hooks` |

**Unit vs integration distinction:** Unit tests (`mise run test`) are fast and have no external dependencies. Integration tests (`mise run test:integration`) fork real `bd` subprocesses and use the embedded fixture harness; they are gated behind `//go:build integration` in `*_integration_test.go` files. If your test forks a real subprocess, replays the embedded fixture, or costs >1s, it belongs in an integration test file.

**Tool version pins:** `golangci-lint` and `gotestsum` are pinned in `.mise.toml` under `[tools]` (no leading `v`, e.g. `2.1.6`). `mise` installs and resolves these binaries on the `PATH` for tasks like `mise run lint` and `mise run test`.

For the authoritative pre-handoff landing workflow, see
`docs/CHANGE-WORKFLOW.md#code-change-verification-sequence`.

`mise run quality` covers:

- `go vet ./...`
- pinned `golangci-lint` execution using the version in `.mise.toml`
- fast architecture-guardrail verification via
  `go test ./cmd/bwb -run TestArchitectureGuardrails`
- unit tests and integration tests

CI (`.github/workflows/ci.yml`) runs a **superset** of this on an
ubuntu/macos/windows matrix: it additionally runs `scripts:check`,
`fmt:check`, `build`, and the `test:coverage` threshold gate. A change can
pass local `mise run quality` and still fail CI on one of those, so also run
`mise run fmt:check` and `mise run scripts:check` before handoff.

### `golangci-lint` install/invocation policy

- Version pin lives in `.mise.toml` under `[tools]`
  (`"go:github.com/golangci/golangci-lint/v2/cmd/golangci-lint"`).
- Local and CI invocation both use the `mise`-installed binary on `PATH`:
  `mise run lint` runs `golangci-lint run --timeout=5m`. CI activates `mise`
  via `jdx/mise-action@v4` (see `.github/workflows/ci.yml`), so contributors
  do not need a separate global install.
- Lint scope is intentionally minimal for this repo: `staticcheck` and
  `errcheck` only (configured in `.golangci.yml`).
- The initial lint pass is intentionally scoped to non-test packages
  (`run.tests: false`) to keep rollout conservative and signal high.

See `project-plan/ARCHITECTURE.md` for the full architecture definition, interface contracts, and `project-plan/IMPLEMENTATION.md` for phase sequencing and donor reuse strategy.
