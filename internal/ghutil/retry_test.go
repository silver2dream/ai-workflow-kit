package ghutil

import (
	"testing"
)

func TestIsRetryable_NonRetryableErrors(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		want     bool
	}{
		{
			name:     "authentication error",
			output:   "authentication required: please run gh auth login",
			exitCode: 1,
			want:     false,
		},
		{
			name:     "auth login prompt",
			output:   "To get started with GitHub CLI, please run: gh auth login",
			exitCode: 4,
			want:     false,
		},
		{
			name:     "not found 404",
			output:   "HTTP 404: Not Found",
			exitCode: 1,
			want:     false,
		},
		{
			name:     "validation failed 422",
			output:   "HTTP 422: Validation Failed",
			exitCode: 1,
			want:     false,
		},
		{
			name:     "already exists",
			output:   "label already exists",
			exitCode: 1,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.output, tt.exitCode)
			if got != tt.want {
				t.Errorf("IsRetryable(%q, %d) = %v, want %v", tt.output, tt.exitCode, got, tt.want)
			}
		})
	}
}

func TestIsRetryable_RetryableErrors(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		exitCode int
		want     bool
	}{
		{
			name:     "rate limit",
			output:   "API rate limit exceeded",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "rate_limit underscore",
			output:   "secondary rate_limit detected",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "server error 502",
			output:   "HTTP 502: Bad Gateway",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "server error 503",
			output:   "HTTP 503: Service Unavailable",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "timeout",
			output:   "context deadline exceeded (timeout)",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "timed out",
			output:   "net/http: request timed out",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "connection refused",
			output:   "dial tcp: connection refused",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "connection reset",
			output:   "read: connection reset by peer",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "no such host",
			output:   "dial tcp: lookup api.github.com: no such host",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "network error",
			output:   "network is unreachable",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "temporary failure",
			output:   "temporary failure in name resolution",
			exitCode: 1,
			want:     true,
		},
		{
			name:     "generic non-zero exit",
			output:   "something unexpected happened",
			exitCode: 1,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.output, tt.exitCode)
			if got != tt.want {
				t.Errorf("IsRetryable(%q, %d) = %v, want %v", tt.output, tt.exitCode, got, tt.want)
			}
		})
	}
}

func TestIsRetryable_SuccessExitCode(t *testing.T) {
	// Exit code 0 with empty output should not be retryable
	got := IsRetryable("", 0)
	if got != false {
		t.Errorf("IsRetryable(\"\", 0) = %v, want false", got)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
	}
	if cfg.BaseDelay != 2*1e9 { // 2 seconds in nanoseconds
		t.Errorf("BaseDelay = %v, want 2s", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 30*1e9 { // 30 seconds in nanoseconds
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
}
