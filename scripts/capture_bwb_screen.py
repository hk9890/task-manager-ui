#!/usr/bin/env python3
import argparse
import json
import os
import pty
import select
import signal
import sys
import termios
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any

import pyte


ESC = "\x1b"
WAIT_POLL_SECONDS = 0.05


@dataclass
class CaptureResult:
    ok: bool
    command: list[str]
    screen: list[str]
    raw: bytes
    started_at: str
    ended_at: str
    steps: list[dict[str, Any]]
    checkpoints: list[dict[str, Any]]
    failure: dict[str, Any] | None
    cleanup: dict[str, Any]


@dataclass
class Step:
    raw: str
    kind: str
    value: str | None
    timeout_ms: int | None = None


def set_winsize(fd: int, rows: int, cols: int) -> None:
    import fcntl
    import struct

    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


def feed_keys(fd: int, keys: str) -> None:
    keymap = {
        "ENTER": "\r",
        "ESC": ESC,
        "TAB": "\t",
        "SPACE": " ",
        "BACKSPACE": "\x7f",
        "CTRL+C": "\x03",
        "CTRL+Q": "\x11",
        "CTRL+SPACE": "\x00",
        "PGDOWN": ESC + "[6~",
        "PGUP": ESC + "[5~",
        "HOME": ESC + "[H",
        "END": ESC + "[F",
        "LEFT": ESC + "[D",
        "RIGHT": ESC + "[C",
        "UP": ESC + "[A",
        "DOWN": ESC + "[B",
    }
    if keys not in keymap and len(keys) != 1:
        raise ValueError(
            f"unknown send-key name {keys!r}; pass a single character or one of {sorted(keymap)}"
        )
    data = keymap.get(keys, keys).encode()
    os.write(fd, data)


def answer_terminal_queries(fd: int, chunk: bytes) -> None:
    if b"\x1b[6n" in chunk:
        os.write(fd, b"\x1b[1;1R")
    if b"\x1b]11;?\x1b\\" in chunk or b"\x1b]11;?\a" in chunk:
        os.write(fd, b"\x1b]11;rgb:0000/0000/0000\x1b\\")
    if b"\x1b[c" in chunk:
        os.write(fd, b"\x1b[?64;1;2;6;9;15;18;21;22c")


def now_rfc3339() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def screen_excerpt(lines: list[str], max_lines: int = 30) -> str:
    trimmed = [line.rstrip() for line in lines]
    if len(trimmed) <= max_lines:
        return "\n".join(trimmed)
    return "\n".join(trimmed[-max_lines:])


def raw_excerpt(raw: bytes, max_bytes: int = 2048) -> str:
    if not raw:
        return ""
    return raw[-max_bytes:].decode("utf-8", errors="ignore")


def parse_step(step_text: str, default_timeout_ms: int) -> Step:
    raw = step_text.strip()
    if not raw:
        raise ValueError("empty step")

    if raw.startswith("send-key:"):
        return Step(raw=raw, kind="send-key", value=raw[len("send-key:") :])

    if raw.startswith("checkpoint:"):
        return Step(raw=raw, kind="checkpoint", value=raw[len("checkpoint:") :])

    if raw.startswith("sleep-ms:"):
        value = raw[len("sleep-ms:") :]
        if not value.isdigit():
            raise ValueError(f"invalid sleep-ms step {raw!r}; expected integer milliseconds")
        return Step(raw=raw, kind="sleep-ms", value=value)

    if raw.startswith("wait-for-text:"):
        body = raw[len("wait-for-text:") :]
        text, timeout_ms = parse_wait_body(raw, body, default_timeout_ms)
        return Step(raw=raw, kind="wait-for-text", value=text, timeout_ms=timeout_ms)

    if raw.startswith("wait-for-text-once:"):
        body = raw[len("wait-for-text-once:") :]
        text, timeout_ms = parse_wait_body(raw, body, default_timeout_ms)
        return Step(raw=raw, kind="wait-for-text-once", value=text, timeout_ms=timeout_ms)

    if raw.startswith("wait-for-no-text:"):
        body = raw[len("wait-for-no-text:") :]
        text, timeout_ms = parse_wait_body(raw, body, default_timeout_ms)
        return Step(raw=raw, kind="wait-for-no-text", value=text, timeout_ms=timeout_ms)

    raise ValueError(
        f"invalid step {raw!r}; expected send-key:, wait-for-text:, wait-for-text-once:, wait-for-no-text:, checkpoint:, or sleep-ms:"
    )


