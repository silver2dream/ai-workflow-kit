package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// generateValidateSubmodulesWorkflow — pure string builder
// ---------------------------------------------------------------------------

func TestGenerateValidateSubmodulesWorkflow_ContainsYAML(t *testing.T) {
	git := GitConfig{
		IntegrationBranch: "develop",
		ReleaseBranch:     "main",
	}
	content := generateValidateSubmodulesWorkflow(git)
	if !strings.Contains(content, "validate-submodules") {
		t.Errorf("expected validate-submodules in content, got:\n%s", content)
	}
	if !strings.Contains(content, "develop") {
		t.Errorf("expected integration branch 'develop', got:\n%s", content)
	}
	if !strings.Contains(content, "main") {
		t.Errorf("expected release branch 'main', got:\n%s", content)
	}
}

func TestGenerateValidateSubmodulesWorkflow_SameBranch(t *testing.T) {
	git := GitConfig{
		IntegrationBranch: "main",
		ReleaseBranch:     "main",
	}
	content := generateValidateSubmodulesWorkflow(git)
	if !strings.Contains(content, "main") {
		t.Error("expected 'main' in content")
	}
}

func TestGenerateValidateSubmodulesWorkflow_EmptyReleaseBranch(t *testing.T) {
	git := GitConfig{
		IntegrationBranch: "develop",
		ReleaseBranch:     "",
	}
	content := generateValidateSubmodulesWorkflow(git)
	if !strings.Contains(content, "develop") {
		t.Error("expected 'develop' in content")
	}
}

// ---------------------------------------------------------------------------
// copyDir — filesystem operation
// ---------------------------------------------------------------------------

func TestCopyDir_BasicCopy(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create source structure
	os.WriteFile(filepath.Join(src, "file1.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(src, "subdir"), 0755)
	os.WriteFile(filepath.Join(src, "subdir", "file2.txt"), []byte("world"), 0644)

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir: %v", err)
	}

	// Verify
	data1, err := os.ReadFile(filepath.Join(dst, "file1.txt"))
	if err != nil || string(data1) != "hello" {
		t.Errorf("file1.txt content = %q, want hello", string(data1))
	}
	data2, err := os.ReadFile(filepath.Join(dst, "subdir", "file2.txt"))
	if err != nil || string(data2) != "world" {
		t.Errorf("subdir/file2.txt content = %q, want world", string(data2))
	}
}

func TestCopyDir_EmptyDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir empty: %v", err)
	}
}

func TestCopyDir_NonExistentSrc(t *testing.T) {
	dst := t.TempDir()
	err := copyDir("/nonexistent/path", dst)
	if err == nil {
		t.Error("expected error for non-existent source")
	}
}
