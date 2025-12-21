"""
Shared pytest fixtures for AI Workflow Kit tests.
"""
import pytest
import tempfile
import shutil
import subprocess
import os
from pathlib import Path


# Get the .ai root directory
AI_ROOT = Path(__file__).parent.parent
PROJECT_ROOT = AI_ROOT.parent
SCRIPTS_DIR = AI_ROOT / "scripts"
FIXTURES_DIR = Path(__file__).parent / "fixtures"


@pytest.fixture
def ai_root():
    """Return the .ai directory path."""
    return AI_ROOT


@pytest.fixture
def project_root():
    """Return the project root directory."""
    return PROJECT_ROOT


@pytest.fixture
def scripts_dir():
    """Return the scripts directory path."""
    return SCRIPTS_DIR


@pytest.fixture
def fixtures_dir():
    """Return the fixtures directory path."""
    return FIXTURES_DIR


@pytest.fixture
def temp_dir():
    """Create a temporary directory for tests."""
    tmpdir = tempfile.mkdtemp(prefix="awk_test_")
    yield Path(tmpdir)
    shutil.rmtree(tmpdir, ignore_errors=True)


@pytest.fixture
def temp_git_repo(temp_dir):
    """Create a temporary git repository."""
    subprocess.run(
        ["git", "init"],
        cwd=temp_dir,
        capture_output=True,
        check=True
    )
    subprocess.run(
        ["git", "config", "user.email", "test@test.com"],
        cwd=temp_dir,
        capture_output=True,
        check=True
    )
    subprocess.run(
        ["git", "config", "user.name", "Test User"],
        cwd=temp_dir,
        capture_output=True,
        check=True
    )
    # Create initial commit
    (temp_dir / ".gitkeep").write_text("")
    subprocess.run(
        ["git", "add", "."],
        cwd=temp_dir,
        capture_output=True,
        check=True
    )
    subprocess.run(
        ["git", "commit", "-m", "Initial commit"],
        cwd=temp_dir,
        capture_output=True,
        check=True
    )
    return temp_dir


@pytest.fixture
def mock_repo_structure(temp_git_repo):
    """Create a mock repo with full .ai structure."""
    ai_dir = temp_git_repo / ".ai"

    # Create directories
    (ai_dir / "config").mkdir(parents=True)
    (ai_dir / "scripts").mkdir(parents=True)
    (ai_dir / "rules" / "_kit").mkdir(parents=True)
    (ai_dir / "specs").mkdir(parents=True)
    (ai_dir / "state").mkdir(parents=True)
    (ai_dir / "results").mkdir(parents=True)

    # Create required files
    (temp_git_repo / "CLAUDE.md").write_text("# CLAUDE.md\n\nProject instructions.\n")
    (temp_git_repo / "AGENTS.md").write_text("# AGENTS.md\n\nAgent instructions.\n")

    # Create workflow.yaml
    workflow_content = '''version: "1.0"
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
git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"
specs:
  base_path: ".ai/specs"
  active: []
github:
  repo: ""
rules:
  kit: []
  custom: []
escalation:
  max_consecutive_failures: 3
'''
    (ai_dir / "config" / "workflow.yaml").write_text(workflow_content)

    # Copy schema from actual project
    schema_src = AI_ROOT / "config" / "workflow.schema.json"
    if schema_src.exists():
        shutil.copy(schema_src, ai_dir / "config" / "workflow.schema.json")

    return temp_git_repo


@pytest.fixture
def sample_workflow_yaml():
    """Return sample workflow.yaml content."""
    return '''version: "1.0"
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
git:
  integration_branch: "develop"
  release_branch: "main"
  commit_format: "[type] subject"
specs:
  base_path: ".ai/specs"
  active: []
github:
  repo: ""
rules:
  kit: []
  custom: []
escalation:
  max_consecutive_failures: 3
'''
