# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Repository backend replaced.** bwb now reads and writes issues in-process through the [task-manager](https://github.com/hk9890/task-manager) Go SDK (`github.com/hk9890/task-manager/sdk/tasks`) instead of shelling out to the `bd` (beads) CLI. The default `--repo` backend is now `taskmgr`. Issues live as Markdown files under a `.tasks/` directory; bwb expects a `.tasks/` store in the target project (create one with `taskmgr init`).
- `--debug` repository traces are now in-process; there is no external subprocess argv in the product path.

### Removed

- `--repo beads` and `--repo caching` backends, and the caching layer itself — the in-process SDK removes the subprocess latency the cache existed to hide. The remaining backends are `taskmgr` (default) and `memory` (for tests/inspection).
- The `bd`-coupled test harness and tooling: the repository parity/contract suite, the `bwb-smoke` and `bwb-loadtest` binaries, the embedded bd fixture, the load-generation and scale-seed scripts, and the `mise` `smoke` / `test:load` / `bwb:fixture` / `verify:refresh` tasks.

## [v0.9.0]

### Added

- Detail Content pane header restyled to match the dashboard: a compact metadata row (type · priority · status, colored as on the board, with a muted issue ID), then the title, then a thin rule separating the header from the body.

### Changed

- Drilling into a related issue from the Detail Dependencies rail (Enter) now keeps focus on the Dependencies rail when the target has its own dependencies, and only moves focus to the Content pane for a leaf issue (previously focus always jumped to Content).
- Detail Content pane loading skeleton now renders prose-shaped placeholder blocks instead of board issue-row shapes; the Dependencies rail keeps its row skeleton.

### Fixed

- Async dialog-open race: pressing Esc during the status/create/update catalog-load window no longer pops Detail→Board and orphans the dialog over the Board — the pending open is cancelled and focus stays in place.
- Search "no selection" Content preview no longer renders a junk metadata row (`? P0 (NO (none)`) for the empty placeholder.

## [v0.7.0]

### Added

- Details pane now renders an epic's children, with Enter-gated navigation into related issues.
- Startup prunes stale `bwb` session log files so the log directory no longer accumulates dead sessions.

### Changed

- **Default repository backend is now `beads`** (live `bd` subprocess reads) instead of `caching`. The caching decorator has known correctness bugs (e.g. a stale persisted snapshot can serve a near-empty board — see `internal/repository/caching/testdata/known-bad-cache-fbea/`), so it is now opt-in only via `--repo caching`. No flag change is needed for the new default; existing `--repo caching` / `--repo memory` invocations are unaffected.

### Fixed

- Default mode-switch keybindings are now reachable (previously some defaults were shadowed/unbound).
- Details pane labels the owner field "Owner" and shows the Children count in the compact summary.
- Metadata-rail Counts block now includes the Children count.

## [v0.6.0]

### Added

- Done column pagination: the board's Done column now supports load-more navigation. Press `>` for explicit, or scroll past the loaded edge with `j`/`down` for implicit threshold-triggered load-more. Pages of `max(2 * sectionItemCapacity, 50)` closed issues append to the existing slice; the composer dedups by ID and recomputes `TotalIsExact`. Manual reload (`r`), focus-regain auto-refresh, and the periodic background tick all reset the loaded count to page 1.
- Selection-following scroll window in the board (Ready/Not Ready/In Progress/Done) and details panes (Dependencies, Metadata). On a column or pane with more rows than fit in the viewport, `j`/`k` keeps the selection chevron inside the visible window — no more "selection moves invisibly past the viewport" when destructive actions (`x`/`e`/`u`) might act on an off-screen row.
- New `internal/ui/scroll.EnsureVisible(offset, sel, window)` helper, shared by board and details models.

### Changed

- Board column headers now show `"N of M"` whenever the rendered window is smaller than the loaded slice — applies to Ready/Not Ready/In Progress when overflowing, and to Done with load-more. Previously these columns showed only `N`, hiding the truncation from the operator.
- `repository.DashboardOptions` gained `ClosedOffset int`; all three impls (memory, beads, caching) honour it. The beads impl emulates `--offset` via over-fetch + composer-side dedup (race-safe under concurrent closes; required because `bd` 1.0.4 lacks `--offset`).
- `caching.Repository` Dashboard passes ClosedOffset > 0 calls through to the backing repo unconditionally, then merges the returned closed page into the persisted snapshot under lock (ID dedup, incoming wins).
- Details Dependencies and Metadata panes now expose `"N of M"` in their headers when the visible window is smaller than the rendered list.
- Done column "load-more in flight" affordance: while a page fetch is pending, the renderer appends a single skeleton row at the bottom of the visible window (`Loading=true && ScrollOffset>0`).

### Fixed

- Race condition where load-more's `[offset:offset+limit]` slice over the over-fetched `bd query --sort closed --limit N` result silently overlapped with the prior page (after concurrent closes), causing the composer's ID dedup to keep the Done column at its initial size. The beads impl now returns the full over-fetched list and lets the composer perform the merge.
- `mise run quality` / `quality:fast` gates re-aligned: the parity contract scenario `PaginatedClosedFetch` now documents the per-impl divergence (exact slice for memory/caching; over-fetch superset for beads) with a single composer-merged union as the parity guarantee.

## [v0.5.2]

### Changed

- Startup now gates board section loads on the beads health check: a single `bd ping` detects a missing database instead of firing several `bd` subprocesses that all fail in a non-beads directory

### Fixed

