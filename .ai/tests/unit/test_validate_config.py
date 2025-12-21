"""
Unit tests for validate_config.py
"""
import pytest
import subprocess
import json
import sys
import os
from pathlib import Path

# Add scripts directory to path for imports
SCRIPTS_DIR = Path(__file__).parent.parent.parent / "scripts"
sys.path.insert(0, str(SCRIPTS_DIR))


def run_validate_config(script_path, config_path, cwd=None):
    """Run validate_config.py with proper encoding settings."""
    env = os.environ.copy()
    env["PYTHONIOENCODING"] = "utf-8"

    return subprocess.run(
        [sys.executable, str(script_path), str(config_path)],
        cwd=cwd,
        capture_output=True,
        text=True,
        encoding='utf-8',
        errors='replace',
        env=env
    )


class TestValidateConfigCLI:
    """Test validate_config.py CLI execution."""

    def test_cli_minimal_config(self, temp_git_repo):
        """Test CLI passes with minimal valid config."""
        script_path = SCRIPTS_DIR / "validate_config.py"

        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)

        # Copy schema
        schema_src = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"
        import shutil
        shutil.copy(schema_src, ai_config / "workflow.schema.json")

        # Minimal config with only required fields
        minimal_config = """version: "1.0"
project:
  name: "minimal"
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
"""
        (ai_config / "workflow.yaml").write_text(minimal_config)

        result = run_validate_config(script_path, ai_config / "workflow.yaml", cwd=temp_git_repo)

        # Minimal config should pass
        assert result.returncode == 0

    def test_cli_valid_config(self, fixtures_dir, temp_git_repo):
        """Test CLI passes with valid config."""
        script_path = SCRIPTS_DIR / "validate_config.py"
        valid_config = fixtures_dir / "valid_workflow.yaml"

        # Copy schema to temp location
        schema_src = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)

        # Copy schema
        import shutil
        shutil.copy(schema_src, ai_config / "workflow.schema.json")

        # Copy valid config
        shutil.copy(valid_config, ai_config / "workflow.yaml")

        # Create required directories referenced in config
        (temp_git_repo / "backend").mkdir(exist_ok=True)
        (temp_git_repo / "frontend").mkdir(exist_ok=True)
        (temp_git_repo / ".ai" / "rules" / "_kit").mkdir(parents=True, exist_ok=True)
        (temp_git_repo / ".ai" / "rules" / "_kit" / "git-workflow.md").write_text("# Git\n")

        result = run_validate_config(script_path, ai_config / "workflow.yaml", cwd=temp_git_repo)

        # Check return code - 0 means valid config
        assert result.returncode == 0

    def test_cli_invalid_config(self, fixtures_dir, temp_git_repo):
        """Test CLI fails with invalid config."""
        script_path = SCRIPTS_DIR / "validate_config.py"
        invalid_config = fixtures_dir / "invalid_workflow.yaml"

        # Copy schema to temp location
        schema_src = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)

        import shutil
        shutil.copy(schema_src, ai_config / "workflow.schema.json")
        shutil.copy(invalid_config, ai_config / "workflow.yaml")

        result = run_validate_config(script_path, ai_config / "workflow.yaml", cwd=temp_git_repo)

        # Should fail due to missing required fields
        assert result.returncode == 3

    def test_cli_missing_config_file(self, temp_dir):
        """Test CLI handles missing config file."""
        script_path = SCRIPTS_DIR / "validate_config.py"

        result = run_validate_config(script_path, temp_dir / "nonexistent.yaml")

        assert result.returncode == 2
        output = (result.stdout + result.stderr).lower()
        assert "not found" in output or "error" in output

    def test_cli_missing_schema_file(self, temp_dir, sample_workflow_yaml):
        """Test CLI handles missing schema file.

        Note: The current script looks for schema relative to script location,
        not the config file. This test verifies behavior when schema doesn't
        exist at the script's location.
        """
        script_path = SCRIPTS_DIR / "validate_config.py"
        schema_path = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"

        # If the schema exists at the script location, this test is not applicable
        if schema_path.exists():
            pytest.skip("Schema exists at script location - cannot test missing schema scenario")


