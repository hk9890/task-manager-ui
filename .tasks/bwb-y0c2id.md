---
id: bwb-y0c2id
title: 'Search quality on task-manager backend: phrase-only multi-word, no ranking (rely on backend; decide upstream asks)'
status: open
type: bug
priority: 1
creator: hans
labels:
  - needs:discussion
created: 2026-06-14T14:22:21Z
updated: 2026-06-26T19:12:58Z
---

## Summary

Search quality is poor against the task-manager backend. Investigation (2026-06-14)
tested the live UI search path (`internal/repository/taskmgr/search.go` →
`Criteria.Text` → `text ~ "..."`) against this project's real `.tasks` store
(1 open issue, 806 closed).

Decision from this session: **the UI stays a thin adapter over task-manager's own
search.** We do NOT hand-roll tokenization/ranking in
`internal/repository/taskmgr/`. If the backend's search behavior is
unsatisfactory, raise it upstream against `github.com/hk9890/task-manager`. This
issue tracks that discussion + any upstream filing.

## Findings (evidence)

1. **Multi-word queries are phrase-only (contiguous substring).** The whole query
   is passed as `Criteria.Text`, compiling to a single `text ~ "phrase"`. Words
   must appear adjacent and in order:
   - `"search mode"` → 1 result, `"mode search"` → 0
   - `"dashboard total"` → 0 (both words present in a title, not adjacent)
   - `"dashboard  done"` (double space) → 0
   Users expect AND-of-words (Google/GitHub/Jira semantics). Verified the backend
   CAN do order-independent matching via raw expr
   (`text ~ "seed" && text ~ "pagination"` matches; `text ~ "seed pagination"`
   does not), but that is a backend/SDK capability, not something we want to
   reconstruct in the UI adapter.

2. **No relevance ranking; description matches add noise.** Results sort by
   `SortWork` (priority, then created), not match quality. `Criteria.Text` matches
   id+title+description, so description-only hits interleave above title hits.
   Example: searching `"search"` surfaces `bwb-00684 "Define core domain and
   gateway interfaces"` (matched on body) above issues with "search" in the title.

3. **Closed issues dominate (intentionally kept).** `search.go` sets
   `IncludeClosed: true` and the UI sets no status filter. With 806 closed vs 1
   open, every search returns 100% closed results and the single open issue never
   appears in the top 20. DECISION: keep including closed for now (search should
   span all history). Noted here as the consequence of that choice; revisit if it
   proves painful. (Note: the old `bd` backend excluded closed from search by
   default, so this is a behavior change.)

## Candidate upstream asks (task-manager SDK)

- AND-of-words / multi-term text matching as a first-class `Criteria` capability
  (currently only a single `Text` substring field).
- Optional relevance ranking for text search (title weighted over description).

## Acceptance / next step

- Decide which of the above (if any) to file upstream against task-manager.
- No UI-side search reimplementation unless explicitly decided otherwise.
