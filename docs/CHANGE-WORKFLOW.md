# Change Workflow

## Tracker-first rule

This project uses **bd (beads)** for issue tracking. Use `bd` for ALL task
tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists.

- Run `bd prime` for the full tracker workflow, command reference, and session-close protocol.
- Use `bd remember` for persistent repo knowledge; do NOT create markdown memory files.

Quick reference:

```bash
bd ready               # find available work
bd show <id>           # view issue details
bd update <id> --claim # claim work
bd list --status open  # inspect open work
bd close <id>          # complete work
```

## Optional local pre-commit hook (staged Go formatting)

This repo includes a lightweight pre-commit hook at `scripts/git-hooks/pre-commit`.
It only formats staged `*.go` files with `gofmt -w` and re-stages those files.
It intentionally does not run broader checks (tests, vet, or build).

Install once per clone (no Makefile required):

```bash
git config core.hooksPath scripts/git-hooks
```

Alternatively, `mise run hooks:install` runs the same command.

Verify your local hook path:

```bash
git config --get core.hooksPath
```

## Local change loop

1. Confirm the issue or follow-up work is tracked in `bd`.
2. Make the change.
3. Run the right verification depth for the change:
   - Docs-only changes: verify touched paths, commands, routes, and links directly.
   - Code changes: run the code-change verification sequence below.
   - Diagnostics/logging changes: also update and cross-check `docs/MONITORING.md`.
   - Runtime UI changes: also run `docs/RUNTIME_UI_VERIFICATION.md`.
4. Update tracker state before handoff:
   - close finished work with `bd close <id>`
   - create follow-up issues for remaining work
   - keep issue descriptions/statuses aligned with reality

### Code-change verification sequence

Use this sequence before handoff for code changes:

```bash
mise run quality
```

This runs `go vet`, golangci-lint, architecture guardrails, unit tests, and
integration tests. For individual tasks, see `docs/CODING.md`.

Use `mise run quality:fast` for a lighter in-flight check (skips integration
tests only). Run `mise tasks` for the full task list.

CI runs a **superset** of `mise run quality` — it additionally runs
`fmt:check`, `scripts:check`, `build`, and a `test:coverage` gate across an
ubuntu/macos/windows matrix — so local-green does not guarantee CI-green. Run
`mise run fmt:check` and `mise run scripts:check` before handoff.

## Landing the plane

Before ending a work session:

```bash
git status
git add <files>
git commit -m "..."
git pull --rebase
bd dolt push
git push
git status
```

`bd dolt push` syncs the beads issue database (a Dolt-backed store under
`.beads/`) to its remote; it is separate from the source `git push`. Both must
succeed for the session to be complete.

If `git pull --rebase` changes the commit or requires conflict resolution,
rerun the relevant verification before pushing.

Completion bar:

- follow-up work is tracked in `bd`
- verification is complete for the touched surface
- finished issues are closed or updated
- `bd dolt push` succeeds
- `git push` succeeds
- final `git status` shows the branch is up to date with `origin`

Work is not done until the tracker state and git state are both pushed.

## Review and release handoff

- For normal code review / branch landing expectations, keep local quality gates green and ensure the `CI` workflow in `.github/workflows/ci.yml` passes.
- For version tags and release assets, follow `docs/RELEASING.md`.
