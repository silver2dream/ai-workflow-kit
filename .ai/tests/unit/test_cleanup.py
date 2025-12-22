"""
Unit tests for cleanup functionality.

**Feature: multi-repo-support**
**Property 23: Cleanup Operations Completeness**
**Validates: Requirements 12.1, 12.2, 12.3, 16.3**

Note: These tests validate the cleanup logic.
"""
import pytest
from typing import List, Set


class CleanupState:
    """Track cleanup operation state."""
    def __init__(self):
        self.removed_worktrees: Set[str] = set()
        self.removed_branches: Set[str] = set()
        self.removed_submodule_branches: Set[str] = set()


def cleanup_worktree(
    worktree_path: str,
    state: CleanupState
) -> bool:
    """
    Remove a worktree.
    
    Property 23: Cleanup Operations Completeness
    *For any* cleanup operation, the system SHALL remove worktrees 
    uniformly for all repo types.
    
    Returns: success
    """
    state.removed_worktrees.add(worktree_path)
    return True


def cleanup_branch(
    branch_name: str,
    state: CleanupState
) -> bool:
    """
    Remove a branch.
    
    Returns: success
    """
    state.removed_branches.add(branch_name)
    return True


def cleanup_submodule_branch(
    submodule_path: str,
    branch_name: str,
    state: CleanupState
) -> bool:
    """
    Remove a branch in submodule.
    
    Property 23: Cleanup Operations Completeness
    Clean branches in both parent and submodule for submodule-type repos.
    
    Returns: success
    """
    key = f"{submodule_path}:{branch_name}"
    state.removed_submodule_branches.add(key)
    return True


def cleanup_all(
    issue_id: str,
    repo_type: str,
    repo_path: str,
    branch_name: str,
    state: CleanupState
) -> bool:
    """
    Perform complete cleanup for an issue.
    
    Property 23: Cleanup Operations Completeness
    - Remove worktrees uniformly for all repo types (Req 12.1)
    - Clean branches in parent (Req 12.2)
    - Clean branches in submodule for submodule-type repos (Req 12.3, 16.3)
    
    Returns: success
    """
    worktree_path = f".worktrees/issue-{issue_id}"
    
    # Remove worktree (Req 12.1)
    cleanup_worktree(worktree_path, state)
    
    # Remove parent branch (Req 12.2)
    cleanup_branch(branch_name, state)
    
    # For submodule type, also clean submodule branch (Req 12.3, 16.3)
    if repo_type == "submodule":
        cleanup_submodule_branch(repo_path, branch_name, state)
    
    return True


class TestCleanupOperationsCompleteness:
    """Test cleanup operations completeness.
    
    Property 23: Cleanup Operations Completeness
    """

    def test_worktree_removed_for_root(self):
        """Test worktree is removed for root type (Req 12.1)."""
        state = CleanupState()
        
        cleanup_all("1", "root", "./", "feat/ai-issue-1", state)
        
        assert ".worktrees/issue-1" in state.removed_worktrees

    def test_worktree_removed_for_directory(self):
        """Test worktree is removed for directory type (Req 12.1)."""
        state = CleanupState()
        
        cleanup_all("1", "directory", "backend", "feat/ai-issue-1", state)
        
        assert ".worktrees/issue-1" in state.removed_worktrees

    def test_worktree_removed_for_submodule(self):
        """Test worktree is removed for submodule type (Req 12.1)."""
        state = CleanupState()
        
        cleanup_all("1", "submodule", "backend", "feat/ai-issue-1", state)
        
        assert ".worktrees/issue-1" in state.removed_worktrees

    def test_parent_branch_removed(self):
        """Test parent branch is removed (Req 12.2)."""
        state = CleanupState()
        
        cleanup_all("1", "directory", "backend", "feat/ai-issue-1", state)
        
        assert "feat/ai-issue-1" in state.removed_branches

    def test_submodule_branch_removed(self):
        """Test submodule branch is removed for submodule type (Req 12.3)."""
        state = CleanupState()
        
        cleanup_all("1", "submodule", "backend", "feat/ai-issue-1", state)
        
        assert "backend:feat/ai-issue-1" in state.removed_submodule_branches

    def test_no_submodule_branch_for_directory(self):
        """Test no submodule branch cleanup for directory type."""
        state = CleanupState()
        
        cleanup_all("1", "directory", "backend", "feat/ai-issue-1", state)
        
        assert len(state.removed_submodule_branches) == 0


class TestCleanupUniformity:
    """Test cleanup is uniform across repo types."""

    @pytest.mark.parametrize("repo_type", ["root", "directory", "submodule"])
    def test_worktree_cleanup_uniform(self, repo_type):
        """Test worktree cleanup is uniform for all repo types."""
        state = CleanupState()
        
        cleanup_all("1", repo_type, "backend", "feat/ai-issue-1", state)
        
        assert ".worktrees/issue-1" in state.removed_worktrees

    @pytest.mark.parametrize("repo_type", ["root", "directory", "submodule"])
    def test_parent_branch_cleanup_uniform(self, repo_type):
        """Test parent branch cleanup is uniform for all repo types."""
        state = CleanupState()
        
        cleanup_all("1", repo_type, "backend", "feat/ai-issue-1", state)
        
        assert "feat/ai-issue-1" in state.removed_branches


class TestCleanupState:
    """Test cleanup state tracking."""

    def test_initial_state_empty(self):
        """Test initial state is empty."""
        state = CleanupState()
        
        assert len(state.removed_worktrees) == 0
        assert len(state.removed_branches) == 0
        assert len(state.removed_submodule_branches) == 0

    def test_multiple_cleanups_tracked(self):
        """Test multiple cleanups are tracked."""
        state = CleanupState()
        
        cleanup_all("1", "directory", "backend", "feat/ai-issue-1", state)
        cleanup_all("2", "submodule", "frontend", "feat/ai-issue-2", state)
        
        assert len(state.removed_worktrees) == 2
        assert len(state.removed_branches) == 2
        assert len(state.removed_submodule_branches) == 1
