#!/usr/bin/env python3
"""
run_with_timeout.py

Cross-platform timeout wrapper for AWK bash scripts.

Usage:
  python3 run_with_timeout.py <timeout_seconds> <command> [args...]

Exit codes:
  - exits with the underlying command's exit code
  - 124 on timeout (GNU timeout compatible)
  - 125 on invalid usage
  - 127 if command is not found
"""

from __future__ import annotations

import os
import shlex
import signal
import subprocess
import sys
import time
from typing import Sequence


def _eprint(msg: str) -> None:
    print(msg, file=sys.stderr)


def _cmd_str(cmd: Sequence[str]) -> str:
    if os.name == "nt":
        return subprocess.list2cmdline(list(cmd))
    return " ".join(shlex.quote(c) for c in cmd)


def _normalize_return_code(rc: int | None) -> int:
    if rc is None:
        return 1
    if rc < 0:
        sig = -rc
        if 0 < sig < 128:
            return 128 + sig
        return 1
    return rc


def _kill_process_tree(proc: subprocess.Popen[bytes]) -> None:
    if proc.poll() is not None:
        return

    if os.name == "nt":
        try:
            result = subprocess.run(
                ["taskkill", "/PID", str(proc.pid), "/T", "/F"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=False,
            )
            if result.returncode == 0:
                return
            _eprint(
                f"[TIMEOUT] taskkill returned {result.returncode} for pid {proc.pid}; falling back to proc.kill()"
            )
        except Exception as e:
            _eprint(f"[TIMEOUT] failed to taskkill process tree for pid {proc.pid}: {e}")
        try:
            proc.kill()
        except Exception as e:
            _eprint(f"[TIMEOUT] failed to kill process pid {proc.pid} on Windows: {e}")
        return

    # POSIX: try TERM then KILL the whole process group.
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except Exception as e:
        _eprint(f"[TIMEOUT] failed to send SIGTERM to process group for pid {proc.pid}: {e}")
        try:
            proc.terminate()
        except Exception as e2:
            _eprint(f"[TIMEOUT] failed to terminate process pid {proc.pid}: {e2}")

    deadline = time.time() + 2.0
    while time.time() < deadline:
        if proc.poll() is not None:
            return
        time.sleep(0.05)

    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except Exception as e:
        _eprint(f"[TIMEOUT] failed to send SIGKILL to process group for pid {proc.pid}: {e}")
        try:
            proc.kill()
        except Exception as e2:
            _eprint(f"[TIMEOUT] failed to kill process pid {proc.pid} after SIGKILL failure: {e2}")


def main(argv: list[str]) -> int:
    if len(argv) < 3:
        _eprint("Usage: run_with_timeout.py <timeout_seconds> <command> [args...]")
        return 125

    try:
        timeout_sec = float(argv[1])
    except ValueError:
        _eprint(f"[TIMEOUT] invalid timeout_seconds: {argv[1]!r}")
        return 125

    cmd = argv[2:]
    if timeout_sec <= 0:
        try:
            rc = subprocess.call(cmd)
        except FileNotFoundError:
            _eprint(f"[TIMEOUT] command not found: {cmd[0]}")
            return 127
        return _normalize_return_code(rc)

    popen_kwargs: dict[str, object] = {}
    if os.name == "nt":
        popen_kwargs["creationflags"] = getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
    else:
        popen_kwargs["start_new_session"] = True

    try:
        proc = subprocess.Popen(cmd, **popen_kwargs)  # type: ignore[arg-type]
    except FileNotFoundError:
        _eprint(f"[TIMEOUT] command not found: {cmd[0]}")
        return 127

    started = time.time()
    try:
        rc = proc.wait(timeout=timeout_sec)
        return _normalize_return_code(rc)
    except subprocess.TimeoutExpired:
        elapsed = int(time.time() - started)
        _eprint(f"[TIMEOUT] timeout after {elapsed}s (limit={int(timeout_sec)}s): {_cmd_str(cmd)}")
        _kill_process_tree(proc)
        return 124


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
