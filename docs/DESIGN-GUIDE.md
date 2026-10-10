# Design Guide

The interaction and rendering law for every surface under `internal/ui/` and `internal/mode/`.

Three rules here are gated — type/colour parity (`renderhelpers/type_style_parity_test.go`), the
tab strip's ownership of `tab`/`shift+tab`, and no action on a key a browse surface takes for
itself (both `internal/config/keybindings_test.go`; `internal/mode/search/model_test.go` pins the
reserved keys to the surfaces). The rest is held at review ([REVIEWING.md](REVIEWING.md)).

[CODING.md](CODING.md)'s rule 8 owns the `internal/ui/` and `internal/mode/` boundary. `modal` and
`toaster` are the exception to it — they carry Bubble Tea state of their own. `loading` is stateless
but owns a message and a command (`TickMsg`, `SpinnerTickCmd`); the frame counter lives in the
shell. Every other package is pure.

## Colour roles

- Name a role from `internal/ui/styles/colors.go`; never write a hex literal or a palette colour at
  a call site.
- A role takes its colour in `applyFlavor` (`internal/ui/styles/theme.go`), from the Catppuccin
  flavour of the theme. That function is the only place a colour is chosen, so a new role is one
  declaration in `colors.go` and one line there, and it then holds in every theme.
- A new theme is one entry in `flavors`, and in `lightThemes` when it draws on a light background —
  `styles.Dark` is what markdown takes its glamour style from.
- `styles.Apply` runs from `cmd/taskmgr-ui` before the first frame, and again on each change made
  on the configuration screen (`Model.applyConfigChange`, `internal/app/config_screen.go`). The
  roles are package variables with no guard, so call it under a running program from `Update`
  only, never inside a `tea.Cmd`: `View` reads them on the same goroutine.
- After an `Apply` under a running program, clamp the scroll offsets as a resize does
  (`applyWorkspaceSizeToBrowseModes`, `detail.ClampScroll`): a light theme renders markdown in
  another glamour style and a glyph set draws tokens of another width.
- A test draws with `catppuccin-mocha` and the `unicode` glyphs, which the package starts on; one
  that applies another restores it in `t.Cleanup` and stays out of `t.Parallel`.
- The roles are grouped by what they mean, not by hue: text (`TextPrimaryColor`, `TextMutedColor`,
  `TextSecondaryColor`), shell chrome (`ShellTab*`, `ShellAction*`, `ShellRuleColor`,
  `ShellFooterHelpColor`), the query line and its matches (`QueryAccentColor`, `MatchTextColor`),
  the configuration screen (`SectionHeadingColor`, `SettingLabelColor`),
  borders and overlays (`BorderDefaultColor`, `OverlayBorderColor`, `BorderHighlightFocusColor`),
  buttons (primary / secondary / danger, each with a `Focus` variant), toasts
  (`ToastBorder{Success,Error,Info,Warn}Color`), and the issue vocabulary below.
- Focus on a pane or a column is `BorderHighlightFocusColor` on the border. Only the tabs and
  the modal buttons carry focus on a background instead, each with its own `Focus` role. A row's
  background is not focus: it is the selection or the pointer (`RowSelectedBgColor`,
  `RowHoverBgColor`).

## The issue vocabulary

An issue's type, priority and status each render as a compact token plus a colour, resolved by
`styles.IssueTypeStyle`, `styles.IssuePriorityStyle`, and `styles.IssueStatusStyle`.

| Field | Token in the `unicode` and `ascii` sets | Source |
|---|---|---|
| Type | `B` bug, `T` task, `F` feature, `E` epic, `C` chore, `D` doc, `?` unknown | `renderhelpers.CompactIssueType` |
| Priority | `P0`–`P4` | `renderhelpers.CompactPriority` |
| Status | `OPN`, `IP`, `BLK`, `CLS`, `RDY`, `DFR` | `renderhelpers.CompactIssueState` |
| Status (dense rows) | `O`, `I`, `B`, `C`, `R`, `D` | `renderhelpers.CompactIssueStateNarrow` |

The `nerd` set draws each of the three as a one-cell icon instead (`glyphSets` in
`internal/ui/styles/glyphs.go`), and one icon serves both status widths.

