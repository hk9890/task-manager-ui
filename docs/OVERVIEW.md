# Overview

The map of this repository: where things live and how to find them fast. Module
`github.com/hk9890/task-manager-ui`, binary `taskmgr-ui`, entrypoint `cmd/taskmgr-ui/main.go`.

## Repository layout

```
cmd/taskmgr-ui/           entrypoint: flag parsing, config resolution, logging setup, repository
                          backend selection. Resolves the store the app starts on; every later
                          store is opened through storecatalog/ from the picker
internal/
  app/                    the root shell: mode lifecycle, routing, selection and detail coordination
  mode/                   board, docs, search, detail, storepicker and configscreen feature
                          models, plus the shell message contracts and Query, the typed filter
                          text (query.go).
                          Type.IsWork() is false for doc, so doc issues reach no board column —
                          docs/ is the tab that browses them. search/ is the store search: a
                          browse surface that is not a tab. rowlist/ is the list code those
                          two share
  ui/                     rendering: a state struct in, a string out; reads no repository (DESIGN-GUIDE.md)
    styles/                 every colour role, the themes, the glyph sets, the key legend and the
                            shared FormSection chrome
    shared/                 issuerow, markdown, renderhelpers, textutil — reused across modes
    board/                  the columns and the query line above them; the docs tab and the
                            store search draw through it too
    detail/                 the issue detail panes
    storepicker/            the full-screen store list; not a tab, so it renders instead of the shell
    configscreen/           the full-screen configuration screen: the theme and the glyph set;
                            it renders instead of the shell too
    modal/ toaster/ overlay/ loading/ scroll/ fatalerror/   shared shell primitives
  domain/                 issue, query, mutation, catalog and error models
  repository/             the Repository interface, plus shared errors and types
    taskmgr/                production backend: in-process adapter over the SDK
    memory/                 test and --repo memory backend, over filestorage JSONL
    filestorage/            the JSONL fixture format and its loader; nothing here writes one
    nostore/                what the app holds while no store is open; every call fails as a
                            missing store
  storecatalog/           the port for the central store registry — which stores exist, against
                          repository/, which reads the issues inside one. taskmgr/ is the SDK
                          implementation over tasks.Stores
  dashboard/              Compose: dashboard.Inputs in, dashboard.Columns out
  config/                 config model, defaults, YAML loading, keybinding resolution, and Set,
                          the one writer of the config file (set.go)
  launcher/               external tool launch actions and the process runner; editor/ is the edit handoff
  logging/                the single logging entrypoint: session IDs, JSON Lines sink, stderr mirroring
  testing/                repository fakes, the UI test harness, and repofixture — the writer for
                          the `--repo-file` JSONL that filestorage only reads
  version/                build-time injected Version, Commit, Date
scripts/                  capture_taskmgr_ui_screen.py (PTY capture) and the git hooks
```

The rules that govern this layout are [CODING.md](CODING.md)'s Core Architectural Rules.

## Finding things

```bash
rg -n '^\t\w+Action\w+ +=' internal/config/keybindings.go   # every bindable action; DefaultKeyBindings has the keys
rg -n '^type \w+Msg\b' internal/                           # every Bubble Tea message type; the shell contracts are the exported ones in internal/mode/contracts.go
rg -n '^\t[A-Z]\w+\(' internal/repository/repository.go    # every repository operation
rg -n '^func Render' internal/ui/                          # every top-level renderer
rg -n '^func HitTest' internal/ui/                         # what each renderer draws at a cell; the mouse reads through these
rg -n '^\t\w+Color = ' internal/ui/styles/theme.go          # every colour role, and the colour each theme gives it
rg -n '^\t"[a-z-]+": +\{?' internal/ui/styles/glyphs.go internal/ui/styles/theme.go   # every glyph set and theme name
rg -n '<config-key>' internal/config/                      # where a config key is read
rg -n 'dep == "|MustCompile' cmd/taskmgr-ui/architecture_guardrails_test.go   # the import bans CI enforces
```

## External resources

| Resource | Where |
|---|---|
| Backing store and SDK | [`github.com/hk9890/task-manager`](https://github.com/hk9890/task-manager) — `sdk/tasks`, pinned in `go.mod`. File repository behavior surprises upstream before working around them in `internal/repository/taskmgr/`. |
| TUI framework | [Bubble Tea](https://pkg.go.dev/github.com/charmbracelet/bubbletea), with [Lip Gloss](https://pkg.go.dev/github.com/charmbracelet/lipgloss) for styling and [Glamour](https://pkg.go.dev/github.com/charmbracelet/glamour) for markdown |
| Git remote | https://github.com/hk9890/task-manager-ui |
