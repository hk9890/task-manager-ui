# Change Workflow

## Tracker-first rule

- Use `bd` for repository task tracking.
- Run `bd prime` when you need the full tracker workflow/refresher.
- Use `bd ready`, `bd list --status open`, and `bd show <id>` to pick up or inspect work.
- Use `bd remember` for persistent repo knowledge; do not create markdown memory files.

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

This runs the full gate sequence: script syntax checks, golangci-lint, architecture guardrails, build, vet, and unit tests. For individual tasks, see `docs/CODING.md`.

Use `mise run quality:fast` for a lighter in-flight check (skips lint and integration tests). Run `mise tasks` for the full task list.

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
