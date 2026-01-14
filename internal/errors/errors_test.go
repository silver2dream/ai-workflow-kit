package errors

import (
	"errors"
	"strings"
	"testing"
)

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"ExitSuccess", ExitSuccess, 0},
		{"ExitGeneralError", ExitGeneralError, 1},
		{"ExitConfigError", ExitConfigError, 2},
		{"ExitValidationError", ExitValidationError, 3},
		{"ExitGitError", ExitGitError, 4},
		{"ExitNetworkError", ExitNetworkError, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, tt.code)
			}
		})
	}
}

func TestAWKError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AWKError
		expected string
	}{
		{
			name: "simple message",
			err: &AWKError{
				Code:    ExitConfigError,
				Message: "config missing",
			},
			expected: "config missing",
		},
		{
			name: "message with cause",
			err: &AWKError{
				Code:    ExitGitError,
				Message: "git operation failed",
				Cause:   errors.New("permission denied"),
			},
			expected: "git operation failed: permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAWKError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &AWKError{
		Code:    ExitGeneralError,
		Message: "operation failed",
		Cause:   cause,
	}

	if unwrapped := err.Unwrap(); unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Test with nil cause
	errNoCause := &AWKError{
		Code:    ExitGeneralError,
		Message: "no cause",
	}

	if unwrapped := errNoCause.Unwrap(); unwrapped != nil {
		t.Errorf("Unwrap() = %v, want nil", unwrapped)
	}
}

func TestAWKError_ErrorsIs(t *testing.T) {
	cause := errors.New("original error")
	err := &AWKError{
		Code:    ExitConfigError,
		Message: "config error",
		Cause:   cause,
	}

	// errors.Is should work through Unwrap
	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the cause")
	}
}

func TestNewConfigError(t *testing.T) {
	msg := "config missing"
	err := NewConfigError(msg)

	if err.Code != ExitConfigError {
		t.Errorf("Code = %d, want %d", err.Code, ExitConfigError)
	}
	if err.Message != msg {
		t.Errorf("Message = %q, want %q", err.Message, msg)
	}
	if err.Cause != nil {
		t.Errorf("Cause = %v, want nil", err.Cause)
	}
}

