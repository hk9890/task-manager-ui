# Releasing

This repository publishes releases with a **manually dispatched GitHub Actions
workflow** backed by GoReleaser, plus a local fallback for when Actions can't be
used.

## Scope and current state

- Release artifacts are published by `.github/workflows/release.yml` (the
  `Release` workflow) using `.goreleaser.yaml`. The workflow is triggered
  manually via `workflow_dispatch` against a release tag; the automatic
  `push: tags: v*` trigger is currently disabled (see
  [Re-enabling the automatic tag trigger](#re-enabling-the-automatic-tag-trigger)).
- The build depends on the **private** module
  `github.com/hk9890/task-manager/sdk`. The workflow authenticates that fetch
  with the `TASKMGR_SDK_TOKEN` repository secret (a fine-grained PAT with
  `Contents: Read` on `hk9890/task-manager`); without it the build fails fast
  with an explicit error. See [Prerequisites](#prerequisites).
- Each release run builds, vets, and tests before publishing, so a green run is
  itself release provenance for the commit.
- Release/snapshot builds inject `taskmgr-ui --version` metadata via GoReleaser
  ldflags into `github.com/hk9890/task-manager-ui/internal/version` (`Version`,
  `Commit`, `Date`), while local developer builds keep the fallback `dev` /
  `unknown` values defined in `internal/version/version.go`. See `docs/CODING.md`
  Version/build metadata behavior for the full symbol list.
- Release archives are intentionally named for installer compatibility, for
  example `taskmgr-ui_0.11.0_linux_x64.tar.gz` and
  `taskmgr-ui_0.11.0_macos_arm64.tar.gz`, so tools like `mise` can auto-detect
  the correct asset.
- **Signing:** GoReleaser signs the checksums file with **cosign** (keyless, via
  the workflow's OIDC token), producing `<checksums>.txt.sig` and
  `<checksums>.txt.pem`. cosign is pinned to **v2.6.3** because cosign v3
  defaults to `--new-bundle-format`, which is incompatible with the
  `--output-signature`/`--output-certificate` flags in `.goreleaser.yaml`.
- **SBOMs:** per-archive SPDX-JSON SBOMs (`<archive>.sbom.json`) are produced
  with **syft**.
- **SLSA build provenance is not produced** for this repository.
  `actions/attest-build-provenance` is unavailable on user-owned private
  repositories ("Feature not available for user-owned private repositories"), so
  the step runs `continue-on-error` and is effectively skipped. It would start
  working automatically if the repository became public or org-owned.
- Release visibility policy: this repository remains **private** and releases
  created here are **internal-only** unless a future maintainer decision
  explicitly changes that policy.

## Prerequisites

Run from the repository root.

1. **`TASKMGR_SDK_TOKEN` secret exists** (one-time setup). Create a fine-grained
   PAT — resource owner `hk9890`, repository access limited to
   `hk9890/task-manager`, **Repository permissions → Contents: Read-only** — and
   add it as a repository secret:

   ```bash
   gh secret set TASKMGR_SDK_TOKEN --repo hk9890/task-manager-ui
   ```

   Editing an existing PAT's permissions does **not** change its token value, so
   an already-stored secret stays valid after a permission change.

2. Confirm a clean working tree:

   ```bash
   git status
   ```

3. Sync with remote before release work:

   ```bash
   git pull --rebase
   git status
   ```

4. Run the pre-handoff quality gate (`mise run quality`); see `docs/CODING.md`
   Quality Gates for the full task set. The Release run repeats build/vet/test in
   CI, but running the gate locally first avoids spending a workflow run on an
   avoidable failure.

5. If the release includes user-facing/runtime behavior changes, run runtime
   verification guidance from:
   - [`docs/TESTING.md`](./TESTING.md)
   - [`docs/RUNTIME_UI_VERIFICATION.md`](./RUNTIME_UI_VERIFICATION.md)

## Release flow (GitHub Actions)

1. Choose the release version/tag (for example `v0.11.0`) and verify it does not
   already exist:

   ```bash
   git tag --list "v*"
   ```

2. Update `CHANGELOG.md` / release notes for this release and commit the release
   prep.

3. Run `mise run quality` on the release commit.

4. Create an annotated tag **on the release commit**:

   ```bash
   git tag -a vX.Y.Z -m "taskmgr-ui vX.Y.Z"
   ```

5. Push the commit and the tag:

   ```bash
   git push origin main
   git push origin vX.Y.Z
   ```

6. Dispatch the Release workflow against the tag:

   ```bash
   gh workflow run release.yml --ref vX.Y.Z
   ```

   > **The dispatched ref must be the tagged release commit.** `workflow_dispatch`
   > reads the workflow file from the ref it runs on, and GoReleaser requires the
   > checked-out commit to be the tagged one. If you ever need to change
   > `release.yml` itself as part of a release, that change must live in the
   > tagged commit — commit it, then move the tag onto it
   > (`git tag -f -a vX.Y.Z -m … && git push -f origin vX.Y.Z`).

7. Watch the run complete:

   ```bash
   gh run watch "$(gh run list --workflow=release.yml --limit 1 --json databaseId -q '.[0].databaseId')" --exit-status
   ```

   The run authenticates the private SDK, builds/vets/tests, then GoReleaser
   builds archives, generates SBOMs, signs the checksums, and creates/updates the
   GitHub release with auto-generated notes. The SLSA provenance step fails-soft
   (see [Scope and current state](#scope-and-current-state)).

8. Post-release verification:

   ```bash
   gh release view vX.Y.Z
   gh release view vX.Y.Z --json assets -q '.assets[].name'
   git ls-remote --tags origin "refs/tags/vX.Y.Z"
   ```

   Expect 4 archives, 4 `.sbom.json` files, `…_checksums.txt`, and both
   `…_checksums.txt.sig` and `…_checksums.txt.pem`.

`.goreleaser.yaml` sets `release.replace_existing_artifacts: true`, so
re-dispatching against an existing tag/release overwrites its assets instead of
erroring.

### Re-enabling the automatic tag trigger

The automatic `push: tags: v*` trigger in `release.yml` is currently commented
out (a holdover from when private-repo Actions billing was paused). Now that the
workflow authenticates the private SDK, the trigger can be restored so pushing a
`vX.Y.Z` tag releases automatically. Until then, use the manual
`workflow_dispatch` step above.

## Local release fallback

When GitHub Actions can't be used, build and publish locally from the same
`.goreleaser.yaml`. This path produces binaries + checksums but **no signing,
SBOMs, or SLSA provenance** (cosign keyless signing needs the workflow's
interactive OIDC; SBOMs need `syft`). The substitute provenance is the local
`mise run quality` output on the release commit.

Prerequisites: `goreleaser` v2 in PATH
(`go install github.com/goreleaser/goreleaser/v2@latest`), a `GITHUB_TOKEN` with
`repo` scope (typically `gh auth token`), local git credentials that can read the
private `hk9890/task-manager` SDK, and a clean working tree on the tagged release
commit.

1. Confirm `mise run quality` passes on the release commit (substitutes for the
   CI provenance gate).
2. Create and push the annotated tag:

   ```bash
   git tag -a vX.Y.Z -m "taskmgr-ui vX.Y.Z"
   git push origin vX.Y.Z
   ```

3. Build, publish, and upload assets locally:

   ```bash
   GITHUB_TOKEN=$(gh auth token) goreleaser release --clean --skip=sign --skip=sbom
   ```

   - `--skip=sign` skips cosign signing (requires `cosign` + an interactive
     Sigstore OIDC flow). Drop the flag and install `cosign` to restore signed
     checksums.
   - `--skip=sbom` skips SPDX SBOM generation (requires `syft`). Drop the flag
     and install `syft` to restore per-archive SBOMs.
   - SLSA build provenance is workflow-only and is not produced by this fallback
     (and is unavailable on this private repo regardless — see above).

4. Verify the release the same way as the Actions flow (see step 8).

## Verifying a downloaded release

The checksums file is signed with cosign keyless. Download `…_checksums.txt`,
its `.sig`, and its `.pem` from the GitHub release, then verify with the workflow
identity. The certificate's identity is the release workflow file at the **ref
the release was dispatched on** — a tag ref for the flow above:

```bash
cosign verify-blob \
  --certificate taskmgr-ui_<version>_checksums.txt.pem \
  --signature  taskmgr-ui_<version>_checksums.txt.sig \
  --certificate-identity "https://github.com/hk9890/task-manager-ui/.github/workflows/release.yml@refs/tags/v<version>" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  taskmgr-ui_<version>_checksums.txt
```

Then confirm a downloaded archive's checksum is listed:

```bash
sha256sum -c taskmgr-ui_<version>_checksums.txt --ignore-missing
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
any required repository/settings changes. Note that making the repository public (or
moving it under an organization) would also enable SLSA build provenance, which is
currently unavailable on this user-owned private repo.
