"""
Unit tests for scan_repo.py
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


class TestScanRepoFunctions:
    """Test individual functions from scan_repo.py"""

    def test_import_scan_repo(self):
        """Test that scan_repo.py can be imported."""
        import scan_repo
        assert hasattr(scan_repo, 'scan_repo')
        assert hasattr(scan_repo, 'get_repo_root')

    def test_get_repo_root_returns_path(self):
        """Test get_repo_root returns a Path object."""
        import scan_repo
        result = scan_repo.get_repo_root()
        assert isinstance(result, Path)

    def test_is_clean_on_clean_repo(self, temp_git_repo):
        """Test is_clean returns True for clean repo."""
        import scan_repo
        assert scan_repo.is_clean(temp_git_repo) is True

    def test_is_clean_on_dirty_repo(self, temp_git_repo):
        """Test is_clean returns False for dirty repo."""
        import scan_repo
        # Create an uncommitted file
        (temp_git_repo / "dirty.txt").write_text("dirty content")
        assert scan_repo.is_clean(temp_git_repo) is False

    def test_detect_language_go(self, temp_git_repo):
        """Test scan detects Go language via go.mod file."""
        import scan_repo

        # Create a submodule-like structure with go.mod
        (temp_git_repo / ".gitmodules").write_text("[submodule \"backend\"]\n\tpath = backend\n")
        backend = temp_git_repo / "backend"
        backend.mkdir()
        (backend / "go.mod").write_text("module example.com/backend\n")
        (backend / ".git").mkdir()  # Fake git dir

        # Initialize git in submodule
        subprocess.run(["git", "init"], cwd=backend, capture_output=True)
        subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=backend, capture_output=True)
        subprocess.run(["git", "config", "user.name", "Test"], cwd=backend, capture_output=True)
        (backend / ".gitkeep").write_text("")
        subprocess.run(["git", "add", "."], cwd=backend, capture_output=True)
        subprocess.run(["git", "commit", "-m", "init"], cwd=backend, capture_output=True)

        result = scan_repo.scan_repo(temp_git_repo)

        assert len(result["submodules"]) == 1
        assert result["submodules"][0]["kind"] == "go"

    def test_detect_language_unity(self, temp_git_repo):
        """Test scan detects Unity project via ProjectSettings folder."""
        import scan_repo

        # Create a submodule-like structure with Unity markers
        (temp_git_repo / ".gitmodules").write_text("[submodule \"game\"]\n\tpath = game\n")
        game = temp_git_repo / "game"
        game.mkdir()
        (game / "ProjectSettings").mkdir()
        (game / "Assets").mkdir()

        # Initialize git in submodule
        subprocess.run(["git", "init"], cwd=game, capture_output=True)
        subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=game, capture_output=True)
        subprocess.run(["git", "config", "user.name", "Test"], cwd=game, capture_output=True)
        (game / ".gitkeep").write_text("")
        subprocess.run(["git", "add", "."], cwd=game, capture_output=True)
        subprocess.run(["git", "commit", "-m", "init"], cwd=game, capture_output=True)

        result = scan_repo.scan_repo(temp_git_repo)

        assert len(result["submodules"]) == 1
        assert result["submodules"][0]["kind"] == "unity"

    def test_count_test_files_go(self, temp_git_repo):
        """Test scan counts Go test files correctly."""
        import scan_repo

        # Create submodule with test files
        (temp_git_repo / ".gitmodules").write_text("[submodule \"backend\"]\n\tpath = backend\n")
        backend = temp_git_repo / "backend"
        backend.mkdir()
        (backend / "go.mod").write_text("module example.com/backend\n")
        (backend / "main_test.go").write_text("package main\n")
        (backend / "handler_test.go").write_text("package main\n")
        (backend / "util_test.go").write_text("package main\n")

        # Initialize git
        subprocess.run(["git", "init"], cwd=backend, capture_output=True)
        subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=backend, capture_output=True)
        subprocess.run(["git", "config", "user.name", "Test"], cwd=backend, capture_output=True)
        subprocess.run(["git", "add", "."], cwd=backend, capture_output=True)
        subprocess.run(["git", "commit", "-m", "init"], cwd=backend, capture_output=True)

        result = scan_repo.scan_repo(temp_git_repo)

        assert result["submodules"][0]["test_files"] == 3


class TestScanRepoOutput:
    """Test scan_repo output structure."""

    def test_scan_output_structure(self, mock_repo_structure):
        """Test scan_repo returns correct structure."""
        import scan_repo
        result = scan_repo.scan_repo(mock_repo_structure)

        assert "timestamp_utc" in result
        assert "root" in result
        assert "submodules" in result
        assert "presence" in result
        assert "ai_config" in result

        # Check root structure
        assert "path" in result["root"]
        assert "branch" in result["root"]
        assert "clean" in result["root"]

    def test_output_matches_schema(self, mock_repo_structure, ai_root):
        """Test scan_repo output conforms to repo_scan.schema.json."""
        import scan_repo

        result = scan_repo.scan_repo(mock_repo_structure)

        # Load schema
        schema_path = ai_root / "config" / "repo_scan.schema.json"
        if not schema_path.exists():
            pytest.skip("repo_scan.schema.json not found")

        with open(schema_path, encoding='utf-8') as f:
            schema = json.load(f)

        # Validate required fields from schema
        assert "root" in result
        assert "submodules" in result
        assert "ai_config" in result

        # Validate root required fields
        assert "path" in result["root"]
        assert "clean" in result["root"]
        assert isinstance(result["root"]["clean"], bool)

        # Validate ai_config required fields
        assert "exists" in result["ai_config"]
        assert isinstance(result["ai_config"]["exists"], bool)

        # Validate submodules is array
        assert isinstance(result["submodules"], list)

    def test_scan_detects_presence(self, mock_repo_structure):
        """Test scan_repo detects file presence correctly."""
        import scan_repo
        result = scan_repo.scan_repo(mock_repo_structure)

        presence = result["presence"]
        assert "claude_md" in presence
        assert "agents_md" in presence
        assert presence["claude_md"] is True
        assert presence["agents_md"] is True

    def test_scan_detects_ai_config(self, mock_repo_structure):
        """Test scan_repo detects AI config correctly."""
        import scan_repo
        result = scan_repo.scan_repo(mock_repo_structure)

        ai_config = result["ai_config"]
        assert ai_config["exists"] is True
        assert ai_config["workflow_yaml"] is True
        assert ai_config["scripts_dir"] is True

    def test_scan_detects_branch(self, temp_git_repo):
        """Test scan_repo detects current branch."""
        import scan_repo
        result = scan_repo.scan_repo(temp_git_repo)

        # Default branch is usually master or main
        assert result["root"]["branch"] in ["master", "main"]


class TestScanRepoErrorHandling:
    """Test scan_repo.py error handling."""

    def test_no_git_dir(self, temp_dir):
        """Test scan_repo handles non-git directory gracefully."""
        import scan_repo

        # temp_dir is not a git repository
        result = scan_repo.scan_repo(temp_dir)

        # Should still return a valid structure
        assert "root" in result
        assert "submodules" in result
        assert "ai_config" in result

        # Should have empty/default values
        assert result["submodules"] == []

    def test_empty_dir(self, temp_dir):
        """Test scan_repo handles empty directory."""
        import scan_repo

        # Create an empty subdirectory
        empty_subdir = temp_dir / "empty"
        empty_subdir.mkdir()

        # Run from temp_dir (not a git repo)
        result = scan_repo.scan_repo(temp_dir)

        # Should return valid structure without crashing
        assert "root" in result
        assert "submodules" in result
        assert result["ai_config"]["exists"] is False

    def test_missing_gitmodules(self, temp_git_repo):
        """Test scan_repo handles missing .gitmodules."""
        import scan_repo

        # temp_git_repo has no .gitmodules
        result = scan_repo.scan_repo(temp_git_repo)

        assert result["presence"]["gitmodules"] is False
        assert result["submodules"] == []

    def test_nonexistent_submodule_path(self, temp_git_repo):
        """Test scan_repo handles submodule path that doesn't exist."""
        import scan_repo

        # Create .gitmodules pointing to non-existent path
        (temp_git_repo / ".gitmodules").write_text(
            "[submodule \"missing\"]\n\tpath = nonexistent_path\n"
        )

        result = scan_repo.scan_repo(temp_git_repo)

        assert len(result["submodules"]) == 1
        assert result["submodules"][0]["path"] == "nonexistent_path"
        assert result["submodules"][0]["exists"] is False


