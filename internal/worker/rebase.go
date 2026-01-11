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

// ForcePushBranch force pushes the current branch to origin.
// Uses --force-with-lease to prevent overwriting unexpected remote changes.
// This is intended for merge conflict resolution workflows where the branch
// needs to be updated after a successful rebase.
//
// SAFETY: This operation is only used in controlled automation scenarios:
// - After successful rebase onto base branch
// - Within the merge conflict resolution flow
// - The PR still requires review before merging
func ForcePushBranch(ctx context.Context, wtDir, branch string, timeout time.Duration) error {
	// Use --force-with-lease for safety: it will fail if remote has unexpected changes
	// that we haven't fetched. This prevents accidentally overwriting others' work.
	return runGit(ctx, wtDir, timeout, "push", "--force-with-lease", "origin", branch)
}

// hasConflicts checks if the worktree has unresolved conflicts.
// Returns true if conflicts are detected or if the check fails (conservative approach).
func hasConflicts(ctx context.Context, wtDir string) bool {
	// git diff --name-only --diff-filter=U lists files with unresolved conflicts
	cmd := exec.CommandContext(ctx, "git", "-C", wtDir, "diff", "--name-only", "--diff-filter=U")
	output, err := cmd.Output()
	if err != nil {
		// If the command fails, assume conflicts exist (conservative approach)
		// This prevents false negatives that could lead to incorrect state
		return true
	}
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
