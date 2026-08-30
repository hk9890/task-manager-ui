# Monitoring

## Current diagnostics surface

The log is the whole diagnostics surface — there is no metrics, health-endpoint or
trace-export path to reach for. Runtime diagnostics are centralized through
`internal/logging` and used by `cmd/taskmgr-ui/main.go` at startup.

- `stdout` remains the success surface for non-interactive `--help`, `--version`,
  `--print-config`, and `--check-config`
- `stderr` remains the operator-facing surface for startup failures, config
  warnings, and other warnings/errors
- all startup paths, including non-interactive `--print-config` and
  `--check-config`, also write diagnostics to the persistent JSON Lines log when
  the sink is available

## Centralized logging contract

`internal/logging` is the single logging entrypoint for runtime diagnostics.

**The persistent JSON sink writes INFO and above.** `--debug` lowers it to DEBUG
(`newJSONFileHandler`). A DEBUG record does not exist in a default run's log file —
if you are looking for one, relaunch with `--debug`.

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
- `component`, on records emitted through a component logger: `startup`
  (`cmd/taskmgr-ui/main.go`), and `board`, `docs`, `search` (`internal/app/model.go`).
  Shell-level records — the issue-edit failures and the health check below — go through
  the root logger and carry no `component` key.

To attribute a session safely in a collected set of log files, use `session_id`
together with `project_root` and `build_version`. Startup records inherit those
root attributes automatically.

### Store-resolution record

Opening the `taskmgr` backend emits one `startup` record, `resolved
task-manager store`, carrying:

- `kind` — `local`, `central`, or `override_name`
- `store_path` — the resolved store directory
- `project_path` — the project the store tracks

This is the record that distinguishes a project running on its own `.tasks`
directory from one whose store was promoted with `taskmgr store move --central`,
and it is the first thing to read when the UI opens a board the `taskmgr` CLI
does not agree with. Note that `project_root` (the root attribute on every
record) is the working directory, while `project_path` is the store's project —
they differ for a central store, and for a run started in a subdirectory
([CONFIGURATION.md](CONFIGURATION.md) covers what a launcher template does with it).

A failure to resolve is reported by the existing `interactive startup failed`
record instead; it names the working directory, or the store name when
`--store-name` was given.

A `WARN` follows the record when `project_path` is not accessible. Resolution
checks the store directory, never the project path recorded for it, so a
registry entry outliving a moved or deleted project opens normally — the board
reads fine, while launchers without an explicit `work_dir` exec in a directory
that is gone. Re-point the entry by running
`taskmgr store move --relink --to <store>` from the project's new location.

### What a healthy run logs after startup

Two things, both expected:

- `temp cleanup: removed stale temp file` (INFO, `internal/app/services.go`) — the
  post-startup sweep armed by `Model.Init` found an old edit temp file and removed it.
- the Done-column load-more trace (DEBUG, `internal/mode/board/model.go`) — only when the
  column actually pages, and only under `--debug`.

Anything else after startup means something went wrong.

### Runtime failure records

These are what a session that went wrong leaves behind, and the first thing to read
when a run misbehaved. All reach the persistent log only, because stderr is
suppressed for the interactive session.

- `task-manager health check failed` — emitted for every failure code. Only
  `no_database_found` also draws the fatal-error screen; an unreadable or corrupt
  store shows nothing on screen and the board simply renders whatever the first
  `Dashboard` call returned.
- `failed to prepare the issue edit document`, `failed to apply the issue edit` —
  an `e` round trip that did not save. The toast carries the same cause.
- `dashboard refresh failed; keeping the last loaded columns` (WARN) — the rows on the
  board are the previous load's.
- `cardinality threshold exceeded` (WARN) — an active group (Ready, Blocked or In Progress)
  holds more than 500 issues (`dashboard.cardinalityThreshold`). Nothing is truncated and
  nothing is wrong; it is a signal that the column is unusually large.
- `load-more for Done column failed` (WARN) — a Done page did not arrive. The error is set
  on the Done column, so the operator sees it on screen too
  (`applyLoadMoreClosed`, `internal/mode/board/model.go`).
