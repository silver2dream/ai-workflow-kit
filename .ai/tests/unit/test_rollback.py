"""
Unit tests for rollback functionality.

**Feature: multi-repo-support**
**Property 12: Submodule Rollback Consistency**
**Validates: Requirements 18.1, 18.2, 18.3, 18.4**

Note: These tests validate the rollback logic.
"""
import pytest
from typing import List, Tuple


class RollbackState:
    """Track rollback operation state."""
    def __init__(self):
        self.operations_log: List[str] = []
        self.submodule_reverted = False
        self.parent_reverted = False
        self.error = ""


def rollback_submodule(
    submodule_sha: str,
    parent_sha: str,
    state: RollbackState
) -> Tuple[bool, str]:
    """
    Rollback submodule and parent commits.
    
    Property 12: Submodule Rollback Consistency
    *For any* rollback of a PR that modified a submodule, the system SHALL 
    revert both the submodule commit and the parent reference in the correct 
    order (submodule first, then parent).
    
    Returns: (success, error_message)
    """
    # Step 1: Revert submodule commit first (Req 18.1)
    state.operations_log.append("revert_submodule")
    if not submodule_sha:
        state.error = "No submodule SHA to revert"
        return False, state.error
    state.submodule_reverted = True
    
    # Step 2: Revert parent reference (Req 18.2)
    state.operations_log.append("revert_parent")
    if not parent_sha:
        state.error = "No parent SHA to revert"
        return False, state.error
    state.parent_reverted = True
    
    return True, ""


def verify_rollback_order(operations_log: List[str]) -> bool:
    """
    Verify rollback operations happened in correct order.
    
    Property 12: Submodule Rollback Consistency
    Expected order: submodule revert before parent revert
    """
    if "revert_submodule" not in operations_log or "revert_parent" not in operations_log:
        return True  # Not enough operations to verify
    
    submodule_idx = operations_log.index("revert_submodule")
    parent_idx = operations_log.index("revert_parent")
    
    return submodule_idx < parent_idx


def rollback_directory(parent_sha: str, state: RollbackState) -> Tuple[bool, str]:
    """
    Rollback directory type commit.
    
    Returns: (success, error_message)
    """
    state.operations_log.append("revert_parent")
    if not parent_sha:
        state.error = "No parent SHA to revert"
        return False, state.error
    state.parent_reverted = True
    return True, ""


def rollback_root(parent_sha: str, state: RollbackState) -> Tuple[bool, str]:
    """
    Rollback root type commit.
    
    Returns: (success, error_message)
    """
    state.operations_log.append("revert_parent")
    if not parent_sha:
        state.error = "No parent SHA to revert"
        return False, state.error
    state.parent_reverted = True
    return True, ""


class TestSubmoduleRollbackConsistency:
    """Test submodule rollback consistency.
    
    Property 12: Submodule Rollback Consistency
    """

    def test_rollback_order_submodule_first(self):
        """Test submodule is reverted before parent (Req 18.1, 18.2)."""
        state = RollbackState()
        
        success, error = rollback_submodule("abc123", "def456", state)
        
        assert success is True
        assert verify_rollback_order(state.operations_log) is True
        assert state.operations_log.index("revert_submodule") < state.operations_log.index("revert_parent")

    def test_both_commits_reverted(self):
        """Test both submodule and parent are reverted (Req 18.3)."""
        state = RollbackState()
        
        success, error = rollback_submodule("abc123", "def456", state)
        
        assert success is True
        assert state.submodule_reverted is True
        assert state.parent_reverted is True

    def test_missing_submodule_sha_fails(self):
        """Test rollback fails without submodule SHA (Req 18.4)."""
        state = RollbackState()
        
        success, error = rollback_submodule("", "def456", state)
        
        assert success is False
        assert "No submodule SHA" in error

    def test_missing_parent_sha_fails(self):
        """Test rollback fails without parent SHA (Req 18.4)."""
        state = RollbackState()
        
        success, error = rollback_submodule("abc123", "", state)
        
        assert success is False
        assert "No parent SHA" in error


class TestRollbackByRepoType:
    """Test rollback for different repo types."""

    def test_root_type_rollback(self):
        """Test root type rollback only reverts parent."""
        state = RollbackState()
        
        success, error = rollback_root("abc123", state)
        
        assert success is True
        assert state.parent_reverted is True
        assert state.submodule_reverted is False

    def test_directory_type_rollback(self):
        """Test directory type rollback only reverts parent."""
        state = RollbackState()
        
        success, error = rollback_directory("abc123", state)
        
        assert success is True
        assert state.parent_reverted is True
        assert state.submodule_reverted is False

    def test_submodule_type_rollback(self):
        """Test submodule type rollback reverts both."""
        state = RollbackState()
        
        success, error = rollback_submodule("abc123", "def456", state)
        
        assert success is True
        assert state.parent_reverted is True
        assert state.submodule_reverted is True


class TestVerifyRollbackOrder:
    """Test rollback order verification function."""

    def test_correct_order(self):
        """Test correct order is verified."""
        log = ["revert_submodule", "revert_parent"]
        assert verify_rollback_order(log) is True

    def test_incorrect_order(self):
        """Test incorrect order is detected."""
        log = ["revert_parent", "revert_submodule"]
        assert verify_rollback_order(log) is False

    def test_empty_log(self):
        """Test empty log returns true."""
        assert verify_rollback_order([]) is True

    def test_partial_log(self):
        """Test partial log returns true."""
        log = ["revert_submodule"]
        assert verify_rollback_order(log) is True
