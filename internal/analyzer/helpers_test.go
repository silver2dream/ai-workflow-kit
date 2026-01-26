package analyzer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestExtractPRNumberForIssue(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir, nil)

	tests := []struct {
		name        string
		issueNumber int
		issueBody   string
		resultFile  map[string]string
		want        int
	}{
		{
			name:        "from issue body",
			issueNumber: 1,
			issueBody:   "https://github.com/owner/repo/pull/100",
			resultFile:  nil,
			want:        100,
		},
		{
			name:        "from result file",
			issueNumber: 2,
			issueBody:   "no PR in body",
			resultFile:  map[string]string{"pr_url": "https://github.com/owner/repo/pull/200"},
			want:        200,
		},
		{
			name:        "result file takes precedence",
			issueNumber: 3,
			issueBody:   "https://github.com/owner/repo/pull/300",
			resultFile:  map[string]string{"pr_url": "https://github.com/owner/repo/pull/301"},
			want:        301,
		},
		{
			name:        "no PR found",
			issueNumber: 4,
			issueBody:   "no PR anywhere",
			resultFile:  nil,
			want:        0,
		},
		{
			name:        "empty result file pr_url",
			issueNumber: 5,
			issueBody:   "https://github.com/owner/repo/pull/500",
			resultFile:  map[string]string{"pr_url": ""},
			want:        500, // Falls back to body
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup result file if needed
			if tt.resultFile != nil {
				resultDir := filepath.Join(tmpDir, ".ai", "results")
				os.MkdirAll(resultDir, 0755)
				data, _ := json.Marshal(tt.resultFile)
				os.WriteFile(filepath.Join(resultDir, "issue-"+itoa(tt.issueNumber)+".json"), data, 0644)
			}

			got := a.extractPRNumberForIssue(tt.issueNumber, tt.issueBody)
			if got != tt.want {
				t.Errorf("extractPRNumberForIssue() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetReviewAttempts(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir, nil)

	// No file - should return 0
	count := a.getReviewAttempts(100)
	if count != 0 {
		t.Errorf("getReviewAttempts() = %d, want 0 (no file)", count)
	}

	// Create file with value
	attemptDir := filepath.Join(tmpDir, ".ai", "state", "attempts")
	os.MkdirAll(attemptDir, 0755)
	os.WriteFile(filepath.Join(attemptDir, "review-pr-100"), []byte("2"), 0644)

	count = a.getReviewAttempts(100)
	if count != 2 {
		t.Errorf("getReviewAttempts() = %d, want 2", count)
	}
}

func TestIncrementReviewAttempts(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir, nil)

	// Increment from 0
	if err := a.incrementReviewAttempts(200); err != nil {
		t.Fatalf("First increment failed: %v", err)
	}
	count := a.getReviewAttempts(200)
	if count != 1 {
		t.Errorf("After first increment, count = %d, want 1", count)
	}

	// Increment again
	if err := a.incrementReviewAttempts(200); err != nil {
		t.Fatalf("Second increment failed: %v", err)
	}
	count = a.getReviewAttempts(200)
	if count != 2 {
		t.Errorf("After second increment, count = %d, want 2", count)
	}
}

func TestCheckTasksFiles(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		config     *Config
		setupFiles func()
		wantAction string
		wantSpec   string
		wantLine   int
	}{
		{
			name:       "nil config",
			config:     nil,
			setupFiles: func() {},
			wantAction: "",
		},
		{
			name: "no active specs",
			config: &Config{
				Specs: SpecsConfig{
					Active: []string{},
				},
			},
			setupFiles: func() {},
			wantAction: "",
		},
		{
			name: "design exists but no tasks",
			config: &Config{
				Specs: SpecsConfig{
					BasePath: ".ai/specs",
					Active:   []string{"feature1"},
				},
			},
			setupFiles: func() {
				specDir := filepath.Join(tmpDir, ".ai", "specs", "feature1")
				os.MkdirAll(specDir, 0755)
				os.WriteFile(filepath.Join(specDir, "design.md"), []byte("# Design"), 0644)
			},
			wantAction: ActionGenerateTasks,
			wantSpec:   "feature1",
		},
		{
			name: "tasks with uncompleted task",
			config: &Config{
				Specs: SpecsConfig{
					BasePath: ".ai/specs",
					Active:   []string{"feature2"},
				},
			},
			setupFiles: func() {
				specDir := filepath.Join(tmpDir, ".ai", "specs", "feature2")
				os.MkdirAll(specDir, 0755)
				content := `# Tasks
- [x] Completed task <!-- Issue #1 -->
- [ ] Uncompleted task
- [x] Another completed <!-- Issue #2 -->
`
				os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte(content), 0644)
			},
			wantAction: ActionCreateTask,
			wantSpec:   "feature2",
			wantLine:   3, // Line 3 has the uncompleted task
		},
		{
			name: "all tasks completed or have issues",
			config: &Config{
				Specs: SpecsConfig{
					BasePath: ".ai/specs",
					Active:   []string{"feature3"},
				},
			},
			setupFiles: func() {
				specDir := filepath.Join(tmpDir, ".ai", "specs", "feature3")
				os.MkdirAll(specDir, 0755)
				content := `# Tasks
- [x] Completed task <!-- Issue #1 -->
- [ ] Pending task <!-- Issue #2 -->
`
				os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte(content), 0644)
			},
			wantAction: "", // No uncompleted task without Issue
		},
		{
			name: "empty spec name in active list",
			config: &Config{
				Specs: SpecsConfig{
					BasePath: ".ai/specs",
					Active:   []string{"", "  ", "feature4"},
				},
			},
			setupFiles: func() {
				specDir := filepath.Join(tmpDir, ".ai", "specs", "feature4")
				os.MkdirAll(specDir, 0755)
				os.WriteFile(filepath.Join(specDir, "design.md"), []byte("# Design"), 0644)
			},
			wantAction: ActionGenerateTasks,
			wantSpec:   "feature4",
		},
		{
			name: "spec with neither design nor tasks",
			config: &Config{
				Specs: SpecsConfig{
					BasePath: ".ai/specs",
					Active:   []string{"nonexistent"},
				},
			},
			setupFiles: func() {},
			wantAction: "", // Nothing to do
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up and setup
			os.RemoveAll(filepath.Join(tmpDir, ".ai"))
			tt.setupFiles()

			a := New(tmpDir, tt.config)
			decision := a.checkTasksFiles()

			if tt.wantAction == "" {
				if decision != nil {
					t.Errorf("checkTasksFiles() = %+v, want nil", decision)
				}
				return
			}

			if decision == nil {
				t.Fatal("checkTasksFiles() = nil, want non-nil")
			}

			if decision.NextAction != tt.wantAction {
				t.Errorf("NextAction = %q, want %q", decision.NextAction, tt.wantAction)
			}
			if decision.SpecName != tt.wantSpec {
				t.Errorf("SpecName = %q, want %q", decision.SpecName, tt.wantSpec)
			}
			if tt.wantLine > 0 && decision.TaskLine != tt.wantLine {
				t.Errorf("TaskLine = %d, want %d", decision.TaskLine, tt.wantLine)
			}
		})
	}
}

