# Beads Workbench Implementation Plan

> Repository: **`hk9890/beads-workbench`** (private initially)
>
> Go module: **`github.com/hk9890/beads-workbench`**
>
> Binary: **`bwb`**
>
> Local implementation repo: **`/home/hans/dev/github/beads-workbench`**
>
> Current donor/reference repo: **`/home/hans/dev/github/perles`**

This document explains how to implement the standalone Beads Workbench direction defined in:

- [PRODUCT.md](./PRODUCT.md)
- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [EXECUTION-PLAN.md](./EXECUTION-PLAN.md)

Use those documents as the source of truth for scope, design intent, and the initial execution breakdown. This document is about **execution strategy**.

## Implementation Approach

Beads Workbench should be implemented as a **breaking simplification**, not as an attempt to preserve current Perles feature parity.

That means:

- prefer replacement over compatibility shims
- prefer smaller interfaces over preserving old abstractions
- reuse isolated UI parts where helpful
- avoid keeping SQL/BQL/orchestration code in the critical path

The intended mental model is:

> **fresh house, salvage the good doors and windows**

In practice, that means:

- build a new Beads Workbench codebase around the new architecture
- transplant only the pieces that still fit the new design
- leave behind systems that exist only to support the old product shape

## Repository Strategy

The preferred implementation path is a **new standalone GitHub repository**, not an in-place rewrite of the current Perles repository.

### Why a new repo

- the new product is intentionally smaller and differently scoped
- the old repository carries strong SQL/BQL/orchestration assumptions
- a new repo reduces pressure to preserve legacy architecture just because it is present
- the standalone fork should have its own identity, release flow, module path, and binary name

### Recommended repository setup

- create a new standalone repository once naming is finalized
- do **not** rely on the GitHub fork button as the long-term project identity
- carry forward required license and provenance notices
- keep the current Perles repository available as a donor/upstream reference during migration

The current donor repository is located at `/home/hans/dev/github/perles`.

### Practical consequence

Core Beads Workbench implementation should happen in the new repository.

This repository remains useful for:

- planning
- design discussion
- identifying reusable code
- selectively transplanting components into Beads Workbench

The new implementation repository is located at `/home/hans/dev/github/beads-workbench`.

## Guiding Rules for Implementers

1. **Do not reintroduce direct SQL as a convenience shortcut**
   - If a feature needs issue data, route it through the gateway.

2. **Do not preserve old architecture just because it already exists**
   - Old mode/service shapes can be replaced if they pull SQL/BQL assumptions forward.

3. **Prefer launcher integrations to embedded automation systems**
   - If a workflow can be handled by opening `nvim`, `opencode`, `tmux`, or another command, prefer that over building orchestration logic into Beads Workbench.

4. **Keep dashboard rendering separate from dashboard definition loading**
   - v1 uses built-ins; later file-backed definitions can plug into the same renderer.

5. **Treat editor handoff as a primary flow, not an edge case**
   - Rich issue editing should work well through the external editor path.

6. **Implement in the new Beads Workbench repo, not by evolving Perles in place**
   - Use the current repository as a source of reusable components, not as the long-term home of the new architecture.

## Bootstrap Order

Before major implementation begins, the work should proceed in this order:

1. finalize naming, repository, module path, and binary identity
2. create the new private GitHub repository `hk9890/beads-workbench`
3. copy the Beads Workbench design docs into the new repository
4. bootstrap the new module, app shell, and package layout
5. implement the gateway seam first
6. transplant or rewrite UI pieces against that seam

This order is preferred over building the new architecture inside the old repository and moving it later.

## Recommended Delivery Phases

### Phase 0: carve out the standalone direction

Goal:

- establish Beads Workbench docs, naming direction, and scoping decisions

Artifacts:

- product definition
- architecture definition
- implementation plan

Status:

- this document set is that phase

### Phase 0.5: bootstrap the new standalone repository

Goal:

- create the real implementation home for Beads Workbench before major feature work begins

Deliverables:

