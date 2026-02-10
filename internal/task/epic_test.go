package task

import (
	"os"
	"strings"
	"testing"
)

func TestBuildEpicBody(t *testing.T) {
	tasksContent := `- [ ] 1. Implement login system
  - [ ] 1.1 Add password hashing
  - [ ] 1.2 Add session management
- [x] 2. Design database schema
- [ ] 3. Add API endpoints _depends_on: 1, 2_
`
	tasks := ParseTasks(tasksContent)
	if len(tasks) != 3 {
		t.Fatalf("ParseTasks returned %d tasks, want 3", len(tasks))
	}

	body := buildEpicBody("my-project", tasks, tasksContent)

	// Check header
	if !strings.Contains(body, "# my-project Task Tracking") {
		t.Error("missing epic header")
	}

	// Check task list
	if !strings.Contains(body, "- [ ] 1. Implement login system") {
		t.Error("missing unchecked task 1")
	}
	if !strings.Contains(body, "- [x] 2. Design database schema") {
		t.Error("missing checked task 2")
	}
	if !strings.Contains(body, "- [ ] 3. Add API endpoints") {
		t.Error("missing task 3")
	}

	// Check subtasks
	if !strings.Contains(body, "  - [ ] 1.1 Add password hashing") {
		t.Error("missing subtask 1.1")
	}
	if !strings.Contains(body, "  - [ ] 1.2 Add session management") {
		t.Error("missing subtask 1.2")
	}

	// Check footer
	if !strings.Contains(body, "This is a GitHub Tracking Issue") {
		t.Error("missing tracking issue note")
	}
}

func TestBuildEpicBody_WithIssueRefs(t *testing.T) {
	tasksContent := `- [ ] 1. First task <!-- Issue #10 -->
- [x] 2. Second task <!-- Issue #11 -->
- [ ] 3. Third task
`
	tasks := ParseTasks(tasksContent)
	body := buildEpicBody("test-spec", tasks, tasksContent)

	// Tasks with issue refs should show as #N
	if !strings.Contains(body, "- [ ] #10 First task") {
		t.Errorf("task with issue ref should be formatted as #10, got:\n%s", body)
	}
	if !strings.Contains(body, "- [x] #11 Second task") {
		t.Errorf("completed task with ref should be formatted as #11, got:\n%s", body)
	}
	// Task without ref should use ID
	if !strings.Contains(body, "- [ ] 3. Third task") {
		t.Errorf("task without ref should use ID format, got:\n%s", body)
	}
}

func TestUpdateConfigForEpic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal config
	configContent := `specs:
    base_path: .ai/specs
    active:
        - my-project
github:
    repo: owner/repo
`
	configPath := tmpDir + "/workflow.yaml"
	if err := writeTestFile(configPath, configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Update config
	if err := updateConfigForEpic(configPath, "my-project", 42); err != nil {
		t.Fatalf("updateConfigForEpic() error = %v", err)
	}

	// Read back and verify
	data, err := readTestFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "github_epic") {
		t.Errorf("config should contain github_epic mode, got:\n%s", content)
	}
	if !strings.Contains(content, "my-project") {
		t.Errorf("config should contain spec name, got:\n%s", content)
	}
	if !strings.Contains(content, "42") {
		t.Errorf("config should contain epic issue number 42, got:\n%s", content)
	}
}

// Helper functions for test file operations
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func readTestFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
