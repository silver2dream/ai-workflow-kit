package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// parseIssueOutput
// ---------------------------------------------------------------------------

func TestParseIssueOutput_ValidURL(t *testing.T) {
	output := "https://github.com/owner/repo/issues/42\n"
	num, url, err := parseIssueOutput(output)
	if err != nil {
		t.Fatalf("parseIssueOutput: %v", err)
	}
	if num != 42 {
		t.Errorf("num = %d, want 42", num)
	}
	if !strings.Contains(url, "/issues/42") {
		t.Errorf("url = %q, want to contain /issues/42", url)
	}
}

func TestParseIssueOutput_NumberOnly(t *testing.T) {
	output := "Created issue: /issues/100 (some text)"
	num, _, err := parseIssueOutput(output)
	if err != nil {
		t.Fatalf("parseIssueOutput: %v", err)
	}
	if num != 100 {
		t.Errorf("num = %d, want 100", num)
	}
}

func TestParseIssueOutput_NoMatch(t *testing.T) {
	_, _, err := parseIssueOutput("error: something went wrong")
	if err == nil {
		t.Error("expected error when no issue number found")
	}
}

func TestParseIssueOutput_Empty(t *testing.T) {
	_, _, err := parseIssueOutput("")
	if err == nil {
		t.Error("expected error for empty output")
	}
}

// ---------------------------------------------------------------------------
// buildGHCreateArgs
// ---------------------------------------------------------------------------

func TestBuildGHCreateArgs_WithRepo(t *testing.T) {
	args := buildGHCreateArgs("My Title", "/tmp/body.md", "ai-task", "owner/repo")
	// Should include --repo
	hasRepo := false
	for i, a := range args {
		if a == "--repo" && i+1 < len(args) && args[i+1] == "owner/repo" {
			hasRepo = true
		}
	}
	if !hasRepo {
		t.Errorf("buildGHCreateArgs with repo should include --repo owner/repo, got %v", args)
	}
}

func TestBuildGHCreateArgs_WithoutRepo(t *testing.T) {
	args := buildGHCreateArgs("My Title", "/tmp/body.md", "ai-task", "")
	for _, a := range args {
		if a == "--repo" {
			t.Errorf("buildGHCreateArgs without repo should not include --repo, got %v", args)
		}
	}
}

func TestBuildGHCreateArgs_HasRequiredFields(t *testing.T) {
	args := buildGHCreateArgs("Test Title", "/tmp/body.md", "my-label", "")

	checks := map[string]bool{
		"issue": false, "create": false,
		"--title": false, "Test Title": false,
		"--body-file": false, "/tmp/body.md": false,
		"--label": false, "my-label": false,
	}
	for _, a := range args {
		checks[a] = true
	}
	for field, found := range checks {
		if !found {
			t.Errorf("args missing %q: %v", field, args)
		}
	}
}

// ---------------------------------------------------------------------------
// resolveGHParams
// ---------------------------------------------------------------------------

func TestResolveGHParams_DefaultLabel(t *testing.T) {
	opts := CreateTaskOptions{Repo: ""}
	cfg := &analyzer.Config{}
	label, repo := resolveGHParams(opts, cfg)
	if label != "ai-task" {
		t.Errorf("label = %q, want ai-task (default)", label)
	}
	if repo != "" {
		t.Errorf("repo = %q, want empty", repo)
	}
}

func TestResolveGHParams_ConfigLabel(t *testing.T) {
	opts := CreateTaskOptions{}
	cfg := &analyzer.Config{}
	cfg.GitHub.Labels.Task = "custom-task-label"
	label, _ := resolveGHParams(opts, cfg)
	if label != "custom-task-label" {
		t.Errorf("label = %q, want custom-task-label", label)
	}
}

func TestResolveGHParams_OptsRepo(t *testing.T) {
	opts := CreateTaskOptions{Repo: "opts-owner/opts-repo"}
	cfg := &analyzer.Config{}
	cfg.GitHub.Repo = "config-owner/config-repo"
	_, repo := resolveGHParams(opts, cfg)
	if repo != "opts-owner/opts-repo" {
		t.Errorf("repo = %q, should prefer opts.Repo over cfg.GitHub.Repo", repo)
	}
}

func TestResolveGHParams_ConfigRepo(t *testing.T) {
	opts := CreateTaskOptions{Repo: ""}
	cfg := &analyzer.Config{}
	cfg.GitHub.Repo = "owner/repo"
	_, repo := resolveGHParams(opts, cfg)
	if repo != "owner/repo" {
		t.Errorf("repo = %q, want owner/repo from config", repo)
	}
}

// ---------------------------------------------------------------------------
// updateConfigForEpic
// ---------------------------------------------------------------------------

