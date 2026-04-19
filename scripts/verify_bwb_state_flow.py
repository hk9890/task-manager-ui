#!/usr/bin/env python3
import argparse
import json
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any


@dataclass(frozen=True)
class FlowSpec:
    name: str
    expect_change: bool
    steps: list[str]
    expect_fields: dict[str, dict[str, Any]]


FLOW_SPECS: dict[str, FlowSpec] = {
    "read-only": FlowSpec(
        name="read-only",
        expect_change=False,
        steps=[
            "wait-for-text:Ready:3000",
            "wait-for-text:Selected: bwf-2:3000",
            "send-key:ENTER",
            "wait-for-text:Detail::3000",
            "checkpoint:details-open",
            "send-key:ESC",
            "wait-for-text:Board:1500",
            "send-key:CTRL+Q",
        ],
        expect_fields={
            "status": {"unchanged": True},
            "closed_at": {"unchanged": True},
        },
    ),
    "mutation-save": FlowSpec(
        name="mutation-save",
        expect_change=True,
        steps=[
            "wait-for-text:Ready:3000",
            "wait-for-text:Selected::3000",
            "send-key:l",
            "wait-for-text:Selected: bwf-1:3000",
            "send-key:ENTER",
            "wait-for-text:Detail::2000",
            "send-key:x",
            "wait-for-text:Provide an optional close reason.:2000",
            "send-key:ENTER",
            "send-key:ENTER",
            "wait-for-no-text:Provide an optional close reason.:3000",
            "wait-for-text:Closed issue :3000",
            "wait-for-text:Board:2000",
            "checkpoint:mutation-save-done",
            "send-key:CTRL+Q",
        ],
        expect_fields={
            "status": {"equals": "closed", "changed": True},
            "closed_at": {"changed": True},
        },
    ),
    "mutation-cancel": FlowSpec(
        name="mutation-cancel",
        expect_change=False,
        steps=[
            "wait-for-text:Ready:3000",
            "wait-for-text:Selected::3000",
            "send-key:l",
            "wait-for-text:Selected: bwf-1:3000",
            "send-key:ENTER",
            "wait-for-text:Detail::2000",
            "send-key:x",
            "wait-for-text:Provide an optional close reason.:2000",
            "send-key:ESC",
            "wait-for-no-text:Provide an optional close reason.:2000",
            "wait-for-text:Detail::2000",
            "checkpoint:mutation-cancel-done",
            "send-key:CTRL+Q",
        ],
        expect_fields={
            "status": {"equals": "open", "unchanged": True},
            "closed_at": {"unchanged": True},
        },
    ),
    "mutation-save-intentional-fail": FlowSpec(
        name="mutation-save-intentional-fail",
        expect_change=True,
        steps=[
            "wait-for-text:Ready:3000",
            "wait-for-text:Selected::3000",
            "send-key:l",
            "wait-for-text:Selected: bwf-1:3000",
            "send-key:ENTER",
            "wait-for-text:Detail::2000",
            "send-key:x",
            "wait-for-text:Provide an optional close reason.:2000",
            "send-key:ENTER",
            "send-key:ENTER",
            "wait-for-no-text:Provide an optional close reason.:3000",
            "wait-for-text:Closed issue :3000",
            "checkpoint:mutation-save-done",
            "wait-for-text:Board:2000",
            "send-key:CTRL+Q",
        ],
        expect_fields={
            "status": {"equals": "open", "changed": True},
            "closed_at": {"changed": True},
        },
    ),
}


def run_json(command: list[str], cwd: str | None = None) -> dict[str, Any]:
    proc = subprocess.run(command, text=True, capture_output=True, check=False, cwd=cwd)
    if proc.returncode != 0:
        raise RuntimeError(
            f"command failed ({proc.returncode}): {' '.join(command)}\n"
            f"stdout:\n{proc.stdout}\n"
            f"stderr:\n{proc.stderr}"
        )
    try:
        return json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"command did not return valid JSON: {' '.join(command)}") from exc


def shallow_diff(before: dict[str, Any], after: dict[str, Any]) -> list[str]:
    keys = sorted(set(before.keys()) | set(after.keys()))
    changed: list[str] = []
    for key in keys:
        if before.get(key) != after.get(key):
            changed.append(key)
    return changed


