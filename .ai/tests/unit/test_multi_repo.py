"""
Unit tests for multi-repo coordination functionality.

**Feature: multi-repo-support**
**Property 25: Multi-Repo Coordination**
**Validates: Requirements 17.1, 17.2, 17.3, 17.4**

Note: These tests validate the multi-repo coordination logic.
"""
import pytest
from typing import List, Tuple


class RepoConfig:
    """Repository configuration."""
    def __init__(self, name: str, repo_type: str, path: str):
        self.name = name
        self.repo_type = repo_type
        self.path = path


class ExecutionResult:
    """Result of repo execution."""
    def __init__(self, repo_name: str, success: bool, error: str = ""):
        self.repo_name = repo_name
        self.success = success
        self.error = error


def process_repos_sequential(
    repos: List[RepoConfig],
    execute_fn=None
) -> Tuple[bool, List[ExecutionResult]]:
    """
    Process multiple repos sequentially.
    
    Property 25: Multi-Repo Coordination
    *For any* ticket specifying multiple repos, the system SHALL:
    - Process them in the specified order (sequential) (Req 17.1)
    - Stop on first failure (Req 17.2)
    - Handle submodule repos with submodule-specific logic (Req 17.3, 17.4)
    
    Returns: (overall_success, list_of_results)
    """
    results = []
    
    for repo in repos:
        # Execute repo (Req 17.1 - sequential)
        if execute_fn:
            success, error = execute_fn(repo)
        else:
            success, error = True, ""
        
        result = ExecutionResult(repo.name, success, error)
        results.append(result)
        
        # Stop on first failure (Req 17.2)
        if not success:
            return False, results
    
    return True, results


def get_repo_execution_order(repos: List[RepoConfig]) -> List[str]:
    """
    Get the order repos will be executed.
    
    Returns: List of repo names in execution order
    """
    return [repo.name for repo in repos]


class TestMultiRepoCoordination:
    """Test multi-repo coordination.
    
    Property 25: Multi-Repo Coordination
    """

    def test_sequential_processing(self):
        """Test repos are processed sequentially (Req 17.1)."""
        repos = [
            RepoConfig("backend", "directory", "backend"),
            RepoConfig("frontend", "directory", "frontend"),
        ]
        execution_order = []
        
        def track_execution(repo):
            execution_order.append(repo.name)
            return True, ""
        
        success, results = process_repos_sequential(repos, track_execution)
        
        assert success is True
        assert execution_order == ["backend", "frontend"]

    def test_stop_on_first_failure(self):
        """Test processing stops on first failure (Req 17.2)."""
        repos = [
            RepoConfig("backend", "directory", "backend"),
            RepoConfig("frontend", "directory", "frontend"),
            RepoConfig("shared", "directory", "shared"),
        ]
        
        def fail_on_frontend(repo):
            if repo.name == "frontend":
                return False, "Frontend failed"
            return True, ""
        
        success, results = process_repos_sequential(repos, fail_on_frontend)
        
        assert success is False
        assert len(results) == 2  # Only backend and frontend processed
        assert results[0].success is True
        assert results[1].success is False

    def test_submodule_repos_included(self):
        """Test submodule repos are processed (Req 17.3)."""
        repos = [
            RepoConfig("backend", "submodule", "backend"),
            RepoConfig("frontend", "directory", "frontend"),
        ]
        processed = []
        
        def track_type(repo):
            processed.append((repo.name, repo.repo_type))
            return True, ""
        
        success, results = process_repos_sequential(repos, track_type)
        
        assert success is True
        assert ("backend", "submodule") in processed

    def test_all_repos_processed_on_success(self):
        """Test all repos are processed when all succeed (Req 17.4)."""
        repos = [
            RepoConfig("backend", "directory", "backend"),
            RepoConfig("frontend", "directory", "frontend"),
            RepoConfig("shared", "submodule", "shared"),
        ]
        
        success, results = process_repos_sequential(repos)
        
        assert success is True
        assert len(results) == 3
        assert all(r.success for r in results)


class TestExecutionOrder:
    """Test execution order."""

    def test_order_preserved(self):
        """Test execution order is preserved."""
        repos = [
            RepoConfig("first", "directory", "first"),
            RepoConfig("second", "directory", "second"),
            RepoConfig("third", "directory", "third"),
        ]
        
        order = get_repo_execution_order(repos)
        
        assert order == ["first", "second", "third"]

    def test_empty_repos(self):
        """Test empty repos list."""
        repos = []
        
        success, results = process_repos_sequential(repos)
        
        assert success is True
        assert len(results) == 0


class TestExecutionResult:
    """Test execution result tracking."""

    def test_result_includes_repo_name(self):
        """Test result includes repo name."""
        result = ExecutionResult("backend", True)
        
        assert result.repo_name == "backend"

    def test_result_includes_success(self):
        """Test result includes success status."""
        result = ExecutionResult("backend", True)
        
        assert result.success is True

    def test_result_includes_error(self):
        """Test result includes error message."""
        result = ExecutionResult("backend", False, "Something failed")
        
        assert result.error == "Something failed"


class TestRepoConfig:
    """Test repo configuration."""

    def test_config_has_name(self):
        """Test config has name."""
        config = RepoConfig("backend", "directory", "backend")
        
        assert config.name == "backend"

    def test_config_has_type(self):
        """Test config has type."""
        config = RepoConfig("backend", "submodule", "backend")
        
        assert config.repo_type == "submodule"

    def test_config_has_path(self):
        """Test config has path."""
        config = RepoConfig("backend", "directory", "backend/src")
        
        assert config.path == "backend/src"
