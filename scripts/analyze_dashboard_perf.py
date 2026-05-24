#!/usr/bin/env python3
"""Analyze bwb dashboard refresh performance from persistent session logs.

Reads JSONL log files written by `internal/logging` (default location:
$XDG_STATE_HOME/bwb/bwb-<session_id>.log, fallback ~/.local/state/bwb/) and
reports cold-load wall time, and per-call latency for the dashboard's
5-command board refresh.

The argv-level in-process read cache was removed in 8pxi.7. Sessions
generated after that change will always report hit=0; all gateway activity
now appears as "bd command finished". Hit-rate tracking is retained for
backward compatibility with older log files that still contain cache hit
lines.

The gateway emits two log messages this script keys on:
  - "bd command finished"   (a bd subprocess execution; all reads after 8pxi.7)
  - "bd command cache hit"  (a read from the in-process cache; pre-8pxi.7 only)

Usage
-----
  scripts/analyze_dashboard_perf.py                       # list projects
  scripts/analyze_dashboard_perf.py --project <name>      # all sessions for one project
  scripts/analyze_dashboard_perf.py --all                 # aggregate every project
  scripts/analyze_dashboard_perf.py --session <id>        # one session deep-dive
  scripts/analyze_dashboard_perf.py --log-dir <path>      # override log directory

Project matching is a substring against the absolute project_root recorded in
each log. The bare default behaviour (no flags) prints one line per project so
you can pick the right --project filter.
"""
from __future__ import annotations

import argparse
import json
import os
import statistics
import sys
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path


# Commands that form the "cold-load" of the dashboard. Sourced from
# internal/mode/board/model.go:startReload (loadReadyExplain, loadInProgress,
# loadClosed, loadClosedCount, loadStoredBlocked). The "health check" ping at
# bwb startup is treated separately.
BOARD_OPERATIONS = frozenset({
    "ready explain",
    "query issues",
    "count issues",
})


@dataclass
class Call:
    """One bd-gateway interaction, hit or miss."""
    t_offset_s: float          # seconds since session start
    op: str                    # operation label
    argv: list[str]            # full argv (incl. "bd" head)
    duration_ms: int           # 0 for cache hits
    cache: str                 # "hit" or "miss"


@dataclass
class Session:
    session_id: str
    project_root: str
    build_version: str
    path: Path
    start_ts: datetime | None = None
    end_ts: datetime | None = None
    calls: list[Call] = field(default_factory=list)

    @property
    def duration_s(self) -> float:
        if self.start_ts is None or self.end_ts is None:
            return 0.0
        return (self.end_ts - self.start_ts).total_seconds()

    @property
    def misses(self) -> list[Call]:
        return [c for c in self.calls if c.cache == "miss"]

    @property
    def hits(self) -> list[Call]:
        return [c for c in self.calls if c.cache == "hit"]

    @property
    def hit_rate(self) -> float | None:
        total = len(self.calls)
        return (100.0 * len(self.hits) / total) if total else None

    def cold_load_time_s(self) -> float | None:
        """Wall time from session start to the last miss of the initial burst.

        We define the initial burst as: the contiguous run of misses that begins
        within 1s of session start and is followed by a >1.5s gap (i.e. the
        next bd call lands more than 1.5s after the previous one ends). Returns
        None if no misses are within the first second.
        """
        if not self.misses:
            return None
        if self.misses[0].t_offset_s > 1.0:
            return None
        prev_end = self.misses[0].t_offset_s
        last_in_burst = self.misses[0]
        for c in self.misses[1:]:
            gap = c.t_offset_s - prev_end
            if gap > 1.5:
                break
            last_in_burst = c
            prev_end = c.t_offset_s + (c.duration_ms / 1000.0)
        return last_in_burst.t_offset_s + (last_in_burst.duration_ms / 1000.0)


