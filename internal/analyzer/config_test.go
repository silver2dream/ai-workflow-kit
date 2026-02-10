package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	// Create a valid config file
	configContent := `
specs:
  base_path: .ai/specs
  active:
    - feature1
    - feature2
github:
  repo: owner/repo
  labels:
    task: my-task
    in_progress: my-in-progress
    pr_ready: my-pr-ready
repos:
  - name: backend
    path: backend
    type: directory
    language: go
    verify:
      build: go build ./...
      test: go test ./...
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if config.Specs.BasePath != ".ai/specs" {
		t.Errorf("BasePath = %q, want %q", config.Specs.BasePath, ".ai/specs")
	}
	if len(config.Specs.Active) != 2 {
		t.Errorf("Active specs = %d, want 2", len(config.Specs.Active))
	}
	if config.GitHub.Labels.Task != "my-task" {
		t.Errorf("Task label = %q, want %q", config.GitHub.Labels.Task, "my-task")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	// Create minimal config
	configContent := `
github:
  repo: owner/repo
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Check defaults are applied
	if config.Specs.BasePath != ".ai/specs" {
		t.Errorf("BasePath = %q, want %q", config.Specs.BasePath, ".ai/specs")
	}
	if config.GitHub.Labels.Task != "ai-task" {
		t.Errorf("Task label = %q, want %q", config.GitHub.Labels.Task, "ai-task")
	}
	if config.GitHub.Labels.InProgress != "in-progress" {
		t.Errorf("InProgress label = %q, want %q", config.GitHub.Labels.InProgress, "in-progress")
	}
	if config.GitHub.Labels.PRReady != "pr-ready" {
		t.Errorf("PRReady label = %q, want %q", config.GitHub.Labels.PRReady, "pr-ready")
	}
	if config.GitHub.Labels.WorkerFailed != "worker-failed" {
		t.Errorf("WorkerFailed label = %q, want %q", config.GitHub.Labels.WorkerFailed, "worker-failed")
	}
	if config.GitHub.Labels.NeedsHumanReview != "needs-human-review" {
		t.Errorf("NeedsHumanReview label = %q, want %q", config.GitHub.Labels.NeedsHumanReview, "needs-human-review")
	}
	if config.GitHub.Labels.ReviewFailed != "review-failed" {
		t.Errorf("ReviewFailed label = %q, want %q", config.GitHub.Labels.ReviewFailed, "review-failed")
	}
	if config.GitHub.Labels.MergeConflict != "merge-conflict" {
		t.Errorf("MergeConflict label = %q, want %q", config.GitHub.Labels.MergeConflict, "merge-conflict")
	}
	if config.GitHub.Labels.NeedsRebase != "needs-rebase" {
		t.Errorf("NeedsRebase label = %q, want %q", config.GitHub.Labels.NeedsRebase, "needs-rebase")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("LoadConfig() should return error for nonexistent file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	// Create invalid YAML
	os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0644)

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("LoadConfig() should return error for invalid YAML")
	}
}

func TestDefaultLabels(t *testing.T) {
	labels := DefaultLabels()

	if labels.Task != "ai-task" {
		t.Errorf("Task = %q, want %q", labels.Task, "ai-task")
	}
	if labels.InProgress != "in-progress" {
		t.Errorf("InProgress = %q, want %q", labels.InProgress, "in-progress")
	}
	if labels.PRReady != "pr-ready" {
		t.Errorf("PRReady = %q, want %q", labels.PRReady, "pr-ready")
	}
	if labels.WorkerFailed != "worker-failed" {
		t.Errorf("WorkerFailed = %q, want %q", labels.WorkerFailed, "worker-failed")
	}
	if labels.NeedsHumanReview != "needs-human-review" {
		t.Errorf("NeedsHumanReview = %q, want %q", labels.NeedsHumanReview, "needs-human-review")
	}
	if labels.ReviewFailed != "review-failed" {
		t.Errorf("ReviewFailed = %q, want %q", labels.ReviewFailed, "review-failed")
	}
	if labels.MergeConflict != "merge-conflict" {
		t.Errorf("MergeConflict = %q, want %q", labels.MergeConflict, "merge-conflict")
	}
	if labels.NeedsRebase != "needs-rebase" {
		t.Errorf("NeedsRebase = %q, want %q", labels.NeedsRebase, "needs-rebase")
	}
}

