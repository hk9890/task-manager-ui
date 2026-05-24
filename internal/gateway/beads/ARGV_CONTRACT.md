# bd argv contract

Complete inventory of every distinct `bd` argv shape bwb emits at runtime.
Each row maps `<consumer>` → `<gateway method>` → `<exact argv shape>`.
Produced by audit task `beads-workbench-ppja.2`.

## Methodology

Static analysis: traced every `runner.Run`/`RunJSON`/`RunJSONInto` callsite in
`internal/repository/beads/read_gateway.go` and `internal/repository/beads/writes.go`
back through gateway methods to UI consumers in `internal/app/model.go`,
`internal/mode/board/model.go`, `internal/mode/search/model.go`, and
`internal/launcher/editor/service.go`.

Dynamic cross-check: the bwb log (`~/.local/state/bwb/bwb-<session_id>.log`) was inspected.
The most recent sessions in the log are from build `0.4.0`; the current codebase
is a later build that replaced `bd list` + `bd ready` + `bd blocked` (old board
data layer) with `bd ready --explain` + `bd query` (current). The log evidence is
therefore stale and was used only to confirm that no additional bd verbs appear
at runtime. All argv shapes below are derived from static analysis of the current
codebase; the `TestBoardInitRealGatewaySubprocessArgvCardinality` test provides
the only current dynamic cross-check (see Pinning Test Coverage below).

## Read operations

| Consumer | Gateway method | argv shape | Source location |
|---|---|---|---|
| `app.Model.Init` (startup) | `HealthCheck` | `bd ping --json` | `internal/repository/beads/read_gateway.go:72` |
| `board.loadReadyExplainCmd` | `ReadyExplain` | `bd ready --explain --json` | `internal/repository/beads/read_gateway.go:194–199` |
| `board.loadReadyExplainCmd` (with limit) | `ReadyExplain` | `bd ready --explain --json --limit <N>` | `internal/repository/beads/read_gateway.go:195–198` |
| `board.loadInProgressCmd` | `Query` | `bd query status=in_progress --json` | `internal/repository/beads/read_gateway.go:117–143`, triggered in `internal/mode/board/model.go:616` |
| `board.loadClosedCmd` | `Query` | `bd query status=closed --json -a --sort closed --limit <N>` | `internal/repository/beads/read_gateway.go:117–143`, triggered in `internal/mode/board/model.go:625–630` |
| `board.loadClosedCountCmd` | `CountIssues` | `bd count --by-status --json --status closed` | `internal/repository/beads/read_gateway.go:402–411`, triggered in `internal/mode/board/model.go:638–641` |
| `app.loadDetailCmd` (board/search detail) | `ShowIssue` | `bd show <issueID> --json` | `internal/repository/beads/read_gateway.go:247`, triggered in `internal/app/model.go:1227` |
| `ShowIssue` (internal — parent sibling lookup) | `ShowIssue` (via `parentChildSiblings`) | `bd show <parentID> --json` | `internal/repository/beads/read_gateway.go:829`, called from `internal/repository/beads/read_gateway.go:295` |
| `launcher/editor.IssueEditor.EditIssue` | `ShowIssue` | `bd show <issueID> --json` | `internal/launcher/editor/service.go:57` |
| `search.loadSearchCmd` (empty text, no WorkState filter) | `SearchIssues` → `searchIssuesFromList` | `bd list --json --all [--limit <N>]` | `internal/repository/beads/read_gateway.go:431–448` |
| `search.loadSearchCmd` (empty text, with status filter) | `SearchIssues` → `searchIssuesFromList` | `bd list --json --status <csv> [--limit <N>]` | `internal/repository/beads/read_gateway.go:431–448` |
| `search.loadSearchCmd` (non-empty text, WorkState=Any) | `SearchIssues` | `bd search <text> --json --status all [--status <csv>] [--type <csv>] [--priority-min <N>] [--priority-max <N>] [--assignee <name>] [--label <l>]... [--limit <N>]` | `internal/repository/beads/read_gateway.go:354–396` |
| `search.loadSearchCmd` (WorkState=Ready) | `SearchIssues` → `searchIssuesFromReady` | `bd ready --json` | `internal/repository/beads/read_gateway.go:473` |
| `search.loadSearchCmd` (WorkState=Blocked) | `SearchIssues` → `searchIssuesFromBlocked` | `bd blocked --json` | `internal/repository/beads/read_gateway.go:484` |
| `app.loadMutationCatalogsCmd` (create/update/comment) | `StatusCatalog` | `bd statuses --json` | `internal/repository/beads/read_gateway.go:666` |
| `app.loadMutationCatalogsCmd` (create/update/comment) | `TypeCatalog` | `bd types --json` | `internal/repository/beads/read_gateway.go:687` |
| `app.loadMutationCatalogsCmd` (create/update/comment) | `LabelCatalog` | `bd label list-all --json` | `internal/repository/beads/read_gateway.go:708` |
| `app.loadStatusCatalogForIssueCmd` (status/priority quick action) | `StatusCatalog` | `bd statuses --json` | `internal/repository/beads/read_gateway.go:666` |

