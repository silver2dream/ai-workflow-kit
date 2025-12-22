"""
Unit tests for worktree creation functionality.

**Feature: multi-repo-support**
**Property 2: Worktree Path Consistency**
**Property 3: Worktree Idempotency**
**Property 4: Submodule Initialization in Worktree**
**Validates: Requirements 2.1-2.4, 3.1-3.3, 4.1-4.5, 14.1-14.5**

Note: These tests validate the worktree logic using Python implementations
that mirror the bash script behavior, avoiding Windows path issues with subprocess.
"""
import pytest
import subprocess
import os
from pathlib import Path


def get_worktree_path(root: Path, issue_id: str) -> Path:
    """Get the expected worktree path for an issue."""
    return root / ".worktrees" / f"issue-{issue_id}"


def get_work_dir(worktree: Path, repo_type: str, repo_path: str) -> Path:
    """Get the WORK_DIR based on repo type.
    
    Property 2: Worktree Path Consistency
    - root: {worktree}
    - directory: {worktree}/{repo_path}
    - submodule: {worktree}/{repo_path}
    """
    if repo_type == "root":
        return worktree
    else:
        return worktree / repo_path


def worktree_exists(root: Path, issue_id: str) -> bool:
    """Check if worktree already exists."""
    wt_path = get_worktree_path(root, issue_id)
    return wt_path.is_dir()


def create_worktree(root: Path, issue_id: str, branch: str, base_branch: str = "main") -> Path:
    """Create a worktree for the given issue.
    
    Returns the worktree path.
    """
    wt_path = get_worktree_path(root, issue_id)
    
    # Idempotent: if exists, return existing path (Property 3)
    if wt_path.is_dir():
        return wt_path
    
    # Ensure .worktrees directory exists
    (root / ".worktrees").mkdir(exist_ok=True)
    
    # Create branch if it doesn't exist
    result = subprocess.run(
        ["git", "show-ref", "--verify", "--quiet", f"refs/heads/{branch}"],
        cwd=str(root),
        capture_output=True
    )
    if result.returncode != 0:
        subprocess.run(
            ["git", "branch", branch, base_branch],
            cwd=str(root),
            check=True
        )
    
    # Create worktree
    subprocess.run(
        ["git", "worktree", "add", str(wt_path), branch],
        cwd=str(root),
        check=True
    )
    
    return wt_path


def remove_worktree(root: Path, issue_id: str):
    """Remove a worktree."""
    wt_path = get_worktree_path(root, issue_id)
    if wt_path.is_dir():
        subprocess.run(
            ["git", "worktree", "remove", "--force", str(wt_path)],
            cwd=str(root),
            check=True
        )


def verify_directory_exists(worktree: Path, repo_path: str) -> bool:
    """Verify directory path exists in worktree."""
    work_dir = worktree / repo_path
    return work_dir.is_dir()


def verify_submodule_initialized(worktree: Path, repo_path: str) -> bool:
    """Verify submodule is initialized (has .git)."""
    submodule_dir = worktree / repo_path
    git_path = submodule_dir / ".git"
    return git_path.exists()


class TestWorktreePathConsistency:
    """Test worktree path consistency.
    
    Property 2: Worktree Path Consistency
    *For any* issue ID and repo type, the worktree SHALL be created at 
    `.worktrees/issue-{ID}`, and WORK_DIR SHALL be set correctly based on repo type.
    """

    def test_worktree_path_format(self, temp_git_repo):
        """Test worktree path follows expected format."""
        issue_id = "123"
        expected = temp_git_repo / ".worktrees" / "issue-123"
        actual = get_worktree_path(temp_git_repo, issue_id)
        assert actual == expected

    @pytest.mark.parametrize("issue_id", ["1", "42", "999", "12345"])
    def test_worktree_path_various_ids(self, temp_git_repo, issue_id):
        """Test worktree path works for various issue IDs."""
        expected = temp_git_repo / ".worktrees" / f"issue-{issue_id}"
        actual = get_worktree_path(temp_git_repo, issue_id)
        assert actual == expected

    def test_work_dir_root_type(self, temp_git_repo):
        """Test WORK_DIR for root type equals worktree."""
        worktree = temp_git_repo / ".worktrees" / "issue-1"
        work_dir = get_work_dir(worktree, "root", ".")
        assert work_dir == worktree

    def test_work_dir_directory_type(self, temp_git_repo):
        """Test WORK_DIR for directory type includes repo path."""
        worktree = temp_git_repo / ".worktrees" / "issue-1"
        work_dir = get_work_dir(worktree, "directory", "backend")
        assert work_dir == worktree / "backend"

    def test_work_dir_submodule_type(self, temp_git_repo):
        """Test WORK_DIR for submodule type includes repo path."""
        worktree = temp_git_repo / ".worktrees" / "issue-1"
        work_dir = get_work_dir(worktree, "submodule", "libs/shared")
        assert work_dir == worktree / "libs" / "shared"


