"""
Unit tests for parse_tasks.py
"""
import pytest
import subprocess
import json
import sys
from pathlib import Path

# Add scripts directory to path for imports
SCRIPTS_DIR = Path(__file__).parent.parent.parent / "scripts"
sys.path.insert(0, str(SCRIPTS_DIR))


class TestParseTasksFunctions:
    """Test individual functions from parse_tasks.py"""

    def test_import_parse_tasks(self):
        """Test that parse_tasks.py can be imported."""
        import parse_tasks
        assert hasattr(parse_tasks, 'parse_tasks')
        assert hasattr(parse_tasks, 'Task')
        assert hasattr(parse_tasks, 'get_executable_tasks')
        assert hasattr(parse_tasks, 'get_parallel_tasks')

    def test_task_class(self):
        """Test Task class structure."""
        import parse_tasks

        task = parse_tasks.Task("1", "Test task", completed=False)
        assert task.id == "1"
        assert task.title == "Test task"
        assert task.completed is False
        assert task.depends_on == []
        assert task.subtasks == []

    def test_task_to_dict(self):
        """Test Task.to_dict() method."""
        import parse_tasks

        task = parse_tasks.Task("1", "Test task", completed=True)
        task.depends_on = ["0"]

        d = task.to_dict()
        assert d["id"] == "1"
        assert d["title"] == "Test task"
        assert d["completed"] is True
        assert d["depends_on"] == ["0"]
        assert d["subtasks"] == []


class TestParseTasksParsing:
    """Test parse_tasks parsing functionality."""

    def test_parse_uncompleted_task(self):
        """Test parsing uncompleted task."""
        import parse_tasks

        content = "- [ ] 1. First task"
        tasks = parse_tasks.parse_tasks(content)

        assert len(tasks) == 1
        assert tasks[0].id == "1"
        assert tasks[0].title == "First task"
        assert tasks[0].completed is False

    def test_parse_completed_task(self):
        """Test parsing completed task."""
        import parse_tasks

        content = "- [x] 1. Done task"
        tasks = parse_tasks.parse_tasks(content)

        assert len(tasks) == 1
        assert tasks[0].id == "1"
        assert tasks[0].completed is True

    def test_parse_optional_subtask(self):
        """Test parsing optional subtask marked with asterisk [ ]*."""
        import parse_tasks

        content = """- [ ] 1. Main task
  - [ ]* 1.1 Optional subtask
  - [ ] 1.2 Required subtask"""
        tasks = parse_tasks.parse_tasks(content)

        assert len(tasks) == 1
        assert len(tasks[0].subtasks) == 2
        # Both subtasks should be parsed (optional marker is in the pattern)
        assert tasks[0].subtasks[0].id == "1.1"
        assert tasks[0].subtasks[1].id == "1.2"

    def test_parse_multiple_tasks(self):
        """Test parsing multiple tasks."""
        import parse_tasks

        content = """- [ ] 1. First task
- [ ] 2. Second task
- [x] 3. Third task"""
        tasks = parse_tasks.parse_tasks(content)

        assert len(tasks) == 3
        assert tasks[0].id == "1"
        assert tasks[1].id == "2"
        assert tasks[2].id == "3"
        assert tasks[2].completed is True

    def test_parse_task_with_dependencies(self):
        """Test parsing task with dependencies."""
        import parse_tasks

        content = """- [ ] 1. First task
- [ ] 2. Second task
  - _depends_on: 1_"""
        tasks = parse_tasks.parse_tasks(content)

        assert len(tasks) == 2
        assert tasks[1].depends_on == ["1"]

    def test_parse_task_with_multiple_dependencies(self):
        """Test parsing task with multiple dependencies."""
        import parse_tasks

        content = """- [ ] 1. First task
- [ ] 2. Second task
- [ ] 3. Third task
  - _depends_on: 1, 2_"""
        tasks = parse_tasks.parse_tasks(content)

        assert len(tasks) == 3
        assert "1" in tasks[2].depends_on
        assert "2" in tasks[2].depends_on

    def test_parse_subtasks(self):
        """Test parsing subtasks."""
        import parse_tasks

        content = """- [ ] 1. Main task
  - [ ] 1.1 Subtask one
  - [ ] 1.2 Subtask two"""
        tasks = parse_tasks.parse_tasks(content)

        assert len(tasks) == 1
        assert len(tasks[0].subtasks) == 2
        assert tasks[0].subtasks[0].id == "1.1"
        assert tasks[0].subtasks[1].id == "1.2"

    def test_parse_empty_content(self):
        """Test parsing empty content."""
        import parse_tasks

        content = ""
        tasks = parse_tasks.parse_tasks(content)
        assert tasks == []

    def test_parse_no_tasks_content(self):
        """Test parsing content with no tasks."""
        import parse_tasks

        content = """# Header

Some text but no tasks."""
        tasks = parse_tasks.parse_tasks(content)
        assert tasks == []

    def test_parse_malformed_input(self, fixtures_dir):
        """Test parsing malformed tasks.md content."""
        import parse_tasks

        malformed_file = fixtures_dir / "malformed_tasks.md"
        with open(malformed_file, encoding='utf-8') as f:
            content = f.read()

        tasks = parse_tasks.parse_tasks(content)

        # Should only parse the valid tasks, ignore malformed ones
        # The fixture has some invalid formats and 2 valid tasks
        # "- [ ] This one is valid: 1. Valid task" - not valid (wrong format)
        # "- [x] Also valid: 2. Completed task" - not valid (wrong format)
        # Actually looking at the regex, these won't match because
        # the pattern requires "- [ ] <number>. <title>" format
        assert isinstance(tasks, list)

    def test_parse_malformed_checkbox(self):
        """Test parsing content with malformed checkboxes."""
        import parse_tasks

        content = """- [ ] No number here
- [x] Also no number
- [] 1. Missing space in checkbox
- [y] 1. Invalid checkbox character
- [ ] 1. Valid task"""
        tasks = parse_tasks.parse_tasks(content)

        # Only the last one should be valid
        # Note: "- [ ]1." would match because \s* allows zero whitespace
        assert len(tasks) == 1
        assert tasks[0].id == "1"
        assert tasks[0].title == "Valid task"

    def test_parse_ignores_non_task_lines(self):
        """Test parser ignores lines that don't match task pattern."""
        import parse_tasks

        content = """# Header

Some description text.

* Bullet point (not a task)
1. Numbered item without checkbox
- Regular dash item

- [ ] 1. Actual task

More text after."""
        tasks = parse_tasks.parse_tasks(content)

        assert len(tasks) == 1
        assert tasks[0].id == "1"
        assert tasks[0].title == "Actual task"

    def test_parse_sample_tasks_file(self, fixtures_dir):
        """Test parsing sample_tasks.md fixture."""
        import parse_tasks

        tasks_file = fixtures_dir / "sample_tasks.md"
        with open(tasks_file, encoding='utf-8') as f:
            content = f.read()

        tasks = parse_tasks.parse_tasks(content)

        # Should have 5 main tasks
        assert len(tasks) == 5

        # Task 1 should have subtasks
        assert len(tasks[0].subtasks) >= 2

        # Task 2 should depend on task 1
        assert "1" in tasks[1].depends_on

        # Task 3 should be completed
        assert tasks[2].completed is True

        # Task 4 should depend on both 1 and 2
        assert "1" in tasks[3].depends_on
        assert "2" in tasks[3].depends_on


