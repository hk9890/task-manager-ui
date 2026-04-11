# Releasing

This repository currently uses a **manual, operator-driven release flow**.

## Scope and current state

- CI provenance is now available via GitHub Actions workflow `.github/workflows/ci.yml` (`CI` workflow), which runs `go build ./cmd/bwb`, `go vet ./...`, and `go test ./...` on `push` and `pull_request`.
- Release verification should include both local operator checks and the corresponding successful CI workflow run(s) for the release commit.
- Release visibility policy (public vs private defaults and expectations) is **not decided yet** and is tracked in `beads-workbench-2rj`.

## Prerequisites

Run from the repository root.

1. Confirm a clean working tree:

   ```bash
   git status
   ```

2. Sync with remote before release work:

   ```bash
   git pull --rebase
   git status
   ```

3. Run required quality gates from `docs/CODING.md`:

   ```bash
   go build ./cmd/bwb
   go vet ./...
   go test ./...
   ```

4. Confirm CI provenance for the candidate release commit:

   - Ensure the `CI` GitHub Actions workflow for the commit completed successfully.
   - Treat that workflow run as part of release provenance evidence.

5. If the release includes user-facing/runtime behavior changes, run runtime verification guidance from:
   - [`docs/TESTING.md`](./TESTING.md)
   - [`docs/RUNTIME_UI_VERIFICATION.md`](./RUNTIME_UI_VERIFICATION.md)

## Release flow used in this repository

1. Choose the release version/tag (for example `v0.4.0`) and verify it does not already exist:

   ```bash
   git tag --list "v*"
   ```

2. Confirm whether release notes/changelog updates are needed for this release; prepare/update them before tagging.

3. Verify the release commit has a successful `CI` workflow run (release provenance checkpoint).

4. Create an annotated tag on the release commit:

   ```bash
   git tag -a vX.Y.Z -m "bwb vX.Y.Z"
   ```

5. Push the tag to origin:

   ```bash
   git push origin vX.Y.Z
   ```

6. Create or edit the GitHub release with `gh release`:

   ```bash
   gh release create vX.Y.Z --title "vX.Y.Z" --notes-file <path-to-notes>
   ```

   If a release already exists and needs correction:

   ```bash
   gh release edit vX.Y.Z --title "vX.Y.Z" --notes-file <path-to-notes>
   ```

7. Post-release verification:

   ```bash
   gh release view vX.Y.Z
   git ls-remote --tags origin "refs/tags/vX.Y.Z"
   ```

## Important decision checkpoint (pending)

Before publishing/finalizing visibility settings, check the status of `beads-workbench-2rj`. Do not invent or assume a public/private policy in this guide until that issue is resolved.
