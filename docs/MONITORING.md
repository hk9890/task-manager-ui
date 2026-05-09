# Monitoring

## Current diagnostics surface

The current runtime diagnostics contract still comes from `cmd/bwb/main.go` and
`internal/gateway/beads/runner.go`:

- `stdout` is used for successful non-interactive output from `--help`,
  `--version`, `--print-config`, and `--check-config`
- `stderr` is used for startup failures, config warnings, and other
  operator-facing errors from `cmd/bwb/main.go`
- `--debug` adds extra stderr diagnostics with the compatibility prefix
  `[bwb-debug]`

In normal `bwb` runs today, that means the effective diagnostics sink is still
stderr.

## In-repo centralized logging package

The repository also now includes `internal/logging`, a central slog-based
logging package that is being wired into the app in follow-up logging tasks.

Implemented package capabilities:

- persistent JSON Lines log sink at `$XDG_STATE_HOME/bwb/bwb.log`
  - fallback path: `~/.local/state/bwb/bwb.log`
- per-run `session_id` attached to structured records
- fixed lumberjack rotation defaults
  - max size: 10 MB
  - max backups: 5
  - max age: 30 days
  - compression enabled
- stderr mirroring for warnings/errors and debug-prefix compatibility
- stderr-only fallback with a single warning if the persistent sink is
  unavailable

Current status:

- `internal/logging` exists in the repo and has direct tests
- `cmd/bwb/main.go` and `internal/gateway/beads/runner.go` do not yet initialize
  or use it in the current working tree
- until that wiring lands, operators should treat stderr as the active runtime
  diagnostics surface

## `--debug` coverage

Today `--debug` emits two categories of machine-visible diagnostics:

- startup resolution lines from `cmd/bwb/main.go`
  - resolved config path
  - resolved cwd
  - auto-refresh enabled/disabled
- `bd` CLI execution traces from `internal/gateway/beads/runner.go`
  - `bd argv=...`
  - `exit_code=...`

## Capture commands

Use stderr capture when you need reproducible evidence:

```bash
bwb --cwd /path/to/beads-project --debug 2> /tmp/bwb-debug.log
```

Because stderr is still the active runtime sink today, the effective capture
destination depends on how `bwb` is launched:

- interactive terminal scrollback
- shell redirection
- CI job logs
- tmux/screen scrollback
- any external supervisor that captures stderr

## Relevant code paths

- `cmd/bwb/main.go` — CLI parsing, startup warnings/errors, `--debug` startup lines
- `internal/gateway/beads/runner.go` — per-command `bd` debug traces
- `internal/gateway/beads/runner_test.go` — debug trace coverage for argv/exit code logging
- `internal/logging/logging.go` — central logger construction, persistent JSON Lines sink, session IDs, stderr mirroring, fallback warning
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
