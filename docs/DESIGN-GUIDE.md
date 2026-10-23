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
- After an `Apply` of another theme or glyph set under a running program, clamp the scroll
  offsets as a resize does (`applyWorkspaceSizeToBrowseModes`, `detail.ClampScroll`): a light
  theme renders markdown in another glamour style and a glyph set draws tokens of another width.
  An `Apply` of another colour profile moves no width and needs no clamp.
- A test draws with `catppuccin-mocha` and the `unicode` glyphs, which the package starts on; one
  that applies another restores it in `t.Cleanup` and stays out of `t.Parallel`.
- The roles are grouped by what they mean, not by hue: text (`TextPrimaryColor`, `TextMutedColor`,
  `TextSecondaryColor`), shell chrome (`ShellTab*`, `ShellAction*`, `ShellHoverBgColor`,
  `ShellRuleColor`, `ShellFooterHelpColor`), the query line and its matches (`QueryAccentColor`,
  `MatchTextColor`), the full screens (`ScreenSubtitleColor`, `SectionHeadingColor`,
  `SettingLabelColor` on the configuration screen and the issue editor, and `ErrorTextColor` for
  the refusal of a save on the issue editor),
  borders and overlays (`BorderDefaultColor`, `OverlayBorderColor`, `BorderHighlightFocusColor`),
  buttons (primary / secondary / danger, each with a `Focus` variant), toasts
  (`ToastBorder{Success,Error,Info,Warn}Color`), and the issue vocabulary below.
- Focus on a pane or a column is `BorderHighlightFocusColor` on the border. Only the tabs, the
  modal buttons and the Save and Cancel buttons of the issue editor carry focus on a background
  instead, each with its own `Focus` role. A row's
  background is not focus: it is the selection or the pointer (`RowSelectedBgColor`,
  `RowHoverBgColor`).
- The pointer lights what is under it one surface below the selection: on a true-colour terminal
  `RowHoverBgColor` under a row and `ShellHoverBgColor` under a tab or a menu-bar button are
  Surface0, and `RowSelectedBgColor` is Surface1, so both stand apart from the terminal
  background: lighter on a dark flavour, darker on latte.
- On 256 colours both hover roles are the grey of the ramp nearest to Surface0 (`greyRamp`,
  `internal/ui/styles/theme.go`), so the band does not depend on the palette entry the colour
  conversion picks for Surface0. On a dark flavour no other role is drawn in that entry.
- The 16 colours have no entry between the background and the selection, so both hover roles are
  empty there and draw no band. What is left under the pointer is the bold label of a button and
  the text colour of a tab; a row shows nothing.
- `styles.Apply` takes the colour profile for the two cases. `cmd/taskmgr-ui` reads the terminal's
  profile with `colorprofile.Detect` and passes it there and in `app.RuntimeOptions.ColorProfile`,
  which the configuration screen applies a theme on. The program detects the profile again when it
  starts; `Model.update` re-applies the theme when its `tea.ColorProfileMsg` reports another one.
- In true colour and on 256 colours the hover band is neither the selection band nor the entry of
  Base, which a row at rest stands on. On every profile a hovered tab and a hovered button differ
  from the ones at rest, and the key of a hovered button from its background
  (`TestHoverRolesStayDistinctOnEveryTerminal`, `TestTheKeyOfAHoveredButtonIsReadableOnEveryTerminal`,
  `internal/ui/styles/row_highlight_test.go`).

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
| `‹ ›` | either side of a value the operator steps through, on the configuration screen and the issue editor | `StepPrev`, `StepNext` |
| `✅ ❌ ℹ ⚠` | toast severity | `ToastSuccess` and its siblings |
| the issue tokens above | type, priority, status | `IssueType`, `Priority`, `Status` |

- A new marker is a field of `GlyphSet` with a value in all three sets. A row marker that is not a
  letter token is one cell wide; a toast glyph may be two, and `glyphCells`
  (`internal/ui/toaster/toaster.go`) budgets for the wider. A spinner's frame count divides ten,
  the length of the shell's frame counter.

The rest is the same in every set, and each has one definition:

