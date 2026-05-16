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

Non-interactive flags (`--help`, `--version`, `--print-config`,
`--check-config`) return without booting the Bubble Tea program.

### Path resolution and examples

- `--config` sets an explicit config file path. Relative paths resolve against
  the process start cwd.
- `--cwd` sets the target beads project directory used by gateway commands.
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

- `main.version` defaults to `dev` for local builds.
- Release/snapshot builds inject version metadata via GoReleaser ldflags:
  `-X main.version={{ .Version }}` (see `.goreleaser.yaml`).

### Debug diagnostics contract

When `--debug` is set, diagnostics are emitted to stderr and prefixed with:

```text
[bwb-debug]
```

Event categories:

- startup resolution (`resolved config path`, `resolved cwd`, `auto-refresh`)
- one per-run `session_id` line for correlation
- `bd` execution traces from the command runner (operation, full argv,
  `exit_code`, `duration_ms`)

All config-loading startup paths initialize `internal/logging`, including
interactive startup plus non-interactive `--print-config` and `--check-config`.
That logger writes structured JSON Lines diagnostics to the default state path:

- `$XDG_STATE_HOME/bwb/bwb.log`
- fallback: `~/.local/state/bwb/bwb.log`

Warnings and errors are mirrored to both stderr and the persistent log. When the
persistent sink cannot be opened, BWB emits one stderr warning and continues
with stderr-only logging.

For the current diagnostics/logging surface and capture guidance, see
`docs/MONITORING.md`.

## Package Layout

Current bootstrapped layout:

```
cmd/bwb/             # binary entrypoint
internal/
  app/               # Bubble Tea root shell: mode ownership, routing, selection/detail coordination
  config/            # runtime configuration model + defaults
  domain/            # Beads Workbench issue and dashboard models
  gateway/beads/     # BeadsGateway interface + CLI adapter with typed bd payload decoding
  logging/           # central slog logging package used by runtime startup and gateway tracing
  launcher/          # external editor and command launch actions
  dashboard/         # dashboard metadata catalog (section IDs/titles) + provider interface + validation guardrails
  mode/              # board/search/details feature models + shell message contracts
  ui/                # reusable rendering components (loading, modal, toaster, styles)
project-plan/        # product, architecture, and execution planning docs
```

## Core Architectural Rules

1. **No direct SQL.** All issue reads and writes go through the `BeadsGateway` interface. No `database/sql`, no Dolt server client, no BQL executor in the primary product path.

2. **Official beads surfaces only.** The gateway implementation talks to `bd` CLI commands. Do not read beads internals directly.

3. **Gateway is source-specific.** A gateway instance is bound to one beads project. Federation is a future layer above gateways, not a change to the core interface.

4. **Dashboard renderer and dashboard provider are separate.** The provider (`internal/dashboard`) is a metadata-only catalog: it returns section IDs and titles only. The board model owns gateway query routing for each section (three parallel `Query` / `ReadyExplain` gateway calls, fanned out after the provider responds). A file-backed provider can be added later by supplying section IDs and titles without touching the renderer or the board model's query logic.

5. **Editor handoff is a first-class flow.** Rich issue editing opens `$EDITOR` rather than building complex inline forms.

   **Issue edit document contract (v1):**
   - Editable fields map directly to gateway update capabilities: `title`, `description`, `status`, `type`, `priority`, `assignee`, and `labels`.
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

10. **Gateway decoding is typed and operation-scoped.** `internal/gateway/beads` decodes command output through typed payload structs and explicit mappers (for example `RunJSON[T]` + `bd*Payload` types). Avoid `map[string]any`/generic map decoding paths for primary read flows.

11. **Dashboard provider output must validate before rendering.** Board rendering consumes `dashboard.Definition` values only after `dashboard.ValidateDefinitions` checks. Validation enforces non-empty IDs, titles, and sections. Query payload validation is no longer enforced at the provider boundary; the board model owns gateway query routing and validates query types internally.

## Runtime Configuration (v1)

Configuration lives in `internal/config` and is loaded once at startup via
`config.Load()`.

Runtime config path resolution uses `os.UserConfigDir()` and looks for:

- `bwb/config.yaml`
- on Linux this is typically `~/.config/bwb/config.yaml`

Load semantics:

- Missing config file is allowed; BWB starts with defaults.
- Explicit config file values override environment-driven defaults.
- Unknown YAML keys are ignored and surfaced as startup warnings.
- Invalid YAML, unreadable existing config files, invalid values, and duplicate
  launcher actions fail startup.

The v1 model is intentionally small and only covers app-shell concerns:

- `Editor.Command`
  - Defaults to `$EDITOR` when set.
  - Falls back to `vi` when `$EDITOR` is unset/empty.
  - `editor.command` in `config.yaml` overrides both.
