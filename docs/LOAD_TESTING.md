# Load Testing

## Purpose

`mise run test:load` exercises the bwb data layer under a configurable synthetic workload and produces a timing report. It answers the question "how long does Dashboard / Search / Issue-detail take on a repo of shape X?" and provides a baseline for detecting performance regressions by hand.

**This is distinct from the scale-fixture parity tests** (`mise run test:integration`). Those tests assert *correctness* of the bd adapter on a fixed ~590-issue corpus; they never measure latency. The load test asserts nothing about correctness — it measures latency only, on a freshly generated repo whose shape you control.

## Quick Start

```bash
mise run test:load
```

Requires `bd` on PATH and a Go toolchain. No other setup needed.

The command:

1. Generates a fresh seeded beads repo in a temp directory (default: 30 issues).
2. Runs the measurement harness over the data layer (Dashboard cold/warm, caching lifecycle, search matrix, issue detail).
3. Writes a machine-readable JSON report to `./load-test-report.json`.
4. Prints a human-readable summary table to stdout.

**Expected wall-clock time:** ~90 seconds for the default 30-issue workload. Generation dominates — each `bd create` subprocess call costs ~0.7–1 second. This is a bd-subprocess floor, not a bwb performance issue.

## Configuration

All parameters are set via environment variables. There are no extra `mise` arguments. Pass env vars inline or export them before running:

```bash
LOAD_OPEN=100 LOAD_CLOSED=25 mise run test:load
```

| Variable | Default | Description |
|---|---|---|
| `LOAD_OPEN` | `20` | Open issues to generate |
| `LOAD_CLOSED` | `5` | Closed issues to generate |
| `LOAD_IN_PROGRESS` | `3` | In-progress issues to generate |
| `LOAD_BLOCKED` | `2` | Blocked issues to generate (each gets ≥1 blocker dep) |
| `LOAD_DEP_DENSITY` | `0.5` | Average dependency edges per issue |
| `LOAD_COMMENTS_PER` | `0` | Average comments per issue (disabled by default; see note below) |
| `LOAD_SEED` | `42` | Random seed — same seed + same vars = same workload shape |
| `LOAD_SAMPLES_COLD` | `5` | Samples for cold-path operations (Dashboard cold, cache hydrate) |
| `LOAD_SAMPLES_WARM` | `20` | Samples for warm-path operations (Dashboard warm, search, detail) |
| `LOAD_OUT` | `./load-test-report.json` | JSON report output path; use `-` to emit JSON to stdout instead of a file |

**Note on `LOAD_COMMENTS_PER`:** Each comment requires a separate `bd comment` subprocess call. At scale (e.g. 200 issues × 5 avg comments ≈ 1000 bd calls) this adds minutes to generation. Leave it at 0 unless you specifically need realistic comment depth.

## Example Invocations

### Default shape (30 issues, ~90s)

```bash
mise run test:load
```

Generates 20 open + 3 in-progress + 2 blocked + 5 closed issues with density 0.5 and seed 42.

### Medium profile (~4 min)

```bash
LOAD_OPEN=100 LOAD_CLOSED=25 LOAD_IN_PROGRESS=10 LOAD_BLOCKED=5 mise run test:load
```

### Large / big-repo profile (~30+ min)

```bash
LOAD_OPEN=500 LOAD_CLOSED=100 LOAD_IN_PROGRESS=50 LOAD_BLOCKED=20 mise run test:load
```

Use for finding latency cliffs. Expect the generation phase alone to exceed 30 minutes.

### Small-but-deep-deps shape

```bash
LOAD_OPEN=10 LOAD_CLOSED=2 LOAD_IN_PROGRESS=2 LOAD_BLOCKED=2 LOAD_DEP_DENSITY=2.0 mise run test:load
```

Generates 16 issues with ~2 dep edges each. Useful for verifying that dependency-graph traversal does not scale poorly.

### Reproducible baseline run

```bash
LOAD_SEED=100 LOAD_OUT=./baseline.json mise run test:load
```

Save with a named output path so you can compare it against a later run without overwriting.

## Where the Report Lands and How to Read It

### JSON report (`./load-test-report.json` by default)

The report has two top-level fields: `header` and `operations`.

#### `header`

```json
"header": {
  "generated_at": "2026-05-25T12:43:09.8046274Z",
  "bd_version": "bd version 1.0.4 (ce242a879)",
  "samples_cold": 5,
  "samples_warm": 20,
  "issue_detail_ids": ["lt-e0j", "lt-5sx", ...],
  "manifest": {
    "spec": {
      "Counts": {"blocked": 2, "closed": 5, "in_progress": 3, "open": 20},
      "DepDensity": 0.5,
      "CommentsPer": 0,
      "Seed": 42
    },
    "actual_counts": {"blocked": 2, "closed": 5, "in_progress": 3, "open": 20},
    "actual_edges": 15,
    "issues_path": "/tmp/bwb-loadtest-1512193870/.beads",
    "bd_version": "bd version 1.0.4 (ce242a879)"
  }
}
```