| Glyph | Means | Defined in |
|---|---|---|
| `…` | truncated content — one cell, so it keeps more text than `...` | `textutil.TruncateString`, `textutil.TruncateStringFront`; `legendCut` (`internal/ui/styles/legend.go`) where the legend drops hints; `capLines` (`internal/ui/toaster/toaster.go`) where a toast drops lines; `errorLines` (`internal/ui/issueedit/issueedit.go`) where the refusal of a save drops lines |
| `╭ ╮ ╰ ╯ ─ │` | a section border (a modal or toast frames itself with `lipgloss.RoundedBorder()`) | `styles.FormSection` |
| `░` | skeleton loading bar | `issuerow.SkeletonGlyph` |
| `├─ └─ │` | comment output tree | `internal/ui/detail/comments.go` |
| `─` in `BorderDefaultColor` | the rule under the Content header and between two comments | `contentHeaderRule` in `internal/ui/detail/details.go` |
| `• ` | a metadata list item | `internal/ui/detail/metadata.go` |
| ` • ` | separator between legend hints | `legendSeparator` in `internal/ui/styles/legend.go` |
| `·` | separator between fields, between menu-bar buttons and between the buttons in a pane | inline at the call site; `buttonSeparator` in `internal/ui/detail/buttons.go` |

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
- Draw a full screen that is text and not a list — help, the configuration screen, the issue
  editor — with
  `styles.Screen` (`internal/ui/styles/screen.go`), not with a box. It owns six things:
  - the title line in the menu bar's place: one space, the title bold in `TextPrimaryColor`, and
    the build version in `ShellFooterHelpColor` flush right, left off when fewer than two cells
    separate the two;
  - the `styles.InsetRule` the shell has under its bar, on the same cells, so the rule does not
    move when the screen opens;
  - the subtitle, one cell in, in `ScreenSubtitleColor`: one line that says what the body is;
  - a `styles.Rule` under the subtitle, from the screen's first cell to its last;
  - the body, `styles.ScreenBodyRows(height)` rows of it. A line wider than the screen ends at the
    last cell with no `…`: a renderer wraps the line or cuts it with the mark itself;
  - the key legend on the last line, one cell in. Build it with `styles.KeyLegend` at
    `styles.ScreenTextWidth(width)`.
- Open a section of such a screen with `styles.SectionHeading`: the title bold in
  `SectionHeadingColor`, a space, and a `styles.Rule` to the width passed. Put a blank line between
  two sections.
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

### A button in a pane

A menu-bar button is about the surface; a button in a pane is about what that pane shows. Detail
has four, all in the Content pane: `edit` on the ID line, the first line of the pane, `add` on the
Comments heading, and `edit` and `delete` on the header line of the selected comment.

- Draw it with `paneButton` (`internal/ui/detail/buttons.go`): the two runs and the roles of a
  menu-bar button, `" label "` in `ShellActionColor` and `"key "` in `ShellFooterHelpColor`.
- It shows the key that does the same, read from the binding; `enter` on a comment is built in
  and spelled out. The button is a second way to reach the action, never the only one.
- Under the pointer both runs take `ShellHoverBgColor` and the label turns bold, as on the bar
  (`State.Hover`).
- Put buttons at the right end of a line with `placeButtons`, joined by `buttonSeparator`, so
  the last cell of every button line is the same column: `edit` on the ID line ends where `add`
  ends (`TestTheEditButtonEndsInTheColumnOfTheAddButton`, `internal/ui/detail/comments_test.go`).
  It draws none when fewer than `buttonTextFloor` cells would stay for the text. The ID is never
  cut for a button: an ID line without room for both draws no `edit` (`renderContentPaneLines`).
- `HitTest` reads the cells `placeButtons` returned, through `renderContentPaneLines`, so a click
  lands on the button that is drawn.
- A pane border holds no button.
- A pane in skeleton state draws no button and reports none.

## The screen margin

Everything the app draws is the **screen**, and the screen stands inside a margin of blank cells:
2 columns on each side and 1 row above and below. The margin fills a cell at a time and each
direction by itself, so the screen is never narrower nor shorter on a terminal that is wider or
taller:

| Terminal columns | Margin, left + right | Screen columns |
|---|---|---|
| up to 100 (`marginMinWidth`) | none | the terminal's |
| 101 to 104 | 1 + 0, 1 + 1, 2 + 1, 2 + 2 | 100 |
| 105 or more | 2 + 2 | the terminal's less 4 |

| Terminal rows | Margin, top + bottom | Screen rows |
|---|---|---|
| up to 24 (`marginMinHeight`) | none | the terminal's |
| 25, 26 | 1 + 0, 1 + 1 | 24 |
| 27 or more | 1 + 1 | the terminal's less 2 |

- The rows do not wait for a side column: a terminal of 80 columns and 30 rows has a blank row
  above and below and none at the sides. Rows that came with the first side column made the
  screen 2 rows shorter at 101 columns than at 100.
