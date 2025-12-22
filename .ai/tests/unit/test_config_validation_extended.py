"""
Extended unit tests for config validation functionality.

**Feature: multi-repo-support**
**Property 7: Config Validation Completeness**
**Property 15: Path Traversal Prevention**
**Property 18: Submodule Remote Validation**
**Validates: Requirements 1.5, 1.6, 9.1-9.5, 22.1-22.4, 26.1-26.4**

Note: These tests extend the existing test_validate_config.py with multi-repo specific tests.
"""
import pytest
from pathlib import Path


def validate_repo_path(repo_type: str, repo_path: str) -> tuple:
    """
    Validate repo path based on type.
    
    Property 15: Path Traversal Prevention
    *For any* repo path or WORK_DIR, the system SHALL reject paths 
    containing `..` or paths that resolve outside the worktree.
    
    Returns: (is_valid, error_message)
    """
    # Check for path traversal (Req 22.1, 22.2)
    if ".." in repo_path:
        return False, f"Path traversal detected: '{repo_path}' contains '..'"
    
    # Normalize path
    normalized = repo_path.replace("\\", "/").rstrip("/")
    
    # Root type must be ./ or . (Req 9.3)
    if repo_type == "root":
        if normalized not in (".", "./", ""):
            return False, f"Root type path must be './' or '.', got '{repo_path}'"
    
    return True, ""


def validate_submodule_config(
    repo_path: str,
    gitmodules_content: str,
    has_git: bool
) -> tuple:
    """
    Validate submodule configuration.
    
    Property 7: Config Validation Completeness
    *For any* workflow.yaml with submodule-type repos, validation SHALL verify:
    - .gitmodules exists (Req 1.5)
    - repo path is listed in .gitmodules (Req 9.1)
    - repo path has .git file/directory (Req 9.2)
    
    Returns: (is_valid, list_of_errors)
    """
    errors = []
    
    # Check .gitmodules exists (Req 1.5)
    if not gitmodules_content:
        errors.append("Missing .gitmodules file for submodule type repo")
        return False, errors
    
    # Check path is in .gitmodules (Req 9.1)
    # Simple check: look for path = <repo_path>
    normalized_path = repo_path.rstrip("/")
    if f"path = {normalized_path}" not in gitmodules_content:
        errors.append(f"Submodule path '{repo_path}' not found in .gitmodules")
    
    # Check .git exists (Req 9.2)
    if not has_git:
        errors.append(f"Submodule path '{repo_path}' has no .git file/directory")
    
    return len(errors) == 0, errors


def check_directory_has_git_file(repo_path: str, has_git_file: bool) -> tuple:
    """
    Check if directory type path has .git file (might be submodule).
    
    Returns: (has_warning, warning_message)
    """
    if has_git_file:
        return True, f"WARNING: Directory '{repo_path}' has .git file - might be a submodule"
    return False, ""


def validate_submodule_remote(
    repo_path: str,
    gitmodules_url: str,
    actual_remote_url: str
) -> tuple:
    """
    Validate submodule remote URL matches .gitmodules.
    
    Property 18: Submodule Remote Validation
    *For any* submodule initialization or push, the system SHALL verify 
    the remote URL matches the definition in .gitmodules.
    
    Returns: (is_valid, error_message)
    """
    if not gitmodules_url:
        return False, "No URL found in .gitmodules for submodule"
    
    if not actual_remote_url:
        return False, "No remote URL configured for submodule"
    
    # Normalize URLs for comparison
    gitmodules_url = gitmodules_url.rstrip("/").rstrip(".git")
    actual_remote_url = actual_remote_url.rstrip("/").rstrip(".git")
    
    if gitmodules_url != actual_remote_url:
        return False, f"Remote URL mismatch: .gitmodules has '{gitmodules_url}', actual is '{actual_remote_url}'"
    
    return True, ""


class TestPathTraversalPrevention:
    """Test path traversal prevention.
    
    Property 15: Path Traversal Prevention
    """

    def test_reject_double_dot_path(self):
        """Test paths with .. are rejected (Req 22.1)."""
        is_valid, error = validate_repo_path("directory", "../outside")
        
        assert is_valid is False
        assert "Path traversal" in error

    def test_reject_embedded_double_dot(self):
        """Test paths with embedded .. are rejected (Req 22.2)."""
        is_valid, error = validate_repo_path("directory", "backend/../../../etc")
        
        assert is_valid is False
        assert "Path traversal" in error

    def test_accept_valid_path(self):
        """Test valid paths are accepted."""
        is_valid, error = validate_repo_path("directory", "backend/internal")
        
        assert is_valid is True
        assert error == ""

    def test_root_type_requires_dot_path(self):
        """Test root type requires ./ or . path (Req 9.3)."""
        is_valid, error = validate_repo_path("root", "backend")
        
        assert is_valid is False
        assert "Root type path must be" in error

    def test_root_type_accepts_dot(self):
        """Test root type accepts . path."""
        is_valid, error = validate_repo_path("root", ".")
        
        assert is_valid is True

    def test_root_type_accepts_dot_slash(self):
        """Test root type accepts ./ path."""
        is_valid, error = validate_repo_path("root", "./")
        
        assert is_valid is True


