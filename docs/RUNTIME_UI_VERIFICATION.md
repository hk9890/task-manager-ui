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

**What this verifies:** after the iwvm change, `closedLimit()` scales with terminal height. A small terminal floors at 50 closed rows; resizing taller and pressing `r` fetches more rows up to the new height-derived cap.

**Pre-conditions:**

- Repository has more than 50 closed issues. Both `--repo beads` (against this project) and `--repo caching` satisfy this; the `beads` repo has >600 closed issues.
- `bwb` is built and on `$PATH` or run via `mise run bwb`.

**Procedure:**

1. Open a terminal and resize it so the height is **less than 70 rows** (e.g. 40 rows). Verify with `echo $LINES` or your terminal's title bar.
2. Launch `bwb` against a qualifying repo:
   ```
   mise run bwb -- --repo beads
   ```
3. On the board, locate the **Done** column header. It shows `N of M` where `N` is the number of rows loaded and `M` is the true closed total in the database.
4. Confirm `N` is **50** (the floor). `M` should be the real closed total, e.g. `50 of 679`. If `N` is not 50, the floor is broken.
5. Keep `bwb` running. Resize the terminal **taller** — at least **200 rows** (e.g. drag the window to maximum height or `printf '\e[8;200;220t'` in a supporting terminal emulator).
6. Press **`r`** to refresh.
7. Wait for the board to reload (the Done column header will update).
8. Confirm `N` is now **larger than 50** — for a 200-row terminal, `closedLimit()` returns `sectionItemCapacity()` which exceeds 50 by a significant margin (roughly `height - 10` divided across columns). The exact value is not critical; what matters is `N > 50`.
9. Confirm `M` is unchanged — it must still equal the real closed total, not the cap value. `M` must not equal `N` unless the repo genuinely has that few closed issues.

**Expected outcome (falsifiable):**

| Step | Pass condition | Fail signal |
|------|---------------|-------------|
| 4 (small terminal) | Done header shows `50 of M` where M > 50 | N ≠ 50, or M = N despite large DB |
| 8 (after resize + r) | Done header shows `N of M` where N > 50 | N is still 50 after resize + refresh |
| 9 (M unchanged) | M equals the same total as in step 4 | M changed or equals N |

**Failure modes and diagnostics:**

- `N` stays at 50 after resize: `loadDashboardCmd` is not passing `closedLimit()` into `DashboardOptions.ClosedLimit`, or the repository impl is ignoring it. Check `internal/mode/board/model.go` and the relevant repository impl.
- `M` equals `N` after resize: `ClosedTotal` is being computed after the limit slice instead of before. Check `internal/repository/memory/repository.go` and `internal/repository/beads/lean_reads.go`.
- `N` does not increase proportionally to height: `sectionItemCapacity()` may not be receiving the updated window size — check the `WindowSizeMsg` handler in `internal/mode/board/model.go`.