Adding an issue type or status takes a token in every glyph set **and** a colour: one with a
distinct glyph but no distinct colour reads as unrecognised on the board.
`internal/ui/shared/renderhelpers/type_style_parity_test.go` pins both sets together — the status
half reads an explicit case in `CompactIssueState` as the signal, so a token that matches what its
default branch would derive counts as no token at all.

Reach for the `*Styled` variant (`CompactIssueTypeStyled`, `CompactPriorityStyled`, …) to render, and
the plain one for width math — a styled token carries escape bytes that break `len`-based arithmetic.

## Glyphs

A marker that carries meaning comes from the applied glyph set, `styles.Glyphs`. The operator picks
the set — `nerd`, `unicode` or `ascii` — because a program cannot ask which font is loaded. The
column below shows the `unicode` set:

| Glyph | Means | Field of `styles.GlyphSet` |
|---|---|---|
| `▌ ` / two spaces | the selection gutter, always 2 cells wide; take it from `styles.SelectionPrefix` | `Cursor` |
| `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` | work in flight; draw it with `loading.Glyph` | `Spinner` |
| `❯` | the prompt in front of a query line | `Prompt` |
| `‹ ›` | either side of a value the operator steps through, on the configuration screen | `StepPrev`, `StepNext` |
| `✅ ❌ ℹ ⚠` | toast severity | `ToastSuccess` and its siblings |
| the issue tokens above | type, priority, status | `IssueType`, `Priority`, `Status` |

- A new marker is a field of `GlyphSet` with a value in all three sets. A row marker that is not a
  letter token is one cell wide; a toast glyph may be two, and `glyphCells`
  (`internal/ui/toaster/toaster.go`) budgets for the wider. A spinner's frame count divides ten,
  the length of the shell's frame counter.

The rest is the same in every set, and each has one definition:

| Glyph | Means | Defined in |
|---|---|---|
| `…` | truncated content — one cell, so it keeps more text than `...` | `textutil.TruncateString`, `textutil.TruncateStringFront` |
| `╭ ╮ ╰ ╯ ─ │` | a section border (a modal or toast frames itself with `lipgloss.RoundedBorder()`) | `styles.FormSection` |
| `░` | skeleton loading bar | `issuerow.SkeletonGlyph` |
| `├─ └─ │` | comment output tree | `internal/ui/detail/comments.go` |
| `• ` | a metadata list item | `internal/ui/detail/metadata.go` |
| `·` | separator between fields, menu-bar buttons and legend hints | inline at the call site |

Spend a new glyph only when an existing one cannot carry the meaning, and define it next to its
siblings rather than inline at the call site.

Pick the **text-presentation** form of a symbol that has both — `ℹ` and `⚠`, never `ℹ️` and `⚠️`.
The emoji-presentation variant appends U+FE0F, which `lipgloss.Width` measures as two cells and a
terminal following wcwidth draws as one. The frame is then built a cell wider than it is drawn and
`overlay.Place` splices the line short, so the toast rendered as a broken box with no message.
`toaster.TestToastGlyphWidthsAgreeWithWcwidth` pins the two measures together for the toast set.

## Build from the shared chrome

- Frame every column, pane and shell with `styles.FormSection`. It owns the rounded corners, the
  title inlays (`TopLeft` / `TopRight`), focus colouring, and padding each line to the inner width.
- `FormSection` returns the literal string `too narrow` below width 6. A caller that needs a
  different degraded rendering handles the narrow case before calling.
- `ui/shared/issuerow` is the single compact issue-row renderer for every issue list. Row rendering
  stays there.
- A row in a list is two lines: what it is, then its details, dimmer. `issuerow.RenderCompact`
  draws the type and the title over the priority, the status and the ID; `ui/storepicker` draws a
  store's name and status over its project path. A list that scrolls, counts or hit-tests its rows
  reads `issuerow.Height` (the picker its own `rowLines`) rather than assuming a line. A relation
  in a detail pane is the exception, `issuerow.RenderReferenceCompact`: those panes are a few rows
  tall.
