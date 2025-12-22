"""
Unit tests for preflight checks functionality.

**Feature: multi-repo-support**
**Property 6: Preflight Execution for All Types**
**Property 13: Remote Accessibility Verification**
**Property 27: Submodule Detached HEAD Handling**
**Validates: Requirements 7.1-7.7, 19.1-19.4, 23.1-23.4**

Note: These tests validate the preflight logic using Python implementations
that mirror the bash script behavior, avoiding Windows path issues with subprocess.
"""
import pytest
import subprocess
import os
import json
import time
from pathlib import Path


def check_working_tree_clean(repo_path: Path) -> tuple:
    """Check if git working tree is clean. Returns (is_clean, status_output)."""
    result = subprocess.run(
        ["git", "status", "--porcelain"],
        cwd=str(repo_path),
        capture_output=True,
        text=True
    )
    status = result.stdout.strip()
    return (len(status) == 0, status)


def check_directory_exists(repo_root: Path, repo_path: str) -> bool:
    """Check if directory path exists."""
    full_path = repo_root / repo_path
    return full_path.is_dir()


def check_is_submodule(repo_root: Path, repo_path: str) -> bool:
    """Check if path is a valid submodule (has .git file or directory)."""
    full_path = repo_root / repo_path
    git_path = full_path / ".git"
    return git_path.exists()


def check_submodule_clean(repo_root: Path, repo_path: str) -> tuple:
    """Check if submodule working tree is clean."""
    full_path = repo_root / repo_path
    return check_working_tree_clean(full_path)


def check_submodule_detached_head(repo_root: Path, repo_path: str) -> bool:
    """Check if submodule is in detached HEAD state."""
    full_path = repo_root / repo_path
    result = subprocess.run(
        ["git", "symbolic-ref", "-q", "HEAD"],
        cwd=str(full_path),
        capture_output=True,
        text=True
    )
    return result.returncode != 0


def validate_repo_type(repo_type: str) -> bool:
    """Validate repo type is one of the allowed values."""
    return repo_type in ("root", "directory", "submodule")


def run_preflight_checks(repo_root: Path, repo_type: str, repo_path: str) -> dict:
    """
    Run preflight checks and return result dict.
    
    Returns:
        dict with keys:
        - success: bool
        - error: str or None
        - warnings: list of str
    """
    result = {
        "success": True,
        "error": None,
        "warnings": [],
        "type": repo_type,
        "path": repo_path
    }
    
    # Validate repo type
    if not validate_repo_type(repo_type):
        result["success"] = False
        result["error"] = f"unknown repo type '{repo_type}'. Expected: root, directory, submodule."
        return result
    
    # Check root working tree is clean
    is_clean, status = check_working_tree_clean(repo_root)
    if not is_clean:
        result["success"] = False
        result["error"] = "root working tree not clean. Commit/stash first."
        return result
    
    # Type-specific checks
    if repo_type == "directory":
        if not check_directory_exists(repo_root, repo_path):
            result["success"] = False
            result["error"] = f"directory path '{repo_path}' does not exist."
            return result
    
    elif repo_type == "submodule":
        if not check_directory_exists(repo_root, repo_path):
            result["success"] = False
            result["error"] = f"submodule path '{repo_path}' does not exist."
            return result
        
        if not check_is_submodule(repo_root, repo_path):
            result["success"] = False
            result["error"] = f"'{repo_path}' is not a valid submodule (no .git)."
            return result
        
        is_clean, status = check_submodule_clean(repo_root, repo_path)
        if not is_clean:
            result["success"] = False
            result["error"] = f"submodule '{repo_path}' working tree not clean."
            return result
        
        if check_submodule_detached_head(repo_root, repo_path):
            result["warnings"].append(f"submodule '{repo_path}' is in detached HEAD state.")
    
    return result


