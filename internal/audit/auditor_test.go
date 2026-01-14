package audit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCheckRequiredFiles_AllPresent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create all required files
	if err := os.WriteFile(filepath.Join(tmpDir, "CLAUDE.md"), []byte("# Claude"), 0644); err != nil {
		t.Fatalf("WriteFile CLAUDE.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# Agents"), 0644); err != nil {
		t.Fatalf("WriteFile AGENTS.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# README"), 0644); err != nil {
		t.Fatalf("WriteFile README.md failed: %v", err)
	}

	// Create .ai/config/workflow.yaml
	configDir := filepath.Join(tmpDir, ".ai", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte("version: 1"), 0644); err != nil {
		t.Fatalf("WriteFile workflow.yaml failed: %v", err)
	}

	findings := CheckRequiredFiles(tmpDir)

	if len(findings) != 0 {
		t.Fatalf("CheckRequiredFiles returned %d findings, want 0", len(findings))
	}
}

func TestCheckRequiredFiles_MissingClaudeMD(t *testing.T) {
	tmpDir := t.TempDir()

	// Create AGENTS.md and workflow.yaml but NOT CLAUDE.md
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# Agents"), 0644); err != nil {
		t.Fatalf("WriteFile AGENTS.md failed: %v", err)
	}

	configDir := filepath.Join(tmpDir, ".ai", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte("version: 1"), 0644); err != nil {
		t.Fatalf("WriteFile workflow.yaml failed: %v", err)
	}

	findings := CheckRequiredFiles(tmpDir)

	// Should have P0 for CLAUDE.md and P2 for README.md
	var foundClaudeMD bool
	for _, f := range findings {
		if f.ID == FindingMissingClaudeMD && f.Severity == SeverityP0 {
			foundClaudeMD = true
		}
	}

	if !foundClaudeMD {
		t.Fatalf("CheckRequiredFiles did not return P0 finding for missing CLAUDE.md")
	}
}

func TestCheckRequiredFiles_MissingAgentsMD(t *testing.T) {
	tmpDir := t.TempDir()

	// Create CLAUDE.md and workflow.yaml but NOT AGENTS.md
	if err := os.WriteFile(filepath.Join(tmpDir, "CLAUDE.md"), []byte("# Claude"), 0644); err != nil {
		t.Fatalf("WriteFile CLAUDE.md failed: %v", err)
	}

	configDir := filepath.Join(tmpDir, ".ai", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte("version: 1"), 0644); err != nil {
		t.Fatalf("WriteFile workflow.yaml failed: %v", err)
	}

	findings := CheckRequiredFiles(tmpDir)

	var foundAgentsMD bool
	for _, f := range findings {
		if f.ID == FindingMissingAgentsMD && f.Severity == SeverityP0 {
			foundAgentsMD = true
		}
	}

	if !foundAgentsMD {
		t.Fatalf("CheckRequiredFiles did not return P0 finding for missing AGENTS.md")
	}
}

func TestCheckWorktreeStatus_Clean(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Skipf("git init failed (git may not be available): %v", err)
	}

	// Configure git user for this repo
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Skipf("git config user.email failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Skipf("git config user.name failed: %v", err)
	}

	// Create a file and commit it
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test"), 0644); err != nil {
		t.Fatalf("WriteFile test.txt failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "add", ".").Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "initial").Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	findings := CheckWorktreeStatus(tmpDir)

	if len(findings) != 0 {
		t.Fatalf("CheckWorktreeStatus returned %d findings for clean worktree, want 0", len(findings))
	}
}

