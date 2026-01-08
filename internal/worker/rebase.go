package worker

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// ErrRebaseConflict indicates that rebase has conflicts that need manual resolution
var ErrRebaseConflict = errors.New("rebase has conflicts")

// RebaseOntoBase attempts to rebase the current branch onto the base branch.
// Returns ErrRebaseConflict if there are conflicts that need resolution.
func RebaseOntoBase(ctx context.Context, wtDir, baseBranch string, timeout time.Duration) error {
	// 1. Fetch latest from origin
	if err := runGit(ctx, wtDir, timeout, "fetch", "origin", "--prune"); err != nil {
		return err
	}

	// 2. Attempt rebase
	if err := runGit(ctx, wtDir, timeout, "rebase", "origin/"+baseBranch); err != nil {
		// Check if the error is due to conflicts
		if hasConflicts(ctx, wtDir) {
			return ErrRebaseConflict
		}
		return err
	}

	return nil
}

// ForcePushBranch force pushes the current branch to origin
func ForcePushBranch(ctx context.Context, wtDir, branch string, timeout time.Duration) error {
	return runGit(ctx, wtDir, timeout, "push", "--force-with-lease", "origin", branch)
}

// hasConflicts checks if the worktree has unresolved conflicts
func hasConflicts(ctx context.Context, wtDir string) bool {
	// git diff --name-only --diff-filter=U lists files with unresolved conflicts
	cmd := exec.CommandContext(ctx, "git", "-C", wtDir, "diff", "--name-only", "--diff-filter=U")
	output, _ := cmd.Output()
	return len(bytes.TrimSpace(output)) > 0
}

// getRepoName gets the current repo's owner/name using gh CLI
func getRepoName(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
