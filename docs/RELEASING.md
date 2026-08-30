# Releasing

`.github/workflows/release.yml` publishes releases with GoReleaser, dispatched by
hand against a release tag. The run builds, vets and tests before publishing, so
a green run is the provenance for that commit.

A release carries 4 archives, a per-archive SPDX SBOM (syft), a checksums file
signed keyless with cosign (`.sig` + `.pem`), and SLSA build provenance from
`actions/attest-build-provenance`. Archive names —
`taskmgr-ui_0.16.0_linux_x64.tar.gz` — let installers such as `mise` auto-detect
the right asset. Version injection: [CODING.md](CODING.md) → Version/build
metadata.

Two settings that look wrong and are not: cosign is pinned to **v2.6.3** because
v3 defaults to `--new-bundle-format`, which the `--output-signature` and
`--output-certificate` flags in `.goreleaser.yaml` do not accept; and the
provenance step keeps `continue-on-error` for user-owned **private** forks, where
attestation is unavailable.

## Cut a release

`mise run ci` gates every step. Drive the binary per [RUNNING.md](RUNNING.md) as
well when the release changes runtime behaviour.

1. Pick `vX.Y.Z` and confirm it is free: `git tag --list "v*"`.
2. Write the `CHANGELOG.md` section (see below) and land it like any other
   change: worktree, PR, merge ([CHANGE-WORKFLOW.md](CHANGE-WORKFLOW.md)). `main`
   is not a working branch, and a release is no exception.
3. Fast-forward `main` to the merged commit and run `mise run ci` on it.
4. Tag that commit and push the tag:

   ```bash
   git tag -a vX.Y.Z -m "taskmgr-ui vX.Y.Z"
   git push origin vX.Y.Z
   ```

5. Dispatch the workflow and watch it. The run takes a few seconds to register;
   re-run the lookup if it returns nothing. GoReleaser creates or updates the
   GitHub release itself, with notes generated from the commits.

   ```bash
   gh workflow run release.yml --ref vX.Y.Z
   gh run watch "$(gh run list --workflow=release.yml --branch vX.Y.Z --limit 1 --json databaseId -q '.[0].databaseId')" --exit-status
   ```

6. Verify. Expect 4 archives, 4 `.sbom.json`, `…_checksums.txt` and both
   `…_checksums.txt.sig` and `…_checksums.txt.pem`.

   ```bash
   gh release view vX.Y.Z --json assets -q '.assets[].name'
   gh release download vX.Y.Z -p '*_linux_x64.tar.gz'
   gh attestation verify taskmgr-ui_X.Y.Z_linux_x64.tar.gz -R hk9890/task-manager-ui
   ```

**The dispatched ref must be the tagged commit.** `workflow_dispatch` reads the
workflow file from the ref it runs on, and GoReleaser requires the checked-out
commit to be the tagged one. A `release.yml` change that a release needs must
live in the tagged commit — commit it, then move the tag
(`git tag -f -a vX.Y.Z -m … && git push -f origin vX.Y.Z`).

Re-dispatching against an existing tag overwrites its assets instead of failing
(`release.replace_existing_artifacts`). To add a missing asset without a new tag:
`gh release upload vX.Y.Z dist/* --clobber`.

## What goes in the CHANGELOG section

Write it for the operator, not for this repository. Its reader runs `taskmgr-ui`
and has never opened the source. An entry earns its place by telling them what
they must do, what they will see that they did not see before, or what was wrong
that is now fixed. State the symptom; the cause belongs in the commit message.

- Never name an internal symbol, a Go package, a third-party library, or a test.
  `lipgloss measured the U+FE0F variation selector as two cells` is a commit
  message; `info and warning toasts drew an empty box with no text` is an entry.
- A change with no user-visible effect gets no entry — a refactor, added
  coverage, a doc fix, a dependency bump nobody has to act on.
- A security entry leads with exposure: what it let an attacker do, and whether
  the shipped defaults were enough.
- Anything that now refuses a config, a store or a habit the operator already has
  leads with **Action required** and names the fix.

## Local fallback

When Actions can't be used. Needs `goreleaser` v2 in PATH
(`go install github.com/goreleaser/goreleaser/v2@latest`), a clean tree on the
tagged commit, and `mise run ci` green there — that run substitutes for the CI
provenance.

```bash
GITHUB_TOKEN=$(gh auth token) goreleaser release --clean --skip=sign --skip=sbom
```

This produces binaries and checksums but no signing, SBOMs or provenance. Drop
`--skip=sign` with `cosign` installed — keyless signing locally needs an
interactive Sigstore flow, which the workflow avoids with its OIDC token — and
drop `--skip=sbom` with `syft` installed. SLSA provenance needs that same OIDC
token and cannot be produced locally. Verify as in step 6.

## Verifying a downloaded release

The signing identity is the release workflow file at the ref the release was
dispatched on — a tag ref for the flow above.

```bash
cosign verify-blob \
  --certificate taskmgr-ui_<version>_checksums.txt.pem \
  --signature  taskmgr-ui_<version>_checksums.txt.sig \
  --certificate-identity "https://github.com/hk9890/task-manager-ui/.github/workflows/release.yml@refs/tags/v<version>" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  taskmgr-ui_<version>_checksums.txt
sha256sum -c taskmgr-ui_<version>_checksums.txt --ignore-missing
```
