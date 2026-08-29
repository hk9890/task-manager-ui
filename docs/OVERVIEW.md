# Overview

The map of this repository: where things live and how to find them fast. Module
`github.com/hk9890/task-manager-ui`, binary `taskmgr-ui`, entrypoint `cmd/taskmgr-ui/main.go`.

## Repository layout

```
cmd/taskmgr-ui/           entrypoint: flag parsing, config resolution, logging setup, repository
                          backend selection. Calls tasks.Resolve, so nothing below cmd/ knows
                          where the store lives — the adapter receives an already-open *tasks.Store
internal/
  app/                    the root shell: mode lifecycle, routing, selection and detail coordination
  mode/                   board, docs, search and detail feature models, plus the shell message
                          contracts. Type.IsWork() is false for doc, so doc issues reach no board
                          column — docs/ is the tab that browses them
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
  dashboard/              Compose: dashboard.Inputs in, dashboard.Columns out
  config/                 config model, defaults, YAML loading, keybinding resolution
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
rg -n '^\t\w+Color +=' internal/ui/styles/colors.go        # every colour role
rg -n '<config-key>' internal/config/                      # where a config key is read
rg -n 'dep == "|MustCompile' cmd/taskmgr-ui/architecture_guardrails_test.go   # the import bans CI enforces
```

## External resources

| Resource | Where |
|---|---|
| Backing store and SDK | [`github.com/hk9890/task-manager`](https://github.com/hk9890/task-manager) — `sdk/tasks`, pinned in `go.mod`. File repository behavior surprises upstream before working around them in `internal/repository/taskmgr/`. |
| TUI framework | [Bubble Tea](https://pkg.go.dev/github.com/charmbracelet/bubbletea), with [Lip Gloss](https://pkg.go.dev/github.com/charmbracelet/lipgloss) for styling and [Glamour](https://pkg.go.dev/github.com/charmbracelet/glamour) for markdown |
| Git remote | https://github.com/hk9890/task-manager-ui |
