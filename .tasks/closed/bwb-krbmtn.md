---
id: bwb-krbmtn
title: Search query box drops space key (multi-word queries impossible)
status: closed
type: bug
priority: 2
creator: hans
created: 2026-06-26T18:20:41Z
updated: 2026-06-26T18:20:41Z
closed: 2026-06-26T18:20:41Z
close_reason: 'Fixed: search query handler now accepts tea.KeySpace; regression test added. Ships in chore/bump-task-manager-sdk-v0.6.0.'
---

The search query handler in internal/mode/search/model.go only accepted tea.KeyRunes. Bubble Tea delivers a lone space as tea.KeySpace (Runes=[' ']), so spaces were silently dropped, making multi-word search impossible to type. Discovered while aligning UI search to v0.6.0 AND-of-words semantics (bwb-0igwdl). Fix: accept tea.KeySpace in the FocusQuery branch. Covered by TestSearchQueryAcceptsSpaceForMultiWord.