def load_session(path: Path) -> Session | None:
    """Parse one JSONL log file. Returns None if it has no gateway records."""
    session_id = project_root = build_version = ""
    start_ts: datetime | None = None
    end_ts: datetime | None = None
    calls: list[Call] = []

    try:
        with path.open() as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    r = json.loads(line)
                except json.JSONDecodeError:
                    continue
                ts_raw = r.get("timestamp")
                if not ts_raw:
                    continue
                try:
                    ts = datetime.fromisoformat(ts_raw)
                except ValueError:
                    continue
                if start_ts is None:
                    start_ts = ts
                    session_id = r.get("session_id", "")
                    project_root = r.get("project_root", "")
                    build_version = r.get("build_version", "")
                end_ts = ts

                msg = r.get("message", "")
                if msg not in ("bd command finished", "bd command cache hit"):
                    continue
                if r.get("component") != "gateway":
                    continue
                # Only count successful misses; non-zero exit codes are not
                # representative of normal load behaviour.
                if msg == "bd command finished" and r.get("exit_code", 0) != 0:
                    continue
                calls.append(Call(
                    t_offset_s=(ts - start_ts).total_seconds(),
                    op=r.get("operation", "?"),
                    argv=list(r.get("argv", [])),
                    duration_ms=int(r.get("duration_ms", 0)),
                    cache="hit" if msg == "bd command cache hit" else "miss",
                ))
    except OSError as e:
        print(f"warning: cannot read {path}: {e}", file=sys.stderr)
        return None

    if start_ts is None:
        return None
    return Session(
        session_id=session_id or path.stem.removeprefix("bwb-"),
        project_root=project_root,
        build_version=build_version,
        path=path,
        start_ts=start_ts,
        end_ts=end_ts,
        calls=calls,
    )


def default_log_dir() -> Path:
    xdg = os.environ.get("XDG_STATE_HOME")
    if xdg:
        return Path(xdg) / "bwb"
    return Path.home() / ".local" / "state" / "bwb"


def discover_sessions(log_dir: Path) -> list[Session]:
    if not log_dir.is_dir():
        print(f"error: log dir does not exist: {log_dir}", file=sys.stderr)
        return []
    sessions = []
    for path in sorted(log_dir.glob("bwb-*.log"), key=lambda p: p.stat().st_mtime):
        s = load_session(path)
        if s is not None:
            sessions.append(s)
    return sessions


