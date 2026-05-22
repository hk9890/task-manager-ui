# Beads Workbench

A standalone terminal UI for browsing and updating beads issues.

## Repository Identity

- Module: `github.com/hk9890/beads-workbench`
- Binary: `bwb`

## Getting Started

### Prerequisites

- [mise](https://mise.jdx.dev/) — provisions the pinned Go toolchain and dev
  tools from `.mise.toml`, so a separate Go install is not required.
- `bd` (the beads issue tracker CLI) — required at runtime; `bwb` is a thin TUI
  over `bd`.

### Build and run

```bash
mise run build                    # build the bwb binary
bwb --cwd /path/to/beads-project  # run against a directory containing a .beads/ repo
```

## CLI Surface

`bwb` is intentionally a **TUI-first** binary with a small startup CLI.

For the full flag list, exit codes, config-path behavior, and debug contract, see
`docs/CODING.md`.

Common examples:

```bash
bwb --cwd /path/to/beads-project
bwb --config "$HOME/.config/bwb/config.yaml" --print-config
bwb --check-config
```

For exit codes, config details, and centralized debug/logging behavior, see
`docs/CODING.md` and `docs/MONITORING.md`.

## Developer Tasks

This repository uses [mise](https://mise.jdx.dev/) as the execution layer.
Run `mise tasks` to list all available tasks.

```bash
mise run build
mise run test:all
mise run vet
mise run quality
```

See `docs/CHANGE-WORKFLOW.md` for the landing workflow and pre-handoff quality
gates, and `docs/CODING.md` for build/test details.

## Docs

- [`docs/OVERVIEW.md`](./docs/OVERVIEW.md) — runtime flow, package map, architecture boundaries
- [`docs/CODING.md`](./docs/CODING.md) — build commands, architectural rules, quality gates
- [`docs/CONFIGURATION.md`](./docs/CONFIGURATION.md) — runtime config model, keybindings, launcher reference
- [`docs/TESTING.md`](./docs/TESTING.md) — test policy, fixtures, and runtime verification expectations
- [`docs/MONITORING.md`](./docs/MONITORING.md) — centralized logging contract and evidence capture points
- [`docs/RUNTIME_UI_VERIFICATION.md`](./docs/RUNTIME_UI_VERIFICATION.md) — built-binary runtime UI verification runbook
- [`docs/CHANGE-WORKFLOW.md`](./docs/CHANGE-WORKFLOW.md) — beads-first change landing and session completion workflow
- [`docs/RELEASING.md`](./docs/RELEASING.md) — tag-triggered release workflow
- [`docs/user-guide/key-bindings.md`](./docs/user-guide/key-bindings.md) — default keybindings reference

For deeper design and planning context, see:

- [`project-plan/PRODUCT.md`](./project-plan/PRODUCT.md)
- [`project-plan/ARCHITECTURE.md`](./project-plan/ARCHITECTURE.md)
- [`project-plan/IMPLEMENTATION.md`](./project-plan/IMPLEMENTATION.md)
- [`project-plan/EXECUTION-PLAN.md`](./project-plan/EXECUTION-PLAN.md)
- [`CHANGELOG.md`](./CHANGELOG.md)

## Verifying releases

Releases include a cosign-signed checksum file and per-archive SBOMs. See
[`docs/RELEASING.md`](./docs/RELEASING.md) for the verification commands.

## Release visibility policy

This repository remains **private**. GitHub releases created here are
**internal-only** and are not intended to be publicly accessible unless a future
maintainer decision explicitly changes this policy.
