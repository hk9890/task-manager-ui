# Releasing

This repository uses a **tag-triggered GitHub Actions release flow** backed by
GoReleaser.

## Scope and current state

- CI provenance is available via GitHub Actions workflow `.github/workflows/ci.yml` (`CI` workflow), which on `push` and `pull_request` runs the full quality gate set — script syntax checks, format check, lint, build, vet, architecture guardrails, unit tests, and a `test:coverage` threshold gate — across an ubuntu/macos/windows matrix.
- Release artifacts are published by `.github/workflows/release.yml` using `.goreleaser.yaml` whenever a `v*` tag is pushed.
- Release/snapshot builds inject `taskmgr-ui --version` metadata via GoReleaser ldflags into `github.com/hk9890/task-manager-ui/internal/version` (`Version`, `Commit`, `Date`), while local developer builds keep the fallback `dev` / `unknown` values defined in `internal/version/version.go`. See `docs/CODING.md` Version/build metadata behavior for the full symbol list.
- Release archives are intentionally named for installer compatibility, for example `taskmgr-ui_0.2.0_linux_x64.tar.gz` and `taskmgr-ui_0.2.0_macos_arm64.tar.gz`, so tools like `mise` can auto-detect the correct asset.
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

3. Run the pre-handoff quality gate (`mise run quality`); see `docs/CODING.md`
   Quality Gates for the full task set.

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
   git tag -a vX.Y.Z -m "taskmgr-ui vX.Y.Z"
   ```

5. Push the tag to origin to trigger the automated release workflow:

   ```bash
   git push origin vX.Y.Z
   ```

6. Watch the `Release` workflow complete successfully. It will:

   - build `taskmgr-ui` archives for supported platforms
   - create or update the GitHub release for the tag
   - upload release assets and checksums

7. Post-release verification:

   ```bash
   gh release view vX.Y.Z
   gh release view vX.Y.Z --json assets
   git ls-remote --tags origin "refs/tags/vX.Y.Z"
   ```

## Local release fallback (workflows paused)

When the GitHub Actions workflows are temporarily disabled (for example, while
private-repo Actions billing is paused), releases can be built and published
locally from the same `.goreleaser.yaml` used by the CI release job. The
substitute provenance is the local `mise run quality` output recorded on the
release commit.

Prerequisites: `goreleaser` v2 in PATH (`go install github.com/goreleaser/goreleaser/v2@latest`), a `GITHUB_TOKEN` with `repo` scope (typically `gh auth token`), and a clean working tree on the release commit.

Procedure:

1. Confirm `mise run quality` passes on the release commit (substitutes for
   the CI provenance gate).
2. Create and push the annotated tag:

   ```bash
   git tag -a vX.Y.Z -m "taskmgr-ui vX.Y.Z"
   git push origin vX.Y.Z
   ```

3. Build, publish, and upload assets locally:

   ```bash
   GITHUB_TOKEN=$(gh auth token) goreleaser release --clean --skip=sign --skip=sbom
   ```

   - `--skip=sign` skips cosign signing (requires `cosign` + interactive
     Sigstore OIDC flow). Drop the flag and install `cosign` to restore
     signed checksums.
   - `--skip=sbom` skips SPDX SBOM generation (requires `syft`). Drop the
     flag and install `syft` to restore per-archive SBOMs.
   - SLSA build provenance (`actions/attest-build-provenance`) is workflow-only
     and is not produced by this fallback path.

4. Verify the release the same way as the CI flow (see Post-release
   verification under the main release flow above).

When Actions billing is restored, re-enable the `push`/`pull_request` and
`push: tags: v*` triggers in `.github/workflows/ci.yml` and
`.github/workflows/release.yml` to return to the documented tag-triggered
flow.

## Verifying a downloaded release

Each release includes a cosign-signed checksum file and per-archive SBOM
(SPDX-JSON). Download the `.sig` and `.pem` files for the checksum from the
GitHub release page, then run:

```bash
cosign verify-blob \
  --signature taskmgr-ui_<version>_checksums.txt.sig \
  --certificate taskmgr-ui_<version>_checksums.txt.pem \
  taskmgr-ui_<version>_checksums.txt
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
