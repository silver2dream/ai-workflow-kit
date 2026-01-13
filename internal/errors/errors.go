// Package errors provides centralized error types and exit codes for AWK.
package errors

import "fmt"

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
