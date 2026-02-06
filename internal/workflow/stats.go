package workflow

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
)

// WorkflowStats holds GitHub issue statistics
type WorkflowStats struct {
	TotalIssues  int
	OpenIssues   int
	ClosedIssues int
	InProgress   int
	PRReady      int
	WorkerFailed int
	NeedsReview  int
}

// CollectGitHubStats collects issue statistics from GitHub
func CollectGitHubStats(ctx context.Context, timeout time.Duration) (*WorkflowStats, error) {
	stats := &WorkflowStats{}

	// Helper to run gh command with timeout and retry
	ghQuery := func(args ...string) int {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		output, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", args...)
		if err != nil {
			return 0
		}

		var items []struct {
			Number int `json:"number"`
		}
		if err := json.Unmarshal(output, &items); err != nil {
			return 0
		}
		return len(items)
	}

	// Count issues with ai-task label
	stats.TotalIssues = ghQuery("issue", "list", "--label", "ai-task", "--state", "all", "--json", "number")
	stats.OpenIssues = ghQuery("issue", "list", "--label", "ai-task", "--state", "open", "--json", "number")
	stats.ClosedIssues = stats.TotalIssues - stats.OpenIssues

	// Count issues by status labels
	stats.InProgress = ghQuery("issue", "list", "--label", "in-progress", "--state", "open", "--json", "number")
	stats.PRReady = ghQuery("issue", "list", "--label", "pr-ready", "--state", "open", "--json", "number")
	stats.WorkerFailed = ghQuery("issue", "list", "--label", "worker-failed", "--state", "open", "--json", "number")
	stats.NeedsReview = ghQuery("issue", "list", "--label", "needs-human-review", "--state", "open", "--json", "number")

	return stats, nil
}

// CollectGitHubStatsWithMock collects stats using provided mock data (for testing)
func CollectGitHubStatsWithMock(mockStats *WorkflowStats) *WorkflowStats {
	return mockStats
}

// String returns a human-readable summary
func (s *WorkflowStats) String() string {
	return "Total: " + strconv.Itoa(s.TotalIssues) +
		", Open: " + strconv.Itoa(s.OpenIssues) +
		", Closed: " + strconv.Itoa(s.ClosedIssues)
}