class TestScanRepoCLI:
    """Test scan_repo.py CLI execution."""

    def test_cli_json_output(self, mock_repo_structure):
        """Test scan_repo.py --json outputs valid JSON."""
        script_path = SCRIPTS_DIR / "scan_repo.py"

        result = subprocess.run(
            [sys.executable, str(script_path), "--json"],
            cwd=mock_repo_structure,
            capture_output=True,
            text=True
        )

        assert result.returncode == 0
        # Should be valid JSON
        data = json.loads(result.stdout)
        assert "root" in data
        assert "submodules" in data
        assert "presence" in data
        assert "ai_config" in data

    def test_cli_writes_state_file(self, mock_repo_structure):
        """Test scan_repo.py writes state file."""
        script_path = SCRIPTS_DIR / "scan_repo.py"

        result = subprocess.run(
            [sys.executable, str(script_path)],
            cwd=mock_repo_structure,
            capture_output=True,
            text=True
        )

        assert result.returncode == 0

        state_file = mock_repo_structure / ".ai" / "state" / "repo_scan.json"
        assert state_file.exists()

        # Validate content
        with open(state_file) as f:
            data = json.load(f)
        assert "root" in data
        assert "submodules" in data

    def test_cli_text_output(self, mock_repo_structure):
        """Test scan_repo.py text output contains expected info."""
        script_path = SCRIPTS_DIR / "scan_repo.py"

        result = subprocess.run(
            [sys.executable, str(script_path)],
            cwd=mock_repo_structure,
            capture_output=True,
            text=True
        )

        assert result.returncode == 0
        # Should contain key information
        assert "Repository:" in result.stdout or "Branch:" in result.stdout
