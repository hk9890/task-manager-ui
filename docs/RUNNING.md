# Running

How an agent launches and drives `taskmgr-ui` by hand to reproduce a bug or verify a change. Use the
`run` skill for the generic launch-and-drive flow; everything here is this repository's delta and
wins where the two disagree.

For the automated suites and the gates see [TESTING.md](TESTING.md); for reading what a run already
did see [MONITORING.md](MONITORING.md).

## Launch

```bash
mise run taskmgr-ui                                   # build + run against this project's own store
go build -o /tmp/taskmgr-ui ./cmd/taskmgr-ui          # a throwaway binary to drive elsewhere
/tmp/taskmgr-ui --repo memory --repo-file seed.jsonl  # a disposable seeded board, no store needed
```

`taskmgr-ui` is TUI-first: it takes over the terminal with the alt screen, so **raw stdout capture
proves nothing about what rendered.** Capture the screen, not the stream.

The repository backend is in-process, so a run needs no tracker subprocess, no daemon and no
prompt-suppression environment variable.

## Seed a throwaway store

```bash
repoPath="$(mktemp -d)"
( cd "$repoPath" \
  && taskmgr init --prefix demo \
  && taskmgr create --title "Ready issue" \
  && taskmgr create --title "In-progress issue" --type bug )
(cd "$repoPath" && /tmp/taskmgr-ui)
```

`taskmgr create --json` prints the new ID when a later step must reference it. `taskmgr` is the
default backend, so no `--repo` flag is needed.

## Drive it without a terminal

`scripts/capture_taskmgr_ui_screen.py` runs the binary under a PTY and returns the rendered screen,
which is how a run becomes agent-visible proof. It needs `pyte`:

```bash
python3 -m pip install --user pyte
```

Drive it with `--step` instructions rather than a delay chain — a wait settles on what is on screen,
a sleep guesses:

| Step | Waits for |
|---|---|
| `send-key:<KEY>` | nothing; sends the key |
| `wait-for-text:<TEXT>[:timeout-ms]` | `TEXT` to appear |
| `wait-for-text-once:<TEXT>[:timeout-ms]` | `TEXT` to appear, matching once |
| `wait-for-no-text:<TEXT>[:timeout-ms]` | `TEXT` to disappear |
| `sleep-ms:<MS>` | the clock, as a last resort |
| `checkpoint:<name>` | nothing; records the screen under `<name>` |

```bash
python3 scripts/capture_taskmgr_ui_screen.py \
  --cwd "$repoPath" --width 120 --height 34 --startup-wait 1.2 \
  --step 'wait-for-text:Ready:3000' \
  --step 'wait-for-text:Selected::3000' \
  --step 'send-key:ENTER' \
  --step 'wait-for-text:Detail::3000' \
  --step 'checkpoint:detail-open' \
  --step 'send-key:ESC' \
  --step 'wait-for-text:Board:2000' \
  --step 'send-key:CTRL+Q' \
  -- -- /tmp/taskmgr-ui
```

The legacy `--steps delay:key,...` form still works; wait-based `--step` flows are the default
because a mutation check cannot be timed reliably.

### Prove a mutation actually landed

A rendered screen shows what the app drew, not what it wrote. Bracket the capture with the store:

```bash
issueID="$( (cd "$repoPath" && taskmgr list --json) | python3 -c 'import json,sys; print(json.load(sys.stdin)[0]["id"])' )"
before="$( (cd "$repoPath" && taskmgr show "$issueID" --json) )"
# ... capture run that opens Detail, edits, saves, returns to Board ...
after="$( (cd "$repoPath" && taskmgr show "$issueID" --json) )"
[ "$before" != "$after" ] && echo "changed: true" || echo "changed: false"
```

A save flow must report `changed: true`; drive the same flow out through `ESC` instead of save and it
must report `changed: false`. The `--cwd` store persists between the capture run and the assertion.

## Gotchas

