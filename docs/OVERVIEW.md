# Overview

## Project identity

- Module: `github.com/hk9890/task-manager-ui`
- Binary: `taskmgr-ui`
- Entrypoint: `cmd/taskmgr-ui/main.go`
- Primary runtime surfaces: Bubble Tea UI + an in-process task-manager-backed `repository.Repository`

## Upstream dependency

- task-manager Go SDK: `github.com/hk9890/task-manager/sdk` (imported as
  `github.com/hk9890/task-manager/sdk/tasks`).
- taskmgr-ui is a thin TUI over the in-process task-manager store; there is no external
  backend binary in the product path. The SDK version is pinned in `go.mod`
  (`require github.com/hk9890/task-manager/sdk vX.Y.Z`). When investigating
  repository behavior surprises, file bugs upstream or check the SDK
  source/issues there before patching workarounds in
  `internal/repository/taskmgr/`.

## CLI startup contract

`taskmgr-ui` includes a small pre-TUI CLI layer for help/version/config inspection and
startup overrides. For the full flag list, exit-code contract, and path
resolution behavior, see `docs/CODING.md`.

## Runtime flow

1. `cmd/taskmgr-ui/main.go` parses CLI flags and handles non-interactive exits first.
2. It resolves startup cwd/config options and loads runtime config with
   `internal/config.LoadWithOptions(...)`.
3. It initializes centralized runtime logging, then constructs the
   `repository.Repository` via `constructRepository` in `cmd/taskmgr-ui/main.go`:
   for the default `taskmgr` backend it opens an in-process store with
   `tasks.Open(projectRoot)`, wraps it with
   `repository/taskmgr.New(store, WithAuthor(...))`, and uses that adapter
   directly (no validating decorator, no caching layer).
4. It builds shell services with `internal/app.NewServices(...)`.
5. It starts the TUI with
   `tea.NewProgram(..., tea.WithAltScreen(), tea.WithReportFocus())`.

When `--debug` is enabled, stderr diagnostics are prefixed with `[taskmgr-ui-debug]`
and include startup resolution events plus in-process repository execution
traces (there is no external subprocess argv), while the same run also writes
structured JSON Lines records with `session_id` to the persistent log file. See
`docs/MONITORING.md` for the logging contract and capture paths.

## Package map

| Path | Responsibility |
| --- | --- |
| `cmd/taskmgr-ui` | Binary entrypoint and program bootstrap |
| `internal/app` | Root shell, mode lifecycle, selection/detail coordination |
| `internal/config` | Runtime config model, defaults, YAML loading, keybinding resolution |
| `internal/domain` | Issue, query, mutation, catalog, and error models |
| `internal/repository` | `Repository` interface and shared error/types helpers |
| `internal/repository/taskmgr` | Production `repository.Repository` over the in-process task-manager SDK (`sdk/tasks`); maps the SDK's typed model onto taskmgr-ui domain types |
| `internal/repository/memory` | In-memory `repository.Repository` for tests and `--repo memory`; backed by `internal/repository/filestorage` JSONL load/save |
| `internal/logging` | Central slog-based logging package used by startup and repository code; owns session IDs, persistent JSON Lines logs, stderr mirroring, and fallback behavior |
| `internal/dashboard` | Board column composition (`Compose`) from a `DashboardData` result |
| `internal/mode/*` | Board, search, and details feature-local state/controllers |
| `internal/launcher` | External tool launch actions and process runner |
| `internal/launcher/editor` | Rich issue editor handoff flow |
| `internal/ui/*` | Reusable rendering components and shared styles |
| `internal/testing/*` | Repository fakes and UI test harnesses |
| `internal/version` | Build-time injected `Version`, `Commit`, `Date` symbols (see `docs/CODING.md` Version/build metadata behavior) |
| `.tasks/` | On-disk task-manager store for the project's own dev issue tracking (file-based, managed by `taskmgr`; local-only, not published) |

## Architectural boundaries

- Single repository abstraction: active product behavior goes through `repository.Repository`. The production implementation is `internal/repository/taskmgr`, an in-process adapter over the task-manager SDK, composed via `constructRepository` and used directly. There is no tracker-CLI subprocess in the product path and no caching layer (the in-process SDK is fast enough). The only alternate backend is `--repo memory` (file-backed, for tests/inspection).
- No direct SQL/database access and no orchestration/control-plane dependencies in the active `./cmd/taskmgr-ui` path; see `cmd/taskmgr-ui/architecture_guardrails_test.go`.
- Launchers start subprocesses and return immediately; they do not supervise or orchestrate tools. See `internal/launcher/service.go`.
- Rich issue editing is a separate editor handoff flow under `internal/launcher/editor`.
- Dashboard columns are composed by `internal/dashboard.Compose` from a single `repository.DashboardData`; the board model owns query routing and calls `Compose`.

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
- `ui/scroll` is the shared `EnsureVisible` viewport/offset helper used by board
  and details to keep the selected row visible.
- `ui/overlay` provides ANSI-aware overlay placement used by modal/toaster/app.
- `ui/fatalerror` is the full-screen startup-failure view used by app.

## Supporting docs

`AGENTS.md` is the single entry point that routes to every doc by topic (coding,
configuration, testing, monitoring, releasing, change workflow) — see it instead
of duplicating that index here. Operator keybindings live in
`docs/user-guide/key-bindings.md`.