- new standalone GitHub repository `hk9890/beads-workbench`
- new `go.mod` with module path `github.com/hk9890/beads-workbench`
- minimal `cmd/bwb` entrypoint
- copied Beads Workbench product / architecture / implementation docs
- initial package layout for gateway, app, modes, launcher, dashboard, and UI

Important note:

- do not start the real rewrite by expanding the old Perles architecture further
- once the new repo exists, treat this repo primarily as a donor/reference

### Phase 1: introduce the new backend seam

Goal:

- create the standalone app’s central `BeadsGateway` interface
- provide a first `bd` CLI implementation

Deliverables:

- gateway interface package
- command runner / CLI adapter
- JSON decoding and error normalization
- tests around official command parsing and failure behavior

Important note:

- this is the foundation that all later UI work should build on
- do **not** build new screens directly on subprocess calls

### Phase 2: build a minimal Beads Workbench app shell

Goal:

- stand up a small Bubble Tea app that depends only on the new gateway and UI services

Suggested features:

- top-level mode switching between board and search
- selected issue details pane
- toasts / errors / loading states

Deliverables:

- simplified root app model
- simplified shared services container
- no orchestration wiring
- no BQL executor injection

### Phase 3: deliver the browsing experience

Goal:

- make Beads Workbench useful as a daily issue browser

Suggested initial screens:

- built-in board/dashboard
- search view with basic structured filtering
- issue detail view

Implementation rule:

- board columns must map to supported gateway queries, not arbitrary query strings

### Phase 4: deliver the editor and launcher story

Goal:

- make Beads Workbench useful as a tool hub, not just a viewer

Deliverables:

- external editor flow for issue editing
- quick metadata actions
- launcher actions for external commands
- issue-context interpolation into launcher commands

Examples:

- open issue in `nvim`
- open a new terminal tab for the current issue
- start an `opencode` session for the current issue

### Phase 5: add extension points without expanding scope too early

Goal:

- prepare for future flexibility without pulling that complexity into v1

Good candidates:

- file-backed dashboard definition provider
- federated multi-project read aggregation above per-project gateways
- optional local caching layer
- small local filter language

Bad candidates for this phase:

- bringing back BQL
- reviving orchestration
- adding a direct SQL fast path

## Suggested Package Direction

Exact names may change, but the shape should resemble:

```text
internal/
  app/                 # simplified Bubble Tea shell
  domain/              # Beads Workbench issue and dashboard models if needed
  gateway/beads/       # UI-facing interface + CLI-backed implementation
  launcher/            # external editor + command launch actions
  dashboard/           # dashboard definitions + provider interfaces
  mode/                # board/search/details controllers
  ui/                  # reusable rendering components
```

## Current Perles Reuse Strategy

### Likely reusable with adaptation

- `internal/ui/board/*`
- `internal/ui/details/*`
- `internal/ui/shared/*` primitives such as modal/toaster/editor helpers
- issue markdown helpers used for editor handoff
- Bubble Tea app patterns from `internal/app`

### Likely reusable only after refactoring

- current mode controllers
- issue editor modal pieces
- config handling

These areas are often entangled with BQL, SQL, or old service assumptions and should be reused selectively.

### Likely not worth carrying forward

- `internal/orchestration/*`
- `internal/bql/*`
- SQL/Dolt server client code under current beads infrastructure
- compatibility logic dedicated to server-only startup assumptions

## Migration Strategy from Current Repo

Recommended approach:

1. keep the new docs as the design baseline
2. create the new Beads Workbench repository
3. bootstrap the new module and package layout there
4. introduce the new gateway seam in the new repo
5. build a reduced app shell in the new repo
6. migrate or rewrite screens one by one against the new seam
7. transplant only reusable UI and editor components from Perles
8. leave SQL/BQL/orchestration systems behind unless a specific isolated helper is worth extracting

This is safer than trying to convert the old architecture in place all at once.

## First Code to Transplant

When the new repo is ready, the best early transplant candidates are:

- external editor helpers
- issue markdown rendering/parsing helpers
- reusable UI primitives such as modal, toaster, and shared styles
- board and detail rendering pieces that can be detached from SQL/BQL assumptions

