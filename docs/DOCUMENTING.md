# Documenting

The generic standard — which file owns what, and how a project doc is written — is the
`instruction-writing:writing-project-docs` skill. This file records only the delta, and wins where
the two disagree.

## The citation gate

`TestDocsCiteLivePaths` (`cmd/taskmgr-ui/doc_citation_hygiene_integration_test.go`) fails when a
tracked Markdown file cites a path under `internal/`, `cmd/`, `scripts/`, `docs/` or `.github/` that
does not exist. It runs under `mise run ci`, not the unit suite, because it shells out to `git`.

- **Cite a symbol, never a line number.** `path:42` and `path#L42` both fail the gate. A line anchor
  survives no edit above it, and no check can see that it moved, because the file still exists.
- A citation carrying a `*` passes when the shape still matches something, so `internal/mode/*` is
  fine.
- The gate proves cited paths resolve. It cannot prove a *symbol* still exists — `constructRepository`
  outlived its rename in two docs — so read the code when you cite a function.

## Doc trees outside the canonical set

- [DESIGN-GUIDE.md](DESIGN-GUIDE.md) owns the visual and interaction law for `internal/ui/` and
  `internal/mode/`. It sits outside the canonical topic set deliberately: folding the interaction law
  into [CODING.md](CODING.md) would bury the implementation constraints an agent opens that file for,
  and this product *is* its terminal surface.
- [CONFIGURATION.md](CONFIGURATION.md) owns the runtime config model — the config file, keybinding
  resolution, and the launcher interpolation surface. It is a reference for a data format, so it
  outranks CODING.md inside `internal/config/`.
- [user-guide/](user-guide/) is written for someone who runs `taskmgr-ui` and never opens this
  repository. It may name a source path only where the reader sets the thing themselves — the
  keybinding defaults cite `internal/config/keybindings.go` because that is what a config override
  replaces.

## Decisions

What this repository has decided not to document, and why. Re-open one here rather than filling the
gap somewhere else.

- **`CHANGELOG.md` stays.** The GitHub release notes are generated per release, but the tracked file
  is the one history readable from a checkout with no network.
- **No prose architecture doc.** [OVERVIEW.md](OVERVIEW.md) maps the packages and stops. The
  boundaries are stated once in [CODING.md](CODING.md)'s Core Architectural Rules and are blocking at
  review ([REVIEWING.md](REVIEWING.md)); a third copy in prose would drift from both.
- **No manual gate list.** [CODING.md](CODING.md) says which gate enforces a rule and where that rule
  is written; `mise tasks` and `.mise.toml` say what each task runs. Restating a task body in prose
  is a copy that goes stale on the next edit.
- **Every runbook that needs a real terminal lives in [RUNNING.md](RUNNING.md).** Three behaviours —
  height-scaled closed limits, chevron following, Done-column pagination — cannot be driven under the
  PTY harness. They are procedures for driving the product, not test policy, so TESTING.md links them
  rather than holding them.
