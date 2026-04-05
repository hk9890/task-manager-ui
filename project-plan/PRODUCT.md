# Beads Workbench Product Definition

> Product name: **Beads Workbench**
>
> Repository: **`hk9890/beads-workbench`** (private initially)
>
> Go module: **`github.com/hk9890/beads-workbench`**
>
> Binary: **`bwb`**
>
> Local implementation repo: **`/home/hans/dev/github/beads-workbench`**
>
> Current donor/reference repo: **`/home/hans/dev/github/perles`**

Beads Workbench is the planned standalone successor to Perles: a smaller, more stable beads TUI focused on browsing work, inspecting issues, and handing work off to external tools.

This document defines the intended product scope for the standalone fork. It is the source of truth for **what the product is**. For **how it is built**, see [ARCHITECTURE.md](./ARCHITECTURE.md). For **how to implement it**, see [IMPLEMENTATION.md](./IMPLEMENTATION.md).

These docs are intended to be copied into the new Beads Workbench repository. Keep references self-contained within this folder, and when referring to the current Perles donor codebase use the absolute path `/home/hans/dev/github/perles`.

## Product Summary

Beads Workbench is a terminal UI for beads users who want:

- a fast way to browse issues and common work queues
- a clear issue detail view
- simple create/update/close/comment flows
- easy handoff into external tools such as `nvim`, `opencode`, `tmux`, or a terminal launcher

Beads Workbench is **not** trying to be a general orchestration system, a SQL-backed analytics UI, or a power-user query workbench.

## Product Principles

1. **Official beads surfaces only**
   - Beads Workbench talks to beads through official `bd` CLI/API surfaces.
   - No direct SQL backend is part of the product design.

2. **Embedded-mode compatibility first**
   - The default beads experience must work well.
   - The product should behave correctly in beads repositories initialized in embedded Dolt mode.

3. **Tool launcher, not tool orchestrator**
   - Beads Workbench should help users start external tools.
   - Beads Workbench should not own a complex internal multi-agent or control-plane system.

4. **Opinionated over infinitely flexible**
   - Built-in workflows are preferred over a large customization surface.
   - Simpler, stable behavior is more important than feature parity with Perles.

5. **Editor-friendly**
   - Deep issue editing should feel natural for people who work in `nvim` or another `$EDITOR`.
   - The TUI should make editor handoff easy instead of forcing every edit into modal forms.

## Core User Workflows

### 1. Browse work

Users can open Beads Workbench and immediately see useful queues such as:

- ready work
- active/in-progress work
- blocked work
- recently updated work

These views are built-in and do not require BQL or custom board authoring.

### 2. Inspect issue details

Users can select an issue and view:

- title, ID, status, priority, type, assignee, labels
- description and notes
- dependencies / related issue references when available from official beads surfaces
- comments / thread context

### 3. Search and narrow work

Users can search for issues without needing BQL.

Initial scope:

- text search UX
- common structured narrowing through built-in filters such as status, type, priority, assignee, labels, ready, and blocked

The initial product does **not** require a custom query language.

### 4. Create or update issues

Users can:

- create issues
- update issue metadata
- close issues
- add comments

Preferred editing flow:

- open the current issue in an external editor such as `nvim`
- edit a markdown or structured issue buffer
- save and return to Beads Workbench
- Beads Workbench parses the result and applies the change through official beads commands

Inline quick actions may still exist for small metadata changes, but the product should optimize for editor handoff rather than modal complexity.

### 5. Launch external tools from the current issue

Beads Workbench should make it easy to start other tools from the selected issue context.

Examples:

- open issue in `nvim`
- launch an `opencode` session in a new tab/window for the current issue
- open a shell command or `tmux` window with issue ID/title injected

This is a first-class product direction: **Beads Workbench launches tools; it does not reimplement them.**

## v1 Feature Scope

### Included

- beads-backed issue browser
- fixed dashboard / board views for common work states
- search and browse flows without BQL
- issue detail view
- issue create / update / close / comment actions
- external editor handoff for rich issue editing
- external tool launch actions tied to the current issue
- a single backend model based on official beads surfaces only
- built-in dashboard definitions

### Deliberately Excluded

- direct SQL reads
- a native BQL parser or executor
- the existing orchestration / control-plane system
- user-authored dashboard configuration in v1
- feature parity with current Perles query flexibility
- a mandatory full in-memory index of every issue for v1

### Deferred / Later Candidates

- a small local filter language (the old “option 2” direction)
- file-backed custom dashboard definitions
- federated multi-project browsing across multiple beads sources
- richer launcher configuration
- optional local caching for responsiveness
- richer dependency navigation if official surfaces support it cleanly

## Non-Goals

Beads Workbench is not intended to:

- preserve the current Perles orchestration engine
- be a general-purpose beads SQL client
- expose every existing Perles feature in the first standalone release
- require custom query authoring to be useful
- depend on unsupported or unofficial beads internals

## Dashboard Product Model

Beads Workbench v1 ships with built-in dashboard definitions only.

Important product decision:

- **dashboard rendering and dashboard definition loading are separate concerns**

This allows Beads Workbench to start with built-in dashboards while leaving room for a future implementation that loads dashboard definitions from a file or another provider.

## Editing Product Model

The preferred Beads Workbench editing experience is:

1. select an issue
2. press the edit action
3. Beads Workbench opens the issue in the configured editor (`$EDITOR`, often `nvim`)
4. the user edits and saves
5. Beads Workbench parses the edited content and applies changes through official beads operations

This flow is intentional. Beads Workbench should feel like a strong terminal companion for editor-driven users.

## External Tool Integration Model

Beads Workbench should support launcher-style actions that operate on the selected issue.

Examples of issue context passed to launchers:

- issue ID
- title
- labels
- assignee
- working directory / project root

This enables integrations such as:

- `opencode` issue implementation session
- opening a shell or terminal tab
- starting repo-local scripts

## Success Criteria for the Standalone Product Direction

Beads Workbench succeeds if it becomes:

- simpler than Perles
- more stable than the current SQL/BQL-centered architecture
- compatible with default beads embedded-mode setups
- useful as an everyday issue browser and launcher hub
- easy to extend later without reintroducing orchestration or SQL complexity