## Write operations

| Consumer | Gateway method | argv shape | Source location |
|---|---|---|---|
| `app.submitMutationCmd` (mutationCreate) | `CreateIssue` | `bd create --json --title <title> [--description <desc>] [--type <type>] [--priority <N>] [--assignee <name>] [--labels <csv>]` | `internal/repository/beads/writes.go:32–58` |
| `app.submitMutationCmd` (mutationUpdate) | `UpdateIssue` | `bd update <issueID> [--title <t>] [--description <d>] [--status <s>] [--type <t>] [--priority <N>] [--assignee <name>] [--set-labels <csv>]` | `internal/repository/beads/writes.go:79–115` |
| `app.submitMutationCmd` (mutationStatus quick action) | `UpdateIssue` | `bd update <issueID> --status <status>` | `internal/repository/beads/writes.go:79–115`, triggered at `internal/app/model.go:1701` |
| `app.submitMutationCmd` (mutationPriority quick action) | `UpdateIssue` | `bd update <issueID> --priority <N>` | `internal/repository/beads/writes.go:79–115`, triggered at `internal/app/model.go:1720` |
| `app.submitMutationCmd` (mutationClose) | `CloseIssue` | `bd close <issueID> [--reason <reason>]` | `internal/repository/beads/writes.go:129–140` |
| `app.submitMutationCmd` (mutationComment) | `AddComment` | `bd comments add <issueID> <body>` | `internal/repository/beads/writes.go:153–158` |
| `launcher/editor.IssueEditor.EditIssue` | `UpdateIssue` | `bd update <issueID> [--title <t>] [--description <d>] [--status <s>] [--type <t>] [--priority <N>] [--assignee <name>] [--set-labels <csv>]` | `internal/repository/beads/writes.go:79–115`, triggered at `internal/launcher/editor/service.go:91` |

## Dynamic flag notes

Several argv shapes include dynamic/conditional flags:

- `--limit <N>`: driven by `sectionItemCapacity()` (board) or `searchItemCapacity()` (search); both default to 20 before the first `tea.WindowSizeMsg`. `closedLimit()` enforces a floor of 50 for the Done column.
- `--sort closed`: always present for the Done column (hardcoded `SortFieldClosedAt`).
- `-a` (IncludeClosed): always present for the Done column (hardcoded `IncludeClosed: true`).
- `--status <csv>`: present only when `query.Statuses` has exactly one entry (list/search) or hardcoded to `closed` (CountIssues for board). See per-subcommand semantics below.
- `--all`: present for `searchIssuesFromList` only when `query.Statuses` is empty.
- Write flags (`--title`, `--description`, etc.): each present only when the corresponding `UpdateIssueInput` field is non-nil.

## Per-subcommand --status semantics (bd 1.0.4 audit, g2h5)

Verified against real bd 1.0.4 in a fresh workspace. See `CountIssues` in `read_gateway.go` for the workaround.

