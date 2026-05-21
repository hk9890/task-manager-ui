# Monitoring

## Current diagnostics surface

Runtime diagnostics are now centralized through `internal/logging` and used by
`cmd/bwb/main.go` plus `internal/gateway/beads/runner.go`.

- `stdout` remains the success surface for non-interactive `--help`, `--version`,
  `--print-config`, and `--check-config`
- `stderr` remains the operator-facing surface for startup failures, config
  warnings, and other warnings/errors
- all startup paths, including non-interactive `--print-config` and
  `--check-config`, also write diagnostics to the persistent JSON Lines log when
  the sink is available
- `--debug` enables DEBUG/INFO diagnostic mirroring to `stderr` with the
  compatibility prefix `[bwb-debug]`

## Centralized logging contract

`internal/logging` is the single logging entrypoint for runtime diagnostics.

Implemented behavior:

- persistent JSON Lines log sink at `$XDG_STATE_HOME/bwb/bwb-<session_id>.log`
  - fallback path: `~/.local/state/bwb/bwb-<session_id>.log`
  - each BWB process writes to its own file named after its `session_id`, so
    concurrent processes never share a file or its rotation state
  - this sink is user/machine scoped and the directory can contain log files
    from multiple sessions, beads projects, and multiple BWB builds
- per-run `session_id` attached to structured records
- root provenance fields on every record:
  - `project_root`
  - `build_version`
- fixed lumberjack rotation defaults
  - max size: 10 MB
  - max backups: 5
  - max age: 30 days
  - compression enabled
- stderr mirroring for warnings/errors and debug-prefix compatibility
- stderr-only fallback with a single warning if the persistent sink is
  unavailable

Structured records include at least:

- `timestamp`
- `level`
- `message`
- `session_id`
- `project_root`
- `build_version`
- component-specific fields such as `component`, `argv`, `operation`,
  `exit_code`, and `duration_ms`

To attribute a session safely in a collected set of log files, use `session_id`
together with `project_root` and `build_version`. Startup and gateway records
both inherit those root attributes automatically.

## `--debug` coverage

`--debug` mirrors two categories of machine-visible diagnostics to `stderr`:

- startup resolution lines from `cmd/bwb/main.go` for both interactive and
  non-interactive startup paths that load config
  - resolved config path
  - resolved cwd
  - auto-refresh enabled/disabled
- `bd` CLI execution traces from `internal/gateway/beads/runner.go`
  - operation name
  - full argv
  - exit code
  - duration in milliseconds

The startup debug stream also prints the run `session_id` once so operators can
correlate stderr output with structured log records. This applies equally to
interactive startup and startup-only commands such as `--check-config` and
`--print-config`.

## Capture commands

Use stderr capture when you need reproducible operator-facing evidence:

```bash
bwb --cwd /path/to/beads-project --debug 2> /tmp/bwb-debug.log
bwb --cwd /path/to/beads-project --debug --check-config 2> /tmp/bwb-debug-check.log
```

Use the persistent JSON Lines log when you need durable machine-readable
diagnostics. Each process writes to its own file named after its `session_id`.
To follow all active sessions at once, use a glob:

```bash
tail -f ~/.local/state/bwb/bwb-*.log
```

Or, if `XDG_STATE_HOME` is set:

```bash
tail -f "$XDG_STATE_HOME/bwb/bwb-*.log"
```

To follow a specific session by ID (the `session_id` is printed on `stderr`
when `--debug` is set):

```bash
tail -f "$XDG_STATE_HOME/bwb/bwb-<session_id>.log"
# e.g.:
tail -f ~/.local/state/bwb/bwb-deadbeef.log
```

When inspecting multiple log files, do not assume adjacent records across files
came from the same repository or binary. Filter or inspect by `session_id`,
`project_root`, and `build_version`.

Effective capture destinations therefore include:

- interactive terminal scrollback
- shell redirection
- CI job logs
- tmux/screen scrollback
- any external supervisor that captures stderr

## Relevant code paths

- `cmd/bwb/main.go` — CLI parsing, startup logger initialization, startup warnings/errors, and non-interactive startup command handling
- `internal/gateway/beads/runner.go` — structured per-command `bd` execution traces
- `internal/gateway/beads/runner_test.go` — execution trace coverage for argv/exit code/duration logging
- `internal/logging/logging.go` — central logger construction, persistent JSON Lines sink, session IDs, stderr mirroring, and fallback warning
- `internal/logging/logging_test.go` — record-shape, session-id, rotation, and fallback coverage

## Runtime UI evidence

For user-visible runtime capture rather than stderr diagnostics, use the
verification tooling documented in `docs/RUNTIME_UI_VERIFICATION.md`:

- `scripts/capture_bwb_screen.py`
- `scripts/verify_bwb_state_flow.py`

Those scripts capture rendered TUI state; they are not part of the logging
surface.

## Current limitations

The active runtime path still does not provide:

- health endpoints
- metrics collection
- tracing/span export

Update this file when `internal/logging/`, `cmd/bwb/main.go`, or
`internal/gateway/beads/runner.go` changes the diagnostics contract.
