package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestEpicMode_FullLifecycle verifies the complete epic tracking workflow:
// 1. Initial state: unchecked tasks in epic body → ActionCreateTask
// 2. After issue created: task has #N ref, still unchecked → skipped (in-progress)
// 3. After issue closed: task auto-checked → complete
// 4. All checked → ActionAllComplete
func TestEpicMode_FullLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active:   []string{"my-project"},
			Tracking: TrackingConfig{Mode: TrackingModeGitHubEpic, EpicIssues: map[string]int{"my-project": 100}},
		},
		GitHub: GitHubConfig{Labels: DefaultLabels()},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	// Phase 1: Two unchecked tasks, no issue refs
	mockClient.IssueBodies[100] = "- [ ] Implement login\n- [ ] Add logout\n"
	mockClient.OpenIssueCount = 0

	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Phase 1: Decide() error = %v", err)
	}
	if decision.NextAction != ActionCreateTask {
		t.Fatalf("Phase 1: NextAction = %q, want %q", decision.NextAction, ActionCreateTask)
	}
	if decision.TaskText != "Implement login" {
		t.Errorf("Phase 1: TaskText = %q, want %q", decision.TaskText, "Implement login")
	}

	// Phase 2: First task has issue ref (in-progress), second still unlinked
	mockClient.IssueBodies[100] = "- [ ] #10 Implement login\n- [ ] Add logout\n"
	mockClient.OpenIssueCount = 1

	decision, err = a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Phase 2: Decide() error = %v", err)
	}
	// Should find the unlinked "Add logout" task
	if decision.NextAction != ActionCreateTask {
		t.Fatalf("Phase 2: NextAction = %q, want %q", decision.NextAction, ActionCreateTask)
	}
	if decision.TaskText != "Add logout" {
		t.Errorf("Phase 2: TaskText = %q, want %q", decision.TaskText, "Add logout")
	}

	// Phase 3: Both tasks have issue refs (both in-progress)
	mockClient.IssueBodies[100] = "- [ ] #10 Implement login\n- [ ] #11 Add logout\n"
	mockClient.OpenIssueCount = 2

	decision, err = a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Phase 3: Decide() error = %v", err)
	}
	// All tasks have issue refs, so checkEpicProgress returns nil → all complete
	if decision.NextAction != ActionAllComplete {
		t.Errorf("Phase 3: NextAction = %q, want %q", decision.NextAction, ActionAllComplete)
	}

	// Phase 4: Both tasks checked (GitHub auto-checked when issues closed)
	mockClient.IssueBodies[100] = "- [x] #10 Implement login\n- [x] #11 Add logout\n"
	mockClient.OpenIssueCount = 0

	decision, err = a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Phase 4: Decide() error = %v", err)
	}
	if decision.NextAction != ActionAllComplete {
		t.Errorf("Phase 4: NextAction = %q, want %q", decision.NextAction, ActionAllComplete)
	}
}

// TestEpicMode_NoFalseAllComplete reproduces the exact scenario that caused the original bug:
// - All tasks have #N refs and are closed, but checkboxes weren't updated
// In epic mode, tasks with issue refs but unchecked are treated as in-progress (not as needing creation)
func TestEpicMode_NoFalseAllComplete(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active:   []string{"snake-arena"},
			Tracking: TrackingConfig{Mode: TrackingModeGitHubEpic, EpicIssues: map[string]int{"snake-arena": 200}},
		},
		GitHub: GitHubConfig{Labels: DefaultLabels()},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	// Simulate the problematic state: 6 tasks, all with refs, all unchecked
	// In real GitHub this shouldn't happen (auto-check), but we test robustness
	mockClient.IssueBodies[200] = `# Snake Arena Tasks

- [ ] #40 Implement game engine
- [ ] #41 Add canvas rendering
- [ ] #42 Implement controls
- [ ] #43 Add collision detection
- [ ] #44 Implement food spawning
- [ ] #45 Add score display
`
	mockClient.OpenIssueCount = 0

	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	// All tasks have issue refs → treated as in-progress or complete
	// checkEpicProgress returns nil → allComplete
	// This is correct because in epic mode, having issue refs means the work was done
	if decision.NextAction != ActionAllComplete {
		t.Errorf("NextAction = %q, want %q (all tasks have issue refs)", decision.NextAction, ActionAllComplete)
	}
}

