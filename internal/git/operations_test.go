package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Helper function to create a temporary git repository
func setupTempGitRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	cmd.Run()

	// Create initial commit
	readme := filepath.Join(dir, "README.md")
	os.WriteFile(readme, []byte("# Test"), 0644)
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	cmd.Run()

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	return dir
}

// TestSubmoduleFileBoundary tests submodule file boundary checking.
// Property 14: Submodule File Boundary

func TestChangesWithinBoundaryAllowed(t *testing.T) {
	// Test changes within submodule boundary are allowed
	dir := setupTempGitRepo(t)

	// Create a simple directory structure
	backendDir := filepath.Join(dir, "backend")
	os.MkdirAll(backendDir, 0755)
	os.WriteFile(filepath.Join(backendDir, "main.go"), []byte("package main"), 0644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Add backend")
	cmd.Dir = dir
	cmd.Run()

	// Make change within submodule path
	os.WriteFile(filepath.Join(backendDir, "new_file.go"), []byte("package main"), 0644)
	cmd = exec.Command("git", "add", "backend/new_file.go")
	cmd.Dir = dir
	cmd.Run()

	isValid, outsideFiles := CheckSubmoduleBoundary(dir, "backend", false)

	if !isValid {
		t.Error("expected changes within boundary to be valid")
	}
	if len(outsideFiles) != 0 {
		t.Errorf("expected no outside files, got %v", outsideFiles)
	}
}

func TestChangesOutsideBoundaryRejected(t *testing.T) {
	// Test changes outside submodule boundary are rejected
	dir := setupTempGitRepo(t)

	// Create a simple directory structure
	backendDir := filepath.Join(dir, "backend")
	os.MkdirAll(backendDir, 0755)
	os.WriteFile(filepath.Join(backendDir, "main.go"), []byte("package main"), 0644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Add backend")
	cmd.Dir = dir
	cmd.Run()

	// Make change outside submodule
	os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("outside content"), 0644)
	cmd = exec.Command("git", "add", "outside.txt")
	cmd.Dir = dir
	cmd.Run()

	isValid, outsideFiles := CheckSubmoduleBoundary(dir, "backend", false)

	if isValid {
		t.Error("expected changes outside boundary to be rejected")
	}
	found := false
	for _, f := range outsideFiles {
		if f == "outside.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'outside.txt' in outside files, got %v", outsideFiles)
	}
}

func TestChangesOutsideBoundaryAllowedWithFlag(t *testing.T) {
	// Test changes outside boundary allowed with allowParent flag
	dir := setupTempGitRepo(t)

	// Create a simple directory structure
	backendDir := filepath.Join(dir, "backend")
	os.MkdirAll(backendDir, 0755)
	os.WriteFile(filepath.Join(backendDir, "main.go"), []byte("package main"), 0644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Add backend")
	cmd.Dir = dir
	cmd.Run()

	// Make change outside submodule
	os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("outside content"), 0644)
	cmd = exec.Command("git", "add", "outside.txt")
	cmd.Dir = dir
	cmd.Run()

	isValid, outsideFiles := CheckSubmoduleBoundary(dir, "backend", true)

	if !isValid {
		t.Error("expected changes outside boundary to be allowed with flag")
	}
	// Still reported but allowed
	found := false
	for _, f := range outsideFiles {
		if f == "outside.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'outside.txt' in outside files (reported but allowed), got %v", outsideFiles)
	}
}

// TestSubmoduleGitOperationsOrder tests submodule git operations order.
// Property 5: Submodule Git Operations Order

func setupRepoWithSubmodule(t *testing.T) string {
	t.Helper()
	dir := setupTempGitRepo(t)

	// Create submodule directory
	submoduleDir := filepath.Join(dir, "backend")
	os.MkdirAll(submoduleDir, 0755)
	cmd := exec.Command("git", "init")
	cmd.Dir = submoduleDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = submoduleDir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = submoduleDir
	cmd.Run()
	os.WriteFile(filepath.Join(submoduleDir, "main.go"), []byte("package main"), 0644)
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = submoduleDir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Initial")
	cmd.Dir = submoduleDir
	cmd.Run()

	// Add to parent
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Add backend")
	cmd.Dir = dir
	cmd.Run()

	return dir
}

func TestCommitOrderSubmoduleFirst(t *testing.T) {
	// Test submodule is committed before parent
	dir := setupRepoWithSubmodule(t)
	state := NewSubmoduleState()

	// Make change in submodule
	os.WriteFile(filepath.Join(dir, "backend", "new_file.go"), []byte("package main\n// new"), 0644)

	success, errMsg := GitCommitSubmodule(dir, "backend", "test commit", state)

	if !success {
		t.Errorf("expected success, got error: %s", errMsg)
	}
	if !VerifyCommitOrder(state.OperationsLog) {
		t.Error("expected commit order to be correct")
	}

	// Find indices manually to verify
	submoduleCommitIdx := -1
	parentCommitIdx := -1
	for i, op := range state.OperationsLog {
		if op == "submodule_commit" {
			submoduleCommitIdx = i
		}
		if op == "parent_commit" {
			parentCommitIdx = i
		}
	}
	if submoduleCommitIdx >= parentCommitIdx {
		t.Error("expected submodule_commit to come before parent_commit")
	}
}

