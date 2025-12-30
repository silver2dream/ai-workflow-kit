#!/usr/bin/env python3
"""
query_traces.py - Query execution traces.

Usage:
    python3 .ai/scripts/query_traces.py [--issue-id ID] [--status success|failed] [--json] [--log-level LEVEL]
"""
import argparse
import json
import os
import sys
from pathlib import Path
from typing import Any, Dict, List

# Add scripts directory to Python path for lib imports
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
if SCRIPT_DIR not in sys.path:
    sys.path.insert(0, SCRIPT_DIR)

from lib.errors import AWKError, ConfigError, handle_unexpected_error, print_error
from lib.logger import Logger, normalize_level


def load_traces(trace_dir: Path) -> List[Dict[str, Any]]:
    traces = []
    for entry in trace_dir.glob("*.json"):
        try:
            with open(entry, "r", encoding="utf-8") as handle:
                traces.append(json.load(handle))
        except Exception:
            continue
    return traces


def summarize_trace(trace: Dict[str, Any]) -> Dict[str, Any]:
    steps = trace.get("steps", [])
    failed_steps = [s.get("name", "unknown") for s in steps if s.get("status") == "failed"]
    return {
        "trace_id": trace.get("trace_id", ""),
        "issue_id": trace.get("issue_id", ""),
        "repo": trace.get("repo", ""),
        "status": trace.get("status", ""),
        "duration_seconds": trace.get("duration_seconds", 0),
        "started_at": trace.get("started_at", ""),
        "ended_at": trace.get("ended_at", ""),
        "error": trace.get("error", ""),
        "failed_steps": failed_steps,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Query execution traces.")
    parser.add_argument("--issue-id", dest="issue_id", default="", help="Filter by issue id")
    parser.add_argument("--status", dest="status", default="", help="Filter by status (success|failed)")
    parser.add_argument("--json", dest="output_json", action="store_true", help="Output JSON")
    parser.add_argument("--log-level", dest="log_level", default="info", help="Log level")
    args = parser.parse_args()

    script_dir = Path(__file__).resolve().parent
    ai_root = script_dir.parent
    trace_dir = ai_root / "state" / "traces"
    logger = Logger("query_traces", ai_root / "logs", level=normalize_level(args.log_level))

    try:
        if not trace_dir.is_dir():
            raise ConfigError(
                f"Trace directory not found: {trace_dir}",
                suggestion="Run a workflow first to generate traces.",
            )

        traces = load_traces(trace_dir)
        if args.issue_id:
            traces = [t for t in traces if str(t.get("issue_id", "")) == str(args.issue_id)]
        if args.status:
            traces = [t for t in traces if t.get("status") == args.status]

        summaries = [summarize_trace(t) for t in traces]

        if args.output_json:
            print(json.dumps({"count": len(summaries), "traces": summaries}, indent=2, ensure_ascii=True))
        else:
            print(f"Total traces: {len(summaries)}")
            for summary in summaries:
                print(f"- issue={summary['issue_id']} status={summary['status']} duration={summary['duration_seconds']}s")
                if summary["failed_steps"]:
                    print(f"  failed_steps: {', '.join(summary['failed_steps'])}")
                if summary["error"]:
                    print(f"  error: {summary['error']}")
        logger.info("query complete", {"count": len(summaries)})
    except AWKError as err:
        logger.error("query failed", {"error": err.message})
        print_error(err)
        raise SystemExit(err.exit_code)
    except Exception as exc:
        err = handle_unexpected_error(exc)
        logger.error("query failed", {"error": str(exc)})
        print_error(err)
        raise SystemExit(err.exit_code)


if __name__ == "__main__":
    main()