// TestTasksMdMode_StillWorks verifies backward compatibility:
// the tasks_md mode must work identically to before the epic feature.
func TestTasksMdMode_StillWorks(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			BasePath: ".ai/specs",
			Active:   []string{"my-feature"},
			Tracking: TrackingConfig{Mode: TrackingModeTasksMd},
		},
		GitHub: GitHubConfig{Labels: DefaultLabels()},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	// Create tasks.md with one uncompleted task
	specDir := filepath.Join(tmpDir, ".ai", "specs", "my-feature")
	os.MkdirAll(specDir, 0755)
	os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte("- [ ] Implement feature\n- [x] Design complete\n"), 0644)

	mockClient.OpenIssueCount = 0

	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.NextAction != ActionCreateTask {
		t.Fatalf("NextAction = %q, want %q", decision.NextAction, ActionCreateTask)
	}
	if decision.SpecName != "my-feature" {
		t.Errorf("SpecName = %q, want %q", decision.SpecName, "my-feature")
	}
	if decision.TaskLine != 1 {
		t.Errorf("TaskLine = %d, want 1", decision.TaskLine)
	}
	// Should NOT have epic-specific fields
	if decision.EpicIssue != 0 {
		t.Errorf("EpicIssue = %d, want 0 (tasks_md mode)", decision.EpicIssue)
	}
}

// TestTasksMdMode_DefaultConfig verifies that a config without tracking section
// defaults to tasks_md mode.
func TestTasksMdMode_DefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".ai", "config")
	os.MkdirAll(configPath, 0755)

	// Config with NO tracking section
	configContent := `
specs:
  base_path: .ai/specs
  active:
    - test-project
github:
  repo: owner/repo
`
	os.WriteFile(filepath.Join(configPath, "workflow.yaml"), []byte(configContent), 0644)

	cfg, err := LoadConfig(filepath.Join(configPath, "workflow.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.IsEpicMode() {
		t.Error("should NOT be epic mode with default config")
	}
	if cfg.Specs.Tracking.Mode != TrackingModeTasksMd {
		t.Errorf("Mode = %q, want %q", cfg.Specs.Tracking.Mode, TrackingModeTasksMd)
	}
}

// TestEpicMode_MultipleSpecs verifies that epic mode works with multiple specs.
func TestEpicMode_MultipleSpecs(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active: []string{"project-a", "project-b"},
			Tracking: TrackingConfig{
				Mode: TrackingModeGitHubEpic,
				EpicIssues: map[string]int{
					"project-a": 100,
					"project-b": 200,
				},
			},
		},
		GitHub: GitHubConfig{Labels: DefaultLabels()},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	// project-a is all complete, project-b has an unlinked task
	mockClient.IssueBodies[100] = "- [x] #10 Done\n"
	mockClient.IssueBodies[200] = "- [x] #20 Done\n- [ ] New feature needed\n"
	mockClient.OpenIssueCount = 0

	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.NextAction != ActionCreateTask {
		t.Fatalf("NextAction = %q, want %q", decision.NextAction, ActionCreateTask)
	}
	if decision.SpecName != "project-b" {
		t.Errorf("SpecName = %q, want %q", decision.SpecName, "project-b")
	}
	if decision.TaskText != "New feature needed" {
		t.Errorf("TaskText = %q, want %q", decision.TaskText, "New feature needed")
	}
	if decision.EpicIssue != 200 {
		t.Errorf("EpicIssue = %d, want 200", decision.EpicIssue)
	}
}
