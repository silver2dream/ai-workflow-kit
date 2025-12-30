#!/usr/bin/env python3
"""
create_task.py - Create a GitHub Issue from a tasks.md entry and write back the mapping.

This script is intended to be used by Principal when NEXT_ACTION=create_task.

Key behaviors:
- Validates the provided issue body draft (required sections + checkboxes)
- Creates the issue via `gh issue create`
- Appends `<!-- Issue #N -->` to the target tasks.md line (idempotent)
- Injects AWK metadata (**Spec**, **Task Line**) into the issue body if missing
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from lib.errors import AWKError, ConfigError, ValidationError, handle_unexpected_error, print_error


ISSUE_REF_RE = re.compile(r"<!--\s*Issue\s*#(?P<number>\d+)\s*-->")


def _resolve_root() -> Path:
    env_root = os.environ.get("AI_STATE_ROOT", "").strip()
    if env_root:
        return Path(env_root).resolve()

    try:
        out = subprocess.check_output(["git", "rev-parse", "--show-toplevel"], text=True).strip()
    except Exception as exc:
        raise ConfigError(
            "failed to resolve repo root (set AI_STATE_ROOT or run inside a git repo)",
            details={"exception": exc.__class__.__name__},
        ) from exc
    return Path(out).resolve()


def _load_yaml(path: Path) -> dict:
    try:
        import yaml  # type: ignore
    except Exception as exc:
        raise ConfigError(
            "python dependency missing: pyyaml",
            suggestion="install with: pip3 install pyyaml",
            details={"exception": exc.__class__.__name__},
        ) from exc

    try:
        data = yaml.safe_load(path.read_text(encoding="utf-8"))
    except Exception as exc:
        raise ConfigError(
            f"failed to read workflow config: {path}",
            details={"exception": exc.__class__.__name__},
        ) from exc

    return data or {}


def _extract_task_text(task_line: str) -> str:
    line = task_line.rstrip("\r\n")
    line = ISSUE_REF_RE.sub("", line).strip()
    line = re.sub(r"^\s*-\s*\[[ xX]\]\s*", "", line).strip()
    return line


def _default_issue_title(task_text: str) -> str:
    if not task_text:
        return "[feat] implement task"

    if task_text.lstrip().startswith("["):
        return task_text.strip()

    normalized = re.sub(r"\s+", " ", task_text.strip()).lower()
    return f"[feat] {normalized}"


def _validate_body(body: str) -> None:
    required_sections = [
        "Summary",
        "Scope",
        "Acceptance Criteria",
        "Testing Requirements",
        "Metadata",
    ]

    missing = []
    for section in required_sections:
        if not re.search(rf"(?im)^\s*##\s+{re.escape(section)}\s*$", body):
            missing.append(section)

    if missing:
        raise ValidationError(
            "ticket body missing required sections",
            details={"missing_sections": missing},
            suggestion="add the missing sections and re-run create_task.py",
        )

    if not re.search(r"(?m)^\s*-\s*\[\s*\]\s+", body):
        raise ValidationError(
            "ticket body missing acceptance checkboxes (- [ ])",
            suggestion="add at least one checklist item under '## Acceptance Criteria'",
        )


def _ensure_awk_metadata(body: str, spec: str, task_line: int) -> str:
    needs_spec = not re.search(r"(?i)\*\*spec\*\*\s*:", body)
    needs_task_line = not re.search(r"(?i)\*\*task\s+line\*\*\s*:", body)

    if not (needs_spec or needs_task_line):
        return body

    appended = "\n\n## AWK Metadata\n"
    if needs_spec:
        appended += f"- **Spec**: {spec}\n"
    if needs_task_line:
        appended += f"- **Task Line**: {task_line}\n"

    return body.rstrip() + appended


def _append_issue_ref(tasks_path: Path, line_number: int, issue_number: int) -> None:
    lines = tasks_path.read_text(encoding="utf-8").splitlines(keepends=True)
    if not (1 <= line_number <= len(lines)):
        raise ValidationError(
            f"task_line out of range for tasks file: {tasks_path}",
            details={"task_line": line_number, "lines": len(lines)},
        )

    current = lines[line_number - 1]
    existing = ISSUE_REF_RE.search(current)
    if existing:
        return

    suffix = f" <!-- Issue #{issue_number} -->\n"
    updated = current.rstrip("\r\n")
    if updated.endswith(" "):
        lines[line_number - 1] = updated + suffix.lstrip()
    else:
        lines[line_number - 1] = updated + suffix

    tasks_path.write_text("".join(lines), encoding="utf-8")


def _run_gh_issue_create(cmd: list[str]) -> tuple[str, str]:
    try:
        proc = subprocess.run(cmd, check=True, capture_output=True, text=True)
    except FileNotFoundError as exc:
        raise ConfigError("gh CLI not found", suggestion="install GitHub CLI: https://cli.github.com/") from exc
    except subprocess.CalledProcessError as exc:
        raise ValidationError(
            "gh issue create failed",
            details={
                "stdout": (exc.stdout or "").strip(),
                "stderr": (exc.stderr or "").strip(),
                "returncode": exc.returncode,
            },
        ) from exc

    return proc.stdout or "", proc.stderr or ""


def _parse_issue_number(output: str) -> int:
    match = re.search(r"/issues/(?P<number>\d+)\b", output)
    if not match:
        raise ValidationError(
            "failed to parse issue number from gh output",
            details={"output": output.strip()},
            suggestion="ensure `gh issue create` prints the created issue URL",
        )
    return int(match.group("number"))


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--spec", required=True)
    parser.add_argument("--task-line", required=True, type=int)
    parser.add_argument("--body-file", required=True)
    parser.add_argument("--title", default="")
    parser.add_argument("--repo", default="")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)

    root = _resolve_root()
    config_path = root / ".ai" / "config" / "workflow.yaml"
    config = _load_yaml(config_path) if config_path.exists() else {}

    spec_base = str(config.get("specs", {}).get("base_path", ".ai/specs"))
    tasks_path = root / spec_base / args.spec / "tasks.md"
    if not tasks_path.exists():
        raise ConfigError(
            f"tasks.md not found: {tasks_path}",
            suggestion="run generate.sh or fix specs.base_path / SPEC_NAME",
        )

    task_lines = tasks_path.read_text(encoding="utf-8").splitlines()
    if not (1 <= args.task_line <= len(task_lines)):
        raise ValidationError(
            "TASK_LINE out of range for tasks.md",
            details={"task_line": args.task_line, "tasks_path": str(tasks_path)},
        )

    raw_task_line = task_lines[args.task_line - 1]
    existing = ISSUE_REF_RE.search(raw_task_line)
    if existing:
        return 0

    task_text = _extract_task_text(raw_task_line)
    title = args.title.strip() or _default_issue_title(task_text)

    body_path = Path(args.body_file)
    if not body_path.is_absolute():
        body_path = (Path.cwd() / body_path).resolve()
    if not body_path.exists():
        raise ValidationError("body file not found", details={"body_file": str(body_path)})

    body = body_path.read_text(encoding="utf-8")
    _validate_body(body)
    body = _ensure_awk_metadata(body, args.spec, args.task_line)

    temp_dir = root / ".ai" / "temp"
    temp_dir.mkdir(parents=True, exist_ok=True)
    final_body_path = temp_dir / f"create-task-{args.spec}-{args.task_line}.md"
    final_body_path.write_text(body, encoding="utf-8")

    label_task = str(config.get("github", {}).get("labels", {}).get("task", "ai-task"))
    repo = args.repo.strip() or str(config.get("github", {}).get("repo", "")).strip()

    gh_cmd = [
        "gh",
        "issue",
        "create",
        "--title",
        title,
        "--body-file",
        str(final_body_path),
        "--label",
        label_task,
    ]
    if repo:
        gh_cmd.extend(["--repo", repo])

    if args.dry_run:
        sys.stdout.write("DRY_RUN: " + " ".join(gh_cmd) + "\n")
        return 0

    stdout, stderr = _run_gh_issue_create(gh_cmd)
    issue_number = _parse_issue_number(stdout + "\n" + stderr)

    _append_issue_ref(tasks_path, args.task_line, issue_number)
    sys.stdout.write(f"Created Issue #{issue_number}\n")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except AWKError as err:
        print_error(err)
        raise SystemExit(err.exit_code)
    except Exception as exc:  # pragma: no cover
        err = handle_unexpected_error(exc)
        print_error(err)
        raise SystemExit(err.exit_code)

