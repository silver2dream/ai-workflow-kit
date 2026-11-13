package analyzer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
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
				resultDir := layout.Results(tmpDir)
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
