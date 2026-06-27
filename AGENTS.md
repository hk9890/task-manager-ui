# Task Manager UI

A terminal UI for browsing task-manager issues, creating and updating work, and launching external tools from issue context.

**Tech Stack**: Go, Bubble Tea, the task-manager Go SDK (`github.com/hk9890/task-manager/sdk/tasks`)

## Project Overview

Read `docs/OVERVIEW.md` for the runtime flow, package map, architectural boundaries, and supporting doc index.

## Coding

Read `docs/CODING.md` for build commands, package layout, and core architectural rules.

Read `docs/CONFIGURATION.md` for the runtime config model, keybindings, and launcher interpolation reference.

Run `mise tasks` to see all available build/test/quality tasks (`mise run <task>` to execute).

## Testing

Read `docs/TESTING.md` for test policy, verification depth, fixtures, and focused commands.

Use `docs/RUNTIME_UI_VERIFICATION.md` when a change touches runtime UI behavior.

## Monitoring

Read `docs/MONITORING.md` for the centralized diagnostics surface, `--debug` behavior, persistent log location, and machine-visible evidence guidance.

## Releases

Read `docs/RELEASING.md` for the tag-triggered GitHub release flow backed by `.github/workflows/release.yml` and `.goreleaser.yaml`.

## Change Workflow

Read `docs/CHANGE-WORKFLOW.md` for tracker usage, quality gates, session completion, and push requirements.

## Issue Tracker (task-manager)

This project tracks its own dev work with **task-manager** (`taskmgr`), the file-based tracker whose store lives in `.tasks/`. Use `taskmgr` for ALL task tracking — do NOT use TodoWrite or ad-hoc markdown TODO lists. Run `taskmgr commands` for the full machine-readable command catalog, and see `docs/CHANGE-WORKFLOW.md` for the tracker-first rule and session-close protocol.