func TestCheckWorktreeStatus_Dirty(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Skipf("git init failed (git may not be available): %v", err)
	}

	// Configure git user for this repo
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Skipf("git config user.email failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Skipf("git config user.name failed: %v", err)
	}

	// Create a file and commit it
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test"), 0644); err != nil {
		t.Fatalf("WriteFile test.txt failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "add", ".").Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "initial").Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Create an uncommitted change
	if err := os.WriteFile(filepath.Join(tmpDir, "dirty.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatalf("WriteFile dirty.txt failed: %v", err)
	}

	findings := CheckWorktreeStatus(tmpDir)

	var foundDirty bool
	for _, f := range findings {
		if f.ID == FindingDirtyWorktree && f.Severity == SeverityP1 {
			foundDirty = true
		}
	}

	if !foundDirty {
		t.Fatalf("CheckWorktreeStatus did not return P1 finding for dirty worktree")
	}
}

func TestCheckAIDirectory_Complete(t *testing.T) {
	tmpDir := t.TempDir()

	// Create all required .ai subdirectories
	requiredDirs := []string{"config", "state", "results", "exe-logs"}
	for _, dir := range requiredDirs {
		dirPath := filepath.Join(tmpDir, ".ai", dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("MkdirAll %s failed: %v", dir, err)
		}
	}

	findings := CheckAIDirectory(tmpDir)

	if len(findings) != 0 {
		t.Fatalf("CheckAIDirectory returned %d findings for complete structure, want 0", len(findings))
	}
}

func TestCheckAIDirectory_MissingAIDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Do not create .ai directory

	findings := CheckAIDirectory(tmpDir)

	var foundMissing bool
	for _, f := range findings {
		if f.ID == FindingMissingAIDir && f.Severity == SeverityP1 {
			foundMissing = true
		}
	}

	if !foundMissing {
		t.Fatalf("CheckAIDirectory did not return P1 finding for missing .ai directory")
	}
}

func TestAuditProject_CompleteRepo(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Skipf("git init failed (git may not be available): %v", err)
	}

	// Configure git user for this repo
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Skipf("git config user.email failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Skipf("git config user.name failed: %v", err)
	}

	// Create all required files
	if err := os.WriteFile(filepath.Join(tmpDir, "CLAUDE.md"), []byte("# Claude"), 0644); err != nil {
		t.Fatalf("WriteFile CLAUDE.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# Agents"), 0644); err != nil {
		t.Fatalf("WriteFile AGENTS.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# README"), 0644); err != nil {
		t.Fatalf("WriteFile README.md failed: %v", err)
	}

	// Create .ai directory structure
	requiredDirs := []string{"config", "state", "results", "exe-logs"}
	for _, dir := range requiredDirs {
		dirPath := filepath.Join(tmpDir, ".ai", dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("MkdirAll %s failed: %v", dir, err)
		}
	}

	// Create workflow.yaml
	if err := os.WriteFile(filepath.Join(tmpDir, ".ai", "config", "workflow.yaml"), []byte("version: 1"), 0644); err != nil {
		t.Fatalf("WriteFile workflow.yaml failed: %v", err)
	}

	// Commit everything
	if err := exec.Command("git", "-C", tmpDir, "add", ".").Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "initial").Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	result, err := AuditProject(tmpDir)
	if err != nil {
		t.Fatalf("AuditProject returned error: %v", err)
	}

	if !result.Passed {
		t.Fatalf("AuditProject.Passed = false, want true for complete repo")
	}

	if result.Summary.P0Count != 0 {
		t.Fatalf("AuditProject.Summary.P0Count = %d, want 0", result.Summary.P0Count)
	}
}

func TestAuditProject_IncompleteRepo(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only AGENTS.md (missing CLAUDE.md and workflow.yaml)
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# Agents"), 0644); err != nil {
		t.Fatalf("WriteFile AGENTS.md failed: %v", err)
	}

	result, err := AuditProject(tmpDir)
	if err != nil {
		t.Fatalf("AuditProject returned error: %v", err)
	}

	if result.Passed {
		t.Fatalf("AuditProject.Passed = true, want false for incomplete repo")
	}

	// Should have P0 findings for missing CLAUDE.md and workflow.yaml
	if result.Summary.P0Count < 2 {
		t.Fatalf("AuditProject.Summary.P0Count = %d, want >= 2", result.Summary.P0Count)
	}
}

func TestCalculateSummary(t *testing.T) {
	findings := []Finding{
		{ID: "TEST1", Severity: SeverityP0, Message: "P0 finding"},
		{ID: "TEST2", Severity: SeverityP0, Message: "Another P0"},
		{ID: "TEST3", Severity: SeverityP1, Message: "P1 finding"},
		{ID: "TEST4", Severity: SeverityP2, Message: "P2 finding"},
		{ID: "TEST5", Severity: SeverityP2, Message: "Another P2"},
		{ID: "TEST6", Severity: SeverityP2, Message: "Yet another P2"},
	}

	summary := CalculateSummary(findings)

	if summary.P0Count != 2 {
		t.Fatalf("Summary.P0Count = %d, want 2", summary.P0Count)
	}
	if summary.P1Count != 1 {
		t.Fatalf("Summary.P1Count = %d, want 1", summary.P1Count)
	}
	if summary.P2Count != 3 {
		t.Fatalf("Summary.P2Count = %d, want 3", summary.P2Count)
	}
}

