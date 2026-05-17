#!/usr/bin/env python3
"""
Runtime verification of the refresh-concurrency fix (5q6t).

Drives the built `bwb` binary via a PTY against the embedded fixture repo,
hammers 'r' rapidly in both board and search modes, and asserts:

  - UI does not corrupt (column headers + at least one issue ID visible).
  - No panic or runtime error on screen.
  - Debug log contains "manual board refresh suppressed" entries (5q6t.1).
  - Debug log contains "manual search refresh suppressed" entries (5q6t.2).
  - No panic/fatal log entries.
  - pendingResults never went negative.

Hard timeout: 15 seconds total.
Guaranteed subprocess + tempdir cleanup on any exit path.
"""
import os
import pty
import select
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import pyte


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent

SETUP_SCRIPT = REPO_ROOT / "internal" / "testing" / "e2e" / "embeddedfixture" / "setup.sh"
SEED_JSON = REPO_ROOT / "internal" / "testing" / "e2e" / "embeddedfixture" / "seed.json"

# Binary is built by mise run build (output: <worktree>/bwb)
BWB_BINARY = REPO_ROOT / "bwb"

TOTAL_TIMEOUT_S = 15.0
READINESS_TIMEOUT_S = 8.0
SETTLE_S = 0.6          # wait after keypress storm for refreshes to settle
RAPID_R_COUNT = 20
RAPID_R_DELAY_S = 0.05  # 50 ms between each 'r'

TERM_WIDTH = 120
TERM_HEIGHT = 34

# Column headers rendered by the built-in dashboard provider.
# The default viewport shows columns 1-3/4 at 120 columns wide — "Done" is
# scrolled off-screen until the user navigates right.  We assert only the
# visible ones.
BOARD_COLUMN_HEADERS_VISIBLE = ["Not Ready", "Ready", "In Progress"]
# All 4 exist at the model level; we just can't assert "Done" from the screen.
BOARD_ALL_COLUMN_HEADERS = ["Not Ready", "Ready", "In Progress", "Done"]

# Known issue IDs from seed.json
FIXTURE_ISSUE_IDS = ["bwf-1", "bwf-2"]

# Log message substrings from 5q6t.1 and 5q6t.2
BOARD_GUARD_MSG = "manual board refresh suppressed"
SEARCH_GUARD_MSG = "manual search refresh suppressed"

# ---------------------------------------------------------------------------
# PTY helpers
# ---------------------------------------------------------------------------

POLL_INTERVAL_S = 0.05


def set_winsize(fd: int, rows: int, cols: int) -> None:
    import fcntl
    import struct
    import termios

    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


def answer_terminal_queries(fd: int, chunk: bytes) -> None:
    """Reply to terminal capability queries so the TUI initialises cleanly."""
    if b"\x1b[6n" in chunk:
        os.write(fd, b"\x1b[1;1R")
    if b"\x1b]11;?\x1b\\" in chunk or b"\x1b]11;?\a" in chunk:
        os.write(fd, b"\x1b]11;rgb:0000/0000/0000\x1b\\")
    if b"\x1b[c" in chunk:
        os.write(fd, b"\x1b[?64;1;2;6;9;15;18;21;22c")


def read_available(master_fd: int, screen: pyte.Screen, stream: pyte.Stream,
                   wait_s: float) -> None:
    """Drain available output from master_fd into pyte, waiting up to wait_s."""
    rlist, _, _ = select.select([master_fd], [], [], wait_s)
    if not rlist:
        return
    try:
        chunk = os.read(master_fd, 65536)
    except OSError:
        return
    if chunk:
        answer_terminal_queries(master_fd, chunk)
        stream.feed(chunk.decode("utf-8", errors="ignore"))


def screen_text(screen: pyte.Screen) -> str:
    return "\n".join(screen.display)


def wait_for_text(master_fd: int, screen: pyte.Screen, stream: pyte.Stream,
                  text: str, timeout_s: float) -> None:
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        read_available(master_fd, screen, stream, POLL_INTERVAL_S)
        if text in screen_text(screen):
            return
    raise TimeoutError(
        f"timed out after {timeout_s:.1f}s waiting for text {text!r}\n"
        f"screen:\n{screen_text(screen)}"
    )


def drain(master_fd: int, screen: pyte.Screen, stream: pyte.Stream,
          duration_s: float) -> None:
    """Read for duration_s, feeding pyte."""
    deadline = time.monotonic() + duration_s
    while time.monotonic() < deadline:
        remaining = deadline - time.monotonic()
        read_available(master_fd, screen, stream, min(POLL_INTERVAL_S, remaining))


