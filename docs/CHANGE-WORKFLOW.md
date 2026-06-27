# Change Workflow

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

1. Confirm the issue or follow-up work is tracked in `taskmgr`.
2. Make the change.
3. Run the right verification depth for the change:
   - Docs-only changes: verify touched paths, commands, routes, and links directly.
   - Code changes: run the code-change verification sequence below.
   - Diagnostics/logging changes: also update and cross-check `docs/MONITORING.md`.
   - Runtime UI changes: also run `docs/RUNTIME_UI_VERIFICATION.md`.
4. Update tracker state before handoff:
   - close finished work with `taskmgr close <id>`
   - create follow-up issues for remaining work with `taskmgr create`
   - keep issue descriptions/statuses aligned with reality (`taskmgr update <id>`)

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
ubuntu/macos matrix — so local-green does not guarantee CI-green. Run
`mise run fmt:check` and `mise run scripts:check` before handoff.

## Landing the plane

Before ending a work session:

```bash
git status
git add <files>
git commit -m "..."
git pull --rebase
git push
git status
```

The task-manager store lives in `.tasks/`, which is gitignored and local-only
(kept out of git, never published), so tracker state does NOT ship with the
source commit — do not stage or commit `.tasks/`.

If `git pull --rebase` changes the commit or requires conflict resolution,
rerun the relevant verification before pushing.

Completion bar:

- follow-up work is tracked in `taskmgr`
- verification is complete for the touched surface
- finished issues are closed or updated
- `git push` succeeds
- final `git status` shows the branch is up to date with `origin`

Work is not done until the code/git state is pushed.

## Review and release handoff

- For normal code review / branch landing expectations, keep local quality gates green and ensure the `CI` workflow in `.github/workflows/ci.yml` passes.
- For version tags and release assets, follow `docs/RELEASING.md`.
