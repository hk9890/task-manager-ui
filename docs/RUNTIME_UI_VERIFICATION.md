# Runtime UI Verification Workflow

Use this runbook when a change touches user-visible runtime behavior (layout, navigation, search, startup shell UX, editor/launcher flows).

## Pre-release rendering checks

Run these before tagging any release. They guard against visible UI artifacts.

### Render-regression test packages

| Package | What it verifies |
|---------|-----------------|
| `internal/mode/board/render_regression_test.go` | Frame-stacking regression guard — confirms expected border count per rendered board frame. |
| `internal/mode/search/render_regression_test.go` | Frame-stacking regression guard for search mode. |
| `internal/logging/render_regression_test.go` | Log-bleed regression guard — confirms log output does not bleed into rendered frames. |
| `internal/app/render_regression_test.go` | Doubled-column-header / frame-stacking guard at the app composition level. |

The `taskmgr` repository backend has its own behavior tests in `internal/repository/taskmgr` that confirm the in-process reads (counts, sort order, search) match the task-manager SDK.

### Commands

```bash
# Integration tests (build tag: integration) — includes taskmgr backend behavior tests:
mise run test:integration
```

### Release-blocking vs advisory

| Check | Failure means | Action |
|-------|--------------|--------|
| render regression (board/search/logging) | Visible UI artifact — frame bleed or corrupt borders | **Release blocker** |
| `taskmgr` backend behavior tests | Wrong numbers, order, or search results on screen | **Release blocker** |
| `t.Logf` diagnostic with PASS status | Advisory only — logs diagnostic counts but does not fail | Review the diagnostic; no hard block |

Any test that prints a diagnostic message but ends in PASS is advisory: read the message, judge whether follow-up is needed, but do not hold the release for it.

## 1) Fast deterministic automated loop

Run the focused scenario set first. These are all unit tests (no build tag required).

```bash
go test ./internal/testing/ui ./internal/mode/search ./internal/app -run 'TestAssertionHelpersCoverStartupErrorsSearchAndActions|TestSearchModeReusableScenarioHelpersCoverTypingFragileAndClear|TestModelReusableBoardSearchDetailScenarioCoversTypingClearScrollAndBack|TestModelStartupBoardLayoutSanityAndNoRuntimeErrors' -v
```

This is the default quick proof for runtime behavior during implementation. See `docs/TESTING.md` for the unit-vs-integration tier vocabulary.

## 2) Fast manual built-binary review

Seed a throwaway `.tasks` store with the `taskmgr` CLI, then run the real app against it. `taskmgr` is the default `--repo` backend, so no flag is needed.

```bash
go build -o /tmp/taskmgr-ui ./cmd/taskmgr-ui
repoPath="$(mktemp -d)"
( cd "$repoPath" \
  && taskmgr init --prefix bwb \
  && taskmgr create --title "Ready issue" \
  && taskmgr create --title "In-progress issue" --type bug )
(cd "$repoPath" && /tmp/taskmgr-ui)
```

`taskmgr create --json` prints the new ID (e.g. `bwb-<code>`) if you need to reference it in a later step.

### Ad-hoc PTY recipes (copy/paste)

Use this when you need agent-visible proof of runtime behavior without manually watching a terminal.

```bash
python3 -m pip install --user pyte
go build -o /tmp/taskmgr-ui ./cmd/taskmgr-ui
```

#### PTY step toolkit (`scripts/capture_taskmgr_ui_screen.py`)

Prefer repeatable `--step` instructions over blind delay chains.

- `send-key:<KEY>`
- `wait-for-text:<TEXT>[:timeout-ms]`
- `wait-for-text-once:<TEXT>[:timeout-ms]`
- `wait-for-no-text:<TEXT>[:timeout-ms]`
- `sleep-ms:<MS>`
- `checkpoint:<name>`

Legacy `--steps delay:key,...` still works, but `--step` wait-based flows are the default for reliable mutation verification.

#### A) Read-only navigation check (wait-based)

