# Key Bindings

This document describes the current default keyboard shortcuts used by Beads
Workbench.

These defaults are defined in `internal/config/keybindings.go` and can be
overridden through runtime config.

## Shell / Global

- `ctrl+q` — quit
- `?` — toggle help
- `ctrl+@` — toggle search mode
- `3` — switch to detail mode
- `esc` — return from detail/search to browse, or dismiss toast state
- `r` — reload detail mode from the gateway (detail mode only)
- `e` — edit selected issue in external editor
- `c` — create issue
- `u` — update selected issue metadata
- `x` — close selected issue
- `a` — add comment to selected issue
- `n` — launch `nvim` action in detail mode
- `p` — launch `opencode` action in detail mode
- `l` — launch `shell-command` action in detail mode

## Board Mode

- `h`, `left` — move to previous column
- `l`, `right`, `tab` — move to next column
- `k`, `up` — move up within the current column
- `j`, `down` — move down within the current column
- `enter`, `o` — open selected issue in detail mode
- `r` — reload board data

## Search Mode

Typing while the query field is focused updates the search query directly.

- `k`, `up` — move up in results
- `j`, `down` — move down in results
- `h`, `left` — move focus left between panes
- `l`, `right` — move focus right between panes
- `/` — focus the query field
- `r` — reload current search
- `enter` — open selected result in detail mode
- `tab`, `ctrl+j` — cycle focus to next search pane
- `shift+tab`, `ctrl+k` — cycle focus to previous search pane
- `backspace` — delete previous query character when query is focused
- `ctrl+u` — clear query when query is focused

## Detail Mode

- `k`, `up` — scroll up one line
- `j`, `down` — scroll down one line
- `pgup` — page up
- `pgdown` — page down
- `home` — jump to top
- `end` — jump to bottom

## Modal Dialogs

- `tab`, `down` — move to next field
- `shift+tab`, `up` — move to previous field
- `left` — move button focus left
- `right` — move button focus right
- `enter` — advance from input focus or confirm on button focus
- `esc` — cancel when the modal is not required
- `y` — submit when button row is focused
- `n` — cancel when button row is focused

## Notes

- Keybindings are context-specific. The same key may do different things in
  shell, board, search, detail, and modal contexts.
- Search mode intentionally captures normal text entry while the query field is
  focused.
- Modal `y`/`n` behavior exists in addition to the configurable modal keymap.