- `Launcher.Definitions`
  - Defaults to four built-in launcher actions:
    - `editor` → launches the resolved editor command (`editor.command`, else `$EDITOR`, else `vi`).
    - `nvim` → launches `nvim` with a read-only issue context buffer seeded from
      interpolation placeholders.
    - `opencode` → launches `opencode run` with issue metadata args/env.
    - `shell-command` → launches `sh -lc` with a simple formatted issue-context
      print command.
  - Each definition supports:
    - `Action` (required unique action key)
    - `Command` (required executable/template string)
    - `Args` (optional argv templates)
    - `Env` (optional `KEY=value` templates)
    - `WorkDir` (optional working-directory template; defaults to project root)
  - YAML launcher overrides merge by `Action`:
    - matching built-ins are replaced field-by-field from the provided override
    - new action names are appended
    - unspecified built-ins remain available
    - `Args` and `Env` follow nil-sentinel semantics: omitting the key in YAML
      leaves the field nil in the override struct, so the built-in value is
      preserved; writing `args: []` produces a non-nil empty slice that
      **replaces** the built-in args (use this to explicitly clear defaults)
- `UI.ShowModeSwitcherHelp`
  - Defaults to `true`.
  - Controls whether the shell renders the mode hotkey hint line.

Example config:

```yaml
editor:
  command: nvim

launcher:
  definitions:
    - action: opencode
      command: opencode-dev
      args:
        - run
        - --issue
        - "{{issue.id}}"
    - action: tmux-note
      command: tmux
      args:
        - new-window
        - "issue {{issue.id}}"

ui:
  show_mode_switcher_help: false

keybindings:
  shell:
    quit: [ctrl+q]
    toggle_help: [F1]
  board:
    move_left: [left]
    move_right: [right]
  search:
    cycle_focus_next: [ctrl+n]
    cycle_focus_prev: [ctrl+p]
    open_detail: [space]
  detail:
    scroll_down: [ctrl+d]
    scroll_up: [ctrl+u]
  modal:
    enter: [space]
    escape: [q]
```

### Keybindings (v1)

Keybindings are resolved once at startup from the `keybindings` section.

- Supported contexts: `shell`, `board`, `search`, `detail`, `modal`
- Overrides merge per action; you only need to specify actions you want to change
- Unknown actions, invalid key names, missing required actions, and conflicting keys
  in the same context fail startup
- Search query typing still accepts normal text input; only configured search actions
  intercept keys in search mode

Supported actions by context:

- `shell`
  - `quit`, `toggle_help`, `mode_board`, `mode_search`, `toggle_search`,
    `mode_detail`, `mode_cycle_next`, `mode_cycle_prev`, `escape`,
    `reload_detail`, `edit_issue`, `create_issue`, `update_issue`,
    `close_issue`, `comment_issue`, `launch_nvim`, `launch_opencode`,
    `launch_shell_command`
- `board`
  - `move_left`, `move_right`, `move_up`, `move_down`, `open_detail`, `reload`
- `search`
  - `move_up`, `move_down`, `focus_left`, `focus_right`, `focus_query`,
    `reload`, `open_detail`, `cycle_focus_next`, `cycle_focus_prev`
- `detail`
  - `scroll_up`, `scroll_down`, `page_up`, `page_down`, `home`, `end`
- `modal`
  - `next`, `prev`, `left`, `right`, `enter`, `escape`

### Launcher interpolation/context surface

Launcher templates support these placeholders across `Command`, every `Args`
entry, every `Env` entry, and `WorkDir`:

- `{{issue.id}}`
- `{{issue.title}}`
- `{{issue.labels}}` (comma-joined label list)
- `{{issue.assignee}}`
- `{{project.root}}`

Notes:

- Unsupported placeholders are passed through literally.
- Empty issue fields interpolate as empty strings.
- `WorkDir` falls back to project root when blank.

### Shell editor/launcher UX behavior (v1)

- `e` opens the rich marker-based issue edit document flow for the currently
  selected issue via `services.Editor`.
- `n`, `p`, `l` trigger `nvim`, `opencode`, `shell-command` launchers from
  detail mode.
- If no issue is selected, the shell shows a warning toast and does not launch.
- Successful rich editor updates trigger detail reload; launchers remain
  non-blocking and do not auto-refresh issue detail.
- Launcher actions are explicitly **background fire-and-forget** in v1 (no
  managed terminal handoff/return contract). After launching, the app stays
  active and shows guidance toast text; use `e` for edit/save flows that round
  trip back into app state with detail reload.