func TestNewConfigErrorWithCause(t *testing.T) {
	msg := "config load failed"
	cause := errors.New("file not found")
	err := NewConfigErrorWithCause(msg, cause)

	if err.Code != ExitConfigError {
		t.Errorf("Code = %d, want %d", err.Code, ExitConfigError)
	}
	if err.Message != msg {
		t.Errorf("Message = %q, want %q", err.Message, msg)
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewValidationError(t *testing.T) {
	msg := "invalid input"
	err := NewValidationError(msg)

	if err.Code != ExitValidationError {
		t.Errorf("Code = %d, want %d", err.Code, ExitValidationError)
	}
	if err.Message != msg {
		t.Errorf("Message = %q, want %q", err.Message, msg)
	}
}

func TestNewValidationErrorWithCause(t *testing.T) {
	msg := "validation failed"
	cause := errors.New("field required")
	err := NewValidationErrorWithCause(msg, cause)

	if err.Code != ExitValidationError {
		t.Errorf("Code = %d, want %d", err.Code, ExitValidationError)
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewGitError(t *testing.T) {
	msg := "git checkout failed"
	err := NewGitError(msg)

	if err.Code != ExitGitError {
		t.Errorf("Code = %d, want %d", err.Code, ExitGitError)
	}
	if err.Message != msg {
		t.Errorf("Message = %q, want %q", err.Message, msg)
	}
}

func TestNewGitErrorWithCause(t *testing.T) {
	msg := "git push failed"
	cause := errors.New("remote rejected")
	err := NewGitErrorWithCause(msg, cause)

	if err.Code != ExitGitError {
		t.Errorf("Code = %d, want %d", err.Code, ExitGitError)
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewNetworkError(t *testing.T) {
	msg := "connection timeout"
	err := NewNetworkError(msg)

	if err.Code != ExitNetworkError {
		t.Errorf("Code = %d, want %d", err.Code, ExitNetworkError)
	}
	if err.Message != msg {
		t.Errorf("Message = %q, want %q", err.Message, msg)
	}
}

func TestNewNetworkErrorWithCause(t *testing.T) {
	msg := "API call failed"
	cause := errors.New("connection refused")
	err := NewNetworkErrorWithCause(msg, cause)

	if err.Code != ExitNetworkError {
		t.Errorf("Code = %d, want %d", err.Code, ExitNetworkError)
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewGeneralError(t *testing.T) {
	msg := "something went wrong"
	err := NewGeneralError(msg)

	if err.Code != ExitGeneralError {
		t.Errorf("Code = %d, want %d", err.Code, ExitGeneralError)
	}
	if err.Message != msg {
		t.Errorf("Message = %q, want %q", err.Message, msg)
	}
}

func TestNewGeneralErrorWithCause(t *testing.T) {
	msg := "operation failed"
	cause := errors.New("unknown error")
	err := NewGeneralErrorWithCause(msg, cause)

	if err.Code != ExitGeneralError {
		t.Errorf("Code = %d, want %d", err.Code, ExitGeneralError)
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestIsConfigError(t *testing.T) {
	configErr := NewConfigError("config error")
	gitErr := NewGitError("git error")
	stdErr := errors.New("standard error")

	if !IsConfigError(configErr) {
		t.Error("IsConfigError should return true for config error")
	}
	if IsConfigError(gitErr) {
		t.Error("IsConfigError should return false for git error")
	}
	if IsConfigError(stdErr) {
		t.Error("IsConfigError should return false for standard error")
	}
}

func TestIsValidationError(t *testing.T) {
	validationErr := NewValidationError("validation error")
	configErr := NewConfigError("config error")
	stdErr := errors.New("standard error")

	if !IsValidationError(validationErr) {
		t.Error("IsValidationError should return true for validation error")
	}
	if IsValidationError(configErr) {
		t.Error("IsValidationError should return false for config error")
	}
	if IsValidationError(stdErr) {
		t.Error("IsValidationError should return false for standard error")
	}
}

func TestIsGitError(t *testing.T) {
	gitErr := NewGitError("git error")
	configErr := NewConfigError("config error")
	stdErr := errors.New("standard error")

	if !IsGitError(gitErr) {
		t.Error("IsGitError should return true for git error")
	}
	if IsGitError(configErr) {
		t.Error("IsGitError should return false for config error")
	}
	if IsGitError(stdErr) {
		t.Error("IsGitError should return false for standard error")
	}
}

func TestIsNetworkError(t *testing.T) {
	networkErr := NewNetworkError("network error")
	configErr := NewConfigError("config error")
	stdErr := errors.New("standard error")

	if !IsNetworkError(networkErr) {
		t.Error("IsNetworkError should return true for network error")
	}
	if IsNetworkError(configErr) {
		t.Error("IsNetworkError should return false for config error")
	}
	if IsNetworkError(stdErr) {
		t.Error("IsNetworkError should return false for standard error")
	}
}

func TestGetExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"nil error", nil, ExitSuccess},
		{"config error", NewConfigError("config"), ExitConfigError},
		{"validation error", NewValidationError("validation"), ExitValidationError},
		{"git error", NewGitError("git"), ExitGitError},
		{"network error", NewNetworkError("network"), ExitNetworkError},
		{"general error", NewGeneralError("general"), ExitGeneralError},
		{"standard error", errors.New("standard"), ExitGeneralError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetExitCode(tt.err); got != tt.expected {
				t.Errorf("GetExitCode() = %d, want %d", got, tt.expected)
			}
		})
	}
}

// TestPythonTestParity verifies the same behavior as the Python tests
func TestPythonTestParity(t *testing.T) {
	t.Run("test_error_exit_codes parity", func(t *testing.T) {
		// Python: ConfigError("config").exit_code == 2
		configErr := NewConfigError("config")
		if configErr.Code != 2 {
			t.Errorf("ConfigError exit code = %d, want 2", configErr.Code)
		}

		// Python: ValidationError("validation").exit_code == 3
		validationErr := NewValidationError("validation")
		if validationErr.Code != 3 {
			t.Errorf("ValidationError exit code = %d, want 3", validationErr.Code)
		}

		// Python: ExecutionError("execution").exit_code == 1
		// In Go, we use GeneralError for this
		generalErr := NewGeneralError("execution")
		if generalErr.Code != 1 {
			t.Errorf("GeneralError exit code = %d, want 1", generalErr.Code)
		}
	})

	t.Run("error message consistency", func(t *testing.T) {
		// Python test verifies error message is preserved
		msg := "config missing"
		err := NewConfigError(msg)
		if err.Message != msg {
			t.Errorf("Message = %q, want %q", err.Message, msg)
		}
	})
}

// ============================================================
// Error Message Specificity Tests (from test_error_handling.py)
// Property 11: Error Message Specificity
// ============================================================

func TestErrorMessageSpecificity(t *testing.T) {
	t.Run("error_includes_operation", func(t *testing.T) {
		// Test error includes specific operation (Req 13.1)
		formatted := FormatError(ErrorContext{
			Operation: "git_commit",
			ErrorMsg:  "Commit failed",
		})

		if !strings.Contains(formatted, "Operation: git_commit") {
			t.Errorf("FormatError should include operation, got: %s", formatted)
		}
	})

	t.Run("error_includes_repo_type", func(t *testing.T) {
		// Test error includes repo type (Req 13.2)
		formatted := FormatError(ErrorContext{
			Operation: "git_commit",
			ErrorMsg:  "Commit failed",
			RepoType:  "submodule",
		})

		if !strings.Contains(formatted, "Repo Type: submodule") {
			t.Errorf("FormatError should include repo type, got: %s", formatted)
		}
	})

	t.Run("error_includes_repo_path", func(t *testing.T) {
		// Test error includes repo path (Req 13.3)
		formatted := FormatError(ErrorContext{
			Operation: "git_commit",
			ErrorMsg:  "Commit failed",
			RepoPath:  "backend",
		})

		if !strings.Contains(formatted, "Repo Path: backend") {
			t.Errorf("FormatError should include repo path, got: %s", formatted)
		}
	})

	t.Run("error_includes_suggestion", func(t *testing.T) {
		// Test error includes suggestion (Req 13.4)
		formatted := FormatError(ErrorContext{
			Operation:  "git_commit",
			ErrorMsg:   "Commit failed",
			Suggestion: "Check file permissions",
		})

		if !strings.Contains(formatted, "SUGGESTION: Check file permissions") {
			t.Errorf("FormatError should include suggestion, got: %s", formatted)
		}
	})

	t.Run("error_includes_all_context", func(t *testing.T) {
		// Test error includes all context (Req 13.5)
		formatted := FormatError(ErrorContext{
			Operation:  "worktree_setup",
			ErrorMsg:   "Work directory not found",
			RepoType:   "submodule",
			RepoPath:   "backend",
			Worktree:   "/worktrees/issue-1",
			WorkDir:    "/worktrees/issue-1/backend",
			Branch:     "feat/ai-issue-1",
			Suggestion: "Check that the submodule is initialized",
		})

		checks := []string{
			"Operation: worktree_setup",
			"Repo Type: submodule",
			"Repo Path: backend",
			"Worktree:",
			"Work Dir:",
			"Branch:",
			"SUGGESTION:",
		}

		for _, check := range checks {
			if !strings.Contains(formatted, check) {
				t.Errorf("FormatError should include %q, got: %s", check, formatted)
			}
		}
	})
}

func TestErrorSuggestions(t *testing.T) {
	t.Run("submodule_worktree_suggestion", func(t *testing.T) {
		// Test suggestion for submodule worktree error
		suggestion := GetErrorSuggestion("worktree_setup", "submodule")

		if !strings.Contains(strings.ToLower(suggestion), "submodule") {
			t.Errorf("Suggestion should mention submodule, got: %s", suggestion)
		}
		if !strings.Contains(strings.ToLower(suggestion), "init") {
			t.Errorf("Suggestion should mention init, got: %s", suggestion)
		}
	})

	t.Run("directory_worktree_suggestion", func(t *testing.T) {
		// Test suggestion for directory worktree error
		suggestion := GetErrorSuggestion("worktree_setup", "directory")

		if !strings.Contains(strings.ToLower(suggestion), "directory") {
			t.Errorf("Suggestion should mention directory, got: %s", suggestion)
		}
	})

	t.Run("submodule_commit_suggestion", func(t *testing.T) {
		// Test suggestion for submodule commit error
		suggestion := GetErrorSuggestion("git_commit", "submodule")

		if !strings.Contains(strings.ToLower(suggestion), "boundary") {
			t.Errorf("Suggestion should mention boundary, got: %s", suggestion)
		}
	})

	t.Run("submodule_push_suggestion", func(t *testing.T) {
		// Test suggestion for submodule push error
		suggestion := GetErrorSuggestion("git_push", "submodule")

		if !strings.Contains(strings.ToLower(suggestion), "permission") {
			t.Errorf("Suggestion should mention permission, got: %s", suggestion)
		}
	})

	t.Run("unknown_operation_no_suggestion", func(t *testing.T) {
		// Test unknown operation returns empty suggestion
		suggestion := GetErrorSuggestion("unknown_op", "root")

		if suggestion != "" {
			t.Errorf("Unknown operation should return empty suggestion, got: %s", suggestion)
		}
	})
}

func TestErrorFormatting(t *testing.T) {
	t.Run("error_has_separator_lines", func(t *testing.T) {
		// Test error has separator lines
		formatted := FormatError(ErrorContext{
			Operation: "test",
			ErrorMsg:  "Test error",
		})

		if !strings.Contains(formatted, "============") {
			t.Errorf("FormatError should include separator lines, got: %s", formatted)
		}
	})

	t.Run("error_has_error_prefix", func(t *testing.T) {
		// Test error has ERROR prefix
		formatted := FormatError(ErrorContext{
			Operation: "test",
			ErrorMsg:  "Test error",
		})

		if !strings.Contains(formatted, "ERROR:") {
			t.Errorf("FormatError should include ERROR prefix, got: %s", formatted)
		}
	})

	t.Run("minimal_error", func(t *testing.T) {
		// Test minimal error with only required fields
		formatted := FormatError(ErrorContext{
			Operation: "test",
			ErrorMsg:  "Test error",
		})

		if !strings.Contains(formatted, "Operation: test") {
			t.Errorf("FormatError should include operation, got: %s", formatted)
		}
		if !strings.Contains(formatted, "ERROR: Test error") {
			t.Errorf("FormatError should include error message, got: %s", formatted)
		}
	})

	t.Run("error_without_suggestion", func(t *testing.T) {
		// Test error without suggestion doesn't have SUGGESTION line
		formatted := FormatError(ErrorContext{
			Operation: "test",
			ErrorMsg:  "Test error",
		})

		if strings.Contains(formatted, "SUGGESTION:") {
			t.Errorf("FormatError without suggestion should not include SUGGESTION line, got: %s", formatted)
		}
	})
}

func TestErrorByRepoType(t *testing.T) {
	repoTypes := []string{"root", "directory", "submodule"}

	for _, repoType := range repoTypes {
		t.Run("error_includes_repo_type_"+repoType, func(t *testing.T) {
			formatted := FormatError(ErrorContext{
				Operation: "test",
				ErrorMsg:  "Test error",
				RepoType:  repoType,
			})

			expected := "Repo Type: " + repoType
			if !strings.Contains(formatted, expected) {
				t.Errorf("FormatError should include %q, got: %s", expected, formatted)
			}
		})
	}
}

// ============================================================
// Early Failure Logging Tests
// ============================================================

func TestEarlyFailureLogging(t *testing.T) {
	t.Run("early_failure_log_includes_issue_id", func(t *testing.T) {
		// Test early failure log includes issue ID
		log := FormatEarlyFailureLog(EarlyFailureContext{
			IssueID: "42",
			Stage:   "preflight",
			Message: "working tree not clean",
		})

		if !strings.Contains(log, "issue-42") {
			t.Errorf("FormatEarlyFailureLog should include issue ID, got: %s", log)
		}
	})

	t.Run("early_failure_log_includes_stage", func(t *testing.T) {
		// Test early failure log includes failure stage
		log := FormatEarlyFailureLog(EarlyFailureContext{
			IssueID: "1",
			Stage:   "worktree",
			Message: "work directory not found",
		})

		if !strings.Contains(log, "Stage: worktree") {
			t.Errorf("FormatEarlyFailureLog should include stage, got: %s", log)
		}
	})

	t.Run("early_failure_log_includes_error_message", func(t *testing.T) {
		// Test early failure log includes error message
		log := FormatEarlyFailureLog(EarlyFailureContext{
			IssueID: "1",
			Stage:   "preflight",
			Message: "preflight.sh returned non-zero",
		})

		if !strings.Contains(log, "Error: preflight.sh returned non-zero") {
			t.Errorf("FormatEarlyFailureLog should include error message, got: %s", log)
		}
	})

	t.Run("early_failure_log_includes_repo_context", func(t *testing.T) {
		// Test early failure log includes repo context
		log := FormatEarlyFailureLog(EarlyFailureContext{
			IssueID:  "5",
			Stage:    "worktree",
			Message:  "work directory not found",
			Repo:     "backend",
			RepoType: "directory",
			RepoPath: "backend/",
		})

		checks := []string{
			"Repo: backend",
			"Repo Type: directory",
			"Repo Path: backend/",
		}

		for _, check := range checks {
			if !strings.Contains(log, check) {
				t.Errorf("FormatEarlyFailureLog should include %q, got: %s", check, log)
			}
		}
	})

	t.Run("early_failure_stages", func(t *testing.T) {
		// Test all early failure stages are properly logged
		stages := []string{"attempt_guard", "preflight", "worktree"}

		for _, stage := range stages {
			t.Run(stage, func(t *testing.T) {
				log := FormatEarlyFailureLog(EarlyFailureContext{
					IssueID: "1",
					Stage:   stage,
					Message: stage + " failed",
				})

				if !strings.Contains(log, "Stage: "+stage) {
					t.Errorf("FormatEarlyFailureLog should include stage %q, got: %s", stage, log)
				}
				if !strings.Contains(log, "EARLY FAILURE LOG") {
					t.Errorf("FormatEarlyFailureLog should include header, got: %s", log)
				}
			})
		}
	})
}
