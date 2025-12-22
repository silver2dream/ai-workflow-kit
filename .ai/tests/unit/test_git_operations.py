"""
Unit tests for git operations functionality.

**Feature: multi-repo-support**
**Property 5: Submodule Git Operations Order**
**Property 14: Submodule File Boundary**
**Property 16: Submodule Consistency Tracking**
**Property 22: Directory Type Git Operations**
**Validates: Requirements 5.1-5.4, 6.1-6.5, 20.1-20.4, 24.1-24.5**

Note: These tests validate the git operations logic using Python implementations
that mirror the bash script behavior.
"""
import pytest
import subprocess
import os
from pathlib import Path
from typing import Tuple, List


# ============================================================
# Submodule Git Operations Functions (mirroring run_issue_codex.sh)
# ============================================================

class SubmoduleState:
    """Track submodule operation state."""
    def __init__(self):
        self.submodule_sha = ""
        self.parent_sha = ""
        self.consistency_status = "consistent"
        self.operations_log: List[str] = []


def check_submodule_boundary(
    wt_dir: Path, 
    submodule_path: str, 
    allow_parent: bool = False
) -> Tuple[bool, List[str]]:
    """
    Check if all staged changes are within submodule boundary.
    
    Property 14: Submodule File Boundary
    *For any* commit in a submodule-type repo, all changed files SHALL be 
    within the submodule path (unless explicitly overridden).
    
    Returns: (is_valid, list_of_outside_files)
    """
    result = subprocess.run(
        ["git", "diff", "--cached", "--name-only"],
        cwd=str(wt_dir),
        capture_output=True,
        text=True
    )
    
    changed_files = [f for f in result.stdout.strip().split('\n') if f]
    outside_files = []
    
    # Normalize submodule path
    submodule_path = submodule_path.rstrip('/')
    
    for file in changed_files:
        # Check if file is within submodule path
        if not file.startswith(f"{submodule_path}/") and file != submodule_path:
            outside_files.append(file)
    
    if outside_files and not allow_parent:
        return False, outside_files
    
    return True, outside_files


def git_commit_submodule(
    wt_dir: Path,
    submodule_path: str,
    commit_msg: str,
    state: SubmoduleState
) -> Tuple[bool, str]:
    """
    Commit changes in submodule first, then update parent reference.
    
    Property 5: Submodule Git Operations Order
    *For any* submodule-type repo, git operations SHALL follow this order:
    1. Commit in submodule first
    2. Update parent's submodule reference
    
    Returns: (success, error_message)
    """
    submodule_dir = wt_dir / submodule_path
    
    # Step 1: Stage and commit in submodule first
    state.operations_log.append("submodule_stage")
    subprocess.run(["git", "add", "-A"], cwd=str(submodule_dir), check=True)
    
    # Check if there are changes
    result = subprocess.run(
        ["git", "diff", "--cached", "--quiet"],
        cwd=str(submodule_dir),
        capture_output=True
    )
    if result.returncode == 0:
        return False, "no changes in submodule"
    
    state.operations_log.append("submodule_commit")
    result = subprocess.run(
        ["git", "commit", "-m", commit_msg],
        cwd=str(submodule_dir),
        capture_output=True,
        text=True
    )
    if result.returncode != 0:
        state.consistency_status = "submodule_commit_failed"
        return False, f"submodule commit failed: {result.stderr}"
    
    # Record submodule SHA
    result = subprocess.run(
        ["git", "rev-parse", "HEAD"],
        cwd=str(submodule_dir),
        capture_output=True,
        text=True,
        check=True
    )
    state.submodule_sha = result.stdout.strip()
    
    # Step 2: Update parent's submodule reference
    state.operations_log.append("parent_stage_submodule_ref")
    subprocess.run(["git", "add", submodule_path], cwd=str(wt_dir), check=True)
    
    state.operations_log.append("parent_commit")
    result = subprocess.run(
        ["git", "commit", "-m", commit_msg],
        cwd=str(wt_dir),
        capture_output=True,
        text=True
    )
    if result.returncode != 0:
        state.consistency_status = "submodule_committed_parent_failed"
        return False, f"parent commit failed: {result.stderr}"
    
    # Record parent SHA
    result = subprocess.run(
        ["git", "rev-parse", "HEAD"],
        cwd=str(wt_dir),
        capture_output=True,
        text=True,
        check=True
    )
    state.parent_sha = result.stdout.strip()
    
    return True, ""


def verify_commit_order(operations_log: List[str]) -> bool:
    """
    Verify that operations happened in correct order.
    
    Property 5: Submodule Git Operations Order
    Expected order: submodule operations before parent operations
    """
    submodule_ops = ["submodule_stage", "submodule_commit"]
    parent_ops = ["parent_stage_submodule_ref", "parent_commit"]
    
    # Find indices
    submodule_indices = [operations_log.index(op) for op in submodule_ops if op in operations_log]
    parent_indices = [operations_log.index(op) for op in parent_ops if op in operations_log]
    
    if not submodule_indices or not parent_indices:
        return True  # Not enough operations to verify
    
    # All submodule ops should come before all parent ops
    return max(submodule_indices) < min(parent_indices)


