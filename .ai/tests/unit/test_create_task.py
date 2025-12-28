"""
Unit tests for create_task.py
"""

from __future__ import annotations

import sys
from pathlib import Path

import pytest

# Add scripts directory to path for imports
SCRIPTS_DIR = Path(__file__).parent.parent.parent / "scripts"
sys.path.insert(0, str(SCRIPTS_DIR))

import create_task  # noqa: E402
from lib.errors import ValidationError  # noqa: E402


def _ticket_body_without_spec_and_task_line() -> str:
    return """## Summary
Implement something.

## Scope
- Add a thing

## Acceptance Criteria
- [ ] Works

## Testing Requirements
- Unit tests added

## Metadata
- **Repo**: root
- **Priority**: P2
- **Release**: false
"""


def test_validate_body_missing_sections_raises() -> None:
    with pytest.raises(ValidationError):
        create_task._validate_body("## Summary\nMissing the rest\n")


def test_main_creates_issue_and_updates_tasks(temp_git_repo: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("AI_STATE_ROOT", str(temp_git_repo))
    monkeypatch.chdir(temp_git_repo)

    (temp_git_repo / ".ai" / "config").mkdir(parents=True, exist_ok=True)
    (temp_git_repo / ".ai" / "specs" / "snake-arena").mkdir(parents=True, exist_ok=True)
    (temp_git_repo / ".ai" / "temp").mkdir(parents=True, exist_ok=True)

    (temp_git_repo / ".ai" / "config" / "workflow.yaml").write_text(
        """version: "1.0"
project:
  name: "test-project"
  type: "single-repo"
repos:
  - name: root
    path: "./"
    type: root
    language: python
    verify:
      build: "echo build"
      test: "echo test"
specs:
  base_path: ".ai/specs"
  active: []
github:
  repo: ""
  labels:
    task: "ai-task"
""",
        encoding="utf-8",
    )

    tasks_path = temp_git_repo / ".ai" / "specs" / "snake-arena" / "tasks.md"
    tasks_path.write_text("- [ ] Implement in-memory room manager (create/join/leave)\n", encoding="utf-8")

    body_path = temp_git_repo / ".ai" / "temp" / "create-task-body.md"
    body_path.write_text(_ticket_body_without_spec_and_task_line(), encoding="utf-8")

    calls: list[list[str]] = []

    def fake_run_gh_issue_create(cmd: list[str]) -> tuple[str, str]:
        calls.append(cmd)
        return ("https://github.com/org/repo/issues/123\n", "")

    monkeypatch.setattr(create_task, "_run_gh_issue_create", fake_run_gh_issue_create)

    rc = create_task.main(
        [
            "--spec",
            "snake-arena",
            "--task-line",
            "1",
            "--body-file",
            str(body_path),
        ]
    )
    assert rc == 0

    updated = tasks_path.read_text(encoding="utf-8")
    assert "<!-- Issue #123 -->" in updated

    assert calls, "expected gh issue create to be invoked"
    cmd = calls[0]
    assert cmd[:3] == ["gh", "issue", "create"]
    assert "--label" in cmd
    assert cmd[cmd.index("--label") + 1] == "ai-task"
    assert "--title" in cmd
    assert cmd[cmd.index("--title") + 1] == "[feat] implement in-memory room manager (create/join/leave)"

    final_body_path = temp_git_repo / ".ai" / "temp" / "create-task-snake-arena-1.md"
    final_body = final_body_path.read_text(encoding="utf-8")
    assert "## AWK Metadata" in final_body
    assert "- **Spec**: snake-arena" in final_body
    assert "- **Task Line**: 1" in final_body


def test_main_noop_when_issue_ref_present(temp_git_repo: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("AI_STATE_ROOT", str(temp_git_repo))
    monkeypatch.chdir(temp_git_repo)

    (temp_git_repo / ".ai" / "config").mkdir(parents=True, exist_ok=True)
    (temp_git_repo / ".ai" / "specs" / "snake-arena").mkdir(parents=True, exist_ok=True)
    (temp_git_repo / ".ai" / "temp").mkdir(parents=True, exist_ok=True)

    (temp_git_repo / ".ai" / "config" / "workflow.yaml").write_text(
        """version: "1.0"
specs:
  base_path: ".ai/specs"
github:
  repo: ""
""",
        encoding="utf-8",
    )

    tasks_path = temp_git_repo / ".ai" / "specs" / "snake-arena" / "tasks.md"
    tasks_path.write_text(
        "- [ ] Implement in-memory room manager (create/join/leave) <!-- Issue #5 -->\n",
        encoding="utf-8",
    )

    body_path = temp_git_repo / ".ai" / "temp" / "create-task-body.md"
    body_path.write_text(_ticket_body_without_spec_and_task_line(), encoding="utf-8")

    invoked: list[bool] = []

    def fake_run_gh_issue_create(_: list[str]) -> tuple[str, str]:
        invoked.append(True)
        return ("https://github.com/org/repo/issues/999\n", "")

    monkeypatch.setattr(create_task, "_run_gh_issue_create", fake_run_gh_issue_create)

    rc = create_task.main(
        [
            "--spec",
            "snake-arena",
            "--task-line",
            "1",
            "--body-file",
            str(body_path),
        ]
    )
    assert rc == 0
    assert invoked == []
