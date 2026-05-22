# Releasing

This repository uses a **tag-triggered GitHub Actions release flow** backed by
GoReleaser.

## Scope and current state

- CI provenance is available via GitHub Actions workflow `.github/workflows/ci.yml` (`CI` workflow), which on `push` and `pull_request` runs the full quality gate set — script syntax checks, format check, lint, build, vet, architecture guardrails, unit tests, and a `test:coverage` threshold gate — across an ubuntu/macos/windows matrix.
- Release artifacts are published by `.github/workflows/release.yml` using `.goreleaser.yaml` whenever a `v*` tag is pushed.
- Release/snapshot builds inject `bwb --version` metadata via GoReleaser ldflags into `github.com/hk9890/beads-workbench/internal/version` (`Version`, `Commit`, `Date`), while local developer builds keep the fallback `dev` / `unknown` values defined in `internal/version/version.go`. See `docs/CODING.md` Version/build metadata behavior for the full symbol list.
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

4. **Smoke check** — run the data-consistency smoke suite from the repo root:

   ```bash
   mise run smoke
   ```

   - If exit code is non-zero (any check shows `FAIL`), **do NOT tag** — fix the failure first.
   - Optional: validate against a second real DB to catch environment-specific issues (substitute any path that contains a `.beads/` directory):

     ```bash
     BWB_SMOKE_DIR=/path/to/external/repo mise run smoke
     ```

   See [`docs/RUNTIME_UI_VERIFICATION.md` — Pre-release data-consistency checks](./RUNTIME_UI_VERIFICATION.md#pre-release-data-consistency-checks) for a full explanation of what each check covers and which failures are release-blocking vs advisory.

5. Create an annotated tag on the release commit:

   ```bash
   git tag -a vX.Y.Z -m "bwb vX.Y.Z"
   ```

6. Push the tag to origin to trigger the automated release workflow:

   ```bash
   git push origin vX.Y.Z
   ```

7. Watch the `Release` workflow complete successfully. It will:

   - build `bwb` archives for supported platforms
   - create or update the GitHub release for the tag
   - upload release assets and checksums

8. Post-release verification:

   ```bash
   gh release view vX.Y.Z
   gh release view vX.Y.Z --json assets
   git ls-remote --tags origin "refs/tags/vX.Y.Z"
   ```

## Verifying a downloaded release

Each release includes a cosign-signed checksum file and per-archive SBOM
(SPDX-JSON). Download the `.sig` and `.pem` files for the checksum from the
GitHub release page, then run:

```bash
cosign verify-blob \
  --signature bwb_<version>_checksums.txt.sig \
  --certificate bwb_<version>_checksums.txt.pem \
  bwb_<version>_checksums.txt
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