def normalize_issue_payload(payload: Any) -> dict[str, Any]:
    if isinstance(payload, dict):
        return payload
    if isinstance(payload, list) and len(payload) == 1 and isinstance(payload[0], dict):
        return payload[0]
    raise RuntimeError("expected bd show --json to return an object or single-item list")


def check_field_expectations(
    before: dict[str, Any],
    after: dict[str, Any],
    expectations: dict[str, dict[str, Any]],
) -> dict[str, Any]:
    checks: list[dict[str, Any]] = []
    ok = True

    for field, expectation in expectations.items():
        before_value = before.get(field)
        after_value = after.get(field)
        field_changed = before_value != after_value
        field_ok = True
        problems: list[str] = []

        if expectation.get("changed") is True and not field_changed:
            field_ok = False
            problems.append("expected field to change")
        if expectation.get("unchanged") is True and field_changed:
            field_ok = False
            problems.append("expected field to stay unchanged")
        if "equals" in expectation and after_value != expectation["equals"]:
            field_ok = False
            problems.append(f"expected after value {expectation['equals']!r}")

        checks.append(
            {
                "field": field,
                "ok": field_ok,
                "before": before_value,
                "after": after_value,
                "changed": field_changed,
                "expectation": expectation,
                "problems": problems,
            }
        )
        ok = ok and field_ok

    return {"ok": ok, "checks": checks}


def build_capture_command(args: argparse.Namespace, steps: list[str]) -> list[str]:
    command: list[str] = [
        sys.executable,
        str(Path(args.capture_script).resolve()),
        "--cwd",
        args.cwd,
        "--width",
        str(args.width),
        "--height",
        str(args.height),
        "--startup-wait",
        str(args.startup_wait),
        "--timeout",
        str(args.timeout),
    ]
    all_steps = list(args.pre_step) + list(steps)
    for step in all_steps:
        command.extend(["--step", step])
    command.append("--")
    command.extend(args.app_command)
    return command


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Capture before/after bd show JSON around a PTY verification flow."
    )
    parser.add_argument("--issue", required=True, help="Issue id for bd show --json")
    parser.add_argument("--cwd", required=True, help="Beads project directory")
    parser.add_argument(
        "--flow",
        choices=sorted(FLOW_SPECS.keys()),
        required=True,
        help="Representative flow preset",
    )
    parser.add_argument(
        "--capture-script",
        default="scripts/capture_bwb_screen.py",
        help="Path to capture script",
    )
    parser.add_argument("--width", type=int, default=120)
    parser.add_argument("--height", type=int, default=34)
    parser.add_argument("--startup-wait", type=float, default=1.2)
    parser.add_argument("--timeout", type=float, default=12.0)
    parser.add_argument(
        "--pre-step",
        action="append",
        default=[],
        help="Optional setup steps inserted before the selected flow",
    )
    parser.add_argument(
        "--app-command",
        nargs=argparse.REMAINDER,
        required=True,
        help="Command after --app-command, e.g. --app-command env BD_NON_INTERACTIVE=1 /tmp/bwb",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    app_command = list(args.app_command)
    while app_command and app_command[0] == "--":
        app_command = app_command[1:]
    if not app_command:
        raise SystemExit("--app-command requires a command after --")
    args.app_command = app_command

    flow = FLOW_SPECS[args.flow]
    before = normalize_issue_payload(run_json(["bd", "show", args.issue, "--json"], cwd=args.cwd))
    capture_payload = run_json(build_capture_command(args, flow.steps))
    after = normalize_issue_payload(run_json(["bd", "show", args.issue, "--json"], cwd=args.cwd))

    changed_fields = shallow_diff(before, after)
    changed = bool(changed_fields)
    expectation_met = changed if flow.expect_change else not changed
    field_expectation_result = check_field_expectations(before, after, flow.expect_fields)
    field_expectation_met = bool(field_expectation_result["ok"])

    payload = {
        "ok": bool(capture_payload.get("ok")) and expectation_met and field_expectation_met,
        "flow": flow.name,
        "issue": args.issue,
        "expect_change": flow.expect_change,
        "changed": changed,
        "changed_fields": changed_fields,
        "expect_fields": flow.expect_fields,
        "field_expectations_ok": field_expectation_met,
        "field_expectations": field_expectation_result["checks"],
        "capture_ok": bool(capture_payload.get("ok")),
        "capture_failure": capture_payload.get("failure"),
        "capture_cleanup": capture_payload.get("cleanup"),
        "before": before,
        "after": after,
    }
    json.dump(payload, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0 if payload["ok"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
