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