- `Model.setTerminalSize` (`internal/app/render.go`) is the only place the terminal's size is
  acted on. It splits each direction with `splitMargin` into `Model.marginLeft`, `marginTop`,
  `marginBottom` and the screen, `Model.width` and `Model.height`. The model does not hold the
  terminal size. `TestTheScreenNeverShrinksWhenTheTerminalGrows` (`internal/app/margin_test.go`)
  holds both sizes against a terminal a column narrower and a row shorter.
- No surface acts on a `tea.WindowSizeMsg`: `shouldForwardTo` (`internal/app/routing.go`) keeps it
  from the tabs, and the shell sizes the modal, the picker, the configuration screen and the issue
  editor with `SetSize`. An open dialog is handed the message with every message the shell does
  not consume, and has no arm for it. The board, docs, search and store picker models still have
  an arm for the message; the shell never feeds it.
- Size every surface, overlay and toast from `Model.width` and `Model.height`. `Model.screen` draws
  them all for that size, and `Model.frame` is the only place the margin is drawn.
- `Model.update` turns a `tea.MouseMsg` into a `pointer` (`internal/app/mouse.go`) and moves it
  into screen coordinates once, before `mouseHeld` and `handleMouse`. Count every cell after that
  from the screen's first cell; a cell of the margin is outside every hit test.
- A line wider than the screen runs through the margin and off the terminal, so a renderer cuts or
  wraps to the width it is given — `fatalerror.Render` and `helpscreen.Render` wrap for that reason.
  `TestEverySurfaceIsDrawnInsideTheMargin` (`internal/app/margin_test.go`) holds each surface to it.

## The shell chrome

The shell frames the workspace with four lines, three while Detail is open, all drawn in
`internal/app/render.go`: the menu bar, the rule under it and the tab line (`Model.renderHeader`),
and the key legend below (`Model.renderFooter`).

- The menu bar holds the buttons from the left edge and the build version flush right. A button is
  a shell action that is not about the selected row, and it shows the key bound to that action: the
  bar is a second way to reach it, never the only one. Add one as an entry in `barActions`, with
  the method the key switch in `handleShellKey` also calls.
- A button is two runs, `" label "` in `ShellActionColor`, which is the text colour, and `"key "`
  in `ShellFooterHelpColor` (`barCell.runs`), so it has a space on each side, and two buttons are
  joined by `barSeparator`.
  The first button's leading space is the line's gutter: its label starts in the screen's second
  cell. The cell with both spaces is what the pointer lights and what a click takes
  (`barCell.covers`); a separator cell runs nothing.
- Under the pointer the whole cell takes `ShellHoverBgColor` and the label turns bold
  (`Model.renderMenuBar`). No foreground changes.
- The rule under the bar is `styles.InsetRule`: it starts one cell in, under the first button's
  label, and ends at the screen's edge.
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
  highlighted row is the selection. The store search lights no tab.
- Detail does not draw the tab line: it is a drill-in and would light no tab, so the body has the
  row. The spinner stands there in the menu bar's first cell, the gutter in front of the store
  button's label, which is the column it has on the tab line (`Model.renderMenuBar`). The glyph
  is one cell, so no button moves and `barCell.covers` still holds.
- Size a surface from its own header height, `browseHeaderRows` or `detailHeaderRows`, never from
  the header on screen: a resize in Detail must not give the board the row Detail has more
  (`applyWorkspaceSizeToBrowseModes`, `detailViewportHeight`). `Model.workspaceSize` stays the
  measure of the surface on screen.
- The legend is one line of `styles.KeyHint` values through `styles.KeyLegend`, one cell in
  (`Model.renderFooter`). A key is `ShellFooterHelpColor`, what it does `TextPrimaryColor`, and
  the separator `ShellRuleColor`: the key is the dim run. `KeyLegend` drops the hints that do not
  fit from the end and marks the drop with ` …` where that fits. Order a surface's hints in
  `footerHints` by how much an operator needs them. A shell action with no button, such as creating an issue, is named there.
  The workspace height is measured from the rendered chrome (`Model.workspaceSize`), so none of
  the four lines may wrap. `Model.renderSurface` cuts the body to that height: a renderer keeps a
  floor of rows, and a frame taller than the screen loses its top lines and puts every click a
  row off.

### The tabs

