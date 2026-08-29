# Reviewing

Run the `code-review` skill for the correctness pass. This file is what a `taskmgr-ui` review must
cover on top of it, and wins where the two disagree. The rules live in the docs named here — read the
one the change touches and flag what it does not respect.

## Severity

Blocking — the change does not merge until it is fixed:

- A Core Architectural Rule broken ([CODING.md](CODING.md)).
- The shell-launcher security rule broken ([CODING.md](CODING.md)).
- A same-PR doc update missing (below).

Everything else this file raises is a suggestion.

## What no gate can catch

- **The UI law** — [DESIGN-GUIDE.md](DESIGN-GUIDE.md) governs every change under `internal/ui/` and
  `internal/mode/`: colour roles, the issue token vocabulary, glyphs, shared chrome, the selection
  chevron, overlay placement, ANSI-aware width math. Nothing enforces it.
- **The architectural rules** — [CODING.md](CODING.md)'s Core Architectural Rules. The guardrail test
  proves one import ban from rule 1 plus a package-path ban; every numbered rule still needs reading.
- **Doc ownership and placement** — content put in the wrong file is governed by
  [DOCUMENTING.md](DOCUMENTING.md) and the `instruction-writing:writing-project-docs` skill. The
  citation and anchor gates prove that paths resolve and that fragments name headings, never that a
  passage belongs where it sits.
- **Config surface** — a new config key, keybinding, or launcher placeholder is governed by
  [CONFIGURATION.md](CONFIGURATION.md).

## Shell-launcher security

Blocking. [CODING.md](CODING.md)'s Shell-launcher security rule states the two forbidden shapes;
check the change against it, not against memory.

`app.ValidateLauncherDefinitions` runs at every start and under `--check-config`, so an unsafe
definition in a config file never reaches a running app. Review what the validator cannot see: issue
content formatted into a shell body from Go, and unsafe launcher examples added to docs.

## What must land in the same PR

- A new or renamed **keybinding** reaches [user-guide/key-bindings.md](user-guide/key-bindings.md),
  and a new **action** also reaches [CONFIGURATION.md](CONFIGURATION.md)'s supported-actions list —
  nothing proves that list still matches `internal/config/keybindings.go`.
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
- A golden diff in the PR needs a reason ([TESTING.md](TESTING.md)).

## Not a finding

- Anything `mise run ci` already rejects. Report only what a green gate would still ship broken.
- Style the narrow lint scope accepts ([CODING.md](CODING.md)) is a suggestion, never a blocker.
- The absence of a shared board/search list container. That is a recorded decision, not an oversight
  ([DESIGN-GUIDE.md](DESIGN-GUIDE.md)).
