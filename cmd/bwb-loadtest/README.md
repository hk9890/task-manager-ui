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

## Next steps

- `bjyt.3` — the measurement harness consumes this API to time `Repository.Dashboard`, search, and detail operations.
- `bjyt.1` — the `mise run test:load` task wires the generator and harness behind a single command.
