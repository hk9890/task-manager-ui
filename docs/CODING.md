# Coding

Repository-specific implementation constraints, for a change under `cmd/`, `internal/` or
`scripts/`, or to `.mise.toml` or `.github/workflows/ci.yml`. Go, Bubble Tea, module
`github.com/hk9890/task-manager-ui`, binary `taskmgr-ui`.

**Another doc outranks this file inside its area** — read the row before you edit:

| Area | Doc |
|---|---|
| `internal/ui/`, `internal/mode/` | [DESIGN-GUIDE.md](DESIGN-GUIDE.md) — colour roles, glyphs, chrome, selection, overlays, width math |
| `internal/config/` | [CONFIGURATION.md](CONFIGURATION.md) — the config model, keybinding resolution, launcher interpolation |
| `internal/logging/` | [MONITORING.md](MONITORING.md) — the record shape, the sink, and what `--debug` mirrors |
| `scripts/capture_taskmgr_ui_screen.py` | [RUNNING.md](RUNNING.md) — the `--step` vocabulary and failure strings a change must keep |
| `scripts/git-hooks/` | [CHANGE-WORKFLOW.md](CHANGE-WORKFLOW.md) — what the hook does, and `mise run hooks:install` |

## Build and run

```bash
mise run build       # the binary, with version metadata injected
mise run ci          # the merge gate; see What the tools enforce
```

## Core Architectural Rules

1. **No direct SQL.** All issue reads and writes go through `repository.Repository`. No `database/sql` and no direct database access in the primary product path.

2. **SDK surfaces only.** The `taskmgr.Repository` implementation talks to the task-manager Go SDK (`github.com/hk9890/task-manager/sdk/tasks`) in-process. There is no external CLI binary in the product path; do not read the store's internals directly.

3. **Repository is source-specific.** A `taskmgr.Repository` instance is bound to one task-manager store (one project directory). Federation is a future layer above repositories, not a change to the core interface.

4. **Dashboard composition is centralized.** `internal/dashboard.Compose` is pure: `dashboard.Inputs` in, `dashboard.Columns` out. The board model maps one `repository.DashboardData` result into `Inputs` (`internal/mode/board/model.go`) and owns query routing; the renderer stays independent of where the data came from.

5. **Editor handoff is a first-class flow.** Rich issue editing opens `$EDITOR` rather than building complex inline forms. The launch goes through `tea.Exec`, which suspends the TUI and restores it when the editor exits, so the editor and Bubble Tea never contend for the terminal.

   **Issue edit document contract:**
   - Editable fields map directly to repository update capabilities: `title`, `description`, `status`, `type`, `priority`, `assignee`, and `labels`.
   - Round-trip behavior is marker-based (`TASKMGRUI:EDITABLE` / `TASKMGRUI:FIELD:*`) so parser changes are deterministic and testable.
   - The external editor launch is behind a replaceable seam (`internal/launcher/editor.Service`) so tests never spawn a real interactive editor.