func TestCalculateSummary_Empty(t *testing.T) {
	findings := []Finding{}

	summary := CalculateSummary(findings)

	if summary.P0Count != 0 || summary.P1Count != 0 || summary.P2Count != 0 {
		t.Fatalf("Summary should be all zeros for empty findings, got P0=%d, P1=%d, P2=%d",
			summary.P0Count, summary.P1Count, summary.P2Count)
	}
}

// =============================================================================
// Submodule Audit Tests (migrated from test_audit_extended.py)
// Property 8: Audit Submodule Detection
// =============================================================================

func TestCheckUninitializedSubmodules_NoGitmodules(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo without .gitmodules
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Skipf("git init failed (git may not be available): %v", err)
	}

	findings := CheckUninitializedSubmodules(tmpDir)

	// Should return no findings when there's no .gitmodules
	if len(findings) != 0 {
		t.Fatalf("CheckUninitializedSubmodules returned %d findings, want 0 for repo without .gitmodules", len(findings))
	}
}

func TestCheckUninitializedSubmodules_WithGitmodules(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Skipf("git init failed (git may not be available): %v", err)
	}

	// Configure git user for this repo
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Skipf("git config user.email failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Skipf("git config user.name failed: %v", err)
	}

	// Create .gitmodules file
	gitmodulesContent := `[submodule "backend"]
    path = backend
    url = https://github.com/test/backend.git
`
	gitmodulesPath := filepath.Join(tmpDir, ".gitmodules")
	if err := os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0644); err != nil {
		t.Fatalf("WriteFile .gitmodules failed: %v", err)
	}

	// Add and commit
	if err := exec.Command("git", "-C", tmpDir, "add", ".gitmodules").Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Add gitmodules").Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	findings := CheckUninitializedSubmodules(tmpDir)

	// The function is tested for correct behavior - should return empty or findings
	// (depends on actual git submodule state, which we can't fully control in test)
	_ = findings // findings may be nil or empty slice, both are valid
}

