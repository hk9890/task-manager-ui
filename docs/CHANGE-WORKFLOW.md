# Change Workflow

How changes land in `taskmgr-ui`.

## Standard flow

1. Confirm the issue or follow-up work is tracked in `taskmgr` (AGENTS.md → Issue Tracker).
2. Start the change in a worktree — see [Worktrees](#worktrees). Every change gets its own worktree
   and branch off `main`; `main` is not a working branch.
3. Make the change.
4. Run the right [verification depth](#verification-depth) for what you touched.
5. Update tracker state and [land the branch](#landing-the-plane).

The GitHub remote is `origin`. Use the `commit-commands:commit` skill for a standard commit and
`commit-commands:commit-push-pr` to commit + push + open a PR — don't hand-roll the commit flow.
Merge with `gh pr merge --merge` once the `CI` check is green. **Agents open the PR and stop there**
unless asked to merge.

## Worktrees

- **Start work with the `EnterWorktree` tool — not a hand-rolled `git worktree add`.** It creates
  the worktree under `.claude/worktrees/` and enters it in one step; leave with `ExitWorktree`.
  `.claude/hooks/worktree-guard.sh`, wired up by `.claude/settings.json`, denies persistent
  hand-rolled worktrees.
- **`.tasks/` reaches a worktree only through `worktree.symlinkDirectories`.** The tracker store is
  gitignored and lives in the main checkout, so a worktree created without that symlink fails every
  `taskmgr` call with `no .tasks directory found`. `.claude/settings.local.json` carries `.tasks`
  and `hako-private` across. It is local-only and absent from a fresh clone — recreate it once per
  clone:

  ```json
  {
    "worktree": {
      "baseRef": "head",
      "symlinkDirectories": [".tasks", "hako-private"]
    }
  }
  ```

- `baseRef: head` branches from your local HEAD, not `origin/main` — fetch and fast-forward `main`
  first if you want the pushed tip.
- **No per-worktree install step.** The Go module cache and the `mise`-provisioned toolchain are
  machine-global, so `mise run <task>` works in a fresh worktree immediately.
- **Exception: a throwaway clean-build probe** — `git worktree add --detach <tmpdir> <ref>` into
  `/tmp` or the scratchpad is fine; the guard exempts those paths and denies everything else.
- `.gitignore` lists `.tasks` and `hako-private` without a trailing slash on purpose: inside a
  worktree they are symlinks, and a directory-only pattern would leave them showing as untracked.

## Local pre-commit hook (staged Go formatting)

This repo includes a lightweight pre-commit hook at `scripts/git-hooks/pre-commit`.
It only formats staged `*.go` files with `gofmt -w` (and `goimports` when available)
and re-stages those files. It intentionally does not run broader checks (tests, vet, or build).

Install once per clone (no Makefile required):

```bash
git config core.hooksPath scripts/git-hooks
```

Alternatively, `mise run hooks:install` runs the same command.

Verify your local hook path:

```bash
git config --get core.hooksPath
```

Keep it relative. `core.hooksPath` lives in the shared `.git/config`, and a relative path resolves
against whichever worktree the commit runs in; an absolute path pins every worktree to one
directory and silently stops working if the clone is moved or renamed.

## Verification depth

Run the depth that matches the change:

- Docs-only changes: verify touched paths, commands, routes, and links directly.
- Code changes: run the code-change verification sequence below.
- Diagnostics/logging changes: also update and cross-check `docs/MONITORING.md`.
- Runtime UI changes: also run `docs/RUNTIME_UI_VERIFICATION.md`.

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

Before ending a work session, from inside the worktree:

1. Update tracker state:
   - close finished work with `taskmgr close <id>`
   - create follow-up issues for remaining work with `taskmgr create`
   - keep issue descriptions/statuses aligned with reality (`taskmgr update <id>`)
2. Commit, push the branch, and open the PR — `commit-commands:commit-push-pr` does all three.
3. Confirm `CI` is green on the PR (`gh pr checks`).

The task-manager store lives in `.tasks/`, which is gitignored and local-only
(kept out of git, never published), so tracker state does NOT ship with the
source commit — do not stage or commit `.tasks/`. The same holds for the `.tasks`
symlink inside a worktree.

If you rebase onto a moved `main` and that changes the commit or requires conflict
resolution, rerun the relevant verification before pushing again.

Completion bar:

- follow-up work is tracked in `taskmgr`
- verification is complete for the touched surface
- finished issues are closed or updated
- the branch is pushed and a PR is open
- `git status` in the worktree is clean and the branch is up to date with `origin`

Work is not done until the branch is pushed and the PR is open. Leave the worktree with
`ExitWorktree` — `keep` if the branch has not merged yet, `remove` once it has.

## Review and release handoff

- For normal code review / branch landing expectations, keep local quality gates green and ensure the `CI` workflow in `.github/workflows/ci.yml` passes.
- For version tags and release assets, follow `docs/RELEASING.md`.
