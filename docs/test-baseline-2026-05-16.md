# Test Baseline — 2026-05-16

Pre-migration baseline captured before epic `beads-workbench-i1th` (test stratification + mise execution layer).
Blocks `i1th.1` (acceptance review) and `i1th.2` (build-tag gating).

## Hardware / Go fingerprint

| Field | Value |
|---|---|
| OS | Linux 6.17.0-118023-tuxedo x86_64 |
| Go | go1.26.3 linux/amd64 |
| Date | 2026-05-16 |

## Total test count

**445 tests** (unique top-level test names across all packages, measured with `go test -json ./... 2>/dev/null | jq ... | sort -u | wc -l`)

## Wall-time measurements (cold cache, `go clean -testcache` before each run)

| Run | Wall time |
|---|---|
| Run 1 | 96.2 s |
| Run 2 | 92.3 s |
| Run 3 | 93.4 s |
| **Median** | **93.4 s** |

Command used:
```bash
go clean -testcache && timeout 240 go test ./... -timeout=180s >/dev/null 2>&1
```

## Top-20 slowest individual tests

Measured by `jq .Elapsed` from `go test -json` output (one cold-cache run). Tests with `Elapsed > 0.1s` shown, ranked descending.

| Elapsed | Package :: Test |
|---|---|
| 13.66 s | `internal/app` :: `TestModelEmbeddedFixtureBoardToDetailSmokeWorkflow` |
| 13.41 s | `internal/mode/search` :: `TestSearchModeEmbeddedFixtureInitUsesEmptyQueryFallback` |
| 13.21 s | `internal/app` :: `TestModelEmbeddedFixtureFullBoardCaptureGolden` |
| 12.71 s | `internal/app` :: `TestModelEmbeddedFixtureMutationModalsOpenWithoutCatalogDecodeToast` |
| 12.43 s | `internal/app` :: `TestModelEmbeddedFixtureBoardToDetailSmokeWorkflow` *(run 5 sample)* |
| 12.39 s | `internal/testing/e2e/embeddedfixture` :: `TestSeedSkipsWhenToolsUnavailable` |
| 12.12 s | `internal/app` :: `TestModelEmbeddedFixtureMutationModalsOpenWithoutCatalogDecodeToast` |
| 12.01 s | `internal/app` :: `TestModelReusableDetailToolScenarioCoversEditorAndLaunchersWithFakes` |
| 11.90 s | `internal/app` :: `TestModelEmbeddedFixtureStartupLoadsBoardWithoutGatewaySectionErrors` |
| 11.19 s | `internal/app` :: `TestModelEmbeddedFixtureDetailShowsRelatedFromRealBDRelatedLink` |
| 10.85 s | `internal/app` :: `TestModelEmbeddedFixtureDetailEditHotkeyUsesEditorService` |
| 10.52 s | `internal/app` :: `TestModelEmbeddedFixtureDetailShowsRelatesToDependentOnlyUnderRelated` |
| 10.07 s | `internal/app` :: `TestModelEmbeddedFixtureDetailShowsRelatedFromRealBDRelatedLink` |
| 10.01 s | `internal/app` :: `TestModelEmbeddedFixtureDetailShowsRelatesToDependentOnlyUnderRelated` |
| 9.51 s | `internal/app` :: `TestModelEmbeddedFixtureStartupLoadsBoardWithoutGatewaySectionErrors` |
| 9.44 s | `internal/app` :: `TestModelEmbeddedFixtureDetailEditHotkeyUsesEditorService` |
| 9.41 s | `internal/app` :: `TestModelEmbeddedFixtureFullBoardCaptureGolden` |
| 9.01 s | `internal/app` :: `TestModelBuiltInLauncherHotkeysUseLauncherService` |
| 3.00 s | `internal/app` :: `TestModelMutationResultMarksBrowseDirtyAndRefreshesOnlyActiveSurface` |
| 3.00 s | `internal/app` :: `TestModelEditIssueActionUsesEditorServiceAndUpdatesDetail` |

**Note:** The dominant cost is the embedded-fixture tests in `internal/app` and `internal/mode/search`, which each spin up real `bd` subprocess invocations. These are the primary targets for the `//go:build integration` gating work in `i1th.2`.

## Methodology

All runs performed with both timeout layers as required:
```bash
timeout 240 go test ./... -timeout=180s
```

Slowest-test breakdown command:
```bash
go clean -testcache && timeout 240 go test -json ./... -timeout=180s 2>/dev/null \
  | jq -r 'select(.Action=="pass" or .Action=="fail") | select(.Elapsed != null and .Elapsed > 0.5) | select(.Test != null) | "\(.Elapsed)s  \(.Package)::\(.Test)"' \
  | sort -rn | head -20
```