# ============================================================
# Directory Type Git Operations
# ============================================================

def git_commit_directory(
    wt_dir: Path,
    repo_path: str,
    commit_msg: str
) -> Tuple[bool, str]:
    """
    Commit changes for directory type repo.
    
    Property 22: Directory Type Git Operations
    *For any* directory-type repo, git operations SHALL execute from the 
    worktree root and include all changes in the commit.
    
    Returns: (success, error_message)
    """
    # Stage all changes from worktree root
    subprocess.run(["git", "add", "-A"], cwd=str(wt_dir), check=True)
    
    # Check if there are changes
    result = subprocess.run(
        ["git", "diff", "--cached", "--quiet"],
        cwd=str(wt_dir),
        capture_output=True
    )
    if result.returncode == 0:
        return False, "no changes"
    
    # Commit from worktree root
    result = subprocess.run(
        ["git", "commit", "-m", commit_msg],
        cwd=str(wt_dir),
        capture_output=True,
        text=True
    )
    if result.returncode != 0:
        return False, f"commit failed: {result.stderr}"
    
    return True, ""


# ============================================================
# Tests
# ============================================================

class TestSubmoduleFileBoundary:
    """Test submodule file boundary checking.
    
    Property 14: Submodule File Boundary
    """

    def test_changes_within_boundary_allowed(self, temp_git_repo):
        """Test changes within submodule boundary are allowed."""
        # Create a simple directory structure (not embedded git)
        submodule_dir = temp_git_repo / "backend"
        submodule_dir.mkdir()
        (submodule_dir / "main.go").write_text("package main")
        subprocess.run(["git", "add", "."], cwd=temp_git_repo, check=True)
        subprocess.run(["git", "commit", "-m", "Add backend"], cwd=temp_git_repo, check=True)
        
        # Make change within submodule path
        (submodule_dir / "new_file.go").write_text("package main")
        subprocess.run(["git", "add", "backend/new_file.go"], cwd=temp_git_repo, check=True)
        
        is_valid, outside_files = check_submodule_boundary(temp_git_repo, "backend")
        
        assert is_valid is True
        assert len(outside_files) == 0

    def test_changes_outside_boundary_rejected(self, temp_git_repo):
        """Test changes outside submodule boundary are rejected."""
        # Create a simple directory structure
        submodule_dir = temp_git_repo / "backend"
        submodule_dir.mkdir()
        (submodule_dir / "main.go").write_text("package main")
        subprocess.run(["git", "add", "."], cwd=temp_git_repo, check=True)
        subprocess.run(["git", "commit", "-m", "Add backend"], cwd=temp_git_repo, check=True)
        
        # Make change outside submodule
        (temp_git_repo / "outside.txt").write_text("outside content")
        subprocess.run(["git", "add", "outside.txt"], cwd=temp_git_repo, check=True)
        
        is_valid, outside_files = check_submodule_boundary(temp_git_repo, "backend")
        
        assert is_valid is False
        assert "outside.txt" in outside_files

    def test_changes_outside_boundary_allowed_with_flag(self, temp_git_repo):
        """Test changes outside boundary allowed with allow_parent flag."""
        # Create a simple directory structure
        submodule_dir = temp_git_repo / "backend"
        submodule_dir.mkdir()
        (submodule_dir / "main.go").write_text("package main")
        subprocess.run(["git", "add", "."], cwd=temp_git_repo, check=True)
        subprocess.run(["git", "commit", "-m", "Add backend"], cwd=temp_git_repo, check=True)
        
        # Make change outside submodule
        (temp_git_repo / "outside.txt").write_text("outside content")
        subprocess.run(["git", "add", "outside.txt"], cwd=temp_git_repo, check=True)
        
        is_valid, outside_files = check_submodule_boundary(temp_git_repo, "backend", allow_parent=True)
        
        assert is_valid is True
        assert "outside.txt" in outside_files  # Still reported but allowed