Recommended donor paths in the current Perles repository:

- `/home/hans/dev/github/perles/internal/ui/shared/vimtextarea/external_editor.go`
- `/home/hans/dev/github/perles/internal/ui/shared/editor/issue_markdown.go`
- `/home/hans/dev/github/perles/internal/ui/details/details.go`
- `/home/hans/dev/github/perles/internal/ui/board/board.go`
- `/home/hans/dev/github/perles/internal/ui/shared/modal/`
- `/home/hans/dev/github/perles/internal/ui/shared/toaster/`
- `/home/hans/dev/github/perles/internal/ui/styles/`

These are the parts most likely to provide leverage without pulling the old architecture into the new codebase.

## Code to Leave Behind Initially

Do not start by copying:

- orchestration packages
- BQL packages
- SQL-backed beads infrastructure
- current app-wide service wiring
- current mode controllers that still depend on executor/query contracts

Important donor paths to avoid as starting points:

- `/home/hans/dev/github/perles/internal/orchestration/`
- `/home/hans/dev/github/perles/internal/bql/`
- `/home/hans/dev/github/perles/internal/beads/infrastructure/`
- `/home/hans/dev/github/perles/internal/app/app.go`

Those systems should be treated as legacy unless a very specific, isolated piece proves reusable.

## Search Implementation Guidance

In v1, search should avoid a full local indexing requirement.

Recommended sequence:

1. use official beads command filters first
2. support text search and common structured filters
3. optionally allow local narrowing of already loaded results
4. defer a dedicated mini-language until the browsing experience is proven

If later work adds a local filter language, it should remain layered on top of the gateway-backed result model described in [ARCHITECTURE.md](./ARCHITECTURE.md).

## Future Federation Guidance

If Beads Workbench later gains federated mode, implement it as a layer **above** per-project gateways rather than by changing the core gateway into a global multi-project abstraction.

Recommended order:

1. keep the v1 gateway source-specific
2. add a source registry / source selection layer
3. add federated read aggregation for browse and search flows
4. keep writes routed through the selected issue's owning source

This preserves the single-project design while allowing multi-project expansion later.

## Dashboard Implementation Guidance

In v1:

- implement built-in dashboard definitions only
- keep the board renderer independent from the provider
- model each board section as a supported gateway query type

Do not:

- store arbitrary query strings as the core abstraction
- make the renderer depend on BQL parsing

## External Editor Implementation Guidance

The editor handoff should likely use a temp-file flow:

1. gateway loads issue details
2. Beads Workbench renders an editable issue document
3. external editor opens the document
4. Beads Workbench parses the saved result
5. Beads Workbench computes the changed fields
6. gateway applies the update

Good reuse candidates in the current repo:

- external editor command helpers
- issue markdown parsing/rendering helpers

The editor document format should remain stable and human-readable.

## Launcher Implementation Guidance

Launchers should be simple command templates with issue context substitution.

Possible context fields:

- issue ID
- title
- status
- labels
- project root

Example launcher uses:

- `nvim`
- `tmux new-window`
- terminal tab opener
- `opencode` command starter

Launchers should stay intentionally thin. If a launcher starts doing workflow coordination, move that concern back out of Beads Workbench.

## Documentation Expectations for Future Tasks

Future implementation tasks should reference these docs directly:

- use [PRODUCT.md](./PRODUCT.md) for feature scope decisions
- use [ARCHITECTURE.md](./ARCHITECTURE.md) for gateway/provider/launcher boundaries
- use this document for phase ordering and reuse guidance

If future work changes scope, update the product and architecture docs before implementation continues.

## Definition of Success for Initial Implementation

The initial Beads Workbench implementation is successful when:

- the active product path no longer depends on SQL or BQL
- the app can browse issues through official beads surfaces only
- users can open, inspect, comment on, close, create, and update issues
- rich editing through an external editor works cleanly
- external tool launching is part of the normal workflow
- built-in dashboards are useful without custom query authoring
- orchestration remains out of the product