- **A modal holds the keyboard until it is dismissed.** Keys sent meanwhile are typed into the
  overlay. Send `ESC`, then wait for the overlay to be gone rather than counting keystrokes.
- **`e` hands the terminal to `$EDITOR` and swallows every key until that program exits.** A script
  that follows it with `CTRL+Q` types the quit into the editor and hangs.
- **`--debug` reaches stderr only before the TUI starts.** Once the interactive session raises stderr
  suppression, diagnostics go to the persistent log alone — tail that, not a stderr redirect
  ([MONITORING.md](MONITORING.md)).
- Capture failures name themselves: `step <index> (...) timed out after <N>ms` is one wait that did
  not settle (read `steps[*].observed_excerpt`), `capture timed out after <Ns>` needs a longer
  `--timeout`, `missing command after --` means the script never got the binary, and
  `ModuleNotFoundError: No module named 'pyte'` means the install step above was skipped.

## What to check in a manual run

Pass or fail each yourself; do not ask the operator to validate basics.

1. **Layout** — the first screen renders cleanly, and board, detail and search stay readable at your
   terminal size.
2. **Navigation** — board → detail → board and board ↔ search survive the round trip without lost or
   stuck focus.
3. **Search** — type a query, refine it, clear it; empty and no-results states stay usable.
4. **External tools** — `n`, `p` and `l` from detail leave the app alive with the expected toast, and
   `e` round-trips through the editor and reloads the detail.

Check the areas your change did not target too — cross-flow regressions are the ones tests miss.

## Behaviours that need a real terminal

These three cannot be driven under the PTY harness: two need a live resize, and all three need a
store large enough to page.

### Closed-limit scales with terminal height

**Proves:** `sectionItemCapacity()` scales with terminal height (`height - 3`, floored at 1, and `20`
before the first `WindowSizeMsg`), and a refresh re-reads it.

Seed a store with more than 200 closed issues. At height 40, the Done column header reads `37 of M`
where `M` is the true closed total. Keep the app running, resize to 200 rows, press `r`: the header
must read `197 of M`, with `M` unchanged.

`N` unchanged after the resize means `loadDashboardCmd` is not passing `sectionItemCapacity()` into
`DashboardOptions.ClosedLimit`, or the `WindowSizeMsg` handler never saw the new size — both in
`internal/mode/board/model.go`. `M` equal to `N` means `ClosedTotal` is computed after the limit
slice instead of before.

### The chevron follows the selection

**Proves:** `scroll.EnsureVisible` keeps the selected row on screen when the window clips the list.

Seed more than 22 ready issues, open at height 25, focus the Ready column and press `j` thirty times.
The `›` chevron must still be on screen and the header must read `N of M` with `N < M`. Repeat in the
detail Dependencies pane (press `h` to focus it) on an issue with more than 12 relations.

A lost chevron means `EnsureVisible` is not called from `moveRow` in `internal/mode/board/model.go`
or `moveRelatedSelection` in `internal/mode/detail/model.go`. A plain count where `N of M` belongs
means the clipping branch in `internal/ui/board/board.go` or the pane header in
`internal/ui/detail/details.go` never fired.

### Done-column pagination

**Proves:** scrolling past the loaded slice pages in more, once per crossing, and the header flips to
a plain count when the list is complete.

Seed roughly 89 closed issues and open at 30 rows or fewer. Focus Done and hold `j`: the header `N`
grows monotonically toward `M`, the chevron stays visible, and the run logs one
`dispatching load-more for Done column` per threshold crossing — any `load-more suppressed` beside it
is the double-load guard doing its job. Press `r`: the header returns to the opening `N` and the
selection returns to the top. Keep going to the end: `89 of 89` flips to a plain `89` and no further
load fires.

`N` stuck means the `loadMoreClosedCmd` threshold or its offset wiring; a header that never flips
means `TotalIsExact` is not set on the last page; `r` not resetting means the `doneLoadedCount` reset
path, and repeated loads per crossing mean the `doneLoadInFlight` guard — all in
`internal/mode/board/model.go`.
