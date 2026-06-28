# Coding

## Project Identity

- **Module:** `github.com/hk9890/task-manager-ui`
- **Binary:** `taskmgr-ui` (`cmd/taskmgr-ui`)
- **Language:** Go
- **TUI framework:** Bubble Tea

## Build and Test

Use standard Go tooling from the repository root for build, vet, and test work.
See `docs/CHANGE-WORKFLOW.md` for the authoritative pre-handoff landing
workflow. See [Quality Gates](#quality-gates) for the code-change verification
commands used by that workflow.

For testing strategy, vocabulary, and harness conventions (teatest, golden files,
fake seams), see `docs/TESTING.md`.

## CLI startup semantics (v1)

`cmd/taskmgr-ui/main.go` intentionally keeps a minimal pre-TUI CLI surface before
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
- `--repo <backend>` — repository backend: `taskmgr | memory` (default: `taskmgr`)
  - `taskmgr` (default): in-process implementation over the task-manager Go SDK
    (`github.com/hk9890/task-manager/sdk/tasks`). The store is opened at the
    target project directory; reads and writes run in-process with no subprocess
    or external binary in the product path.
  - `memory`: loads the full repository from a JSONL file on startup; all reads
    are served from memory; requires `--repo-file`.
- `--repo-file <path>` — path to the JSONL repository file:
  - `taskmgr` mode: ignored (not read or written); the task-manager store at the
    project directory is the source of truth.
  - `memory` mode: required; the file is the sole source of truth.

Non-interactive flags (`--help`, `--version`, `--print-config`,
`--check-config`) return without booting the Bubble Tea program.

### Path resolution and examples

- `--config` sets an explicit config file path. Relative paths resolve against
  the process start cwd.
- `--cwd` sets the target project directory used to open the repository backend.
  Relative paths also resolve against process start cwd.
- `--print-config` loads config, prints the resolved source comment and YAML,
  then exits.
- `--check-config` loads config, emits warnings, prints `config OK`, then exits.

Examples:

```bash
taskmgr-ui --config "$HOME/.config/taskmgr-ui/config.yaml"
taskmgr-ui --cwd ../another-project
taskmgr-ui --config "$HOME/.config/taskmgr-ui/config.yaml" --print-config
taskmgr-ui --check-config
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
  - `-X github.com/hk9890/task-manager-ui/internal/version.Version={{ .Version }}`
  - `-X github.com/hk9890/task-manager-ui/internal/version.Commit={{ .ShortCommit }}`
  - `-X github.com/hk9890/task-manager-ui/internal/version.Date={{ .Date }}`
- The `mise run build` task also injects the same three symbols using
  `git describe` / `git rev-parse` / `date -u` for local dev builds.

### Debug diagnostics contract

`--debug` mirrors startup-resolution and repository-operation diagnostics to
stderr (prefixed `[taskmgr-ui-debug]`); every config-loading startup path also writes
structured JSON Lines records to a persistent per-session log. Repository traces
are in-process (no subprocess argv) since the backend runs over the task-manager
SDK. `docs/MONITORING.md` owns the full contract — event categories, log paths,
`session_id` correlation, fallback behavior, and capture commands.

## Package Layout

Current bootstrapped layout:

```
cmd/
  taskmgr-ui/               # primary TUI binary entrypoint
internal/
  app/               # Bubble Tea root shell: mode ownership, routing, selection/detail coordination
  config/            # runtime configuration model + defaults
  domain/            # Task Manager UI issue and dashboard models
  repository/        # repository.Repository interface + shared errors/types
  repository/taskmgr/   # production backend: in-process adapter over the task-manager Go SDK
  repository/memory/    # in-memory backend (loaded from a JSONL file via filestorage)
  repository/filestorage/  # JSONL load/save for the memory backend
  logging/           # central slog logging package used by runtime startup and repository tracing
  launcher/          # external editor and command launch actions
  dashboard/         # board column composition (Compose) from a DashboardData result
  mode/              # board/search/details feature models + shell message contracts
  ui/                # reusable rendering components (board, search, details, modal, toaster, loading, overlay, fatalerror, shared, scroll, styles)
  testing/           # fakes and ui harness helpers
  version/           # build-time injected Version/Commit/Date symbols
```

## Core Architectural Rules

1. **No direct SQL.** All issue reads and writes go through `repository.Repository`. No `database/sql` and no direct database access in the primary product path.

2. **SDK surfaces only.** The `taskmgr.Repository` implementation talks to the task-manager Go SDK (`github.com/hk9890/task-manager/sdk/tasks`) in-process. There is no external CLI binary in the product path; do not read the store's internals directly.

3. **Repository is source-specific.** A `taskmgr.Repository` instance is bound to one task-manager store (one project directory). Federation is a future layer above repositories, not a change to the core interface.

4. **Dashboard composition is centralized.** `internal/dashboard.Compose` turns a single `repository.DashboardData` result into the fixed board columns (`ColumnData`). The board model owns repository query routing and calls `Compose` to lay out the columns; the renderer stays independent of where the data came from.

5. **Editor handoff is a first-class flow.** Rich issue editing opens `$EDITOR` rather than building complex inline forms.

   **Issue edit document contract (v1):**
   - Editable fields map directly to repository update capabilities: `title`, `description`, `status`, `type`, `priority`, `assignee`, and `labels`.
   - Read-only context (issue id, timestamps, notes, dependencies, related items, comments) is rendered for operator context and ignored by parser/diff logic.
   - Round-trip behavior is marker-based (`TASKMGRUI:EDITABLE` / `TASKMGRUI:FIELD:*`) so parser changes are deterministic and testable.
   - The external editor launch is behind a replaceable seam (`internal/launcher/editor.Service`) so tests never spawn a real interactive editor.

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

10. **Repository mapping is typed and operation-scoped.** `internal/repository/taskmgr` maps the SDK's typed model onto taskmgr-ui's domain types through explicit converters (see `convert.go`). Avoid `map[string]any`/generic map decoding paths for primary read flows.

## Naming Conventions

### Constructor names

- **Feature mode models** (`internal/mode/board`, `internal/mode/search`, `internal/app`): use
  `NewModel(...)` returning a named `Model` (or `*Model`). These are stateful Bubble Tea controllers
  with complex dependency injection.
- **UI leaf components** (`internal/ui/toaster`, `internal/ui/modal`): use `New(...)` returning a
  `Model`. These are lightweight rendering components with no cross-cutting dependencies.
- **No constructor** when a mode model is simple enough to zero-initialize directly (e.g.
  `internal/mode/details.Model` is set up via field assignment in the owning shell).

Do not add `Model` suffix to leaf UI component constructors; do not use bare `New` for feature-level
controllers.

### Package doc placement

All packages must have a package-level doc comment. The preferred placement is a `doc.go` file for
packages with multiple files; inline on the `package` declaration for single-file packages. Do not
place the doc in the largest source file and leave sibling files uncommented.

### Test fakes (`internal/testing/fakes`)

Two naming styles coexist by design:

| Style | When to use | Examples |
|---|---|---|
| `Fake<Thing>` + public struct literal | Simple, non-repository seams — typically a one-method interface or command type | `FakeEditor`, `FakeLauncher`, `FakeProcessRunner`, `FakeExecCommand` |
| `<Adjective>Repository` + `New*` constructor | Repository wrappers that carry non-trivial state or construction logic | `ErrorInjectingRepository` / `NewErrorInjecting`, `DelayingRepository` / `NewDelayingRepository` |

New fakes follow the style that matches the complexity: struct literal + `Fake` prefix for thin
seams; named type + constructor for stateful wrappers.

## UI Rendering Conventions

**Comment ordering:** The detail view renders comments newest-first (sorted by
`CreatedAt` descending; ties broken by `ID` descending). This diverges from the
backend's default comment order (oldest-first) and is intentional — surfacing
the most recent activity at the top makes triage faster when an issue has many
comments. The section header reads "Comments (N · newest first)" to make the
ordering obvious to the reader.

## Runtime Configuration

Runtime config loading, the config model, keybindings, and the launcher
interpolation reference live in `docs/CONFIGURATION.md`. The shell-launcher
security rule and the editor/launcher handoff rules are in the Core
Architectural Rules above; UI component placement is in `docs/OVERVIEW.md`
under Architectural boundaries.

## Enforced Architecture Guardrails

Automated guardrails are enforced in `cmd/taskmgr-ui/architecture_guardrails_test.go` by checking the full dependency graph for `./cmd/taskmgr-ui` (`go list -deps ./cmd/taskmgr-ui`).

The checks fail if any dependency in the active product path violates these boundaries:

1. **No direct SQL in the active product path.**
   - Forbidden at minimum: `database/sql`, `database/sql/driver`

2. **No orchestration/control-plane subsystem in the active product path.**
   - Any import path segment matching `orchestration`, `control-plane`, or `control_plane` is forbidden.

These checks are intentionally lightweight and local-friendly: they run as a normal Go test and require no external services.

## Quality Gates

The repository uses `.mise.toml` tasks as the execution layer. Run `mise tasks` to see all available tasks.

Key tasks:

| Task | What it runs |
|---|---|
| `mise run build` | `go build ./cmd/taskmgr-ui` |
| `mise run vet` | `go vet ./...` |
| `mise run test` | unit tests only (no `//go:build integration` tests) |
| `mise run test:integration` | integration tests (build tag: `integration`) |
| `mise run test:all` | unit + integration |
| `mise run test:verbose` | unit tests with `-v` |
| `mise run lint` | pinned `golangci-lint` (version from `.mise.toml` `[tools]`) |
| `mise run guardrails` | `go test ./cmd/taskmgr-ui -run TestArchitectureGuardrails` |
| `mise run fmt:check` | `goimports` formatting + `go mod tidy` cleanliness check (CI-enforced) |
| `mise run scripts:check` | shell + Python script syntax validation (CI-enforced) |
| `mise run test:coverage` | unit+integration tests with a coverage-threshold gate (CI-enforced) |
| `mise run quality` | full pre-handoff gate: `vet`, `lint`, `guardrails`, unit `test`, `test:integration` |
| `mise run quality:fast` | fast pre-commit gate: `vet`, `lint`, `guardrails`, unit `test` (skips `test:integration` only) |
| `mise run vuln` | `govulncheck ./...` — CVE scan against deps + stdlib (local/on-demand; needs network) |
| `mise run hooks:install` | `git config core.hooksPath scripts/git-hooks` |

**Unit vs integration distinction:** See `docs/TESTING.md` for the authoritative decision rule (threshold, seam definition, and the `tasks.Init(t.TempDir())` carve-out). In short: unit tests are fast with no external processes; integration tests (`//go:build integration`) exercise real OS seams such as subprocess execution.

**Tool version pins:** `golangci-lint` and `gotestsum` are pinned in `.mise.toml` under `[tools]` (no leading `v`, e.g. `2.1.6`). `mise` installs and resolves these binaries on the `PATH` for tasks like `mise run lint` and `mise run test`.

For the authoritative pre-handoff landing workflow, see
`docs/CHANGE-WORKFLOW.md#code-change-verification-sequence`.

`mise run quality` covers:

- `go vet ./...`
- pinned `golangci-lint` execution using the version in `.mise.toml`
- fast architecture-guardrail verification via
  `go test ./cmd/taskmgr-ui -run TestArchitectureGuardrails`
- unit tests and integration tests

CI (`.github/workflows/ci.yml`) runs a **superset** of this on an
ubuntu/macos matrix: it additionally runs `scripts:check`,
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

See `docs/OVERVIEW.md` for the architecture map and boundaries.
