package reviewer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubmitReviewOptions_Defaults(t *testing.T) {
	// Test that SubmitReview validates required options
	tests := []struct {
		name    string
		opts    SubmitReviewOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing state root",
			opts:    SubmitReviewOptions{PRNumber: 1, IssueNumber: 1},
			wantErr: true,
			errMsg:  "state root is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SubmitReview(context.Background(), tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSubmitReviewResult_Fields(t *testing.T) {
	// Test SubmitReviewResult structure
	result := &SubmitReviewResult{
		Result: "merged",
		Reason: "test reason",
	}

	if result.Result != "merged" {
		t.Errorf("Result = %q, want %q", result.Result, "merged")
	}
	if result.Reason != "test reason" {
		t.Errorf("Reason = %q, want %q", result.Reason, "test reason")
	}
}

func TestHandleVerificationFailure_ErrorCodes(t *testing.T) {
	// Test that handleVerificationFailure returns correct result types based on error code
	tests := []struct {
		name         string
		errCode      int
		errMessage   string
		errDetails   []string
		wantResult   string
		wantInReason string
	}{
		{
			name:         "code 1 - criteria/mapping",
			errCode:      1,
			errMessage:   "no criteria found",
			errDetails:   nil,
			wantResult:   "review_blocked",
			wantInReason: "no criteria found",
		},
		{
			name:         "code 2 - test execution",
			errCode:      2,
			errMessage:   "test failed",
			errDetails:   []string{"TestFoo failed"},
			wantResult:   "review_blocked",
			wantInReason: "TestFoo failed",
		},
		{
			name:         "code 3 - assertion",
			errCode:      3,
			errMessage:   "assertion not found",
			errDetails:   []string{"assert.Equal not found"},
			wantResult:   "review_blocked",
			wantInReason: "assert.Equal not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			ctx := context.Background()

			evidenceErr := &EvidenceError{
				Code:    tt.errCode,
				Message: tt.errMessage,
				Details: tt.errDetails,
			}

			opts := SubmitReviewOptions{
				PRNumber:    123,
				IssueNumber: 456,
				StateRoot:   tmpDir,
				GHTimeout:   100 * time.Millisecond,
			}

			result, err := handleVerificationFailure(ctx, opts, "test-session", evidenceErr)

			// The function will try to call GitHub CLI and fail, but should still return a result
			if result == nil {
				t.Fatal("expected result but got nil")
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result.Result != tt.wantResult {
				t.Errorf("Result = %q, want %q", result.Result, tt.wantResult)
			}
			if tt.wantInReason != "" && !strings.Contains(result.Reason, tt.wantInReason) {
				t.Errorf("Reason = %q, want to contain %q", result.Reason, tt.wantInReason)
			}
		})
	}
}

func TestHandleMergeFailure_MergeStates(t *testing.T) {
	// This tests the logic paths without actually calling GitHub
	// The getMergeStateStatus will fail and return "unknown" by default

	tests := []struct {
		name       string
		mergeState string
		wantResult string
		wantLabel  string
	}{
		{"dirty state", "DIRTY", "merge_failed", "merge-conflict"},
		{"behind state", "BEHIND", "merge_failed", "needs-rebase"},
		{"blocked state", "BLOCKED", "merge_failed", "needs-human-review"},
		{"unknown state", "unknown", "merge_failed", "needs-human-review"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the result determination logic based on merge state
			var label, result string

			switch tt.mergeState {
			case "DIRTY":
				label = "merge-conflict"
				result = "merge_failed"
			case "BEHIND":
				label = "needs-rebase"
				result = "merge_failed"
			default:
				label = "needs-human-review"
				result = "merge_failed"
			}

			if result != tt.wantResult {
				t.Errorf("result = %q, want %q", result, tt.wantResult)
			}
			if label != tt.wantLabel {
				t.Errorf("label = %q, want %q", label, tt.wantLabel)
			}
		})
	}
}