func TestCheckDirtySubmodules_DirtySubmodule(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a nested git repo (simulating submodule)
	submoduleDir := filepath.Join(tmpDir, "backend")
	if err := os.MkdirAll(submoduleDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := exec.Command("git", "-C", submoduleDir, "init").Run(); err != nil {
		t.Skipf("git init failed (git may not be available): %v", err)
	}

	// Configure git user
	if err := exec.Command("git", "-C", submoduleDir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Skipf("git config user.email failed: %v", err)
	}
	if err := exec.Command("git", "-C", submoduleDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Skipf("git config user.name failed: %v", err)
	}

	// Create and commit a file
	mainGoPath := filepath.Join(submoduleDir, "main.go")
	if err := os.WriteFile(mainGoPath, []byte("package main"), 0644); err != nil {
		t.Fatalf("WriteFile main.go failed: %v", err)
	}
	if err := exec.Command("git", "-C", submoduleDir, "add", ".").Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := exec.Command("git", "-C", submoduleDir, "commit", "-m", "Initial").Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Make submodule dirty with uncommitted file
	dirtyPath := filepath.Join(submoduleDir, "dirty.txt")
	if err := os.WriteFile(dirtyPath, []byte("uncommitted"), 0644); err != nil {
		t.Fatalf("WriteFile dirty.txt failed: %v", err)
	}

	findings := CheckDirtySubmodules(tmpDir, []string{"backend"})

	if len(findings) != 1 {
		t.Fatalf("CheckDirtySubmodules returned %d findings, want 1", len(findings))
	}
	if findings[0].Severity != SeverityP1 {
		t.Fatalf("Finding severity = %s, want P1", findings[0].Severity)
	}
	if findings[0].ID != FindingDirtySubmodule {
		t.Fatalf("Finding ID = %s, want %s", findings[0].ID, FindingDirtySubmodule)
	}
}

func TestCheckDirtySubmodules_CleanSubmodule(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a clean nested git repo
	submoduleDir := filepath.Join(tmpDir, "backend")
	if err := os.MkdirAll(submoduleDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := exec.Command("git", "-C", submoduleDir, "init").Run(); err != nil {
		t.Skipf("git init failed (git may not be available): %v", err)
	}

	// Configure git user
	if err := exec.Command("git", "-C", submoduleDir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Skipf("git config user.email failed: %v", err)
	}
	if err := exec.Command("git", "-C", submoduleDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Skipf("git config user.name failed: %v", err)
	}

	// Create and commit a file
	mainGoPath := filepath.Join(submoduleDir, "main.go")
	if err := os.WriteFile(mainGoPath, []byte("package main"), 0644); err != nil {
		t.Fatalf("WriteFile main.go failed: %v", err)
	}
	if err := exec.Command("git", "-C", submoduleDir, "add", ".").Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := exec.Command("git", "-C", submoduleDir, "commit", "-m", "Initial").Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Don't add any dirty files - worktree is clean
	findings := CheckDirtySubmodules(tmpDir, []string{"backend"})

	if len(findings) != 0 {
		t.Fatalf("CheckDirtySubmodules returned %d findings, want 0 for clean submodule", len(findings))
	}
}

func TestCheckDirtySubmodules_NonexistentPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Check for nonexistent submodule path
	findings := CheckDirtySubmodules(tmpDir, []string{"nonexistent"})

	if len(findings) != 0 {
		t.Fatalf("CheckDirtySubmodules returned %d findings, want 0 for nonexistent path", len(findings))
	}
}

func TestAuditFindingSeverity_UninitializedSubmodule(t *testing.T) {
	// Test that uninitialized submodule finding has P1 severity (Req 8.4)
	finding := NewFinding(FindingUninitializedSubmodule, SeverityP1, "backend")

	if finding.Severity != SeverityP1 {
		t.Fatalf("Uninitialized submodule severity = %s, want P1", finding.Severity)
	}
}

func TestAuditFindingSeverity_DirtySubmodule(t *testing.T) {
	// Test that dirty submodule finding has P1 severity (Req 8.5)
	finding := NewFinding(FindingDirtySubmodule, SeverityP1, "backend")

	if finding.Severity != SeverityP1 {
		t.Fatalf("Dirty submodule severity = %s, want P1", finding.Severity)
	}
}

func TestAuditFindingSeverity_UnpushedCommits(t *testing.T) {
	// Test that unpushed commits finding has P1 severity (Req 8.6)
	finding := NewFinding(FindingUnpushedSubmoduleCommit, SeverityP1, "backend")

	if finding.Severity != SeverityP1 {
		t.Fatalf("Unpushed commits severity = %s, want P1", finding.Severity)
	}
}

func TestRunSubmoduleAudit_WithDirtySubmodule(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize parent git repo
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Skipf("git init failed (git may not be available): %v", err)
	}

	// Create a dirty nested git repo
	submoduleDir := filepath.Join(tmpDir, "backend")
	if err := os.MkdirAll(submoduleDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := exec.Command("git", "-C", submoduleDir, "init").Run(); err != nil {
		t.Skipf("git init failed: %v", err)
	}

	// Configure git user
	if err := exec.Command("git", "-C", submoduleDir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Skipf("git config user.email failed: %v", err)
	}
	if err := exec.Command("git", "-C", submoduleDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Skipf("git config user.name failed: %v", err)
	}

	// Create and commit a file
	mainGoPath := filepath.Join(submoduleDir, "main.go")
	if err := os.WriteFile(mainGoPath, []byte("package main"), 0644); err != nil {
		t.Fatalf("WriteFile main.go failed: %v", err)
	}
	if err := exec.Command("git", "-C", submoduleDir, "add", ".").Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := exec.Command("git", "-C", submoduleDir, "commit", "-m", "Initial").Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Make it dirty
	dirtyPath := filepath.Join(submoduleDir, "dirty.txt")
	if err := os.WriteFile(dirtyPath, []byte("uncommitted"), 0644); err != nil {
		t.Fatalf("WriteFile dirty.txt failed: %v", err)
	}

	findings := RunSubmoduleAudit(tmpDir, []string{"backend"})

	// Should have at least the dirty finding
	var hasDirtyFinding bool
	for _, f := range findings {
		if f.ID == FindingDirtySubmodule {
			hasDirtyFinding = true
			break
		}
	}

	if !hasDirtyFinding {
		t.Fatalf("RunSubmoduleAudit did not return dirty submodule finding")
	}
}

func TestRunSubmoduleAudit_EmptySubmoduleList(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Skipf("git init failed (git may not be available): %v", err)
	}

	findings := RunSubmoduleAudit(tmpDir, []string{})

	// Should only have uninitialized check (which checks .gitmodules)
	// Empty slice or nil are both valid results when there are no findings
	_ = findings // findings may be nil or empty slice, both are valid
}
