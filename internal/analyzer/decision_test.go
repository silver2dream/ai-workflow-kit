package analyzer

import (
	"strings"
	"testing"
)

func TestDecision_FormatBashOutput(t *testing.T) {
	tests := []struct {
		name     string
		decision *Decision
		want     map[string]string
	}{
		{
			name: "dispatch worker",
			decision: &Decision{
				NextAction:  ActionDispatchWorker,
				IssueNumber: 42,
				PRNumber:    0,
				ExitReason:  "",
				MergeIssue:  "",
			},
			want: map[string]string{
				"NEXT_ACTION":  ActionDispatchWorker,
				"ISSUE_NUMBER": "42",
				"PR_NUMBER":    "0",
				"EXIT_REASON":  "",
				"MERGE_ISSUE":  "",
			},
		},
		{
			name: "review PR",
			decision: &Decision{
				NextAction:  ActionReviewPR,
				IssueNumber: 10,
				PRNumber:    100,
				ExitReason:  "",
				MergeIssue:  "",
			},
			want: map[string]string{
				"NEXT_ACTION":  ActionReviewPR,
				"ISSUE_NUMBER": "10",
				"PR_NUMBER":    "100",
			},
		},
		{
			name: "none with exit reason",
			decision: &Decision{
				NextAction:  ActionNone,
				IssueNumber: 20,
				PRNumber:    0,
				ExitReason:  ReasonWorkerFailed,
				MergeIssue:  "",
			},
			want: map[string]string{
				"NEXT_ACTION":  ActionNone,
				"ISSUE_NUMBER": "20",
				"EXIT_REASON":  ReasonWorkerFailed,
			},
		},
		{
			name: "merge conflict",
			decision: &Decision{
				NextAction:  ActionDispatchWorker,
				IssueNumber: 30,
				PRNumber:    300,
				ExitReason:  "",
				MergeIssue:  MergeIssueConflict,
			},
			want: map[string]string{
				"NEXT_ACTION":  ActionDispatchWorker,
				"ISSUE_NUMBER": "30",
				"PR_NUMBER":    "300",
				"MERGE_ISSUE":  MergeIssueConflict,
			},
		},
		{
			name: "all complete",
			decision: &Decision{
				NextAction:  ActionAllComplete,
				IssueNumber: 0,
				PRNumber:    0,
				ExitReason:  "",
				MergeIssue:  "",
			},
			want: map[string]string{
				"NEXT_ACTION": ActionAllComplete,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.decision.FormatBashOutput()

			// Parse the output into key-value pairs
			lines := strings.Split(strings.TrimSpace(output), "\n")
			got := make(map[string]string)
			for _, line := range lines {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					got[parts[0]] = parts[1]
				}
			}

			for key, wantVal := range tt.want {
				if gotVal, ok := got[key]; !ok {
					t.Errorf("Missing key %q in output", key)
				} else if gotVal != wantVal {
					t.Errorf("%s = %q, want %q", key, gotVal, wantVal)
				}
			}
			// The spec-driven fields are gone from the output altogether.
			for _, gone := range []string{"SPEC_NAME", "TASK_LINE", "EPIC_ISSUE", "TASK_TEXT"} {
				if _, ok := got[gone]; ok {
					t.Errorf("the output still has %s", gone)
				}
			}
			if len(got) != 5 {
				t.Errorf("the output has %d variables, want the five the loop reads: %v", len(got), got)
			}
		})
	}
}

func TestActionConstants(t *testing.T) {
	// Ensure action constants have expected values
	actions := map[string]string{
		"ActionDispatchWorker": ActionDispatchWorker,
		"ActionCheckResult":    ActionCheckResult,
		"ActionReviewPR":       ActionReviewPR,
		"ActionAllComplete":    ActionAllComplete,
		"ActionNone":           ActionNone,
	}

	expected := map[string]string{
		"ActionDispatchWorker": "dispatch_worker",
		"ActionCheckResult":    "check_result",
		"ActionReviewPR":       "review_pr",
		"ActionAllComplete":    "all_complete",
		"ActionNone":           "none",
	}

	for name, got := range actions {
		want := expected[name]
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestExitReasonConstants(t *testing.T) {
	// Ensure exit reason constants have expected values
	reasons := map[string]string{
		"ReasonWorkerFailed":           ReasonWorkerFailed,
		"ReasonNeedsHumanReview":       ReasonNeedsHumanReview,
		"ReasonReviewMaxRetries":       ReasonReviewMaxRetries,
		"ReasonMaxLoopReached":         ReasonMaxLoopReached,
		"ReasonMaxConsecutiveFailures": ReasonMaxConsecutiveFailures,
		"ReasonNoActionableTasks":      ReasonNoActionableTasks,
		"ReasonConfigNotFound":         ReasonConfigNotFound,
		"ReasonConfigInvalid":          ReasonConfigInvalid,
		"ReasonLoopCountError":         ReasonLoopCountError,
		"ReasonGitHubAPIError":         ReasonGitHubAPIError,
	}

	expected := map[string]string{
		"ReasonWorkerFailed":           "worker_failed",
		"ReasonNeedsHumanReview":       "needs_human_review",
		"ReasonReviewMaxRetries":       "review_max_retries",
		"ReasonMaxLoopReached":         "max_loop_reached",
		"ReasonMaxConsecutiveFailures": "max_consecutive_failures",
		"ReasonNoActionableTasks":      "no_actionable_tasks",
		"ReasonConfigNotFound":         "config_not_found",
		"ReasonConfigInvalid":          "config_invalid",
		"ReasonLoopCountError":         "loop_count_error",
		"ReasonGitHubAPIError":         "github_api_error",
	}

	for name, got := range reasons {
		want := expected[name]
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestMergeIssueConstants(t *testing.T) {
	if MergeIssueConflict != "conflict" {
		t.Errorf("MergeIssueConflict = %q, want %q", MergeIssueConflict, "conflict")
	}
	if MergeIssueRebase != "rebase" {
		t.Errorf("MergeIssueRebase = %q, want %q", MergeIssueRebase, "rebase")
	}
}
