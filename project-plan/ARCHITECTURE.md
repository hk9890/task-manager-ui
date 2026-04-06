# Beads Workbench Architecture

> Repository: **`hk9890/beads-workbench`** (private initially)
>
> Go module: **`github.com/hk9890/beads-workbench`**
>
> Binary: **`bwb`**
>
> Local implementation repo: **`/home/hans/dev/github/beads-workbench`**
>
> Current donor/reference repo: **`/home/hans/dev/github/perles`**

This document defines the target architecture for Beads Workbench, the standalone successor to Perles.

For product intent, see [PRODUCT.md](./PRODUCT.md). For implementation sequencing, see [IMPLEMENTATION.md](./IMPLEMENTATION.md). For the concrete initial beads planning breakdown, see [EXECUTION-PLAN.md](./EXECUTION-PLAN.md).

## Architectural Intent

Beads Workbench is a smaller beads TUI built around one core technical idea:

> the UI talks to a **single official-beads gateway**, and everything else is layered on top of that

This architecture deliberately avoids the current Perles design assumptions around:

- direct SQL reads
- BQL execution
- Dolt server discovery and reconnection logic
- internal orchestration/control-plane services

## System Overview

Beads Workbench should be composed of five main areas:

1. **App shell**
   - Bubble Tea root model
   - mode switching
   - shared notifications and layout

2. **Feature modes**
   - board/dashboard browsing
   - search/browse
   - issue details
   - create/update/comment flows

3. **Gateway layer**
   - the single UI-facing interface for all beads reads and writes
   - implemented through official `bd` commands / official API surfaces only

4. **Launcher layer**
   - editor handoff
   - external tool launch actions
   - terminal/tab/tmux integration

5. **Definition providers**
   - built-in dashboard definitions in v1
   - future file-backed dashboard definition provider

## Core Rule: No Direct SQL

Beads Workbench must not depend on direct SQL access for primary product behavior.

Consequences:

- no MySQL/Dolt SQL client as the main read path
- no startup logic that assumes `dolt_mode=server`
- no BQL executor that requires `*sql.DB`
- no SQL schema inspection as a precondition for normal UI behavior

This rule exists to keep Beads Workbench aligned with official beads behavior and with embedded-mode compatibility.

## Core Interface: Beads Gateway

The UI should depend on a single interface, referred to here as the **Beads Gateway**.

Example responsibilities:

- list issues with supported filters
- load ready work
- load blocked work
- show full issue details
- create issue
- update issue
- close issue
- add comment
- load statuses / types / labels as needed for UI controls

Representative shape:

```go
type BeadsGateway interface {
    ListIssues(ctx context.Context, q ListIssuesQuery) ([]IssueSummary, error)
    ReadyIssues(ctx context.Context, q ReadyQuery) ([]IssueSummary, error)
    BlockedIssues(ctx context.Context, q BlockedQuery) ([]BlockedIssue, error)
    ShowIssue(ctx context.Context, id string) (IssueDetail, error)

    CreateIssue(ctx context.Context, input CreateIssueInput) (CreateResult, error)
    UpdateIssue(ctx context.Context, id string, input UpdateIssueInput) error
    CloseIssue(ctx context.Context, id string, reason string) error
    AddComment(ctx context.Context, id string, input AddCommentInput) error

    StatusCatalog(ctx context.Context) ([]StatusOption, error)
    TypeCatalog(ctx context.Context) ([]TypeOption, error)
}
```

The exact method names can differ, but the architectural rule should remain: **the UI never shells out directly and never talks to SQL directly; it talks to the gateway.**

## Gateway Implementation Strategy

The initial implementation should be a `bd` CLI adapter.

It should translate gateway operations into official commands such as:

- `bd list --json`
- `bd ready --json`
- `bd blocked --json`
- `bd show --json`
- `bd create`
- `bd update`
- `bd close`
- `bd comments add`

The adapter is responsible for:

- command construction
- environment propagation (`BEADS_DIR`, actor, etc.)
- JSON parsing
- domain mapping
- error normalization for the UI

## Future Federated Mode

Beads Workbench v1 is single-project first, but the architecture should leave room for a future federated mode that can browse multiple beads projects.

### Design rule

- treat a gateway instance as bound to **one beads source/project**
- do not assume the entire app will always talk to exactly one global source forever

This allows a later federation layer to sit above the per-project gateway without changing the core UI-to-gateway contract.

### Preferred future shape

Later federation should add a higher-level layer such as:

- source registry
- workspace/source selection
- federated read aggregation

Representative model:

```go
type SourceRef struct {
    ID   string
    Name string
    Root string
}

type FederatedGateway interface {
    Sources(ctx context.Context) ([]SourceRef, error)
    QueryAcrossSources(ctx context.Context, q FederatedQuery) ([]FederatedIssue, error)
}
```

### Scope expectation

Federated mode should start as **read aggregation first**:

- browse multiple projects
- search across multiple projects
- show which source/project an issue belongs to

Write behavior should remain source-specific. Once a user selects a concrete issue, Beads Workbench can route follow-up actions through that issue's owning source gateway.

