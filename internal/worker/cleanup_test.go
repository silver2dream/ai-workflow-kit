package worker

import (
	"testing"
)

// CleanupState tracks cleanup operation state for testing.
// This mirrors the Python CleanupState class for testing cleanup completeness.
type CleanupState struct {
	RemovedWorktrees        map[string]bool
	RemovedBranches         map[string]bool
	RemovedSubmoduleBranches map[string]bool
}

// NewCleanupState creates a new cleanup state tracker.
func NewCleanupState() *CleanupState {
	return &CleanupState{
		RemovedWorktrees:        make(map[string]bool),
		RemovedBranches:         make(map[string]bool),
		RemovedSubmoduleBranches: make(map[string]bool),
	}
}

// CleanupWorktree marks a worktree as removed.
// Property 23: Cleanup Operations Completeness - worktrees removed uniformly for all repo types.
func CleanupWorktree(worktreePath string, state *CleanupState) bool {
	state.RemovedWorktrees[worktreePath] = true
	return true
}

// CleanupBranch marks a branch as removed.
func CleanupBranch(branchName string, state *CleanupState) bool {
	state.RemovedBranches[branchName] = true
	return true
}

// CleanupSubmoduleBranch marks a submodule branch as removed.
// Property 23: Clean branches in both parent and submodule for submodule-type repos.
func CleanupSubmoduleBranch(submodulePath, branchName string, state *CleanupState) bool {
	key := submodulePath + ":" + branchName
	state.RemovedSubmoduleBranches[key] = true
	return true
}

// CleanupAll performs complete cleanup for an issue.
// Property 23: Cleanup Operations Completeness
// - Remove worktrees uniformly for all repo types (Req 12.1)
// - Clean branches in parent (Req 12.2)
// - Clean branches in submodule for submodule-type repos (Req 12.3, 16.3)
func CleanupAll(issueID, repoType, repoPath, branchName string, state *CleanupState) bool {
	worktreePath := ".worktrees/issue-" + issueID

	// Remove worktree (Req 12.1)
	CleanupWorktree(worktreePath, state)

	// Remove parent branch (Req 12.2)
	CleanupBranch(branchName, state)

	// For submodule type, also clean submodule branch (Req 12.3, 16.3)
	if repoType == "submodule" {
		CleanupSubmoduleBranch(repoPath, branchName, state)
	}

	return true
}

// TestCleanupOperationsCompleteness tests cleanup operations completeness.
// Property 23: Cleanup Operations Completeness
func TestCleanupOperationsCompleteness(t *testing.T) {
	t.Run("worktree_removed_for_root", func(t *testing.T) {
		// Test worktree is removed for root type (Req 12.1)
		state := NewCleanupState()

		CleanupAll("1", "root", "./", "feat/ai-issue-1", state)

		if !state.RemovedWorktrees[".worktrees/issue-1"] {
			t.Error("Expected worktree .worktrees/issue-1 to be removed for root type")
		}
	})

	t.Run("worktree_removed_for_directory", func(t *testing.T) {
		// Test worktree is removed for directory type (Req 12.1)
		state := NewCleanupState()

		CleanupAll("1", "directory", "backend", "feat/ai-issue-1", state)

		if !state.RemovedWorktrees[".worktrees/issue-1"] {
			t.Error("Expected worktree .worktrees/issue-1 to be removed for directory type")
		}
	})

	t.Run("worktree_removed_for_submodule", func(t *testing.T) {
		// Test worktree is removed for submodule type (Req 12.1)
		state := NewCleanupState()

		CleanupAll("1", "submodule", "backend", "feat/ai-issue-1", state)

		if !state.RemovedWorktrees[".worktrees/issue-1"] {
			t.Error("Expected worktree .worktrees/issue-1 to be removed for submodule type")
		}
	})

	t.Run("parent_branch_removed", func(t *testing.T) {
		// Test parent branch is removed (Req 12.2)
		state := NewCleanupState()

		CleanupAll("1", "directory", "backend", "feat/ai-issue-1", state)

		if !state.RemovedBranches["feat/ai-issue-1"] {
			t.Error("Expected branch feat/ai-issue-1 to be removed")
		}
	})

	t.Run("submodule_branch_removed", func(t *testing.T) {
		// Test submodule branch is removed for submodule type (Req 12.3)
		state := NewCleanupState()

		CleanupAll("1", "submodule", "backend", "feat/ai-issue-1", state)

		if !state.RemovedSubmoduleBranches["backend:feat/ai-issue-1"] {
			t.Error("Expected submodule branch backend:feat/ai-issue-1 to be removed")
		}
	})

	t.Run("no_submodule_branch_for_directory", func(t *testing.T) {
		// Test no submodule branch cleanup for directory type
		state := NewCleanupState()

		CleanupAll("1", "directory", "backend", "feat/ai-issue-1", state)

		if len(state.RemovedSubmoduleBranches) != 0 {
			t.Errorf("Expected no submodule branches removed for directory type, got %d", len(state.RemovedSubmoduleBranches))
		}
	})
}

