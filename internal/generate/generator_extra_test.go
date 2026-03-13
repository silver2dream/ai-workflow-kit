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
// generateUnrealCIContent — pure string builder
// ---------------------------------------------------------------------------

func TestGenerateUnrealCIContent_BasicStructure(t *testing.T) {
	repo := RepoConfig{Name: "frontend", Language: "unreal"}
	git := GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"}
	content := generateUnrealCIContent(repo, git)
	if !strings.Contains(content, "frontend CI") {
		t.Errorf("expected 'frontend CI' in Unreal content, got:\n%s", content)
	}
	if !strings.Contains(content, ".uproject") {
		t.Errorf("expected .uproject check, got:\n%s", content)
	}
	if !strings.Contains(content, "develop") {
		t.Errorf("expected branch develop, got:\n%s", content)
	}
}

func TestGenerateUnrealCIContent_SameBranch(t *testing.T) {
	repo := RepoConfig{Name: "ue-game", Language: "unreal"}
	git := GitConfig{IntegrationBranch: "main", ReleaseBranch: "main"}
	content := generateUnrealCIContent(repo, git)
	if !strings.Contains(content, "main") {
		t.Error("expected 'main' in content")
	}
	// Should not repeat main twice in the branch list
	if strings.Count(content, "pull_request:") != 1 {
		t.Error("should have exactly one pull_request trigger")
	}
}

// ---------------------------------------------------------------------------
// generateGodotCIContent — pure string builder
// ---------------------------------------------------------------------------

func TestGenerateGodotCIContent_BasicStructure(t *testing.T) {
	repo := RepoConfig{Name: "game", Language: "godot"}
	git := GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"}
	content := generateGodotCIContent(repo, git)
	if !strings.Contains(content, "game CI") {
		t.Errorf("expected 'game CI' in Godot content, got:\n%s", content)
	}
	if !strings.Contains(content, "GODOT") {
		t.Errorf("expected GODOT in content, got:\n%s", content)
	}
}