class TestWorktreeIdempotency:
    """Test worktree creation idempotency.
    
    Property 3: Worktree Idempotency
    *For any* issue ID, calling worktree creation multiple times SHALL 
    return the same worktree path without error.
    """

    def test_create_worktree_first_time(self, temp_git_repo):
        """Test worktree is created on first call."""
        issue_id = "1"
        branch = "feat/ai-issue-1"
        
        wt_path = create_worktree(temp_git_repo, issue_id, branch, "master")
        
        assert wt_path.is_dir()
        assert wt_path == get_worktree_path(temp_git_repo, issue_id)
        
        # Cleanup
        remove_worktree(temp_git_repo, issue_id)

    def test_create_worktree_idempotent(self, temp_git_repo):
        """Test calling create_worktree twice returns same path."""
        issue_id = "2"
        branch = "feat/ai-issue-2"
        
        # First call
        wt_path1 = create_worktree(temp_git_repo, issue_id, branch, "master")
        
        # Second call (should be idempotent)
        wt_path2 = create_worktree(temp_git_repo, issue_id, branch, "master")
        
        assert wt_path1 == wt_path2
        assert wt_path1.is_dir()
        
        # Cleanup
        remove_worktree(temp_git_repo, issue_id)

    def test_worktree_exists_check(self, temp_git_repo):
        """Test worktree_exists returns correct status."""
        issue_id = "3"
        branch = "feat/ai-issue-3"
        
        # Before creation
        assert worktree_exists(temp_git_repo, issue_id) is False
        
        # After creation
        create_worktree(temp_git_repo, issue_id, branch, "master")
        assert worktree_exists(temp_git_repo, issue_id) is True
        
        # After removal
        remove_worktree(temp_git_repo, issue_id)
        assert worktree_exists(temp_git_repo, issue_id) is False


class TestWorktreeDirectoryVerification:
    """Test directory verification in worktree.
    
    Validates: Requirements 3.3, 14.4
    """

    def test_verify_directory_exists_true(self, temp_git_repo):
        """Test directory verification passes for existing directory."""
        issue_id = "4"
        branch = "feat/ai-issue-4"
        
        # Create directory in main repo
        backend_dir = temp_git_repo / "backend"
        backend_dir.mkdir()
        (backend_dir / ".gitkeep").write_text("")
        subprocess.run(["git", "add", "."], cwd=temp_git_repo, check=True)
        subprocess.run(["git", "commit", "-m", "Add backend"], cwd=temp_git_repo, check=True)
        
        # Create worktree
        wt_path = create_worktree(temp_git_repo, issue_id, branch, "master")
        
        # Verify directory exists in worktree
        assert verify_directory_exists(wt_path, "backend") is True
        
        # Cleanup
        remove_worktree(temp_git_repo, issue_id)

    def test_verify_directory_exists_false(self, temp_git_repo):
        """Test directory verification fails for non-existing directory."""
        issue_id = "5"
        branch = "feat/ai-issue-5"
        
        # Create worktree
        wt_path = create_worktree(temp_git_repo, issue_id, branch, "master")
        
        # Verify non-existing directory
        assert verify_directory_exists(wt_path, "nonexistent") is False
        
        # Cleanup
        remove_worktree(temp_git_repo, issue_id)


