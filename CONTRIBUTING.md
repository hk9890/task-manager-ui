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
go install ./cmd/taskmgr-ui   # put the binary on your PATH
```

Run `mise tasks` to list every available task.

## Before you open a PR

```bash
mise run ci          # the merge gate
```

`mise run ci` is exactly what the linux CI job runs, so a green run locally is the signal that
predicts CI. Run `mise run quality:fast` (~15s) while you iterate — the ci subset without integration tests or
coverage.

## Propose a change

Open a GitHub issue first for a change spanning more than one `internal/` package, so the approach
can be agreed. Then:

1. Fork the repository, or branch off `main` if you have push access. The remote is `origin`.
2. Make the change. [docs/CODING.md](docs/CODING.md) is the implementation contract; run
   `mise run ci` before you push.
3. Commit with a Conventional Commits subject — `type(scope): imperative summary`.
4. `gh pr create`, and fill the [pull request template](.github/PULL_REQUEST_TEMPLATE.md). Put the
   GitHub issue number on its `Task:` line; the `bwb-` ids there are the maintainers' internal
   tracker and you are not expected to have one.

[docs/CHANGE-WORKFLOW.md](docs/CHANGE-WORKFLOW.md) carries the same flow for AI agents. Its
`EnterWorktree` tool, `commit-commands:*` skills and `taskmgr` store are the agent path only — you
need none of them.

## Where things live

[AGENTS.md](AGENTS.md) is the route list agents follow, and it is the authority on which doc owns
what. Read the one that matches what you are about to do:

| You are | Read |
|---|---|
| finding your way around the packages | [docs/OVERVIEW.md](docs/OVERVIEW.md) |
| changing code under `cmd/`, `internal/` or `scripts/` | [docs/CODING.md](docs/CODING.md) |
| changing a screen, a key, a glyph or a colour | [docs/DESIGN-GUIDE.md](docs/DESIGN-GUIDE.md) |
| adding a config key, a keybinding or a launcher template | [docs/CONFIGURATION.md](docs/CONFIGURATION.md) |
| writing a test, or running a gate | [docs/TESTING.md](docs/TESTING.md) |
| driving the built binary by hand | [docs/RUNNING.md](docs/RUNNING.md) |
| reading a log after a run went wrong | [docs/MONITORING.md](docs/MONITORING.md) |
| reviewing someone's diff | [docs/REVIEWING.md](docs/REVIEWING.md) |
| editing any tracked Markdown file | [docs/DOCUMENTING.md](docs/DOCUMENTING.md) |
| cutting a release (maintainers) | [docs/RELEASING.md](docs/RELEASING.md) |

[docs/user-guide/](docs/user-guide/) is for someone who runs `taskmgr-ui` and never opens this
repository.

## Reporting bugs and security issues

- For bugs, open a GitHub issue with reproduction steps.
- For security vulnerabilities, **do not** open a public issue — follow [SECURITY.md](SECURITY.md).

By contributing, you agree that your contributions will be licensed under the
[Apache License 2.0](LICENSE).