- `ui/board` is the one list container. The docs tab and the store search are each a board column
  by another name, so they draw through `board.Render` with a single `Column` rather than growing a
  renderer of their own. Each takes the content lines its column holds from `board.ContentRows`,
  which subtracts the list head and the two borders; none repeats that arithmetic. Both hold a
  `rowlist.List` (`internal/mode/rowlist`) for the selection, the scroll offset, the move keys and
  the mouse.
- `ui/detail` renders the issue detail; it is separate from compact row rendering by design.

## The shell chrome

The shell frames the workspace with four lines, all drawn in `internal/app/render.go`: the menu
bar, the rule under it and the tab line (`Model.renderHeader`), and the key legend below
(`Model.renderFooter`).

- The menu bar holds the buttons from the left edge and the build version flush right. A button is
  a shell action that is not about the selected row, and it shows the key bound to that action: the
  bar is a second way to reach it, never the only one. Add one as an entry in `barActions`, with
  the method the key switch in `handleShellKey` also calls.
- The first button opens the store picker, and its label is the active store's name
  (`Model.storeLabel`), bold in `ShellActionColor`: the header names the store nowhere else. A
  name wider than `storeLabelMax` is cut; with no name the label is `stores`.
- `reload` is one button for every surface: it runs the reload of the surface on screen and shows
  that surface's key (`Model.reloadKey`).
- `config` opens the configuration screen (`Model.openConfig`), a surface above the shell.
- When the bar does not fit, the version goes first. Then the store's name is cut down to
  `storeLabelFloor`, then the buttons go from the right, and the store button, when it stands
  alone, is cut to the room that is left (`Model.barCells`).
- The tab line holds the spinner cell and the view tabs, and nothing else. Do not repeat there
  what the screen already says: on Board and Docs the active tab is the surface, and the
  highlighted row is the selection. The store search and Detail light no tab.
- The legend is one line of `styles.KeyHint` values through `styles.KeyLegend`, which drops the
  hints that do not fit from the end. Order a surface's hints in `footerHints` by how much an
  operator needs them. A shell action with no button, such as creating an issue, is named there.
  The workspace height is measured from the rendered chrome (`Model.workspaceSize`), so none of
  the four lines may wrap. `Model.renderSurface` cuts the body to that height: a renderer keeps a
  floor of rows, and a frame taller than the terminal loses its top lines and puts every click a
  row off.

### The tabs

