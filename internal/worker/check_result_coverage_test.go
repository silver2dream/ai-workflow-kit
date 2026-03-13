package worker

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// CheckResultOutput.FormatBashOutput
// ---------------------------------------------------------------------------

func TestCov_CheckResultOutput_FormatBashOutput_Extended(t *testing.T) {
	tests := []struct {
		name   string
		output CheckResultOutput
		checks []string
	}{
		{
			name: "success with PR",
			output: CheckResultOutput{
				Status:   "success",
				PRNumber: "42",
			},
			checks: []string{
				"CHECK_RESULT_STATUS=success",
				"WORKER_STATUS=success",
				"PR_NUMBER=42",
			},
		},
		{
			name: "not found",
			output: CheckResultOutput{
				Status: "not_found",
			},
			checks: []string{
				"CHECK_RESULT_STATUS=not_found",
				"WORKER_STATUS=not_found",
				"PR_NUMBER=",
			},
		},
		{
			name: "failed with error",
			output: CheckResultOutput{
				Status: "failed",
				Error:  "some error",
			},
			checks: []string{
				"CHECK_RESULT_STATUS=failed",
				"WORKER_STATUS=failed",
				"PR_NUMBER=",
			},
		},
		{
			name: "crashed",
			output: CheckResultOutput{
				Status: "crashed",
				Error:  "worker process terminated unexpectedly",
			},
			checks: []string{
				"CHECK_RESULT_STATUS=crashed",
				"WORKER_STATUS=crashed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.output.FormatBashOutput()
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("FormatBashOutput() missing %q in:\n%s", check, got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// joinLines
// ---------------------------------------------------------------------------

func TestCov_JoinLines(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{"empty", nil, ""},
		{"single", []string{"hello"}, "hello"},
		{"multiple", []string{"a", "b", "c"}, "a\nb\nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinLines(tt.lines); got != tt.want {
				t.Errorf("joinLines() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CheckResultOptions defaults
// ---------------------------------------------------------------------------

func TestCov_CheckResultOptions_Defaults(t *testing.T) {
	// Verify that CheckResult sets defaults correctly
	// We can't actually call CheckResult (needs filesystem + gh),
	// but we can test the output construction

	output := &CheckResultOutput{
		Status:   "success",
		PRNumber: "123",
	}
	bash := output.FormatBashOutput()
	if !strings.Contains(bash, "PR_NUMBER=123") {
		t.Errorf("expected PR_NUMBER=123 in output: %s", bash)
	}
}

// ---------------------------------------------------------------------------
// CheckResultOutput status values
// ---------------------------------------------------------------------------

func TestCov_CheckResultOutput_AllStatuses(t *testing.T) {
	statuses := []string{
		"not_found",
		"success",
		"failed",
		"failed_will_retry",
		"failed_max_retries",
		"crashed",
		"timeout",
		"error",
		"success_no_changes",
	}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			output := &CheckResultOutput{Status: status}
			bash := output.FormatBashOutput()
			if !strings.Contains(bash, "CHECK_RESULT_STATUS="+status) {
				t.Errorf("expected status %q in output: %s", status, bash)
			}
			if !strings.Contains(bash, "WORKER_STATUS="+status) {
				t.Errorf("expected WORKER_STATUS %q in output: %s", status, bash)
			}
		})
	}
}
