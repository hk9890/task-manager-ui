#!/usr/bin/env bash
# gen-scale-seed.sh — one-shot generator for scale-seed.json
#
# Produces internal/testing/e2e/embeddedfixture/scale-seed.json
# This script is a BUILD TOOL, not production code. It does not need to be
# deterministic across runs because the output artifact (scale-seed.json) is
# committed. Run once; commit the output.
#
# Usage: bash scripts/gen-scale-seed.sh [output-path]
# Default output: internal/testing/e2e/embeddedfixture/scale-seed.json

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT="${1:-$REPO_ROOT/internal/testing/e2e/embeddedfixture/scale-seed.json}"

# Run the Go generator
go run "$REPO_ROOT/cmd/gen-scale-seed/main.go" "$OUTPUT"

echo "Generated: $OUTPUT"
wc -l "$OUTPUT"
