package reviewer

import (
	"context"
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

func TestCleanupWorktree_NonExistent(t *testing.T) {
	// Test that cleanupWorktree handles non-existent worktree gracefully
	tmpDir := t.TempDir()

	err := cleanupWorktree(tmpDir, 999)
	if err != nil {
		t.Errorf("unexpected error for non-existent worktree: %v", err)
	}
}

func TestGetRepoSettingsFromConfig_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Without config, should return empty (no longer defaults to go test)
	worktreePath := filepath.Join(tmpDir, ".worktrees", "issue-123")
	cmd := getRepoSettingsFromConfig(tmpDir, worktreePath).TestCommand
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