```bash
repoPath="$(mktemp -d)"
( cd "$repoPath" && taskmgr init --prefix bwb && taskmgr create --title "Ready issue" )
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

#### B) Mutation save check (before/after store assertion)

Capture the mutation flow, then compare the issue state with `taskmgr show --json` before and after. The `--cwd` store seeded above persists between the capture run and the assertion.

```bash
repoPath="$(mktemp -d)"
( cd "$repoPath" && taskmgr init --prefix bwb && taskmgr create --title "Ready issue" )
issueID="$( (cd "$repoPath" && taskmgr list --json) | python3 -c 'import json,sys; print(json.load(sys.stdin)[0]["id"])' )"

before="$( (cd "$repoPath" && taskmgr show "$issueID" --json) )"

# Insert the mode-specific edit + save keystrokes as extra --step lines
# between opening Detail and returning to Board.
python3 scripts/capture_taskmgr_ui_screen.py \
  --cwd "$repoPath" --width 120 --height 34 --startup-wait 1.2 \
  --step 'wait-for-text:Selected::3000' \
  --step 'send-key:ENTER' \
  --step 'wait-for-text:Detail::3000' \
  --step 'wait-for-text:Board:2000' \
  --step 'send-key:CTRL+Q' \
  -- -- /tmp/taskmgr-ui

