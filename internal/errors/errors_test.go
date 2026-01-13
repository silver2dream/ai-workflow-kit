package errors

import (
	"errors"
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
