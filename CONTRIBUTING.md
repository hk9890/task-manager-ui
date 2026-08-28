# Contributing

The human contributor's front door to `taskmgr-ui`, a Go and Bubble Tea terminal UI for the
file-based `task-manager` issue tracker. (AI agents start from [AGENTS.md](AGENTS.md) instead.)

## Set up

This repository uses [mise](https://mise.jdx.dev/) as the execution layer. It provisions the pinned
Go toolchain and dev tools from `.mise.toml`, so a separate Go install is not needed.

```bash
mise install         # provision the toolchain and dev tools
mise run build       # build the taskmgr-ui binary
mise run taskmgr-ui  # build and run it against this project's own store
mise run hooks:install   # once per clone: the pre-commit hook that formats staged Go files
```

Run `mise tasks` to list every available task.

## Before you open a PR

```bash
mise run ci          # the merge gate: scripts, formatting, lint, build, vet, guardrails, tests, coverage
```

`mise run ci` is exactly what the linux CI job runs, so a green run locally is the signal that
predicts CI. `mise run quality:fast` (~15s) is the pre-commit subset while you iterate.

## Propose a change

Follow [docs/CHANGE-WORKFLOW.md](docs/CHANGE-WORKFLOW.md) to branch, commit and open the PR. The
[pull request template](.github/PULL_REQUEST_TEMPLATE.md) lists the expected checklist. Open an issue
first for a change spanning more than one `internal/` package, so the approach can be agreed.

## Where things live

[AGENTS.md](AGENTS.md) is the authoritative route list; this table mirrors it for humans, in the same
order.

| Topic | Doc |
|---|---|
| Architecture, layout, the store boundary — and finding anything | [docs/OVERVIEW.md](docs/OVERVIEW.md) |
| Coding and file changes — `cmd/`, `internal/`, `scripts/` | [docs/CODING.md](docs/CODING.md) |
| Screens, keys, colours, glyphs | [docs/DESIGN-GUIDE.md](docs/DESIGN-GUIDE.md) |
| Runtime config, keybindings, launcher templates | [docs/CONFIGURATION.md](docs/CONFIGURATION.md) |
| Test tiers, fixtures, goldens, the gates | [docs/TESTING.md](docs/TESTING.md) |
| Running the app by hand and capturing the screen | [docs/RUNNING.md](docs/RUNNING.md) |
| Reading the log and diagnosing a failed run | [docs/MONITORING.md](docs/MONITORING.md) |
| Reviewing a PR or a diff | [docs/REVIEWING.md](docs/REVIEWING.md) |
| Writing documentation | [docs/DOCUMENTING.md](docs/DOCUMENTING.md) |
| Commit / branch / worktree / PR / merge | [docs/CHANGE-WORKFLOW.md](docs/CHANGE-WORKFLOW.md) |
| Cutting a release | [docs/RELEASING.md](docs/RELEASING.md) |
| Using `taskmgr-ui` | [README.md](README.md) → [docs/user-guide/](docs/user-guide/) |

## Reporting bugs and security issues

- For bugs, open a GitHub issue with reproduction steps.
- For security vulnerabilities, **do not** open a public issue — follow [SECURITY.md](SECURITY.md).

By contributing, you agree that your contributions will be licensed under the
[Apache License 2.0](LICENSE).
