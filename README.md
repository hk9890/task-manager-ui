# Beads Workbench

A standalone terminal UI for browsing and updating beads issues.

## Repository Identity

- Module: `github.com/hk9890/beads-workbench`
- Binary: `bwb`
- Primary planning docs: [`project-plan/`](./project-plan/)

## Getting Started

```bash
go build ./cmd/bwb
```

## Docs

- [`docs/OVERVIEW.md`](./docs/OVERVIEW.md) — runtime flow, package map, architecture boundaries
- [`docs/CODING.md`](./docs/CODING.md) — build commands, config model, implementation constraints
- [`docs/TESTING.md`](./docs/TESTING.md) — test policy, fixtures, and runtime verification expectations
- [`docs/RUNTIME_UI_VERIFICATION.md`](./docs/RUNTIME_UI_VERIFICATION.md) — built-binary runtime UI verification runbook
- [`docs/CHANGE-WORKFLOW.md`](./docs/CHANGE-WORKFLOW.md) — beads-first change landing and session completion workflow
- [`docs/RELEASING.md`](./docs/RELEASING.md) — tag-triggered release workflow
- [`docs/user-guide/key-bindings.md`](./docs/user-guide/key-bindings.md) — default keybindings reference

## Configuration

BWB optionally loads runtime config from:

- `~/.config/bwb/config.yaml` on typical Linux setups

It uses `os.UserConfigDir()` internally, so the exact base config directory is
platform-aware.

Highlights:

- missing config file is fine; defaults are used
- config file values override env-based defaults like `$EDITOR`
- unknown YAML keys are ignored with warnings
- invalid YAML or unreadable config files fail startup
- shell, board, search, detail, and modal keybindings are configurable

See [`docs/CODING.md`](./docs/CODING.md) for the current config schema and
examples.

For deeper design and planning context, see:

- [`project-plan/ARCHITECTURE.md`](./project-plan/ARCHITECTURE.md)
- [`project-plan/IMPLEMENTATION.md`](./project-plan/IMPLEMENTATION.md)
- [`project-plan/EXECUTION-PLAN.md`](./project-plan/EXECUTION-PLAN.md)
- [`CHANGELOG.md`](./CHANGELOG.md)

## Release visibility policy

This repository remains **private**. GitHub releases created here are
**internal-only** and are not intended to be publicly accessible unless a future
maintainer decision explicitly changes this policy.
