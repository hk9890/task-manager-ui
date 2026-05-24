# Beads Workbench Execution Plan

> Repository: **`hk9890/beads-workbench`** (private initially)
>
> Go module: **`github.com/hk9890/beads-workbench`**
>
> Binary: **`bwb`**
>
> Local implementation repo: **`/home/hans/dev/github/beads-workbench`**
>
> Current donor/reference repo: **`/home/hans/dev/github/perles`**

This document translates the product and architecture direction into a concrete beads planning shape for the new repository.

> **Historical Phase 1 planning doc.** The vocabulary below reflects the original design intent and no longer matches the implementation. Current state is described in `docs/OVERVIEW.md` — the runtime stack now uses `repository.Repository` backed by the lean `beads.Repository` built directly on `CommandRunner`.

Use this file when creating the initial beads epic and tasks in `/home/hans/dev/github/beads-workbench`.

Source documents for this plan:

- [PRODUCT.md](./PRODUCT.md)
- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [IMPLEMENTATION.md](./IMPLEMENTATION.md)

## Current Starting State

Already true:

- the new GitHub repository exists: `hk9890/beads-workbench`
- the new local repository exists: `/home/hans/dev/github/beads-workbench`
- the current donor/reference project remains available at `/home/hans/dev/github/perles`
- the docs in this folder are intended to be copied into the new repository before implementation proceeds

## Planning Goal

Create a proper beads plan in the new repository that starts execution without pulling old Perles architecture forward.

The first implementation wave should establish:

- repo bootstrap in the new codebase
- the repository seam
- the CLI-backed beads adapter
- core domain and error models
- enough foundation to begin the minimal app shell afterward

## Recommended Phase 1 Epic

### Epic Title

**Bootstrap Beads Workbench foundation and official bd repository seam**

### Epic Description

Create the new repository foundation for Beads Workbench and implement the first production-ready backend seam that talks only to official beads surfaces. This epic should end with a tested gateway layer that the future UI can depend on directly, without SQL, BQL, or orchestration code in the critical path.

### Epic Success Criteria

- the new repository has the baseline module, binary entrypoint, and copied design docs
- a source-specific `repository.Repository` implementation exists in the new repo
- a CLI-backed implementation can perform core reads and writes through `bd`
- repository errors are normalized into UI-usable categories
- tests cover command construction, output decoding, and failure behavior
- no direct SQL/BQL dependency exists in the active foundation path

## Recommended Phase 1 Task Breakdown

### Task 1 — Bootstrap Beads Workbench repository foundation

#### Purpose

Set up the new repository so implementation can begin in `/home/hans/dev/github/beads-workbench` instead of continuing inside `/home/hans/dev/github/perles`.

#### Scope

- copy `docs/standalone/` into the new repo
- create `go.mod` with `github.com/hk9890/beads-workbench`
- add minimal `cmd/bwb` entrypoint
- create initial package layout for app, gateway, launcher, dashboard, mode, and ui
- add minimal README/provenance scaffolding as needed

#### Acceptance Criteria

- the new repo builds a minimal `bwb` entrypoint
- the docs folder exists in the new repo
- package layout exists for the planned architecture

### Task 2 — Define core domain and gateway interfaces

#### Purpose

Create the code contracts that everything else will build on.

#### Scope

- define issue summary/detail models appropriate for Beads Workbench
- define query input models for list/ready/blocked/show flows
- define mutation input models for create/update/close/comment flows
- define normalized repository error types
- define the source-specific `repository.Repository` interface

#### Acceptance Criteria

- repository interface is source-specific, not global/federated
- core read/write models are defined without SQL-specific leakage
- error model is suitable for future TUI use

### Task 3 — Implement shared command runner and CLI decoding helpers

#### Purpose

Avoid scattering subprocess logic throughout the codebase.

#### Scope

- implement a reusable `bd` command runner
- support working-directory / environment propagation
- support JSON command execution helpers
- normalize process errors, stderr, and exit failures
- build test seams for command execution

#### Acceptance Criteria

- gateway implementation can use one shared command execution layer
- command behavior is testable without fragile end-to-end shell dependence