class TestSubmoduleConfigValidation:
    """Test submodule configuration validation.
    
    Property 7: Config Validation Completeness
    """

    def test_missing_gitmodules(self):
        """Test missing .gitmodules is detected (Req 1.5)."""
        is_valid, errors = validate_submodule_config(
            repo_path="backend",
            gitmodules_content="",
            has_git=True
        )
        
        assert is_valid is False
        assert any("Missing .gitmodules" in e for e in errors)

    def test_path_not_in_gitmodules(self):
        """Test path not in .gitmodules is detected (Req 9.1)."""
        gitmodules = """[submodule "frontend"]
    path = frontend
    url = https://github.com/test/frontend.git
"""
        is_valid, errors = validate_submodule_config(
            repo_path="backend",
            gitmodules_content=gitmodules,
            has_git=True
        )
        
        assert is_valid is False
        assert any("not found in .gitmodules" in e for e in errors)

    def test_missing_git_directory(self):
        """Test missing .git is detected (Req 9.2)."""
        gitmodules = """[submodule "backend"]
    path = backend
    url = https://github.com/test/backend.git
"""
        is_valid, errors = validate_submodule_config(
            repo_path="backend",
            gitmodules_content=gitmodules,
            has_git=False
        )
        
        assert is_valid is False
        assert any("no .git" in e for e in errors)

    def test_valid_submodule_config(self):
        """Test valid submodule config passes."""
        gitmodules = """[submodule "backend"]
    path = backend
    url = https://github.com/test/backend.git
"""
        is_valid, errors = validate_submodule_config(
            repo_path="backend",
            gitmodules_content=gitmodules,
            has_git=True
        )
        
        assert is_valid is True
        assert len(errors) == 0


class TestDirectoryGitFileWarning:
    """Test directory type .git file warning."""

    def test_warn_directory_with_git_file(self):
        """Test warning when directory has .git file (Req 9.4)."""
        has_warning, warning = check_directory_has_git_file("backend", has_git_file=True)
        
        assert has_warning is True
        assert "WARNING" in warning
        assert "might be a submodule" in warning

    def test_no_warning_without_git_file(self):
        """Test no warning when directory has no .git file."""
        has_warning, warning = check_directory_has_git_file("backend", has_git_file=False)
        
        assert has_warning is False
        assert warning == ""


class TestSubmoduleRemoteValidation:
    """Test submodule remote URL validation.
    
    Property 18: Submodule Remote Validation
    """

    def test_matching_urls(self):
        """Test matching URLs pass validation (Req 26.1)."""
        is_valid, error = validate_submodule_remote(
            repo_path="backend",
            gitmodules_url="https://github.com/test/backend.git",
            actual_remote_url="https://github.com/test/backend.git"
        )
        
        assert is_valid is True
        assert error == ""

    def test_matching_urls_without_git_suffix(self):
        """Test URLs match even without .git suffix."""
        is_valid, error = validate_submodule_remote(
            repo_path="backend",
            gitmodules_url="https://github.com/test/backend.git",
            actual_remote_url="https://github.com/test/backend"
        )
        
        assert is_valid is True

    def test_mismatched_urls(self):
        """Test mismatched URLs fail validation (Req 26.2)."""
        is_valid, error = validate_submodule_remote(
            repo_path="backend",
            gitmodules_url="https://github.com/test/backend.git",
            actual_remote_url="https://github.com/other/backend.git"
        )
        
        assert is_valid is False
        assert "mismatch" in error

    def test_missing_gitmodules_url(self):
        """Test missing .gitmodules URL fails (Req 26.3)."""
        is_valid, error = validate_submodule_remote(
            repo_path="backend",
            gitmodules_url="",
            actual_remote_url="https://github.com/test/backend.git"
        )
        
        assert is_valid is False
        assert "No URL found in .gitmodules" in error

    def test_missing_actual_remote(self):
        """Test missing actual remote fails (Req 26.4)."""
        is_valid, error = validate_submodule_remote(
            repo_path="backend",
            gitmodules_url="https://github.com/test/backend.git",
            actual_remote_url=""
        )
        
        assert is_valid is False
        assert "No remote URL configured" in error


class TestValidRepoTypes:
    """Test validation for all repo types."""

    @pytest.mark.parametrize("repo_type,repo_path,expected_valid", [
        ("root", "./", True),
        ("root", ".", True),
        ("root", "backend", False),
        ("directory", "backend", True),
        ("directory", "libs/shared", True),
        ("directory", "../outside", False),
        ("submodule", "backend", True),
        ("submodule", "libs/shared", True),
        ("submodule", "../outside", False),
    ])
    def test_path_validation_by_type(self, repo_type, repo_path, expected_valid):
        """Test path validation for different repo types."""
        is_valid, error = validate_repo_path(repo_type, repo_path)
        assert is_valid == expected_valid
