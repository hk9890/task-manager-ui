# Runtime UI Verification Workflow

Use this runbook when a change touches user-visible runtime behavior (layout, navigation, search, startup shell UX, editor/launcher flows).

## Pre-release data-consistency checks

Run these before tagging any release. They verify that what `bwb` renders on screen matches the live `bd` database.

### Parity test packages

| Package | What it verifies |
|---------|-----------------|
| `internal/dashboard/parity` | **Count parity** — column totals (NotReady/Ready/InProgress/Done) match `bd count`; **sort parity** — row order in each column matches `bd ready`, `bd blocked`, `bd list`. |
| `internal/mode/search/parity` | Search result count and order match `bd search`. |
| `internal/mode/board/render_regression_test.go` | Frame-stacking regression guard — confirms expected border count per rendered board frame. |
| `internal/mode/search/render_regression_test.go` | Frame-stacking regression guard for search mode. |
| `internal/logging/render_regression_test.go` | Log-bleed regression guard — confirms log output does not bleed into rendered frames. |

### Commands

```bash
# Fast fixture-backed run (no live DB required, always available):
mise run test:integration

# Parity against this repo's live beads DB (~30 s):
BWB_PARITY_THIS_REPO=1 mise run test:integration

# Parity against an arbitrary external beads DB (read-only):
BWB_PARITY_EXTERNAL_PATH=/path/to/external/repo mise run test:integration

# Smoke check — builds bwb-smoke and runs all checks against this repo; prints PASS/FAIL report:
mise run smoke

# Smoke check against an arbitrary external repo (override via env var):
BWB_SMOKE_DIR=/path/to/external/repo mise run smoke
```

### Release-blocking vs advisory

| Check | Failure means | Action |
|-------|--------------|--------|
| count parity (dashboard) | Wrong numbers on screen | **Release blocker** |
| sort parity (dashboard) | Wrong row order on screen | **Release blocker** |
| search parity | Wrong search results on screen | **Release blocker** |
| render regression (board/search/logging) | Visible UI artifact — frame bleed or corrupt borders | **Release blocker** |
| `t.Logf` diagnostic with PASS status | Advisory only — e.g. `ClosedAtDescPreservedOnRealData` logs diagnostic counts but does not fail | Review the diagnostic; no hard block |

Any test that prints a diagnostic message but ends in PASS is advisory: read the message, judge whether follow-up is needed, but do not hold the release for it.

## 1) Fast deterministic automated loop

Run the focused scenario set first. These are all unit tests (no build tag required).

```bash
go test ./internal/testing/ui ./internal/mode/search ./internal/app -run 'TestAssertionHelpersCoverStartupErrorsSearchAndActions|TestSearchModeReusableScenarioHelpersCoverTypingFragileAndClear|TestModelReusableBoardSearchDetailScenarioCoversTypingClearScrollAndBack|TestModelStartupBoardLayoutSanityAndNoRuntimeErrors' -v
```

This is the default quick proof for runtime behavior during implementation. See `docs/TESTING.md` for the unit-vs-integration tier vocabulary.

## 2) Fast manual built-binary review

Run the real app against the embedded fixture:

```bash
go build -o /tmp/bwb ./cmd/bwb
repoPath="$(mktemp -d)"
sh internal/testing/e2e/embeddedfixture/setup.sh "$repoPath" internal/testing/e2e/embeddedfixture/seed.json
(cd "$repoPath" && BD_NON_INTERACTIVE=1 /tmp/bwb)
```

### Ad-hoc PTY recipes (copy/paste)

Use this when you need agent-visible proof of runtime behavior without manually watching a terminal.

```bash
python3 -m pip install --user pyte
go build -o /tmp/bwb ./cmd/bwb
```

#### PTY step toolkit (`scripts/capture_bwb_screen.py`)

Prefer repeatable `--step` instructions over blind delay chains.

- `send-key:<KEY>`
- `wait-for-text:<TEXT>[:timeout-ms]`
- `wait-for-no-text:<TEXT>[:timeout-ms]`
- `checkpoint:<name>`

Legacy `--steps delay:key,...` still works, but `--step` wait-based flows are the default for reliable mutation verification.

#### A) Read-only navigation check (wait-based)

```bash
repoPath="$(mktemp -d)"
sh internal/testing/e2e/embeddedfixture/setup.sh "$repoPath" internal/testing/e2e/embeddedfixture/seed.json
python3 scripts/capture_bwb_screen.py \
  --cwd "$repoPath" --width 120 --height 34 --startup-wait 1.2 \
  --step 'wait-for-text:Ready:3000' \
  --step 'wait-for-text:Selected::3000' \
  --step 'send-key:ENTER' \
  --step 'wait-for-text:Detail::3000' \
  --step 'checkpoint:detail-open' \
  --step 'send-key:ESC' \
  --step 'wait-for-text:Board:2000' \
  --step 'send-key:CTRL+Q' \
  -- -- env BD_NON_INTERACTIVE=1 /tmp/bwb
```

#### B) Mutation save check (wrapper + before/after)

`scripts/verify_bwb_state_flow.py` wraps capture + before/after `bd show --json` assertions.