The rich marker-based edit document flow in `internal/launcher/editor` is the
actual interactive shell edit path. Launcher definitions remain a separate
external-tool surface for non-edit actions.

### Testing references for editor/launcher behavior

- Config defaults and built-ins: `internal/config/model_test.go`
- Interpolation/runner behavior: `internal/launcher/service_test.go`
- Shell key wiring and launcher actions: `internal/app/model_test.go`
- Editor round-trip service seam: `internal/launcher/editor/service_test.go`
- Embedded fixture smoke coverage for edit flow: `internal/app/model_test.go`

For broader policy and full-app verification expectations, see `docs/TESTING.md`.

Shared shell feedback primitives live under `internal/ui/`:

- `ui/loading` renders loading/status feedback for board, search, and detail
  surfaces.
- `ui/toaster` renders transient error/warn/info/success feedback.
- `ui/modal` renders help/confirmation overlays.
- `ui/shared/issuerow` owns compact issue row rendering for list-like surfaces
  (board/search): selected-row prefix, type/priority/status/ID token assembly,
  and width-aware truncation.

UI component responsibility boundary:

- **Row component**: `ui/shared/issuerow` is the single compact issue-row
  renderer for board/search-style lists.
- **Optional list component**: there is currently **no shared issue-list
  component**. Row-level sharing is sufficient for now because the remaining
  board/search containers differ materially in layout and behavior.
- **Panel/container component**: `ui/styles.FormSection` is the generic rounded
  bordered section/container primitive used to frame board columns, search
  panes, and detail shells.
- **Detail component**: `ui/details` is the dedicated issue-detail renderer and
  stays separate from compact row/list rendering.

Issue-list responsibility boundary:

- Keep **row rendering** shared via `ui/shared/issuerow`.
- Keep **list/panel containers** mode-specific for now (`ui/board` columns vs
  `ui/search` query/results/preview panes), because their layout, empty-state,
  focus, and composition responsibilities still differ materially.
- If future changes create meaningful duplication above the row level, extract a
  minimal list component under `internal/ui/shared/` with focused tests.

Design intent:

- Keep config loading and access behind a clear boundary (`internal/config` +
  `app.Services.Config`).
- Avoid introducing legacy-style broad config surfaces (custom views, BQL,
  orchestration settings, etc.).

## Donor Migration Rules (Perles → Beads Workbench)

When adapting code from the donor repo (`/home/hans/dev/github/perles`), prefer **small, isolated UI primitives** and keep imports local to rendering concerns.

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
| `mise run lint` | pinned `golangci-lint` via `.golangci-version` |
| `mise run guardrails` | `go test ./cmd/bwb -run TestArchitectureGuardrails` |
| `mise run quality` | full pre-handoff gate (scripts, lint, guardrails, build, vet, test) |
| `mise run quality:fast` | lighter in-flight check (build, vet, test) |
| `mise run hooks:install` | `git config core.hooksPath scripts/git-hooks` |

**Unit vs integration distinction:** Unit tests (`mise run test`) are fast and have no external dependencies. Integration tests (`mise run test:integration`) fork real `bd` subprocesses and use the embedded fixture harness; they are gated behind `//go:build integration` in `*_integration_test.go` files. If your test forks a real subprocess, replays the embedded fixture, or costs >1s, it belongs in an integration test file.

**`golangci-lint` version pin:** The version is in `.golangci-version` (leading-v convention, e.g. `v2.1.6`). The `mise run lint` task reads this file automatically. Similarly, `gotestsum` is pinned in `.gotestsum-version`.

For the authoritative pre-handoff landing workflow, see
`docs/CHANGE-WORKFLOW.md#code-change-verification-sequence`.

That verification sequence covers:

- script syntax validation for `internal/testing/e2e/embeddedfixture/setup.sh`
  and `scripts/*.py`
- pinned `golangci-lint` execution using `.golangci-version`
- fast architecture-guardrail verification via
  `go test ./cmd/bwb -run TestArchitectureGuardrails`
- core implementation gates: `go build ./cmd/bwb`, `go vet ./...`, and
  unit tests

### `golangci-lint` install/invocation policy

- Version pin lives in `.golangci-version`.
- Local and CI invocation both use `go run ...@${GOLANGCI_LINT_VERSION}` so
  contributors do not need a separate global install.
- Lint scope is intentionally minimal for this repo: `staticcheck` and
  `errcheck` only (configured in `.golangci.yml`).
- The initial lint pass is intentionally scoped to non-test packages
  (`run.tests: false`) to keep rollout conservative and signal high.

See `project-plan/ARCHITECTURE.md` for the full architecture definition, interface contracts, and `project-plan/IMPLEMENTATION.md` for phase sequencing and donor reuse strategy.