| Field | Meaning |
|---|---|
| `generated_at` | RFC3339 timestamp of the run |
| `bd_version` | bd CLI version string captured at measurement time |
| `samples_cold` / `samples_warm` | Number of samples taken for cold / warm operations |
| `issue_detail_ids` | Issue IDs sampled for the detail measurement (up to 10) |
| `manifest.spec` | Requested workload parameters |
| `manifest.actual_counts` | Issues actually created per status (may differ from spec if generation fails) |
| `manifest.actual_edges` | Actual dependency edges created (see blocked-issue note below) |
| `manifest.issues_path` | Temp directory where the seeded `.beads/` repo lives |

#### `operations`

Each entry covers one measured operation:

```json
{
  "operation": "dashboard.cold",
  "sample_count": 5,
  "p50_ms": 1457.04,
  "p95_ms": 1466.55,
  "p99_ms": 1467.15,
  "max_ms": 1467.30
}
```

| Field | Meaning |
|---|---|
| `operation` | Stable name (see table below) |
| `sample_count` | Number of timing samples taken |
| `approximate` | Present and `true` when `sample_count < 5`; percentile values are valid but less reliable |
| `p50_ms` / `p95_ms` / `p99_ms` | 50th / 95th / 99th-percentile latency in milliseconds (linear interpolation) |
| `max_ms` | Worst-case observed latency in milliseconds |

**Operation names:**

| Operation | Description |
|---|---|
| `dashboard.cold` | `Repository.Dashboard` — fresh `CachingRepository` instance (cache miss, hits bd) |
| `dashboard.warm` | `Repository.Dashboard` — second call on the same instance (served from memory) |
| `cache.hydrate` | Caching-layer cold hydrate (equivalent to `dashboard.cold`; framed as a lifecycle measurement) |
| `cache.hot_read` | Caching-layer repeated read (equivalent to `dashboard.warm`) |
| `cache.force_fresh` | Dashboard on a newly constructed `CachingRepository` (dirty state) |
| `search.empty` | `Repository.Search` — empty query (triggers `bd list --all`) |
| `search.text=load-test` | `Repository.Search` — text query that matches generated issues |
| `search.text=issue` | `Repository.Search` — text query with broader match |
| `search.status=open` | `Repository.Search` — status filter |
| `search.workstate=ready` | `Repository.Search` — ready-state filter (uses `bd ready`) |
| `search.workstate=blocked` | `Repository.Search` — blocked-state filter (uses `bd blocked`) |
| `issue.detail.warm` | `Repository.Issue` — detail lookup served from in-memory cache |
| `issue.detail.cold` | `Repository.Issue` — detail lookup via fresh backing store call |

### Human summary table (stdout)

The command also prints a summary table:

```
=== bwb load-test measurement report ===
Generated:    2026-05-25T12:43:09.8046274Z
bd version:   bd version 1.0.4 (ce242a879)
Cold samples: 5
Warm samples: 20
Workload:     30 issues (open=20 in_progress=3 blocked=2 closed=5) edges=15

operation                           N  p50(ms)  p95(ms)  p99(ms)  max(ms)
-------------------------------------------------------------------------------
dashboard.cold                     5   1457.04  1466.55  1467.15  1467.30
dashboard.warm                    20      0.00     0.00     0.00     0.00
cache.hydrate                      5   1457.32  1471.01  1473.44  1474.04
cache.hot_read                    20      0.00     0.00     0.00     0.00
cache.force_fresh                  5   1459.09  1463.02  1463.39  1463.48
search.empty                      20    614.51   640.16   892.15   955.14
search.text=load-test             20    285.11   288.06   293.31   294.63
search.text=issue                 20    284.38   287.58   288.34   288.54
search.status=open                20    645.04   651.18   655.55   656.64
search.workstate=ready            20    516.24   523.67   528.24   529.38
search.workstate=blocked          20    172.42   183.57   183.72   183.76
issue.detail.warm                200      0.00     0.00     0.00     0.00
issue.detail.cold                 50    348.37   381.58   422.17   459.15
```

The `p50/p95/p99/max` columns for warm / hot / detail-warm ops print as `0.00` because those calls are served from in-memory cache and take sub-microsecond wall time. This is correct; those rows are not the signal.

