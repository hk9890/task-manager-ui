# Coding

## Project Identity

- **Module:** `github.com/hk9890/beads-workbench`
- **Binary:** `bwb` (`cmd/bwb`)
- **Language:** Go
- **TUI framework:** Bubble Tea

## Build and Test

```bash
go build ./cmd/bwb    # build the binary
go test ./...         # run all tests
go vet ./...          # vet
```

For testing strategy, vocabulary, and harness conventions (teatest, golden files,
fake seams, embedded fixture usage), see `docs/TESTING.md`.

Recommended local verification before handoff:

```bash
go test ./cmd/bwb -run TestArchitectureGuardrails
go build ./cmd/bwb
go vet ./...
go test ./...
```

## Package Layout

Current bootstrapped layout:

```
cmd/bwb/             # binary entrypoint
internal/
  app/               # Bubble Tea root model, mode switching, shared layout
  config/            # runtime configuration model + defaults
  domain/            # Beads Workbench issue and dashboard models
  gateway/beads/     # BeadsGateway interface + CLI-backed implementation
  launcher/          # external editor and command launch actions
  dashboard/         # dashboard definition providers and built-in definitions
  mode/              # board/search/details controllers
  ui/                # reusable rendering components (loading, modal, toaster, styles)
project-plan/        # product, architecture, and execution planning docs
```

## Core Architectural Rules

1. **No direct SQL.** All issue reads and writes go through the `BeadsGateway` interface. No `database/sql`, no Dolt server client, no BQL executor in the primary product path.

2. **Official beads surfaces only.** The gateway implementation talks to `bd` CLI commands. Do not read beads internals directly.

3. **Gateway is source-specific.** A gateway instance is bound to one beads project. Federation is a future layer above gateways, not a change to the core interface.

4. **Dashboard renderer and dashboard provider are separate.** v1 uses built-in definitions. A file-backed provider can be added later without touching the renderer.

5. **Editor handoff is a first-class flow.** Rich issue editing opens `$EDITOR` rather than building complex inline forms.

   **Issue edit document contract (v1):**
   - Editable fields map directly to gateway update capabilities: `title`, `description`, `status`, `type`, `priority`, `assignee`, and `labels`.
   - Read-only context (issue id, timestamps, notes, dependencies, related items, comments) is rendered for operator context and ignored by parser/diff logic.
   - Round-trip behavior is marker-based (`BWB:EDITABLE` / `BWB:FIELD:*`) so parser changes are deterministic and testable.
   - The external editor launch is behind a replaceable seam (`internal/launcher/editor.Opener`) so tests never spawn a real interactive editor.

6. **Launchers are thin.** Launchers receive issue context and produce a subprocess. They must not become an orchestration engine.

   **Launcher behavior contract (v1):**
   - `internal/launcher.Service` resolves an action name to one configured command template.
   - Interpolation is simple placeholder replacement (no scripting/conditionals).
   - Launchers start a subprocess and return immediately (no process supervision/retry).
   - Launch success/failure is surfaced in shell toast feedback.

7. **Create vs edit ownership boundary is explicit.** The rich marker-based document flow currently owns **issue editing** (`e` in detail context). Issue creation remains on the existing create/update task boundary and is not coupled to this editor document contract.

## Runtime Configuration (v1)

Configuration lives in `internal/config` and is loaded once at startup via
`config.Default()`.

The v1 model is intentionally small and only covers app-shell concerns:

- `Editor.Command`
  - Uses `$EDITOR` when set.
  - Falls back to `vi` when `$EDITOR` is unset/empty.
- `Launcher.Definitions`
  - Defaults to four built-in launcher actions:
    - `editor` → launches the resolved editor command (`$EDITOR`, fallback `vi`).
    - `nvim` → launches `nvim` with a read-only issue context buffer seeded from
      interpolation placeholders.
    - `opencode` → launches `opencode run` with issue metadata args/env.
    - `shell-command` → launches `sh -lc` with a simple formatted issue-context
      print command.
  - Each definition supports:
    - `Action` (required unique action key)
    - `Command` (required executable/template string)
    - `Args` (optional argv templates)
    - `Env` (optional `KEY=value` templates)
    - `WorkDir` (optional working-directory template; defaults to project root)
- `UI.ShowModeSwitcherHelp`
  - Defaults to `true`.
  - Controls whether the shell renders the mode hotkey hint line.

