"""
Unit tests for result recording functionality.

**Feature: multi-repo-support**
**Property 10: Result Recording Completeness**
**Validates: Requirements 11.1, 11.2, 11.3, 11.4, 11.5, 11.6, 24.3, 24.4, 24.5**

Note: These tests validate the result recording logic.
"""
import pytest
import json
from datetime import datetime, timezone
from typing import Optional


def build_result(
    issue_id: str,
    status: str,
    repo: str,
    repo_type: str,
    branch: str,
    base_branch: str,
    head_sha: str,
    work_dir: str,
    pr_url: str = "",
    submodule_sha: str = "",
    consistency_status: str = "consistent",
    failure_stage: str = "",
    recovery_command: str = "",
    duration_seconds: int = 0,
    retry_count: int = 0
) -> dict:
    """
    Build result record with all required fields.
    
    Property 10: Result Recording Completeness
    *For any* execution result, the result.json SHALL contain:
    - repo_type field (Req 11.1)
    - work_dir field (Req 11.2)
    - For submodule: submodule_sha and parent_sha (Req 11.3, 11.4)
    - On failure: failure_stage (Req 11.5, 11.6)
    - consistency_status (Req 24.3)
    - recovery_command when inconsistent (Req 24.4, 24.5)
    """
    result = {
        "issue_id": issue_id,
        "status": status,
        "repo": repo,
        "repo_type": repo_type,  # Req 11.1
        "branch": branch,
        "base_branch": base_branch,
        "head_sha": head_sha,
        "work_dir": work_dir,  # Req 11.2
        "timestamp_utc": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "pr_url": pr_url,
        "metrics": {
            "duration_seconds": duration_seconds,
            "retry_count": retry_count
        }
    }
    
    # Submodule-specific fields (Req 11.3, 11.4)
    if repo_type == "submodule":
        result["submodule_sha"] = submodule_sha
        result["consistency_status"] = consistency_status  # Req 24.3
    
    # Failure-specific fields (Req 11.5, 11.6)
    if status == "failed":
        result["failure_stage"] = failure_stage
    
    # Recovery command for inconsistent state (Req 24.4, 24.5)
    if consistency_status != "consistent" and recovery_command:
        result["recovery_command"] = recovery_command
    
    return result


def validate_result(result: dict) -> tuple:
    """
    Validate result record has all required fields.
    
    Returns: (is_valid, list_of_missing_fields)
    """
    required_fields = [
        "issue_id",
        "status",
        "repo",
        "repo_type",
        "branch",
        "base_branch",
        "head_sha",
        "work_dir",
        "timestamp_utc",
        "metrics"
    ]
    
    missing = [f for f in required_fields if f not in result]
    
    # Check submodule-specific fields
    if result.get("repo_type") == "submodule":
        if "submodule_sha" not in result:
            missing.append("submodule_sha")
        if "consistency_status" not in result:
            missing.append("consistency_status")
    
    # Check failure-specific fields
    if result.get("status") == "failed":
        if "failure_stage" not in result:
            missing.append("failure_stage")
    
    return (len(missing) == 0, missing)


class TestResultRecordingCompleteness:
    """Test result recording completeness.
    
    Property 10: Result Recording Completeness
    """

    def test_result_includes_repo_type(self):
        """Test result.json includes repo_type field (Req 11.1)."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="backend",
            repo_type="directory",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend"
        )
        
        assert "repo_type" in result
        assert result["repo_type"] == "directory"

    def test_result_includes_work_dir(self):
        """Test result.json includes work_dir field (Req 11.2)."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="backend",
            repo_type="directory",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend"
        )
        
        assert "work_dir" in result
        assert result["work_dir"] == "/worktree/backend"

    def test_result_includes_submodule_sha(self):
        """Test result.json includes submodule_sha for submodule type (Req 11.3, 11.4)."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="backend",
            repo_type="submodule",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend",
            submodule_sha="def456"
        )
        
        assert "submodule_sha" in result
        assert result["submodule_sha"] == "def456"

    def test_result_includes_failure_stage(self):
        """Test result.json includes failure_stage on failure (Req 11.5, 11.6)."""
        result = build_result(
            issue_id="1",
            status="failed",
            repo="backend",
            repo_type="directory",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend",
            failure_stage="git_commit"
        )
        
        assert "failure_stage" in result
        assert result["failure_stage"] == "git_commit"

    def test_result_includes_consistency_status(self):
        """Test result.json includes consistency_status for submodule (Req 24.3)."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="backend",
            repo_type="submodule",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend",
            submodule_sha="def456",
            consistency_status="consistent"
        )
        
        assert "consistency_status" in result
        assert result["consistency_status"] == "consistent"

    def test_result_includes_recovery_command(self):
        """Test result.json includes recovery_command when inconsistent (Req 24.4, 24.5)."""
        result = build_result(
            issue_id="1",
            status="failed",
            repo="backend",
            repo_type="submodule",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend",
            submodule_sha="def456",
            consistency_status="parent_push_failed_submodule_pushed",
            failure_stage="git_push",
            recovery_command="git -C backend reset --hard HEAD~1 && git push -f origin feat/ai-issue-1"
        )
        
        assert "recovery_command" in result
        assert "reset --hard" in result["recovery_command"]


