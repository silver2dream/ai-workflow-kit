"""
Unit tests for query_traces.py
"""
import json
import os
import subprocess
import sys
import uuid
from pathlib import Path


def test_query_traces_filters_by_issue_id(ai_root):
    script_path = ai_root / "scripts" / "query_traces.py"
    trace_dir = ai_root / "state" / "traces"
    trace_dir.mkdir(parents=True, exist_ok=True)

    issue_id = f"test-{uuid.uuid4()}"
    trace_file = trace_dir / f"issue-{issue_id}.json"
    trace_payload = {
        "trace_id": f"issue-{issue_id}-trace",
        "issue_id": issue_id,
        "repo": "root",
        "branch": "feat/ai-issue-test",
        "base_branch": "feat/example",
        "status": "failed",
        "started_at": "2024-01-01T00:00:00Z",
        "ended_at": "2024-01-01T00:00:05Z",
        "duration_seconds": 5,
        "error": "test failure",
        "steps": [
            {
                "name": "codex_exec_attempt_1",
                "status": "failed",
                "started_at": "2024-01-01T00:00:00Z",
                "ended_at": "2024-01-01T00:00:05Z",
                "duration_seconds": 5,
                "error": "rc=1",
                "context": {"attempt": 1},
            }
        ],
    }

    trace_file.write_text(json.dumps(trace_payload), encoding="utf-8")

    try:
        result = subprocess.run(
            [sys.executable, str(script_path), "--issue-id", issue_id, "--json"],
            capture_output=True,
            text=True,
            env=os.environ.copy(),
        )

        assert result.returncode == 0
        payload = json.loads(result.stdout)
        assert payload["count"] == 1
        assert payload["traces"][0]["issue_id"] == issue_id
    finally:
        trace_file.unlink(missing_ok=True)
