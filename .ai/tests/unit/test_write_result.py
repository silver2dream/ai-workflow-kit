"""
Unit tests for write_result.sh
"""
import json
import os
import shutil
import subprocess
from pathlib import Path

import pytest


def _posix_path_for_bash(bash_path: str, path: str) -> str:
    if os.name != "nt":
        return path
    drive, rest = os.path.splitdrive(path)
    if not drive:
        return path.replace("\\", "/")
    drive_letter = drive.rstrip(":").lower()
    rest = rest.replace("\\", "/")
    if rest.startswith("/"):
        rest = rest[1:]
    bash_lower = bash_path.lower()
    if "system32\\bash.exe" in bash_lower:
        return f"/mnt/{drive_letter}/{rest}"
    if "\\git\\" in bash_lower:
        return f"/{drive_letter}/{rest}"
    return f"/mnt/{drive_letter}/{rest}"


def test_write_result_includes_retry_count(temp_git_repo):
    script_path = Path(__file__).parent.parent.parent / "scripts" / "write_result.sh"
    bash_path = shutil.which("bash")
    if not bash_path:
        pytest.skip("bash not available")

    env = os.environ.copy()
    env.update({
        "AI_STATE_ROOT": str(temp_git_repo),
        "AI_RESULTS_ROOT": str(temp_git_repo),
        "AI_REPO_NAME": "root",
        "AI_BRANCH_NAME": "feat/ai-issue-123",
        "AI_PR_BASE": "feat/example",
        "AI_EXEC_DURATION": "12",
        "AI_RETRY_COUNT": "1",
    })

    if os.name == "nt":
        script_arg = _posix_path_for_bash(bash_path, str(script_path))
        root_arg = _posix_path_for_bash(bash_path, str(temp_git_repo))
        cmd = (
            f'AI_STATE_ROOT="{root_arg}" AI_RESULTS_ROOT="{root_arg}" '
            f'AI_EXEC_DURATION="{env["AI_EXEC_DURATION"]}" '
            f'AI_RETRY_COUNT="{env["AI_RETRY_COUNT"]}" '
            f'AI_REPO_NAME="{env["AI_REPO_NAME"]}" '
            f'AI_BRANCH_NAME="{env["AI_BRANCH_NAME"]}" '
            f'AI_PR_BASE="{env["AI_PR_BASE"]}" '
            f'"{script_arg}" 123 success "https://example.com/pr/1" "summary.txt"'
        )
        result = subprocess.run(
            [bash_path, "-lc", cmd],
            capture_output=True,
            text=True,
            env=env,
        )
    else:
        result = subprocess.run(
            [bash_path, str(script_path), "123", "success", "https://example.com/pr/1", "summary.txt"],
            cwd=temp_git_repo,
            capture_output=True,
            text=True,
            env=env,
        )

    assert result.returncode == 0

    output_path = temp_git_repo / ".ai" / "results" / "issue-123.json"
    assert output_path.exists()

    with open(output_path, "r", encoding="utf-8") as handle:
        data = json.load(handle)

    metrics = data.get("metrics", {})
    assert metrics.get("duration_seconds") == 12
    assert metrics.get("retry_count") == 1
