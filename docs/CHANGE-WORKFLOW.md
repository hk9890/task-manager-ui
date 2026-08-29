# Change Workflow

## Tracker

Track every unit of work in `taskmgr`, the project's own task-manager store — never TodoWrite or a
markdown TODO list.

- Open the issue before the worktree; `taskmgr commands` prints the full command catalog.
- The store is central, not a `.tasks` directory in this repo — `taskmgr where` resolves it.

## Worktrees

Every change starts in its own worktree; `main` is not a working branch.

- Create it with the `EnterWorktree` tool — `.claude/hooks/worktree-guard.sh` denies a persistent
  `git worktree add`. A throwaway `--detach` probe into `/tmp` or the scratchpad is exempt.
- `baseRef: head` branches from local HEAD, so fast-forward `main` first to build on the pushed tip.
- Uncommitted edits already in the main checkout: `git stash -u`, `EnterWorktree`, then
  `git stash pop` in the new tree. `EnterWorktree` builds a clean tree from a base ref and does not
  carry them across.
- Nothing to install: the Go module cache and the `mise` toolchain are machine-global.
- `ExitWorktree` with `keep` when you hand off the PR; a later session clears the merged tree and
  branch with `commit-commands:clean_gone`.

## Commits, branches, PRs

Use the `commit-commands:commit` skill for a standard commit, `commit-commands:commit-push-pr` to
commit + push + open a PR.

**Local delta:** branch off `main`, remote is `origin`, the subject is Conventional Commits
(`type(scope): imperative summary`), and add a `Refs <task-id>` line for the `taskmgr` issue the
change serves. Merge with `gh pr merge --squash` once `CI` is green — **agents open the PR and stop
there** unless asked to merge.

**PR body:** fill `.github/PULL_REQUEST_TEMPLATE.md`. Its Verification boxes are the depths: `mise
run ci` always, plus [RUNNING.md](RUNNING.md) when the change touches runtime UI,
[MONITORING.md](MONITORING.md) when it touches logging, and the docs check below when it is
docs-only.

## Pre-handoff gates

- Code: `mise run ci` — the merge gate, and exactly what the linux CI job runs
  ([CODING.md → What the tools enforce](CODING.md#what-the-tools-enforce)).
- Docs: `mise run ci` catches dead paths and anchors ([DOCUMENTING.md](DOCUMENTING.md)); by hand,
  re-read every symbol you cited and re-run every command you quoted.

`scripts/git-hooks/pre-commit` formats staged `*.go` files and checks nothing else — it is not a
gate. `core.hooksPath` is relative, so it fires inside each worktree too. Installing it is a
once-per-clone step ([CONTRIBUTING.md](../CONTRIBUTING.md)).

## Landing the plane

1. `taskmgr close <id>` finished work, `taskmgr create` follow-ups, `taskmgr update <id>` whatever
   the change made stale.
2. `commit-commands:commit-push-pr`.
3. `gh pr checks` green.
4. `git status` clean in the worktree.