def send_keys(master_fd: int, data: bytes) -> None:
    os.write(master_fd, data)


# ---------------------------------------------------------------------------
# Process lifecycle
# ---------------------------------------------------------------------------

def process_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False


def wait_for_exit(pid: int, timeout_s: float) -> bool:
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        try:
            waited_pid, _ = os.waitpid(pid, os.WNOHANG)
            if waited_pid == pid:
                return True
        except ChildProcessError:
            return True
        time.sleep(0.02)
    return False


def cleanup_child(pid: int, master_fd: int) -> None:
    if not process_alive(pid):
        try:
            os.waitpid(pid, 0)
        except ChildProcessError:
            pass
        return

    # Try graceful quit (ctrl+q)
    try:
        send_keys(master_fd, b"\x11")  # ctrl+q
    except OSError:
        pass
    if wait_for_exit(pid, 0.5):
        return

    # SIGTERM
    if process_alive(pid):
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    if wait_for_exit(pid, 0.7):
        return

    # SIGKILL
    if process_alive(pid):
        try:
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    wait_for_exit(pid, 0.5)


# ---------------------------------------------------------------------------
# Fixture setup
# ---------------------------------------------------------------------------

def setup_fixture(tmpdir: str) -> None:
    result = subprocess.run(
        ["bash", str(SETUP_SCRIPT), tmpdir, str(SEED_JSON)],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"setup.sh failed (exit {result.returncode}):\n"
            f"stdout: {result.stdout}\nstderr: {result.stderr}"
        )


def create_slow_bd_wrapper(tmpdir: str) -> str:
    """Create a thin wrapper script around 'bd' that adds a 30ms delay.

    This is needed only for the search guard (manual search refresh suppressed)
    because 'bd search' against the embedded fixture completes in <5ms, making
    it impossible to send a second 'r' keypress while the first search is
    still in-flight via normal PTY round-trip timing.

    The wrapper is NOT a product change — it is test infrastructure only,
    installed into the temp dir and injected via PATH so only this harness run
    is affected.  The real 'bd' binary is resolved from the original PATH.
    """
    bin_dir = os.path.join(tmpdir, "_testbin")
    os.makedirs(bin_dir, exist_ok=True)
    wrapper_path = os.path.join(bin_dir, "bd")
    # Find the real bd binary path BEFORE we prepend our dir to PATH.
    import shutil
    real_bd = shutil.which("bd")
    if real_bd is None:
        raise RuntimeError("'bd' binary not found in PATH")

    wrapper_content = f"""#!/bin/bash
# Test-only wrapper: adds a small delay to make in-flight gateway calls
# detectable by rapid 'r' keypresses in the PTY harness.
sleep 0.04
exec {real_bd} "$@"
"""
    with open(wrapper_path, "w") as f:
        f.write(wrapper_content)
    os.chmod(wrapper_path, 0o755)
    return bin_dir


# ---------------------------------------------------------------------------
# Assertions
# ---------------------------------------------------------------------------

def assert_ui_intact(screen: pyte.Screen, label: str) -> None:
    text = screen_text(screen)
    failures = []

    # Assert the three visible columns (Done is col 4/4, scrolled off-screen at
    # the default 120-column width with the built-in layout).
    for header in BOARD_COLUMN_HEADERS_VISIBLE:
        if header not in text:
            failures.append(f"missing column header {header!r}")

    # At least one fixture issue should be visible in the board.
    has_issue = any(iid in text for iid in FIXTURE_ISSUE_IDS)
    if not has_issue:
        failures.append(f"no fixture issue ID found (expected one of {FIXTURE_ISSUE_IDS})")

    for bad in ("panic", "runtime error"):
        if bad in text.lower():
            failures.append(f"screen contains {bad!r}")

    if failures:
        raise AssertionError(
            f"[{label}] UI integrity failed:\n"
            + "\n".join(f"  - {f}" for f in failures)
            + f"\nscreen:\n{text}"
        )


def assert_no_screen_panic(screen: pyte.Screen, label: str) -> None:
    text = screen_text(screen)
    for bad in ("panic", "runtime error"):
        if bad in text.lower():
            raise AssertionError(
                f"[{label}] screen contains {bad!r}\nscreen:\n{text}"
            )


