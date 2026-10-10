# Task Manager UI

[![CI](https://github.com/hk9890/task-manager-ui/actions/workflows/ci.yml/badge.svg)](https://github.com/hk9890/task-manager-ui/actions/workflows/ci.yml)

A standalone terminal UI for browsing and updating task-manager issues.

What it does:

- **Board** — issues in Not Ready, Ready, In Progress and Done columns, with the Done column paging
  in closed history as you scroll.
- **Docs tab** — every `doc`-type issue, open and closed, in one column.
- **Type to filter** — on the Board and Docs tabs every key you type narrows the rows to the
  titles and IDs that match, and marks the match.
- **Search** — `alt+f` searches the whole store as you type, widens to closed work, and drills
  into any result.
- **Detail** — the full issue with its dependencies, content and metadata panes.
- **Edit in place** — create, update, close and comment on issues; `alt+e` opens the full issue in
  `$EDITOR` and applies what you save.
- **Launch external tools** — run `nvim`, `opencode` or your own command against the selected issue.
- **Stores** — `alt+s` lists every central task store on the machine and switches to one in place;
  started somewhere with no store, it opens there instead of exiting and offers to create one.
- **Mouse** — click, wheel and drag-select do what the keys do. A plain drag belongs to the app;
  hold `shift` and drag for the terminal's own selection.
- **Themes and glyph sets** — four Catppuccin themes, and issue tokens as letters or as Nerd Font
  icons.
- **No daemon** — the store is opened in-process through the task-manager Go SDK.

## Getting Started

### Install

#### From a release (recommended)

Download a prebuilt archive from the
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

See [CONTRIBUTING.md](CONTRIBUTING.md#set-up).

### Run

No external CLI is required at runtime. `taskmgr-ui` reads and writes issues
in-process via the [task-manager](https://github.com/hk9890/task-manager) Go SDK
(`github.com/hk9890/task-manager/sdk/tasks`), pinned in `go.mod`.

```bash
taskmgr-ui                          # run against the current project's store
taskmgr-ui --cwd /path/to/project   # run against another project's store
taskmgr-ui --store-name acme        # run against a central store, by registry name
```

The store is resolved the way the `taskmgr` CLI resolves it: a local `.tasks/`
directory found by walking up, otherwise the central registry
(`taskmgr store list`). A project whose store was promoted with
`taskmgr store move --central` needs no flag. When nothing resolves, the app opens
on the store picker, where you can open a registered store or create one for the
directory.

The default theme, `catppuccin-mocha`, is dark and does not follow the terminal background. On a
light terminal put this in `~/.config/taskmgr-ui/config.yaml`:

```yaml
ui:
  theme: catppuccin-latte
```

With a [Nerd Font](https://www.nerdfonts.com/), `ui.glyphs: nerd` draws type, priority and status
as icons.

## CLI surface

`taskmgr-ui` is a TUI-first binary with a small startup CLI. `taskmgr-ui --help` is
the full flag list.

```bash
taskmgr-ui --print-config
taskmgr-ui --check-config      # validate the config file and exit
taskmgr-ui --debug             # mirror startup diagnostics to stderr
```

Configuring it — the config file, the theme and glyph set, keybinding overrides, launcher
templates — is
[`docs/CONFIGURATION.md`](./docs/CONFIGURATION.md).

## Docs

- [`docs/user-guide/key-bindings.md`](./docs/user-guide/key-bindings.md) — the default keybindings
- [`docs/CONFIGURATION.md`](./docs/CONFIGURATION.md) — the config file, the theme and glyph set, keybinding overrides, and launcher templates
- [`CHANGELOG.md`](./CHANGELOG.md) — release history

Developer and agent documentation lives under [`docs/`](./docs/); [`CONTRIBUTING.md`](./CONTRIBUTING.md)
is the entry point for humans and [`AGENTS.md`](./AGENTS.md) for AI tools.

## Security

See [`SECURITY.md`](./SECURITY.md) for how to report security issues.

## Verifying releases

Releases include a cosign-signed checksum file and per-archive SBOMs. See
[`docs/RELEASING.md`](./docs/RELEASING.md) for the verification commands.

## License

Licensed under the [Apache License 2.0](./LICENSE).
