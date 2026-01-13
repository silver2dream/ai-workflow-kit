// Package errors provides centralized error types and exit codes for AWK.
package errors

import (
	"fmt"
	"strings"
)

// Exit codes for different error categories.
const (
	ExitSuccess         = 0
	ExitGeneralError    = 1
	ExitConfigError     = 2
	ExitValidationError = 3
	ExitGitError        = 4
	ExitNetworkError    = 5
)

// AWKError is the base error type for all AWK-specific errors.
type AWKError struct {
	Code    int
	Message string
	Cause   error
}

// Error returns the error message, including the cause if present.
func (e *AWKError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying cause of the error.
func (e *AWKError) Unwrap() error {
	return e.Cause
}

// NewConfigError creates a new configuration error.
func NewConfigError(msg string) *AWKError {
	return &AWKError{
		Code:    ExitConfigError,
		Message: msg,
	}
}

// NewConfigErrorWithCause creates a new configuration error with an underlying cause.
func NewConfigErrorWithCause(msg string, cause error) *AWKError {
	return &AWKError{
		Code:    ExitConfigError,
		Message: msg,
		Cause:   cause,
	}
}

// NewValidationError creates a new validation error.
func NewValidationError(msg string) *AWKError {
	return &AWKError{
		Code:    ExitValidationError,
		Message: msg,
	}
}

// NewValidationErrorWithCause creates a new validation error with an underlying cause.
func NewValidationErrorWithCause(msg string, cause error) *AWKError {
	return &AWKError{
		Code:    ExitValidationError,
		Message: msg,
		Cause:   cause,
	}
}

// NewGitError creates a new git error.
func NewGitError(msg string) *AWKError {
	return &AWKError{
		Code:    ExitGitError,
		Message: msg,
	}
}

// NewGitErrorWithCause creates a new git error with an underlying cause.
func NewGitErrorWithCause(msg string, cause error) *AWKError {
	return &AWKError{
		Code:    ExitGitError,
		Message: msg,
		Cause:   cause,
	}
}

// NewNetworkError creates a new network error.
func NewNetworkError(msg string) *AWKError {
	return &AWKError{
		Code:    ExitNetworkError,
		Message: msg,
	}
}

// NewNetworkErrorWithCause creates a new network error with an underlying cause.
func NewNetworkErrorWithCause(msg string, cause error) *AWKError {
	return &AWKError{
		Code:    ExitNetworkError,
		Message: msg,
		Cause:   cause,
	}
}

// NewGeneralError creates a new general error.
func NewGeneralError(msg string) *AWKError {
	return &AWKError{
		Code:    ExitGeneralError,
		Message: msg,
	}
}

// NewGeneralErrorWithCause creates a new general error with an underlying cause.
func NewGeneralErrorWithCause(msg string, cause error) *AWKError {
	return &AWKError{
		Code:    ExitGeneralError,
		Message: msg,
		Cause:   cause,
	}
}

// IsConfigError checks if an error is a configuration error.
func IsConfigError(err error) bool {
	if awkErr, ok := err.(*AWKError); ok {
		return awkErr.Code == ExitConfigError
	}
	return false
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	if awkErr, ok := err.(*AWKError); ok {
		return awkErr.Code == ExitValidationError
	}
	return false
}

// IsGitError checks if an error is a git error.
func IsGitError(err error) bool {
	if awkErr, ok := err.(*AWKError); ok {
		return awkErr.Code == ExitGitError
	}
	return false
}

// IsNetworkError checks if an error is a network error.
func IsNetworkError(err error) bool {
	if awkErr, ok := err.(*AWKError); ok {
		return awkErr.Code == ExitNetworkError
	}
	return false
}

// GetExitCode returns the exit code for an error.
// If the error is not an AWKError, it returns ExitGeneralError.
func GetExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if awkErr, ok := err.(*AWKError); ok {
		return awkErr.Code
	}
	return ExitGeneralError
}

