# Change Workflow

## Tracker-first rule

- Use `bd` for repository task tracking.
- Run `bd prime` when you need the full tracker workflow/refresher.
- Use `bd ready`, `bd list --status open`, and `bd show <id>` to pick up or inspect work.
- Use `bd remember` for persistent repo knowledge; do not create markdown memory files.

## Local change loop

1. Confirm the issue or follow-up work is tracked in `bd`.
2. Make the change.
3. Run the right verification depth for the change:
   - Docs-only changes: verify touched paths, commands, routes, and links directly.
   - Code changes: run the quality gates from `docs/CODING.md`.
   - Runtime UI changes: also run `docs/RUNTIME_UI_VERIFICATION.md`.
4. Update tracker state before handoff:
   - close finished work with `bd close <id>`
   - create follow-up issues for remaining work
   - keep issue descriptions/statuses aligned with reality

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
