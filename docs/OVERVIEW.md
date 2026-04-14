# Overview

## Project identity

- Module: `github.com/hk9890/beads-workbench`
- Binary: `bwb`
- Entrypoint: `cmd/bwb/main.go`
- Primary runtime surfaces: Bubble Tea UI + `bd` CLI-backed gateway

## Runtime flow

1. `cmd/bwb/main.go` loads runtime config with `internal/config.Load()`.
2. It creates the source-specific beads gateway with `internal/gateway/beads.NewCLIGateway(...)`.
3. It builds shell services with `internal/app.NewServices(...)`.
4. It starts the TUI with `tea.NewProgram(..., tea.WithAltScreen())`.

## Package map

| Path | Responsibility |
| --- | --- |
| `cmd/bwb` | Binary entrypoint and program bootstrap |
| `internal/app` | Root shell, mode lifecycle, selection/detail coordination |
| `internal/config` | Runtime config model, defaults, YAML loading, keybinding resolution |
| `internal/domain` | Issue, query, mutation, catalog, and error models |
| `internal/gateway/beads` | Official `bd` CLI adapter and typed payload decoding |
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
- `docs/RUNTIME_UI_VERIFICATION.md` — runtime UI runbook for built-binary checks
- `docs/CHANGE-WORKFLOW.md` — beads-first change landing and session completion workflow
- `docs/RELEASING.md` — tag-triggered release process via GitHub Actions + GoReleaser
- `docs/user-guide/key-bindings.md` — default operator keybindings
- `project-plan/*.md` — deeper product/architecture/implementation intent kept as planning docs, not operator runbooks
