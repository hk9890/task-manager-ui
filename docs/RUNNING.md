# Running

How an agent launches and drives `taskmgr-ui` by hand to reproduce a bug or verify a change. Use the
`run` skill for the generic launch-and-drive flow; everything here is this repository's delta and
wins where the two disagree.

For the automated suites and the gates see [TESTING.md](TESTING.md); for reading what a run already
did see [MONITORING.md](MONITORING.md).

## Launch

```bash
mise run taskmgr-ui                                  # build + run against this project's own store
go build -o /tmp/taskmgr-ui ./cmd/taskmgr-ui         # a throwaway binary to drive elsewhere
```

`taskmgr-ui` is TUI-first: it takes over the terminal with the alt screen, so **raw stdout capture
proves nothing about what rendered.** Capture the screen, not the stream.

## Seed a throwaway store

```bash
repoPath="$(mktemp -d)"
( cd "$repoPath" && taskmgr init --prefix demo )
```

Add the issues the run needs. `taskmgr create --json` prints the new ID, which is what a later
`update` or `close` references; `taskmgr` is the default backend, so no `--repo` flag is needed.

```bash
cd "$repoPath"

# N ready issues
for i in $(seq 1 25); do taskmgr create --title "Ready work $i" >/dev/null; done

# one in progress — create sets status open, update moves it
id="$(taskmgr create --title "Active work" --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
taskmgr update "$id" --status in_progress

# N closed — closing is what puts an issue in the Done column
for i in $(seq 1 240); do
  cid="$(taskmgr create --title "Finished work $i" --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
  taskmgr close "$cid" >/dev/null
done
```

Seeding 240 closed issues takes a couple of minutes. The runbooks below say which counts they need.

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
| `wait-for-text:<TEXT>[:timeout-ms]` | `TEXT` to appear on the rendered screen |
| `wait-for-text-once:<TEXT>[:timeout-ms]` | `TEXT` anywhere in the output stream since this step began — the verb for text already overwritten, such as a toast (they dismiss after 3s) |
| `wait-for-no-text:<TEXT>[:timeout-ms]` | `TEXT` to disappear from the rendered screen |
| `sleep-ms:<MS>` | the clock, as a last resort |
| `checkpoint:<name>` | nothing; records the screen under `<name>` |

```bash
python3 scripts/capture_taskmgr_ui_screen.py \
  --cwd "$repoPath" --width 200 --height 34 --startup-wait 1.2 --timeout 25 \
  --step 'wait-for-text:Ready:3000' \
  --step 'send-key:ENTER' \
  --step 'wait-for-text:Detail:3000' \
  --step 'checkpoint:detail-open' \
  --step 'send-key:ESC' \
  --step 'wait-for-text:Board:2000' \
  --step 'send-key:CTRL+Q' \
  -- -- /tmp/taskmgr-ui
```

The script writes one JSON object to stdout and exits non-zero on failure — the final screen is
`screen`, each `checkpoint:` lands in `checkpoints[]`, per-step evidence in `steps[]`. Redirect it to
a file; the payload is large.

`--timeout` caps the whole run and defaults to 10s — set it above the sum of your step timeouts, or a
slow store open aborts the flow mid-way and reads as an application failure.

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

- **The Done column needs a wide terminal.** At `--width 120` only Not Ready, Ready and In Progress
  fit, so a `wait-for-text:Done` never settles. Use `--width 200` for any flow that touches Done.
- **A modal holds the keyboard until it is dismissed.** Keys sent meanwhile are typed into the
  overlay. Send `ESC`, then wait for the overlay to be gone rather than counting keystrokes.
- **`e` hands the terminal to `$EDITOR` and swallows every key until that program exits.** A script
  that follows it with `CTRL+Q` types the quit into the editor and hangs.
- **During a capture, `--debug` output reaches the persistent log, not stderr** — tail the log
  ([MONITORING.md](MONITORING.md)), never a stderr redirect.
- Capture failures name themselves: `step <index> (...) timed out after <N>ms` is one wait that did
  not settle (read `steps[*].observed_excerpt`), `capture timed out after <Ns>` needs a longer
  `--timeout`, `missing command after --` means the script never got the binary, and
  `ModuleNotFoundError: No module named 'pyte'` means the install step above was skipped.

## What to check in a manual run

- **Surfaces** — board, detail and search each render and stay readable at your terminal size.
- **External tools** — `n`, `p` and `l` from detail leave the app alive with the expected toast, and
  `e` round-trips through the editor and reloads the detail.

## Behaviours that need a real terminal

These three cannot be driven under the PTY harness: two need a live resize, and all three need a
store large enough to page.

### Closed-limit scales with terminal height

**Proves:** `sectionItemCapacity()` scales with the height the mode receives (`height - 3`, floored
at 1, and `20` before the first `WindowSizeMsg`), and a refresh re-reads it.

Seed a store with more than 200 closed issues. The mode receives the terminal height minus two rows
of shell chrome, so at a terminal of `H` rows the Done column header reads `H-5 of M`, where `M` is
the true closed total: `35 of M` at height 40, `25 of M` at height 30. Keep the app running, resize
to 200 rows, press `r`: the header must read `195 of M`, with `M` unchanged.

`N` unchanged after the resize means `loadDashboardCmd` is not passing `sectionItemCapacity()` into
`DashboardOptions.ClosedLimit`, or the `WindowSizeMsg` handler never saw the new size — both in
`internal/mode/board/model.go`. `M` equal to `N` means `ClosedTotal` is computed after the limit
slice instead of before.

### The chevron follows the selection

**Proves:** the scroll window keeps the selected row on screen when it clips the list.

Seed more than 22 ready issues, open at height 25, focus the Ready column and press `j` thirty times.
The `›` chevron must still be on screen and the header must read `N of M` with `N < M`. Repeat in the
detail Dependencies pane (press Left to focus it) on an issue with more than 12 relations.

A lost chevron means `scroll.EnsureVisible` is not called from `moveRow` in
`internal/mode/board/model.go`, or `scroll.EnsureVisibleClipped` from `moveRelatedSelection` in
`internal/mode/detail/model.go` — the detail panes spend their first and last rows on the
`… (N earlier)` / `… (N more)` indicators, so they need the clipped form. A plain count where `N of M`
belongs means the clipping branch in `internal/ui/board/board.go` or the pane header in
`internal/ui/detail/details.go` never fired.

### Done-column pagination

**Proves:** scrolling past the loaded slice pages in more, once per crossing.

Seed roughly 89 closed issues and open at 30 rows or fewer. Launch with `--debug` — the load-more
records are DEBUG level and reach the persistent log only under that flag
([MONITORING.md](MONITORING.md)). Focus Done and hold `j`: the header `N` grows monotonically toward
`M`, the chevron stays visible, and the run logs one `dispatching load-more for Done column` per
threshold crossing — any `load-more suppressed` beside it is the double-load guard doing its job.
Press `r`: the header returns to the opening `N` and the selection returns to the top.

Walking to the end does **not** produce a plain count on a store this size. Once every closed issue
is loaded the header switches from "loaded of total" to "visible of total"
(`internal/ui/board/board.go`), so it reads `N of M` with `N` the rows the window shows. The plain
`M` is the branch below that, and it needs the loaded list to fit the window as well — which 89 rows
in a 30-row terminal never do.

`N` stuck means the `loadMoreClosedCmd` threshold or its offset wiring; `r` not resetting means the
`doneLoadedCount` reset path, and repeated loads per crossing mean the `doneLoadInFlight` guard — all
in `internal/mode/board/model.go`.