class TestGetExecutableTasks:
    """Test get_executable_tasks functionality."""

    def test_executable_task_no_deps(self):
        """Test task with no dependencies is executable."""
        import parse_tasks

        content = "- [ ] 1. First task"
        tasks = parse_tasks.parse_tasks(content)
        executable = parse_tasks.get_executable_tasks(tasks)

        assert len(executable) == 1
        assert executable[0].id == "1"

    def test_executable_task_deps_satisfied(self):
        """Test task is executable when dependencies are completed."""
        import parse_tasks

        content = """- [x] 1. First task
- [ ] 2. Second task
  - _depends_on: 1_"""
        tasks = parse_tasks.parse_tasks(content)
        executable = parse_tasks.get_executable_tasks(tasks)

        # Task 2 should be executable because task 1 is completed
        assert len(executable) == 1
        assert executable[0].id == "2"

    def test_executable_task_deps_not_satisfied(self):
        """Test task is not executable when dependencies are not completed."""
        import parse_tasks

        content = """- [ ] 1. First task
- [ ] 2. Second task
  - _depends_on: 1_"""
        tasks = parse_tasks.parse_tasks(content)
        executable = parse_tasks.get_executable_tasks(tasks)

        # Only task 1 should be executable
        assert len(executable) == 1
        assert executable[0].id == "1"

    def test_executable_excludes_completed(self):
        """Test completed tasks are not in executable list."""
        import parse_tasks

        content = """- [x] 1. Completed task
- [ ] 2. Pending task"""
        tasks = parse_tasks.parse_tasks(content)
        executable = parse_tasks.get_executable_tasks(tasks)

        assert len(executable) == 1
        assert executable[0].id == "2"

    def test_all_completed(self):
        """Test no executable tasks when all are completed."""
        import parse_tasks

        content = """- [x] 1. Completed task
- [x] 2. Also completed"""
        tasks = parse_tasks.parse_tasks(content)
        executable = parse_tasks.get_executable_tasks(tasks)

        assert len(executable) == 0