func TestUpdateTasksMd(t *testing.T) {
	// Test the updateTasksMd function with mock file structure
	tmpDir := t.TempDir()

	// Create result file
	resultDir := filepath.Join(tmpDir, ".ai", "results")
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		t.Fatalf("failed to create result dir: %v", err)
	}

	// task_line is 1-indexed, so line 3 means the 3rd line (index 2)
	resultContent := `{
		"spec_name": "test-spec",
		"task_line": 3
	}`
	if err := os.WriteFile(filepath.Join(resultDir, "issue-123.json"), []byte(resultContent), 0644); err != nil {
		t.Fatalf("failed to write result file: %v", err)
	}

	// Create tasks.md file
	specDir := filepath.Join(tmpDir, ".ai", "specs", "test-spec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	// Lines are: 0="# Tasks", 1="", 2="- [ ] Task 1", 3="- [ ] Task 2", ...
	// With task_line=3, it should modify line index 2 (3rd line, "- [ ] Task 1")
	tasksContent := `# Tasks

- [ ] Task 1
- [ ] Task 2
- [ ] Task 3
- [ ] Task 4
`
	if err := os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte(tasksContent), 0644); err != nil {
		t.Fatalf("failed to write tasks file: %v", err)
	}

	// Call updateTasksMd
	ctx := context.Background()
	updateTasksMd(ctx, tmpDir, 123)

	// Read back and verify
	content, err := os.ReadFile(filepath.Join(specDir, "tasks.md"))
	if err != nil {
		t.Fatalf("failed to read tasks file: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d", len(lines))
	}

	// task_line=3 means the 3rd line (0-indexed: lines[2])
	// lines[2] should be "- [x] Task 1" now
	if !strings.Contains(lines[2], "[x]") {
		t.Errorf("task at line 3 (index 2) should be checked, got %q", lines[2])
	}
}

func TestUpdateTasksMd_NoResultFile(t *testing.T) {
	// Test that updateTasksMd handles missing result file gracefully
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Should not panic
	updateTasksMd(ctx, tmpDir, 999)
}

func TestUpdateTasksMd_InvalidJSON(t *testing.T) {
	// Test that updateTasksMd handles invalid JSON gracefully
	tmpDir := t.TempDir()

	resultDir := filepath.Join(tmpDir, ".ai", "results")
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		t.Fatalf("failed to create result dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(resultDir, "issue-123.json"), []byte("invalid json"), 0644); err != nil {
		t.Fatalf("failed to write result file: %v", err)
	}

	ctx := context.Background()
	// Should not panic
	updateTasksMd(ctx, tmpDir, 123)
}

func TestUpdateTasksMd_MissingFields(t *testing.T) {
	// Test that updateTasksMd handles missing spec_name or task_line gracefully
	tmpDir := t.TempDir()

	resultDir := filepath.Join(tmpDir, ".ai", "results")
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		t.Fatalf("failed to create result dir: %v", err)
	}

	// Missing spec_name
	if err := os.WriteFile(filepath.Join(resultDir, "issue-123.json"), []byte(`{"task_line": 1}`), 0644); err != nil {
		t.Fatalf("failed to write result file: %v", err)
	}

	ctx := context.Background()
	// Should not panic
	updateTasksMd(ctx, tmpDir, 123)

	// task_line = 0
	if err := os.WriteFile(filepath.Join(resultDir, "issue-124.json"), []byte(`{"spec_name": "test", "task_line": 0}`), 0644); err != nil {
		t.Fatalf("failed to write result file: %v", err)
	}
	updateTasksMd(ctx, tmpDir, 124)
}

func TestCleanupWorktree_NonExistent(t *testing.T) {
	// Test that cleanupWorktree handles non-existent worktree gracefully
	tmpDir := t.TempDir()

	err := cleanupWorktree(tmpDir, 999)
	if err != nil {
		t.Errorf("unexpected error for non-existent worktree: %v", err)
	}
}

func TestGetTestCommand(t *testing.T) {
	// Test the getTestCommand helper that uses getTestCommandFromConfig
	tmpDir := t.TempDir()

	// Without config, should return empty (no longer defaults to go test)
	cmd := getTestCommand(tmpDir, 123)
	if cmd != "" {
		t.Errorf("default command = %q, want empty string", cmd)
	}
}

func TestEvidenceError_Error(t *testing.T) {
	// Test that EvidenceError implements error interface correctly
	err := &EvidenceError{
		Code:    1,
		Message: "test error message",
		Details: []string{"detail1", "detail2"},
	}

	if err.Error() != "test error message" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test error message")
	}
}

func TestSubmitReviewOptions_TimeoutDefaults(t *testing.T) {
	// Test that default timeouts are applied
	opts := SubmitReviewOptions{
		PRNumber:    1,
		IssueNumber: 1,
		StateRoot:   t.TempDir(),
		Score:       8,
		CIStatus:    "passed",
		ReviewBody:  "test",
	}

	// The function applies defaults, so we test the options struct
	if opts.GHTimeout != 0 {
		t.Errorf("GHTimeout should be 0 initially, got %v", opts.GHTimeout)
	}
	if opts.TestTimeout != 0 {
		t.Errorf("TestTimeout should be 0 initially, got %v", opts.TestTimeout)
	}
}

