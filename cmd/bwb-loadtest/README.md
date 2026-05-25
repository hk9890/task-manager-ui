# bwb-loadtest

CLI tool for generating seeded beads repositories for bwb load testing.

## Requirements

- `bd` must be on PATH (beads issue tracker)
- An empty output directory for the seeded repo

## Build

```bash
go build ./cmd/bwb-loadtest
```

## Usage

```bash
bwb-loadtest gen --dir /tmp/my-test-repo [flags]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--dir` | (required) | Output directory for the seeded `.beads/` repo |
| `--open` | 20 | Number of open issues |
| `--in-progress` | 0 | Number of in_progress issues |
| `--blocked` | 0 | Number of blocked issues (each gets ≥1 blocker dep) |
| `--closed` | 0 | Number of closed issues |
| `--density` | 0.0 | Average dependency edges per issue |
| `--comments-per-issue` | 0 | Average comments per issue (disabled by default; slow at scale) |
| `--seed` | 1 | Random seed — same seed + flags = same workload shape |
| `--out` | `load-test-report.json` | Path to write the manifest JSON (use `-` for stdout) |

### Examples

Generate 50 open + 10 in_progress + 5 blocked issues with moderate dep density:

```bash
mkdir /tmp/loadtest-repo
bwb-loadtest gen \
  --dir /tmp/loadtest-repo \
  --open 50 --in-progress 10 --blocked 5 \
  --density 0.5 \
  --seed 42 \
  --out /tmp/loadtest-manifest.json
```

Run bwb against the generated repo:

```bash
bwb --cwd /tmp/loadtest-repo
```

## Reproducibility

Same `--seed` + same flags = same workload **shape** (counts per status,
creation order, dependency structure). Timestamps in `.beads/` differ run to
run because they reflect the wall-clock time of bd subprocess calls.

## Performance note on --comments-per-issue

Each comment requires a separate `bd comment` subprocess call. At scale
(e.g. 1000 issues × 5 comments = 5000 bd calls), this adds significant
wall-clock time. For the seeding phase of a load test, prefer
`--comments-per-issue 0` (the default) and add comments in a separate pass
if realistic comment data is needed.

## Manifest format

The manifest JSON captures the actual generated shape:

```json
{
  "spec": { ... },
  "actual_counts": { "open": 50, "in_progress": 10, "blocked": 5, "closed": 0 },
  "actual_edges": 32,
  "issues_path": "/tmp/loadtest-repo/.beads",
  "bd_version": "bd version 1.0.4 (ce242a879)",
  "warnings": []
}
```

## measure subcommand

The `measure` subcommand exercises the bwb data layer against a generated (or
pre-existing) `.beads/` directory and emits per-operation timing statistics.

### Usage

```bash
# Generate inline and measure immediately:
bwb-loadtest measure \
  --open 50 --in-progress 10 --blocked 5 --closed 5 \
  --density 0.5 --seed 42 \
  --samples-cold 5 --samples-warm 20 \
  --out /tmp/load-test-report.json

# Measure a pre-existing generated repo:
bwb-loadtest measure --dir /tmp/loadtest-repo --out /tmp/report.json

# Emit JSON report to stdout:
bwb-loadtest measure --dir /tmp/loadtest-repo --out -
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--dir` | (unset) | Pre-existing directory with a seeded `.beads/`; if absent, generates inline |
| `--open` | 50 | Open issues (inline generation) |
| `--in-progress` | 10 | In-progress issues (inline generation) |
| `--blocked` | 5 | Blocked issues (inline generation) |
| `--closed` | 5 | Closed issues (inline generation) |
| `--density` | 0.5 | Avg dep edges per issue (inline generation) |
| `--comments-per-issue` | 0 | Avg comments per issue (inline generation; slow) |
| `--seed` | 1 | Random seed (inline generation) |
| `--samples-cold` | 5 | Samples for cold-path operations (Dashboard cold, cache hydrate) |
| `--samples-warm` | 20 | Samples for warm-path operations (Dashboard warm, search, detail) |
| `--issue-detail-n` | 10 | Number of distinct IDs to sample for detail measurements |
| `--out` | `load-test-report.json` | JSON report path (use `-` for stdout) |

### Operations measured

| Operation name | Description |
|---|---|
| `dashboard.cold` | `Repository.Dashboard` on a fresh cache (cold path) |
| `dashboard.warm` | `Repository.Dashboard` served from in-memory cache (warm path) |
| `cache.hydrate` | CachingRepository cold hydrate (first Dashboard on fresh instance) |
| `cache.hot_read` | Repeated Dashboard calls on a pre-warmed instance |
| `cache.force_fresh` | Dashboard on a newly constructed instance (dirty state) |
| `search.<query>` | `Repository.Search` across the representative query matrix |
| `issue.detail.cold` | `Repository.Issue` — first call per ID (cache miss → backing) |
| `issue.detail.warm` | `Repository.Issue` — repeated calls per ID (cache hit → memory) |

### Report schema

```json
{
  "header": {
    "generated_at": "2026-05-25T12:00:00Z",
    "bd_version": "bd version 1.0.4",
    "samples_cold": 5,
    "samples_warm": 20,
    "manifest": { ... }
  },
  "operations": [
    {
      "operation": "dashboard.cold",
      "sample_count": 5,
      "p50_ms": 120.5,
      "p95_ms": 145.2,
      "p99_ms": 150.0,
      "max_ms": 155.3
    }
  ]
}
```

`approximate: true` is added to an operation when `sample_count < 5`; percentile
values are valid but less statistically reliable.

### Statistics convention

Percentiles use linear interpolation between adjacent order statistics (NumPy
default, R type 7):

    h = (p/100) × (n-1)
    result = v[⌊h⌋] + (h - ⌊h⌋) × (v[⌊h⌋+1] - v[⌊h⌋])

For `[1,2,...,10]`: p50 = 5.5, p95 = 9.55.

## Next steps

- `bjyt.1` — the `mise run test:load` task wires the generator and harness behind a single command.
- `bjyt.4` — docs/LOAD_TESTING.md adds the agent-runnable recipe.