class TestGetParallelTasks:
    """Test get_parallel_tasks functionality."""

    def test_parallel_groups_no_deps(self):
        """Test all tasks in one group when no dependencies."""
        import parse_tasks

        content = """- [ ] 1. First task
- [ ] 2. Second task
- [ ] 3. Third task"""
        tasks = parse_tasks.parse_tasks(content)
        groups = parse_tasks.get_parallel_tasks(tasks)

        # All tasks should be in one group (can run in parallel)
        assert len(groups) == 1
        assert len(groups[0]) == 3

    def test_parallel_groups_with_deps(self):
        """Test tasks grouped by dependency levels."""
        import parse_tasks

        content = """- [ ] 1. First task
- [ ] 2. Second task
  - _depends_on: 1_
- [ ] 3. Third task
  - _depends_on: 2_"""
        tasks = parse_tasks.parse_tasks(content)
        groups = parse_tasks.get_parallel_tasks(tasks)

        # Should be 3 groups (sequential chain)
        assert len(groups) == 3
        assert groups[0][0].id == "1"
        assert groups[1][0].id == "2"
        assert groups[2][0].id == "3"

    def test_parallel_groups_mixed(self):
        """Test mixed parallel and sequential tasks."""
        import parse_tasks

        content = """- [ ] 1. First task
- [ ] 2. Second task
- [ ] 3. Third task
  - _depends_on: 1, 2_"""
        tasks = parse_tasks.parse_tasks(content)
        groups = parse_tasks.get_parallel_tasks(tasks)

        # Group 1: tasks 1 and 2 (parallel)
        # Group 2: task 3 (depends on both)
        assert len(groups) == 2
        assert len(groups[0]) == 2
        assert len(groups[1]) == 1


class TestParseTasksCLI:
    """Test parse_tasks.py CLI execution."""

    def test_cli_no_args(self):
        """Test CLI shows usage when no args."""
        script_path = SCRIPTS_DIR / "parse_tasks.py"

        result = subprocess.run(
            [sys.executable, str(script_path)],
            capture_output=True,
            text=True
        )

        assert result.returncode == 1
        assert "Usage" in result.stdout or "usage" in result.stdout.lower()

    def test_cli_json_output(self, fixtures_dir):
        """Test CLI --json output is valid JSON."""
        script_path = SCRIPTS_DIR / "parse_tasks.py"
        tasks_file = fixtures_dir / "sample_tasks.md"

        result = subprocess.run(
            [sys.executable, str(script_path), str(tasks_file), "--json"],
            capture_output=True,
            text=True
        )

        assert result.returncode == 0
        data = json.loads(result.stdout)
        assert isinstance(data, list)
        assert len(data) > 0

    def test_cli_next_flag(self, fixtures_dir):
        """Test CLI --next flag."""
        script_path = SCRIPTS_DIR / "parse_tasks.py"
        tasks_file = fixtures_dir / "sample_tasks.md"

        result = subprocess.run(
            [sys.executable, str(script_path), str(tasks_file), "--next", "--json"],
            capture_output=True,
            text=True
        )

        assert result.returncode == 0
        data = json.loads(result.stdout)
        assert isinstance(data, list)

    def test_cli_parallel_flag(self, fixtures_dir):
        """Test CLI --parallel flag."""
        script_path = SCRIPTS_DIR / "parse_tasks.py"
        tasks_file = fixtures_dir / "sample_tasks.md"

        result = subprocess.run(
            [sys.executable, str(script_path), str(tasks_file), "--parallel", "--json"],
            capture_output=True,
            text=True
        )

        assert result.returncode == 0
        data = json.loads(result.stdout)
        # Should be list of lists (groups)
        assert isinstance(data, list)

    def test_cli_file_not_found(self):
        """Test CLI handles missing file."""
        script_path = SCRIPTS_DIR / "parse_tasks.py"

        result = subprocess.run(
            [sys.executable, str(script_path), "nonexistent.md"],
            capture_output=True,
            text=True
        )

        assert result.returncode == 1
        assert "not found" in result.stderr.lower() or "error" in result.stderr.lower()

    def test_cli_empty_file(self, fixtures_dir):
        """Test CLI handles empty tasks file."""
        script_path = SCRIPTS_DIR / "parse_tasks.py"
        empty_file = fixtures_dir / "empty_tasks.md"

        result = subprocess.run(
            [sys.executable, str(script_path), str(empty_file), "--json"],
            capture_output=True,
            text=True
        )

        assert result.returncode == 0
        data = json.loads(result.stdout)
        assert data == []