```bash
repoPath="$(mktemp -d)"
sh internal/testing/e2e/embeddedfixture/setup.sh "$repoPath" internal/testing/e2e/embeddedfixture/seed.json
python3 scripts/verify_bwb_state_flow.py \
  --cwd "$repoPath" \
  --issue bwf-1 \
  --flow mutation-save \
  --app-command env BD_NON_INTERACTIVE=1 /tmp/bwb
```

Expected: JSON output includes `"ok": true` and `"changed": true`.

#### C) Mutation cancel / no-save check (wrapper)

```bash
repoPath="$(mktemp -d)"
sh internal/testing/e2e/embeddedfixture/setup.sh "$repoPath" internal/testing/e2e/embeddedfixture/seed.json
python3 scripts/verify_bwb_state_flow.py \
  --cwd "$repoPath" \
  --issue bwf-1 \
  --flow mutation-cancel \
  --app-command env BD_NON_INTERACTIVE=1 /tmp/bwb
```

Expected: JSON output includes `"ok": true` and `"changed": false`.

#### D) Common failure/timeout messages

- `step <index> (...) timed out after <N>ms`: a specific wait step did not settle; inspect `steps[*].observed_excerpt` and `failure`.
- `capture timed out after <Ns>`: global timeout was exceeded; increase `--timeout` for longer flows.
- `missing command after --`: `capture_bwb_screen.py` did not receive the app command.
- `--app-command requires a command after --`: `verify_bwb_state_flow.py` got no executable command.
- `ModuleNotFoundError: No module named 'pyte'`: install `pyte` first (`python3 -m pip install --user pyte`).

## 3) What to verify in the manual run

Use this short checklist (pass/fail, no user handoff needed):

1. **Layout**: startup screen renders cleanly; board/detail/search surfaces remain readable at your terminal size.
2. **Navigation**: board → detail → board and board ↔ search transitions work without focus loss/stuck state.
3. **Search behavior**: type query, refine/clear query, confirm results update and fragile states (empty/no-results) remain usable.
4. **External-tool flows**: from detail mode, trigger launcher keys (`n`, `p`, `l`) and confirm app remains active with expected toast feedback; use `e` for edit round-trip and verify detail reload.

If your change targets only one area, still sanity-check the other areas quickly to avoid cross-flow regressions.

## 4) Process-level policy (optional and narrow)

Default: do **not** add new process-level capture harnesses.

Add process-level automation only when a concrete bug class cannot be proven in-process. Any proposal must define:

1. readiness signal,
2. hard timeout,
3. guaranteed cleanup behavior.

Raw stdout transcript capture alone is not enough proof for alt-screen rendering.

## 5) Done-column closed-limit: resize-then-refresh (iwvm)

**What this verifies:** `sectionItemCapacity()` scales linearly with terminal height — `N = height - 3` with no floor. Resizing the terminal and pressing `r` causes the Done column to fetch a different number of rows proportional to the new height.

**Pre-conditions:**

- Repository has more than 200 closed issues. Both `--repo beads` (against this project) and `--repo caching` satisfy this; the `beads` repo has >600 closed issues.
- `bwb` is built and on `$PATH` or run via `mise run bwb`.

**Procedure:**

1. Open a terminal and resize it so the height is **40 rows**. Verify with `echo $LINES` or your terminal's title bar. Record the height as `H1`.
2. Launch `bwb` against a qualifying repo:
   ```
   mise run bwb -- --repo beads
   ```
3. On the board, locate the **Done** column header. It shows `N of M` where `N` is the number of rows loaded and `M` is the true closed total in the database.
4. Confirm `N` equals `H1 - 3` (e.g. for a 40-row terminal, `N = 37`). `M` should be the real closed total, e.g. `37 of 679`. Record `N` as `N1` and `M` as `M1`.
5. Keep `bwb` running. Resize the terminal **taller** — at least **200 rows** (e.g. drag the window to maximum height or `printf '\e[8;200;220t'` in a supporting terminal emulator). Record the new height as `H2`.
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
- `M` equals `N` after resize: `ClosedTotal` is being computed after the limit slice instead of before. Check `internal/repository/memory/repository.go` and `internal/repository/beads/lean_reads.go`.

## 6) Board and details scroll-window visibility: EnsureVisible (b38b.4)

**What this verifies:** After pressing `j` past the visible window boundary, the selected row stays visible (the `›` chevron remains on screen) and the column/pane header shows `N of M` to indicate the window is clipping the full list.

**Pre-conditions:**

- Repository has more than 22 ready issues (so that 30 `j` presses push past the viewport at height=25).
- `bwb` is built.

**Procedure — board Ready column:**

1. Open a terminal at height=25 (22 usable rows per column after borders).
2. Launch `bwb` against a qualifying repo (e.g. `/home/hans/dev/github/dtctl-test` which has >89 ready issues):
   ```
   (cd /home/hans/dev/github/dtctl-test && BD_NON_INTERACTIVE=1 /tmp/bwb)
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

- `›` disappears after pressing `j` past the viewport: `EnsureVisible` is not being called from `moveRow` (board) or `moveRelatedSelection` (details). Check `internal/mode/board/model.go` and `internal/mode/details/model.go`.
- Header shows plain count instead of `N of M` when window clips: the `visibleCount < len(rows)` branch in `internal/ui/board/board.go:Render` is not triggered, or the deps pane header in `internal/ui/details/details.go` is not updated. Check the header logic in both renderers.