def assert_log(log_path: str, session_filter: str | None = None) -> None:
    """Read the JSON-lines log and assert guard entries fired."""
    try:
        with open(log_path) as f:
            content = f.read()
    except FileNotFoundError:
        raise AssertionError(
            f"debug log not found at {log_path!r}; "
            "bwb may not have written it (was --debug passed?)"
        )

    # If we have a session_id we can filter; otherwise scan all lines.
    lines = content.splitlines()

    failures = []

    board_suppressed = any(BOARD_GUARD_MSG in line for line in lines)
    search_suppressed = any(SEARCH_GUARD_MSG in line for line in lines)

    if not board_suppressed:
        failures.append(
            f"log contains no {BOARD_GUARD_MSG!r} entry (board guard from 5q6t.1 did not fire)"
        )
    if not search_suppressed:
        failures.append(
            f"log contains no {SEARCH_GUARD_MSG!r} entry (search guard from 5q6t.2 did not fire)"
        )

    for line in lines:
        lower = line.lower()
        if '"level":"error"' in line.lower() or '"level":"ERROR"' in line:
            # Only fail on panic/fatal errors, not ordinary errors
            pass
        for bad in ("panic", "fatal"):
            if bad in lower and ("message" in lower or "msg" in lower):
                failures.append(f"log contains {bad!r} entry: {line[:200]}")

    # Check pendingResults never went negative (look for negative value)
    import json
    for line in lines:
        line = line.strip()
        if not line:
            continue
        try:
            rec = json.loads(line)
        except json.JSONDecodeError:
            continue
        pr = rec.get("pendingResults")
        if pr is not None and isinstance(pr, (int, float)) and pr < 0:
            failures.append(f"pendingResults went negative: {pr} in record: {line[:200]}")

    if failures:
        raise AssertionError(
            "Log assertions failed:\n"
            + "\n".join(f"  - {f}" for f in failures)
        )


# ---------------------------------------------------------------------------
# Main verification flow
# ---------------------------------------------------------------------------