| subcommand | `--status open,in_progress` (CSV) | `--status open --status in_progress` (repeated) | notes |
|---|---|---|---|
| `bd list` | WORKS — returns union of both statuses | not tested (CSV works) | CSV union is supported |
| `bd count --by-status` | EMPTY — literal status name match, `"open,in_progress"` is not a real status | EMPTY — last value wins or repeated flags unsupported | Neither form works for multi-status |
| `bd search <text>` | EMPTY — returns no results for CSV status token | n/a | Single-token only; CSV treated as literal |

**Consequence for `CountIssues`**: when `len(query.Statuses) > 1`, the gateway omits `--status` entirely and fetches all groups, then filters in-memory to the requested set. Single-status and no-status queries continue to pass `--status` to bd count unchanged.

**Consequence for `SearchIssues`** (text-search path via `bd search`): multi-status queries are not passed to bd. The `bd search` path is only invoked for non-empty text with `WorkState=Any`; in that context the gateway passes the first status only. In practice, no production call site passes more than one status to the `bd search` path.

## Distinct argv shape count

Total distinct argv shapes (counting conditional-flag variants as one shape each per gateway method path): **19 shapes across 13 gateway methods**.

Collapsed to unique bd verb invocations:
1. `bd ping --json`
2. `bd ready --explain --json [--limit N]`
3. `bd query <expr> --json [-a] [--sort <field>] [--limit N]`
4. `bd count --by-status --json [--status <single-value>]` (see per-subcommand --status semantics; multi-status omits the flag and filters in-memory)
5. `bd show <id> --json`
6. `bd list --json [--all | --status <csv>] [--type <csv>] [--priority-min N] [--priority-max N] [--assignee <name>] [--label <l>]... [--limit N]`
7. `bd search <text> --json --status <token> [--type <csv>] [--priority-min N] [--priority-max N] [--assignee <name>] [--label <l>]... [--limit N]`
8. `bd ready --json`
9. `bd blocked --json`
10. `bd statuses --json`
11. `bd types --json`
12. `bd label list-all --json`
13. `bd create --json --title <title> [--description <desc>] [--type <type>] [--priority N] [--assignee <name>] [--labels <csv>]`
14. `bd update <id> [<field-flags>...]`
15. `bd close <id> [--reason <reason>]`
16. `bd comments add <id> <body>`

**16 distinct verb-level shapes.**

## Pinning test coverage

