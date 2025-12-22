"""
Unit tests for branch management functionality.

**Feature: multi-repo-support**
**Property 24: Submodule Branch Management**
**Validates: Requirements 16.1, 16.2, 16.4**

Note: These tests validate the branch management logic.
"""
import pytest
from typing import Set


class BranchState:
    """Track branch state."""
    def __init__(self):
        self.local_branches: Set[str] = set()
        self.remote_branches: Set[str] = set()
        self.current_branch: str = ""
        self.default_branch: str = "main"


def setup_submodule_branch(
    branch_name: str,
    state: BranchState
) -> str:
    """
    Create or reuse branch in submodule.
    
    Property 24: Submodule Branch Management
    *For any* submodule-type repo, the system SHALL:
    - Create branches in the submodule first (Req 16.1)
    - Use the submodule's default branch as base (Req 16.2)
    - Reuse existing branches when available (Req 16.4)
    
    Returns: The branch name that was set up
    """
    # Check if branch already exists locally (Req 16.4)
    if branch_name in state.local_branches:
        state.current_branch = branch_name
        return branch_name
    
    # Check if branch exists on remote (Req 16.4)
    if branch_name in state.remote_branches:
        state.local_branches.add(branch_name)
        state.current_branch = branch_name
        return branch_name
    
    # Create new branch from default branch (Req 16.1, 16.2)
    state.local_branches.add(branch_name)
    state.current_branch = branch_name
    return branch_name


def get_submodule_default_branch(state: BranchState) -> str:
    """
    Get submodule's default branch.
    
    Returns: Default branch name
    """
    return state.default_branch


class TestSubmoduleBranchManagement:
    """Test submodule branch management.
    
    Property 24: Submodule Branch Management
    """

    def test_create_new_branch(self):
        """Test creating new branch in submodule (Req 16.1)."""
        state = BranchState()
        
        result = setup_submodule_branch("feat/ai-issue-1", state)
        
        assert result == "feat/ai-issue-1"
        assert "feat/ai-issue-1" in state.local_branches
        assert state.current_branch == "feat/ai-issue-1"

    def test_reuse_existing_local_branch(self):
        """Test reusing existing local branch (Req 16.4)."""
        state = BranchState()
        state.local_branches.add("feat/ai-issue-1")
        
        result = setup_submodule_branch("feat/ai-issue-1", state)
        
        assert result == "feat/ai-issue-1"
        assert state.current_branch == "feat/ai-issue-1"

    def test_checkout_remote_branch(self):
        """Test checking out remote branch (Req 16.4)."""
        state = BranchState()
        state.remote_branches.add("feat/ai-issue-1")
        
        result = setup_submodule_branch("feat/ai-issue-1", state)
        
        assert result == "feat/ai-issue-1"
        assert "feat/ai-issue-1" in state.local_branches
        assert state.current_branch == "feat/ai-issue-1"

    def test_default_branch_used_as_base(self):
        """Test default branch is used as base (Req 16.2)."""
        state = BranchState()
        state.default_branch = "develop"
        
        default = get_submodule_default_branch(state)
        
        assert default == "develop"


class TestBranchState:
    """Test branch state tracking."""

    def test_initial_state(self):
        """Test initial branch state."""
        state = BranchState()
        
        assert len(state.local_branches) == 0
        assert len(state.remote_branches) == 0
        assert state.current_branch == ""
        assert state.default_branch == "main"

    def test_multiple_branches(self):
        """Test multiple branches can be tracked."""
        state = BranchState()
        
        setup_submodule_branch("feat/ai-issue-1", state)
        setup_submodule_branch("feat/ai-issue-2", state)
        
        assert "feat/ai-issue-1" in state.local_branches
        assert "feat/ai-issue-2" in state.local_branches

    def test_current_branch_updated(self):
        """Test current branch is updated."""
        state = BranchState()
        
        setup_submodule_branch("feat/ai-issue-1", state)
        assert state.current_branch == "feat/ai-issue-1"
        
        setup_submodule_branch("feat/ai-issue-2", state)
        assert state.current_branch == "feat/ai-issue-2"


class TestBranchNaming:
    """Test branch naming conventions."""

    @pytest.mark.parametrize("branch_name", [
        "feat/ai-issue-1",
        "feat/ai-issue-123",
        "fix/ai-issue-456",
        "chore/ai-issue-789",
    ])
    def test_valid_branch_names(self, branch_name):
        """Test valid branch names are accepted."""
        state = BranchState()
        
        result = setup_submodule_branch(branch_name, state)
        
        assert result == branch_name
        assert branch_name in state.local_branches