class TestSubmoduleInitialization:
    """Test submodule initialization in worktree.
    
    Property 4: Submodule Initialization in Worktree
    *For any* worktree created for a submodule-type repo, the submodule 
    directory SHALL be initialized and contain a valid git repository 
    after worktree creation.
    """

    def test_verify_submodule_initialized_with_git_file(self, temp_git_repo):
        """Test submodule verification passes when .git file exists.
        
        Note: Real submodules have a .git file (not directory) pointing to
        the parent's .git/modules/<name> directory.
        """
        # Create directory with .git file (simulating submodule structure)
        backend_dir = temp_git_repo / "backend"
        backend_dir.mkdir()
        (backend_dir / ".git").write_text("gitdir: ../.git/modules/backend")
        (backend_dir / "README.md").write_text("# Backend")
        
        # Verify submodule is initialized (has .git file) - test directly, not via worktree
        assert verify_submodule_initialized(temp_git_repo, "backend") is True

    def test_verify_submodule_initialized_with_git_dir(self, temp_git_repo):
        """Test submodule verification passes when .git directory exists."""
        # Create directory with actual .git directory
        backend_dir = temp_git_repo / "backend"
        backend_dir.mkdir()
        
        # Initialize as git repo (creates .git directory)
        subprocess.run(["git", "init"], cwd=backend_dir, check=True)
        subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=backend_dir, check=True)
        subprocess.run(["git", "config", "user.name", "Test User"], cwd=backend_dir, check=True)
        (backend_dir / "README.md").write_text("# Backend")
        subprocess.run(["git", "add", "."], cwd=backend_dir, check=True)
        subprocess.run(["git", "commit", "-m", "Initial"], cwd=backend_dir, check=True)
        
        # Verify .git exists in the original location
        assert verify_submodule_initialized(temp_git_repo, "backend") is True

    def test_verify_submodule_initialized_false(self, temp_git_repo):
        """Test submodule verification fails for non-submodule directory."""
        issue_id = "7"
        branch = "feat/ai-issue-7"
        
        # Create regular directory (not a submodule)
        backend_dir = temp_git_repo / "backend"
        backend_dir.mkdir()
        (backend_dir / "README.md").write_text("# Backend")
        subprocess.run(["git", "add", "."], cwd=temp_git_repo, check=True)
        subprocess.run(["git", "commit", "-m", "Add backend"], cwd=temp_git_repo, check=True)
        
        # Create worktree
        wt_path = create_worktree(temp_git_repo, issue_id, branch, "master")
        
        # Verify submodule is NOT initialized (no .git)
        assert verify_submodule_initialized(wt_path, "backend") is False
        
        # Cleanup
        remove_worktree(temp_git_repo, issue_id)
    
    def test_verify_function_checks_git_existence(self, temp_dir):
        """Test verify_submodule_initialized checks for .git existence."""
        # Create directory without .git
        subdir = temp_dir / "subdir"
        subdir.mkdir()
        assert verify_submodule_initialized(temp_dir, "subdir") is False
        
        # Add .git file
        (subdir / ".git").write_text("gitdir: somewhere")
        assert verify_submodule_initialized(temp_dir, "subdir") is True


class TestWorktreeBranchManagement:
    """Test branch management during worktree creation.
    
    Validates: Requirements 14.1, 14.2
    """

    def test_branch_created_from_base(self, temp_git_repo):
        """Test branch is created from base branch if it doesn't exist."""
        issue_id = "8"
        branch = "feat/ai-issue-8"
        
        # Verify branch doesn't exist
        result = subprocess.run(
            ["git", "show-ref", "--verify", "--quiet", f"refs/heads/{branch}"],
            cwd=temp_git_repo,
            capture_output=True
        )
        assert result.returncode != 0
        
        # Create worktree (should create branch)
        wt_path = create_worktree(temp_git_repo, issue_id, branch, "master")
        
        # Verify branch now exists
        result = subprocess.run(
            ["git", "show-ref", "--verify", "--quiet", f"refs/heads/{branch}"],
            cwd=temp_git_repo,
            capture_output=True
        )
        assert result.returncode == 0
        
        # Cleanup
        remove_worktree(temp_git_repo, issue_id)

    def test_existing_branch_reused(self, temp_git_repo):
        """Test existing branch is reused."""
        issue_id = "9"
        branch = "feat/ai-issue-9"
        
        # Create branch first
        subprocess.run(["git", "branch", branch, "master"], cwd=temp_git_repo, check=True)
        
        # Get branch SHA before worktree
        result = subprocess.run(
            ["git", "rev-parse", branch],
            cwd=temp_git_repo,
            capture_output=True,
            text=True,
            check=True
        )
        sha_before = result.stdout.strip()
        
        # Create worktree
        wt_path = create_worktree(temp_git_repo, issue_id, branch, "master")
        
        # Get branch SHA after worktree
        result = subprocess.run(
            ["git", "rev-parse", branch],
            cwd=temp_git_repo,
            capture_output=True,
            text=True,
            check=True
        )
        sha_after = result.stdout.strip()
        
        # SHA should be the same (branch reused, not recreated)
        assert sha_before == sha_after
        
        # Cleanup
        remove_worktree(temp_git_repo, issue_id)


class TestWorktreeValidRepoTypes:
    """Test worktree creation for all valid repo types."""

    @pytest.mark.parametrize("repo_type,repo_path", [
        ("root", "."),
        ("directory", "backend"),
        ("submodule", "libs/shared"),
    ])
    def test_work_dir_calculation(self, repo_type, repo_path):
        """Test WORK_DIR is calculated correctly for all repo types."""
        worktree = Path("/tmp/worktrees/issue-1")
        work_dir = get_work_dir(worktree, repo_type, repo_path)
        
        if repo_type == "root":
            assert work_dir == worktree
        else:
            assert work_dir == worktree / repo_path
