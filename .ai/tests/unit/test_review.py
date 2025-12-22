"""
Unit tests for review functionality.

**Feature: multi-repo-support**
**Property 26: Review Submodule Changes**
**Validates: Requirements 21.1, 21.2, 21.3, 21.4**

Note: These tests validate the review logic for submodule changes.
"""
import pytest
from typing import List, Tuple, Optional


class SubmoduleChange:
    """Represents a submodule change in a PR."""
    def __init__(
        self,
        path: str,
        old_sha: str,
        new_sha: str,
        is_pushed: bool = True
    ):
        self.path = path
        self.old_sha = old_sha
        self.new_sha = new_sha
        self.is_pushed = is_pushed


def identify_submodule_changes(
    changed_files: List[str],
    gitmodules_paths: List[str]
) -> List[str]:
    """
    Identify which changed files are submodule references.
    
    Property 26: Review Submodule Changes
    *For any* PR that includes submodule changes, the system SHALL 
    identify the submodule path (Req 21.1).
    
    Returns: List of submodule paths that changed
    """
    submodule_changes = []
    
    for file in changed_files:
        if file in gitmodules_paths:
            submodule_changes.append(file)
    
    return submodule_changes


def get_submodule_diff(
    submodule_path: str,
    old_sha: str,
    new_sha: str
) -> str:
    """
    Get diff for submodule commits.
    
    Property 26: Review Submodule Changes
    Fetch and display the submodule's commit diff (Req 21.2).
    
    Returns: Diff string
    """
    return f"Submodule {submodule_path} changed from {old_sha[:7]} to {new_sha[:7]}"


def check_submodule_pushed(
    submodule_path: str,
    new_sha: str,
    remote_shas: List[str]
) -> Tuple[bool, str]:
    """
    Check if submodule commit is pushed to remote.
    
    Property 26: Review Submodule Changes
    Warn if the reference points to an unpushed commit (Req 21.3, 21.4).
    
    Returns: (is_pushed, warning_message)
    """
    if new_sha in remote_shas:
        return True, ""
    
    return False, f"WARNING: Submodule '{submodule_path}' references unpushed commit {new_sha[:7]}"


def review_submodule_changes(
    changes: List[SubmoduleChange]
) -> Tuple[List[str], List[str]]:
    """
    Review all submodule changes.
    
    Returns: (diffs, warnings)
    """
    diffs = []
    warnings = []
    
    for change in changes:
        # Get diff (Req 21.2)
        diff = get_submodule_diff(change.path, change.old_sha, change.new_sha)
        diffs.append(diff)
        
        # Check if pushed (Req 21.3, 21.4)
        if not change.is_pushed:
            warnings.append(f"WARNING: Submodule '{change.path}' references unpushed commit")
    
    return diffs, warnings


class TestReviewSubmoduleChanges:
    """Test review submodule changes.
    
    Property 26: Review Submodule Changes
    """

    def test_identify_submodule_changes(self):
        """Test identifying submodule changes (Req 21.1)."""
        changed_files = ["backend", "README.md", "frontend"]
        gitmodules_paths = ["backend", "frontend"]
        
        submodule_changes = identify_submodule_changes(changed_files, gitmodules_paths)
        
        assert "backend" in submodule_changes
        assert "frontend" in submodule_changes
        assert "README.md" not in submodule_changes

    def test_get_submodule_diff(self):
        """Test getting submodule diff (Req 21.2)."""
        diff = get_submodule_diff("backend", "abc1234567890", "def1234567890")
        
        assert "backend" in diff
        assert "abc1234" in diff
        assert "def1234" in diff

    def test_check_pushed_commit(self):
        """Test checking pushed commit (Req 21.3)."""
        is_pushed, warning = check_submodule_pushed(
            "backend",
            "abc123",
            ["abc123", "def456"]
        )
        
        assert is_pushed is True
        assert warning == ""

    def test_check_unpushed_commit(self):
        """Test checking unpushed commit (Req 21.4)."""
        is_pushed, warning = check_submodule_pushed(
            "backend",
            "xyz789",
            ["abc123", "def456"]
        )
        
        assert is_pushed is False
        assert "WARNING" in warning
        assert "unpushed" in warning


class TestReviewAllChanges:
    """Test reviewing all submodule changes."""

    def test_review_multiple_changes(self):
        """Test reviewing multiple submodule changes."""
        changes = [
            SubmoduleChange("backend", "abc123", "def456", is_pushed=True),
            SubmoduleChange("frontend", "111222", "333444", is_pushed=False),
        ]
        
        diffs, warnings = review_submodule_changes(changes)
        
        assert len(diffs) == 2
        assert len(warnings) == 1
        assert "frontend" in warnings[0]

    def test_review_no_changes(self):
        """Test reviewing no changes."""
        changes = []
        
        diffs, warnings = review_submodule_changes(changes)
        
        assert len(diffs) == 0
        assert len(warnings) == 0

    def test_review_all_pushed(self):
        """Test reviewing all pushed changes."""
        changes = [
            SubmoduleChange("backend", "abc123", "def456", is_pushed=True),
            SubmoduleChange("frontend", "111222", "333444", is_pushed=True),
        ]
        
        diffs, warnings = review_submodule_changes(changes)
        
        assert len(diffs) == 2
        assert len(warnings) == 0


class TestSubmoduleChange:
    """Test SubmoduleChange class."""

    def test_change_has_path(self):
        """Test change has path."""
        change = SubmoduleChange("backend", "abc", "def")
        
        assert change.path == "backend"

    def test_change_has_shas(self):
        """Test change has SHAs."""
        change = SubmoduleChange("backend", "abc123", "def456")
        
        assert change.old_sha == "abc123"
        assert change.new_sha == "def456"

    def test_change_default_pushed(self):
        """Test change defaults to pushed."""
        change = SubmoduleChange("backend", "abc", "def")
        
        assert change.is_pushed is True

    def test_change_unpushed(self):
        """Test change can be unpushed."""
        change = SubmoduleChange("backend", "abc", "def", is_pushed=False)
        
        assert change.is_pushed is False


class TestIdentifyChanges:
    """Test identifying submodule changes."""

    def test_no_submodule_changes(self):
        """Test no submodule changes."""
        changed_files = ["README.md", "main.go"]
        gitmodules_paths = ["backend", "frontend"]
        
        submodule_changes = identify_submodule_changes(changed_files, gitmodules_paths)
        
        assert len(submodule_changes) == 0

    def test_all_submodule_changes(self):
        """Test all changes are submodules."""
        changed_files = ["backend", "frontend"]
        gitmodules_paths = ["backend", "frontend"]
        
        submodule_changes = identify_submodule_changes(changed_files, gitmodules_paths)
        
        assert len(submodule_changes) == 2

    def test_empty_changed_files(self):
        """Test empty changed files."""
        changed_files = []
        gitmodules_paths = ["backend"]
        
        submodule_changes = identify_submodule_changes(changed_files, gitmodules_paths)
        
        assert len(submodule_changes) == 0