- Persistent diagnostics now write per-process `bwb-<session_id>.log` files instead of a shared `bwb.log`, so concurrent BWB processes can no longer produce torn JSON Lines records across a rotation boundary
- `bd` command execution traces with a non-zero exit code are logged at `WARN` instead of `INFO`, so failed subprocesses surface above the routine trace stream and through stderr mirroring

## [v0.5.1]

### Fixed

- Repository emulates `CloseIssue` idempotency over a `bd` 1.0.4 close-lookup bug so repeated close attempts no longer surface as errors

## [v0.5.0]

### Added

- Cold-start skeleton UX across all three panes (board, detail, search preview) with a color-cycle pulse animation; unified `RenderCompactSkeleton` primitive for issue-row-shaped placeholders
- Non-blocking loading UX so board/search/detail surfaces remain responsive while data loads
- Startup beads health check with a fatal error screen, plus a per-section load progress counter during board startup
- Pre-release data-consistency smoke check (`mise run smoke` / `cmd/bwb-smoke`) and parity test suites for dashboard count/sort and search results
- Repository capabilities: `ReadyExplain` (via `bd ready --explain --json`), generic `Query(expr, opts)` wrapping `bd query --json`, and `bd ping --json`-backed startup health check
- Test/CI infrastructure: stratified unit vs integration tiers with a `mise` execution layer, `mise run test:integration:verbose`, `-race` enabled across the suite, `mise run bwb:fixture` task, and a `RecordingExecutor` test helper for subprocess argv assertions

### Changed

- Dashboard board data layer rewritten as 3 parallel `bd` reads plus a pure `Compose` function; board model owns query routing
- Dashboard provider collapsed to a metadata-only catalog (section IDs + titles)
- Section totals and per-section item limits are now accurate
- Repository runner split into an `RWMutex` so reads run in parallel while writes remain serialized
- Search results respect terminal height; result-pane prose wraps; query rail scales to ~30% of terminal width
- Go toolchain bumped to 1.26.3; `bd` pinned in `.mise.toml`
- `CLAUDE.md` simplified to `@AGENTS.md` reference

### Fixed

- Refresh concurrency: manual reload now guards against an in-flight refresh in board, search, and inside `startReload` / `triggerSearchWithAnchor` (defense in depth)
- Cold-start skeleton renders correctly on direct-navigation cold start (no longer shows `(no description)`)
- Cross-OS CI failures: Windows-safe file lock in the embedded fixture, Windows-specific test skips, and other cross-OS platform edges
- Tolerate transient `CloseIssue` / idempotency flake against `bd` 1.0.4 in CI; coverage gate adjusted accordingly

## [v0.4.0]

### Added

- Structured logging foundation using `slog` with per-component loggers and runtime diagnostics routed through a central log manager
- Monitoring documentation (`docs/MONITORING.md`) recording the logging architecture and stderr/debug diagnostics model
- Search result metadata: completeness classification (exact/capped/partial), source tracking, and backend notices surfaced in the results panel
- Inline search reload — search results remain visible while a new query runs in the background

### Changed

- Search query pane now shows a status-aware multi-line summary (applied query, draft changes, reload state) instead of truncating raw query text
- Search results panel renders contextual banners for reload-in-progress, stale results on refresh failure, and completeness hints
- Empty search states use specific messages that distinguish "no search has run yet" from "no matches for this query"
- Result count badge in the results header includes a completeness label (exact / capped / partial) when available
- Startup `logManager` is now initialized after CWD validation, eliminating guarded nil checks in the early-exit paths

### Fixed

- Startup error logging now uses the resolved `startupLogger` consistently instead of conditionally falling back to stderr after the log manager was already available

## [v0.3.0]

### Added

- Current-state monitoring/logging plan documenting BWB's existing stderr/debug diagnostics model

### Changed

- Detail metadata quick actions now render the active configured key labels instead of hard-coded shortcuts
- Search preview keeps the search-mode add-comment shortcut while reusing the shared detail metadata renderer
- Detail panes now omit empty notes/comments sections when there is nothing meaningful to show

### Fixed

- Detail comments now render newest-first with better framing/elision for long log-like output
- ANSI-aware string truncation now preserves well-formed styling in compact rows and pane content

## [v0.2.0]

### Added

- Markdown inspector rendering in issue detail to improve readability for rich descriptions
- Wide related-issues rail in issue detail for improved dependency/context scanning

### Changed

- Reworked issue detail layout with a denser metadata rail and improved dependency presentation
- Unified search and detail live preview behavior for more consistent issue navigation
- Search interaction now prioritizes query-first navigation while keeping result/preview focus transitions predictable
- Active board/search/detail views refresh more consistently with focus-aware auto-refresh behavior

### Fixed

- Installable release archives now use names compatible with tool auto-detection (for example `mise`)
- Detail metadata action handling and selection behavior are more stable during navigation and interaction

## [v0.1.0]

### Added

- First release of **bwb**, a terminal UI for browsing and managing beads issues as the standalone successor to Perles
- Board view with Kanban-style columns grouped by status and keyboard navigation across columns and rows
- Search view with full-text issue search and a three-pane layout (query, results, preview) that shows all issues by default
- Detail view with rich issue rendering for description, metadata, dependencies, related issues, and comments
- Tab/hotkey mode switching between board, search, and detail views
- `$EDITOR`-based rich issue editing with marker-based document round-trip
- Configurable external launcher system (nvim, opencode, shell commands) with issue context interpolation
- Runtime YAML configuration for editor, launchers, keybindings, and UI preferences (`~/.config/bwb/config.yaml`)
- Context-sensitive help overlay for keybinding reference
- Architecture foundations: `bd` CLI repository abstraction (no direct SQL or Dolt internals), Bubble Tea TUI with controller/view separation, and automated architecture guardrail tests
