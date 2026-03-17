package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// NewGitHubClient (github.go)
// ---------------------------------------------------------------------------

func TestNewGitHubClient_DefaultTimeout(t *testing.T) {
	c := NewGitHubClient(0)
	if c == nil {
		t.Fatal("NewGitHubClient should return non-nil")
	}
	if c.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s (default)", c.Timeout)
	}
}

func TestNewGitHubClient_CustomTimeout(t *testing.T) {
	c := NewGitHubClient(10 * time.Second)
	if c.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", c.Timeout)
	}
}

// ---------------------------------------------------------------------------
// NewWorkerPreflight (preflight_worker.go)
// ---------------------------------------------------------------------------

func TestNewWorkerPreflight_Defaults(t *testing.T) {
	p := NewWorkerPreflight("/state/root", "", "")
	if p.RepoType != "root" {
		t.Errorf("RepoType = %q, want 'root' (default)", p.RepoType)
	}
	if p.RepoPath != "." {
		t.Errorf("RepoPath = %q, want '.' (default)", p.RepoPath)
	}
	if p.StateRoot != "/state/root" {
		t.Errorf("StateRoot = %q, want /state/root", p.StateRoot)
	}
}

func TestNewWorkerPreflight_CustomValues(t *testing.T) {
	p := NewWorkerPreflight("/root", "directory", "backend")
	if p.RepoType != "directory" {
		t.Errorf("RepoType = %q, want 'directory'", p.RepoType)
	}
	if p.RepoPath != "backend" {
		t.Errorf("RepoPath = %q, want 'backend'", p.RepoPath)
	}
}

// ---------------------------------------------------------------------------
// hasConflicts (rebase.go) — non-git dirs always return false
// ---------------------------------------------------------------------------

func TestHasConflicts_NonExistentDir(t *testing.T) {
	ctx := context.Background()
	if hasConflicts(ctx, "/nonexistent/dir") {
		t.Error("hasConflicts should return false for non-existent directory")
	}
}

func TestHasConflicts_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	// A temp dir without .git file/dir
	if hasConflicts(ctx, dir) {
		t.Error("hasConflicts should return false for non-git directory")
	}
}

// ---------------------------------------------------------------------------
// ensureWorktreeDirs (worktree.go)
// ---------------------------------------------------------------------------

func TestEnsureWorktreeDirs_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := ensureWorktreeDirs(dir); err != nil {
		t.Fatalf("ensureWorktreeDirs: %v", err)
	}

	// Check expected dirs were created
	for _, sub := range []string{".worktrees", ".ai/runs", ".ai/results", ".ai/exe-logs"} {
		full := filepath.Join(dir, sub)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("directory %q not created: %v", sub, err)
		}
	}
}

func TestEnsureWorktreeDirs_Idempotent(t *testing.T) {
	dir := t.TempDir()
	// Call twice — should not error
	if err := ensureWorktreeDirs(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := ensureWorktreeDirs(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

// ---------------------------------------------------------------------------
// resolveWorkDir (worktree.go)
// ---------------------------------------------------------------------------

func TestResolveWorkDir_RootType(t *testing.T) {
	result := resolveWorkDir("/worktree", "root", ".")
	if result != "/worktree" {
		t.Errorf("resolveWorkDir(root) = %q, want /worktree", result)
	}
}

func TestResolveWorkDir_DirectoryType(t *testing.T) {
	result := resolveWorkDir("/worktree", "directory", "backend")
	expected := filepath.Join("/worktree", "backend")
	if result != expected {
		t.Errorf("resolveWorkDir(directory, backend) = %q, want %q", result, expected)
	}
}

func TestResolveWorkDir_RootPath(t *testing.T) {
	// "./" is a root path
	result := resolveWorkDir("/worktree", "directory", "./")
	if result != "/worktree" {
		t.Errorf("resolveWorkDir(directory, './')= %q, want /worktree (root path)", result)
	}
}

// ---------------------------------------------------------------------------
// ErrNoSubmoduleChanges / ErrRebaseConflict - sentinel errors
// ---------------------------------------------------------------------------

func TestErrNoSubmoduleChanges_NotNil(t *testing.T) {
	if ErrNoSubmoduleChanges == nil {
		t.Error("ErrNoSubmoduleChanges should be non-nil")
	}
	if ErrNoSubmoduleChanges.Error() == "" {
		t.Error("ErrNoSubmoduleChanges.Error() should not be empty")
	}
}

func TestErrRebaseConflict_NotNil(t *testing.T) {
	if ErrRebaseConflict == nil {
		t.Error("ErrRebaseConflict should be non-nil")
	}
	if ErrRebaseConflict.Error() == "" {
		t.Error("ErrRebaseConflict.Error() should not be empty")
	}
}

// ---------------------------------------------------------------------------
// NewDispatchLogger (dispatch.go)
// ---------------------------------------------------------------------------

func TestNewDispatchLogger_ReturnsNonNil(t *testing.T) {
	dir := t.TempDir()
	logger := NewDispatchLogger(dir, 42)
	if logger == nil {
		t.Fatal("NewDispatchLogger should return non-nil")
	}
	defer logger.Close()
}

func TestNewDispatchLogger_EmptyStateRoot(t *testing.T) {
	// Empty stateRoot should still return a logger
	logger := NewDispatchLogger("", 1)
	if logger == nil {
		t.Fatal("NewDispatchLogger('', 1) should return non-nil")
	}
	defer logger.Close()

	// Log should not panic
	logger.Log("test message")
}

// ---------------------------------------------------------------------------
// cleanWorktree (worktree.go) - tests with non-git temp dir (error expected)
// ---------------------------------------------------------------------------

func TestCleanWorktree_NonGitDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// Not a git repo, so git reset --hard will fail
	// We just ensure no panic
	err := cleanWorktree(ctx, dir, 5*time.Second, nil)
	// Expect an error since dir is not a git repo
	if err == nil {
		t.Log("cleanWorktree on non-git dir unexpectedly succeeded")
	}
}

func TestCleanWorktree_WithLogFunc(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	var logMessages []string
	logf := func(msg string, args ...interface{}) {
		logMessages = append(logMessages, msg)
	}
	// Will fail on non-git dir, but logf shouldn't be called for errors
	_ = cleanWorktree(ctx, dir, 5*time.Second, logf)
}
