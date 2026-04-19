# Runtime UI Verification Workflow

Use this runbook when a change touches user-visible runtime behavior (layout, navigation, search, startup shell UX, editor/launcher flows).

## 1) Fast deterministic automated loop

Run the focused scenario set first:

```bash
go test ./internal/testing/ui ./internal/mode/search ./internal/app -run 'TestAssertionHelpersCoverStartupErrorsSearchAndActions|TestSearchModeReusableScenarioHelpersCoverTypingFragileAndClear|TestModelReusableBoardSearchDetailScenarioCoversTypingClearScrollAndBack|TestModelEmbeddedFixtureStartupLoadsBoardWithoutGatewaySectionErrors' -v
```

This is the default quick proof for runtime behavior during implementation.

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
