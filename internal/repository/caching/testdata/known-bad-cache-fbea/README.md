# Known-bad cache files (fbea reproducer)

These are real cache files captured from a user's `~/.cache/bwb/` on 2026-05-24
demonstrating the bug tracked in `beads-workbench-fbea` ("caching layer:
Dashboard never seeds memory").

## Provenance

- Project: this repo (`beads-workbench`); project-hash directory was `7efab1bca69f`.
- Sessions captured: `d40f3356` (6075-byte repo.jsonl) and `a52f7fe0` (9635-byte repo.jsonl).
- Both files contain only IDs the user clicked into during that session — the
  Dashboard summaries never made it into `c.memory`, so `SaveNow` persisted
  only the click-trail.

## Symptom these files produce

On a fresh `mise run bwb --repo caching` where the bd hash matches
the manifest, `caching.Hydrate` recomputes Dashboard from the tiny in-memory
store and serves a board with only the persisted IDs. The user sees a near-empty
board even though `bd list --status open` returns 7 and `bd list --status closed`
returns 600+.

## Use

Wire these as integration-test fixtures for the deterministic repro called for
in fbea AC#3 ("drop a 1-issue cache file + matching-hash manifest, observe
correct board within one refresh"). The post-mortem test should also cover the
mid-session click-trail case (multiple IDs persisted) using session-a52f7fe0.

## Do not modify

These files are read-only fixtures. If you need to regenerate them, capture
fresh files first and update this README's provenance.
