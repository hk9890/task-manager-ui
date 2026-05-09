# Beads Workbench

A standalone terminal UI for browsing and updating beads issues.

## Repository Identity

- Module: `github.com/hk9890/beads-workbench`
- Binary: `bwb`

## Getting Started

```bash
go build ./cmd/bwb
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

For exit codes, config details, and debug diagnostics, see
`docs/CODING.md` and `docs/MONITORING.md`.

## Developer Convenience Targets

This repository includes a small `Makefile` as a **thin wrapper** around
documented local commands for discoverability.

```bash
make help
make build
make test
make vet
```

Optional wrappers may also be available for lint/script validation and hook
installation.

See `docs/CHANGE-WORKFLOW.md` for the landing workflow and pre-handoff quality
gates, and `docs/CODING.md` for build/test details.

## Docs

- [`docs/OVERVIEW.md`](./docs/OVERVIEW.md) — runtime flow, package map, architecture boundaries
- [`docs/CODING.md`](./docs/CODING.md) — build commands, config model, implementation constraints
- [`docs/TESTING.md`](./docs/TESTING.md) — test policy, fixtures, and runtime verification expectations
- [`docs/MONITORING.md`](./docs/MONITORING.md) — current diagnostics/logging surface and evidence capture points
- [`docs/RUNTIME_UI_VERIFICATION.md`](./docs/RUNTIME_UI_VERIFICATION.md) — built-binary runtime UI verification runbook
- [`docs/CHANGE-WORKFLOW.md`](./docs/CHANGE-WORKFLOW.md) — beads-first change landing and session completion workflow
- [`docs/RELEASING.md`](./docs/RELEASING.md) — tag-triggered release workflow
- [`docs/user-guide/key-bindings.md`](./docs/user-guide/key-bindings.md) — default keybindings reference

For deeper design and planning context, see:

- [`project-plan/ARCHITECTURE.md`](./project-plan/ARCHITECTURE.md)
- [`project-plan/IMPLEMENTATION.md`](./project-plan/IMPLEMENTATION.md)
- [`project-plan/EXECUTION-PLAN.md`](./project-plan/EXECUTION-PLAN.md)
- [`CHANGELOG.md`](./CHANGELOG.md)

## Release visibility policy

This repository remains **private**. GitHub releases created here are
**internal-only** and are not intended to be publicly accessible unless a future
maintainer decision explicitly changes this policy.