after="$( (cd "$repoPath" && taskmgr show "$issueID" --json) )"
[ "$before" != "$after" ] && echo "changed: true" || echo "changed: false"
```

Expected for a save flow: the `before`/`after` JSON differ (`changed: true`).

#### C) Mutation cancel / no-save check

Same recipe as B, but drive the cancel path (e.g. `ESC` out of the edit without saving). Expected: `before`/`after` JSON are identical (`changed: false`).

#### D) Common failure/timeout messages

- `step <index> (...) timed out after <N>ms`: a specific wait step did not settle; inspect `steps[*].observed_excerpt` and `failure`.
- `capture timed out after <Ns>`: global timeout was exceeded; increase `--timeout` for longer flows.
- `missing command after --`: `capture_taskmgr_ui_screen.py` did not receive the app command.
- `ModuleNotFoundError: No module named 'pyte'`: install `pyte` first (`python3 -m pip install --user pyte`).

## 3) What to verify in the manual run

Use this short checklist (pass/fail, no user handoff needed):

1. **Layout**: startup screen renders cleanly; board/detail/search surfaces remain readable at your terminal size.
2. **Navigation**: board → detail → board and board ↔ search transitions work without focus loss/stuck state.
3. **Search behavior**: type query, refine/clear query, confirm results update and fragile states (empty/no-results) remain usable.
4. **External-tool flows**: from detail mode, trigger launcher keys (`n`, `p`, `l`) and confirm app remains active with expected toast feedback; use `e` for edit round-trip and verify detail reload.

If your change targets only one area, still sanity-check the other areas quickly to avoid cross-flow regressions.

## 4) Process-level policy (optional and narrow)

See `docs/TESTING.md` (Process-level capture policy) for the authoritative rule. In short: do not add new process-level capture harnesses unless a concrete bug class cannot be proven in-process, and any such path must define a readiness signal, a hard timeout, and guaranteed cleanup behavior.

## 5) Done-column closed-limit: resize-then-refresh

**What this verifies:** `sectionItemCapacity()` scales linearly with terminal height — `N = height - 3` with no floor. Resizing the terminal and pressing `r` causes the Done column to fetch a different number of rows proportional to the new height.

**Pre-conditions:**

- A `.tasks` store with more than 200 closed issues. Seed one with the `taskmgr` CLI (`taskmgr init`, then create + `taskmgr close` enough issues), or point `taskmgr-ui` at an existing store that already has a large closed total. `taskmgr` is the default backend, so no `--repo` flag is needed.
- `taskmgr-ui` is built and on `$PATH` or run via `mise run taskmgr-ui`.

**Procedure:**

1. Open a terminal and resize it so the height is **40 rows**. Verify with `echo $LINES` or your terminal's title bar. Record the height as `H1`.
2. Launch `taskmgr-ui` against a qualifying store (run it from the store's directory, or pass `--cwd`):
   ```
   (cd /path/to/store && mise run taskmgr-ui)
   ```
3. On the board, locate the **Done** column header. It shows `N of M` where `N` is the number of rows loaded and `M` is the true closed total in the database.
4. Confirm `N` equals `H1 - 3` (e.g. for a 40-row terminal, `N = 37`). `M` should be the real closed total, e.g. `37 of 679`. Record `N` as `N1` and `M` as `M1`.
5. Keep `taskmgr-ui` running. Resize the terminal **taller** — at least **200 rows** (e.g. drag the window to maximum height or `printf '\e[8;200;220t'` in a supporting terminal emulator). Record the new height as `H2`.
6. Press **`r`** to refresh.
7. Wait for the board to reload (the Done column header will update).
8. Confirm `N` is now `H2 - 3` (e.g. for a 200-row terminal, `N = 197`). The exact value must match `height - 3`; what matters is that `N` increased proportionally to `H2`.
9. Confirm `M` is unchanged — it must still equal `M1` (the real closed total). `M` must not equal `N` unless the repo genuinely has that few closed issues.

**Expected outcome (falsifiable):**

| Step | Pass condition | Fail signal |
|------|---------------|-------------|
| 4 (small terminal, H1=40) | Done header shows `(H1-3) of M` — e.g. `37 of M` where M > H1 | N ≠ H1-3, or M = N despite large DB |
| 8 (after resize + r, H2=200) | Done header shows `(H2-3) of M` — e.g. `197 of M` | N did not increase to H2-3 after resize + refresh |
| 9 (M unchanged) | M equals M1 from step 4 | M changed or equals N |

**Failure modes and diagnostics:**

- `N` does not change after resize: `loadDashboardCmd` is not passing `sectionItemCapacity()` into `DashboardOptions.ClosedLimit`, or the repository impl is ignoring it. Check `internal/mode/board/model.go` and the relevant repository impl.
- `N` does not equal `height - 3`: `sectionItemCapacity()` may not be receiving the updated window size — check the `WindowSizeMsg` handler in `internal/mode/board/model.go`.
- `M` equals `N` after resize: `ClosedTotal` is being computed after the limit slice instead of before. Check `internal/repository/memory/repository.go` and `internal/repository/taskmgr`.

## 6) Board and details scroll-window visibility: EnsureVisible

**What this verifies:** After pressing `j` past the visible window boundary, the selected row stays visible (the `›` chevron remains on screen) and the column/pane header shows `N of M` to indicate the window is clipping the full list.

**Pre-conditions:**

- A `.tasks` store with more than 22 ready issues (so that 30 `j` presses push past the viewport at height=25). Seed one with `taskmgr init` + repeated `taskmgr create`.
- `taskmgr-ui` is built.

**Procedure — board Ready column:**

1. Open a terminal at height=25 (22 usable rows per column after borders).
2. Launch `taskmgr-ui` from a qualifying store directory:
   ```
   (cd /path/to/store && /tmp/taskmgr-ui)
   ```
3. Move focus to the **Ready** column (press `l` or `h` until the Ready header is highlighted).
4. Press `j` 30 times.
5. Confirm:
   - The status line at the bottom shows `Selected:` followed by an issue ID that is deeper in the list (not the topmost issue).
   - The `›` chevron is visible in the Ready column next to the selected issue.
   - The Ready column header reads `N of M` (e.g. `22 of 89`) where `N < M`.

**Procedure — details Dependencies pane:**

1. From the board, open an issue that has more than 12 dependency relations (blocked-by + blocks + related).
2. In the detail view, press `h` to move focus to the Dependencies pane (left pane).
3. Press `j` until the selection moves past the visible window boundary.
4. Confirm:
   - The `›` chevron remains adjacent to the selected dependency row.
   - The Dependencies pane header shows `N of M` (e.g. `8 of 15`).

**Expected outcome:**

| Step | Pass condition | Fail signal |
|------|---------------|-------------|
| Board j×30 | `›` visible in Ready column; header `N of M` with N<M | No chevron on screen; header shows plain count |
| Details deps scroll | `›` visible in deps pane; header `N of M` | No chevron; plain header count |

**Failure modes and diagnostics:**

- `›` disappears after pressing `j` past the viewport: `EnsureVisible` is not being called from `moveRow` (board) or `moveRelatedSelection` (details). Check `internal/mode/board/model.go` and `internal/mode/detail/model.go`.
- Header shows plain count instead of `N of M` when window clips: the `visibleCount < len(rows)` branch in `internal/ui/board/board.go:Render` is not triggered, or the deps pane header in `internal/ui/detail/details.go` is not updated. Check the header logic in both renderers.

## 7) Deep Done-column navigation: load-more pagination

**What this verifies:** Pressing `j` past the initial loaded slice in the Done column triggers incremental loads, the Done header N grows monotonically toward M, the `›` chevron stays visible throughout (scroll-window visibility contract), the `r` reload resets state correctly, and the final header format transitions from `N of M` to plain `N` once all issues are loaded.

**Pre-conditions:**

- A `.tasks` store with roughly 89 closed issues — enough to span multiple pagination pages at small terminal heights. Seed one with the `taskmgr` CLI (`taskmgr init`, then create + `taskmgr close` ~89 issues). The examples below use 89 as the closed total `M`; substitute your store's actual closed count.
- `taskmgr-ui` is built: `mise run build` or `go build -o /tmp/taskmgr-ui ./cmd/taskmgr-ui`.
- Terminal height ≤ 30 rows (so the initial load is a small slice, not all 89). Verify with `echo $LINES`.

**Procedure:**

1. Build taskmgr-ui:
   ```bash
   mise run build
   ```
   Or equivalently: `go build -o /tmp/taskmgr-ui ./cmd/taskmgr-ui`.

2. Open a terminal at height ≤ 30 rows. Confirm with `echo $LINES`.

3. Launch from the seeded store directory:
   ```bash
   (cd /path/to/store && /tmp/taskmgr-ui)
   ```

4. Observe the **Done** column header. It should read `N of 89` where `N ≈ height - 3` (e.g. `25 of 89` at height=28). This confirms the initial load is capped and pagination is active.

5. Press `RIGHT` three times (or use `l`/`h`) to focus the Done column.

6. Press `j` repeatedly past the loaded-slice boundary. Confirm:
   - The Done header N grows (e.g. `25 of 89` → `50 of 89` → `75 of 89`). M stays 89.
   - The `›` chevron remains visible in the Done column next to the selected issue at all times (scroll-window contract — see §6).
   - No double-loads: run with `--debug` and confirm at most one load-more event per threshold crossing in the persistent JSON log (during an interactive session stderr is suppressed, so debug output goes only to the persistent log — not a stderr redirect). The real log messages are `dispatching load-more for Done column` and `load-more suppressed`:
     ```bash
     (cd /path/to/store && /tmp/taskmgr-ui --debug)
     # in another terminal: tail -f ~/.local/state/taskmgr-ui/taskmgr-ui-*.log | grep load-more
     ```

7. Press `r` for manual reload. Confirm:
   - The Done header resets to the initial viewport-sized N (same as step 4).
   - The selection returns to the top of the Done column (load-more reset contract).

8. Press `j` repeatedly until Done header shows `89 of 89`. Then continue pressing `j` once more. Confirm:
   - The final header transitions from `89 of 89` to plain `89` (the `TotalIsExact` flip — see `internal/ui/board/board.go` header logic).
   - No further loads are triggered beyond this point.

**Pass/Fail table:**

| Step | Pass | Fail signal |
|------|------|-------------|
| 4 | Initial header `≈(height-3) of 89` | Header missing `of 89`; or N == 89 immediately (no pagination active) |
| 6 | N grows monotonically; `›` always visible; one load per threshold crossing | N stays at initial value; chevron disappears; multiple loads per crossing |
| 7 | Header resets to initial N; selection at top of Done | Header keeps deep value; selection position lost |
| 8 | Final header reads `89 of 89` then flips to plain `89` | Final N < 89; or header never transitions to plain `N` |

**Diagnostics on failure:**

- `r` does not reset N to the initial value: check the `doneLoadedCount` reset path in `internal/mode/board/model.go`.
- N does not grow when scrolling deep: check the `loadMoreClosedCmd` dispatch threshold and the offset wiring in the `taskmgr` repository backend.
- Double-loads on a single threshold crossing: check the `doneLoadInFlight` guard.
- `›` chevron disappears after pressing `j` past the loaded slice: the `EnsureVisible` scroll-following logic has regressed — see §6 for the diagnostic steps.
- `89 of 89` never transitions to plain `89`: `TotalIsExact` is not being set when the last page is loaded, or the header renderer in `internal/ui/board/board.go` is not checking it.
