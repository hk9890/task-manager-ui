---
id: bwb-0igwdl
title: Bump task-manager SDK to v0.6.0
status: closed
type: chore
priority: 2
creator: hans
created: 2026-06-26T18:03:01Z
updated: 2026-06-26T18:20:41Z
closed: 2026-06-26T18:20:41Z
close_reason: Bumped SDK v0.4.0->v0.6.0 (no API breakage). Aligned UI search to AND-of-words (TextAllWords) in taskmgr + memory backends to match the CLI; fixed search space-drop bug. Full quality gate + runtime UI verification green.
---

Upstream released task-manager v0.6.0 (2026-06-26); repo pinned at v0.4.0. Bump SDK dep, adapt code if the API changed, run full quality gate + runtime UI verification. Notable v0.6.0 surfaces: search semantics (SearchExpr/Criteria.TextMatch, zero-value stays TextPhrase so existing callers unchanged); central stores (Resolve/Stores/InitCentral). Verify build/tests green and runtime UI good.
