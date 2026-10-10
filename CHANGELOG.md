# Changelog

What changed in each release, written for whoever runs `taskmgr-ui`. This is the
history readable from a checkout with no network.

Each [GitHub release](https://github.com/hk9890/task-manager-ui/releases)
carries its `vX.Y.Z` section of this file as its notes.

## Unreleased

- **Action required: the keys changed.** No action is on a bare letter, digit or
  symbol key any more, because those keys now type into the filter. The new
  defaults: `alt+h` help, `alt+s` stores, `alt+f` search, `alt+r` reload,
  `alt+e` edit, `alt+n` new issue, `alt+u` update, `alt+a` comment, `delete`
  close, `alt+v` / `alt+p` / `alt+l` the three launchers, `ctrl+c` quit. Move
  with the arrow keys, `pgup`, `pgdown`, `home` and `end`, and open with
  `enter`: `h`, `j`, `k`, `l` and `o` no longer move or open. `ctrl+q`, `?`,
  `1`–`4`, `ctrl+space`, `/` and `>` are gone. The `y` / `n` keys of a dialog
  are unchanged. The `alt+` keys need a terminal that sends `alt` as an escape
  prefix; on macOS turn on the Option-as-Meta setting of the terminal. `esc`
  and a letter that reach the app together read as that `alt+` key: under tmux
  keep `escape-time` at 10 ms or lower, the default since tmux 3.5.
  [`docs/user-guide/key-bindings.md`](docs/user-guide/key-bindings.md) has the
  whole list.
- **Action required if your config binds keys.** A `keybindings` entry in the
  `shell`, `board` or `detail` context that binds a single printable character
  or `space` now fails startup and `--check-config`, with an error that names
  the key, the action and the context. Bind the action to an `alt+` or `ctrl+`
  key, for example `quit: [ctrl+q]`. The `modal` context still takes any key.
  The shell actions `mode_board`, `mode_docs`, `mode_search`, `mode_detail` and
  `toggle_search`, the board action `load_more` and the whole `search` context
  no longer exist: an entry for one is ignored with a startup warning, so
  delete it. New actions are `open_search` in `shell` and `move_home`,
  `move_end`, `page_up` and `page_down` in `board`.
- **Added: type to filter.** On the Board and Docs tabs every key you type goes
  into a filter on the line above the columns. A row stays when every word of
  the filter is in its title or its ID. The matched text is marked and each
  column header reads `N of M`, the matching rows of the loaded rows.
  `backspace`, `ctrl+w` and `ctrl+u` edit the filter and `esc` clears it. The
  filter stays through a tab switch, a detail round trip and a reload.
- **Changed: search is no longer a tab.** `alt+f`, or the new `search` button
  on the menu bar, opens the store search from a tab or from the detail. It is
  one list of results and searches as you type; there is no `enter` to run the
  query, no preview pane and no pane focus. `ctrl+t` still switches between
  open issues and all issues. A search shows at most 100 results and the
  header reads `100 of M` when the store holds more. `esc` clears the query and
  then returns to where you opened the search; from a result's detail `esc`
  returns to the results. The view tabs are Board and Docs.
- Changed: the Done column loads the next page of closed issues only by itself,
  when the selection comes near the end of the loaded rows. The `>` key that
  loaded a page on demand is removed.
- Added: `pgup`, `pgdown`, `home` and `end` move the selection on the Board, the
  Docs tab, the store search and the store picker.
- Changed: when the terminal is too narrow for all board columns, the columns
  stay in place until the focus leaves them, and then move only as far as
  needed. A click or `left`/`right` on a column already drawn no longer shifts the
  board. The wheel now scrolls the column under the pointer and focuses it.
- Fixed: one central store directory that cannot be read no longer empties the
  store picker. That store is listed as `broken`, and `enter` on it shows the
  reason on a second toast line.
- Fixed: a refresh you did not ask for moved the cursor to another issue in the
  Done column when other closes had pushed the selected issue below the rows
  already loaded. The refresh now loads up to 3 more pages of Done to find the
  issue again, and the cursor returns to it. When the issue is not in those
  pages, the cursor stays on its row as before.

## v0.18.0

- **Action required on a light terminal.** The app now draws in a theme of its
  own, `catppuccin-mocha` by default, and no longer follows the terminal
  background. Mocha is dark: on a light terminal the selected row and much of
  the text are hard to read. Set `ui: {theme: catppuccin-latte}` in your config.
- **Action required if you select text by dragging.** The app now takes the
  mouse, and a plain drag no longer makes a terminal selection. Hold `shift`
  and drag for the terminal's own selection. The mouse cannot be turned off.
- **Changed: the screen layout.** The first line is a menu bar with the
  `stores`, `reload`, `help` and `quit` buttons, each with its key, and the
  version on the right. The view tabs Board, Docs and Search have a line of
  their own under it. An issue row is two lines: type and title, then priority,
  status and ID. A store picker row is the name and status, then the project
  path. The selected row is marked by a bar and a band of colour in place of
  `›`. The last line names what each key does. A screen shows about half as
  many issues as before.
- **Added: the views follow the store.** The board, the Docs tab, the search
  results and the detail now reload when anything writes the store: the
  `taskmgr` CLI, an agent or another `taskmgr-ui`. This also happens while the
  terminal is not focused. Before, a view reloaded at most once a minute, and
  an app whose terminal reported that it lost the focus did not reload until
  the focus came back. A change that arrives under a dialog or the help overlay
  shows when it closes. A change that arrives while you type a search query
  shows when you run the query or leave the query field. Where the store cannot
  be watched, the views refresh on focus and once a minute as before.
  `--no-auto-refresh` turns the new refresh off with the other two; `r` reloads
  at once either way.
- **Added: mouse support.** Click a row to select it and click it again to open
  it. Click a view tab, a menu-bar button or a pane. The wheel moves a list's
  selection and scrolls detail text and the help. Dialogs take keys only.
  [`docs/user-guide/key-bindings.md`](https://github.com/hk9890/task-manager-ui/blob/v0.18.0/docs/user-guide/key-bindings.md#mouse)
  has the whole list.
- **Added: drag to copy.** A drag selects a box of text and letting go sends it
  to the clipboard through the terminal. The toast says `Sent`, not `Copied`:
  a terminal or multiplexer that ignores the OSC 52 clipboard sequence leaves
  the clipboard as it was, and the app cannot tell. Use `shift` and drag there.
- **Added: themes.** `ui.theme` takes `catppuccin-mocha`, `catppuccin-macchiato`,
  `catppuccin-frappe` or `catppuccin-latte`, the one light theme.
- **Added: glyph sets.** `ui.glyphs` takes `unicode`, the default, which spells
  type, priority and status as letters as before; `nerd`, which draws them as
  icons and needs a [Nerd Font](https://www.nerdfonts.com/); or `ascii`. An
  unknown theme or glyph set fails startup and `--check-config` with the valid
  names.
- **Changed: the search results scroll.** A search loads up to 100 results and
  the Results pane scrolls through them with the selection. Before, it loaded
  only the results that fit the pane, and the others could be reached only by
  narrowing the query. The header reads `7 of 13` when the pane shows part of
  the list.
- Fixed: the help overlay was cut at the bottom of the terminal and the rest
  could not be read. It now scrolls on the detail scroll keys, `j`/`k`,
  `pgup`/`pgdown`, `home`/`end`, and under the wheel.
- Fixed: on the Search view in a terminal shorter than 11 rows, the first lines
  of the screen scrolled off the top. The screen is now cut to the terminal
  height: the menu bar stays on the first line and the key legend on the last.
- Fixed: a refresh you did not ask for returned the Done column to its first
  page. With the column paged down, the selected issue left the column and the
  cursor moved to another one. The column now reloads as deep as it was paged,
  and the cursor stays on its issue while that issue is within those rows. A
  cursor move made while a refresh ran was also undone when it finished. `r`
  still returns to the first page.

## v0.17.0

- **Action required if your config binds `s` in the shell context.** `s` now
  opens the store picker, and a config that already binds it to another shell
  action fails startup with `key "s" conflicts between actions ... in shell
  context`. Rebind one of the two; [`docs/user-guide/key-bindings.md`](./docs/user-guide/key-bindings.md)
  has an example.
- **Added: a store picker.** `s` from any tab lists every central task store
  registered on this machine with its name, project path and health, and marks
  the one in use. `enter` switches the running app to the highlighted store in
  place: the board, Docs tab, search, detail and launchers all follow, and the
  header names the new store. A store the registry knows but cannot open stays
  listed with the reason. `esc` returns to the tab you came from.
- **Changed: starting with no task store opens the picker instead of exiting.**
  `taskmgr-ui` run outside any project used to print `no .tasks directory found`
  and exit 1. It now opens on the picker with a toast saying why, as does an
  unknown `--store-name`. A store that exists but cannot be opened still exits
  1.
- **Added: create a store from the picker.** When started somewhere with no
  store, two rows above the registry offer to create a local store or a central
  store for that directory, with the ID prefix and registry name prefilled the
  way `taskmgr init` would choose them. The new store opens straight away.
- **Changed: launchers are refused while the active store's project directory
  is missing.** A central store whose project was moved or deleted still opens,
  but `n`, `p` and `l` used to run in a directory that was gone and blamed the
  launcher. Each now refuses with a toast naming the path, and the Detail
  footer says the launchers are off. A launcher with its own `workdir` still
  runs; `e` is unaffected.
- **Columns now read newest change first.** Not Ready, Ready, In Progress and
  the Docs tab order their rows by last change, most recent at the top, with
  priority as the tie-break. Before, priority came first and a P0 issue
  untouched for a month sat above the work you changed an hour ago. Done keeps
  its close-date order.
- **Age markers show where the stale work starts.** A muted divider reading
  `older than 1 day` and another reading `older than 1 week` sit before the
  first issue past that age, each with the count of issues below it. A column
  of fresh work shows no divider; a column where everything is stale shows
  both stacked at the top. Done draws none.
- Fixed: a write that a store hook refused was reported as
  `update issue failed: update issue: unknown: ...`, as if the app had faulted.
  It is now reported as a refusal with the hook's own reason, the way the
  `taskmgr` CLI reports it.
- Fixed: under `--repo memory`, a search page shorter than the full result set
  was reported as complete, so no further page was offered and the list was
  silently truncated.

## v0.16.0

- **Security — you were exposed if you use the built-in `nvim` launcher.**
  Pressing `n` on an issue ran whatever an attacker had put in its title,
  assignee or labels as a shell command. The shipped launcher was enough; no
  customisation was needed. It is fixed.
- **Security — your own launcher config may now be refused at startup.** Two
  shapes that let issue content decide what runs are rejected: a command line
  handed to another program to re-parse — `sh -c`, `tmux new-window`, `ssh`,
  `watch`, `su`, `python`, and editors taking an Ex command — and a `command` or
  `workdir` that begins with an issue placeholder. `--check-config` names the
  offending action. The `tmux` example that used to be in
  [`docs/CONFIGURATION.md`](./docs/CONFIGURATION.md) was one of these; replace it
  if you copied it. Passing an issue field as an ordinary argument or through
  `env` is unaffected, as is the documented `tool-{{issue.id}}` shape.
- **Action required if a `hooks:` block is left in your store's
  `.tasks/config.yaml` or in your `~/.taskmgr/config.yaml`.** taskmgr-ui is now
  read-only against such a store: the board, detail and search render normally,
  and every write is refused. Move each hook entry into a package directory, add
  it with `taskmgr package add`, then delete the `hooks:` block.
  `taskmgr package list` prints the full diagnostic, and
  [`docs/CONFIGURATION.md`](./docs/CONFIGURATION.md) has the detail.
- Fixed: updating an issue you had drilled into from Dependencies wiped its
  labels and assignee — the dialog opened with those fields blank, and
  submitting it saved the blanks.
- Fixed: when your edited file could not be read back, it was deleted along with
  everything you had just written. It is now kept, and the error names its path.
- Fixed: the editor round-trip could save one field's text into another, and an
  editor set to `fileformat=dos` reported a change to a file you had not touched
  and wrote the carriage returns into the issue.
- Fixed: a board row kept its old title after an `e` edit until the next
  refresh — a minute away — under a toast saying the update had succeeded.
- Fixed: an issue could appear in two board columns at once and be counted in
  both headers, and auto-refresh then moved the cursor out of In Progress.
- Fixed: drilling from Dependencies into a child issue left `e`, `x`, `u`, `a`
  and the launchers acting on the parent, with nothing on screen to say so.
- Fixed: `ctrl+t` flipped the search scope badge without re-running the search,
  so the Results header named a scope the rows did not match, with no key to
  restore agreement.
- Fixed: the Update Status and Update Priority dialogs could apply a value other
  than the one you submitted.
- Fixed: info and warning toasts drew an empty box with no text — which is how
  every launcher result and every warning reached you.
- Fixed: a toast wider than the terminal was drawn past the right edge.
- Fixed: descriptions, notes and comments rendered in dark-theme colours on a
  light terminal, and a CJK or emoji line was cut off instead of wrapped.
- Fixed: a line containing tabs — pasted source, a Makefile, a TSV table — ran
  past the pane border instead of wrapping, and a wrapped line lost its indent.
- Fixed: two panes rendering markdown at the same time could interleave their
  text or crash the app.
- Fixed: `deferred` issues rendered as an unrecognised status. They now have
  their own colour and the `DFR` and `D` tokens.
- Fixed: opening any dialog froze the spinner and ended auto-refresh for the
  rest of the session, and a terminal resized behind a dialog was ignored.
- Fixed: one failed auto-refresh cleared the board behind an error row, reset the
  closed count to zero — which suppressed every later load-more — and dropped
  your selection. The error is now shown over the rows already on screen, and a
  resized or shrunk column no longer scrolls to an empty pane.
- Fixed: a page of rows that arrived after a reload left a gap in the column that
  no later load-more filled.
- Fixed: `r` in detail mode did nothing and left the header stuck on
  "Loading: detail" for the rest of the session.
- Fixed: an unreadable or corrupt store, and a failed editor round-trip, reported
  no cause. Both now name it.
- Fixed: a failed write to the terminal stopped the same diagnostic reaching the
  log file, which is the surface that matters when the terminal is already
  misbehaving.
- Fixed: the search skeleton did not animate on a cold start, and `ready`
  rendered in the muted unknown-status colour.
- Changed: detail mode no longer redraws the issue ten times a second for the
  life of the process.
- Fixed: `--repo memory` diverged from a real store on create, update, close,
  reopen and search. The two now agree.

## v0.15.0

- Added: a Docs tab listing every `doc`-type issue in one column. It is the only
  surface that shows an **open** doc — the SDK keeps non-work types out of the
  Ready queue, so an open doc never reaches a board column. Enter opens the
  selected doc in detail mode, `r` reloads, and `4` switches to the tab
  directly.
- Changed: `tab` and `shift+tab` now cycle the header tab strip (Board, Docs,
  Search) instead of moving board columns and search panes. The board keeps
  `l`/`right` for the next column; search keeps `ctrl+j`/`ctrl+k` for pane
  focus. `ctrl+pgdown`/`ctrl+pgup` still cycle, but over the three tabs only —
  they no longer step into detail mode, which is reached with `enter` (or `3`)
  and left with `esc`.
- Changed: upgraded the task-manager SDK from v0.7.0 to v0.8.0. The UI's own
  behaviour is unchanged. The SDK now reads a `hooks` block from the per-user
  `~/.taskmgr/config.yaml` and runs it before the store's own hooks, so a global
  hook that already applied to `taskmgr` writes now also applies to writes made
  from `taskmgr-ui`; a hook that refuses one surfaces its message in the error
  toast.
- Changed: search covers open work by default. A store accumulates closed issues
  without bound, so every query used to bury live work under finished work. The
  Results header names the active scope, and `ctrl+t` toggles closed history
  into the query — a control key because the query box takes every printable
  rune as text. The Docs tab still lists closed docs: closing a doc archives
  reference material rather than finishing work.
- Fixed: the selection chevron could vanish in a clipped detail pane. A scrolled
  pane spends its first and last row on the `… (N earlier)` / `… (N more)`
  indicators, and the selection was allowed to sit on exactly those rows, so
  holding `j` in the Dependencies or Metadata pane scrolled the window with no
  visible chevron.
- Fixed: the `--repo memory` backend put open `doc` issues in the Ready and
  Blocked columns, which the real backend never does. The two now apply the same
  rule.

## v0.14.0

- Fixed: `taskmgr-ui` failed to start in any project whose store had been
  promoted with `taskmgr store move --central`, reporting "no .tasks directory
  found". Stores are now resolved the way the `taskmgr` CLI resolves them —
  local `.tasks` by walk-up, then the central registry — and a new
  `--store-name <name>` flag opens a registered central store from anywhere.
- Changed: `{{project.root}}` in launcher templates now interpolates the
  resolved store's project path rather than the directory `taskmgr-ui` was
  started in. These differ when the app is started from a subdirectory of the
  project, or against a central store.
- Changed: `TASKMGR_DIR`, which the previous store open ignored, is now refused
  by the SDK with an explanatory error. Unset it and use `--cwd` or
  `--store-name`.
- Added: a startup warning when the resolved store's project path no longer
  exists — a central registry entry outliving a moved or deleted project opens
  normally, while launchers without an explicit `work_dir` would exec in a
  directory that is gone.

## v0.13.0

- Upgraded the task-manager SDK to v0.7.0 and added support for its `doc` issue
  type. Doc issues now render with their own compact-row token and colour
  instead of `?`, and — following the SDK's `Type.IsWork` — are excluded from
  the Ready and Blocked board columns, so an open doc is reachable via search.
  Issue bodies over 64KB, which the SDK now keeps in a content sidecar, render
  in the detail view instead of appearing empty.
- Internal: documentation/code drift fixes across the repository contract and
  backend catalogs, runtime diagnostics routed through the injected logger, the
  application lifecycle context threaded through shell reads, a unified
  golden-file harness, and expanded modal keyboard coverage.

## v0.12.1

- Maintenance release: internal refactors (god-file splits, consistency-seam
  unification, repository test seams), removal of dead code including leftover
  Windows-only paths, expanded and cross-backend test coverage, hardened
  tracker-ID guardrails, and documentation accuracy fixes. No user-facing
  behavior changes.

## v0.12.0

- Initial public release.
