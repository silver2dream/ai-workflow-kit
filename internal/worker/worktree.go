package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorktreeOptions controls worktree creation.
type WorktreeOptions struct {
	StateRoot         string
	IssueID           int
	Branch            string
	RepoType          string
	RepoPath          string
	IntegrationBranch string
	GitTimeout        time.Duration
	Logf              func(string, ...interface{})
}

// WorktreeInfo contains resolved worktree paths.
type WorktreeInfo struct {
	WorktreeDir string
	WorkDir     string
}

// SetupWorktree creates or reuses a worktree for the issue.
func SetupWorktree(ctx context.Context, opts WorktreeOptions) (*WorktreeInfo, error) {
	wtDir := filepath.Join(opts.StateRoot, ".worktrees", fmt.Sprintf("issue-%d", opts.IssueID))
	if _, err := os.Stat(wtDir); err == nil {
		// Clean worktree on reuse to prevent "working tree not clean" errors (Issue #122)
		if cleanErr := cleanWorktree(ctx, wtDir, opts.GitTimeout, opts.Logf); cleanErr != nil {
			// Log warning but continue - cleanup is best effort
			if opts.Logf != nil {
				opts.Logf("WARNING: failed to clean existing worktree: %v", cleanErr)
			}
		}
		return &WorktreeInfo{
			WorktreeDir: wtDir,
			WorkDir:     resolveWorkDir(wtDir, opts.RepoType, opts.RepoPath),
		}, nil
	}

	if err := ensureWorktreeDirs(opts.StateRoot); err != nil {
		return nil, err
	}

	baseBranch := strings.TrimSpace(os.Getenv("AI_BASE_BRANCH"))
	if baseBranch == "" {
		baseBranch = opts.IntegrationBranch
	}
	if baseBranch == "" {
		baseBranch = "develop"
	}

	remoteBase := strings.TrimSpace(os.Getenv("AI_REMOTE_BASE"))
	if remoteBase == "" {
		remoteBase = "origin/" + opts.IntegrationBranch
	}

	if err := runGit(ctx, opts.StateRoot, opts.GitTimeout, "fetch", "origin", "--prune"); err != nil {
		return nil, err
	}

	if err := runGit(ctx, opts.StateRoot, opts.GitTimeout, "show-ref", "--verify", "--quiet", "refs/heads/"+baseBranch); err != nil {
		if err := runGit(ctx, opts.StateRoot, opts.GitTimeout, "checkout", "-q", "-b", baseBranch, remoteBase); err != nil {
			return nil, err
		}
	}

	// Checkout base branch - ignore error if already on the branch
	_ = runGit(ctx, opts.StateRoot, opts.GitTimeout, "checkout", "-q", baseBranch)

	// Pull latest changes - this is critical for worktree to be based on latest code
	if err := runGit(ctx, opts.StateRoot, opts.GitTimeout, "pull", "--ff-only", "origin", baseBranch); err != nil {
		// Pull may fail if remote branch doesn't exist yet or network issues
		// Try to continue if the local branch exists and is valid
		if verifyErr := runGit(ctx, opts.StateRoot, opts.GitTimeout, "rev-parse", "--verify", baseBranch); verifyErr != nil {
			return nil, fmt.Errorf("failed to pull and verify base branch %s: %w", baseBranch, err)
		}
		// Local branch exists, continue with potentially stale state (logged but not fatal)
	}

	if err := ensureWorktreeBranch(ctx, opts.StateRoot, opts.Branch, baseBranch, opts.GitTimeout); err != nil {
		return nil, err
	}

	if err := runGit(ctx, opts.StateRoot, opts.GitTimeout, "worktree", "add", wtDir, opts.Branch); err != nil {
		return nil, err
	}

	if err := finalizeWorktree(ctx, wtDir, opts); err != nil {
		return nil, err
	}

	return &WorktreeInfo{
		WorktreeDir: wtDir,
		WorkDir:     resolveWorkDir(wtDir, opts.RepoType, opts.RepoPath),
	}, nil
}

