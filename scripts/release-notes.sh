#!/usr/bin/env bash
# Print the CHANGELOG.md section for one release tag, for GoReleaser's
# --release-notes. Exits 1 when the section is missing or empty, so a dispatch
# against a tag whose changelog was never written fails before anything is
# published.
set -euo pipefail

tag="${1:?usage: scripts/release-notes.sh vX.Y.Z}"
changelog="$(dirname "$0")/../CHANGELOG.md"

notes="$(awk -v heading="## ${tag}" '
  $0 == heading { found = 1; next }
  /^## / { found = 0 }
  found { print }
' "$changelog")"

if [[ -z "${notes//[[:space:]]/}" ]]; then
  echo "release-notes: no '## ${tag}' section in CHANGELOG.md" >&2
  exit 1
fi

printf '%s\n' "$notes"
