# Key Bindings

This document describes the current default keyboard shortcuts used by Task
Manager UI.

These defaults are defined in `internal/config/keybindings.go` and can be
overridden through runtime config.

## Shell / Global

- `ctrl+q`, `ctrl+c` — quit
- `?` — toggle help
- `tab`, `ctrl+pgdown` — next tab in the header strip (Board → Docs → Search → Board)
- `shift+tab`, `ctrl+pgup` — previous tab in the header strip
- `ctrl+space` (`ctrl+@`) — go to search from any other tab; from search or detail, return to board
- `1` — switch to board mode
- `2` — switch to search mode
- `3` — switch to detail mode
- `4` — switch to docs mode
- `esc` — return from detail to the tab you came from, from any other tab to board, or dismiss toast state
- `r` — manually reload detail mode from the repository immediately (detail mode only)
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
- `l`, `right` — move to next column
- `k`, `up` — move up within the current column
- `j`, `down` — move down within the current column
- `enter`, `o` — open selected issue in detail mode
- `r` — manually reload board data immediately
- `>` — load more closed issues into the Done column (only active when the Done column is focused; configurable as the board `load_more` action)

## Docs Mode

One column listing every `doc`-type issue, open and closed — closing a doc
archives it. Docs are not work, so they never reach the Ready or In Progress
columns; this tab is where they are browsed. Docs mode reuses the board keymap:

- `k`, `up` — move up
- `j`, `down` — move down
- `enter`, `o` — open selected doc in detail mode
- `r` — manually reload the docs list immediately

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
- `enter` (metadata focused) — edit the selected Status or Priority field
- `ctrl+j` — cycle focus to the next of the four panes (Query, Results, Content, Metadata)
- `ctrl+k` — cycle focus to the previous pane
- `backspace` — delete previous query character when query is focused
- `ctrl+u` — clear query when query is focused
- `ctrl+t` — widen the search to closed issues, or narrow it back to open work (works in
  any focus state). Search starts on open work only; the Results header shows `open` or `all`

`backspace`, `ctrl+u` and `ctrl+t` are built in and cannot be rebound.

## Detail Mode

- `left`, `right` — move focus between the Dependencies, Content and Metadata panes
- `k`, `up` — scroll up one line in the focused pane; in Dependencies and Metadata, move the cursor
- `j`, `down` — scroll down one line in the focused pane; in Dependencies and Metadata, move the cursor
- `pgup` — page up
- `pgdown` — page down
- `home` — jump to top
- `end` — jump to bottom
- `enter` (Dependencies focused) — open the highlighted related issue
- `enter` (Metadata focused) — edit the selected Status or Priority field

## Modal Dialogs

- `tab`, `down` — move to next field
- `shift+tab`, `up` — move to previous field
- `left` — move button focus left
- `right` — move button focus right
- `enter` — submit in the Status and Priority dialogs; otherwise advance from input focus, or confirm on button focus
- `esc` — cancel any modal
- `y` — submit when button row is focused
- `n` — cancel when the button row is focused, except in the Status and Priority dialogs, which require a value

## Notes

- Keybindings are context-specific. The same key may do different things in
  shell, board, search, detail, and modal contexts.
- `ctrl+space` may be reported by some terminals as `ctrl+@`; both refer to the
  same default toggle-search binding.
- `tab`/`shift+tab` belong to the shell tab strip everywhere except in a modal,
  where they still move between fields — a modal consumes keys before the shell
  sees them. They also switch tabs while the search query field is focused;
  typed text is unaffected.
- Search mode intentionally captures normal text entry while the query field is
  focused.
- Modal `y`/`n` behavior exists in addition to the configurable modal keymap.
- The startup-error screen also quits on `q`.
- Data views also auto-refresh when the app regains focus and on a low-frequency
  background schedule. Use `r` when you want an immediate manual refresh.