- The tabs are the two views in `mode.BrowseModes` order — Board, Docs. Detail never appears
  there: it is a drill-in, not a tab. Nor does [the store search](#the-store-search), and no
  tab is drawn active while it is up.
- The active tab is `ShellTabActiveTextColor` on `ShellTabActiveBgColor` and bold; the rest are
  `ShellTabInactiveColor`. The tab under the pointer is `ShellTabHoverColor` on `ShellHoverBgColor`,
  its two padding cells included: the background a hovered button takes (`Model.renderTabs`). Tabs
  and buttons are the two surfaces whose state rides a background — on a pane or a column it rides
  the border instead.
- A new browse surface is one entry in `mode.BrowseModes`, one arm in `Model.browseController`, and
  one label in `tabLabels` (`internal/app/render.go`). The controller must satisfy `mode.Browse`;
  registering it there is what wires forwarding, sizing, loading state and auto-refresh at once.
  Adding it anywhere else puts the strip and the cycle order out of step.
- `tab` / `shift+tab` belong to the tabs everywhere except inside a modal and the issue editor,
  each of which takes keys before the shell switch and moves between its fields with them. From
  the store search and from Detail they leave it for a tab, although Detail does not draw the
  strip. A browse surface must not claim either key.

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
- **A full screen that takes typed text** — the issue editor is the one — takes every key before
  the shell switch and gives back only quit, help and Escape (`HandleKey`,
  `internal/mode/issueedit/model.go`):
  - a rune key without Alt is text. The bubbles widgets insert the runes of an `alt+` chord too,
    so pass a rune key to a widget only when its `Mod` does not contain `tea.ModAlt`;
  - the keys that move between the fields are matched as the keys themselves, as on the
    configuration screen. They do not read the `modal` context: that context allows a printable
    key, and a printable key here is text;
  - an action of the screen is a key that prints nothing, bound in the `shell` context.
    `save_edit` is the one, and it has no case in the shell switch, so it does nothing elsewhere;
  - bind no widget key to the system clipboard (`KeyMap.Paste` is unbound). The terminal's own
    paste arrives as a `tea.PasteMsg`, which `HandlePaste` types into the focused field, and so
    does a key message that carries more than one rune.
- The query has no cursor and is edited at its end only, so the arrow keys stay with the list.
  `mode.QueryLimit` caps it.
- A paste (`tea.PasteMsg`) goes where a typed key goes: the active query (`Query.Paste`), an open
  dialog's input, or the issue editor. Help and a drag selection drop it, as they drop a key.
- On a tab or the store search, Escape clears a non-empty query before it leaves the surface or
  hides a toast: the shell calls `Browse.ClearQuery` and acts on Escape itself only when that
  reports nothing cleared. One thing comes first: an Escape that arrives while the issue editor is
  still opening cancels that open and leaves the query as it is (`pendingEditor`, `handleShellKey`).
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
  rule from `styles.Rule`, across the whole width, and the query line, indented
  one cell so the prompt stands under the label of the first menu-bar button. `HitTest` reports no column on
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

The store picker (`internal/mode/storepicker`, `internal/ui/storepicker`), the configuration
screen (`internal/mode/configscreen`, `internal/ui/configscreen`), the issue editor
(`internal/mode/issueedit`, `internal/ui/issueedit`) and help
(`internal/app/help.go`, `internal/ui/helpscreen`) are neither tabs nor drill-ins: each renders
**instead of** the shell, as `fatalerror` does, so the shell chrome is absent while it is up and
it draws its own key legend in the footer's place (`Model.renderSurface`). None is in
`mode.BrowseModes`, so none appears in the tab cycle. The configuration screen, the issue editor
and help are drawn in the chrome of `styles.Screen`
([Build from the shared chrome](#build-from-the-shared-chrome)); the picker keeps its list frame.

The picker, the configuration screen and the issue editor are modes. Each takes keys before the
shell key switch and reports whether it consumed each one — `Model.HandleKey` returns
`(consumed, cmd)` — so Escape, quit and help keep working without it re-implementing them. Escape returns to the mode it was opened from, including Detail.
The shell actions that act on the selected issue are inert there: the picker has no issue
selection, and the answer `currentSelection()` would give is a row that is not on screen.

Opening a store from the picker lands on the new store's Board, whatever mode the picker was
opened from — the previous store's Detail and selection are gone.

The picker is also the start screen when no store resolves. With no store open there is nothing
below it, so the operator is held there: Escape quits, quit and help work, and every other shell
key is inert until a store is opened.

The configuration screen opens from a tab, the store search and Detail (`Model.openConfig`), and
not from the picker: with no store open the picker has nothing below it.

- Its title is `Configuration`. It has one section, Appearance, under a `styles.SectionHeading`
  that starts in the screen's first cell and runs to the edge, with a row for the theme and one
  for the glyph set. Each row is the selection gutter, the label in `SettingLabelColor` padded to
  `labelWidth`, and the value between `Glyphs.StepPrev` and `Glyphs.StepNext`, all three one run
  in `TextPrimaryColor`: the markers say the value is stepped, not typed.
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
- The subtitle names the file a change is written to. A path too long for the line loses its
  front, so the file name stays.
- A screen too short for the body loses the body's first lines, as far as it takes to draw the
  selected row (`Render`, `internal/ui/configscreen/configscreen.go`).
- A selection that lands under the screen starts no detail load; Escape starts it
  (`ensureDetailForCurrentSelectionCmd`), so the Detail below shows the issue its keys act on.
- It takes no mouse event. The bar button opens it and the keys work it.

The issue editor opens from a tab, the store search and Detail, through the openers of
`internal/app/issue_editor.go` ([CODING.md](CODING.md)'s rule 7 lists them), and Escape returns to
the surface it was opened from (`Model.closeEditor`).

- Its title names what it writes — `New issue`, `Edit issue <id>`, `New comment on <id>`,
  `Edit comment on <id>` — over the issue's title as the subtitle. An issue has a row for each
  one-line field and the description under a `styles.SectionHeading`; a comment has the text
  alone.
- A row is the selection gutter, the label in `SettingLabelColor` padded to `labelWidth`, and the
  value. Status, type and priority are stepped between `Glyphs.StepPrev` and `Glyphs.StepNext`
  through the store's catalog, so a save can send only a value the store has; `left` and `right`
  step and wrap. The other rows are text inputs. A stepped value and a read-only one are cut
  with `…` at the cells an input has (`renderRow`).
- The Reason row is drawn only while the status is `closed`. On an issue that was closed before
  it is muted and takes no focus: the store takes a reason only when it closes.
- `tab` and `shift+tab` walk every field and the two buttons, and wrap. `up`, `down` and `enter`
  move between the rows; in the text `enter` is a newline and the arrows move the cursor.
- `up` on the first drawn line of the text moves the focus to the field above, and `down` on the
  last drawn line moves it to Save (`textCursorOnFirstLine`, `textDown`,
  `internal/mode/issueedit/model.go`). A wrapped line is several drawn lines. `LineInfo` counts
  an empty row under a line that fills the width, and the widget does not move the cursor down
  past that row: `textDown` leaves from the last line of the text only, and reaches the line
  under a full one from the end of that line. A comment has no field above its text, so `up` on
  its first line does nothing. The page keys and the wheel stay in the text.
- Save and Cancel stand under the text, each with the key that does the same (`save_edit`,
  Escape), and the focused one takes its `Focus` role. The focused row has the selection bar and
  no band.
- The text takes the rows the form leaves, and never fewer than `minTextRows`
  (`TextRows`, `internal/ui/issueedit/issueedit.go`). At 80x24 every field and both buttons are
  drawn at all times. A screen too short for the form shows a window of it that ends on the
  focused field (`window`), with no earlier or more mark, as on the configuration screen.
- A refused save stays on the screen: the editor keeps the typed values and draws the refusal in
  `ErrorTextColor` above the form, wrapped, `maxErrorLines` lines at most with `…` on the last.
  It does not scroll with the form. A short screen gives it fewer lines, as many as leave the
  text its heading and `minTextRows`, or one, so the focused field is still drawn (`errorLines`).
- Escape discards the typed text without a question. A save is in flight from the key or the
  click that asks for it (`submit`, `internal/mode/issueedit/model.go`), not from the moment the
  shell hears of it, so a second save key starts no second write. Until the result is back the
  button line reads `Saving…` and the screen takes no key but quit and help, Escape included,
  so the result always finds the typed values on screen.
- The screen writes nothing itself: Save emits `issueedit.SubmitMsg`, and the shell builds the
  write ([CODING.md](CODING.md)'s rule 5).
- A request to edit or close an issue reads the issue and the catalogs first, and one to create
  an issue reads the catalogs. Any key, click or wheel notch before the answer drops the request
  (`pendingEditor`), because each can move the selection the editor was asked for. A comment
  opens at once, with no read.
- The legend names `tab/shift+tab`, the save key, Escape, `left/right` on an issue, and quit
  (`editorHints`).

Help is not a mode: `Model.showHelp` puts it in the place of whatever surface is active, the
picker, the configuration screen and the issue editor included, and `Model.active` stays as it was.

- Its title is `Keyboard Help`, over the subtitle `every key taskmgr-ui answers to`.
  `helpSections` (`internal/app/help.go`) is the content: sections named by what their keys act
  on, each an entry per key, the dialogs, the configuration screen and the issue editor included. Add a key there as
  a `helpscreen.Entry`, with the key read from `config.ResolvedKeyBindings`; spell a key out only
  when no binding names it — the query's keys, the scope key of the store search, the pane keys
  and `enter` in Detail, `y` and `n` in a dialog, the keys that step a value on the configuration
  screen, the keys that move in the issue editor, the mouse. An entry names a key
  and what it does, nothing else.
- A new action in `internal/config/keybindings.go` takes an entry in `helpSections`:
  `TestHelpNamesEveryBoundAction` (`internal/app/help_screen_test.go`) fails without one. No test
  finds a key a surface matches as the key itself, so add its entry with the key.
- `helpscreen.Render` draws each section under a `styles.SectionHeading` that starts one cell in
  and stops one cell short of the edge. An entry is three spaces, the key in
  `ShellFooterHelpColor`, two spaces, and the description in `TextPrimaryColor`. Every section
  pads its keys to one column, the widest key on the screen, so the screen reads as one table.
- A description wider than its column goes on in that column on the next lines, broken at spaces
  with `textutil.WrapAtSpaces`, and stops one cell short of the edge. A column under
  `wrapMinWidth` cells keeps the entry on one line, cut at the edge.
- A body that does not start at the first line draws `scroll.Earlier` on its first row, and one
  that does not end at the last line `scroll.More` on its last row: the words of a clipped detail
  pane, uncoloured as there. Each counts the lines beyond the window, as there: not the one it stands on. A
  body under `clipMinRows` rows draws lines alone (`window`, `internal/ui/helpscreen/helpscreen.go`).
- The screen is drawn from the resolved bindings and the applied theme on every frame
  (`Model.renderHelp`), and the model holds only `helpOffset`. It therefore shows a rebound key
  and follows a theme change with no rebuild.
- `Model.helpKey` takes every key while help is up: `toggle_help` and the shell's Escape close it,
  the detail scroll keys scroll it, and every other key does nothing, quit included. It opens at
  the top (`Model.openHelp`); `scrollHelpBy` clamps the offset to `helpscreen.MaxOffset`, also
  after a resize. `MaxOffset` counts the wrapped lines at the screen's width, and a page key moves
  `helpscreen.PageRows`, the body less the two indicator rows.

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
- A selected comment is text, not a row: it takes the bar on each of its lines and no band. Every
  comment line starts with the 2-cell gutter whatever the selection, so the line count does not
  follow the cursor (`renderComments`, `internal/ui/detail/comments.go`). The bar is drawn only
  while the comments have the focus (`FocusPaneComments`).
- The Metadata pane of Detail only shows. It is no focus stop (`focusStops`,
  `internal/mode/detail/model.go`), so its border never takes the focus colour; `HitTest` reports
  nothing on it; it has no cursor and no scroll offset. A pane too short for its lines ends on
  `… (N more)` (`RenderMetadataPane`). Every action on the issue is a key and a button of the
  Content pane ([A button in a pane](#a-button-in-a-pane)).
- A move of the comment cursor scrolls the Content pane with two calls, the last line of the
  comment and then its first: a comment that fits is drawn whole and a taller one starts at its
  header line (`scrollToComment`, `internal/mode/detail/model.go`).
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
The selection menu is the one exception: no key selects text, so no key reaches what it offers.

- `Model.handleMouse` (`internal/app/mouse.go`) routes every `pointer`, which arrives in
  screen coordinates ([The screen margin](#the-screen-margin)). It routes in the keyboard's
  order — help or a dialog, surface above the shell, header, active surface — and hands the
  surface a `mode.MouseMsg` in that surface's own coordinates. A mode never sees the raw event.
- Help or an open dialog takes the event and the surface below gets a `mode.MouseLeave`. Help
  scrolls three lines a wheel notch (`helpWheelLines`) and no more than `helpscreen.PageRows`, so a
  body of a few rows skips no line, as it scrolls on the detail scroll keys (`Model.scrollHelp`);
  a dialog ignores the mouse.
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
  (Selection and scrolling); a hovered tab or menu-bar button takes `ShellHoverBgColor` over its
  whole padded cell ([The shell chrome](#the-shell-chrome)).
- A click on a menu-bar button runs the method its key runs (`Model.mouseOnHeader`).
- A click on a button in a pane sends the request its key sends (`buttonActions`,
  `internal/mode/detail/mouse.go`). A click on a comment gives the comments the focus and puts
  the cursor on it; a second click does nothing more, and the click does not scroll the pane.
- The configuration screen takes no mouse event: `handleMouse` returns before the header and the
  surface are measured. Text selection still works there, because it runs before the routing.
- The issue editor takes the event at the same place, before the header is measured
  (`Model.HandleMouse`, `internal/mode/issueedit/model.go`): a click on a row or on the text
  gives it the focus, as `tab` does; a click on Save or Cancel does what `enter` does there; the
  wheel over the text moves its cursor three lines a notch. A read-only row takes no click, and
  nothing does during a save. It lights nothing under the pointer: a lit button there would show
  a focus that Enter does not act on.
- `Model.View` asks for `tea.MouseModeAllMotion`, which stops the terminal's own drag-select, so
  the shell selects text itself (`internal/app/textselect.go`): a drag of the left button draws a
  reverse-video box over the screen as it was when the drag began, and the release sends the box
  to the clipboard with OSC 52. A drag that runs into the margin stops the box at the screen's
  edge (`mouseHeld`). While the box is up every other mouse event and every key
  but Escape and quit is dropped — each would change the screen the box stands over. A resize
  ends the drag with no copy: the box stands over a screen of the old size.
- The drag freezes the screen without the toast (`Model.frozenScreen`), and `Model.screen` draws
  the toast live over the box and over the selection menu: its dismiss timer runs on, so a toast
  that came meanwhile would otherwise go unseen.
- The release opens the selection menu (`internal/app/selectmenu.go`, drawn by
  `internal/ui/selectmenu`) beside the release cell, over the frozen screen with the box still on
  it. `selectmenu.Place` moves it left of or above the cell where it would leave the screen.
  - Its title says sent, not copied: OSC 52 has no reply.
  - It offers a quote of the text in a new comment on the selected issue, a store search for the
    text on one line, and close. The quote goes to the issue its label names, taken when the menu
    opens. Quote and search read the text without the screen's chrome (`quotable`): the leading
    columns that hold a border, the gutter or a space on every line, and the trailing borders; the
    clipboard keeps the box as drawn. The search runs once, at once (`search.Model.SetQuery`).
    Quote and search are off on the picker, the
    configuration screen, the issue editor, help and under a dialog: each would leave the surface
    with work on it.
  - Up and down move over the entries that are on, Enter or a click runs one, Escape or a click
    outside closes it, and quit quits. A click outside does nothing else. Every other key, paste and mouse event is
    dropped while it is up, and a resize closes it.

## Overlays

- Place an overlay with `overlay.Place` — it is ANSI-aware, splicing the foreground into the
  background line by line while preserving the escapes on both sides. Lip Gloss's own placement
  helpers corrupt already-rendered colour, so they are not an alternative here.
- A modal is centred (`overlay.Center`); a toast is bottom-centred with `PadY: 1`
  (`overlay.Bottom`). Both are placed on the screen, so the margin stays blank around them.
- The action modal, the toast and the selection menu are the only overlays. Help is a full screen
  ([Surfaces above the shell](#surfaces-above-the-shell)).
- The action modal carries two things: the store form of the picker (`Model.openStoreForm`) and
  the yes or no before a comment is deleted (`Model.confirmCommentDelete`), which has no input,
  a `modal.ButtonDanger` confirm, and takes `y` and `n`. `handleOverlayMessage` tells them apart
  by `storeForm.kind`. A new field of an issue is a row of the issue editor, never a modal.
- A toast carries an identity: `toaster.Model.Show` bumps `seq`, and a scheduled `DismissMsg` carries
  the `seq` it was scheduled for. Compare it on receipt, so a stale timer cannot dismiss the toast
  that replaced it.
- A toast draws at most `maxMessageLines` lines (`internal/ui/toaster/toaster.go`): a line feed
  in a path or an error text is a line of the box. `capLines` gives the last of them to the cut
  mark, a line that holds only `…`.

## Width and height

- Measure rendered width with `lipgloss.Width`, never `len` — a styled string carries escape bytes
  and a wide rune covers two cells.
- Measure and cut text with `internal/ui/shared/textutil` — `TruncateString`,
  `TruncateStringFront`, `WrapLines`, `WrapAtSpaces`, `PadToWidth`, `StripANSI`, `Clamp`. Each is ANSI-aware; the `strings` equivalents are not.
  A body is the exception: it wraps in `internal/ui/shared/markdown` ([Text and markdown](#text-and-markdown)).
  `styles` owns colour and chrome, not text math.
- A string the app did not write — a directory name, a path, an error text that embeds one — is
  cleaned with `internal/displaytext` before it is drawn. A newline in it adds a line to the frame
  and puts every click below a row off; an escape sequence runs in the terminal; a bidirectional
  override reorders the cells beside it. The value a file or store operation uses stays raw, so
  the cleaning is at the places such a text passes on its way to the screen:
  - where a display-only value enters: `StoreName`, and the path and detail of a catalog entry
    (`internal/storecatalog/taskmgr`);
  - `Model.showToast`, with `displaytext.Lines`, because a toast may hold line feeds the app wrote.
    A caller that wraps the text first cleans it before the wrap, as `Model.applyConfigChange`
    does: a control character has no width until it is drawn as a space;
  - `Model.openStoreForm`, the configuration screen's `Model.Open` and the store picker's
    `renderRow`, each of which draws a value that is also a key; the author in the comment
    delete question (`Model.confirmCommentDelete`);
  - the error row of a board column, the error text of the detail view and the store picker's
    `renderError`;
  - every field of an issue, where it is drawn: `issuerow`, `renderhelpers.CompactIssueID` and
    the default branch of `renderhelpers.CompactIssueState` for a title, an ID and a status in a
    row; `Render`, `contentHeaderMetaRow`, `renderContentPaneLines`, `renderCompact`,
    `renderMetadataPrefixedLine` and `renderComments` in `internal/ui/detail`; the title, the
    subtitle, a stepped value and the error of the issue editor (`Render`,
    `internal/ui/issueedit/issueedit.go`). The text inside an editor widget is not cleaned: it
    is the value that is written;
  - the source of a body, in `markdown.Renderer.RenderReadOnly` and `markdown.RenderPlain`
    (`drawable`), which also expands a TAB to its 4-cell stop: a TAB measures as no cell and the
    terminal draws it to the next stop, so a line with one runs past its pane.

  A new place that draws such a string calls `displaytext.OneLine` itself. A log record is not
  cleaned ([MONITORING.md](MONITORING.md#what-a-healthy-run-logs-after-startup)).
- `renderhelpers.CompactIssueID` shortens an ID from the front (`…` + tail) after first dropping the
  `task-manager-ui-` prefix, because the distinguishing part of an issue ID is its tail.

## Loading feedback

- Long work renders the spinner: advance the frame with `loading.NextFrame`, draw it with
  `loading.Glyph`, drive it with `loading.SpinnerTickCmd`.
- The shell says work is in flight with the spinner cell in front of the tabs
  (`Model.headerSpinnerCell`), drawn while `Model.workInFlight` reports a surface loading; idle draws nothing.
  In Detail, which has no tab line, the glyph stands in the menu bar's first cell
  ([The shell chrome](#the-shell-chrome)).
- A cold start draws skeleton rows (`issuerow.RenderCompactSkeleton`) rather than an empty frame.
  Their shade cycles through `styles.SkeletonShades` on the phase from `loading.SkeletonPhase`, which
  advances every 4 spinner frames for a ~1.2 s pulse.
- Launch success and failure both reach the operator as a toast; a launcher never fails silently.

## Text and markdown

- Comments render newest-first, against the backend's oldest-first default, and the heading says so:
  `Comments (N · newest first)`. `detail.OrderedComments` is that order, for the renderer and for
  the model's cursor. An issue with no comment still draws the heading, as `Comments (0)`, because
  the `add` button stands on it.
- A comment starts with its author line, bold: `[i/N] author · time`. Two comments are separated
  by a blank line, `contentHeaderRule` and a blank line; the three lines belong to no comment.
- The issue editor's widgets hold a TAB as 4 spaces and CRLF as LF. `Model.Open` reads its
  baseline back from the widgets, so an untouched description is no change; a changed one is
  written as the widget holds it.
- Markdown on a read-only surface goes through `markdown.Renderer.RenderReadOnly`, which degrades
  deterministically: empty input to `(no content)`, plain text and any renderer failure to plain
  wrapped text.
- A line of a body is wrapped, never cut: a pane scrolls down only, so a cut line loses its end.
  Every line `RenderReadOnly` returns fits the width it was given, and the text is the same at
  every width. A body is shortened in two places, each with a mark on its last line:
  a comment (`applyCommentElision`) and the description of the compact view
  (`renderMarkdownPreviewLines`).
- glamour styles a body and wraps none of it (`renderMarkdownANSIMemoized`,
  `internal/ui/shared/markdown/renderer.go`). `wrapRenderedLine` wraps each line at spaces, breaks
  a word only when it is wider than the line, and starts a continuation line under the text of its
  first line, with the quote bars. Only a table wider than the pane comes from a glamour render
  at the width (`spliceTables`), because glamour sizes the columns. Where glamour cut a cell to
  make the table fit, the rows wrap as text instead.
- Text that is not markdown wraps through `markdown.RenderPlain`, which keeps the line breaks and
  the indent of the source. A log-like comment body is drawn with it.