func TestGetRepoByName(t *testing.T) {
	config := &Config{
		Repos: []RepoConfig{
			{Name: "backend", Path: "backend", Language: "go"},
			{Name: "frontend", Path: "frontend", Language: "unity"},
		},
	}

	// Found
	repo := config.GetRepoByName("backend")
	if repo == nil {
		t.Fatal("GetRepoByName() returned nil for existing repo")
	}
	if repo.Language != "go" {
		t.Errorf("Language = %q, want %q", repo.Language, "go")
	}

	// Not found
	repo = config.GetRepoByName("nonexistent")
	if repo != nil {
		t.Error("GetRepoByName() should return nil for nonexistent repo")
	}
}

func TestGetVerifyCommands(t *testing.T) {
	config := &Config{
		Repos: []RepoConfig{
			{
				Name:     "backend",
				Path:     "backend",
				Language: "go",
				Verify: VerifyConfig{
					Build: "go build ./...",
					Test:  "go test ./...",
				},
			},
			{
				Name:     "frontend",
				Path:     "frontend",
				Language: "unity",
				Verify:   VerifyConfig{}, // No verify commands
			},
		},
	}

	// With both commands
	commands := config.GetVerifyCommands("backend")
	if len(commands) != 2 {
		t.Errorf("GetVerifyCommands() returned %d commands, want 2", len(commands))
	}
	if commands[0] != "go build ./..." {
		t.Errorf("First command = %q, want %q", commands[0], "go build ./...")
	}

	// With no commands
	commands = config.GetVerifyCommands("frontend")
	if len(commands) != 0 {
		t.Errorf("GetVerifyCommands() returned %d commands, want 0", len(commands))
	}

	// Nonexistent repo
	commands = config.GetVerifyCommands("nonexistent")
	if commands != nil {
		t.Error("GetVerifyCommands() should return nil for nonexistent repo")
	}
}

func TestLoadConfig_TrackingDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	// Config without tracking section
	configContent := `
github:
  repo: owner/repo
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if config.Specs.Tracking.Mode != TrackingModeTasksMd {
		t.Errorf("Tracking.Mode = %q, want %q", config.Specs.Tracking.Mode, TrackingModeTasksMd)
	}
	if config.IsEpicMode() {
		t.Error("IsEpicMode() should be false for default config")
	}
}

func TestLoadConfig_TrackingEpicMode(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "workflow.yaml")

	configContent := `
specs:
  base_path: .ai/specs
  active:
    - my-project
    - other-project
  tracking:
    mode: github_epic
    epic_issues:
      my-project: 42
      other-project: 99
github:
  repo: owner/repo
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if config.Specs.Tracking.Mode != TrackingModeGitHubEpic {
		t.Errorf("Tracking.Mode = %q, want %q", config.Specs.Tracking.Mode, TrackingModeGitHubEpic)
	}
	if !config.IsEpicMode() {
		t.Error("IsEpicMode() should be true for github_epic mode")
	}
	if config.GetEpicIssue("my-project") != 42 {
		t.Errorf("GetEpicIssue(my-project) = %d, want 42", config.GetEpicIssue("my-project"))
	}
	if config.GetEpicIssue("other-project") != 99 {
		t.Errorf("GetEpicIssue(other-project) = %d, want 99", config.GetEpicIssue("other-project"))
	}
	if config.GetEpicIssue("nonexistent") != 0 {
		t.Errorf("GetEpicIssue(nonexistent) = %d, want 0", config.GetEpicIssue("nonexistent"))
	}
}

func TestIsEpicMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want bool
	}{
		{"empty defaults to tasks_md after LoadConfig", TrackingModeTasksMd, false},
		{"tasks_md mode", TrackingModeTasksMd, false},
		{"github_epic mode", TrackingModeGitHubEpic, true},
		{"unknown mode", "something_else", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Specs: SpecsConfig{
					Tracking: TrackingConfig{Mode: tt.mode},
				},
			}
			if got := config.IsEpicMode(); got != tt.want {
				t.Errorf("IsEpicMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEpicIssue(t *testing.T) {
	config := &Config{
		Specs: SpecsConfig{
			Tracking: TrackingConfig{
				Mode: TrackingModeGitHubEpic,
				EpicIssues: map[string]int{
					"project-a": 10,
					"project-b": 20,
				},
			},
		},
	}

	if got := config.GetEpicIssue("project-a"); got != 10 {
		t.Errorf("GetEpicIssue(project-a) = %d, want 10", got)
	}
	if got := config.GetEpicIssue("project-b"); got != 20 {
		t.Errorf("GetEpicIssue(project-b) = %d, want 20", got)
	}
	if got := config.GetEpicIssue("missing"); got != 0 {
		t.Errorf("GetEpicIssue(missing) = %d, want 0", got)
	}

	// Nil map
	configNilMap := &Config{}
	if got := configNilMap.GetEpicIssue("anything"); got != 0 {
		t.Errorf("GetEpicIssue on nil map = %d, want 0", got)
	}
}