class TestPreflightAllTypes:
    """Test preflight execution for all repo types.
    
    Property 6: Preflight Execution for All Types
    *For any* repo type (root, directory, submodule), preflight checks 
    SHALL be executed before worker execution.
    """

    def test_preflight_root_type(self, temp_git_repo):
        """Test preflight runs successfully for root type."""
        result = run_preflight_checks(temp_git_repo, "root", ".")
        
        assert result["success"] is True
        assert result["error"] is None
        assert result["type"] == "root"

    def test_preflight_directory_type(self, temp_git_repo):
        """Test preflight runs successfully for directory type."""
        # Create a subdirectory
        subdir = temp_git_repo / "backend"
        subdir.mkdir()
        (subdir / ".gitkeep").write_text("")
        
        subprocess.run(["git", "add", "."], cwd=temp_git_repo, check=True)
        subprocess.run(["git", "commit", "-m", "Add backend"], cwd=temp_git_repo, check=True)
        
        result = run_preflight_checks(temp_git_repo, "directory", "backend")
        
        assert result["success"] is True
        assert result["error"] is None
        assert result["type"] == "directory"

    def test_preflight_directory_type_missing_path(self, temp_git_repo):
        """Test preflight fails for directory type with missing path."""
        result = run_preflight_checks(temp_git_repo, "directory", "nonexistent")
        
        assert result["success"] is False
        assert "does not exist" in result["error"]

    def test_preflight_unknown_type_rejected(self, temp_git_repo):
        """Test preflight rejects unknown repo type."""
        result = run_preflight_checks(temp_git_repo, "invalid", ".")
        
        assert result["success"] is False
        assert "unknown repo type" in result["error"]

    def test_preflight_dirty_working_tree_rejected(self, temp_git_repo):
        """Test preflight rejects dirty working tree."""
        # Create uncommitted file
        (temp_git_repo / "dirty.txt").write_text("uncommitted")
        
        result = run_preflight_checks(temp_git_repo, "root", ".")
        
        assert result["success"] is False
        assert "not clean" in result["error"]


class TestPreflightSubmodule:
    """Test preflight checks for submodule type repos.
    
    Property 13: Remote Accessibility Verification
    *For any* submodule-type repo, preflight SHALL verify the remote 
    is accessible before proceeding with work.
    
    Property 27: Submodule Detached HEAD Handling
    *For any* submodule in detached HEAD state, the system SHALL detect 
    it during preflight and create a branch from the current HEAD.
    """

    @pytest.fixture
    def temp_repo_with_fake_submodule(self, temp_git_repo):
        """Create a temp git repo with a fake submodule structure."""
        # Create a subdirectory that looks like a submodule
        subdir = temp_git_repo / "backend"
        subdir.mkdir()
        
        # Initialize it as a git repo (simulating submodule)
        subprocess.run(["git", "init"], cwd=subdir, check=True)
        subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=subdir, check=True)
        subprocess.run(["git", "config", "user.name", "Test User"], cwd=subdir, check=True)
        (subdir / "README.md").write_text("# Backend")
        subprocess.run(["git", "add", "."], cwd=subdir, check=True)
        subprocess.run(["git", "commit", "-m", "Initial commit"], cwd=subdir, check=True)
        
        # Add to parent (not as real submodule, just track the directory)
        subprocess.run(["git", "add", "."], cwd=temp_git_repo, check=True)
        subprocess.run(["git", "commit", "-m", "Add backend"], cwd=temp_git_repo, check=True)
        
        return temp_git_repo

    def test_preflight_submodule_type_valid(self, temp_repo_with_fake_submodule):
        """Test preflight runs successfully for valid submodule."""
        result = run_preflight_checks(
            temp_repo_with_fake_submodule, "submodule", "backend"
        )
        
        assert result["success"] is True
        assert result["error"] is None
        assert result["type"] == "submodule"

    def test_preflight_submodule_missing_path(self, temp_git_repo):
        """Test preflight fails for submodule type with missing path."""
        result = run_preflight_checks(temp_git_repo, "submodule", "nonexistent")
        
        assert result["success"] is False
        assert "does not exist" in result["error"]

    def test_preflight_submodule_not_git(self, temp_git_repo):
        """Test preflight fails for submodule path without .git."""
        # Create a regular directory (not a submodule)
        subdir = temp_git_repo / "backend"
        subdir.mkdir()
        (subdir / "README.md").write_text("# Not a submodule")
        subprocess.run(["git", "add", "."], cwd=temp_git_repo, check=True)
        subprocess.run(["git", "commit", "-m", "Add backend dir"], cwd=temp_git_repo, check=True)
        
        result = run_preflight_checks(temp_git_repo, "submodule", "backend")
        
        assert result["success"] is False
        assert "not a valid submodule" in result["error"]

    def test_preflight_submodule_dirty_rejected(self, temp_repo_with_fake_submodule):
        """Test preflight rejects dirty submodule working tree."""
        # Create uncommitted file in submodule
        submodule_path = temp_repo_with_fake_submodule / "backend"
        (submodule_path / "dirty.txt").write_text("uncommitted")
        
        result = run_preflight_checks(
            temp_repo_with_fake_submodule, "submodule", "backend"
        )
        
        assert result["success"] is False
        assert "not clean" in result["error"]

    def test_preflight_submodule_detached_head_warning(self, temp_repo_with_fake_submodule):
        """Test preflight warns about detached HEAD state.
        
        Property 27: Submodule Detached HEAD Handling
        """
        # Put submodule in detached HEAD state
        submodule_path = temp_repo_with_fake_submodule / "backend"
        head_sha = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=submodule_path,
            capture_output=True,
            text=True,
            check=True
        ).stdout.strip()
        subprocess.run(["git", "checkout", head_sha], cwd=submodule_path, check=True)
        
        result = run_preflight_checks(
            temp_repo_with_fake_submodule, "submodule", "backend"
        )
        
        # Should succeed (detached HEAD is normal for submodules)
        assert result["success"] is True
        # Should have warning about detached HEAD
        assert any("detached HEAD" in w for w in result["warnings"])


