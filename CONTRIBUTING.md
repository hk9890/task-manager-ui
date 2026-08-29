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

[AGENTS.md](AGENTS.md) is the full route list — which doc owns what. The three you are most likely
to need are [docs/CODING.md](docs/CODING.md) for a code change,
[docs/TESTING.md](docs/TESTING.md) for the tiers behind `mise run ci`, and
[docs/RUNNING.md](docs/RUNNING.md) for driving the built binary.

## Reporting bugs and security issues

- For bugs, open a GitHub issue with reproduction steps.
- For security vulnerabilities, **do not** open a public issue — follow [SECURITY.md](SECURITY.md).

By contributing, you agree that your contributions will be licensed under the
[Apache License 2.0](LICENSE).