class TestResultValidation:
    """Test result validation function."""

    def test_valid_root_result(self):
        """Test valid root type result passes validation."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="root",
            repo_type="root",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree"
        )
        
        is_valid, missing = validate_result(result)
        assert is_valid is True
        assert len(missing) == 0

    def test_valid_directory_result(self):
        """Test valid directory type result passes validation."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="backend",
            repo_type="directory",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend"
        )
        
        is_valid, missing = validate_result(result)
        assert is_valid is True
        assert len(missing) == 0

    def test_valid_submodule_result(self):
        """Test valid submodule type result passes validation."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="backend",
            repo_type="submodule",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend",
            submodule_sha="def456"
        )
        
        is_valid, missing = validate_result(result)
        assert is_valid is True
        assert len(missing) == 0

    def test_invalid_submodule_missing_sha(self):
        """Test submodule result without submodule_sha fails validation."""
        result = {
            "issue_id": "1",
            "status": "success",
            "repo": "backend",
            "repo_type": "submodule",
            "branch": "feat/ai-issue-1",
            "base_branch": "develop",
            "head_sha": "abc123",
            "work_dir": "/worktree/backend",
            "timestamp_utc": "2025-01-01T00:00:00Z",
            "metrics": {}
        }
        
        is_valid, missing = validate_result(result)
        assert is_valid is False
        assert "submodule_sha" in missing

    def test_invalid_failed_missing_stage(self):
        """Test failed result without failure_stage fails validation."""
        result = {
            "issue_id": "1",
            "status": "failed",
            "repo": "backend",
            "repo_type": "directory",
            "branch": "feat/ai-issue-1",
            "base_branch": "develop",
            "head_sha": "abc123",
            "work_dir": "/worktree/backend",
            "timestamp_utc": "2025-01-01T00:00:00Z",
            "metrics": {}
        }
        
        is_valid, missing = validate_result(result)
        assert is_valid is False
        assert "failure_stage" in missing


class TestResultMetrics:
    """Test result metrics recording."""

    def test_result_includes_duration(self):
        """Test result includes duration_seconds in metrics."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="backend",
            repo_type="directory",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend",
            duration_seconds=120
        )
        
        assert "metrics" in result
        assert result["metrics"]["duration_seconds"] == 120

    def test_result_includes_retry_count(self):
        """Test result includes retry_count in metrics."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="backend",
            repo_type="directory",
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/backend",
            retry_count=2
        )
        
        assert result["metrics"]["retry_count"] == 2


class TestResultAllRepoTypes:
    """Test result recording for all repo types."""

    @pytest.mark.parametrize("repo_type", ["root", "directory", "submodule"])
    def test_result_has_repo_type_field(self, repo_type):
        """Test all repo types have repo_type field."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="test",
            repo_type=repo_type,
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree",
            submodule_sha="def456" if repo_type == "submodule" else ""
        )
        
        assert result["repo_type"] == repo_type

    @pytest.mark.parametrize("repo_type", ["root", "directory", "submodule"])
    def test_result_has_work_dir_field(self, repo_type):
        """Test all repo types have work_dir field."""
        result = build_result(
            issue_id="1",
            status="success",
            repo="test",
            repo_type=repo_type,
            branch="feat/ai-issue-1",
            base_branch="develop",
            head_sha="abc123",
            work_dir="/worktree/test",
            submodule_sha="def456" if repo_type == "submodule" else ""
        )
        
        assert "work_dir" in result
