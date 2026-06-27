# Monitoring

## Current diagnostics surface

Runtime diagnostics are now centralized through `internal/logging` and used by
`cmd/taskmgr-ui/main.go` plus the repository backend (the validating decorator in
`internal/repository/validating.go`).

- `stdout` remains the success surface for non-interactive `--help`, `--version`,
  `--print-config`, and `--check-config`
- `stderr` remains the operator-facing surface for startup failures, config
  warnings, and other warnings/errors
- all startup paths, including non-interactive `--print-config` and
  `--check-config`, also write diagnostics to the persistent JSON Lines log when
  the sink is available
- `--debug` enables DEBUG/INFO diagnostic mirroring to `stderr` with the
  compatibility prefix `[taskmgr-ui-debug]`

## Centralized logging contract

`internal/logging` is the single logging entrypoint for runtime diagnostics.

Implemented behavior:

- persistent JSON Lines log sink at `$XDG_STATE_HOME/taskmgr-ui/taskmgr-ui-<session_id>.log`
  - fallback path: `~/.local/state/taskmgr-ui/taskmgr-ui-<session_id>.log`
  - each taskmgr-ui process writes to its own file named after its `session_id`, so
    concurrent processes never share a file or its rotation state
  - this sink is user/machine scoped and the directory can contain log files
    from multiple sessions, projects, and multiple taskmgr-ui builds
- per-run `session_id` attached to structured records
- root provenance fields on every record:
  - `project_root`
  - `build_version`
- fixed lumberjack rotation defaults
  - max size: 10 MB
  - max backups: 5
  - max age: 30 days
  - compression enabled
- on startup, stale `taskmgr-ui-*.log` files older than the rotation max age (30 days)
  are pruned from the state directory; the current session's file is never
  deleted regardless of age; errors from individual prune operations are
  silently ignored so that cleanup never aborts startup
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
- component-specific fields such as `component` (for example `startup` or
  `validating`)

To attribute a session safely in a collected set of log files, use `session_id`
together with `project_root` and `build_version`. Startup and repository records
both inherit those root attributes automatically.

## `--debug` coverage

`--debug` mirrors machine-visible startup diagnostics to `stderr`:

- startup resolution lines from `cmd/taskmgr-ui/main.go` for both interactive and
  non-interactive startup paths that load config
  - resolved config path
  - resolved cwd
  - auto-refresh enabled/disabled
  - repo backend (`repo` and `repo_file`)

The repository backend is in-process (the task-manager Go SDK,
`github.com/hk9890/task-manager/sdk/tasks`); there is no external subprocess in
the product data path and therefore no per-command argv/exit-code/duration
execution trace. Repository diagnostics are limited to contract-violation
warnings emitted by the validating decorator (see below).

### Repository contract violations

The validating decorator (`internal/repository/validating.go`) wraps the
in-process backend and logs structural-invariant violations at `WARN` under the
`validating` component:

- WARN `"repository contract violation"` — carries the offending `method`, the
  violated `rule`, and a `sample` of the bad value. The call is never failed;
  the inner result is returned unchanged. Because `WARN` records mirror to
  `stderr`, a contract violation is operator-visible even without `--debug`.

Only the production task-manager backend is wrapped by the validating decorator;
the `--repo=memory` (filestorage) backend is returned unwrapped and therefore
emits no contract-violation diagnostics.

In a healthy session no repository records are emitted at all — the in-process
SDK is fast and the validating decorator is silent unless it detects a
malformed return value.

The startup debug stream also prints the run `session_id` once so operators can
correlate stderr output with structured log records. This applies equally to
interactive startup and startup-only commands such as `--check-config` and
`--print-config`.

## Capture commands

Use stderr capture when you need reproducible operator-facing evidence:

```bash
taskmgr-ui --cwd /path/to/project --debug 2> /tmp/taskmgr-ui-debug.log
taskmgr-ui --cwd /path/to/project --debug --check-config 2> /tmp/taskmgr-ui-debug-check.log
```

Use the persistent JSON Lines log when you need durable machine-readable
diagnostics. Each process writes to its own file named after its `session_id`.
To follow all active sessions at once, use a glob:

```bash
tail -f ~/.local/state/taskmgr-ui/taskmgr-ui-*.log
```

Or, if `XDG_STATE_HOME` is set:

```bash
tail -f "$XDG_STATE_HOME"/taskmgr-ui/taskmgr-ui-*.log
```

To follow a specific session by ID (the `session_id` is printed on `stderr`
when `--debug` is set):

```bash
tail -f "$XDG_STATE_HOME/taskmgr-ui/taskmgr-ui-<session_id>.log"
# e.g.:
tail -f ~/.local/state/taskmgr-ui/taskmgr-ui-deadbeef.log
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

- `cmd/taskmgr-ui/main.go` — CLI parsing, startup logger initialization, startup warnings/errors, non-interactive startup command handling, and repository construction (`constructRepository`: `tasks.Open` → `taskmgr.New` → `repository.NewValidating`)
- `internal/repository/taskmgr/` — in-process task-manager backend (the production repository); behavior tests live alongside it
- `internal/repository/validating.go` — validating decorator; emits `"repository contract violation"` WARN records under the `validating` component
- `internal/logging/logging.go` — central logger construction, persistent JSON Lines sink, session IDs, stderr mirroring, and fallback warning
- `internal/logging/logging_test.go` — record-shape, session-id, rotation, and fallback coverage

## Runtime UI evidence

For user-visible runtime capture rather than stderr diagnostics, use the
verification tooling documented in `docs/RUNTIME_UI_VERIFICATION.md`:

- `scripts/capture_taskmgr_ui_screen.py`

That script captures rendered TUI state; it is not part of the logging
surface.

## Current limitations

The active runtime path still does not provide:

- health endpoints
- metrics collection
- tracing/span export

Update this file when `internal/logging/`, `cmd/taskmgr-ui/main.go`, or
`internal/repository/validating.go` changes the diagnostics contract.
