# Reviewing

Run the `code-review` skill for the correctness pass. This file is what a `taskmgr-ui` review must
cover on top of it, and wins where the two disagree. The rules live in the docs named here — read the
one the change touches and flag what it does not respect.

## What no gate can catch

- **The UI law** — [DESIGN-GUIDE.md](DESIGN-GUIDE.md) governs every change under `internal/ui/` and
  `internal/mode/`: colour roles, the issue token vocabulary, glyphs, shared chrome, the selection
  chevron, overlay placement, ANSI-aware width math. Nothing enforces it.
- **The architectural rules** — [CODING.md](CODING.md)'s Core Architectural Rules. The guardrail test
  proves only the two import bans; the other eight rules are prose.
- **The shell-launcher security rule** — a launcher template using `sh -c` or `sh -lc` must pass
  issue fields as positional arguments, never interpolate them into the shell body. Issue fields are
  operator-untrusted input. This one is blocking ([CODING.md](CODING.md)).
- **Config surface** — a new config key, keybinding, or launcher placeholder is governed by
  [CONFIGURATION.md](CONFIGURATION.md).

## What must land in the same PR

- A new or renamed **keybinding** reaches [user-guide/key-bindings.md](user-guide/key-bindings.md).
- A new or changed **config key or launcher placeholder** reaches
  [CONFIGURATION.md](CONFIGURATION.md).
- A new or moved **`internal/` package** reaches [OVERVIEW.md](OVERVIEW.md)'s package map. The
  citation guard proves only that cited paths still exist — never that a new one got cited.
- A new **CLI flag or exit code** reaches [CODING.md](CODING.md)'s startup section, and
  [README.md](../README.md) when a user would type it.
- A change to what the log records reaches [MONITORING.md](MONITORING.md).

## Tests

- The test drives the real component. [TESTING.md](TESTING.md) fixes which tier owns it, and a test
  asserting only against its own fakes proves nothing.
- What may be faked at all is [TESTING.md](TESTING.md)'s Shared Fake Seams list — an editor, a
  launcher, a process runner. A fake that stands in for a component this repository owns is a
  finding.
- A user-visible change needs the built binary driven, not only a green suite
  ([RUNNING.md](RUNNING.md)).
- A golden diff in the PR needs a reason. Regeneration against unchanged code produces no diff, so
  one that appears means the rendering really moved.

## Not a finding

- Anything `mise run ci` already rejects. Report only what a green gate would still ship broken.
- Style the linter accepts. Lint scope here is deliberately narrow — `staticcheck` and `errcheck`
  only — so a naming preference it tolerates is a suggestion, never a blocker.
- The absence of a shared board/search list container. That is a recorded decision, not an oversight
  ([DESIGN-GUIDE.md](DESIGN-GUIDE.md)).