class TestSubmoduleGitOperationsOrder:
    """Test submodule git operations order.
    
    Property 5: Submodule Git Operations Order
    """

    @pytest.fixture
    def repo_with_submodule(self, temp_git_repo):
        """Create a repo with a proper submodule structure."""
        # Create submodule directory
        submodule_dir = temp_git_repo / "backend"
        submodule_dir.mkdir()
        subprocess.run(["git", "init"], cwd=submodule_dir, check=True)
        subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=submodule_dir, check=True)
        subprocess.run(["git", "config", "user.name", "Test User"], cwd=submodule_dir, check=True)
        (submodule_dir / "main.go").write_text("package main")
        subprocess.run(["git", "add", "."], cwd=submodule_dir, check=True)
        subprocess.run(["git", "commit", "-m", "Initial"], cwd=submodule_dir, check=True)
        
        # Add to parent
        subprocess.run(["git", "add", "."], cwd=temp_git_repo, check=True)
        subprocess.run(["git", "commit", "-m", "Add backend"], cwd=temp_git_repo, check=True)
        
        return temp_git_repo

    def test_commit_order_submodule_first(self, repo_with_submodule):
        """Test submodule is committed before parent."""
        repo = repo_with_submodule
        state = SubmoduleState()
        
        # Make change in submodule
        (repo / "backend" / "new_file.go").write_text("package main\n// new")
        
        success, error = git_commit_submodule(repo, "backend", "test commit", state)
        
        assert success is True
        assert verify_commit_order(state.operations_log) is True
        assert state.operations_log.index("submodule_commit") < state.operations_log.index("parent_commit")

    def test_submodule_sha_recorded(self, repo_with_submodule):
        """Test submodule SHA is recorded after commit."""
        repo = repo_with_submodule
        state = SubmoduleState()
        
        # Make change in submodule
        (repo / "backend" / "new_file.go").write_text("package main\n// new")
        
        success, error = git_commit_submodule(repo, "backend", "test commit", state)
        
        assert success is True
        assert state.submodule_sha != ""
        assert len(state.submodule_sha) == 40  # SHA-1 hash length

    def test_parent_sha_recorded(self, repo_with_submodule):
        """Test parent SHA is recorded after commit."""
        repo = repo_with_submodule
        state = SubmoduleState()
        
        # Make change in submodule
        (repo / "backend" / "new_file.go").write_text("package main\n// new")
        
        success, error = git_commit_submodule(repo, "backend", "test commit", state)
        
        assert success is True
        assert state.parent_sha != ""
        assert len(state.parent_sha) == 40


class TestSubmoduleConsistencyTracking:
    """Test submodule consistency tracking.
    
    Property 16: Submodule Consistency Tracking
    """

    def test_consistency_status_initial(self):
        """Test initial consistency status is consistent."""
        state = SubmoduleState()
        assert state.consistency_status == "consistent"

    def test_consistency_status_on_no_changes(self, temp_git_repo):
        """Test consistency status when no changes."""
        # Create submodule structure
        submodule_dir = temp_git_repo / "backend"
        submodule_dir.mkdir()
        subprocess.run(["git", "init"], cwd=submodule_dir, check=True)
        subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=submodule_dir, check=True)
        subprocess.run(["git", "config", "user.name", "Test User"], cwd=submodule_dir, check=True)
        (submodule_dir / "main.go").write_text("package main")
        subprocess.run(["git", "add", "."], cwd=submodule_dir, check=True)
        subprocess.run(["git", "commit", "-m", "Initial"], cwd=submodule_dir, check=True)
        
        state = SubmoduleState()
        
        # Try to commit with no changes
        success, error = git_commit_submodule(temp_git_repo, "backend", "test", state)
        
        assert success is False
        assert "no changes" in error


class TestDirectoryTypeGitOperations:
    """Test directory type git operations.
    
    Property 22: Directory Type Git Operations
    """

    def test_commit_from_worktree_root(self, temp_git_repo):
        """Test commit executes from worktree root."""
        # Create subdirectory
        subdir = temp_git_repo / "backend"
        subdir.mkdir()
        (subdir / "main.go").write_text("package main")
        
        success, error = git_commit_directory(temp_git_repo, "backend", "test commit")
        
        assert success is True

    def test_all_changes_included(self, temp_git_repo):
        """Test all changes are included in commit."""
        # Create changes in multiple locations
        subdir = temp_git_repo / "backend"
        subdir.mkdir()
        (subdir / "main.go").write_text("package main")
        (temp_git_repo / "root_file.txt").write_text("root content")
        
        success, error = git_commit_directory(temp_git_repo, "backend", "test commit")
        
        assert success is True
        
        # Verify both files are committed
        result = subprocess.run(
            ["git", "show", "--name-only", "--format="],
            cwd=temp_git_repo,
            capture_output=True,
            text=True,
            check=True
        )
        committed_files = result.stdout.strip().split('\n')
        
        assert "backend/main.go" in committed_files
        assert "root_file.txt" in committed_files

    def test_no_changes_returns_false(self, temp_git_repo):
        """Test returns false when no changes."""
        success, error = git_commit_directory(temp_git_repo, "backend", "test commit")
        
        assert success is False
        assert "no changes" in error


class TestVerifyCommitOrder:
    """Test commit order verification function."""

    def test_correct_order(self):
        """Test correct order is verified."""
        log = ["submodule_stage", "submodule_commit", "parent_stage_submodule_ref", "parent_commit"]
        assert verify_commit_order(log) is True

    def test_incorrect_order(self):
        """Test incorrect order is detected."""
        log = ["parent_stage_submodule_ref", "submodule_stage", "submodule_commit", "parent_commit"]
        assert verify_commit_order(log) is False

    def test_empty_log(self):
        """Test empty log returns true."""
        assert verify_commit_order([]) is True

    def test_partial_log(self):
        """Test partial log returns true."""
        log = ["submodule_stage"]
        assert verify_commit_order(log) is True