## Comparing Two Reports Manually

Until automated regression detection exists, compare reports by eye using these rules:

### Signal fields (move with real changes)

| Operation | Baseline (30-issue default) | What a regression looks like |
|---|---|---|
| `dashboard.cold` | ~1400–1500 ms | Sustained increase of >20% across multiple runs |
| `cache.hydrate` | ~1400–1500 ms | Same as above (these two track closely) |
| `cache.force_fresh` | ~1400–1500 ms | Same |
| `search.empty` | ~600–650 ms | Sustained increase — often means `bd list --all` is slower |
| `search.status=open` | ~640–660 ms | Similar to `search.empty` |
| `search.workstate=ready` | ~510–530 ms | Uses `bd ready`; a spike here is in that path |
| `search.text=*` | ~280–295 ms | Text search path |
| `search.workstate=blocked` | ~170–185 ms | Uses `bd blocked` |
| `issue.detail.cold` | ~340–460 ms | Per-issue backing-store lookup |

### Noise fields (not actionable)

- `dashboard.warm`, `cache.hot_read`, `issue.detail.warm` — sub-microsecond in-memory reads. Run-to-run variation is >100% (nanoseconds of jitter dominate). Ignore these for regression purposes.

### Run-to-run tolerance

Same seed + same `LOAD_*` vars → identical workload **shape** (issue counts, dep structure, ID assignment order). Timing numbers vary roughly ±20% run-to-run due to system load, OS scheduling, and bd subprocess timing. A 10% increase on a single field in a single run is noise. A 30%+ increase across multiple fields that persists across 2–3 runs is a regression signal.

### Comparing procedure

1. Save a baseline: `LOAD_SEED=42 LOAD_OUT=./baseline.json mise run test:load`
2. Make your code change.
3. Run again with the same seed and output a comparison report: `LOAD_SEED=42 LOAD_OUT=./after.json mise run test:load`
4. Compare signal fields:
   ```bash
   diff \
     <(jq '.operations[] | select(.operation | test("cold|hydrate|force_fresh|search")) | {op: .operation, p50: .p50_ms, p95: .p95_ms}' baseline.json) \
     <(jq '.operations[] | select(.operation | test("cold|hydrate|force_fresh|search")) | {op: .operation, p50: .p50_ms, p95: .p95_ms}' after.json)
   ```
5. Verify `header.manifest.actual_counts` and `actual_edges` match between both reports (confirming same workload shape).
6. Accept ±20% variation as noise. Flag anything beyond ±30% as a candidate regression.

## Troubleshooting

### `error: bd not found on PATH`

Install `bd` (the beads issue tracker) and ensure it is on your shell's PATH before running `mise run test:load`. The command fails loudly with a non-zero exit code if bd is missing — it does not silently skip.

### Seed errors or generation failures

If the generator exits with a non-zero code before measurement begins, the issue is usually in the generation phase. Look for lines like:

```
error: generate: bd create: ...
```

Run `bd --version` to confirm bd is functional, then retry with a simpler workload:

```bash
LOAD_OPEN=5 LOAD_CLOSED=1 LOAD_IN_PROGRESS=0 LOAD_BLOCKED=0 mise run test:load
```

### Blocked issues and dep-density budget

A blocked issue requires at least one incoming blocker edge. If `LOAD_DEP_DENSITY × total_issues < LOAD_BLOCKED`, the mandatory blocker edges exceed the density budget. The generator emits a warning and proceeds — it does not fail. The `actual_edges` field in the report will reflect the actual edge count. This is expected behavior.

### Slow runs

Generation is the bottleneck — each `bd create` subprocess call takes ~0.7–1 second. There is no way to parallelize these calls. Expected times:

| Profile | Issues | Approximate wall time |
|---|---|---|
| Default | 30 | ~90s |
| Medium | 140 | ~4 min |
| Large | 670 | ~30+ min |

If a run seems slower than expected, check system load and bd version. Measurement itself (after generation) adds only a few seconds.

### Report overwrites previous run

The default output path is `./load-test-report.json` in the repo root. Use `LOAD_OUT` to write to a named path:

```bash
LOAD_OUT=./before.json mise run test:load
```

`./load-test-report.json` is listed in `.gitignore` and will not be committed.

## See Also

- `cmd/bwb-loadtest/README.md` — underlying CLI flag reference for the `gen` and `measure` subcommands (useful if you want to generate a persistent seeded repo separately from the measurement step).
- `scripts/load-test.sh` — the script backing `mise run test:load`; the header comment is the authoritative env-var reference.
- `internal/testing/loadgen/measure.go` — measurement harness and report schema source of truth.
