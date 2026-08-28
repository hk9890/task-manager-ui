# Design Guide

The interaction and rendering law for every surface under `internal/ui/` and `internal/mode/`. No
gate enforces it — [REVIEWING.md](REVIEWING.md) is where a change is held against it.

`internal/ui/` renders; `internal/mode/` owns feature-local state and emits shell contracts. A
renderer takes a state struct and returns a string — no package under `internal/ui/` reads a
repository. The three interactive primitives are the exception that carry Bubble Tea state of their
own: `modal`, `toaster` and `loading` own messages and commands; every other package is pure.

## Colour roles

- Name a role from `internal/ui/styles/colors.go`; never write a hex literal at a call site. That
  file is the only place a colour is spelled.
- Every colour is a `lipgloss.AdaptiveColor` carrying both a `Light` and a `Dark` value. A new
  colour needs both — a single value renders unreadable on one of the two theme families.
- The roles are grouped by what they mean, not by hue: text (`TextPrimaryColor`, `TextMutedColor`,
  `TextSecondaryColor`), shell chrome (`ShellTitleColor`, `ShellTab*`, `ShellFooterHelpColor`),
  borders and overlays (`BorderDefaultColor`, `OverlayBorderColor`, `BorderHighlightFocusColor`),
  buttons (primary / secondary / danger, each with a `Focus` variant), toasts
  (`ToastBorder{Success,Error,Info,Warn}Color`), and the issue vocabulary below.
- Focus is carried by `BorderHighlightFocusColor` on the border, not by a background change.

## The issue vocabulary

An issue's type, priority and status each render as a compact token plus a colour, resolved by
`styles.IssueTypeStyle`, `styles.IssuePriorityStyle`, and `styles.IssueStatusStyle`.

| Field | Token | Source |
|---|---|---|
| Type | `B` bug, `T` task, `F` feature, `E` epic, `C` chore, `D` doc, `?` unknown | `renderhelpers.CompactIssueType` |
| Priority | `P0`–`P3` | `renderhelpers.CompactPriority` |
| Status | `OPN`, `IP`, `BLK`, `CLS`, `RDY` | `renderhelpers.CompactIssueState` |
| Status (dense rows) | `O`, `I`, `B`, `C`, `R` | `renderhelpers.CompactIssueStateNarrow` |

Adding an issue type takes a token **and** a colour: a type with a distinct glyph but no distinct
colour reads as unrecognised on the board. `internal/ui/shared/renderhelpers/type_style_parity_test.go`
pins the two sets together.

Reach for the `*Styled` variant (`CompactIssueTypeStyled`, `CompactPriorityStyled`, …) to render, and
the plain one for width math — a styled token carries escape bytes that break `len`-based arithmetic.

## Glyphs

The whole vocabulary, and the one place each is defined:

| Glyph | Means | Defined in |
|---|---|---|
| `› ` / two spaces | the selection gutter, always 2 cells wide | `styles.SelectionPrefix` |
| `…` | truncated content — one cell, so it keeps more text than `...` | `styles.TruncateString` |
| `╭ ╮ ╰ ╯ ─ │` | a section border | `styles.FormSection` |
| `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` | work in flight, 10 frames | `loading.SpinnerFrames` |
| `✅ ❌ ℹ️ ⚠️` | toast severity | `toaster.Model.View` |
| `├─ └─ │` | comment output tree | `internal/ui/detail/comments.go` |
| `• ` | a metadata list item | `internal/ui/detail/metadata.go` |

Spend a new glyph only when an existing one cannot carry the meaning, and define it next to its
siblings rather than inline at the call site.

## Build from the shared chrome

- Frame every column, pane and shell with `styles.FormSection`; do not hand-roll a border. It owns
  the rounded corners, the title inlays (`TopLeft` / `TopRight`), focus colouring, and padding each
  line to the inner width.
- `FormSection` returns the literal string `too narrow` below width 6. A caller that needs a
  different degraded rendering handles the narrow case before calling.
- `ui/shared/issuerow` is the single compact issue-row renderer for board- and search-style lists.
  Row rendering stays there.
- There is intentionally **no shared issue-list component.** Board and search containers differ
  materially in layout, empty state and focus, so the containers stay mode-specific — `ui/board`
  columns against `ui/search` panes. Extract a shared container only when real duplication appears
  above the row level.
- `ui/detail` renders the issue detail; it is separate from compact row rendering by design.

## Selection and scrolling

- Take the gutter from `styles.SelectionPrefix(selected, styled)`. It returns both variants: use
  `plain` for width math and truncation, `rendered` for output. Deriving one from the other by
  stripping escapes is what the two return values exist to prevent.
- A move that changes the selection calls `scroll.EnsureVisible(offset, sel, window)` — the `›`
  chevron staying on screen is a contract, not a nicety, and it is the first thing a scroll
  regression breaks.
- A header whose window clips its list reads `N of M`; once the full list is loaded it reads a plain
  `N`. `internal/ui/board/board.go` holds the board's, `internal/ui/detail/details.go` the detail
  panes'.

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
- Truncate with `styles.TruncateString`, wrap with `styles.WrapLines`, right-pad with
  `textutil.PadToWidth`. Each is ANSI-aware; the `strings` equivalents are not.
- `renderhelpers.CompactIssueID` shortens an ID from the front (`…` + tail) after first dropping the
  `task-manager-ui-` prefix, because the distinguishing part of an issue ID is its tail.

## Nothing waits silently

- Long work renders the spinner: advance the frame with `loading.NextFrame`, draw it with
  `loading.Glyph`, drive it with `loading.SpinnerTickCmd`.
- The shell status line comes from `loading.Summary` — `Idle`, or `Loading: ` and the active scopes.
- A cold start draws skeleton rows (`issuerow.RenderCompactSkeleton`) rather than an empty frame.
  Their shade cycles through `styles.SkeletonShades` on the phase from `loading.SkeletonPhase`, which
  advances every 4 spinner frames for a ~1.2 s pulse.
- Launch success and failure both reach the operator as a toast; a launcher never fails silently.

## Words

- Comments render newest-first, against the backend's oldest-first default, and the header says so:
  `Comments (N · newest first)`. Surfacing recent activity first is what makes triage fast on a long
  issue; the header exists so the reader is not left inferring the order.
- Markdown on a read-only surface goes through `markdown.Renderer.RenderReadOnly`, which degrades
  deterministically: empty input to `(no content)`, plain text and any renderer failure to plain
  wrapped text.