func TestUpdateIssueLabels(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := defaultTestConfig()

	a := newTestAnalyzer(tmpDir, config, mockClient)

	// Test adding and removing labels
	a.updateIssueLabels(context.Background(), 10,
		[]string{"needs-human-review", "blocked"},
		[]string{"in-progress", "pr-ready"})

	// Check removed labels
	if len(mockClient.RemovedLabels[10]) != 2 {
		t.Errorf("RemovedLabels count = %d, want 2", len(mockClient.RemovedLabels[10]))
	}

	// Check added labels
	if len(mockClient.AddedLabels[10]) != 2 {
		t.Errorf("AddedLabels count = %d, want 2", len(mockClient.AddedLabels[10]))
	}
}

func TestUpdateIssueLabels_WithErrors(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	mockClient.AddLabelError = os.ErrPermission
	mockClient.RemoveLabelError = os.ErrPermission
	config := defaultTestConfig()

	a := newTestAnalyzer(tmpDir, config, mockClient)

	// Should not panic even with errors
	a.updateIssueLabels(context.Background(), 20,
		[]string{"label1"},
		[]string{"label2"})
	// No assertion needed - just verify no panic
}

func TestWriteFileAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "subdir", "test.txt")

	// Should create parent directories
	err := writeFileAtomic(filePath, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("writeFileAtomic() error = %v", err)
	}

	// Verify content
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "test content" {
		t.Errorf("Content = %q, want %q", string(data), "test content")
	}

	// Overwrite should work
	err = writeFileAtomic(filePath, []byte("new content"), 0644)
	if err != nil {
		t.Fatalf("writeFileAtomic() overwrite error = %v", err)
	}

	data, _ = os.ReadFile(filePath)
	if string(data) != "new content" {
		t.Errorf("Content after overwrite = %q, want %q", string(data), "new content")
	}
}

func TestUpdateLoopCount_Error(t *testing.T) {
	// Use a path that should cause write errors
	tmpDir := t.TempDir()

	// Create a file where the directory should be to cause an error
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	os.MkdirAll(filepath.Dir(stateDir), 0755)
	os.WriteFile(stateDir, []byte("not a directory"), 0644)

	a := New(tmpDir, nil)
	_, err := a.updateLoopCount()
	if err == nil {
		t.Error("updateLoopCount() should return error when directory creation fails")
	}
}

// Helper function for converting int to string
func itoa(i int) string {
	return strconv.Itoa(i)
}