### Task 4 — Implement gateway read operations through official beads commands

#### Purpose

Deliver the first useful read path for the future UI.

#### Scope

- implement `ListIssues`
- implement `ReadyIssues`
- implement `BlockedIssues`
- implement `ShowIssue`
- map official command JSON into Beads Workbench models

#### Recommended command baseline

- `bd list --json`
- `bd ready --json`
- `bd blocked --json`
- `bd show --json`

#### Acceptance Criteria

- the gateway can successfully return typed read models
- read operations do not depend on SQL or BQL

### Task 5 — Implement gateway write operations through official beads commands

#### Purpose

Complete the first end-to-end backend seam.

#### Scope

- implement create issue
- implement update issue
- implement close issue
- implement add comment
- support only the official command surface

#### Recommended command baseline

- `bd create`
- `bd update`
- `bd close`
- `bd comments add`

#### Acceptance Criteria

- write operations are available through the gateway interface
- error behavior is consistent with the read-path error model

### Task 6 — Verify gateway behavior with tests and fixture coverage

#### Purpose

Make the gateway safe enough for UI work to begin.

#### Scope

- test command building
- test JSON decoding
- test failure cases
- test representative read and write flows
- document known limitations if a command surface lacks required data

#### Acceptance Criteria

- the gateway layer has focused automated tests
- failures produce actionable errors for later UI integration

### Task 7 — Acceptance Review: Phase 1 gateway foundation

#### Purpose

Verify that the repository and backend seam are ready for the minimal app-shell phase.

#### Acceptance Criteria

- all phase-1 implementation tasks are closed
- `bwb` entrypoint exists in the new repository
- the new code path depends on official beads surfaces only
- no direct SQL/BQL/orchestration dependency is required for the gateway foundation
- tests for the gateway foundation pass

## Recommended Follow-up Epics After Phase 1

These should usually be separate epics in the new repository after the gateway foundation is complete.

### Epic 2 — Build minimal Beads Workbench app shell

Purpose:

- add Bubble Tea root app
- add simplified services container
- wire the UI to the new gateway

### Epic 3 — Deliver browsing experience

Purpose:

- built-in board/dashboard
- search/browse
- issue details

### Epic 4 — Deliver editor and launcher flows

Purpose:

- external editor integration
- quick metadata actions
- launcher commands for issue-context workflows

### Epic 5 — Add extension points carefully

Purpose:

- optional file-backed dashboard definitions
- optional caching
- future federated read mode
- optional small local filter language

## Donor Code References

When an implementation task needs to inspect the old Perles codebase, use absolute donor paths so the implementing agent knows exactly where to look.

Useful donor references:

- `/home/hans/dev/github/perles/internal/ui/shared/vimtextarea/external_editor.go`
- `/home/hans/dev/github/perles/internal/ui/shared/editor/issue_markdown.go`
- `/home/hans/dev/github/perles/internal/ui/details/details.go`
- `/home/hans/dev/github/perles/internal/ui/board/board.go`
- `/home/hans/dev/github/perles/internal/ui/shared/modal/`
- `/home/hans/dev/github/perles/internal/ui/shared/toaster/`
- `/home/hans/dev/github/perles/internal/ui/styles/`

Do not use these as starting points for the new active architecture:

- `/home/hans/dev/github/perles/internal/orchestration/`
- `/home/hans/dev/github/perles/internal/bql/`
- `/home/hans/dev/github/perles/internal/beads/infrastructure/`
- `/home/hans/dev/github/perles/internal/app/app.go`

## Notes for the Next Agent in the New Repository

When this folder is copied into `/home/hans/dev/github/beads-workbench`, the next agent should:

1. read [PRODUCT.md](./PRODUCT.md)
2. read [ARCHITECTURE.md](./ARCHITECTURE.md)
3. read [IMPLEMENTATION.md](./IMPLEMENTATION.md)
4. read this file
5. create the proper beads epic/tasks in the new repository based on this phase-1 breakdown

The new implementation should happen in `/home/hans/dev/github/beads-workbench`.

The old Perles donor/reference codebase remains at `/home/hans/dev/github/perles`.
