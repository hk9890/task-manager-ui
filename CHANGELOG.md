# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
- Architecture foundations: `bd` CLI gateway abstraction (no direct SQL or Dolt internals), Bubble Tea TUI with controller/view separation, and automated architecture guardrail tests
