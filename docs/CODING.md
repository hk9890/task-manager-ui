# Coding

Repository-specific implementation constraints, for a change under `cmd/`, `internal/` or
`scripts/`. Go, Bubble Tea, module `github.com/hk9890/task-manager-ui`, binary `taskmgr-ui`.

**Another doc outranks this file inside its area** — read the row before you edit:

| Area | Doc |
|---|---|
| `internal/ui/`, `internal/mode/` | [DESIGN-GUIDE.md](DESIGN-GUIDE.md) — colour roles, glyphs, chrome, selection, overlays, width math |
| `internal/config/` | [CONFIGURATION.md](CONFIGURATION.md) — the config model, keybinding resolution, launcher interpolation |
| `internal/logging/` | [MONITORING.md](MONITORING.md) — the record shape, the sink, and what `--debug` mirrors |

## Build and run

```bash
mise run build       # the binary, with version metadata injected
mise run ci          # the merge gate; see What the tools enforce
```

## CLI startup semantics

`taskmgr-ui --help` is the flag roster. `cmd/taskmgr-ui/main.go` keeps that surface
minimal by design, and only what `--help` cannot say is written here.

`--help`, `--version`, `--print-config` and `--check-config` return without booting
Bubble Tea. Everything else starts the TUI.

### Store resolution

`buildRepository` opens the `taskmgr` backend with `tasks.Resolve`, not
`tasks.Open`: `Open` performs local discovery only, so a project whose store was
promoted with `taskmgr store move --central` would report "no .tasks directory
found". `Resolve` applies the same precedence as the `taskmgr` CLI:

1. `--store-name` — the central store registered under that name, ignoring the
   working directory. An unregistered name is an error, never a fallback.
2. a local `.tasks` directory, found by walking up from the target directory.
3. the central registry (`~/.taskmgr/mapping.yaml`), matched on the longest
   registered project path that is an ancestor of the target directory.

Nothing resolving is an error: startup fails with exit code `1` rather than
booting against an empty board.

The project root the app runs with is the resolved store's project path, not the
target directory. That is what `{{project.root}}` interpolates to
([CONFIGURATION.md](CONFIGURATION.md#launcher-interpolationcontext-surface)) and it differs from
the working directory whenever the app is started from a subdirectory or against
a central store. `--repo=memory` has no store to ask, so it keeps the target
directory.

`TASKMGR_DIR` is rejected by the SDK rather than honored; unset it and use
`--cwd` or `--store-name`.

### Path resolution and examples

- `--config` sets an explicit config file path. Relative paths resolve against
  the process start cwd.
- `--cwd` sets the target project directory the repository backend resolves from.
  Relative paths also resolve against process start cwd.
- `--store-name` takes precedence over `--cwd` for store selection.
- `--print-config` loads config, prints the resolved source comment and YAML,
  then exits.
- `--check-config` loads config, emits warnings, prints `config OK`, then exits.

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
| `test:integration` | the real-OS-seam tier, plus two repo hygiene scans: no leaked tracker IDs, and no stale doc citations ([DOCUMENTING.md](DOCUMENTING.md)) | `cmd/taskmgr-ui/tracker_id_hygiene_integration_test.go`, `cmd/taskmgr-ui/doc_citation_hygiene_integration_test.go` |

The guardrail and hygiene scans are ordinary Go tests needing no external service. The two hygiene
scans are tagged `//go:build integration` because they shell out to `git` — absent `git` would
otherwise fail every unit test in the package.

**Tool versions** are pinned in `.mise.toml` under `[tools]` with no leading `v`. `mise` puts them on
`PATH`, and CI activates `mise` through `jdx/mise-action`, so no separate global install is needed.