### Launcher interpolation/context surface

Launcher templates support these placeholders across `Command`, every `Args`
entry, every `Env` entry, and `WorkDir`:

- `{{issue.id}}`
- `{{issue.title}}`
- `{{issue.labels}}` (comma-joined label list)
- `{{issue.assignee}}`
- `{{project.root}}`

Notes:

- Unsupported placeholders are passed through literally.
- Empty issue fields interpolate as empty strings.
- `WorkDir` falls back to project root when blank.

### Shell editor/launcher UX behavior (v1)

- `e` opens the rich marker-based issue edit document flow for the currently
  selected issue via `services.Editor`.
- `n`, `p`, `l` trigger `nvim`, `opencode`, `shell-command` launchers from
  detail mode.
- If no issue is selected, the shell shows a warning toast and does not launch.
- Successful rich editor updates trigger detail reload; launchers remain
  non-blocking and do not auto-refresh issue detail.

The rich marker-based edit document flow in `internal/launcher/editor` is the
actual interactive shell edit path. Launcher definitions remain a separate
external-tool surface for non-edit actions.

### Testing references for editor/launcher behavior

- Config defaults and built-ins: `internal/config/model_test.go`
- Interpolation/runner behavior: `internal/launcher/service_test.go`
- Shell key wiring and launcher actions: `internal/app/model_test.go`
- Editor round-trip service seam: `internal/launcher/editor/service_test.go`
- Embedded fixture smoke coverage for edit flow: `internal/app/model_test.go`

For broader policy and full-app verification expectations, see `docs/TESTING.md`.

Shared shell feedback primitives live under `internal/ui/`:

- `ui/loading` renders loading/status feedback for board, search, and detail
  surfaces.
- `ui/toaster` renders transient error/warn/info/success feedback.
- `ui/modal` renders help/confirmation overlays.

Design intent:

- Keep config loading and access behind a clear boundary (`internal/config` +
  `app.Services.Config`).
- Avoid introducing legacy-style broad config surfaces (custom views, BQL,
  orchestration settings, etc.).

## Donor Migration Rules (Perles → Beads Workbench)

When adapting code from the donor repo (`/home/hans/dev/github/perles`), prefer **small, isolated UI primitives** and keep imports local to rendering concerns.

### Allowed donor paths (UI primitive scope)

- `internal/ui/shared/modal/`
- `internal/ui/shared/toaster/`
- `internal/ui/styles/`
- `internal/ui/shared/overlay/` (only as a rendering helper used by UI primitives)

### Forbidden donor paths (do not copy into standalone shell)

- `internal/bql/**`
- `internal/orchestration/**`
- `internal/control-plane/**`, `internal/control_plane/**`
- `internal/store/**`, `internal/sql/**`, or any direct `database/sql` usage
- Any package that requires Perles service containers, session orchestration, or donor runtime wiring

### Adaptation requirements

1. Keep APIs small and shell-focused (modal prompts, toast feedback, shared style/render helpers).
2. Remove donor-specific assumptions, including SQL/BQL/orchestration/service-container dependencies.
3. Prefer value-oriented, reusable helpers with explicit inputs/outputs over hidden global state.
4. Keep package boundaries under `internal/ui/*` aligned to standalone ownership.

## Enforced Architecture Guardrails

Automated guardrails are enforced in `cmd/bwb/architecture_guardrails_test.go` by checking the full dependency graph for `./cmd/bwb` (`go list -deps ./cmd/bwb`).

The checks fail if any dependency in the active product path violates these boundaries:

1. **No direct SQL in the active product path.**
   - Forbidden at minimum: `database/sql`, `database/sql/driver`

2. **No `internal/bql` dependency in the standalone app.**
   - Any import path containing `/internal/bql` is forbidden.

3. **No orchestration/control-plane subsystem in the active product path.**
   - Any import path segment matching `orchestration`, `control-plane`, or `control_plane` is forbidden.

These checks are intentionally lightweight and local-friendly: they run as a normal Go test and require no external services.

## Quality Gates

The minimum quality gates for implementation and acceptance are:

```bash
go build ./cmd/bwb
go vet ./...
```

These are in addition to `go test ./...`, which includes the architecture guardrail test.

See `project-plan/ARCHITECTURE.md` for the full architecture definition, interface contracts, and `project-plan/IMPLEMENTATION.md` for phase sequencing and donor reuse strategy.