def fmt_ms(values: list[int]) -> str:
    if not values:
        return "n/a"
    values = sorted(values)
    n = len(values)
    p50 = values[n // 2]
    p95 = values[min(int(n * 0.95), n - 1)]
    return (f"min={values[0]} p50={p50} p95={p95} max={values[-1]} "
            f"mean={statistics.mean(values):.0f} (n={n})")


def project_short(root: str) -> str:
    """Short project label for display: basename, or "(unknown)" if blank."""
    return Path(root).name or "(unknown)" if root else "(unknown)"


def print_session_summary(s: Session) -> None:
    proj = project_short(s.project_root)
    cold = s.cold_load_time_s()
    cold_str = f"{cold:.2f}s" if cold is not None else "n/a"
    hr = s.hit_rate
    hr_str = f"{hr:.1f}%" if hr is not None else "n/a"
    print(
        f"  {s.session_id[:12]:12s}  {s.start_ts.strftime('%Y-%m-%d %H:%M') if s.start_ts else '':16s}  "
        f"{proj:30.30s}  build={s.build_version:6.6s}  "
        f"dur={s.duration_s:>6.1f}s  cold={cold_str:>6s}  hit={hr_str:>5s}  "
        f"miss={len(s.misses):>3d}  hit={len(s.hits):>3d}"
    )


def print_session_detail(s: Session, *, limit_calls: int = 40) -> None:
    print(f"\nSession {s.session_id}")
    print(f"  project: {s.project_root}")
    print(f"  build:   {s.build_version}")
    print(f"  log:     {s.path}")
    print(f"  start:   {s.start_ts.isoformat() if s.start_ts else 'n/a'}")
    print(f"  end:     {s.end_ts.isoformat() if s.end_ts else 'n/a'}")
    print(f"  duration:{s.duration_s:.2f}s")
    print(f"  reads:   {len(s.calls)}  miss={len(s.misses)}  hit={len(s.hits)}  "
          f"hit_rate={s.hit_rate:.1f}%" if s.hit_rate is not None else "  reads: 0")
    cold = s.cold_load_time_s()
    if cold is not None:
        print(f"  cold-load wall time: {cold:.2f}s (initial miss burst)")
    print(f"  miss latency ms: {fmt_ms([c.duration_ms for c in s.misses])}")

    print(f"\n  Chronological bd activity (first {limit_calls}):")
    for c in s.calls[:limit_calls]:
        kind = "HIT " if c.cache == "hit" else "MISS"
        ms = f"{c.duration_ms:>5}ms" if c.cache == "miss" else "       "
        argv_short = " ".join(c.argv[1:5])
        print(f"    T+{c.t_offset_s:>6.2f}s  {kind}  {ms}  {c.op:18s}  {argv_short}")
    if len(s.calls) > limit_calls:
        print(f"    ... {len(s.calls) - limit_calls} more")


def print_project_list(sessions: list[Session]) -> None:
    by_proj: dict[str, list[Session]] = {}
    for s in sessions:
        by_proj.setdefault(s.project_root or "(unknown)", []).append(s)
    print("Projects with sessions in log dir:\n")
    print(f"  {'project':40s}  sessions  most-recent")
    for proj in sorted(by_proj):
        plist = by_proj[proj]
        latest = max(plist, key=lambda s: s.start_ts or datetime.min)
        ts = latest.start_ts.strftime("%Y-%m-%d %H:%M") if latest.start_ts else "?"
        print(f"  {project_short(proj):40s}  {len(plist):>8d}  {ts}")
    print(f"\nTotal: {len(sessions)} session(s) across {len(by_proj)} project(s).")
    print("Filter with --project <substring> or aggregate with --all.")


def print_aggregate(sessions: list[Session], label: str) -> None:
    if not sessions:
        print(f"No sessions matched '{label}'.")
        return
    print(f"\n=== Aggregate over {len(sessions)} session(s): {label} ===\n")
    print(f"  {'session':12s}  {'started':16s}  {'project':30s}  "
          f"{'build':6s}  {'dur':>7s}  {'cold':>6s}  {'hit%':>5s}  "
          f"{'miss':>4s}  {'hit':>4s}")
    for s in sessions:
        print_session_summary(s)

    all_miss_ms = [c.duration_ms for s in sessions for c in s.misses]
    total_miss = sum(len(s.misses) for s in sessions)
    total_hit = sum(len(s.hits) for s in sessions)
    total_reads = total_miss + total_hit
    cold_times = [c for s in sessions if (c := s.cold_load_time_s()) is not None]

    print(f"\n  Totals: reads={total_reads}  miss={total_miss}  hit={total_hit}", end="")
    if total_reads:
        print(f"  hit_rate={100*total_hit/total_reads:.1f}%")
    else:
        print()
    print(f"  Miss latency across all sessions: {fmt_ms(all_miss_ms)}")
    if cold_times:
        ct = sorted(cold_times)
        n = len(ct)
        print(f"  Cold-load wall time (s): min={ct[0]:.2f} p50={ct[n//2]:.2f} "
              f"p95={ct[min(int(n*0.95), n-1)]:.2f} max={ct[-1]:.2f} "
              f"mean={statistics.mean(ct):.2f} (n={n})")


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--project", help="filter to sessions whose project_root contains this substring")
    p.add_argument("--all", action="store_true", help="aggregate across every project")
    p.add_argument("--session", help="show a single-session deep-dive by session_id")
    p.add_argument("--log-dir", type=Path, default=default_log_dir(),
                   help=f"directory of bwb-*.log files (default: {default_log_dir()})")
    args = p.parse_args()

    sessions = discover_sessions(args.log_dir)
    if not sessions:
        print(f"no parseable sessions found in {args.log_dir}", file=sys.stderr)
        return 1

    if args.session:
        matches = [s for s in sessions if s.session_id.startswith(args.session)]
        if not matches:
            print(f"no session matches {args.session!r}", file=sys.stderr)
            return 1
        for s in matches:
            print_session_detail(s)
        return 0

    if args.project:
        matched = [s for s in sessions if args.project in s.project_root]
        print_aggregate(matched, f"--project {args.project!r}")
        return 0

    if args.all:
        print_aggregate(sessions, "--all projects")
        return 0

    print_project_list(sessions)
    return 0


if __name__ == "__main__":
    sys.exit(main())