def parse_wait_body(raw_step: str, body: str, default_timeout_ms: int) -> tuple[str, int]:
    if not body:
        raise ValueError(f"invalid wait step {raw_step!r}; missing wait text")

    timeout_ms = default_timeout_ms
    text = body
    if ":" in body:
        maybe_text, maybe_timeout = body.rsplit(":", 1)
        if maybe_timeout.isdigit():
            text = maybe_text
            timeout_ms = int(maybe_timeout)

    if not text:
        raise ValueError(f"invalid wait step {raw_step!r}; empty wait text")
    if timeout_ms <= 0:
        raise ValueError(f"invalid wait step {raw_step!r}; timeout must be > 0")
    return text, timeout_ms


def parse_legacy_steps(raw_steps: str) -> list[Step]:
    steps: list[Step] = []
    if not raw_steps.strip():
        return steps
    for chunk in raw_steps.split(","):
        part = chunk.strip()
        if not part:
            continue
        if ":" not in part:
            raise ValueError(f"invalid step {part!r}; expected delay:key")
        delay_text, key = part.split(":", 1)
        delay = float(delay_text)
        if delay < 0:
            raise ValueError(f"invalid step {part!r}; delay must be >= 0")
        steps.append(Step(raw=f"sleep-ms:{int(delay * 1000)}", kind="sleep-ms", value=str(int(delay * 1000))))
        steps.append(Step(raw=f"send-key:{key}", kind="send-key", value=key))
    return steps


def process_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False


def wait_for_exit(pid: int, timeout_seconds: float) -> bool:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        try:
            waited_pid, _ = os.waitpid(pid, os.WNOHANG)
            if waited_pid == pid:
                return True
        except ChildProcessError:
            return True
        time.sleep(0.02)
    return False


def cleanup_child(pid: int, master_fd: int) -> dict[str, Any]:
    actions: list[str] = []
    forced = False

    if not process_alive(pid):
        try:
            os.waitpid(pid, 0)
        except ChildProcessError:
            pass
        return {"status": "ok", "actions": actions}

    try:
        feed_keys(master_fd, "CTRL+C")
        actions.append("ctrl+c")
    except OSError:
        pass

    if wait_for_exit(pid, 0.3):
        return {"status": "ok", "actions": actions}

    if process_alive(pid):
        try:
            os.kill(pid, signal.SIGTERM)
            actions.append("sigterm")
        except ProcessLookupError:
            pass

    if wait_for_exit(pid, 0.7):
        return {"status": "ok", "actions": actions}

    if process_alive(pid):
        forced = True
        try:
            os.kill(pid, signal.SIGKILL)
            actions.append("sigkill")
        except ProcessLookupError:
            pass
        wait_for_exit(pid, 0.5)

    return {"status": "forced" if forced else "ok", "actions": actions}


