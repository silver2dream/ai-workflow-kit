package git

import (
	"os"
	"os/exec"
	"path/filepath"
)

// GetWorktreePath returns the expected worktree path for an issue.
func GetWorktreePath(root, issueID string) string {
	return filepath.Join(root, ".worktrees", "issue-"+issueID)
}

// GetWorkDir returns the WORK_DIR based on repo type.
//
// Property 2: Worktree Path Consistency
//   - root: {worktree}
//   - directory: {worktree}/{repo_path}
//   - submodule: {worktree}/{repo_path}
func GetWorkDir(worktree, repoType, repoPath string) string {
	if repoType == "root" {
		return worktree
	}
	return filepath.Join(worktree, repoPath)
}

// WorktreeExists checks if worktree already exists.
func WorktreeExists(root, issueID string) bool {
	wtPath := GetWorktreePath(root, issueID)
	info, err := os.Stat(wtPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CreateWorktree creates a worktree for the given issue.
//
// Property 3: Worktree Idempotency
// For any issue ID, calling worktree creation multiple times SHALL
// return the same worktree path without error.
//
// Returns the worktree path.
func CreateWorktree(root, issueID, branch, baseBranch string) (string, error) {
	wtPath := GetWorktreePath(root, issueID)

	// Idempotent: if exists, return existing path (Property 3)
	if info, err := os.Stat(wtPath); err == nil && info.IsDir() {
		return wtPath, nil
	}

	// Ensure .worktrees directory exists
	worktreesDir := filepath.Join(root, ".worktrees")
	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		return "", err
	}

	// Create branch if it doesn't exist
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		// Branch doesn't exist, create it
		cmd = exec.Command("git", "branch", branch, baseBranch)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			return "", err
		}
	}

	// Create worktree
	cmd = exec.Command("git", "worktree", "add", wtPath, branch)
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return wtPath, nil
}

// RemoveWorktree removes a worktree.
func RemoveWorktree(root, issueID string) error {
	wtPath := GetWorktreePath(root, issueID)
	if info, err := os.Stat(wtPath); err == nil && info.IsDir() {
		cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
		cmd.Dir = root
		return cmd.Run()
	}
	return nil
}

// VerifyDirectoryExists verifies directory path exists in worktree.
func VerifyDirectoryExists(worktree, repoPath string) bool {
	workDir := filepath.Join(worktree, repoPath)
	info, err := os.Stat(workDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// VerifySubmoduleInitialized verifies submodule is initialized (has .git).
//
// Property 4: Submodule Initialization in Worktree
// For any worktree created for a submodule-type repo, the submodule
// directory SHALL be initialized and contain a valid git repository
// after worktree creation.
func VerifySubmoduleInitialized(worktree, repoPath string) bool {
	submoduleDir := filepath.Join(worktree, repoPath)
	gitPath := filepath.Join(submoduleDir, ".git")
	_, err := os.Stat(gitPath)
	return err == nil
}

// BranchExists checks if a branch exists in the repository.
func BranchExists(root, branch string) bool {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = root
	return cmd.Run() == nil
}

// GetBranchSHA returns the SHA of a branch.
func GetBranchSHA(root, branch string) (string, error) {
	cmd := exec.Command("git", "rev-parse", branch)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output[:len(output)-1]), nil // trim newline
}

// CreateBranch creates a new branch from a base.
func CreateBranch(root, branch, base string) error {
	cmd := exec.Command("git", "branch", branch, base)
	cmd.Dir = root
	return cmd.Run()
}
