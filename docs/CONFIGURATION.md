# Configuration

The runtime config model, keybinding resolution, and the launcher interpolation
surface — what a `config.yaml` may say and how it is resolved. Two different
files are in play: taskmgr-ui's own config, and the store's, which the SDK reads
and which can refuse the app's writes.

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
    - `nvim` → launches `nvim` with a read-only issue context buffer. The issue fields
      reach it through `env` as `TASKMGR_UI_ISSUE_*` and the Ex commands read them back
      as `$VAR`; a `+cmd` argument is an Ex command line, so a placeholder there would be
      executable text.
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
  - Controls whether the shell renders the key legend on its last line.
- `UI.Theme`
  - Defaults to `catppuccin-mocha`. The others are `catppuccin-macchiato`, `catppuccin-frappe`
    and `catppuccin-latte`, the one light theme.
  - The theme does not follow the terminal background: set `catppuccin-latte` on a light terminal.
- `UI.Glyphs`
  - Defaults to `unicode`, which spells an issue's type, priority and status as letters.
  - `nerd` draws the three as icons and needs a [Nerd Font](https://www.nerdfonts.com/); without
    one they show as boxes.
  - `ascii` is for a terminal whose font is not yours.
- An unknown theme or glyph set fails startup, and `--check-config`, with the valid names.
  `styles.Validate` (`internal/ui/styles/theme.go`) owns both lists.

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
  theme: catppuccin-latte
  glyphs: unicode

keybindings:
  shell:
    quit: [ctrl+q]
    toggle_help: [f1]
    open_search: [ctrl+f]
  board:
    move_up: [ctrl+p]
    move_down: [ctrl+n]
  detail:
    scroll_down: [ctrl+d]
    scroll_up: [ctrl+b]
  modal:
    enter: [space]
    escape: [q]
```

## The store's own config

The resolved store's `.tasks/config.yaml` and the per-user `~/.taskmgr/config.yaml`
belong to task-manager, not to taskmgr-ui. The SDK reads both on every command, so
a fault in either reaches the operator as a refused write in the UI.

### The withdrawn `hooks:` key

SDK v0.9.0 withdrew `hooks:`. A config no longer declares hooks inline; it lists
under `use:` the packages it takes them from.

A file still carrying a `hooks:` block is recorded as a config **defect**, and a
defect fails every mutation while leaving reads intact. The board, detail and
search render normally and every write is refused — the app looks healthy and is
read-only. Closing an issue reports:

```
close issue failed: close issue: unknown: <store>/.tasks/config.yaml: the `hooks:` key was withdrawn: …
```

Run `taskmgr package list` for the untruncated text: a defect prints as a broken
row naming the key and the file it is in. The toast clips at terminal width.

Migrate by moving each hook entry into a package directory, adding it with
`taskmgr package add`, then deleting the `hooks:` block from the file.

## Keybindings

Keybindings are resolved once at startup from the `keybindings` section.

- Supported contexts: `shell`, `board`, `detail`, `modal`
- Overrides merge per action; you only need to specify actions you want to change
- Unknown actions and unknown contexts are dropped with a startup warning, not a failure.
  Invalid key names, an empty key list for an action, and two actions bound to the same key
  in one context fail startup.
- **A printable key is refused outside `modal`.** A key that is one printable character, or
  `space`, fails startup and `--check-config` in `shell`, `board` and `detail`:
  `key "s" for action "open_store_picker" in shell context is a printable key, which types
  into the filter; bind it with alt+ or ctrl+`. The Board and Docs tabs and the store search
  type every such key into their query (`mode.Query`, `internal/mode/query.go`), so an action
  on one would never run there. `isPrintableKey` (`internal/config/keybindings.go`) is the test.
- `backspace`, `ctrl+w` and `ctrl+u` edit that query and `ctrl+t` toggles the scope of the
  store search. The four are built in: on those surfaces the query takes them before any
  binding, so bind none of them in `shell` or `board`.

Supported actions by context:

- `shell`
  - `quit`, `toggle_help`, `open_search`, `open_store_picker`, `mode_cycle_next`,
    `mode_cycle_prev`, `escape`, `reload_detail`, `edit_issue`, `create_issue`,
    `update_issue`, `close_issue`, `comment_issue`, `launch_nvim`, `launch_opencode`,
    `launch_shell_command`
- `board`
  - `move_left`, `move_right`, `move_up`, `move_down`, `move_home`, `move_end`, `page_up`,
    `page_down`, `open_detail`, `reload`
  - Docs mode, the store search and the store picker have no context of their own: each
    reads the row movement (`move_up`, `move_down`, `move_home`, `move_end`, `page_up`,
    `page_down`), `open_detail` and `reload` from this one. Rebinding them moves all four
    surfaces together, which is deliberate — each is a single list of rows, and a context
    of its own would ask for the same movement to be rebound twice. In the picker
    `open_detail` opens the highlighted store.
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
- When the active store's project path does not exist, a launcher whose `workdir` is
  blank or uses `{{project.root}}` is refused with a toast rather than exec'd in a
  directory that is gone, and the Detail footer says the launchers are off. A launcher
  with a `workdir` of its own still runs. The edit key is unaffected: it writes a temp
  document and runs the editor there.
- In `command` and `workdir` an issue field may only *extend* what the operator wrote: the value
  must not start with one, and at launch a field carrying `/`, `\` or a `..` segment refuses the
  launch. `command: "/opt/tools/run-{{issue.id}}"` is fine; `command: "{{issue.assignee}}"` is
  rejected at startup, because the issue would then name the program outright.
- `{{project.root}}` is the active store's project path, not the directory
  `taskmgr-ui` was started in: the root of the project holding the local `.tasks`
  store, or the project path registered for a central store. It follows a store
  switch from the picker. See
  [CODING.md → Store resolution](CODING.md#store-resolution).

### Writing a launcher template safely

Issue fields are operator-untrusted input. [CODING.md](CODING.md)'s Shell-launcher
security rule states the invariant; this is how a config author satisfies it.

Two questions decide whether a template is safe: what re-parses an issue field as code, and what
lets an issue field choose the program.

Three shapes re-parse an argument as a command line, and none may carry an interpolated
issue field:

- the body after a `-c` / `-lc`-style flag (`sh`, `bash`, `su`, `python`, …)
- a plain argument handed to a command that dispatches it to a shell (`tmux new-window`,
  `ssh host`, `watch`)
- a `+cmd`, `-c` or `--cmd` argument to an Ex-command editor (`vi`, `vim`, `nvim`,
  `view`, `vimdiff`, `ex`) — Ex chains on `|` and reaches a login shell through `:!`

For the first, pass the placeholders as positional arguments after the body and reference
them as `$0`, `$1`, `$2` inside the script. For the other two, pass the field through
`env`, which is not re-parsed. Inside an Ex command read it back as `$VAR`, and set a
buffer name with `nvim_buf_set_name()` rather than `:execute`, which re-parses its
argument.

A file argument to the same editor is data, not code, so `nvim /tmp/{{issue.id}}.md` is
allowed.

For `command` and `workdir` the rule is the one in the Notes above: the operator's literal fixes the
program and the directory, and the issue field only extends it. Both halves are enforced —
`ValidateDefinitions` rejects a leading placeholder at startup and under `--check-config`, and
`Launch` refuses a value that would walk out of the path with a separator or a `..`.

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
