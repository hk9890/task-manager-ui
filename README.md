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

## CLI Surface

`bwb` is intentionally a **TUI-first** binary with a small startup CLI.

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
`--check-config`) exit before Bubble Tea starts.

### Common CLI examples

```bash
# use an explicit config file (relative paths resolve from process start cwd)
bwb --config ./configs/dev.yaml

# run against a specific project root
bwb --cwd /path/to/beads-project

# inspect the resolved config and source path, then exit
bwb --config ./configs/dev.yaml --print-config

# validate config and print warnings to stderr, then exit
bwb --check-config
```

### Version behavior

- Local developer builds default to `bwb dev`.
- Release/snapshot builds set `main.version` at link time from GoReleaser
  (`.goreleaser.yaml` uses `-X main.version={{ .Version }}`).

### Non-interactive exit codes

| Path | Exit code |
| --- | --- |
| `--help` / `--version` success | `0` |
| `--print-config` / `--check-config` success | `0` |
| Runtime/config failure on those paths (load/parse/encode errors) | `1` |
| CLI usage error (unknown flag, unexpected positional args) | `2` |

### Debug output

`--debug` emits diagnostic lines to stderr with the prefix:

```text
[bwb-debug]
```

Current debug event categories:

- startup resolution (`resolved config path`, `resolved cwd`, `auto-refresh`)
- `bd` command execution traces (`bd argv=... exit_code=...`)

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

`docs/CODING.md` remains the authoritative source for the pre-handoff quality
gate sequence.

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
