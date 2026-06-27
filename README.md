# Task Manager UI

[![CI](https://github.com/hk9890/task-manager-ui/actions/workflows/ci.yml/badge.svg)](https://github.com/hk9890/task-manager-ui/actions/workflows/ci.yml)

A standalone terminal UI for browsing and updating task-manager issues.

## Repository Identity

- Module: `github.com/hk9890/task-manager-ui`
- Binary: `taskmgr-ui`

## Getting Started

### Install

#### From a release (recommended)

Download a prebuilt, signed archive from the
[releases page](https://github.com/hk9890/task-manager-ui/releases) and put the
`taskmgr-ui` binary on your `PATH`. Archives are named
`taskmgr-ui_<version>_<os>_<arch>.tar.gz` (for example
`taskmgr-ui_<version>_linux_x64.tar.gz` or
`taskmgr-ui_<version>_macos_arm64.tar.gz`):

```bash
tar -xzf taskmgr-ui_<version>_linux_x64.tar.gz
mv taskmgr-ui ~/.local/bin/        # or anywhere on your PATH
```

If you use [mise](https://mise.jdx.dev/), the release asset names let it fetch
and pin a release for you:

```bash
mise use -g ubi:hk9890/task-manager-ui
```

Release archives ship with a cosign-signed checksum file — see
[Verifying releases](#verifying-releases) to verify a download.

#### From source

Building from source uses [mise](https://mise.jdx.dev/), which provisions the
pinned Go toolchain and dev tools from `.mise.toml`, so a separate Go install is
not required:

```bash
mise run build                    # build the taskmgr-ui binary at ./taskmgr-ui
go install ./cmd/taskmgr-ui       # or install it onto your PATH
```

### Run

No external CLI is required at runtime. `taskmgr-ui` reads and writes issues
in-process via the [task-manager](https://github.com/hk9890/task-manager) Go SDK
(`github.com/hk9890/task-manager/sdk/tasks`), pinned in `go.mod`.

```bash
taskmgr-ui --cwd /path/to/project   # run against a directory containing a .tasks/ store
```

## CLI Surface

`taskmgr-ui` is intentionally a **TUI-first** binary with a small startup CLI.

For the full flag list, exit codes, config-path behavior, and debug contract, see
`docs/CODING.md`.

Common examples:

```bash
taskmgr-ui --cwd /path/to/project
taskmgr-ui --config "$HOME/.config/taskmgr-ui/config.yaml" --print-config
taskmgr-ui --check-config
```

For exit codes, config details, and centralized debug/logging behavior, see
`docs/CODING.md` and `docs/MONITORING.md`.

## Developer Tasks

Building, testing, and the pre-handoff quality gates are documented in
[`CONTRIBUTING.md`](./CONTRIBUTING.md). Run `mise tasks` to list every available
task; see `docs/CHANGE-WORKFLOW.md` for the landing workflow and `docs/CODING.md`
for build/test details.

## Docs

- [`docs/OVERVIEW.md`](./docs/OVERVIEW.md) — runtime flow, package map, architecture boundaries
- [`docs/CODING.md`](./docs/CODING.md) — build commands, architectural rules, quality gates
- [`docs/CONFIGURATION.md`](./docs/CONFIGURATION.md) — runtime config model, keybindings, launcher reference
- [`docs/TESTING.md`](./docs/TESTING.md) — test policy, fixtures, and runtime verification expectations
- [`docs/MONITORING.md`](./docs/MONITORING.md) — centralized logging contract and evidence capture points
- [`docs/RUNTIME_UI_VERIFICATION.md`](./docs/RUNTIME_UI_VERIFICATION.md) — built-binary runtime UI verification runbook
- [`docs/CHANGE-WORKFLOW.md`](./docs/CHANGE-WORKFLOW.md) — task-manager-first change landing and session completion workflow
- [`docs/RELEASING.md`](./docs/RELEASING.md) — manually dispatched release workflow
- [`docs/user-guide/key-bindings.md`](./docs/user-guide/key-bindings.md) — default keybindings reference

See also [`CHANGELOG.md`](./CHANGELOG.md) for the release history.

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for how to propose changes and the
contribution workflow.

## Security

See [`SECURITY.md`](./SECURITY.md) for how to report security issues.

## Verifying releases

Releases include a cosign-signed checksum file and per-archive SBOMs. See
[`docs/RELEASING.md`](./docs/RELEASING.md) for the verification commands.

## License

Licensed under the [Apache License 2.0](./LICENSE).