// TestCleanupUniformity tests cleanup is uniform across repo types.
func TestCleanupUniformity(t *testing.T) {
	repoTypes := []string{"root", "directory", "submodule"}

	t.Run("worktree_cleanup_uniform", func(t *testing.T) {
		// Test worktree cleanup is uniform for all repo types
		for _, repoType := range repoTypes {
			t.Run(repoType, func(t *testing.T) {
				state := NewCleanupState()

				CleanupAll("1", repoType, "backend", "feat/ai-issue-1", state)

				if !state.RemovedWorktrees[".worktrees/issue-1"] {
					t.Errorf("Expected worktree to be removed for repo type %s", repoType)
				}
			})
		}
	})

	t.Run("parent_branch_cleanup_uniform", func(t *testing.T) {
		// Test parent branch cleanup is uniform for all repo types
		for _, repoType := range repoTypes {
			t.Run(repoType, func(t *testing.T) {
				state := NewCleanupState()

				CleanupAll("1", repoType, "backend", "feat/ai-issue-1", state)

				if !state.RemovedBranches["feat/ai-issue-1"] {
					t.Errorf("Expected branch to be removed for repo type %s", repoType)
				}
			})
		}
	})
}

// TestCleanupState tests cleanup state tracking.
func TestCleanupState(t *testing.T) {
	t.Run("initial_state_empty", func(t *testing.T) {
		// Test initial state is empty
		state := NewCleanupState()

		if len(state.RemovedWorktrees) != 0 {
			t.Errorf("Expected empty removed worktrees, got %d", len(state.RemovedWorktrees))
		}
		if len(state.RemovedBranches) != 0 {
			t.Errorf("Expected empty removed branches, got %d", len(state.RemovedBranches))
		}
		if len(state.RemovedSubmoduleBranches) != 0 {
			t.Errorf("Expected empty removed submodule branches, got %d", len(state.RemovedSubmoduleBranches))
		}
	})

	t.Run("multiple_cleanups_tracked", func(t *testing.T) {
		// Test multiple cleanups are tracked
		state := NewCleanupState()

		CleanupAll("1", "directory", "backend", "feat/ai-issue-1", state)
		CleanupAll("2", "submodule", "frontend", "feat/ai-issue-2", state)

		if len(state.RemovedWorktrees) != 2 {
			t.Errorf("Expected 2 removed worktrees, got %d", len(state.RemovedWorktrees))
		}
		if len(state.RemovedBranches) != 2 {
			t.Errorf("Expected 2 removed branches, got %d", len(state.RemovedBranches))
		}
		if len(state.RemovedSubmoduleBranches) != 1 {
			t.Errorf("Expected 1 removed submodule branch, got %d", len(state.RemovedSubmoduleBranches))
		}
	})
}
