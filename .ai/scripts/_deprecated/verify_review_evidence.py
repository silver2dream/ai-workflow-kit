#!/usr/bin/env python3
"""
verify_review_evidence.py

Verifies that a review body contains concrete evidence lines that match the PR diff.

Evidence line format (one per line):
  EVIDENCE: <file> | <needle>
or:
  EVIDENCE: <needle>

Rules:
  - At least 1 EVIDENCE line is required
  - Each <needle> must be found as a substring in the diff
  - If <file> is provided, the needle must be found within that file's diff section

Exit codes:
  0: OK
  1: Evidence missing / not found
  2: No evidence lines present
  3: Diff missing/empty or unreadable
"""

from __future__ import annotations

import re
import sys
import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Evidence:
    file: str
    needle: str
    raw: str


def _eprint(msg: str) -> None:
    print(msg, file=sys.stderr)


def _read_text(path: str) -> str:
    with open(path, "r", encoding="utf-8", errors="replace") as handle:
        return handle.read()


def _parse_evidence(review_text: str) -> list[Evidence]:
    out: list[Evidence] = []
    for line in review_text.splitlines():
        stripped = line.strip()
        if not stripped.startswith("EVIDENCE:"):
            continue
        rest = stripped[len("EVIDENCE:") :].strip()
        if not rest:
            continue
        if "|" in rest:
            left, right = rest.split("|", 1)
            file = left.strip()
            needle = right.strip()
        else:
            file = ""
            needle = rest.strip()

        # Strip common wrappers to reduce accidental verifier failures.
        if (needle.startswith('"') and needle.endswith('"')) or (needle.startswith("'") and needle.endswith("'")):
            needle = needle[1:-1]
        if needle.startswith("`") and needle.endswith("`") and len(needle) >= 2:
            needle = needle[1:-1]
        needle = needle.strip()

        if not needle:
            continue
        out.append(Evidence(file=file, needle=needle, raw=stripped))
    return out


def _split_diff_by_file(diff_text: str) -> dict[str, str]:
    sections: dict[str, list[str]] = {}
    current_file: str | None = None

    diff_header = re.compile(r"^diff --git a/(.+?) b/(.+?)$")
    for line in diff_text.splitlines(True):
        m = diff_header.match(line.rstrip("\n"))
        if m:
            current_file = m.group(2)
            sections.setdefault(current_file, []).append(line)
            continue
        if current_file is not None:
            sections[current_file].append(line)

    return {k: "".join(v) for k, v in sections.items()}


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        _eprint("Usage: verify_review_evidence.py <diff.patch> <review_body.md>")
        return 3

    diff_path = argv[1]
    review_path = argv[2]

    try:
        min_required = int(os.environ.get("AWK_REVIEW_EVIDENCE_MIN", "3"))
    except Exception:
        min_required = 3
    if min_required < 1:
        min_required = 1

    try:
        diff_text = _read_text(diff_path)
    except Exception as e:
        _eprint(f"[EVIDENCE] failed to read diff: {diff_path}: {e}")
        return 3

    if not diff_text.strip():
        _eprint(f"[EVIDENCE] diff is empty: {diff_path}")
        return 3

    try:
        review_text = _read_text(review_path)
    except Exception as e:
        _eprint(f"[EVIDENCE] failed to read review body: {review_path}: {e}")
        return 1

    evidence = _parse_evidence(review_text)
    if not evidence:
        _eprint("[EVIDENCE] no EVIDENCE lines found in review body")
        return 2
    if len(evidence) < min_required:
        _eprint(f"[EVIDENCE] insufficient EVIDENCE lines: {len(evidence)} < {min_required}")
        return 1

    by_file = _split_diff_by_file(diff_text)

    missing: list[str] = []
    for item in evidence:
        haystack = diff_text
        if item.file:
            if item.file not in by_file:
                missing.append(f"{item.raw} (file not in diff)")
                continue
            haystack = by_file[item.file]
        if item.needle not in haystack:
            missing.append(f"{item.raw} (needle not found)")

    if missing:
        _eprint("[EVIDENCE] evidence verification failed:")
        for m in missing:
            _eprint(f"  - {m}")
        return 1

    _eprint(f"[EVIDENCE] OK ({len(evidence)} evidence line(s) verified)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