| argv shape | Covered by pinning test | Test location |
|---|---|---|
| `bd ready --explain --json` | YES | `TestBoardInitRealGatewaySubprocessArgvCardinality` in `internal/mode/board/model_test.go` |
| `bd query status=in_progress --json` | YES | same |
| `bd query status=closed --json -a --sort closed --limit 50` | YES | same + `TestBoardClosedQueryArgvLimitDynamicBoundaries` (height boundaries) |
| `bd count --by-status --json --status closed` | YES | same |
| `bd ping --json` | YES — `TestGatewayHealthCheckIssuesPingJSON` in `read_gateway_test.go` (package-internal `testRecordingExecutor`) | `internal/repository/beads/read_gateway_test.go` |
| `bd show <id> --json` | YES — argv asserted via `testRecordingExecutor` in multiple ShowIssue tests | `internal/repository/beads/read_gateway_test.go` |
| `bd show <parentID> --json` (parent sibling lookup) | YES — `TestShowIssueParentSiblingArgvShape` (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd ready --explain --json --limit N` (non-zero limit, boundaries) | YES — `TestReadyExplainArgvBoundaryLimits` (limit=1, 20, 21) (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd query <expr> --json --limit N` (dynamic limits) | YES — `TestQueryArgvBoundaryLimits` (limit=1, 50, 51) + `TestBoardClosedQueryArgvLimitDynamicBoundaries` (height-driven) (ppja.3) | `read_gateway_test.go` + `internal/mode/board/model_test.go` |
| `bd list --json --all [--limit N]` (SearchIssues empty-text path) | YES — `TestSearchIssuesEmptyTextNoWorkStateArgvShape` (limit=0,1,20,21) + `TestSearchModeInitArgvShape*` (ppja.3) | `read_gateway_test.go` + `internal/mode/search/argv_cardinality_test.go` |
| `bd list --json --status <csv> [filters]` (SearchIssues with status filter) | YES — `TestSearchIssuesStatusFilteredListArgvShape` (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd search <text> --json --status all [--limit N]` | YES — `TestSearchIssuesTextSearchArgvShape` (limit=1, 20, 21) (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd ready --json` (SearchIssues WorkState=Ready path) | YES — `TestSearchIssuesWorkStateReadyArgvShape` (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd blocked --json` (SearchIssues WorkState=Blocked path) | YES — `TestSearchIssuesWorkStateBlockedArgvShape` (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd statuses --json` | YES — `TestStatusCatalogArgvShape` (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd types --json` | YES — `TestTypeCatalogArgvShape` (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd label list-all --json` | YES — `TestLabelCatalogArgvShape` (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd count --by-status --json` (no status filter) | YES — `TestCountIssuesNoStatusFilterArgvShape` (ppja.3) | `internal/repository/beads/read_gateway_test.go` |
| `bd create --json --title <t> [optional flags]` | YES — `TestGatewayCreateIssueMapsCommandArgs` + `TestGatewayCreateIssueIncludesExplicitZeroPriority` | `internal/repository/beads/writes_test.go` |
| `bd update <id> [field flags]` | YES — `TestGatewayUpdateIssueMapsCommandArgs` + `TestGatewayUpdateIssueClearsLabelsWhenRequested` | `internal/repository/beads/writes_test.go` |
| `bd close <id> [--reason <r>]` | YES — `TestGatewayCloseIssueMapsCommandArgs` | `internal/repository/beads/writes_test.go` |
| `bd comments add <id> <body>` | YES — `TestGatewayAddCommentMapsCommandArgs` | `internal/repository/beads/writes_test.go` |

## ppja.3 backlog (shapes lacking a pinning test)

All backlog items were addressed by `beads-workbench-ppja.3`. See pinning test
coverage table above for full details.

Resolved items:

1. `ShowIssue` — parent-sibling fetch path: `TestShowIssueParentSiblingArgvShape`
2. `ReadyExplain` with non-zero limit: `TestReadyExplainArgvBoundaryLimits` (limit=1,20,21)
3. `Query` general form: `TestQueryArgvBoundaryLimits` + `TestBoardClosedQueryArgvLimitDynamicBoundaries`
4. `SearchIssues` empty-text path: `TestSearchIssuesEmptyTextNoWorkStateArgvShape` + `TestSearchModeInitArgvShape*`
5. `SearchIssues` status-filtered list: `TestSearchIssuesStatusFilteredListArgvShape`
6. `SearchIssues` text search: `TestSearchIssuesTextSearchArgvShape`
7. `SearchIssues` WorkState=Ready: `TestSearchIssuesWorkStateReadyArgvShape`
8. `SearchIssues` WorkState=Blocked: `TestSearchIssuesWorkStateBlockedArgvShape`
9. `StatusCatalog`: `TestStatusCatalogArgvShape`
10. `TypeCatalog`: `TestTypeCatalogArgvShape`
11. `LabelCatalog`: `TestLabelCatalogArgvShape`
12. `CountIssues` no status filter: `TestCountIssuesNoStatusFilterArgvShape`
13. `ListIssues` — method exposed but not called by any UI consumer; no test added (confirmed unreachable in current UI; argv pinned via `TestGatewayListIssuesBuildsCommandAndMapsSummaries`)
14. `ReadyIssues` — covered by item 7 (`bd ready --json` through SearchIssues)
15. `BlockedIssues` — covered by item 8 (`bd blocked --json` through SearchIssues)
16. `HealthCheck` — already pinned via package-internal `testRecordingExecutor` (`TestGatewayHealthCheckIssuesPingJSON`); import-cycle workaround still in place

Per epic `beads-workbench-ppja` tightening note: dynamic flags (`--limit`) must be
pinned at default + max + min + 1 boundary value, not just the common case.

## Discovered discrepancies

None found during this audit. The log evidence (build 0.4.0) showed `bd list` for
board data loading; the current codebase correctly uses `bd ready --explain` and
`bd query`. This is an expected version change, not a bug.

No argv shape was found to differ from what `interface.go` documents.
