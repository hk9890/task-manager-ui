# Key Bindings

This document describes the current default keyboard shortcuts used by Beads
Workbench.

These defaults are defined in `internal/config/keybindings.go` and can be
overridden through runtime config.

## Shell / Global

- `ctrl+q` — quit
- `?` — toggle help
- `ctrl+space` (`ctrl+@`) — toggle search mode
- `f13` — switch to board mode
- `f14` — switch to search mode
- `f15` — cycle to the next mode
- `f16` — cycle to the previous mode
- `3` — switch to detail mode
- `esc` — return from detail/search to browse, or dismiss toast state
- `r` — manually reload detail mode from the gateway immediately (detail mode only)
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
- `r` — manually reload board data immediately

## Search Mode

Typing in the query field edits a draft query; press Enter to run the search.
Results are not updated until Enter is pressed — while the draft differs from
the last applied query, the Results pane marks the displayed rows as stale.

- `k`, `up` — move up in results
- `j`, `down` — move down in results
- `h`, `left` — move focus left between panes
- `l`, `right` — move focus right between panes
- `/` — focus the query field
- `r` — manually reload the current search immediately
- `enter` (query field focused) — submit the draft query and run the search
- `enter` (results focused) — open selected result in detail mode
- `tab`, `ctrl+j` — cycle focus to next search pane
- `shift+tab`, `ctrl+k` — cycle focus to previous search pane
- `backspace` — delete previous query character when query is focused (built-in behavior, not part of the configurable search keymap)
- `ctrl+u` — clear query when query is focused (built-in behavior, not part of the configurable search keymap)

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
- `ctrl+space` may be reported by some terminals as `ctrl+@`; both refer to the
  same default toggle-search binding.
- Search mode intentionally captures normal text entry while the query field is
  focused.
- Modal `y`/`n` behavior exists in addition to the configurable modal keymap.
- Data views also auto-refresh when the app regains focus and on a low-frequency
  background schedule. Use `r` when you want an immediate manual refresh.