- The tabs are the two views in `mode.BrowseModes` order — Board, Docs. Detail never appears
  there: it is a drill-in, not a tab. Nor does [the store search](#the-store-search), and no
  tab is drawn active while it is up.
- The active tab is `ShellTabActiveTextColor` on `ShellTabActiveBgColor` and bold; the rest are
  `ShellTabInactiveColor`. Tabs and buttons are the two surfaces whose state rides a background — on
  a pane or a column it rides the border instead.
- A new browse surface is one entry in `mode.BrowseModes`, one arm in `Model.browseController`, and
  one label in `tabLabels` (`internal/app/render.go`). The controller must satisfy `mode.Browse`;
  registering it there is what wires forwarding, sizing, loading state and auto-refresh at once.
  Adding it anywhere else puts the strip and the cycle order out of step.
- `tab` / `shift+tab` belong to the tabs everywhere except inside a modal, which consumes keys
  before the shell sees them. From the store search they leave it for a tab. A browse surface
  must not claim either key.

## Keys and the query line

Board, Docs and the store search each hold one `mode.Query` (`internal/mode/query.go`) and draw it
on the line above their columns, under a rule. It is always live: no key enters it and nothing
focuses it.

- **No action is bound to a printable key outside a modal.** `mode.IsQueryKey` names the keys a
  query takes — every rune key and `space` without alt, `backspace`, `ctrl+w`, `ctrl+u` — and the
  surface offers each key to `Query.HandleKey` before its bindings. `handleShellKey` asks the
  surface on screen (`Browse.TakesKey`), so a key it took runs no shell action. Bind a new action
  to an `alt+` chord or a key that prints nothing; `config.ResolveKeyBindings` refuses anything
  else ([CONFIGURATION.md](CONFIGURATION.md#keybindings)).
- A built-in key that prints nothing — `backspace`, `ctrl+w`, `ctrl+u`, the scope key of the store
  search — is also in `reservedKeys` (`internal/config/keybindings.go`), which refuses it in the
  `shell` and `board` contexts. `TestReservedKeysOfTheConfigAreTheKeysTheSurfacesTake`
  (`internal/mode/search/model_test.go`) fails while that list and the surfaces disagree.
- The startup-error screen (`internal/ui/fatalerror`) is the one exception: it quits on `q` as well
  as on the quit key. It has no query line and no other action, so the key can type into nothing.
- The query has no cursor and is edited at its end only, so the arrow keys stay with the list.
  `mode.QueryLimit` caps it.
- On a tab or the store search, Escape clears a non-empty query before it leaves the surface or
  hides a toast: the shell calls `Browse.ClearQuery` and acts on Escape itself only when that
  reports nothing cleared. One thing comes first: an Escape that arrives while a dialog is still
  loading cancels that dialog and leaves the query as it is (`pendingDialog`, `handleShellKey`).
- A tab's query is a filter over the rows in memory: `Query.Filter` keeps the rows where every word
  is in the title or the ID, in their order. The model keeps the loaded rows and the matching rows
  apart (`issues` and `shown` in `internal/mode/board/model.go`); selection, scroll, hit test and
  `View` read the matching rows, paging reads the loaded ones.
- When the query changes, a list keeps its selection on the same issue while that issue still
  matches and otherwise takes the first match. A focused board column left with no match gives the
  focus to the first column that has one and takes it back with its first match, unless the
  operator moved the focus meanwhile (`queryChanged`, `queryHome`).
- The query outlives a reload, an auto refresh, a tab switch and a detail round trip. It ends with
  the model, at a store switch.
- `board.Render` draws the list head (`renderHead`, `internal/ui/board/board.go`), two lines: a
  rule from `styles.Rule`, which also draws the rule under the menu bar, and the query line, indented
  one cell so the prompt stands under the first menu-bar button. `HitTest` reports no column on
  either. The query line (`renderQueryLine`, `internal/ui/board/query.go`) comes from
  `State.Query` and `State.Placeholder`: `Glyphs.Prompt`, the text, then a cursor block, the prompt
  and the cursor in `QueryAccentColor`. An empty query shows the placeholder in `TextMutedColor`,
  so the line says what a key press does. A text wider than the line loses its front. A board too
  narrow for all its columns names the drawn ones at the right end of the same line.
- `issuerow.RenderCompact` marks every occurrence of a query word in the title and the ID with
  `styles.MatchTextStyle` (`RenderConfig.Match`). It marks the text after the cut to width, so a
  mark adds no cell.
- The legend names the typing itself — `type` → `filter` — because no key opens the filter.

### The store search

The store search (`internal/mode/search`) is a browse surface in everything but the tab line. It
satisfies `mode.Browse` and is registered in `browseSurfaces` (`internal/app/routing.go`), which is
what gives it forwarding, sizing, the header spinner, auto refresh and a rebuild at a store switch. It
is absent from `mode.BrowseModes`, so the tab cycle never lands on it and `lastBrowse` never names
it.

- A shell action opens it — `open_search`, a menu-bar button — from a tab or from Detail
  (`Model.openSearch`). `Model.searchFrom` holds the surface it was opened from and is empty while
  the search is not up.
- While it is up it owns the selection, also under a Detail opened from it: `currentSelection`
  reads the search row, and Escape in that Detail returns to the results as they were left.
- Its query is not matched in memory. An edit waits `editPause`, and one `Repository.Search` runs
  for the text as it is when the pause ends (`Model.searchAfterPause`), so a typed word costs one
  search. The pause and each search carry a generation, and an older one is dropped. The scope key
  and Escape search at once and replace a waiting edit; a reload and an auto refresh are dropped
  during the pause, as they are during a search. `State.Search` tells the renderer that the
  store matched the rows: the header then counts as a column without a query does.
- Enter, or the second click on a row, while a search is in flight is held (`Model.openDetail`,
  `heldOpen`): the rows on screen answer an older query, or an auto refresh is about to move the
  selection off an issue that left the result. The pause of an edit counts as in flight. The
  detail opens on the selected row of the newest result. Any later key, click or wheel notch drops
  the held Enter, and so does the pointer leaving the surface. An empty or failed result opens
  nothing, and the opening search, which has no rows yet, holds nothing.
- `ctrl+t` toggles the scope between open issues and all of them (`isScopeKey` in
  `internal/mode/search/model.go`). It is built in, as the query keys are, and the column title
  names the scope.
- Escape clears a non-empty query, and with an empty one returns to `searchFrom`
  (`Model.closeSearch`).
- Only the first search draws the column as loading. A search runs after every edit, and dimming
  the rows for each one would flicker them.

## Surfaces above the shell

The store picker (`internal/mode/storepicker`, `internal/ui/storepicker`) and the configuration
screen (`internal/mode/configscreen`, `internal/ui/configscreen`) are neither tabs nor drill-ins:
each renders **instead of** the shell, as `fatalerror` does, so the shell chrome is absent while
it is up and it draws its own key legend in the footer's place (`Model.renderSurface`). Both are
therefore absent from `mode.BrowseModes` and never appear in the tab cycle.

A surface above the shell takes keys before the shell key switch and reports whether it consumed
each one — `Model.HandleKey` returns `(consumed, cmd)` — so Escape, quit and help keep working
without it re-implementing them. Escape returns to the mode it was opened from, including Detail.
The shell actions that act on the selected issue are inert there: the picker has no issue
selection, and the answer `currentSelection()` would give is a row that is not on screen.

Opening a store from the picker lands on the new store's Board, whatever mode the picker was
opened from — the previous store's Detail and selection are gone.

The picker is also the start screen when no store resolves. With no store open there is nothing
below it, so the operator is held there: Escape quits, quit and help work, and every other shell
key is inert until a store is opened.

The configuration screen opens from a tab, the store search and Detail (`Model.openConfig`), and
not from the picker: with no store open the picker has nothing below it.

- It has one section, Appearance, with a row for the theme and one for the glyph set. The
  section title is bold in `SectionHeadingColor` with a `styles.Rule` to the edge
  (`renderHeading`, `internal/ui/configscreen/configscreen.go`), so a section reads as a block.
  Each row is the selection gutter, the label in `SettingLabelColor` padded to `labelWidth`, and
  the value between `Glyphs.StepPrev` and `Glyphs.StepNext`: the markers say the value is
  stepped, not typed.
- `move_up` and `move_down` of the board context move the cursor, clamped. `left`, `right` and
  `enter` step the value through `styles.Themes()` or `styles.GlyphSets()` and wrap at both ends.
  The three are matched as the keys themselves, without Alt, so an `alt+` chord stays a shell key.
- Of the shell key switch it reaches only Escape, quit and help (`handleShellKey`). Every other
  shell action is inert there, the tab cycle, the store picker, the search and reload included:
  nothing below the screen is drawn. This is stricter than the picker.
- The screen changes nothing itself. A step emits `configscreen.ChangeMsg` with both values, and
  the shell validates, writes the config file, applies and then calls the screen's `SetValues`,
  so a write that fails leaves the screen showing what is drawn
  ([CONFIGURATION.md](CONFIGURATION.md#the-configuration-screen-writes-the-file)).
- The first line names the file a change is written to. A path too long for the line loses its
  front, so the file name stays.
- A frame too short for the section loses its first lines, as far as it takes to draw the
  selected row (`Render`, `internal/ui/configscreen/configscreen.go`).
- A selection that lands under the screen starts no detail load; Escape starts it
  (`ensureDetailForCurrentSelectionCmd`), so the Detail below shows the issue its keys act on.
- It takes no mouse event. The bar button opens it and the keys work it.

When nothing resolved for the working directory, the picker offers to create a store there as two
action rows above the registry — `Row.Action` in `internal/ui/storepicker`. An action is a row, not
a key: it costs no binding and no config surface, and it disappears once the directory has a
store. The header count counts stores only. The form rides the shell's action-modal slot and stays
open until the store is created, so a rejected name or prefix is corrected in place.

## Selection and scrolling

- Take the gutter from `styles.SelectionPrefix(selected, styled)`. It returns both variants: use
  `plain` for width math and truncation, `rendered` for output. Deriving one from the other by
  stripping escapes is what the two return values exist to prevent.
- A selected row also carries a band: `styles.RowHighlight(row, width, selected, hovered)` lays
  `RowSelectedBgColor` under the whole row, or the quieter `RowHoverBgColor` under the row the
  pointer is on, and the selection's on a row that is both. `issuerow` applies it; a list that
  renders its own rows calls it on each line of the finished row, as `ui/storepicker` does. The
  selection bar stays with it, on every line of the row: a terminal without colour draws no band.
- A move that changes the selection calls `scroll.EnsureVisible(offset, sel, window)` — or
  `scroll.EnsureVisibleClipped(offset, sel, window, total)` when the pane spends its first and last
  rows on `… (N earlier)` / `… (N more)` indicators, as `ui/detail` does. The selection bar staying
  on a row that actually renders is a contract; `EnsureVisible` in a clipped pane satisfies the
  window check and hides the bar. Both count rows of one line; a list of two-line rows passes its
  capacity in rows, as `mode/storepicker` does with `RowCapacity`.
- A column ordered by last change (`Column.AgeMarkers`) draws a muted divider before the first
  issue older than a day and another before the first older than a week, each carrying the count
  of issues below it (`internal/ui/board/agemarker.go`). A divider is a row, not an issue:
  `ScrollOffset` and `SelectedRow` stay issue indices. A mode model that draws through `ui/board`
  takes its offset from `board.EnsureVisible`, bounded by `board.MaxOffset`, not from
  `scroll.EnsureVisible` — the two count the lines the renderer draws, and reserve the inline
  error row and the dividers between the offset and the selection, and the renderer slides the
  window itself when a divider appears between the key press and the draw. Done keeps the
  backend's close-date order and draws none.
- A header reads a plain `N` only when the whole list is loaded and fits. A clipped window or a
  paginated column (`TotalIsExact` false, or a load-more in flight) reads `N of M`; a skeleton pane
  reads `issuerow.SkeletonGlyph`. While a filter is active the header reads `N of M` for another
  pair: the matching rows of the loaded rows (`Column.Loaded`). `internal/ui/board/board.go` holds
  the board's, which the docs tab and the store search share, and `internal/ui/detail/details.go`
  the detail panes'.

## The mouse

The mouse repeats what a key already does; it adds no behaviour of its own and no config surface.

- `Model.handleMouse` (`internal/app/mouse.go`) is the only reader of `tea.MouseMsg`. It routes in
  the keyboard's order — overlay, surface above the shell, header, active surface — and hands the
  surface a `mode.MouseMsg` in that surface's own coordinates. A mode never sees the raw event.
- An open overlay takes the event and the surface below gets a `mode.MouseLeave`. Help scrolls under
  the wheel, as it does on the detail scroll keys (`Model.scrollHelp`); a dialog ignores the mouse.
- A surface answers "what is drawn at this cell" with a pure `HitTest(state, x, y)` beside its
  `Render`, built from the same layout helpers. A mode model builds one state value for both — its
  `viewState` — so a click cannot land on a row other than the one drawn under it.
- Test a `HitTest` against the renderer, not against arithmetic: `testui.FindCell` finds where
  `Render` drew a text, and the test asserts `HitTest` reports that row there.
- An open request names its row: a surface builds it with `mode.RequestOpenDetailCmd` from the
  selection it holds, and the shell adopts that selection (`Model.adoptSelection`) before it opens
  the detail. The request so does not depend on the `SelectionChangedMsg` of that row arriving
  first.
- One click selects a row and a second opens it. Take the decision from `mode.ClickTracker`: a
  second click on the same cell opens the row the first one selected, because selecting a row drawn
  with only its first line scrolls the list and slides another row under the pointer.
- The wheel moves a list's selection one row a notch — on the board the selection of the column
  under the pointer, which takes the focus — and scrolls a pane of text.
- A focus change must not move what is under the pointer. A board too narrow for all its columns
  keeps the drawn ones in place until the focus leaves them, and then moves only as far as it takes
  to draw the focused column (`board.ColumnStart`). The mode model owns the window start and
  passes it in `State.ColumnStart`.
- Hover is derived on every draw from the stored pointer cell, never stored as a row, so a row that
  scrolls or reloads under a still pointer is the one marked. It draws as the quieter row band
  (Selection and scrolling); a hovered tab or menu-bar button takes `ShellTabHoverColor`.
- A click on a menu-bar button runs the method its key runs (`Model.mouseOnHeader`).
- The configuration screen takes no mouse event: `handleMouse` returns before the header and the
  surface are measured. Text selection still works there, because it runs before the routing.
- The program runs with `tea.WithMouseAllMotion()`, which stops the terminal's own drag-select, so
  the shell selects text itself (`internal/app/textselect.go`): a drag of the left button draws a
  reverse-video box over the screen as it was when the drag began, and the release sends the box
  to the clipboard with OSC 52. The toast says sent, not copied: OSC 52 has no reply. While the box is up every other mouse event and every key
  but Escape and quit is dropped — each would change the screen the box stands over.

## Overlays

- Place an overlay with `overlay.Place` — it is ANSI-aware, splicing the foreground into the
  background line by line while preserving the escapes on both sides. Lip Gloss's own placement
  helpers corrupt already-rendered colour, so they are not an alternative here.
- A modal is centred (`overlay.Center`); a toast is bottom-centred with `PadY: 1`
  (`overlay.Bottom`).
- A toast carries an identity: `toaster.Model.Show` bumps `seq`, and a scheduled `DismissMsg` carries
  the `seq` it was scheduled for. Compare it on receipt, so a stale timer cannot dismiss the toast
  that replaced it.

## Width and height

- Measure rendered width with `lipgloss.Width`, never `len` — a styled string carries escape bytes
  and a wide rune covers two cells.
- Measure and cut text with `internal/ui/shared/textutil` — `TruncateString`,
  `TruncateStringFront`, `WrapLines`, `PadToWidth`, `StripANSI`, `Clamp`. Each is ANSI-aware; the `strings` equivalents are not.
  `styles` owns colour and chrome, not text math.
- A string the app did not write — a directory name, a path, an error text that embeds one — is
  cleaned with `internal/displaytext` before it is drawn. A newline in it adds a line to the frame
  and puts every click below a row off; an escape sequence runs in the terminal; a bidirectional
  override reorders the cells beside it. The value a file or store operation uses stays raw, so
  the cleaning is at the places such a text passes on its way to the screen:
  - where a display-only value enters: `StoreName`, and the path and detail of a catalog entry
    (`internal/storecatalog/taskmgr`);
  - `Model.showToast`, with `displaytext.Lines`, because a toast may hold line feeds the app wrote;
  - `Model.openStoreForm`, the configuration screen's `Model.Open`, and the store picker's
    `renderRow` and `renderError`, each of which draws a value that is also a key;
  - the error row of a board column and the error text of the detail view.

  A new place that draws such a string calls `displaytext.OneLine` itself. The log keeps the raw
  value ([MONITORING.md](MONITORING.md)).
- `renderhelpers.CompactIssueID` shortens an ID from the front (`…` + tail) after first dropping the
  `task-manager-ui-` prefix, because the distinguishing part of an issue ID is its tail.

## Loading feedback

- Long work renders the spinner: advance the frame with `loading.NextFrame`, draw it with
  `loading.Glyph`, drive it with `loading.SpinnerTickCmd`.
- The shell says work is in flight with the spinner cell in front of the tabs
  (`Model.headerSpinnerCell`), drawn while `Model.workInFlight` reports a surface loading; idle draws nothing.
- A cold start draws skeleton rows (`issuerow.RenderCompactSkeleton`) rather than an empty frame.
  Their shade cycles through `styles.SkeletonShades` on the phase from `loading.SkeletonPhase`, which
  advances every 4 spinner frames for a ~1.2 s pulse.
- Launch success and failure both reach the operator as a toast; a launcher never fails silently.

## Text and markdown

- Comments render newest-first, against the backend's oldest-first default, and the header says so:
  `Comments (N · newest first)`.
- Markdown on a read-only surface goes through `markdown.Renderer.RenderReadOnly`, which degrades
  deterministically: empty input to `(no content)`, plain text and any renderer failure to plain
  wrapped text.