func TestSubmoduleSHARecorded(t *testing.T) {
	// Test submodule SHA is recorded after commit
	dir := setupRepoWithSubmodule(t)
	state := NewSubmoduleState()

	// Make change in submodule
	os.WriteFile(filepath.Join(dir, "backend", "new_file.go"), []byte("package main\n// new"), 0644)

	success, errMsg := GitCommitSubmodule(dir, "backend", "test commit", state)

	if !success {
		t.Errorf("expected success, got error: %s", errMsg)
	}
	if state.SubmoduleSHA == "" {
		t.Error("expected submodule SHA to be recorded")
	}
	if len(state.SubmoduleSHA) != 40 {
		t.Errorf("expected SHA-1 hash length 40, got %d", len(state.SubmoduleSHA))
	}
}

func TestParentSHARecorded(t *testing.T) {
	// Test parent SHA is recorded after commit
	dir := setupRepoWithSubmodule(t)
	state := NewSubmoduleState()

	// Make change in submodule
	os.WriteFile(filepath.Join(dir, "backend", "new_file.go"), []byte("package main\n// new"), 0644)

	success, errMsg := GitCommitSubmodule(dir, "backend", "test commit", state)

	if !success {
		t.Errorf("expected success, got error: %s", errMsg)
	}
	if state.ParentSHA == "" {
		t.Error("expected parent SHA to be recorded")
	}
	if len(state.ParentSHA) != 40 {
		t.Errorf("expected SHA-1 hash length 40, got %d", len(state.ParentSHA))
	}
}

// TestSubmoduleConsistencyTracking tests submodule consistency tracking.
// Property 16: Submodule Consistency Tracking

func TestConsistencyStatusInitial(t *testing.T) {
	// Test initial consistency status is consistent
	state := NewSubmoduleState()
	if state.ConsistencyStatus != "consistent" {
		t.Errorf("expected 'consistent', got %q", state.ConsistencyStatus)
	}
}

func TestConsistencyStatusOnNoChanges(t *testing.T) {
	// Test consistency status when no changes
	dir := setupRepoWithSubmodule(t)
	state := NewSubmoduleState()

	// Try to commit with no changes
	success, errMsg := GitCommitSubmodule(dir, "backend", "test", state)

	if success {
		t.Error("expected failure when no changes")
	}
	if errMsg != "no changes in submodule" {
		t.Errorf("expected 'no changes in submodule', got %q", errMsg)
	}
}

// TestDirectoryTypeGitOperations tests directory type git operations.
// Property 22: Directory Type Git Operations

func TestCommitFromWorktreeRoot(t *testing.T) {
	// Test commit executes from worktree root
	dir := setupTempGitRepo(t)

	// Create subdirectory
	subdir := filepath.Join(dir, "backend")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "main.go"), []byte("package main"), 0644)

	success, errMsg := GitCommitDirectory(dir, "backend", "test commit")

	if !success {
		t.Errorf("expected success, got error: %s", errMsg)
	}
}

func TestAllChangesIncluded(t *testing.T) {
	// Test all changes are included in commit
	dir := setupTempGitRepo(t)

	// Create changes in multiple locations
	subdir := filepath.Join(dir, "backend")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "root_file.txt"), []byte("root content"), 0644)

	success, errMsg := GitCommitDirectory(dir, "backend", "test commit")

	if !success {
		t.Errorf("expected success, got error: %s", errMsg)
	}

	// Verify both files are committed
	files, err := GetCommittedFiles(dir)
	if err != nil {
		t.Fatalf("failed to get committed files: %v", err)
	}

	foundBackend := false
	foundRoot := false
	for _, f := range files {
		if f == "backend/main.go" {
			foundBackend = true
		}
		if f == "root_file.txt" {
			foundRoot = true
		}
	}
	if !foundBackend {
		t.Error("expected 'backend/main.go' in committed files")
	}
	if !foundRoot {
		t.Error("expected 'root_file.txt' in committed files")
	}
}

func TestNoChangesReturnsFalse(t *testing.T) {
	// Test returns false when no changes
	dir := setupTempGitRepo(t)

	success, errMsg := GitCommitDirectory(dir, "backend", "test commit")

	if success {
		t.Error("expected failure when no changes")
	}
	if errMsg != "no changes" {
		t.Errorf("expected 'no changes', got %q", errMsg)
	}
}

// TestVerifyCommitOrder tests commit order verification function

func TestCorrectOrder(t *testing.T) {
	// Test correct order is verified
	log := []string{"submodule_stage", "submodule_commit", "parent_stage_submodule_ref", "parent_commit"}
	if !VerifyCommitOrder(log) {
		t.Error("expected correct order to be verified as true")
	}
}

func TestIncorrectOrder(t *testing.T) {
	// Test incorrect order is detected
	log := []string{"parent_stage_submodule_ref", "submodule_stage", "submodule_commit", "parent_commit"}
	if VerifyCommitOrder(log) {
		t.Error("expected incorrect order to be detected as false")
	}
}

func TestEmptyLog(t *testing.T) {
	// Test empty log returns true
	if !VerifyCommitOrder([]string{}) {
		t.Error("expected empty log to return true")
	}
}

func TestPartialLog(t *testing.T) {
	// Test partial log returns true
	log := []string{"submodule_stage"}
	if !VerifyCommitOrder(log) {
		t.Error("expected partial log to return true")
	}
}
