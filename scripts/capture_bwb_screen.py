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

import pyte


ESC = "\x1b"


@dataclass
class CaptureResult:
    screen: list[str]
    raw: bytes


def set_winsize(fd: int, rows: int, cols: int) -> None:
    import fcntl
    import struct

    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


def feed_keys(fd: int, keys: str) -> None:
    keymap = {
        "ENTER": "\r",
        "ESC": ESC,
        "TAB": "\t",
        "CTRL+Q": "\x11",
        "PGDOWN": ESC + "[6~",
        "PGUP": ESC + "[5~",
        "HOME": ESC + "[H",
        "END": ESC + "[F",
        "LEFT": ESC + "[D",
        "RIGHT": ESC + "[C",
        "UP": ESC + "[A",
        "DOWN": ESC + "[B",
    }
    data = keymap.get(keys, keys).encode()
    os.write(fd, data)


def answer_terminal_queries(fd: int, chunk: bytes) -> None:
    if b"\x1b[6n" in chunk:
        os.write(fd, b"\x1b[1;1R")
    if b"\x1b]11;?\x1b\\" in chunk or b"\x1b]11;?\a" in chunk:
        os.write(fd, b"\x1b]11;rgb:0000/0000/0000\x1b\\")
    if b"\x1b[c" in chunk:
        os.write(fd, b"\x1b[?64;1;2;6;9;15;18;21;22c")


def parse_steps(raw_steps: str) -> list[tuple[float, str]]:
    steps: list[tuple[float, str]] = []
    if not raw_steps.strip():
        return steps
    for chunk in raw_steps.split(","):
        part = chunk.strip()
        if not part:
            continue
        if ":" not in part:
            raise ValueError(f"invalid step {part!r}; expected delay:key")
        delay_text, key = part.split(":", 1)
        steps.append((float(delay_text), key))
    return steps


def capture(command: list[str], cwd: str, width: int, height: int, steps: list[tuple[float, str]], startup_wait: float, settle_wait: float, timeout: float) -> CaptureResult:
    screen = pyte.Screen(width, height)
    stream = pyte.Stream(screen)
    buffer = bytearray()

    pid, master_fd = pty.fork()
    if pid == 0:
        os.chdir(cwd)
        env = os.environ.copy()
        env.setdefault("TERM", "xterm-256color")
        os.execvpe(command[0], command, env)

    set_winsize(master_fd, height, width)
    start = time.monotonic()
    next_step = 0
    last_feed = start
    startup_done = False

    try:
        while True:
            now = time.monotonic()
            if now - start > timeout:
                raise TimeoutError(f"capture timed out after {timeout}s")

            if not startup_done and now - start >= startup_wait:
                startup_done = True

            while startup_done and next_step < len(steps) and now - last_feed >= steps[next_step][0]:
                _, key = steps[next_step]
                feed_keys(master_fd, key)
                last_feed = time.monotonic()
                next_step += 1

            wait_budget = settle_wait if next_step >= len(steps) else 0.05
            rlist, _, _ = select.select([master_fd], [], [], wait_budget)
            if rlist:
                try:
                    chunk = os.read(master_fd, 65536)
                except OSError:
                    break
                if not chunk:
                    break
                buffer.extend(chunk)
                answer_terminal_queries(master_fd, chunk)
                stream.feed(chunk.decode("utf-8", errors="ignore"))
                continue

            if startup_done and next_step >= len(steps):
                break
    finally:
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            os.waitpid(pid, 0)
        except ChildProcessError:
            pass
        os.close(master_fd)

    return CaptureResult(screen=list(screen.display), raw=bytes(buffer))


def main() -> int:
    parser = argparse.ArgumentParser(description="Capture the visible BWB alt-screen state via a PTY.")
    parser.add_argument("--cwd", required=True)
    parser.add_argument("--width", type=int, default=120)
    parser.add_argument("--height", type=int, default=34)
    parser.add_argument("--startup-wait", type=float, default=1.0)
    parser.add_argument("--settle-wait", type=float, default=0.25)
    parser.add_argument("--timeout", type=float, default=10.0)
    parser.add_argument("--steps", default="")
    parser.add_argument("--dump-raw", action="store_true")
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()

    command = args.command
    while command and command[0] == "--":
        command = command[1:]
    if not command:
        parser.error("missing command after --")

    result = capture(
        command=command,
        cwd=args.cwd,
        width=args.width,
        height=args.height,
        steps=parse_steps(args.steps),
        startup_wait=args.startup_wait,
        settle_wait=args.settle_wait,
        timeout=args.timeout,
    )

    payload = {
        "width": args.width,
        "height": args.height,
        "screen": result.screen,
    }
    if args.dump_raw:
        payload["raw"] = result.raw.decode("utf-8", errors="ignore")
    json.dump(payload, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