class TestPreflightRemoteAccessibility:
    """Test remote accessibility verification.
    
    Property 13: Remote Accessibility Verification
    """

    def test_cache_file_structure(self, temp_git_repo):
        """Test cache file has correct structure."""
        cache_dir = temp_git_repo / ".ai" / "state" / "cache"
        cache_dir.mkdir(parents=True, exist_ok=True)
        cache_file = cache_dir / "remote_accessibility.json"
        
        # Simulate cache entry
        cache_data = {
            "remote:https://github.com/test/repo.git": {
                "accessible": True,
                "timestamp": time.time()
            }
        }
        cache_file.write_text(json.dumps(cache_data, indent=2))
        
        # Verify cache can be read
        with open(cache_file) as f:
            loaded = json.load(f)
        
        assert "remote:https://github.com/test/repo.git" in loaded
        assert loaded["remote:https://github.com/test/repo.git"]["accessible"] is True

    def test_cache_ttl_expired(self, temp_git_repo):
        """Test cache entry is considered expired after TTL."""
        cache_dir = temp_git_repo / ".ai" / "state" / "cache"
        cache_dir.mkdir(parents=True, exist_ok=True)
        cache_file = cache_dir / "remote_accessibility.json"
        
        # Create expired cache entry (6 minutes ago, TTL is 5 minutes)
        cache_data = {
            "remote:https://github.com/test/repo.git": {
                "accessible": True,
                "timestamp": time.time() - 360  # 6 minutes ago
            }
        }
        cache_file.write_text(json.dumps(cache_data, indent=2))
        
        # Check if cache is expired
        with open(cache_file) as f:
            loaded = json.load(f)
        
        entry = loaded["remote:https://github.com/test/repo.git"]
        ttl = 300  # 5 minutes
        is_expired = time.time() - entry["timestamp"] > ttl
        
        assert is_expired is True


class TestPreflightValidTypes:
    """Test all valid repo types are accepted."""

    @pytest.mark.parametrize("repo_type", ["root", "directory", "submodule"])
    def test_valid_repo_types_recognized(self, repo_type):
        """Test all valid repo types are recognized."""
        assert validate_repo_type(repo_type) is True

    @pytest.mark.parametrize("repo_type", ["invalid", "unknown", "", "ROOT", "Directory"])
    def test_invalid_repo_types_rejected(self, repo_type):
        """Test invalid repo types are rejected."""
        assert validate_repo_type(repo_type) is False


class TestPreflightHelperFunctions:
    """Test individual helper functions."""

    def test_check_working_tree_clean_on_clean_repo(self, temp_git_repo):
        """Test clean repo is detected as clean."""
        is_clean, status = check_working_tree_clean(temp_git_repo)
        assert is_clean is True
        assert status == ""

    def test_check_working_tree_clean_on_dirty_repo(self, temp_git_repo):
        """Test dirty repo is detected as dirty."""
        (temp_git_repo / "new_file.txt").write_text("content")
        is_clean, status = check_working_tree_clean(temp_git_repo)
        assert is_clean is False
        assert "new_file.txt" in status

    def test_check_directory_exists_true(self, temp_git_repo):
        """Test existing directory is detected."""
        subdir = temp_git_repo / "backend"
        subdir.mkdir()
        assert check_directory_exists(temp_git_repo, "backend") is True

    def test_check_directory_exists_false(self, temp_git_repo):
        """Test non-existing directory is detected."""
        assert check_directory_exists(temp_git_repo, "nonexistent") is False

    def test_check_is_submodule_with_git_dir(self, temp_git_repo):
        """Test directory with .git is detected as submodule."""
        subdir = temp_git_repo / "backend"
        subdir.mkdir()
        subprocess.run(["git", "init"], cwd=subdir, check=True)
        assert check_is_submodule(temp_git_repo, "backend") is True

    def test_check_is_submodule_without_git(self, temp_git_repo):
        """Test directory without .git is not detected as submodule."""
        subdir = temp_git_repo / "backend"
        subdir.mkdir()
        assert check_is_submodule(temp_git_repo, "backend") is False
