# Overview

## Project identity

- Module: `github.com/hk9890/beads-workbench`
- Binary: `bwb`
- Entrypoint: `cmd/bwb/main.go`
- Primary runtime surfaces: Bubble Tea UI + `bd` CLI-backed `repository.Repository`

## Upstream dependency

- `bd` (beads issue tracker): <https://github.com/gastownhall/beads>
- bwb is a thin TUI over `bd`; repository compatibility is pinned in `.mise.toml`
  (`github:gastownhall/beads` version). When investigating repository behavior
  surprises, file bugs upstream or check the `bd` source/issues there before
  patching workarounds in `internal/repository/beads/`.

## CLI startup contract

`bwb` includes a small pre-TUI CLI layer for help/version/config inspection and
startup overrides. For the full flag list, exit-code contract, and path
resolution behavior, see `docs/CODING.md`.

## Runtime flow

1. `cmd/bwb/main.go` parses CLI flags and handles non-interactive exits first.
2. It resolves startup cwd/config options and loads runtime config with
   `internal/config.LoadWithOptions(...)`.
3. It initializes centralized runtime logging, then constructs the
   `repository.Repository` stack (beads, validating, caching) via
   `constructRepository` in `cmd/bwb/main.go`.
4. It builds shell services with `internal/app.NewServices(...)`.
5. It starts the TUI with
   `tea.NewProgram(..., tea.WithAltScreen(), tea.WithReportFocus())`.

When `--debug` is enabled, stderr diagnostics are prefixed with `[bwb-debug]`
and include startup resolution events plus repository execution traces, while the
same run also writes structured JSON Lines records with `session_id` to the
persistent log file. See `docs/MONITORING.md` for the logging contract and
capture paths.

## Package map

| Path | Responsibility |
| --- | --- |
| `cmd/bwb` | Binary entrypoint and program bootstrap |
| `cmd/bwb-smoke` | Release-smoke check binary used by `mise run smoke` for data-consistency verification against a live `bd` repo |
| `internal/app` | Root shell, mode lifecycle, selection/detail coordination |
| `internal/config` | Runtime config model, defaults, YAML loading, keybinding resolution |
| `internal/domain` | Issue, query, mutation, catalog, and error models |
| `internal/bd` | `bd` subprocess runner and argv-level types (`CommandRunner`, `RunnerConfig`, `ExecResult`) |
| `internal/repository/beads` | Lean `repository.Repository` implementation built directly on `CommandRunner`; typed `bd` payload decoding |
| `internal/logging` | Central slog-based logging package used by startup and repository code; owns session IDs, persistent JSON Lines logs, stderr mirroring, and fallback behavior |
| `internal/dashboard` | Built-in dashboard definitions and validation |
| `internal/mode/*` | Board, search, and details feature-local state/controllers |
| `internal/launcher` | External tool launch actions and process runner |
| `internal/launcher/editor` | Rich issue editor handoff flow |
| `internal/ui/*` | Reusable rendering components and shared styles |
| `internal/testing/*` | Fakes, UI harnesses, datasets, and embedded-fixture integration support |
| `internal/version` | Build-time injected `Version`, `Commit`, `Date` symbols (see `docs/CODING.md` Version/build metadata behavior) |
| `project-plan/` | Deeper product, architecture, and implementation planning docs |
| `.beads/` | On-disk beads issue database (Dolt-backed; the `bd` tracker store) |
| `ai.package.yaml` | AI tooling package manifest — declares AI packages/skills used with this repo |

## Architectural boundaries

- Official beads surfaces only: active product behavior goes through `repository.Repository`. The `bd`-backed implementation is `internal/repository/beads.Repository`, composed via `constructRepository` with `repository.NewValidating` and `caching.Repository`.
- No direct SQL, no `internal/bql`, and no orchestration/control-plane dependencies in the active `./cmd/bwb` path; see `cmd/bwb/architecture_guardrails_test.go`.
- Launchers start subprocesses and return immediately; they do not supervise or orchestrate tools. See `internal/launcher/service.go`.
- Rich issue editing is a separate editor handoff flow under `internal/launcher/editor`.
- Dashboard definitions must validate before rendering; see `internal/dashboard/definition.go`.

## UI component boundaries

Rendering components live under `internal/ui/`:

- `ui/shared/issuerow` is the single compact issue-row renderer for
  board/search-style lists; keep row rendering shared here.
- There is intentionally **no shared issue-list component** — board and search
  containers differ materially (layout, empty-state, focus), so list/panel
  containers stay mode-specific (`ui/board` columns vs `ui/search` panes).
  Extract a minimal `internal/ui/shared/` list component only if real
  duplication appears above the row level.
- `ui/styles.FormSection` is the shared rounded-border section/container
  primitive used to frame columns, panes, and detail shells.
- `ui/details` is the dedicated issue-detail renderer, separate from compact
  row/list rendering.
- `ui/loading`, `ui/toaster`, and `ui/modal` provide shared loading, transient
  toast, and overlay feedback primitives.

## Supporting docs

- `docs/CODING.md` — build commands, package layout, architectural rules, and quality gates
- `docs/CONFIGURATION.md` — runtime config model, keybindings, and launcher interpolation reference
- `docs/TESTING.md` — test policy, fixtures, and required verification depth
- `docs/MONITORING.md` — centralized logging contract, capture points, and evidence guidance
- `docs/RUNTIME_UI_VERIFICATION.md` — runtime UI runbook for built-binary checks
- `docs/CHANGE-WORKFLOW.md` — beads-first change landing and session completion workflow
- `docs/RELEASING.md` — tag-triggered release process via GitHub Actions + GoReleaser
- `docs/user-guide/key-bindings.md` — default operator keybindings
- `project-plan/*.md` — deeper product/architecture/implementation intent kept as planning docs, not operator runbooks
