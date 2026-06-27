# Contributing

Thanks for your interest in `taskmgr-ui`. This is a Go + Bubble Tea terminal UI
for the file-based `task-manager` issue tracker.

## Getting started

This repository uses [mise](https://mise.jdx.dev/) as the execution layer. It
provisions the pinned Go toolchain and dev tools from `.mise.toml`.

```bash
mise install          # provision the toolchain and dev tools
mise run build        # build the taskmgr-ui binary
mise run test:all     # run the test suite
mise run quality      # run the full pre-handoff quality gate (lint, vet, build, test)
```

Run `mise tasks` to list every available task.

## Development workflow

- Read [`docs/OVERVIEW.md`](./docs/OVERVIEW.md) for the runtime flow, package
  map, and architectural boundaries.
- Read [`docs/CODING.md`](./docs/CODING.md) for build commands and core
  architectural rules, and [`docs/TESTING.md`](./docs/TESTING.md) for test
  policy and verification depth.
- See [`docs/CHANGE-WORKFLOW.md`](./docs/CHANGE-WORKFLOW.md) for the change
  landing workflow and quality gates.

## Pull requests

1. Fork the repository and create a feature branch.
2. Make your change with tests where behavior changes.
3. Run `mise run quality` and ensure it passes.
4. Open a pull request describing the change and its motivation. The
   [pull request template](./.github/PULL_REQUEST_TEMPLATE.md) lists the
   expected checklist.

By contributing, you agree that your contributions will be licensed under the
[Apache License 2.0](./LICENSE).

## Reporting bugs and security issues

- For bugs, open a GitHub issue with reproduction steps.
- For security vulnerabilities, **do not** open a public issue — follow
  [`SECURITY.md`](./SECURITY.md).