- `temp cleanup: glob failed`, `temp cleanup: remove failed` (WARN,
  `internal/app/services.go`) — the sweep could not read or delete a stale edit temp
  file. Nothing on screen changes; the file stays behind.
- `backend sort assumption broken` (WARN) — a dashboard group came back in an order
  `internal/dashboard` does not expect (`Warning.Threshold == -1`). The column still
  renders; its order is the backend's, not the composed one.
- `stale load-more page dropped; a reload superseded it` (DEBUG) — a Done-column page
  discarded because a reload landed first; expected, not a fault. Visible only under
  `--debug`.

### Guard traces

Every surface suppresses work that arrives while its own is still in flight and records
why, at DEBUG, so a run that looks idle can be told from one that is guarding. None is a
fault, and none reaches the log without `--debug`:

| Record | Emitted by |
|---|---|
| `manual <surface> refresh suppressed; refresh already in flight` | board, docs, search, and detail (`internal/app/refresh.go`) |
| `startReload re-entry suppressed`, `triggerSearchWithAnchor re-entry suppressed` | the same reload paths, one level in |
| `load-more suppressed; already in flight`, `load-more suppressed; all closed issues loaded` | `internal/mode/board/model.go` |
| `search scope toggle suppressed; search already in flight`, `search scope toggled` | `internal/mode/search/model.go` |

Repeats of one of these under a key held down are the guard working. The Done-column
runbook in [RUNNING.md](RUNNING.md) reads them that way.

## `--debug` coverage

`--debug` does two things: it lowers the persistent sink to DEBUG, and it mirrors
machine-visible startup diagnostics to `stderr` with the compatibility prefix
`[taskmgr-ui-debug]`:

- startup resolution lines from `cmd/taskmgr-ui/main.go` for both interactive and
  non-interactive startup paths that load config
  - resolved config path
  - resolved cwd
  - auto-refresh enabled/disabled
  - repo backend (`repo`, `repo_file`, and the `--store-name` override
    `store_name`)
- the run `session_id`, printed once so operators can correlate stderr output with
  structured log records — for interactive startup and for startup-only commands such
  as `--check-config` and `--print-config`

The store-resolution record described above is not part of the `stderr` mirror:
it is emitted after stderr suppression is raised for the interactive session, so
it reaches the persistent log only.

The repository backend is in-process, so there is no per-command argv/exit-code/duration
execution trace, and the backend emits no diagnostic records of its own.

## Capture commands

For a non-interactive diagnostics capture:

```bash
taskmgr-ui --cwd /path/to/project --debug --check-config 2> /tmp/taskmgr-ui-debug-check.log
```

Launching the product itself is [RUNNING.md](RUNNING.md)'s subject — a stderr redirect
around an interactive run captures almost nothing, because stderr is suppressed once the
TUI starts.

Use the persistent JSON Lines log when you need durable machine-readable
diagnostics. Each process writes to its own file named after its `session_id`
(printed on `stderr` under `--debug`):

```bash
tail -f ~/.local/state/taskmgr-ui/taskmgr-ui-*.log            # all sessions
tail -f ~/.local/state/taskmgr-ui/taskmgr-ui-<session_id>.log # one session
```

Substitute `"$XDG_STATE_HOME"/taskmgr-ui/` when that variable is set.

When inspecting multiple log files, do not assume adjacent records across files
came from the same repository or binary. Filter by `session_id`, `project_root`,
and `build_version`.

## Relevant code paths

- `cmd/taskmgr-ui/main.go` — CLI parsing, startup logger initialization, startup warnings/errors, non-interactive startup command handling, and repository construction (`buildRepository`: `tasks.Resolve` → `taskmgr.New`)
- `internal/logging/logging.go` — central logger construction, persistent JSON Lines sink, session IDs, stderr mirroring, and fallback warning
- `internal/app/services.go` — the temp-cleanup sweep records
- `internal/mode/board/model.go` — the dashboard-refresh, sort, cardinality and load-more records
- `internal/mode/search/model.go`, `internal/mode/docs/model.go`, `internal/app/refresh.go` — the
  guard traces above

## Runtime UI evidence

For rendered TUI state rather than diagnostics, drive the built binary with
`scripts/capture_taskmgr_ui_screen.py` ([RUNNING.md](RUNNING.md)).
