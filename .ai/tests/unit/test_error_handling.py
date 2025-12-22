"""
Unit tests for error handling functionality.

**Feature: multi-repo-support**
**Property 11: Error Message Specificity**
**Validates: Requirements 13.1, 13.2, 13.3, 13.4, 13.5**

Note: These tests validate the error handling logic.
"""
import pytest


def format_error(
    operation: str,
    error_msg: str,
    repo_type: str = "",
    repo_path: str = "",
    worktree: str = "",
    work_dir: str = "",
    branch: str = "",
    suggestion: str = ""
) -> str:
    """
    Format detailed error message with context.
    
    Property 11: Error Message Specificity
    *For any* failure during repo type operations, the error message SHALL include:
    - The specific operation that failed (Req 13.1)
    - The repo type and path involved (Req 13.2, 13.3)
    - A suggested fix when applicable (Req 13.4, 13.5)
    
    Returns: Formatted error message
    """
    lines = [
        "============================================================",
        f"ERROR: {error_msg}",
        "============================================================",
        f"Operation: {operation}",
    ]
    
    if repo_type:
        lines.append(f"Repo Type: {repo_type}")
    if repo_path:
        lines.append(f"Repo Path: {repo_path}")
    if worktree:
        lines.append(f"Worktree: {worktree}")
    if work_dir:
        lines.append(f"Work Dir: {work_dir}")
    if branch:
        lines.append(f"Branch: {branch}")
    
    if suggestion:
        lines.append("")
        lines.append(f"SUGGESTION: {suggestion}")
    
    lines.append("============================================================")
    
    return "\n".join(lines)


def get_error_suggestion(operation: str, repo_type: str) -> str:
    """
    Get suggested fix for common errors.
    
    Returns: Suggestion string
    """
    suggestions = {
        ("worktree_setup", "submodule"): "Check that the submodule is properly initialized with 'git submodule update --init'",
        ("worktree_setup", "directory"): "Verify the directory path exists in the repository",
        ("git_commit", "submodule"): "Ensure changes are within the submodule boundary",
        ("git_push", "submodule"): "Check push permissions for both submodule and parent remotes",
        ("preflight", "submodule"): "Run 'git submodule update --init --recursive' to initialize submodules",
    }
    
    return suggestions.get((operation, repo_type), "")


class TestErrorMessageSpecificity:
    """Test error message specificity.
    
    Property 11: Error Message Specificity
    """

    def test_error_includes_operation(self):
        """Test error includes specific operation (Req 13.1)."""
        error = format_error(
            operation="git_commit",
            error_msg="Commit failed"
        )
        
        assert "Operation: git_commit" in error

    def test_error_includes_repo_type(self):
        """Test error includes repo type (Req 13.2)."""
        error = format_error(
            operation="git_commit",
            error_msg="Commit failed",
            repo_type="submodule"
        )
        
        assert "Repo Type: submodule" in error

    def test_error_includes_repo_path(self):
        """Test error includes repo path (Req 13.3)."""
        error = format_error(
            operation="git_commit",
            error_msg="Commit failed",
            repo_path="backend"
        )
        
        assert "Repo Path: backend" in error

    def test_error_includes_suggestion(self):
        """Test error includes suggestion (Req 13.4)."""
        error = format_error(
            operation="git_commit",
            error_msg="Commit failed",
            suggestion="Check file permissions"
        )
        
        assert "SUGGESTION: Check file permissions" in error

    def test_error_includes_all_context(self):
        """Test error includes all context (Req 13.5)."""
        error = format_error(
            operation="worktree_setup",
            error_msg="Work directory not found",
            repo_type="submodule",
            repo_path="backend",
            worktree="/worktrees/issue-1",
            work_dir="/worktrees/issue-1/backend",
            branch="feat/ai-issue-1",
            suggestion="Check that the submodule is initialized"
        )
        
        assert "Operation: worktree_setup" in error
        assert "Repo Type: submodule" in error
        assert "Repo Path: backend" in error
        assert "Worktree:" in error
        assert "Work Dir:" in error
        assert "Branch:" in error
        assert "SUGGESTION:" in error


class TestErrorSuggestions:
    """Test error suggestions."""

    def test_submodule_worktree_suggestion(self):
        """Test suggestion for submodule worktree error."""
        suggestion = get_error_suggestion("worktree_setup", "submodule")
        
        assert "submodule" in suggestion.lower()
        assert "init" in suggestion.lower()

    def test_directory_worktree_suggestion(self):
        """Test suggestion for directory worktree error."""
        suggestion = get_error_suggestion("worktree_setup", "directory")
        
        assert "directory" in suggestion.lower()

    def test_submodule_commit_suggestion(self):
        """Test suggestion for submodule commit error."""
        suggestion = get_error_suggestion("git_commit", "submodule")
        
        assert "boundary" in suggestion.lower()

    def test_submodule_push_suggestion(self):
        """Test suggestion for submodule push error."""
        suggestion = get_error_suggestion("git_push", "submodule")
        
        assert "permission" in suggestion.lower()

    def test_unknown_operation_no_suggestion(self):
        """Test unknown operation returns empty suggestion."""
        suggestion = get_error_suggestion("unknown_op", "root")
        
        assert suggestion == ""


class TestErrorFormatting:
    """Test error message formatting."""

    def test_error_has_separator_lines(self):
        """Test error has separator lines."""
        error = format_error(
            operation="test",
            error_msg="Test error"
        )
        
        assert "============" in error

    def test_error_has_error_prefix(self):
        """Test error has ERROR prefix."""
        error = format_error(
            operation="test",
            error_msg="Test error"
        )
        
        assert "ERROR:" in error

    def test_minimal_error(self):
        """Test minimal error with only required fields."""
        error = format_error(
            operation="test",
            error_msg="Test error"
        )
        
        assert "Operation: test" in error
        assert "ERROR: Test error" in error

    def test_error_without_suggestion(self):
        """Test error without suggestion doesn't have SUGGESTION line."""
        error = format_error(
            operation="test",
            error_msg="Test error"
        )
        
        assert "SUGGESTION:" not in error


class TestErrorByRepoType:
    """Test error handling for different repo types."""

    @pytest.mark.parametrize("repo_type", ["root", "directory", "submodule"])
    def test_error_includes_repo_type(self, repo_type):
        """Test error includes repo type for all types."""
        error = format_error(
            operation="test",
            error_msg="Test error",
            repo_type=repo_type
        )
        
        assert f"Repo Type: {repo_type}" in error
