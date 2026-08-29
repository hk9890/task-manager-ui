# Configuration

The runtime config model, keybinding resolution, and the launcher interpolation
surface — what a `config.yaml` may say and how it is resolved.

## Runtime configuration

Configuration lives in `internal/config` and is loaded once at startup via
`config.LoadWithOptions(...)` (the startup path used by `cmd/taskmgr-ui/main.go`;
`config.Load()` is the simpler no-options variant).

Config path resolution, in order:

- `--config <path>` when given — the file must exist, or startup fails with
  `config path %q does not exist`.
- Otherwise `<os.UserConfigDir()>/taskmgr-ui/config.yaml`, which may be absent. On Linux
  this is typically `~/.config/taskmgr-ui/config.yaml`.

Load semantics:

- A missing config file at the **default** path is allowed; taskmgr-ui starts with defaults.
- Explicit config file values override environment-driven defaults.
- Unknown YAML keys are ignored and surfaced as startup warnings.
- Invalid YAML, unreadable existing config files, invalid values, and duplicate
  launcher actions fail startup.

The model is intentionally small and only covers app-shell concerns:

- `Editor.Command`
  - Defaults to `$EDITOR` when set.
  - Falls back to `vi` when `$EDITOR` is unset/empty.
  - `editor.command` in `config.yaml` overrides both.
  - The value is split on whitespace with single/double-quote grouping and no backslash
    escaping, so `code --wait` works; the edit document path is appended as the last
    argument. An unclosed quote is an error.
- `Launcher.Definitions`
  - Defaults to four built-in launcher actions:
    - `editor` → mirrors `editor.command`.
    - `nvim` → launches `nvim` with a read-only issue context buffer seeded from
      interpolation placeholders.
    - `opencode` → launches `opencode run` with issue metadata args/env.
    - `shell-command` → launches `sh -lc` with a simple formatted issue-context
      print command.
  - **`editor` is not launched as a definition.** It exists so `--check-config` does not
    warn that it is unlaunchable, and `syncEditorLauncher` keeps it in step with
    `editor.command`. The edit key runs `editor.command` directly through
    `internal/launcher/editor`. Change the editor with `editor.command`, never with a
    `launcher.definitions` override.
  - Each definition supports these YAML keys:
    - `action` (required unique action key)
    - `command` (required executable/template string)
    - `args` (optional argv templates)
    - `env` (optional `KEY=value` templates)
    - `workdir` (optional working-directory template; defaults to project root)
  - YAML launcher overrides merge by `action`:
    - matching built-ins are replaced field-by-field from the provided override
    - new action names are appended
    - unspecified built-ins remain available
    - `args` and `env` follow nil-sentinel semantics: omitting the key in YAML
      leaves the field nil in the override struct, so the built-in value is
      preserved; writing `args: []` produces a non-nil empty slice that
      **replaces** the built-in args (use this to explicitly clear defaults)
  - Only the four built-in action names can be launched — there is no keybinding
    action for any other. Startup warns by name for a definition nothing can
    start; override a built-in instead of appending a new action.
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
    - action: shell-command
      command: sh
      args:
        - -lc
        - 'printf "issue=%s\n" "$0"'
        - "{{issue.id}}"

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

## Keybindings

Keybindings are resolved once at startup from the `keybindings` section.

- Supported contexts: `shell`, `board`, `search`, `detail`, `modal`
- Overrides merge per action; you only need to specify actions you want to change
- Unknown actions are dropped with a startup warning, not a failure. Invalid key names,
  an empty key list for an action, and two actions bound to the same key in one context
  fail startup.

Supported actions by context:

- `shell`
  - `quit`, `toggle_help`, `mode_board`, `mode_docs`, `mode_search`,
    `toggle_search`, `mode_detail`, `mode_cycle_next`, `mode_cycle_prev`, `escape`,
    `reload_detail`, `edit_issue`, `create_issue`, `update_issue`,
    `close_issue`, `comment_issue`, `launch_nvim`, `launch_opencode`,
    `launch_shell_command`
- `board`
  - `move_left`, `move_right`, `move_up`, `move_down`, `open_detail`, `reload`, `load_more`
  - Docs mode has no context of its own: it reads `move_up`, `move_down`,
    `open_detail`, and `reload` from this one. Rebinding them moves both
    surfaces together, which is deliberate — the docs tab is a board column.
- `search`
  - `move_up`, `move_down`, `focus_left`, `focus_right`, `focus_query`,
    `reload`, `open_detail`, `cycle_focus_next`, `cycle_focus_prev`
  - `backspace`, `ctrl+u` and `ctrl+t` are built in and cannot be rebound; `ctrl+t`
    toggles the search scope between open work and all issues. A printable key could not
    replace any of them — while the query box has focus, every printable rune is typed
    into the query.
  - Enter has a built-in submit-query role when the query field is focused (it submits
    the draft and runs the search), independent of the configurable `open_detail` action.
- `detail`
  - `scroll_up`, `scroll_down`, `page_up`, `page_down`, `home`, `end`
- `modal`
  - `next`, `prev`, `left`, `right`, `enter`, `escape`

For the default operator keybindings (as shipped), see
[user-guide/key-bindings.md](user-guide/key-bindings.md).

## Launcher interpolation/context surface

Launcher templates support these placeholders across `command`, every `args`
entry, every `env` entry, and `workdir`:

- `{{issue.id}}`
- `{{issue.title}}`
- `{{issue.labels}}` (comma-joined label list)
- `{{issue.assignee}}`
- `{{project.root}}`

Notes:

- Unsupported placeholders are passed through literally.
- Empty issue fields interpolate as empty strings.
- `workdir` falls back to project root when blank.
- `{{project.root}}` is the resolved store's project path, not the directory
  `taskmgr-ui` was started in: the root of the project holding the local `.tasks`
  store, or the project path registered for a central store. See
  [CODING.md → Store resolution](CODING.md#store-resolution).

### Writing a launcher template safely

Issue fields are operator-untrusted input. [CODING.md](CODING.md)'s Shell-launcher
security rule states the invariant; this is how a config author satisfies it.

Two shapes re-parse an argument as a command line, and neither may carry an interpolated
issue field:

- the body after a `-c` / `-lc`-style flag (`sh`, `bash`, `su`, `python`, …)
- a plain argument handed to a command that dispatches it to a shell (`tmux new-window`,
  `ssh host`, `watch`)

For the first, pass the placeholders as positional arguments after the body and reference
them as `$0`, `$1`, `$2` inside the script. For the second, pass the field through `env`,
which is not re-parsed.

```yaml
# SAFE — issue fields are positional args, never re-parsed as code
command: sh
args:
  - "-lc"
  - "printf 'id=%s title=%s\n' \"$0\" \"$1\""
  - "{{issue.id}}"
  - "{{issue.title}}"

# UNSAFE — do not do this
args:
  - "-lc"
  - "printf 'id=%s title=%s\n' \"{{issue.id}}\" \"{{issue.title}}\""
```

`taskmgr-ui --check-config` rejects a definition that breaks the rule, and so does every
normal start.
