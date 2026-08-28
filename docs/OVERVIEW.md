# Overview

The map of this repository: where things live and how to find them fast. Module
`github.com/hk9890/task-manager-ui`, binary `taskmgr-ui`, entrypoint `cmd/taskmgr-ui/main.go`.

A Bubble Tea terminal UI over the task-manager Go SDK. There is no server, no database and no tracker
subprocess — the store is opened in-process and read directly.

## Repository layout

```
cmd/taskmgr-ui/           entrypoint: flags → non-interactive exits → config → logger → repository → tea.NewProgram
internal/
  app/                    the root shell: mode lifecycle, routing, selection and detail coordination
  mode/                   board, docs, search and detail feature models, plus the shell message contracts
  ui/                     rendering: a state struct in, a string out; reads no repository (DESIGN-GUIDE.md)
    styles/                 every colour and the shared FormSection chrome
    shared/                 issuerow, markdown, renderhelpers, textutil — reused across modes
    board/ search/ detail/  one renderer per browse surface
    modal/ toaster/ overlay/ loading/ scroll/ fatalerror/   shared shell primitives
  domain/                 issue, query, mutation, catalog and error models
  repository/             the Repository interface, plus shared errors and types
    taskmgr/                production backend: in-process adapter over the SDK
    memory/                 test and --repo memory backend, over filestorage JSONL
    filestorage/            JSONL load and save for the memory backend
  dashboard/              Compose: one DashboardData result into the fixed board columns
  config/                 config model, defaults, YAML loading, keybinding resolution
  launcher/               external tool launch actions and the process runner; editor/ is the edit handoff
  logging/                the single logging entrypoint: session IDs, JSON Lines sink, stderr mirroring
  testing/                repository fakes and the UI test harness
  version/                build-time injected Version, Commit, Date
scripts/                  capture_taskmgr_ui_screen.py (PTY capture) and the git hooks
```

## Key concepts

- **One repository abstraction.** Everything goes through `repository.Repository`. `buildRepository`
  in `cmd/taskmgr-ui/main.go` picks the backend; there is no caching layer and no validating
  decorator, because the in-process SDK is fast enough.
- **Store discovery is the SDK's.** `main.go` calls `tasks.Resolve`, so a local `.tasks` directory and
  a store promoted into the central registry resolve exactly as they do for the `taskmgr` CLI.
  Nothing below `cmd/` knows where the store lives — the adapter receives an already-open
  `*tasks.Store`.
- **The project root is the store's, not the working directory.** They differ whenever the app starts
  in a subdirectory or against a central store, and it is the root launcher templates interpolate.
- **Modes own state, the shell owns lifecycle.** `internal/mode/*` emits `SelectionChangedMsg` and
  `ActionRequestMsg`; `internal/app` decides what switches and what reloads. Synchronization is
  event-driven — there is no polling loop.
- **Three browse tabs, and a drill-in.** `mode.BrowseModes` is the header order — Board, Docs,
  Search. Detail is not a tab: it is entered from a browse tab and left with Escape, and the shell
  keeps `lastBrowse` on a tab so a selection lookup always resolves.
- **Docs are not work.** The SDK excludes `doc` issues from the ready and blocked queues, so an open
  doc reaches no board column. `internal/mode/docs` is where they are browsed — one column from
  `Repository.Search`, drawn by the board renderer.
- **Launchers are thin.** They resolve an action to one command template, start a subprocess and
  return. They never supervise, retry or orchestrate.
- **Editing is an editor handoff.** Rich issue editing opens `$EDITOR` on a marker-delimited document
  rather than building inline forms.

## Finding things

```bash
rg -n '^\t\w+Action\w+ +=' internal/config/keybindings.go   # every bindable action; DefaultKeyBindings has the keys
rg -n '^type \w+Msg\b' internal/                           # every Bubble Tea message the shell routes
rg -n '^\t[A-Z]\w+\(' internal/repository/repository.go    # every repository operation
rg -n '^func Render' internal/ui/                          # every top-level renderer
rg -n '^\t\w+Color +=' internal/ui/styles/colors.go        # every colour role
rg -n '<config-key>' internal/config/                      # where a config key is read
rg -n 'forbidden' cmd/taskmgr-ui/architecture_guardrails_test.go   # the import bans CI enforces
```

## External resources

| Resource | Where |
|---|---|
| Backing store and SDK | [`github.com/hk9890/task-manager`](https://github.com/hk9890/task-manager) — `sdk/tasks`, pinned in `go.mod`. File repository behavior surprises upstream before working around them in `internal/repository/taskmgr/`. |
| TUI framework | [Bubble Tea](https://pkg.go.dev/github.com/charmbracelet/bubbletea), with [Lip Gloss](https://pkg.go.dev/github.com/charmbracelet/lipgloss) for styling and [Glamour](https://pkg.go.dev/github.com/charmbracelet/glamour) for markdown |
| Git remote | https://github.com/hk9890/task-manager-ui |
