package task

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCreateEpic_MissingBodyFile(t *testing.T) {
	_, err := CreateEpic(context.Background(), CreateEpicOptions{
		SpecName:  "test",
		StateRoot: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for missing body-file")
	}
	if !strings.Contains(err.Error(), "body-file is required") {
		t.Errorf("error = %q, want 'body-file is required'", err)
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

func TestCreateEpic_WithBodyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a pre-formatted body file
	bodyContent := "# My Epic\n\n## Tasks\n\n- [ ] First task\n- [ ] Second task\n"
	bodyPath := tmpDir + "/epic-body.md"
	if err := writeTestFile(bodyPath, bodyContent); err != nil {
		t.Fatalf("failed to write body file: %v", err)
	}

	// Create minimal config
	configDir := tmpDir + "/.ai/config"
	os.MkdirAll(configDir, 0755)
	configContent := `specs:
  base_path: .ai/specs
  active:
    - test-project
github:
  repo: owner/repo
`
	configPath := configDir + "/workflow.yaml"
	if err := writeTestFile(configPath, configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Test dry run with body file
	result, err := CreateEpic(context.Background(), CreateEpicOptions{
		SpecName:  "test-project",
		StateRoot: tmpDir,
		DryRun:    true,
		BodyFile:  bodyPath,
	})
	if err != nil {
		t.Fatalf("CreateEpic() error = %v", err)
	}
	if result.DryRunBody != bodyContent {
		t.Errorf("DryRunBody = %q, want %q", result.DryRunBody, bodyContent)
	}
}


// Helper functions for test file operations
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func readTestFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