func TestUpdateConfigForEpic_SetsEpicIssue(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")

	// Write a minimal workflow.yaml
	initial := `specs:
  base_path: .ai/specs
github:
  repo: owner/repo
repos:
  - name: backend
`
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := updateConfigForEpic(configPath, "myspec", 42); err != nil {
		t.Fatalf("updateConfigForEpic: %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "github_epic") {
		t.Errorf("config should contain github_epic mode, got:\n%s", content)
	}
	if !strings.Contains(content, "myspec") {
		t.Errorf("config should contain spec name myspec, got:\n%s", content)
	}
	if !strings.Contains(content, "42") {
		t.Errorf("config should contain epic number 42, got:\n%s", content)
	}
}

func TestUpdateConfigForEpic_MissingFile(t *testing.T) {
	err := updateConfigForEpic("/nonexistent/path/workflow.yaml", "spec", 1)
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

func TestUpdateConfigForEpic_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	os.WriteFile(configPath, []byte("not: valid: yaml: ["), 0644)

	err := updateConfigForEpic(configPath, "spec", 1)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ---------------------------------------------------------------------------
// findOrCreateMapKey
// ---------------------------------------------------------------------------

func TestFindOrCreateMapKey_ExistingKey(t *testing.T) {
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "mykey"}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "myval"}
	mapping.Content = []*yaml.Node{keyNode, valNode}

	result := findOrCreateMapKey(mapping, "mykey")
	if result != valNode {
		t.Error("findOrCreateMapKey should return existing value node")
	}
}

func TestFindOrCreateMapKey_NewKey(t *testing.T) {
	mapping := &yaml.Node{Kind: yaml.MappingNode}

	result := findOrCreateMapKey(mapping, "newkey")
	if result == nil {
		t.Fatal("findOrCreateMapKey should return new node")
	}
	if len(mapping.Content) != 2 {
		t.Errorf("mapping should have 2 content nodes, got %d", len(mapping.Content))
	}
	if mapping.Content[0].Value != "newkey" {
		t.Errorf("key node = %q, want newkey", mapping.Content[0].Value)
	}
}

func TestFindOrCreateMapKey_EmptyMapping(t *testing.T) {
	mapping := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{}}
	result := findOrCreateMapKey(mapping, "key")
	if result == nil {
		t.Error("should create a new node for empty mapping")
	}
}

// ---------------------------------------------------------------------------
// CreateEpic dry run (no gh CLI needed)
// ---------------------------------------------------------------------------

func TestCreateEpic_NoBodyFile(t *testing.T) {
	_, err := createEpicOptions_test(t, "", false)
	if err == nil {
		t.Error("expected error when body-file is missing")
	}
}

func TestCreateEpic_MissingConfig(t *testing.T) {
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "body.md")
	os.WriteFile(bodyFile, []byte("## Epic"), 0644)

	_, err := createEpicOptions_test_withDir(t, bodyFile, dir, true)
	if err == nil {
		t.Error("expected error when config is missing")
	}
}

func TestCreateEpic_DryRunReturnsBody(t *testing.T) {
	dir := t.TempDir()

	// Write workflow.yaml
	cfgDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(cfgDir, 0755)
	cfg := `specs:
  base_path: .ai/specs
  tracking:
    mode: tasks_md
github:
  repo: owner/repo
`
	os.WriteFile(filepath.Join(cfgDir, "workflow.yaml"), []byte(cfg), 0644)

	// Write body file
	bodyFile := filepath.Join(dir, "epic-body.md")
	os.WriteFile(bodyFile, []byte("## Tasks\n- [ ] Task 1"), 0644)

	result, err := createEpicOptions_test_withDir(t, bodyFile, dir, true)
	if err != nil {
		t.Fatalf("CreateEpic dry run: %v", err)
	}
	if result.DryRunBody == "" {
		t.Error("DryRunBody should be populated in dry run mode")
	}
	if !strings.Contains(result.DryRunBody, "Task 1") {
		t.Errorf("DryRunBody = %q, should contain Task 1", result.DryRunBody)
	}
}

func TestCreateEpic_EpicAlreadyExists(t *testing.T) {
	dir := t.TempDir()

	// Write workflow.yaml with existing epic
	cfgDir := filepath.Join(dir, ".ai", "config")
	os.MkdirAll(cfgDir, 0755)
	cfg := `specs:
  base_path: .ai/specs
  tracking:
    mode: github_epic
    epic_issues:
      myspec: 10
github:
  repo: owner/repo
`
	os.WriteFile(filepath.Join(cfgDir, "workflow.yaml"), []byte(cfg), 0644)

	bodyFile := filepath.Join(dir, "epic-body.md")
	os.WriteFile(bodyFile, []byte("## Tasks"), 0644)

	_, err := createEpicOptions_test_withDir(t, bodyFile, dir, true)
	if err == nil {
		t.Error("expected error when epic already exists for spec")
	}
}

// Helper functions to avoid repetition

func createEpicOptions_test(t *testing.T, bodyFile string, dryRun bool) (*CreateEpicResult, error) {
	t.Helper()
	dir := t.TempDir()
	return createEpicOptions_test_withDir(t, bodyFile, dir, dryRun)
}

func createEpicOptions_test_withDir(t *testing.T, bodyFile, dir string, dryRun bool) (*CreateEpicResult, error) {
	t.Helper()
	import_ctx := t.Context()
	_ = import_ctx

	opts := CreateEpicOptions{
		SpecName:  "myspec",
		StateRoot: dir,
		DryRun:    dryRun,
		BodyFile:  bodyFile,
	}
	return CreateEpic(t.Context(), opts)
}
