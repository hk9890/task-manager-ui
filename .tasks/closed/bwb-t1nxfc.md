---
id: bwb-t1nxfc
title: Remove beads tracker; switch dev issue tracking to task-manager (taskmgr)
status: closed
type: chore
priority: 1
creator: hans
created: 2026-06-14T10:58:13Z
updated: 2026-06-14T11:03:19Z
closed: 2026-06-14T11:03:19Z
---

Cut over the project's own dev issue tracker from beads (bd / .beads / Dolt) to task-manager (taskmgr / .tasks). Data already migrated (807 beads issues ~= 806 .tasks issues). Scope: remove bd from .mise.toml + add taskmgr@0.4.0; retarget CI attestation workaround; git rm -r .beads; commit .tasks; clean .gitignore; rewrite tracker-workflow docs (AGENTS.md, docs/CHANGE-WORKFLOW.md, README.md, docs/OVERVIEW.md, PR template).
