#!/usr/bin/env bash
# PreToolUse (Bash) guard: steer persistent `git worktree add` to the EnterWorktree tool.
#
# Only Claude's own Bash calls hit this — the EnterWorktree tool is native and bypasses it.
# Throwaway clean-build probes into a scratch/tmp dir are allowed. Fail-safe: any parsing error
# exits 0 (a hook bug must never wedge the session).
set -u

input="$(cat)"
cmd="$(printf '%s' "$input" | jq -r '.tool_input.command // empty' 2>/dev/null)" || exit 0
[ -z "$cmd" ] && exit 0

# Only worktree *creation* is in scope (list/remove/prune are fine).
printf '%s' "$cmd" | grep -qE 'git[[:space:]]+worktree[[:space:]]+add' || exit 0

# Exempt throwaway probes: they target a scratch/tmp location, not a place you commit from.
if printf '%s' "$cmd" | grep -qE 'scratchpad|/tmp/|/private/tmp/|mktemp|\$\{?TMPDIR'; then
  exit 0
fi

reason='Use the EnterWorktree tool for a persistent worktree, not `git worktree add` — it creates the worktree under .claude/worktrees/ and enters it in one step, and it is what symlinks the shared .tasks store into the worktree (docs/CHANGE-WORKFLOW.md → Worktrees). A hand-rolled worktree has no .tasks, so taskmgr fails there. Exception: a throwaway clean-build probe (`git worktree add --detach <dir> <ref>` into /tmp or the scratchpad) is fine — put it there. If EnterWorktree cannot express what you need (e.g. a base branch other than main/HEAD), say so instead of hand-rolling.'
jq -cn --arg r "$reason" '{hookSpecificOutput:{hookEventName:"PreToolUse",permissionDecision:"deny",permissionDecisionReason:$r}}'
exit 0
