# Releasing

This repository uses a **tag-triggered GitHub Actions release flow** backed by
GoReleaser.

## Scope and current state

- CI provenance is available via GitHub Actions workflow `.github/workflows/ci.yml` (`CI` workflow), which runs `go build ./cmd/bwb`, `go vet ./...`, and `go test ./...` on `push` and `pull_request`.
- Release artifacts are published by `.github/workflows/release.yml` using `.goreleaser.yaml` whenever a `v*` tag is pushed.
- Release archives are intentionally named for installer compatibility, for example `bwb_0.2.0_linux_x64.tar.gz` and `bwb_0.2.0_macos_arm64.tar.gz`, so tools like `mise` can auto-detect the correct asset.
- Release verification should include both local operator checks and the corresponding successful CI workflow run(s) for the release commit.
- Release visibility policy: this repository remains **private** and releases created here are **internal-only** unless a future maintainer decision explicitly changes that policy.

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

5. Push the tag to origin to trigger the automated release workflow:

   ```bash
   git push origin vX.Y.Z
   ```

6. Watch the `Release` workflow complete successfully. It will:

   - build `bwb` archives for supported platforms
   - create or update the GitHub release for the tag
   - upload release assets and checksums

7. Post-release verification:

   ```bash
   gh release view vX.Y.Z
   gh release view vX.Y.Z --json assets
   git ls-remote --tags origin "refs/tags/vX.Y.Z"
   ```

## Backfilling or correcting release assets

If a tag or release already exists but is missing assets, upload corrected
artifacts to the existing release instead of creating a new tag:

```bash
gh release upload vX.Y.Z dist/* --clobber
```

## Repository visibility policy

This repository is intentionally private. Treat `gh release create` / `gh release edit`
outputs in this repository as internal distribution artifacts only.

If public release visibility is desired in the future, record a new maintainer policy
decision first, then update this guide and create concrete implementation tracking for
any required repository/settings changes.
