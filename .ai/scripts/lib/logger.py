"""Structured JSON logger for AI Workflow Kit scripts."""
from __future__ import annotations

import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, Optional, Tuple, List


LEVELS = {
    "debug": 10,
    "info": 20,
    "warn": 30,
    "error": 40,
}


def normalize_level(level: Optional[str], default: str = "info") -> str:
    if not level:
        return default
    lowered = level.strip().lower()
    return lowered if lowered in LEVELS else default


def split_log_level(argv: List[str], default: str = "info") -> Tuple[str, List[str], Optional[str]]:
    """Extract --log-level from argv. Returns (level, remaining_args, error)."""
    level = default
    remaining: List[str] = []
    skip_next = False
    error = None
    for idx, arg in enumerate(argv):
        if skip_next:
            skip_next = False
            continue
        if arg == "--log-level":
            if idx + 1 >= len(argv):
                error = "Missing value for --log-level"
                continue
            level = argv[idx + 1]
            skip_next = True
            continue
        remaining.append(arg)
    return normalize_level(level, default), remaining, error


class Logger:
    def __init__(self, source: str, log_dir: Path, level: str = "info") -> None:
        self.source = source
        self.level = normalize_level(level)
        self.log_dir = Path(log_dir)
        self.log_dir.mkdir(parents=True, exist_ok=True)

    def _log_path(self) -> Path:
        date_stamp = datetime.now(timezone.utc).strftime("%Y-%m-%d")
        filename = f"{self.source}-{date_stamp}.log"
        return self.log_dir / filename

    def _should_log(self, level: str) -> bool:
        return LEVELS.get(level, 100) >= LEVELS.get(self.level, 20)

    def log(self, level: str, message: str, context: Optional[Dict[str, Any]] = None) -> None:
        lvl = normalize_level(level)
        if not self._should_log(lvl):
            return
        record = {
            "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "level": lvl,
            "source": self.source,
            "message": message,
            "context": context or {},
        }
        with open(self._log_path(), "a", encoding="utf-8") as handle:
            handle.write(json.dumps(record, ensure_ascii=True))
            handle.write("\n")

    def debug(self, message: str, context: Optional[Dict[str, Any]] = None) -> None:
        self.log("debug", message, context)

    def info(self, message: str, context: Optional[Dict[str, Any]] = None) -> None:
        self.log("info", message, context)

    def warn(self, message: str, context: Optional[Dict[str, Any]] = None) -> None:
        self.log("warn", message, context)

    def error(self, message: str, context: Optional[Dict[str, Any]] = None) -> None:
        self.log("error", message, context)
