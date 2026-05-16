# Coverage Baseline — 2026-05-16

Captured under ticket `beads-workbench-ug82.4` to inform the CI coverage gate in `ug82.5`.

## Numbers

| Scope | Total Coverage |
|---|---|
| Unit only (`go test -race ./...`) | **75.6%** |
| Combined unit + integration (`go test -race -tags=integration ./...`) | **76.5%** |

**Recommended initial CI gate: 71%** (combined baseline 76.5% minus ~5pts, rounded down). Sets a ratchet without blocking day-1 contributions.

## Per-package coverage (combined, lowest first)

| Coverage | Package | Note |
|---|---|---|
| 28.5% | `internal/testing/fakes` | Test infrastructure — many fake methods unused by current tests |
| 43.6% | `internal/ui/modal` | Modal lifecycle has untested error paths |
| 52.4% | `internal/testing/ui` | Test helpers — some assertions unused |
| 60.0% | `internal/testing/e2e/embeddedfixture` | Setup script helpers; partial coverage |
| 61.0% | `internal/ui/styles` | Many style variants not exercised by tests |
| 62.2% | `internal/gateway/beads/contract` | Test scaffolding — only exercised via the contract runners |
| 69.5% | `internal/mode/search` | Search edge paths (errors, debounce) untested |
| 72.3% | `cmd/bwb` | Startup/wiring code — main path covered, edge paths not |
| 75.2% | `internal/launcher/editor` | |
| 76.7% | `internal/ui/board` | |

## Reading the table

- Test-infrastructure packages (`internal/testing/*`, `internal/gateway/beads/contract`) appearing low is expected — their job is to be USED by other tests, not to test themselves. They drag the global number down without representing a real gap.
- True production gaps worth tracking: `internal/ui/modal` (43.6%), `internal/mode/search` (69.5%), `cmd/bwb` startup edges.

## Method

```bash
CGO_ENABLED=1 go test -race -coverprofile=cov-unit.out -covermode=atomic ./...
go tool cover -func=cov-unit.out | tail -1

CGO_ENABLED=1 go test -race -tags=integration -coverprofile=cov-all.out -covermode=atomic ./...
go tool cover -func=cov-all.out | tail -1
```

## Hand-off to ug82.5

- Gate threshold: **71%** (combined).
- Gate runs on `cov-all.out` (combined profile), not unit-only — combined reflects the real coverage of production code given that integration tests exercise the gateway↔bd seam.