This keeps federation compatible with the single-source architecture rather than forcing a separate product design.

## Data Loading Model

Beads Workbench v1 should **not** assume a full in-memory issue index.

The default data model should be:

- fetch what the active screen needs
- prefer official beads filtering where possible
- load details lazily for the selected issue
- keep caching optional and shallow

This avoids a premature requirement to mirror the whole issue database into memory.

### Examples

- board columns load from targeted gateway queries
- search uses gateway filtering first, not a mandatory local index
- issue details load on selection
- comments and dependency context can be lazy-loaded if needed

## Board / Dashboard Architecture

Beads Workbench keeps the concept of dashboards or boards, but simplifies how they are produced.

### Architectural decision

- **dashboard renderer** and **dashboard definition provider** are separate

That means:

- the renderer knows how to display columns / sections / queues
- the provider decides which definitions exist

Representative model:

```go
type DashboardDefinitionProvider interface {
    Dashboards(ctx context.Context) ([]DashboardDefinition, error)
}
```

In v1:

- use a built-in provider with hard-coded dashboard definitions

Later:

- add a file-backed provider without rewriting the board renderer

### Important limitation

Dashboard definitions should map to supported gateway query types, not arbitrary BQL.

Examples of supported built-in queue types:

- ready
- blocked
- in-progress
- open by status
- recent updates
- assigned to current user

## Search Architecture

Search in Beads Workbench is intentionally smaller than Perles search.

v1 search should be built from:

- text input
- structured filters exposed by the gateway
- optional local narrowing of the currently loaded result set

### Search filter contract note (Phase 1)

The gateway-level search query model includes `priority` and work-state narrowing
(`ready` / `blocked`) so UI layers can issue one structured query contract.

- Priority maps directly to official `bd search` filters (`--priority-min` / `--priority-max`).
- Ready/blocked do not have direct `bd search` flags today. Gateway implementations should
  route through official queue commands (`bd ready --json` / `bd blocked --json`) and apply
  any additional structured filters in-memory rather than requiring UI code to shell out.

Search should **not** depend on:

- BQL parsing
- SQL query generation
- generalized expression evaluation

## Issue Detail Architecture

Issue detail views should render a stable read model, ideally with:

- summary metadata
- markdown-like content rendering for description/notes
- comments
- dependency / related-work context when available from official beads surfaces

The detail view may reuse rendering components from the current repo, but it must not depend on the old BQL executor contract.

## Editing Architecture

Beads Workbench has two editing paths:

1. **quick updates**
   - direct metadata operations through the gateway
   - good for status, priority, labels, close, comments

2. **external editor flow**
   - preferred rich-edit path
   - opens a temp buffer representing the issue
   - user edits in `$EDITOR` / configured editor
   - Beads Workbench parses the result and sends only supported changes back through the gateway

Representative abstraction:

```go
type IssueEditor interface {
    Edit(ctx context.Context, issue IssueDetail) (EditedIssue, error)
}
```

The initial implementation can wrap existing external editor helpers from the current repo.

## Launcher Architecture

Launchers are a first-class part of Beads Workbench.

They should be separate from the beads gateway because they operate on issue context, not on beads persistence.

Representative abstraction:

```go
type IssueLauncher interface {
    Launch(ctx context.Context, action LaunchAction, issue IssueDetail) error
}
```

Launchers may support commands such as:

- open in editor
- open a new terminal/tab
- run an opencode command for the issue
- invoke repo-local scripts

### Design rule

Launchers should receive a structured issue context and produce a subprocess launch.

They should not become a hidden orchestration engine.

## Error Handling Model

The gateway and launcher layers should normalize errors into UI-meaningful categories such as:

- beads command unavailable
- command failed
- parse failure on beads output
- editor launch failed
- launcher command failed

Beads Workbench should prefer clear, actionable errors over silent retries or complex reconnect systems.

## Configuration Model

Beads Workbench should start with a much smaller configuration surface than Perles.

Expected v1 configuration areas:

- editor command / `$EDITOR` fallback
- launcher definitions
- optional dashboard provider source selection later
- basic UI preferences

It should not start with the old custom view / BQL / orchestration configuration burden.

## Reuse Boundaries from Current Perles

Good reuse candidates:

- Bubble Tea shell patterns
- board rendering widgets
- detail rendering widgets
- modal / toaster / style primitives
- external editor helpers
- issue markdown formatting/parsing utilities

When these are copied from the current donor repository, reference the donor files by absolute path under `/home/hans/dev/github/perles`.

Poor reuse candidates:

- SQL/Dolt server client code
- BQL parser/executor/validator
- orchestration control plane
- current dashboard/query loading assumptions

## Explicit Architectural Exclusions

The Beads Workbench architecture excludes:

- `internal/orchestration/*` as a product subsystem
- direct dependency on `database/sql` for primary issue access
- runtime dependency on `dolt_mode=server`
- custom BQL-based dashboard execution

These exclusions are part of the design, not temporary omissions.
