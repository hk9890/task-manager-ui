# Key Bindings

This document describes the current default keyboard shortcuts used by Task
Manager UI.

These defaults are defined in `internal/config/keybindings.go` and can be
overridden through runtime config.

## Shell / Global

Outside a dialog no action is on a bare letter, digit or symbol key: on the
Board and Docs tabs and in the store search those keys type into the filter or
the query. The actions sit on `alt+` chords and on keys that print nothing.

- `ctrl+c` — quit
- `alt+h` — toggle help
- `tab`, `ctrl+pgdown` — next view tab (Board → Docs → Board)
- `shift+tab`, `ctrl+pgup` — previous view tab
- `alt+f` — open the store search, from a tab or from detail
- `esc` — clear the filter or the query when it holds text; otherwise return from detail or the search to where you came from, from Docs to Board, or dismiss toast state
- `alt+r` — manually reload detail mode from the repository immediately (detail mode only)
- `alt+e` — edit selected issue in external editor
- `alt+n` — create issue
- `alt+u` — update selected issue metadata
- `delete` — close selected issue
- `alt+a` — add comment to selected issue
- `alt+s` — open the store picker: every central task store on this machine
- `alt+v` — launch `nvim` action in detail mode
- `alt+p` — launch `opencode` action in detail mode
- `alt+l` — launch `shell-command` action in detail mode

`alt+v`, `alt+p` and `alt+l` run in the store's project directory. When that
directory is gone — the project was moved or deleted — they are refused with a
message, and the detail footer reads `launchers off`.

A terminal must send `alt` as an escape prefix for the `alt+` keys to arrive.
Most do; on macOS, turn on "Use Option as Meta key" (Terminal) or set the Option
key to `Esc+` (iTerm2). Every action can be rebound in the config.

## Filter

The Board and Docs tabs each have a filter on the line above the columns. It is
always live: no key enters it, you type.

- any letter, digit, symbol or `space` — add to the filter; pasted text is added too
- `backspace` — delete the last character
- `ctrl+w` — delete the last word
- `ctrl+u` — clear the filter
- `esc` — clear the filter; with an empty filter `esc` does what it does elsewhere

A row stays when every word of the filter is in its title or its ID, whatever
the case. The rows keep their order, the matched text is marked, and each column
header reads `N of M`: `N` matching rows of the `M` loaded. The filter has no
cursor — `left` and `right` move between the board columns — and holds at most
64 characters.

The board has one filter for all four columns. When the focused column has no
match, the focus moves to the first column that has one, and returns when its
column has a match again unless you moved it meanwhile. In the Done column the
filter covers the rows loaded so far; moving toward the end of the matches loads
the next page.

A filter stays through a tab switch, a detail round trip and a reload. Opening
another store ends it. These keys are built in and cannot be rebound.

## Board Mode

- `left` — move to previous column
- `right` — move to next column
- `up` — move up within the current column
- `down` — move down within the current column
- `pgup`, `pgdown` — move one page within the current column
- `home`, `end` — move to the first or the last row of the current column
- `enter` — open selected issue in detail mode
- `alt+r` — manually reload board data immediately

The Done column loads the next page of closed issues by itself when the
selection comes near the end of the loaded rows.

## Docs Mode

One column listing every `doc`-type issue, open and closed — closing a doc
archives it. Docs are not work, so they never reach the Ready or In Progress
columns; this tab is where they are browsed. Docs mode reuses the board keymap:

- `up`, `down` — move up or down
- `pgup`, `pgdown` — move one page
- `home`, `end` — move to the first or the last doc
- `enter` — open selected doc in detail mode
- `alt+r` — manually reload the docs list immediately

## Store Search

`alt+f`, or the `search` button on the menu bar, opens the store search from a
tab or from detail. It is not a tab: no tab is marked while it is up. It asks
the store when you stop typing, and it also finds an issue through its description;
the filter of a tab reads only the titles and IDs of the rows already loaded.

- type, `backspace`, `ctrl+w`, `ctrl+u` — edit the query, as in the filter; the search runs a moment after the last key
- `ctrl+t` — switch between open issues and all issues. The search starts on
  open issues; the column title reads `Results · open` or `Results · all`
- `up`, `down`, `pgup`, `pgdown`, `home`, `end` — move in the results
- `enter` — open selected result in detail mode; `esc` there returns to the results.
  While a search is still running or about to run, `enter` waits for it and opens the result it
  selects; any other key, a click or the wheel before that cancels the wait
- `alt+r` — run the search again
- `esc` — clear the query; with an empty query, return to where the search was opened from
- `tab`, `shift+tab` — leave the search for a tab

With an empty query the search lists the open issues. A search returns at most
100 issues; the header reads `100 of M` when the store holds `M` matches. The
matched words are marked in the title and the ID, so a result found through its
description carries no mark. Edit, update, comment and close act on the selected
result.

The movement, `enter` and reload keys are the board's and follow a rebind of
them. `ctrl+t` is built in and cannot be rebound.

