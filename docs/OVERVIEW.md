# Overview

## Project identity

- Module: `github.com/hk9890/beads-workbench`
- Binary: `bwb`
- Entrypoint: `cmd/bwb/main.go`
- Primary runtime surfaces: Bubble Tea UI + `bd` CLI-backed gateway

## Upstream dependency

- `bd` (beads issue tracker): <https://github.com/gastownhall/beads>
- bwb is a thin TUI over `bd`; gateway compatibility is pinned in `.mise.toml`
  (`github:gastownhall/beads` version). When investigating gateway behavior
  surprises, file bugs upstream or check the `bd` source/issues there before
  patching workarounds in `internal/gateway/beads/`.

## CLI startup contract

`bwb` includes a small pre-TUI CLI layer for help/version/config inspection and
startup overrides. For the full flag list, exit-code contract, and path
resolution behavior, see `docs/CODING.md`.

## Runtime flow

1. `cmd/bwb/main.go` parses CLI flags and handles non-interactive exits first.
2. It resolves startup cwd/config options and loads runtime config with
   `internal/config.LoadWithOptions(...)`.
3. It initializes centralized runtime logging, then creates the source-specific
   beads gateway with
   `internal/gateway/beads.NewCLIGateway(...)`.
4. It builds shell services with `internal/app.NewServices(...)`.
5. It starts the TUI with
   `tea.NewProgram(..., tea.WithAltScreen(), tea.WithReportFocus())`.

When `--debug` is enabled, stderr diagnostics are prefixed with `[bwb-debug]`
and include startup resolution events plus gateway execution traces, while the
same run also writes structured JSON Lines records with `session_id` to the
persistent log file. See `docs/MONITORING.md` for the logging contract and
capture paths.

## Package map

| Path | Responsibility |
| --- | --- |
| `cmd/bwb` | Binary entrypoint and program bootstrap |
| `internal/app` | Root shell, mode lifecycle, selection/detail coordination |
| `internal/config` | Runtime config model, defaults, YAML loading, keybinding resolution |
| `internal/domain` | Issue, query, mutation, catalog, and error models |
| `internal/gateway/beads` | Official `bd` CLI adapter and typed payload decoding |
| `internal/logging` | Central slog-based logging package used by startup and gateway code; owns session IDs, persistent JSON Lines logs, stderr mirroring, and fallback behavior |
| `internal/dashboard` | Built-in dashboard definitions and validation |
| `internal/mode/*` | Board, search, and details feature-local state/controllers |
| `internal/launcher` | External tool launch actions and process runner |
| `internal/launcher/editor` | Rich issue editor handoff flow |
| `internal/ui/*` | Reusable rendering components and shared styles |
| `internal/testing/*` | Fakes, UI harnesses, and embedded-fixture integration support |
| `project-plan/` | Deeper product, architecture, and implementation planning docs |

## Architectural boundaries

- Official beads surfaces only: active product behavior goes through `internal/gateway/beads` and the `BeadsGateway` interface in `internal/gateway/beads/interface.go`.
- No direct SQL, no `internal/bql`, and no orchestration/control-plane dependencies in the active `./cmd/bwb` path; see `cmd/bwb/architecture_guardrails_test.go`.
- Launchers start subprocesses and return immediately; they do not supervise or orchestrate tools. See `internal/launcher/service.go`.
- Rich issue editing is a separate editor handoff flow under `internal/launcher/editor`.
- Dashboard definitions must validate before rendering; see `internal/dashboard/definition.go`.

## Supporting docs

- `docs/CODING.md` — build commands, package layout, guardrails, config and launcher contracts
- `docs/TESTING.md` — test policy, fixtures, and required verification depth
- `docs/MONITORING.md` — centralized logging contract, capture points, and evidence guidance
- `docs/RUNTIME_UI_VERIFICATION.md` — runtime UI runbook for built-binary checks
- `docs/CHANGE-WORKFLOW.md` — beads-first change landing and session completion workflow
- `docs/RELEASING.md` — tag-triggered release process via GitHub Actions + GoReleaser
- `docs/user-guide/key-bindings.md` — default operator keybindings
- `project-plan/*.md` — deeper product/architecture/implementation intent kept as planning docs, not operator runbooks