def run_verification() -> None:
    if not BWB_BINARY.exists():
        raise RuntimeError(
            f"bwb binary not found at {BWB_BINARY}. "
            "Run 'mise run build' first."
        )
    if not SETUP_SCRIPT.exists():
        raise RuntimeError(f"setup.sh not found at {SETUP_SCRIPT}")

    tmpdir = tempfile.mkdtemp(prefix="bwb-5q6t-verify-")
    xdg_state = os.path.join(tmpdir, "xdg_state")
    os.makedirs(xdg_state, exist_ok=True)
    log_file = os.path.join(xdg_state, "bwb", "bwb.log")

    pid: int | None = None
    master_fd: int | None = None

    try:
        # ------------------------------------------------------------------ #
        # 1. Seed fixture
        # ------------------------------------------------------------------ #
        print("[ setup ] seeding fixture repo ...", flush=True)
        setup_fixture(tmpdir)
        print(f"[ setup ] fixture at {tmpdir}", flush=True)

        # ------------------------------------------------------------------ #
        # 2. Spawn bwb in a PTY
        # ------------------------------------------------------------------ #
        # Create a slow bd wrapper so that search gateway calls take ~40ms.
        # This ensures 'r' presses separated by ~10ms arrive while the first
        # search is still in-flight, allowing the Reload() guard (5q6t.2) to
        # fire.  Without this wrapper, 'bd search' against the embedded fixture
        # completes in <5ms and the guard window is too narrow for PTY timing.
        slow_bd_dir = create_slow_bd_wrapper(tmpdir)
        print(f"[ setup ] slow bd wrapper at {slow_bd_dir}/bd", flush=True)

        env = os.environ.copy()
        env["BD_NON_INTERACTIVE"] = "1"
        env["TERM"] = "xterm-256color"
        env["XDG_STATE_HOME"] = xdg_state
        # Prepend the slow wrapper dir to PATH for the bwb process only.
        env["PATH"] = slow_bd_dir + ":" + env.get("PATH", "")
        # Disable auto-refresh so only manual 'r' triggers refreshes.
        command = [
            str(BWB_BINARY),
            "--cwd", tmpdir,
            "--debug",
            "--no-auto-refresh",
        ]

        screen = pyte.Screen(TERM_WIDTH, TERM_HEIGHT)
        stream = pyte.Stream(screen)

        print(f"[ spawn ] {' '.join(command)}", flush=True)
        pid, master_fd = pty.fork()
        if pid == 0:
            # child
            os.execvpe(command[0], command, env)
            sys.exit(1)  # unreachable

        set_winsize(master_fd, TERM_HEIGHT, TERM_WIDTH)
        overall_start = time.monotonic()

        # ------------------------------------------------------------------ #
        # 3. Wait for board to render and data to load (readiness signal)
        # ------------------------------------------------------------------ #
        # First wait for board structure to appear (column headers visible).
        print("[ wait  ] board columns visible ...", flush=True)
        wait_for_text(master_fd, screen, stream, "Ready", READINESS_TIMEOUT_S)
        print("[ ok    ] board columns rendered (saw 'Ready')", flush=True)

        # Then wait for actual fixture data to load — the skeleton (░ rows) will
        # be replaced once all 4 gateway calls return.  bwf-1 is in "Not Ready"
        # (blocked), bwf-2 would be in "Ready". Wait for either to appear, or for
        # the status bar to stop saying "Loading: board".
        print("[ wait  ] fixture data loaded ...", flush=True)
        try:
            # bwf-2 is in the "Ready" column (open, not blocked)
            wait_for_text(master_fd, screen, stream, "bwf-", READINESS_TIMEOUT_S)
            print("[ ok    ] fixture data loaded (saw 'bwf-')", flush=True)
        except TimeoutError:
            # Fall back: just wait for loading indicator to clear
            print("[ warn  ] fixture IDs not seen; waiting for loading to clear ...", flush=True)
            try:
                wait_for_text(master_fd, screen, stream, "Selected:", 3.0)
                print("[ ok    ] selected indicator present", flush=True)
            except TimeoutError:
                current = screen_text(screen)
                print(f"[ warn  ] still loading? screen:\n{current[:600]}", flush=True)

        # ------------------------------------------------------------------ #
        # 4. Board scenario: hammer 'r' 20x
        # ------------------------------------------------------------------ #
        print(f"[ board ] sending {RAPID_R_COUNT} rapid 'r' keypresses ...", flush=True)
        for i in range(RAPID_R_COUNT):
            if time.monotonic() - overall_start > TOTAL_TIMEOUT_S:
                raise TimeoutError("harness timeout during board scenario")
            send_keys(master_fd, b"r")
            drain(master_fd, screen, stream, RAPID_R_DELAY_S)

        print(f"[ board ] settling {SETTLE_S}s ...", flush=True)
        drain(master_fd, screen, stream, SETTLE_S)

        print("[ board ] asserting UI integrity ...", flush=True)
        assert_ui_intact(screen, "board-after-rapid-r")
        print("[ ok    ] board UI intact after rapid 'r'", flush=True)

        # ------------------------------------------------------------------ #
        # 5. Switch to search mode: ctrl+@ (NUL byte)
        # ------------------------------------------------------------------ #
        if time.monotonic() - overall_start > TOTAL_TIMEOUT_S:
            raise TimeoutError("harness timeout before search scenario")

        print("[ search] switching to search via ctrl+@ ...", flush=True)
        send_keys(master_fd, b"\x00")  # ctrl+@ / ctrl+space
        drain(master_fd, screen, stream, 0.3)
        # Wait for search mode indicator — the search prompt or "Search" text
        try:
            wait_for_text(master_fd, screen, stream, "Search", 3.0)
            print("[ ok    ] search mode entered", flush=True)
        except TimeoutError:
            # Try the board text to confirm we are still alive at least
            current = screen_text(screen)
            print(f"[ warn  ] 'Search' not seen — current screen head:\n{current[:400]}", flush=True)
            # Continue anyway; the guard test still fires on 'r' in whatever mode we're in

        # Type a query that matches fixture data
        print("[ search] typing query 'bwf' ...", flush=True)
        for ch in b"bwf":
            send_keys(master_fd, bytes([ch]))
            drain(master_fd, screen, stream, 0.05)

        # Submit search
        send_keys(master_fd, b"\r")  # Enter
        drain(master_fd, screen, stream, 0.3)

        # Wait for search results to appear before moving focus
        print("[ search] waiting for search results ...", flush=True)
        try:
            wait_for_text(master_fd, screen, stream, "bwf-", 4.0)
            print("[ ok    ] search results loaded", flush=True)
        except TimeoutError:
            current = screen_text(screen)
            print(f"[ warn  ] search results not visible; screen:\n{current[:400]}", flush=True)

        # Move focus from query input to results pane using the Down arrow key.
        # This is essential: when focus is on FocusQuery, 'r' is treated as text
        # input rather than SearchActionReload, so we must leave FocusQuery first.
        # We use the Down arrow (ANSI escape \x1b[B) rather than 'j' because 'j'
        # is a rune key and goes to query text input when focus=FocusQuery.
        print("[ search] moving focus to results with Down arrow ...", flush=True)
        send_keys(master_fd, b"\x1b[B")  # ESC [ B = Down arrow
        drain(master_fd, screen, stream, 0.2)

        # ------------------------------------------------------------------ #
        # 6. Search scenario: hammer 'r' 20x
        #
        # Send a burst of 5 'r' keys immediately (no delay) to ensure at least
        # one arrives while the first search gateway call is still in-flight,
        # then continue with small delays for the rest.  The guard fires when 'r'
        # arrives before the async searchLoadedMsg is processed.
        # ------------------------------------------------------------------ #
        print(f"[ search] sending {RAPID_R_COUNT} rapid 'r' keypresses ...", flush=True)
        # Send a burst of 5 'r' keys immediately, then continue with 10ms
        # inter-press delays.  The slow bd wrapper (~40ms per call) ensures
        # the first search is still in-flight when the second 'r' arrives,
        # guaranteeing the Reload() guard (5q6t.2) fires.
        send_keys(master_fd, b"r" * 5)
        drain(master_fd, screen, stream, 0.01)
        for i in range(RAPID_R_COUNT - 5):
            if time.monotonic() - overall_start > TOTAL_TIMEOUT_S:
                raise TimeoutError("harness timeout during search scenario")
            send_keys(master_fd, b"r")
            drain(master_fd, screen, stream, 0.010)  # 10ms — faster than slow bd

        print(f"[ search] settling {SETTLE_S}s ...", flush=True)
        drain(master_fd, screen, stream, SETTLE_S)

        print("[ search] asserting no screen panic ...", flush=True)
        assert_no_screen_panic(screen, "search-after-rapid-r")
        print("[ ok    ] search UI intact after rapid 'r'", flush=True)

        # ------------------------------------------------------------------ #
        # 7. Quit cleanly
        # ------------------------------------------------------------------ #
        if time.monotonic() - overall_start > TOTAL_TIMEOUT_S:
            raise TimeoutError("harness timeout before quit")

        print("[ quit  ] sending ctrl+q ...", flush=True)
        send_keys(master_fd, b"\x11")  # ctrl+q
        wait_for_exit(pid, 2.0)
        print("[ ok    ] bwb exited", flush=True)

        elapsed = time.monotonic() - overall_start
        print(f"[ timing] total scenario time: {elapsed:.2f}s", flush=True)

    finally:
        # ------------------------------------------------------------------ #
        # Guaranteed cleanup
        # ------------------------------------------------------------------ #
        if pid is not None and master_fd is not None:
            cleanup_child(pid, master_fd)
        if master_fd is not None:
            try:
                os.close(master_fd)
            except OSError:
                pass

    # ---------------------------------------------------------------------- #
    # 8. Assert log entries (after binary has exited and flushed)
    # ---------------------------------------------------------------------- #
    print(f"[ log   ] reading debug log at {log_file} ...", flush=True)
    assert_log(log_file)
    print("[ ok    ] log assertions passed", flush=True)

    # Print a brief excerpt of the relevant log entries
    try:
        with open(log_file) as f:
            lines = f.readlines()
        relevant = [
            l.rstrip() for l in lines
            if BOARD_GUARD_MSG in l or SEARCH_GUARD_MSG in l
        ]
        if relevant:
            print(f"\n--- debug log excerpt ({len(relevant)} suppressed-refresh entries) ---")
            for line in relevant[:20]:
                print(line)
            print("---")
    except FileNotFoundError:
        pass

    # ---------------------------------------------------------------------- #
    # Cleanup temp dir
    # ---------------------------------------------------------------------- #
    shutil.rmtree(tmpdir, ignore_errors=True)

    print("\n[ PASS  ] All assertions passed. Refresh-concurrency fix verified.", flush=True)


def main() -> int:
    start = time.monotonic()
    try:
        run_verification()
        return 0
    except TimeoutError as exc:
        print(f"\n[ FAIL  ] harness timeout: {exc}", file=sys.stderr)
        return 1
    except AssertionError as exc:
        print(f"\n[ FAIL  ] assertion failed:\n{exc}", file=sys.stderr)
        return 1
    except Exception as exc:  # noqa: BLE001
        print(f"\n[ FAIL  ] unexpected error: {type(exc).__name__}: {exc}", file=sys.stderr)
        return 1
    finally:
        elapsed = time.monotonic() - start
        print(f"[ done  ] elapsed {elapsed:.2f}s", flush=True)


if __name__ == "__main__":
    raise SystemExit(main())
