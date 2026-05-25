#!/usr/bin/env bash
# load-test.sh — run the bwb load-test suite end-to-end.
#
# Invoked by: mise run test:load
# Requires: bd on PATH, go toolchain
#
# Workload shape (override via LOAD_* env vars):
#   LOAD_OPEN            open issues          (default: 20)
#   LOAD_CLOSED          closed issues        (default: 5)
#   LOAD_IN_PROGRESS     in_progress issues   (default: 3)
#   LOAD_BLOCKED         blocked issues       (default: 2)
#   LOAD_DEP_DENSITY     avg dep edges/issue  (default: 0.5)
#   LOAD_COMMENTS_PER    avg comments/issue   (default: 0; each comment is a bd subprocess — slow at scale)
#   LOAD_SEED            random seed          (default: 42)
#   LOAD_SAMPLES_COLD    cold-path samples    (default: 5)
#   LOAD_SAMPLES_WARM    warm-path samples    (default: 20)
#   LOAD_OUT             report path          (default: ./load-test-report.json)
#
# Timing note: each issue requires ≥1 `bd create` subprocess call (~0.7–1s each).
# Generation dominates wall-clock time regardless of corpus size. Default 30-issue
# workload takes ~90s on typical hardware. This is a bd-subprocess floor, not a
# bwb performance issue.
#
# Larger profiles (set via LOAD_* env vars):
#   medium:  LOAD_OPEN=100 LOAD_CLOSED=25 LOAD_IN_PROGRESS=10 LOAD_BLOCKED=5   (~4 min)
#   large:   LOAD_OPEN=500 LOAD_CLOSED=100 LOAD_IN_PROGRESS=50 LOAD_BLOCKED=20 (~30+ min)
set -euo pipefail

# ---------- preconditions ----------

if ! command -v bd >/dev/null 2>&1; then
    echo "error: bd not found on PATH — install bd before running this task" >&2
    exit 1
fi

# ---------- workload defaults ----------

OPEN="${LOAD_OPEN:-20}"
CLOSED="${LOAD_CLOSED:-5}"
IN_PROGRESS="${LOAD_IN_PROGRESS:-3}"
BLOCKED="${LOAD_BLOCKED:-2}"
DEP_DENSITY="${LOAD_DEP_DENSITY:-0.5}"
COMMENTS_PER="${LOAD_COMMENTS_PER:-0}"
SEED="${LOAD_SEED:-42}"
SAMPLES_COLD="${LOAD_SAMPLES_COLD:-5}"
SAMPLES_WARM="${LOAD_SAMPLES_WARM:-20}"
OUT="${LOAD_OUT:-./load-test-report.json}"

echo "bwb load test: open=${OPEN} closed=${CLOSED} in_progress=${IN_PROGRESS} blocked=${BLOCKED} density=${DEP_DENSITY} seed=${SEED}"

# ---------- run ----------

exec go run ./cmd/bwb-loadtest measure \
    --open "${OPEN}" \
    --closed "${CLOSED}" \
    --in-progress "${IN_PROGRESS}" \
    --blocked "${BLOCKED}" \
    --density "${DEP_DENSITY}" \
    --comments-per-issue "${COMMENTS_PER}" \
    --seed "${SEED}" \
    --samples-cold "${SAMPLES_COLD}" \
    --samples-warm "${SAMPLES_WARM}" \
    --out "${OUT}"
