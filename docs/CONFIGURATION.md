# Configuration

Runtime configuration, keybindings, and launcher reference for Task Manager UI.
For the architectural rules these settings operate under, see `docs/CODING.md`.

## Runtime configuration (v1)

Configuration lives in `internal/config` and is loaded once at startup via
`config.LoadWithOptions(...)` (the startup path used by `cmd/taskmgr-ui/main.go`;
`config.Load()` is the simpler no-options variant).

Runtime config path resolution uses `os.UserConfigDir()` and looks for:

- `taskmgr-ui/config.yaml`
- on Linux this is typically `~/.config/taskmgr-ui/config.yaml`

Load semantics:

- Missing config file is allowed; taskmgr-ui starts with defaults.
- Explicit config file values override environment-driven defaults.
- Unknown YAML keys are ignored and surfaced as startup warnings.
- Invalid YAML, unreadable existing config files, invalid values, and duplicate
  launcher actions fail startup.

The v1 model is intentionally small and only covers app-shell concerns:

- `Editor.Command`
  - Defaults to `$EDITOR` when set.
  - Falls back to `vi` when `$EDITOR` is unset/empty.
  - `editor.command` in `config.yaml` overrides both.
- `Launcher.Definitions`
  - Defaults to four built-in launcher actions:
    - `editor` → launches the resolved editor command (`editor.command`, else `$EDITOR`, else `vi`).
    - `nvim` → launches `nvim` with a read-only issue context buffer seeded from
      interpolation placeholders.
    - `opencode` → launches `opencode run` with issue metadata args/env.
    - `shell-command` → launches `sh -lc` with a simple formatted issue-context
      print command.
  - Each definition supports:
    - `Action` (required unique action key)
    - `Command` (required executable/template string)
    - `Args` (optional argv templates)
    - `Env` (optional `KEY=value` templates)
    - `WorkDir` (optional working-directory template; defaults to project root)
  - YAML launcher overrides merge by `Action`:
    - matching built-ins are replaced field-by-field from the provided override
    - new action names are appended
    - unspecified built-ins remain available
    - `Args` and `Env` follow nil-sentinel semantics: omitting the key in YAML
      leaves the field nil in the override struct, so the built-in value is
      preserved; writing `args: []` produces a non-nil empty slice that
      **replaces** the built-in args (use this to explicitly clear defaults)
- `UI.ShowModeSwitcherHelp`
  - Defaults to `true`.
  - Controls whether the shell renders the mode hotkey hint line.

Example config:

```yaml
editor:
  command: nvim

launcher:
  definitions:
    - action: opencode
      command: opencode-dev
      args:
        - run
        - --issue
        - "{{issue.id}}"
    - action: tmux-note
      command: tmux
      args:
        - new-window
        - "issue {{issue.id}}"

ui:
  show_mode_switcher_help: false

keybindings:
  shell:
    quit: [ctrl+q]
    toggle_help: [F1]
  board:
    move_left: [left]
    move_right: [right]
  search:
    cycle_focus_next: [ctrl+n]
    cycle_focus_prev: [ctrl+p]
    open_detail: [space]
  detail:
    scroll_down: [ctrl+d]
    scroll_up: [ctrl+u]
  modal:
    enter: [space]
    escape: [q]
```

## Keybindings (v1)

Keybindings are resolved once at startup from the `keybindings` section.

- Supported contexts: `shell`, `board`, `search`, `detail`, `modal`
- Overrides merge per action; you only need to specify actions you want to change
- Unknown actions, invalid key names, missing required actions, and conflicting keys
  in the same context fail startup
- Search query typing still accepts normal text input; only configured search actions
  intercept keys in search mode

Supported actions by context:

- `shell`
  - `quit`, `toggle_help`, `mode_board`, `mode_search`, `toggle_search`,
    `mode_detail`, `mode_cycle_next`, `mode_cycle_prev`, `escape`,
    `reload_detail`, `edit_issue`, `create_issue`, `update_issue`,
    `close_issue`, `comment_issue`, `launch_nvim`, `launch_opencode`,
    `launch_shell_command`
- `board`
  - `move_left`, `move_right`, `move_up`, `move_down`, `open_detail`, `reload`
- `search`
  - `move_up`, `move_down`, `focus_left`, `focus_right`, `focus_query`,
    `reload`, `open_detail`, `cycle_focus_next`, `cycle_focus_prev`
  - Note: Enter has a built-in submit-query role when the query field is
    focused (submits the draft and runs the search), independent of the
    configurable `open_detail` action. This mirrors `backspace` and `ctrl+u`,
    which are also built-in query-editing keys not part of the configurable
    search keymap.
- `detail`
  - `scroll_up`, `scroll_down`, `page_up`, `page_down`, `home`, `end`
- `modal`
  - `next`, `prev`, `left`, `right`, `enter`, `escape`

For the default operator keybindings (as shipped), see
`docs/user-guide/key-bindings.md`.

## Launcher interpolation/context surface

Launcher templates support these placeholders across `Command`, every `Args`
entry, every `Env` entry, and `WorkDir`:

- `{{issue.id}}`
- `{{issue.title}}`
- `{{issue.labels}}` (comma-joined label list)
- `{{issue.assignee}}`
- `{{project.root}}`

Notes:

- Unsupported placeholders are passed through literally.
- Empty issue fields interpolate as empty strings.
- `WorkDir` falls back to project root when blank.

The shell-launcher security rule (do not interpolate issue fields into a
`sh -c`/`sh -lc` body) is an architectural rule — see `docs/CODING.md` Core
Architectural Rules.

## Shell editor/launcher UX behavior (v1)

- `e` opens the rich marker-based issue edit document flow for the currently
  selected issue via `services.Editor`. The editor launch is routed through
  `tea.Exec`, which suspends the TUI and fully restores it after the editor
  exits — eliminating any TTY contention between the editor and Bubble Tea.
- `n`, `p`, `l` trigger `nvim`, `opencode`, `shell-command` launchers from
  detail mode.
- If no issue is selected, the shell shows a warning toast and does not launch.
- Successful rich editor updates trigger detail reload; launchers remain
  non-blocking and do not auto-refresh issue detail.
- Launcher actions are explicitly **background fire-and-forget** in v1 (no
  managed terminal handoff/return contract). After launching, the app stays
  active and shows guidance toast text; use `e` for edit/save flows that round
  trip back into app state with detail reload.

The rich marker-based edit document flow in `internal/launcher/editor` is the
actual interactive shell edit path. Launcher definitions remain a separate
external-tool surface for non-edit actions.

## Testing references for editor/launcher behavior

- Config defaults and built-ins: `internal/config/model_test.go`
- Interpolation/runner behavior: `internal/launcher/service_test.go`
- Shell key wiring and launcher actions: `internal/app/model_test.go`
- Editor round-trip service seam: `internal/launcher/editor/service_test.go`
- Embedded fixture smoke coverage for edit flow: `internal/app/model_test.go`

For broader policy and full-app verification expectations, see `docs/TESTING.md`.

## Design intent

- Keep config loading and access behind a clear boundary (`internal/config` +
  `app.Services.Config`).
- Avoid introducing legacy-style broad config surfaces (custom views,
  orchestration settings, etc.).
