# Monitoring / Logging Plan

## Purpose

This document summarizes **what Beads Workbench has today** for logging and
machine-visible diagnostics. It is a current-state snapshot, not a final design
for a future monitoring system.

## Current Logging Model

Beads Workbench currently uses a **very small stderr-based diagnostics model**.

### What exists today

1. **Normal startup/runtime errors**
   - Printed directly to `stderr`
   - Examples:
     - config load failures
     - cwd resolution failures
     - config warnings
     - TUI startup failures

2. **Opt-in debug diagnostics**
   - Enabled only with `--debug`
   - Written to `stderr`
   - Every debug line is prefixed with:

     ```text
     [bwb-debug]
     ```

3. **Gateway command execution traces**
   - Also routed through the `--debug` stderr path
   - Current gateway debug output includes one line per `bd` invocation with:
     - argv
     - exit code

### Current debug event categories

When `--debug` is enabled, the documented event categories are:

- startup resolution
  - resolved config path
  - resolved cwd
  - auto-refresh enabled/disabled
- `bd` command execution traces
  - `bd argv=... exit_code=...`

## What does **not** exist today

Beads Workbench currently has **no built-in centralized logging sink**.

Specifically, it does **not** currently provide:

- a dedicated log file under `~/.local/state`, `~/.cache`, or `~/.config`
- log rotation
- structured JSON logs
- log levels beyond normal stderr output vs `--debug`
- a machine-local logging service/integration layer
- metrics collection
- tracing/span infrastructure
- health endpoints
- alerting hooks
- a persistent audit/event log beyond normal tracker and git history

## Where logs go right now

Today, BWB sends diagnostics only to the process standard streams:

- `stdout` for successful non-interactive output like `--help`, `--version`,
  `--print-config`, `--check-config`
- `stderr` for warnings, failures, and `--debug` diagnostics

That means the effective "machine-visible logging destination" is whatever
captures `stderr` for the current execution context:

- an interactive terminal
- shell redirection
- CI logs
- tmux/screen scrollback
- systemd/journald **if** BWB is launched that way externally

BWB itself does not currently own that capture destination.

## Relevant Current Code Paths

### CLI / startup diagnostics

- `cmd/bwb/main.go`
  - handles CLI parsing
  - prints startup/config errors to `stderr`
  - emits `[bwb-debug]` startup diagnostics when `--debug` is enabled

### Gateway execution diagnostics

- `internal/gateway/beads/runner.go`
  - supports an injected `DebugLog func(string)` in `RunnerConfig`
  - emits per-command debug lines for `bd` executions

## Relevant Current Documentation

Current logging/debug behavior is already described in:

- `README.md`
- `AGENTS.md`
- `docs/OVERVIEW.md`
- `docs/CODING.md`

Those docs currently describe the debug flag as a lightweight stderr diagnostic
path, not as a full logging subsystem.

## Architectural Implication

The current implementation matches the broader Beads Workbench product and
architecture direction:

- small, focused TUI
- no heavy control-plane/runtime subsystem
- minimal CLI bootstrap before Bubble Tea startup
- official `bd` gateway boundary
- lightweight process diagnostics rather than a large internal observability
  system

In other words: **we currently have debug diagnostics, not a monitoring
solution**.

## Current Gaps

If we want a real logging/monitoring solution later, the current gaps are:

- no durable local log storage
- no central machine-readable log format
- no session correlation identifiers
- no explicit separation between operator-facing stderr messages and
  machine-oriented diagnostics
- no defined retention policy
- no support for background/daemon-style observability
- no monitoring story for launcher subprocesses or long-running external tools

## Questions for the next design discussion

When we design a real logging solution, we should decide:

1. Should BWB keep stderr as the primary operator surface and add an optional
   log file, or should it own a default persistent local log location?
2. Should logs live under XDG state (for example `~/.local/state/bwb/`)?
3. Do we want plain text logs only, or optional structured JSON?
4. Do we want a `--log-file` and/or `--log-format` flag?
5. Should gateway command traces always stay debug-only, or be promoted into a
   persistent debug log?
6. How much monitoring is actually appropriate for a mostly interactive TUI
   process versus future background/helper workflows?

## Bottom line

Current BWB logging is:

- **stderr-based**
- **minimal**
- **debug-flag gated** for diagnostic detail
- **not centralized**
- **not persistent by default**
- **not yet a full monitoring solution**
