# Change Workflow

## Worktrees

Every change starts in its own worktree; `main` is not a working branch.

- Create it with the `EnterWorktree` tool — `.claude/hooks/worktree-guard.sh` denies a persistent
  `git worktree add`. A throwaway `--detach` probe into `/tmp` or the scratchpad is exempt.
- `.claude/settings.json` symlinks `.tasks` in; without it every `taskmgr` call fails with
  `no .tasks directory found`. Personal extras go in `.claude/settings.local.json`, which replaces
  that list rather than extending it — repeat `.tasks` there.
- `baseRef: head` branches from local HEAD, so fast-forward `main` first to build on the pushed tip.
- Nothing to install: the Go module cache and the `mise` toolchain are machine-global.
- `ExitWorktree` with `keep` while the PR is open, `remove` once it merges.

## Commits, branches, PRs

Use the `commit-commands:commit` skill for a standard commit, `commit-commands:commit-push-pr` to
commit + push + open a PR.

**Local delta:** branch off `main`, remote is `origin`, and add a `Refs <task-id>` line for the
`taskmgr` issue the change serves. Merge with `gh pr merge --merge` once `CI` is green — **agents
open the PR and stop there** unless asked to merge.

## Pre-handoff gates

- Code: `mise run quality`, then `mise run fmt:check` and `mise run scripts:check` — CI runs a
  superset of `quality` and fails on those two
  ([CODING.md → Quality Gates](CODING.md#quality-gates)).
- Docs: open every path, command, and link you touched.

`scripts/git-hooks/pre-commit` formats staged `*.go` files and checks nothing else. Install it per
clone with `mise run hooks:install`, which keeps `core.hooksPath` relative so it resolves inside
each worktree.

## Landing the plane

1. `taskmgr close <id>` finished work, `taskmgr create` follow-ups, `taskmgr update <id>` whatever
   the change made stale.
2. `commit-commands:commit-push-pr`.
3. `gh pr checks` green.

The change is not done until the PR is open with green checks and `git status` in the worktree is
clean.
