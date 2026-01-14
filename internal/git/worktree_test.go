package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestWorktreePathConsistency tests worktree path consistency.
// Property 2: Worktree Path Consistency

func TestWorktreePathFormat(t *testing.T) {
	// Test worktree path follows expected format
	root := "/tmp/test-repo"
	issueID := "123"
	expected := filepath.Join(root, ".worktrees", "issue-123")
	actual := GetWorktreePath(root, issueID)
	if actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}

func TestWorktreePathVariousIDs(t *testing.T) {
	// Test worktree path works for various issue IDs (table-driven)
	root := "/tmp/test-repo"
	tests := []struct {
		issueID  string
		expected string
	}{
		{"1", filepath.Join(root, ".worktrees", "issue-1")},
		{"42", filepath.Join(root, ".worktrees", "issue-42")},
		{"999", filepath.Join(root, ".worktrees", "issue-999")},
		{"12345", filepath.Join(root, ".worktrees", "issue-12345")},
	}

	for _, tt := range tests {
		t.Run("issue-"+tt.issueID, func(t *testing.T) {
			actual := GetWorktreePath(root, tt.issueID)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestWorkDirRootType(t *testing.T) {
	// Test WORK_DIR for root type equals worktree
	worktree := "/tmp/worktrees/issue-1"
	workDir := GetWorkDir(worktree, "root", ".")
	if workDir != worktree {
		t.Errorf("expected %q, got %q", worktree, workDir)
	}
}

func TestWorkDirDirectoryType(t *testing.T) {
	// Test WORK_DIR for directory type includes repo path
	worktree := "/tmp/worktrees/issue-1"
	expected := filepath.Join(worktree, "backend")
	workDir := GetWorkDir(worktree, "directory", "backend")
	if workDir != expected {
		t.Errorf("expected %q, got %q", expected, workDir)
	}
}

func TestWorkDirSubmoduleType(t *testing.T) {
	// Test WORK_DIR for submodule type includes repo path
	worktree := "/tmp/worktrees/issue-1"
	expected := filepath.Join(worktree, "libs", "shared")
	workDir := GetWorkDir(worktree, "submodule", "libs/shared")
	if workDir != expected {
		t.Errorf("expected %q, got %q", expected, workDir)
	}
}

// TestWorktreeIdempotency tests worktree creation idempotency.
// Property 3: Worktree Idempotency

func TestCreateWorktreeFirstTime(t *testing.T) {
	// Test worktree is created on first call
	dir := setupTempGitRepo(t)
	issueID := "1"
	branch := "feat/ai-issue-1"

	wtPath, err := CreateWorktree(dir, issueID, branch, "master")
	if err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	if wtPath != GetWorktreePath(dir, issueID) {
		t.Errorf("expected %q, got %q", GetWorktreePath(dir, issueID), wtPath)
	}

	info, err := os.Stat(wtPath)
	if err != nil || !info.IsDir() {
		t.Error("expected worktree directory to exist")
	}

	// Cleanup
	RemoveWorktree(dir, issueID)
}

func TestCreateWorktreeIdempotent(t *testing.T) {
	// Test calling CreateWorktree twice returns same path
	dir := setupTempGitRepo(t)
	issueID := "2"
	branch := "feat/ai-issue-2"

	// First call
	wtPath1, err := CreateWorktree(dir, issueID, branch, "master")
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Second call (should be idempotent)
	wtPath2, err := CreateWorktree(dir, issueID, branch, "master")
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}

	if wtPath1 != wtPath2 {
		t.Errorf("expected paths to be equal: %q != %q", wtPath1, wtPath2)
	}

	info, err := os.Stat(wtPath1)
	if err != nil || !info.IsDir() {
		t.Error("expected worktree directory to exist")
	}

	// Cleanup
	RemoveWorktree(dir, issueID)
}

func TestWorktreeExistsCheck(t *testing.T) {
	// Test WorktreeExists returns correct status
	dir := setupTempGitRepo(t)
	issueID := "3"
	branch := "feat/ai-issue-3"

	// Before creation
	if WorktreeExists(dir, issueID) {
		t.Error("expected worktree to not exist before creation")
	}

	// After creation
	CreateWorktree(dir, issueID, branch, "master")
	if !WorktreeExists(dir, issueID) {
		t.Error("expected worktree to exist after creation")
	}

	// After removal
	RemoveWorktree(dir, issueID)
	if WorktreeExists(dir, issueID) {
		t.Error("expected worktree to not exist after removal")
	}
}

// TestWorktreeDirectoryVerification tests directory verification in worktree.
// Validates: Requirements 3.3, 14.4

func TestVerifyDirectoryExistsTrue(t *testing.T) {
	// Test directory verification passes for existing directory
	dir := setupTempGitRepo(t)
	issueID := "4"
	branch := "feat/ai-issue-4"

	// Create directory in main repo
	backendDir := filepath.Join(dir, "backend")
	os.MkdirAll(backendDir, 0755)
	os.WriteFile(filepath.Join(backendDir, ".gitkeep"), []byte(""), 0644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Add backend")
	cmd.Dir = dir
	cmd.Run()

	// Create worktree
	wtPath, err := CreateWorktree(dir, issueID, branch, "master")
	if err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Verify directory exists in worktree
	if !VerifyDirectoryExists(wtPath, "backend") {
		t.Error("expected directory to exist in worktree")
	}

	// Cleanup
	RemoveWorktree(dir, issueID)
}

func TestVerifyDirectoryExistsFalse(t *testing.T) {
	// Test directory verification fails for non-existing directory
	dir := setupTempGitRepo(t)
	issueID := "5"
	branch := "feat/ai-issue-5"

	// Create worktree
	wtPath, err := CreateWorktree(dir, issueID, branch, "master")
	if err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Verify non-existing directory
	if VerifyDirectoryExists(wtPath, "nonexistent") {
		t.Error("expected directory verification to fail for non-existing directory")
	}

	// Cleanup
	RemoveWorktree(dir, issueID)
}

// TestSubmoduleInitialization tests submodule initialization in worktree.
// Property 4: Submodule Initialization in Worktree

func TestVerifySubmoduleInitializedWithGitFile(t *testing.T) {
	// Test submodule verification passes when .git file exists
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create directory with .git file (simulating submodule structure)
	backendDir := filepath.Join(dir, "backend")
	os.MkdirAll(backendDir, 0755)
	os.WriteFile(filepath.Join(backendDir, ".git"), []byte("gitdir: ../.git/modules/backend"), 0644)
	os.WriteFile(filepath.Join(backendDir, "README.md"), []byte("# Backend"), 0644)

	// Verify submodule is initialized (has .git file)
	if !VerifySubmoduleInitialized(dir, "backend") {
		t.Error("expected submodule verification to pass with .git file")
	}
}

func TestVerifySubmoduleInitializedWithGitDir(t *testing.T) {
	// Test submodule verification passes when .git directory exists
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create directory with actual .git directory
	backendDir := filepath.Join(dir, "backend")
	os.MkdirAll(backendDir, 0755)

	// Initialize as git repo (creates .git directory)
	cmd := exec.Command("git", "init")
	cmd.Dir = backendDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = backendDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = backendDir
	cmd.Run()
	os.WriteFile(filepath.Join(backendDir, "README.md"), []byte("# Backend"), 0644)
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = backendDir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Initial")
	cmd.Dir = backendDir
	cmd.Run()

	// Verify .git exists
	if !VerifySubmoduleInitialized(dir, "backend") {
		t.Error("expected submodule verification to pass with .git directory")
	}
}

func TestVerifySubmoduleInitializedFalse(t *testing.T) {
	// Test submodule verification fails for non-submodule directory
	dir := setupTempGitRepo(t)
	issueID := "7"
	branch := "feat/ai-issue-7"

	// Create regular directory (not a submodule)
	backendDir := filepath.Join(dir, "backend")
	os.MkdirAll(backendDir, 0755)
	os.WriteFile(filepath.Join(backendDir, "README.md"), []byte("# Backend"), 0644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Add backend")
	cmd.Dir = dir
	cmd.Run()

	// Create worktree
	wtPath, err := CreateWorktree(dir, issueID, branch, "master")
	if err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Verify submodule is NOT initialized (no .git)
	if VerifySubmoduleInitialized(wtPath, "backend") {
		t.Error("expected submodule verification to fail for non-submodule directory")
	}

	// Cleanup
	RemoveWorktree(dir, issueID)
}

func TestVerifyFunctionChecksGitExistence(t *testing.T) {
	// Test VerifySubmoduleInitialized checks for .git existence
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create directory without .git
	subdir := filepath.Join(dir, "subdir")
	os.MkdirAll(subdir, 0755)
	if VerifySubmoduleInitialized(dir, "subdir") {
		t.Error("expected verification to fail without .git")
	}

	// Add .git file
	os.WriteFile(filepath.Join(subdir, ".git"), []byte("gitdir: somewhere"), 0644)
	if !VerifySubmoduleInitialized(dir, "subdir") {
		t.Error("expected verification to pass with .git file")
	}
}

// TestWorktreeBranchManagement tests branch management during worktree creation.
// Validates: Requirements 14.1, 14.2

func TestBranchCreatedFromBase(t *testing.T) {
	// Test branch is created from base branch if it doesn't exist
	dir := setupTempGitRepo(t)
	issueID := "8"
	branch := "feat/ai-issue-8"

	// Verify branch doesn't exist
	if BranchExists(dir, branch) {
		t.Error("expected branch to not exist initially")
	}

	// Create worktree (should create branch)
	_, err := CreateWorktree(dir, issueID, branch, "master")
	if err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Verify branch now exists
	if !BranchExists(dir, branch) {
		t.Error("expected branch to exist after worktree creation")
	}

	// Cleanup
	RemoveWorktree(dir, issueID)
}

func TestExistingBranchReused(t *testing.T) {
	// Test existing branch is reused
	dir := setupTempGitRepo(t)
	issueID := "9"
	branch := "feat/ai-issue-9"

	// Create branch first
	CreateBranch(dir, branch, "master")

	// Get branch SHA before worktree
	shaBefore, err := GetBranchSHA(dir, branch)
	if err != nil {
		t.Fatalf("failed to get branch SHA: %v", err)
	}

	// Create worktree
	_, err = CreateWorktree(dir, issueID, branch, "master")
	if err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Get branch SHA after worktree
	shaAfter, err := GetBranchSHA(dir, branch)
	if err != nil {
		t.Fatalf("failed to get branch SHA: %v", err)
	}

	// SHA should be the same (branch reused, not recreated)
	if shaBefore != shaAfter {
		t.Errorf("expected SHA to be unchanged: %q != %q", shaBefore, shaAfter)
	}

	// Cleanup
	RemoveWorktree(dir, issueID)
}

// TestWorktreeValidRepoTypes tests worktree creation for all valid repo types.

func TestWorkDirCalculation(t *testing.T) {
	// Test WORK_DIR is calculated correctly for all repo types (table-driven)
	worktree := "/tmp/worktrees/issue-1"
	tests := []struct {
		name     string
		repoType string
		repoPath string
		expected string
	}{
		{"root type", "root", ".", worktree},
		{"directory type", "directory", "backend", filepath.Join(worktree, "backend")},
		{"submodule type", "submodule", "libs/shared", filepath.Join(worktree, "libs", "shared")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir := GetWorkDir(worktree, tt.repoType, tt.repoPath)
			if workDir != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, workDir)
			}
		})
	}
}
