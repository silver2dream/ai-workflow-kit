package main

import (
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/workflow"
)

// The principal-workflow skill tells the Principal to run
// `awkit stop-workflow <exit_reason>` with the reason analyze-next reports,
// so every reason the analyzer can emit must be accepted.
func TestStopWorkflowAcceptsAnalyzerExitReasons(t *testing.T) {
	reasons := []string{
		analyzer.ReasonWorkerFailed,
		analyzer.ReasonNeedsHumanReview,
		analyzer.ReasonReviewMaxRetries,
		analyzer.ReasonMaxLoopReached,
		analyzer.ReasonMaxConsecutiveFailures,
		analyzer.ReasonNoActionableTasks,
		analyzer.ReasonConfigNotFound,
		analyzer.ReasonConfigInvalid,
		analyzer.ReasonInvalidTicket,
		analyzer.ReasonBlockedByDependencies,
		analyzer.ReasonLoopCountError,
		analyzer.ReasonGitHubAPIError,
		analyzer.ReasonStopRequested,
	}
	for _, reason := range reasons {
		if !workflow.IsValidExitReason(reason) {
			t.Errorf("stop-workflow rejects analyzer exit reason %q", reason)
		}
	}
}
