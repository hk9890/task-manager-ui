# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
