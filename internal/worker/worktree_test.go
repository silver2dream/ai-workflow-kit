package worker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanWorktree(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a temporary git repo
	tmpDir := t.TempDir()
	ctx := context.Background()
	timeout := 30 * time.Second

	// Initialize git repo
	if err := runGit(ctx, tmpDir, timeout, "init"); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git user for commits
	_ = runGit(ctx, tmpDir, timeout, "config", "user.email", "test@test.com")
	_ = runGit(ctx, tmpDir, timeout, "config", "user.name", "Test")

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := runGit(ctx, tmpDir, timeout, "add", "."); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := runGit(ctx, tmpDir, timeout, "commit", "-m", "initial"); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	t.Run("cleans staged changes", func(t *testing.T) {
		// Create staged changes
		if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
			t.Fatalf("failed to modify test file: %v", err)
		}
		if err := runGit(ctx, tmpDir, timeout, "add", "."); err != nil {
			t.Fatalf("git add failed: %v", err)
		}

		// Clean
		if err := cleanWorktree(ctx, tmpDir, timeout, nil); err != nil {
			t.Fatalf("cleanWorktree failed: %v", err)
		}

		// Verify file is reset
		content, _ := os.ReadFile(testFile)
		if string(content) != "initial content" {
			t.Errorf("file not reset: got %q, want %q", string(content), "initial content")
		}
	})

	t.Run("cleans unstaged changes", func(t *testing.T) {
		// Create unstaged changes
		if err := os.WriteFile(testFile, []byte("unstaged content"), 0644); err != nil {
			t.Fatalf("failed to modify test file: %v", err)
		}

		// Clean
		if err := cleanWorktree(ctx, tmpDir, timeout, nil); err != nil {
			t.Fatalf("cleanWorktree failed: %v", err)
		}

		// Verify file is reset
		content, _ := os.ReadFile(testFile)
		if string(content) != "initial content" {
			t.Errorf("file not reset: got %q, want %q", string(content), "initial content")
		}
	})

	t.Run("cleans untracked files", func(t *testing.T) {
		// Create untracked file
		untrackedFile := filepath.Join(tmpDir, "untracked.txt")
		if err := os.WriteFile(untrackedFile, []byte("untracked"), 0644); err != nil {
			t.Fatalf("failed to create untracked file: %v", err)
		}

		// Clean
		if err := cleanWorktree(ctx, tmpDir, timeout, nil); err != nil {
			t.Fatalf("cleanWorktree failed: %v", err)
		}

		// Verify untracked file is removed
		if _, err := os.Stat(untrackedFile); !os.IsNotExist(err) {
			t.Error("untracked file should be removed")
		}
	})

	t.Run("removes stale index.lock", func(t *testing.T) {
		// Create fake index.lock
		lockPath := filepath.Join(tmpDir, ".git", "index.lock")
		if err := os.WriteFile(lockPath, []byte("lock"), 0644); err != nil {
			t.Fatalf("failed to create index.lock: %v", err)
		}

		var logMsg string
		logf := func(format string, args ...interface{}) {
			logMsg = format
		}

		// Clean
		if err := cleanWorktree(ctx, tmpDir, timeout, logf); err != nil {
			t.Fatalf("cleanWorktree failed: %v", err)
		}

		// Verify lock is removed
		if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
			t.Error("index.lock should be removed")
		}

		// Verify log message
		if logMsg == "" {
			t.Error("expected log message about removing index.lock")
		}
	})
}

func TestAutoCleanRootRepository(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	ctx := context.Background()
	timeout := 30 * time.Second

	// Initialize git repo
	if err := runGit(ctx, tmpDir, timeout, "init"); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git user
	_ = runGit(ctx, tmpDir, timeout, "config", "user.email", "test@test.com")
	_ = runGit(ctx, tmpDir, timeout, "config", "user.name", "Test")

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := runGit(ctx, tmpDir, timeout, "add", "."); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := runGit(ctx, tmpDir, timeout, "commit", "-m", "initial"); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	t.Run("cleans dirty repository", func(t *testing.T) {
		// Make repo dirty
		if err := os.WriteFile(testFile, []byte("dirty"), 0644); err != nil {
			t.Fatalf("failed to modify test file: %v", err)
		}

		// Clean
		err := autoCleanRootRepository(ctx, tmpDir, timeout, nil)
		if err != nil {
			t.Fatalf("autoCleanRootRepository failed: %v", err)
		}

		// Verify cleaned
		content, _ := os.ReadFile(testFile)
		if string(content) != "initial" {
			t.Errorf("file not reset: got %q, want %q", string(content), "initial")
		}
	})

	t.Run("skips clean repository", func(t *testing.T) {
		// Repo should be clean now
		var logCalled bool
		logf := func(format string, args ...interface{}) {
			logCalled = true
		}

		err := autoCleanRootRepository(ctx, tmpDir, timeout, logf)
		if err != nil {
			t.Fatalf("autoCleanRootRepository failed: %v", err)
		}

		// Should not log anything if repo was clean
		if logCalled {
			t.Error("should not log when repo is clean")
		}
	})
}