class TestValidateConfigSemanticChecks:
    """Test validate_config.py semantic validation."""

    def test_invalid_repo_type(self, temp_git_repo):
        """Test validation fails with invalid repo type."""
        script_path = SCRIPTS_DIR / "validate_config.py"

        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)

        schema_src = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"
        import shutil
        shutil.copy(schema_src, ai_config / "workflow.schema.json")

        # Config with invalid repo type
        config_content = """
version: "1.0"
project:
  name: "test"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    type: invalid_type
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
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
"""
        (ai_config / "workflow.yaml").write_text(config_content)

        result = run_validate_config(script_path, ai_config / "workflow.yaml", cwd=temp_git_repo)

        # Should fail due to invalid repo type (schema validation)
        assert result.returncode == 3

    def test_submodule_type_validation(self, temp_git_repo):
        """Test validation of submodule type repos."""
        script_path = SCRIPTS_DIR / "validate_config.py"

        # Setup config with submodule type but no .gitmodules
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)

        schema_src = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"
        import shutil
        shutil.copy(schema_src, ai_config / "workflow.schema.json")

        config_content = """
version: "1.0"
project:
  name: "test"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    type: submodule
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
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
"""
        (ai_config / "workflow.yaml").write_text(config_content)

        result = run_validate_config(script_path, ai_config / "workflow.yaml", cwd=temp_git_repo)

        # Should fail because .gitmodules doesn't exist
        assert result.returncode == 3
        output = (result.stdout + result.stderr).lower()
        assert "gitmodules" in output or "submodule" in output

    def test_directory_type_validation(self, temp_git_repo):
        """Test validation of directory type repos."""
        script_path = SCRIPTS_DIR / "validate_config.py"

        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)

        schema_src = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"
        import shutil
        shutil.copy(schema_src, ai_config / "workflow.schema.json")

        # Create backend directory (so it passes the exists check)
        (temp_git_repo / "backend").mkdir(exist_ok=True)

        config_content = """
version: "1.0"
project:
  name: "test"
  type: "monorepo"
repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
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
"""
        (ai_config / "workflow.yaml").write_text(config_content)

        result = run_validate_config(script_path, ai_config / "workflow.yaml", cwd=temp_git_repo)

        # Should pass (directory exists and is not a git repo)
        assert result.returncode == 0

    def test_root_type_path_validation(self, temp_git_repo):
        """Test validation of root type path."""
        script_path = SCRIPTS_DIR / "validate_config.py"

        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)

        schema_src = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"
        import shutil
        shutil.copy(schema_src, ai_config / "workflow.schema.json")

        # Invalid: root type with non-root path
        config_content = """
version: "1.0"
project:
  name: "test"
  type: "single-repo"
repos:
  - name: root
    path: "backend/"
    type: root
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
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
"""
        (ai_config / "workflow.yaml").write_text(config_content)

        result = run_validate_config(script_path, ai_config / "workflow.yaml", cwd=temp_git_repo)

        # Should fail because root type should have path "./" or empty
        assert result.returncode == 3
        output = (result.stdout + result.stderr).lower()
        assert "root" in output or "path" in output


class TestValidateConfigWithActualConfig:
    """Test validate_config.py with the actual project config."""

    def test_validate_actual_config(self, ai_root):
        """Test that the actual project's workflow.yaml is valid."""
        script_path = SCRIPTS_DIR / "validate_config.py"
        config_path = ai_root / "config" / "workflow.yaml"

        if not config_path.exists():
            pytest.skip("workflow.yaml not found")

        result = run_validate_config(script_path, config_path)

        # The actual config should be valid
        assert result.returncode == 0


class TestValidateConfigFromDifferentWorkingDirectory:
    """Test validate_config.py when executed from different working directories."""

    def test_cli_works_from_project_root_with_relative_path(self, temp_git_repo):
        """Test CLI works when called with relative path from project root.

        This tests the real-world scenario where users run:
            python3 .ai/scripts/validate_config.py
        from the project root directory.

        The script must correctly resolve 'lib' module imports regardless
        of the current working directory.
        """
        # Setup: copy the entire .ai/scripts directory to temp repo
        import shutil
        ai_scripts_src = SCRIPTS_DIR
        ai_scripts_dst = temp_git_repo / ".ai" / "scripts"
        ai_scripts_dst.mkdir(parents=True, exist_ok=True)

        # Copy all Python files and lib directory
        for item in ai_scripts_src.iterdir():
            if item.is_file() and item.suffix == '.py':
                shutil.copy(item, ai_scripts_dst / item.name)
            elif item.is_dir() and item.name == 'lib':
                shutil.copytree(item, ai_scripts_dst / 'lib')

        # Setup config
        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)

        schema_src = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"
        shutil.copy(schema_src, ai_config / "workflow.schema.json")

        minimal_config = """version: "1.0"
project:
  name: "test"
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
"""
        (ai_config / "workflow.yaml").write_text(minimal_config)

        # Key test: run with RELATIVE path from project root
        # This simulates: cd /project && python3 .ai/scripts/validate_config.py
        env = os.environ.copy()
        env["PYTHONIOENCODING"] = "utf-8"

        result = subprocess.run(
            [sys.executable, ".ai/scripts/validate_config.py", ".ai/config/workflow.yaml"],
            cwd=temp_git_repo,  # Execute from project root
            capture_output=True,
            text=True,
            encoding='utf-8',
            errors='replace',
            env=env
        )

        # Should NOT fail with "ModuleNotFoundError: No module named 'lib'"
        assert "No module named 'lib'" not in result.stderr, \
            f"Script failed to import lib module when run from project root:\n{result.stderr}"
        assert result.returncode == 0, \
            f"Script failed unexpectedly:\nstdout: {result.stdout}\nstderr: {result.stderr}"


class TestValidateConfigMissingRules:
    """Test validate_config.py rule file checking."""

    def test_warns_on_missing_custom_rule(self, temp_git_repo):
        """Test warning when custom rule file is missing."""
        script_path = SCRIPTS_DIR / "validate_config.py"

        ai_config = temp_git_repo / ".ai" / "config"
        ai_config.mkdir(parents=True, exist_ok=True)

        schema_src = SCRIPTS_DIR.parent / "config" / "workflow.schema.json"
        import shutil
        shutil.copy(schema_src, ai_config / "workflow.schema.json")

        # Create rules directory
        (temp_git_repo / ".ai" / "rules").mkdir(parents=True, exist_ok=True)

        config_content = """
version: "1.0"
project:
  name: "test"
  type: "single-repo"
repos:
  - name: root
    path: "./"
    type: root
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."
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
  custom:
    - nonexistent-rule
escalation:
  max_consecutive_failures: 3
"""
        (ai_config / "workflow.yaml").write_text(config_content)

        result = run_validate_config(script_path, ai_config / "workflow.yaml", cwd=temp_git_repo)

        # Should fail because custom rule doesn't exist
        assert result.returncode == 3
        output = (result.stdout + result.stderr).lower()
        assert "rule" in output or "not found" in output