func ensureWorktreeDirs(stateRoot string) error {
	dirs := []string{
		filepath.Join(stateRoot, ".worktrees"),
		filepath.Join(stateRoot, ".ai", "runs"),
		filepath.Join(stateRoot, ".ai", "results"),
		filepath.Join(stateRoot, ".ai", "exe-logs"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func resolveWorkDir(wtDir, repoType, repoPath string) string {
	if repoType == "root" {
		return wtDir
	}
	repoPath = strings.TrimSuffix(repoPath, "/")
	repoPath = strings.TrimSuffix(repoPath, "\\")
	if repoPath == "" || repoPath == "." {
		return wtDir
	}
	return filepath.Join(wtDir, repoPath)
}

func finalizeWorktree(ctx context.Context, wtDir string, opts WorktreeOptions) error {
	switch opts.RepoType {
	case "root":
		_ = runGit(ctx, wtDir, opts.GitTimeout, "submodule", "sync", "--recursive")
		_ = runGit(ctx, wtDir, opts.GitTimeout, "submodule", "update", "--init", "--recursive")
	case "directory":
		workDir := resolveWorkDir(wtDir, opts.RepoType, opts.RepoPath)
		if _, err := os.Stat(workDir); err != nil {
			_ = removeWorktree(ctx, opts.StateRoot, wtDir, opts.GitTimeout)
			return fmt.Errorf("directory path not found: %s", workDir)
		}
	case "submodule":
		_ = runGit(ctx, wtDir, opts.GitTimeout, "submodule", "sync", opts.RepoPath)
		if err := runGit(ctx, wtDir, opts.GitTimeout, "submodule", "update", "--init", "--recursive", opts.RepoPath); err != nil {
			_ = removeWorktree(ctx, opts.StateRoot, wtDir, opts.GitTimeout)
			return err
		}
		submoduleDir := resolveWorkDir(wtDir, opts.RepoType, opts.RepoPath)
		if _, err := os.Stat(submoduleDir); err != nil {
			_ = removeWorktree(ctx, opts.StateRoot, wtDir, opts.GitTimeout)
			return fmt.Errorf("submodule directory missing: %s", submoduleDir)
		}
		if _, err := os.Stat(filepath.Join(submoduleDir, ".git")); err != nil {
			_ = removeWorktree(ctx, opts.StateRoot, wtDir, opts.GitTimeout)
			return fmt.Errorf("submodule missing .git: %s", submoduleDir)
		}
	default:
		_ = removeWorktree(ctx, opts.StateRoot, wtDir, opts.GitTimeout)
		return fmt.Errorf("unknown repo type: %s", opts.RepoType)
	}

	return nil
}

func removeWorktree(ctx context.Context, root, wtDir string, timeout time.Duration) error {
	ctx, cancel := withOptionalTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "worktree", "remove", "--force", wtDir)
	return cmd.Run()
}

func ensureWorktreeBranch(ctx context.Context, root, branch, baseBranch string, timeout time.Duration) error {
	if branch == "" {
		return fmt.Errorf("branch name is empty")
	}
	if err := runGit(ctx, root, timeout, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return nil
	}
	if err := runGit(ctx, root, timeout, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch); err == nil {
		return runGit(ctx, root, timeout, "branch", branch, "origin/"+branch)
	}
	return runGit(ctx, root, timeout, "branch", branch, baseBranch)
}

// cleanWorktree resets and cleans the worktree to remove leftover changes.
// This prevents "working tree not clean" errors when reusing worktrees (Issue #122).
func cleanWorktree(ctx context.Context, wtDir string, timeout time.Duration, logf func(string, ...interface{})) error {
	// Step 1: Remove stale index lock if present
	lockPath := filepath.Join(wtDir, ".git", "index.lock")
	if _, err := os.Stat(lockPath); err == nil {
		if rmErr := os.Remove(lockPath); rmErr != nil {
			if logf != nil {
				logf("WARNING: failed to remove worktree index.lock: %v", rmErr)
			}
		} else if logf != nil {
			logf("Removed stale worktree index.lock")
		}
	}

	// Step 2: Hard reset to HEAD to discard staged/unstaged changes
	if err := runGit(ctx, wtDir, timeout, "reset", "--hard", "HEAD"); err != nil {
		return fmt.Errorf("git reset --hard failed: %w", err)
	}

	// Step 3: Clean untracked files and directories
	if err := runGit(ctx, wtDir, timeout, "clean", "-fd"); err != nil {
		return fmt.Errorf("git clean -fd failed: %w", err)
	}

	if logf != nil {
		logf("Cleaned worktree: %s", wtDir)
	}
	return nil
}
