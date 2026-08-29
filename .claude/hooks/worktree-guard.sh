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

reason='Create a persistent worktree with the EnterWorktree tool, not `git worktree add` — it creates the worktree under .claude/worktrees/ and switches the session into it, where the central taskmgr store still resolves by ancestor path (docs/CHANGE-WORKFLOW.md → Worktrees). Put a throwaway clean-build probe in /tmp or the scratchpad instead, where `git worktree add --detach` is allowed. If EnterWorktree cannot express what you need, say so rather than hand-rolling.'
jq -cn --arg r "$reason" '{hookSpecificOutput:{hookEventName:"PreToolUse",permissionDecision:"deny",permissionDecisionReason:$r}}'
exit 0