## Detail Mode

- `left`, `right` — move focus between the Dependencies, Content and Metadata panes
- `up` — scroll up one line in the focused pane; in Dependencies and Metadata, move the cursor
- `down` — scroll down one line in the focused pane; in Dependencies and Metadata, move the cursor
- `pgup` — page up
- `pgdown` — page down
- `home` — jump to top
- `end` — jump to bottom
- `enter` (Dependencies focused) — open the highlighted related issue
- `enter` (Metadata focused) — edit the selected Status or Priority field

## Store Picker

The full-screen list of every central task store registered on this machine,
opened with `alt+s` from any tab. A local `.tasks` store is not listed: open one by
starting `taskmgr-ui` in its project. The picker reuses the board keymap:

- `up`, `down` — move up or down
- `pgup`, `pgdown` — move one page
- `home`, `end` — move to the first or the last row
- `enter` — open the highlighted store; the app switches to it and shows its board
- `alt+r` — re-read the registry
- `esc` — return to the tab you opened it from, or quit when no store is open

When `taskmgr-ui` starts somewhere with no task store, it opens on the picker
instead of exiting. Until you open a store from it, only `esc`, quit and help
do anything. Two rows at the top of the list offer to create a local or a
central store for that directory: select one and press `enter` to fill in its
name and ID prefix.

## Modal Dialogs

- `tab`, `down` — move to next field
- `shift+tab`, `up` — move to previous field
- `left` — move button focus left
- `right` — move button focus right
- `enter` — submit in the Status and Priority dialogs; otherwise advance from input focus, or confirm on button focus
- `esc` — cancel any modal
- `y` — submit when button row is focused
- `n` — cancel when the button row is focused, except in the Status and Priority dialogs, which require a value

The help overlay is taller than most terminals and shows `… N more lines` where
it is cut. It scrolls on the Detail Mode scroll keys, and follows them when a
config rebinds them:

- `up` / `down` — scroll one line
- `pgup` / `pgdown` — scroll one page
- `home` / `end` — jump to the top or the bottom
- `alt+h`, `esc`, `enter` — close it; it opens at the top again

## Mouse

The mouse does what the keys above do. It has no bindings to configure and
cannot be turned off: the app takes every click, wheel notch and plain drag.
**Hold `shift` and drag** to select text with the terminal's own selection
instead.

- The selected row carries a band of colour across its width. The row under the
  pointer carries a quieter one, and a tab under the pointer brightens.
- **Click a row** to select it. **Click it again** within 0.4 s to open it, as
  `enter` does: an issue, a doc or a search result opens in detail mode, a row of
  the detail Dependencies pane navigates to that issue, a store picker row opens
  that store.
- **Click a view tab** — Board, Docs — to switch to it.
- **Click a button** on the menu bar, the first line — the name of the open store, `search`,
  `reload`, `help`, `quit` — to do what the key shown beside it does. The store's name opens the
  store picker. `reload` reloads the view on screen.
- **Click a pane** in detail mode to focus it.
- **Wheel** over a list moves its selection one row a notch. On the board that is
  the column under the pointer, which takes the focus.
- **Wheel** over the detail Content or Metadata pane, or over the help overlay,
  scrolls it three lines a notch.
- While help or a dialog is open, the mouse reaches nothing under it. A dialog
  takes keys only.
- **Drag** to select a box of text; `esc` during the drag drops it. The box can
  cross panes and overlays, and the screen holds still under it until you let
  go.
- Letting go sends the text to the clipboard through the terminal, with the
  OSC 52 sequence, and a toast reads `Sent N characters to the clipboard;
  shift+drag if not copied`. The app cannot tell whether the terminal took it.
  A terminal or multiplexer that ignores OSC 52 leaves the clipboard as it
  was: **hold `shift` and drag** there, and copy with the terminal.

## Notes

- Keybindings are context-specific. The same key may do different things in
  shell, board, detail, and modal contexts.
- `tab`/`shift+tab` belong to the view tabs everywhere except in a modal,
  where they still move between fields — a modal consumes keys before the shell
  sees them. From the store search they leave it for a tab.
- Modal `y`/`n` behavior exists in addition to the configurable modal keymap.
- The startup-error screen also quits on `q`.
- A terminal sends `alt+e` as `esc` followed by `e`, so `esc` and a letter that
  reach the app together are read as the `alt+` key: `esc` then a quick `e`
  opens the editor where you meant to clear the filter and type. A local
  terminal keeps them apart. Under tmux set `set -sg escape-time 10` or lower
  (the default since tmux 3.5); over a slow link, clear the filter with `ctrl+u`.
- Data views refresh by themselves when the store changes, whoever wrote it — the
  `taskmgr` CLI, an agent, another `taskmgr-ui` — also while the terminal is not
  focused. A change that arrives under a dialog or the help overlay shows when it
  closes. Where the store cannot be watched, they refresh when the app regains
  focus and once a minute. `--no-auto-refresh` turns all three off; `alt+r` reloads
  at once either way.