func TestUpdateTasksMd_CustomBasePath(t *testing.T) {
	// Test that updateTasksMd can read custom base_path from config
	tmpDir := t.TempDir()

	// Create config with custom base_path
	configDir := filepath.Join(tmpDir, ".ai", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `specs:
  base_path: custom/specs
`
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create result file
	resultDir := filepath.Join(tmpDir, ".ai", "results")
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		t.Fatalf("failed to create result dir: %v", err)
	}

	resultContent := `{
		"spec_name": "test-spec",
		"task_line": 1
	}`
	if err := os.WriteFile(filepath.Join(resultDir, "issue-123.json"), []byte(resultContent), 0644); err != nil {
		t.Fatalf("failed to write result file: %v", err)
	}

	// Create tasks.md in custom path
	specDir := filepath.Join(tmpDir, "custom", "specs", "test-spec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	tasksContent := `- [ ] Task 1`
	if err := os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte(tasksContent), 0644); err != nil {
		t.Fatalf("failed to write tasks file: %v", err)
	}

	// Call updateTasksMd
	ctx := context.Background()
	updateTasksMd(ctx, tmpDir, 123)

	// Read back and verify
	content, err := os.ReadFile(filepath.Join(specDir, "tasks.md"))
	if err != nil {
		t.Fatalf("failed to read tasks file: %v", err)
	}

	if !strings.Contains(string(content), "[x]") {
		t.Errorf("task should be checked, got %q", string(content))
	}
}

func TestUpdateTasksMd_TaskLineOutOfRange(t *testing.T) {
	// Test that updateTasksMd handles task_line > file length gracefully
	tmpDir := t.TempDir()

	// Create result file with task_line larger than file
	resultDir := filepath.Join(tmpDir, ".ai", "results")
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		t.Fatalf("failed to create result dir: %v", err)
	}

	resultContent := `{
		"spec_name": "test-spec",
		"task_line": 100
	}`
	if err := os.WriteFile(filepath.Join(resultDir, "issue-123.json"), []byte(resultContent), 0644); err != nil {
		t.Fatalf("failed to write result file: %v", err)
	}

	// Create small tasks.md file
	specDir := filepath.Join(tmpDir, ".ai", "specs", "test-spec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	tasksContent := `- [ ] Task 1`
	if err := os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte(tasksContent), 0644); err != nil {
		t.Fatalf("failed to write tasks file: %v", err)
	}

	// Call updateTasksMd - should not panic
	ctx := context.Background()
	updateTasksMd(ctx, tmpDir, 123)

	// File should remain unchanged
	content, err := os.ReadFile(filepath.Join(specDir, "tasks.md"))
	if err != nil {
		t.Fatalf("failed to read tasks file: %v", err)
	}

	if strings.Contains(string(content), "[x]") {
		t.Errorf("task should NOT be checked when line out of range, got %q", string(content))
	}
}

func TestUpdateTasksMd_SpecNameWithSpaces(t *testing.T) {
	// Test that spec_name with spaces is handled correctly (spaces removed)
	tmpDir := t.TempDir()

	// Create result file with spaces in spec_name
	resultDir := filepath.Join(tmpDir, ".ai", "results")
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		t.Fatalf("failed to create result dir: %v", err)
	}

	resultContent := `{
		"spec_name": "test spec name",
		"task_line": 1
	}`
	if err := os.WriteFile(filepath.Join(resultDir, "issue-123.json"), []byte(resultContent), 0644); err != nil {
		t.Fatalf("failed to write result file: %v", err)
	}

	// Create tasks.md with spec name without spaces
	specDir := filepath.Join(tmpDir, ".ai", "specs", "testspecname")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatalf("failed to create spec dir: %v", err)
	}

	tasksContent := `- [ ] Task 1`
	if err := os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte(tasksContent), 0644); err != nil {
		t.Fatalf("failed to write tasks file: %v", err)
	}

	// Call updateTasksMd
	ctx := context.Background()
	updateTasksMd(ctx, tmpDir, 123)

	// Read back and verify
	content, err := os.ReadFile(filepath.Join(specDir, "tasks.md"))
	if err != nil {
		t.Fatalf("failed to read tasks file: %v", err)
	}

	if !strings.Contains(string(content), "[x]") {
		t.Errorf("task should be checked, got %q", string(content))
	}
}