// ErrorContext contains contextual information for detailed error messages.
//
// Property 11: Error Message Specificity
// For any failure during repo type operations, the error message SHALL include:
// - The specific operation that failed (Req 13.1)
// - The repo type and path involved (Req 13.2, 13.3)
// - A suggested fix when applicable (Req 13.4, 13.5)
type ErrorContext struct {
	Operation  string
	ErrorMsg   string
	RepoType   string
	RepoPath   string
	Worktree   string
	WorkDir    string
	Branch     string
	Suggestion string
}

// FormatError formats a detailed error message with context.
//
// Property 11: Error Message Specificity
func FormatError(ctx ErrorContext) string {
	var sb strings.Builder

	sb.WriteString("============================================================\n")
	sb.WriteString(fmt.Sprintf("ERROR: %s\n", ctx.ErrorMsg))
	sb.WriteString("============================================================\n")
	sb.WriteString(fmt.Sprintf("Operation: %s\n", ctx.Operation))

	if ctx.RepoType != "" {
		sb.WriteString(fmt.Sprintf("Repo Type: %s\n", ctx.RepoType))
	}
	if ctx.RepoPath != "" {
		sb.WriteString(fmt.Sprintf("Repo Path: %s\n", ctx.RepoPath))
	}
	if ctx.Worktree != "" {
		sb.WriteString(fmt.Sprintf("Worktree: %s\n", ctx.Worktree))
	}
	if ctx.WorkDir != "" {
		sb.WriteString(fmt.Sprintf("Work Dir: %s\n", ctx.WorkDir))
	}
	if ctx.Branch != "" {
		sb.WriteString(fmt.Sprintf("Branch: %s\n", ctx.Branch))
	}

	if ctx.Suggestion != "" {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("SUGGESTION: %s\n", ctx.Suggestion))
	}

	sb.WriteString("============================================================")

	return sb.String()
}

// GetErrorSuggestion returns a suggested fix for common errors.
func GetErrorSuggestion(operation, repoType string) string {
	suggestions := map[string]map[string]string{
		"worktree_setup": {
			"submodule": "Check that the submodule is properly initialized with 'git submodule update --init'",
			"directory": "Verify the directory path exists in the repository",
		},
		"git_commit": {
			"submodule": "Ensure changes are within the submodule boundary",
		},
		"git_push": {
			"submodule": "Check push permissions for both submodule and parent remotes",
		},
		"preflight": {
			"submodule": "Run 'git submodule update --init --recursive' to initialize submodules",
		},
	}

	if opSuggestions, ok := suggestions[operation]; ok {
		if suggestion, ok := opSuggestions[repoType]; ok {
			return suggestion
		}
	}
	return ""
}

// EarlyFailureContext contains information for early failure logging.
type EarlyFailureContext struct {
	IssueID   string
	Stage     string
	Message   string
	Repo      string
	RepoType  string
	RepoPath  string
	Timestamp string
}

// FormatEarlyFailureLog formats an early failure log message.
// Early failures occur before codex execution (e.g., preflight, worktree setup).
func FormatEarlyFailureLog(ctx EarlyFailureContext) string {
	var sb strings.Builder

	// Use a default timestamp if not provided
	timestamp := ctx.Timestamp
	if timestamp == "" {
		timestamp = "unknown"
	}

	// Use defaults for repo type and path
	repoType := ctx.RepoType
	if repoType == "" {
		repoType = "unknown"
	}
	repoPath := ctx.RepoPath
	if repoPath == "" {
		repoPath = "unknown"
	}

	sb.WriteString("============================================================\n")
	sb.WriteString(fmt.Sprintf("EARLY FAILURE LOG - issue-%s\n", ctx.IssueID))
	sb.WriteString("============================================================\n")
	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", timestamp))
	sb.WriteString(fmt.Sprintf("Stage: %s\n", ctx.Stage))
	sb.WriteString(fmt.Sprintf("Repo: %s\n", ctx.Repo))
	sb.WriteString(fmt.Sprintf("Repo Type: %s\n", repoType))
	sb.WriteString(fmt.Sprintf("Repo Path: %s\n", repoPath))
	sb.WriteString(fmt.Sprintf("Error: %s\n", ctx.Message))
	sb.WriteString("============================================================")

	return sb.String()
}
