# Beads Workbench

A terminal UI for browsing beads issues, creating and updating work, and launching external tools from issue context — the standalone successor to Perles.

**Tech Stack**: Go, Bubble Tea, `bd` CLI (beads issue tracker)

## Project Overview

Read `docs/OVERVIEW.md` for the runtime flow, package map, architectural boundaries, and supporting doc index.

## Coding

Read `docs/CODING.md` for build commands, package layout, and core architectural rules.

## Testing

Read `docs/TESTING.md` for test policy, verification depth, fixtures, and focused commands.

Use `docs/RUNTIME_UI_VERIFICATION.md` when a change touches runtime UI behavior.

## Releases

Read `docs/RELEASING.md` for the tag-triggered GitHub release flow backed by `.github/workflows/release.yml` and `.goreleaser.yaml`.

## Change Workflow

Read `docs/CHANGE-WORKFLOW.md` for tracker usage, quality gates, session completion, and push requirements.

Work is not done until `bd dolt push`, `git push`, and a final `git status` show the tracker and branch are fully landed.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:ca08a54f -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files
<!-- END BEADS INTEGRATION -->