def capture(
    command: list[str],
    cwd: str,
    width: int,
    height: int,
    steps: list[Step],
    startup_wait: float,
    timeout: float,
) -> CaptureResult:
    screen = pyte.Screen(width, height)
    stream = pyte.Stream(screen)
    buffer = bytearray()
    started_at = now_rfc3339()
    step_results: list[dict[str, Any]] = []
    checkpoints: list[dict[str, Any]] = []
    failure: dict[str, Any] | None = None
    cleanup: dict[str, Any] = {"status": "ok", "actions": []}

    pid, master_fd = pty.fork()
    if pid == 0:
        os.chdir(cwd)
        env = os.environ.copy()
        env.setdefault("TERM", "xterm-256color")
        os.execvpe(command[0], command, env)

    set_winsize(master_fd, height, width)
    start = time.monotonic()
    timed_out = False

    def read_once(wait_seconds: float) -> None:
        rlist, _, _ = select.select([master_fd], [], [], wait_seconds)
        if not rlist:
            return
        try:
            chunk = os.read(master_fd, 65536)
        except OSError:
            return
        if not chunk:
            return
        buffer.extend(chunk)
        answer_terminal_queries(master_fd, chunk)
        stream.feed(chunk.decode("utf-8", errors="ignore"))

    def run_wait_step(step: Step, result: dict[str, Any], timeout_ms: int) -> None:
        nonlocal timed_out
        deadline = time.monotonic() + (timeout_ms / 1000.0)
        start_raw_len = len(buffer)
        while time.monotonic() < deadline:
            read_once(WAIT_POLL_SECONDS)
            visible = "\n".join(screen.display)
            found = step.value in visible
            if step.kind == "wait-for-text" and found:
                return
            if step.kind == "wait-for-text-once":
                observed = bytes(buffer[start_raw_len:])
                if step.value.encode() in observed:
                    return
            if step.kind == "wait-for-no-text" and not found:
                return

        timed_out = True
        result["status"] = "fail"
        result["error"] = {
            "code": "timeout",
            "message": f"step {result['index']} ({step.raw}) timed out after {timeout_ms}ms",
        }
        result["observed_excerpt"] = screen_excerpt(screen.display, max_lines=30)
        raise TimeoutError(result["error"]["message"])

    def latest_checkpoint_summary() -> dict[str, Any] | None:
        if not checkpoints:
            return None
        checkpoint = checkpoints[-1]
        return {
            "name": checkpoint.get("name"),
            "step_index": checkpoint.get("step_index"),
            "captured_at": checkpoint.get("captured_at"),
            "screen_excerpt": checkpoint.get("screen_excerpt"),
        }

    try:
        startup_deadline = start + startup_wait
        while time.monotonic() < startup_deadline:
            if time.monotonic() - start > timeout:
                raise TimeoutError(f"capture timed out during startup after {timeout}s")
            read_once(WAIT_POLL_SECONDS)

        for index, step in enumerate(steps):
            if time.monotonic() - start > timeout:
                raise TimeoutError(f"capture timed out after {timeout}s")

            result: dict[str, Any] = {
                "index": index,
                "raw": step.raw,
                "type": step.kind,
                "timeout_ms": step.timeout_ms,
                "status": "pass",
                "started_at": now_rfc3339(),
                "ended_at": None,
                "error": None,
                "observed_excerpt": screen_excerpt(screen.display, max_lines=30),
            }

            try:
                if step.kind == "send-key":
                    feed_keys(master_fd, step.value or "")
                    read_once(WAIT_POLL_SECONDS)
                    read_once(WAIT_POLL_SECONDS)
                elif step.kind == "sleep-ms":
                    duration = int(step.value or "0") / 1000.0
                    sleep_until = time.monotonic() + duration
                    while time.monotonic() < sleep_until:
                        read_once(min(WAIT_POLL_SECONDS, sleep_until - time.monotonic()))
                elif step.kind in {"wait-for-text", "wait-for-text-once", "wait-for-no-text"}:
                    run_wait_step(step, result, step.timeout_ms or 2000)
                elif step.kind == "checkpoint":
                    checkpoints.append(
                        {
                            "name": step.value,
                            "step_index": index,
                            "captured_at": now_rfc3339(),
                            "screen": list(screen.display),
                            "screen_excerpt": screen_excerpt(screen.display, max_lines=40),
                        }
                    )
                else:
                    raise RuntimeError(f"unsupported step type: {step.kind}")
            except Exception as exc:  # noqa: BLE001
                if result["error"] is None:
                    result["status"] = "fail"
                    result["error"] = {"code": "runtime", "message": str(exc)}
                    result["observed_excerpt"] = screen_excerpt(screen.display, max_lines=30)
                failure = {
                    "step_index": index,
                    "step_raw": step.raw,
                    "error": result["error"],
                    "timed_out": timed_out,
                    "observed_excerpt": result.get("observed_excerpt"),
                    "latest_checkpoint": latest_checkpoint_summary(),
                    "raw_tail": raw_excerpt(bytes(buffer), max_bytes=2048),
                }
                result["ended_at"] = now_rfc3339()
                step_results.append(result)
                break

            result["ended_at"] = now_rfc3339()
            result["observed_excerpt"] = screen_excerpt(screen.display, max_lines=30)
            step_results.append(result)

        if failure is not None and len(step_results) < len(steps):
            for idx in range(len(step_results), len(steps)):
                skipped_step = steps[idx]
                step_results.append(
                    {
                        "index": idx,
                        "raw": skipped_step.raw,
                        "type": skipped_step.kind,
                        "timeout_ms": skipped_step.timeout_ms,
                        "status": "skipped",
                        "started_at": None,
                        "ended_at": None,
                        "error": None,
                        "observed_excerpt": screen_excerpt(screen.display, max_lines=30),
                    }
                )

        read_once(WAIT_POLL_SECONDS)
    finally:
        cleanup = cleanup_child(pid, master_fd)
        os.close(master_fd)

    ended_at = now_rfc3339()
    return CaptureResult(
        ok=failure is None,
        command=command,
        screen=list(screen.display),
        raw=bytes(buffer),
        started_at=started_at,
        ended_at=ended_at,
        steps=step_results,
        checkpoints=checkpoints,
        failure=failure,
        cleanup=cleanup,
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="Capture the visible BWB alt-screen state via a PTY.")
    parser.add_argument("--cwd", required=True)
    parser.add_argument("--width", type=int, default=120)
    parser.add_argument("--height", type=int, default=34)
    parser.add_argument("--startup-wait", type=float, default=1.0)
    parser.add_argument("--timeout", type=float, default=10.0)
    parser.add_argument("--step", action="append", default=[], help="Step instruction (repeatable)")
    parser.add_argument(
        "--steps",
        default="",
        help="Legacy delay:key CSV syntax. Converted to sleep-ms + send-key steps.",
    )
    parser.add_argument("--default-timeout-ms", type=int, default=2000)
    parser.add_argument("--dump-raw", action="store_true")
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()

    command = args.command
    while command and command[0] == "--":
        command = command[1:]
    if not command:
        parser.error("missing command after --")

    parsed_steps: list[Step] = []
    try:
        parsed_steps.extend(parse_step(raw, args.default_timeout_ms) for raw in args.step)
        if args.steps:
            parsed_steps.extend(parse_legacy_steps(args.steps))
    except ValueError as exc:
        parser.error(str(exc))

    result = capture(
        command=command,
        cwd=args.cwd,
        width=args.width,
        height=args.height,
        steps=parsed_steps,
        startup_wait=args.startup_wait,
        timeout=args.timeout,
    )

    payload = {
        "ok": result.ok,
        "command": result.command,
        "started_at": result.started_at,
        "ended_at": result.ended_at,
        "width": args.width,
        "height": args.height,
        "screen": result.screen,
        "steps": result.steps,
        "checkpoints": result.checkpoints,
        "failure": result.failure,
        "cleanup": result.cleanup,
    }
    if args.dump_raw:
        payload["raw"] = result.raw.decode("utf-8", errors="ignore")
    json.dump(payload, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0 if result.ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