6. **Launchers are thin.** Launchers receive issue context and produce a subprocess. They must not become an orchestration engine.

   **Shell-launcher security rule:** issue fields are operator-untrusted input, and a
   launcher template MUST NOT place one where it is re-parsed as a command line.
   `launcher.ValidateDefinitions` (`internal/launcher/service.go`) rejects such a
   definition at startup and under `--check-config`.
   [CONFIGURATION.md](CONFIGURATION.md#writing-a-launcher-template-safely) has the two
   forbidden shapes and the safe form a config author writes.

7. **Create vs edit ownership boundary is explicit.** The rich marker-based document flow owns **issue editing** (the shell `edit_issue` binding, default `e`, acting on the current selection from any browse tab or detail). Issue creation remains on the existing create/update task boundary and is not coupled to this editor document contract.

8. **App shell owns mode lifecycle and cross-mode coordination.** `internal/app` owns active-mode switching, selection ownership by mode, and detail loading/reloading decisions. `internal/mode/*` packages own feature-local state and emit shell contracts (`SelectionChangedMsg`, `ActionRequestMsg`) instead of reaching across package boundaries.

9. **Selection/detail sync is event-driven, not polled.** Browse modes emit `SelectionChangedMsg` when selection changes; app reacts by updating shared selection state and (when needed) issuing detail loads. Do not reintroduce polling-based synchronization loops.

10. **Repository mapping is typed and operation-scoped.** `internal/repository/taskmgr` maps the SDK's typed model onto taskmgr-ui's domain types through explicit converters (see `convert.go`). Avoid `map[string]any`/generic map decoding paths for primary read flows.

## CLI startup semantics

`taskmgr-ui --help` is the flag roster. Only what `--help` cannot say is written here.

- `--help`, `--version`, `--print-config` and `--check-config` return without booting
  Bubble Tea. Everything else starts the TUI.
- Relative `--config` and `--cwd` paths resolve against the process start cwd.
- `--store-name` takes precedence over `--cwd` for store selection.
- `--print-config` prints the resolved source comment and YAML, then exits; launcher
  validation runs first, so an unsafe definition exits `1` here too.
- `--check-config` loads config, emits warnings, validates the launcher definitions the
  same way an interactive start does, then prints `config OK` and exits. An unsafe or
  malformed definition exits `1` — the point of the command is that it reaches the same
  verdict as a start.

### Store resolution

`buildRepository` must call `tasks.Resolve`, never `tasks.Open`: `Open` performs local
discovery only, so a project whose store was promoted with `taskmgr store move --central`
would report "no .tasks directory found". `Resolve` applies the same precedence as the
`taskmgr` CLI — see the SDK for the order.

Nothing resolving is an error: startup fails with exit code `1` rather than booting
against an empty board.

The project root the app runs with is the resolved store's project path, not the target
directory. That is what `{{project.root}}` interpolates to
([CONFIGURATION.md](CONFIGURATION.md#launcher-interpolationcontext-surface)) and it differs
from the working directory whenever the app is started from a subdirectory or against a
central store. `--repo=memory` has no store to ask, so it keeps the target directory.

`TASKMGR_DIR` is rejected by the SDK rather than honored; unset it and use `--cwd` or
`--store-name`.

### Exit-code contract for non-interactive paths

| Condition | Exit code |
| --- | --- |
| Successful `--help`, `--version`, `--print-config`, `--check-config` | `0` |
| Runtime/config failures (cwd/config load, config marshal, etc.) | `1` |
| CLI usage failures (unknown flag, unexpected positional args) | `2` |

### Version/build metadata

`internal/version.Version`, `Commit` and `Date` default to `dev` / `unknown` / `unknown`.
Release builds inject them via GoReleaser ldflags (`.goreleaser.yaml`); `mise run build`
injects the same three symbols for local dev builds (`.mise.toml`).

## Naming Conventions

### Constructor names

- **Feature mode models** (`internal/mode/board`, `internal/mode/docs`, `internal/mode/search`, `internal/app`): use
  `NewModel(...)` returning a named `Model` (or `*Model`). These are stateful Bubble Tea controllers
  with complex dependency injection.
- **UI leaf components** (`internal/ui/toaster`, `internal/ui/modal`): use `New(...)` returning a
  `Model`. These are lightweight rendering components with no cross-cutting dependencies.
- **No constructor** when a mode model is simple enough to zero-initialize directly (e.g.
  `internal/mode/detail.Model` is set up via field assignment in the owning shell).

Do not add `Model` suffix to leaf UI component constructors; do not use bare `New` for feature-level
controllers.

### Package doc placement

Every package carries a package comment. Put it in `doc.go` when the package has more than
one non-test file; inline on the `package` line for a single-file package.
`internal/repository` and `internal/ui/styles` predate this rule and keep theirs in a
source file.

### Test fakes (`internal/testing/fakes`)

Two naming styles coexist by design:

| Style | When to use | Examples |
|---|---|---|
| `Fake<Thing>` + public struct literal | Simple, non-repository seams — typically a one-method interface or command type | `FakeEditor`, `FakeLauncher`, `FakeProcessRunner`, `FakeExecCommand` |
| `<Adjective>Repository` + `New*` constructor | Repository wrappers that carry non-trivial state or construction logic | `ErrorInjectingRepository` / `NewErrorInjecting`, `DelayingRepository` / `NewDelayingRepository` |

New fakes follow the style that matches the complexity: struct literal + `Fake` prefix for thin
seams; named type + constructor for stateful wrappers.

## What the tools enforce

`mise run ci` is the merge gate; the linux CI job runs exactly it, and `ci:portable` is the subset
that runs on macOS too. `mise run quality:fast` is the ~15s pre-commit subset. Run `mise tasks` for
the rest.

This table says where each rule is written. It does not restate the rules — open the source when you
need the exact one.

| Gate | Enforces | Rule lives in |
|---|---|---|
| `lint` | `staticcheck` and `errcheck`, deliberately narrow, non-test packages only | `.golangci.yml` |
| `vet` | the `go vet` suite | the toolchain |
| `guardrails` | the import bans on `./cmd/taskmgr-ui`'s full transitive graph: no `database/sql`, no `orchestration` / `control-plane` / `control_plane` segment | `cmd/taskmgr-ui/architecture_guardrails_test.go` |
| `fmt:check` | `goimports -local github.com/hk9890/task-manager-ui`, and a tidy `go.mod` / `go.sum` | `.mise.toml` |
| `scripts:check` | shell and Python syntax under `scripts/` | `.mise.toml` |
| `test` | the unit tier | [TESTING.md](TESTING.md) |
| `test:coverage` | the unit + integration tiers against a coverage floor — 75 locally, 69 in CI | `.mise.toml`, `.github/workflows/ci.yml` |
| `test:integration` | the real-OS-seam tier, plus three repo hygiene scans: no leaked tracker IDs, no stale doc citations, and no dead link anchors ([DOCUMENTING.md](DOCUMENTING.md)) | `cmd/taskmgr-ui/tracker_id_hygiene_integration_test.go`, `cmd/taskmgr-ui/doc_citation_hygiene_integration_test.go` |

The guardrail and hygiene scans are ordinary Go tests needing no external service. The two hygiene
scans are tagged `//go:build integration` because they shell out to `git` — absent `git` would
otherwise fail every unit test in the package.

**Tool versions** are pinned in `.mise.toml` under `[tools]` with no leading `v`. `mise` puts them on
`PATH`, and CI activates `mise` through `jdx/mise-action`, so no separate global install is needed.