func TestGenerateGodotCIContent_HasVersion(t *testing.T) {
	repo := RepoConfig{Name: "mygame", Language: "godot"}
	git := GitConfig{IntegrationBranch: "main", ReleaseBranch: ""}
	content := generateGodotCIContent(repo, git)
	if !strings.Contains(content, "GODOT_VERSION") {
		t.Errorf("expected GODOT_VERSION in content, got:\n%s", content)
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

// ---------------------------------------------------------------------------
// generateCIContent with different template types
// ---------------------------------------------------------------------------

func TestGenerateCIContent_Rust(t *testing.T) {
	repo := RepoConfig{Name: "backend", Language: "rust"}
	git := GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"}
	content := generateCIContent(repo, git, "rust")
	if !strings.Contains(content, "backend CI") {
		t.Errorf("expected 'backend CI', got:\n%s", content)
	}
	if !strings.Contains(content, "cargo") {
		t.Errorf("expected cargo in rust CI, got:\n%s", content)
	}
}

func TestGenerateCIContent_DotNet(t *testing.T) {
	repo := RepoConfig{Name: "backend", Language: "dotnet"}
	git := GitConfig{IntegrationBranch: "develop"}
	content := generateCIContent(repo, git, "dotnet")
	if !strings.Contains(content, "dotnet") {
		t.Errorf("expected 'dotnet' in content, got:\n%s", content)
	}
}

func TestGenerateCIContent_Python(t *testing.T) {
	repo := RepoConfig{Name: "api", Language: "python"}
	git := GitConfig{IntegrationBranch: "develop"}
	content := generateCIContent(repo, git, "python")
	if !strings.Contains(content, "python") {
		t.Errorf("expected 'python' in content, got:\n%s", content)
	}
}

func TestGenerateCIContent_Generic(t *testing.T) {
	repo := RepoConfig{Name: "myservice", Language: "generic"}
	git := GitConfig{IntegrationBranch: "develop"}
	content := generateCIContent(repo, git, "generic")
	if content == "" {
		t.Error("generateCIContent with generic template should return non-empty content")
	}
}

func TestGenerateCIContent_Unreal(t *testing.T) {
	repo := RepoConfig{Name: "game", Language: "unreal"}
	git := GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"}
	content := generateCIContent(repo, git, "unreal")
	if !strings.Contains(content, ".uproject") {
		t.Errorf("expected .uproject in unreal content, got:\n%s", content)
	}
}

func TestGenerateCIContent_Godot(t *testing.T) {
	repo := RepoConfig{Name: "game", Language: "godot"}
	git := GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"}
	content := generateCIContent(repo, git, "godot")
	if !strings.Contains(content, "GODOT") {
		t.Errorf("expected GODOT in godot content, got:\n%s", content)
	}
}

// ---------------------------------------------------------------------------
// generateCIWorkflows (file-creating function)
// ---------------------------------------------------------------------------

func TestGenerateCIWorkflows_GoRepo(t *testing.T) {
	dir := t.TempDir()
	ctx := &TemplateContext{
		Config: Config{
			Repos: []RepoConfig{
				{Name: "backend", Language: "go", Type: "root"},
			},
			Git: GitConfig{
				IntegrationBranch: "develop",
				ReleaseBranch:     "main",
			},
			Project: ProjectConfig{Type: "single"},
		},
	}
	generated, err := generateCIWorkflows(dir, ctx)
	if err != nil {
		t.Fatalf("generateCIWorkflows: %v", err)
	}
	if len(generated) == 0 {
		t.Error("expected at least one generated CI file")
	}
	// Verify file was created
	for _, path := range generated {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("generated file missing: %q", path)
		}
	}
}

func TestGenerateCIWorkflows_DirectoryRepo(t *testing.T) {
	dir := t.TempDir()
	ctx := &TemplateContext{
		Config: Config{
			Repos: []RepoConfig{
				{Name: "backend", Language: "go", Type: "directory"},
			},
			Git:     GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"},
			Project: ProjectConfig{Type: "monorepo"},
		},
	}
	generated, err := generateCIWorkflows(dir, ctx)
	if err != nil {
		t.Fatalf("generateCIWorkflows directory: %v", err)
	}
	if len(generated) == 0 {
		t.Error("expected at least one generated CI file")
	}
	// Directory repos get ci-{name}.yml
	found := false
	for _, p := range generated {
		if strings.Contains(p, "ci-backend.yml") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ci-backend.yml, got %v", generated)
	}
}

func TestGenerateCIWorkflows_WithSubmodules(t *testing.T) {
	dir := t.TempDir()
	ctx := &TemplateContext{
		Config: Config{
			Repos: []RepoConfig{
				{Name: "backend", Language: "go", Type: "directory"},
			},
			Git:     GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"},
			Project: ProjectConfig{Type: "monorepo"},
		},
		HasSubmodules: true,
	}
	generated, err := generateCIWorkflows(dir, ctx)
	if err != nil {
		t.Fatalf("generateCIWorkflows with submodules: %v", err)
	}
	// Should also have validate-submodules.yml
	hasValidate := false
	for _, p := range generated {
		if strings.Contains(p, "validate-submodules") {
			hasValidate = true
		}
	}
	if !hasValidate {
		t.Errorf("expected validate-submodules.yml for monorepo with submodules, got %v", generated)
	}
}

func TestGenerateCIWorkflows_EmptyRepos(t *testing.T) {
	dir := t.TempDir()
	ctx := &TemplateContext{
		Config: Config{
			Repos: []RepoConfig{},
			Git:   GitConfig{IntegrationBranch: "develop"},
		},
	}
	generated, err := generateCIWorkflows(dir, ctx)
	if err != nil {
		t.Fatalf("generateCIWorkflows empty: %v", err)
	}
	if len(generated) != 0 {
		t.Errorf("expected 0 generated files for empty repos, got %v", generated)
	}
}
