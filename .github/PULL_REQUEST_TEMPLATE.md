<!--
Keep this short. Detail belongs in the linked task-manager issue and commit messages.
Delete sections that do not apply.
-->

## What & why

<!-- One paragraph: what changed and why. Reference the task-manager issue. -->

Closes #XXX

## Verification

<!-- Which of the verification levels in docs/CHANGE-WORKFLOW.md you ran. -->

- [ ] `mise run quality` green locally
- [ ] Touched runtime UI? Ran `docs/RUNTIME_UI_VERIFICATION.md`
- [ ] Touched logging/diagnostics? Cross-checked `docs/MONITORING.md`
- [ ] Docs-only? Verified affected paths, commands, and links

## Screenshots / output

<!-- For UI changes, attach before/after. For CLI/log changes, paste sample output. -->

## Risk & rollback

<!-- One line: blast radius if this breaks, and how to revert. Most PRs: "trivial; revert the commit". -->
