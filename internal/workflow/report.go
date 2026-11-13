package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Report holds the generated workflow report
type Report struct {
	Content     string
	Path        string
	GeneratedAt time.Time
}

// GenerateReport creates a Markdown report for the workflow stop
func GenerateReport(reason string, stats *WorkflowStats, sessionID string) *Report {
	now := time.Now()

	var sb strings.Builder

	// Header
	sb.WriteString("# AWK Workflow Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated**: %s\n", now.Format("2006-01-02 15:04:05")))
	if sessionID != "" {
		sb.WriteString(fmt.Sprintf("**Session ID**: %s\n", sessionID))
	} else {
		sb.WriteString("**Session ID**: N/A\n")
	}
	sb.WriteString(fmt.Sprintf("**Exit Reason**: %s\n", reason))
	sb.WriteString("\n---\n\n")

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Issues**: %d\n", stats.TotalIssues))
	sb.WriteString(fmt.Sprintf("- **Closed Issues**: %d\n", stats.ClosedIssues))
	sb.WriteString(fmt.Sprintf("- **Open Issues**: %d\n", stats.OpenIssues))
	sb.WriteString("\n")

	// Open Issues Breakdown
	sb.WriteString("### Open Issues Breakdown\n\n")
	sb.WriteString("| Status | Count |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| In Progress | %d |\n", stats.InProgress))
	sb.WriteString(fmt.Sprintf("| PR Ready | %d |\n", stats.PRReady))
	sb.WriteString(fmt.Sprintf("| Worker Failed | %d |\n", stats.WorkerFailed))
	sb.WriteString(fmt.Sprintf("| Needs Human Review | %d |\n", stats.NeedsReview))
	sb.WriteString("\n---\n\n")

	// Exit Reason Details
	sb.WriteString("## Exit Reason Details\n\n")
	sb.WriteString(formatExitReasonDetails(reason))
	sb.WriteString("\n---\n\n")

	// Next Steps
	sb.WriteString("## Next Steps\n\n")
	if stats.WorkerFailed > 0 || stats.NeedsReview > 0 {
		sb.WriteString("### ⚠ Attention Required\n\n")
		if stats.WorkerFailed > 0 {
			sb.WriteString(fmt.Sprintf("- **%d** issues failed (worker-failed) - need investigation\n", stats.WorkerFailed))
		}
		if stats.NeedsReview > 0 {
			sb.WriteString(fmt.Sprintf("- **%d** issues need human review\n", stats.NeedsReview))
		}
		sb.WriteString("\n")
	}

	if stats.PRReady > 0 {
		sb.WriteString("### PRs Ready for Review\n\n")
		sb.WriteString(fmt.Sprintf("There are **%d** PRs ready for review. Run `awkit run` to continue processing.\n\n", stats.PRReady))
	}

	return &Report{
		Content:     sb.String(),
		GeneratedAt: now,
	}
}

// formatExitReasonDetails returns the detailed explanation for each exit reason
func formatExitReasonDetails(reason string) string {
	switch reason {
	case "all_tasks_complete":
		return "✓ **All tasks completed successfully!**\n\nNo further action required.\n"
	case "user_stopped":
		return "⏸ **Workflow was stopped by user.**\n\nTo resume:\n1. Remove stop marker: `rm .ai/state/STOP`\n2. Run: `awkit run`\n"
	case "max_loop_reached":
		return "⚠ **Workflow stopped: maximum loop count reached (1000).**\n\nThis may indicate an infinite loop or stuck state. Please investigate.\n"
	case "max_consecutive_failures":
		return "⚠ **Workflow stopped: too many consecutive failures.**\n\nPlease review failed issues and fix underlying problems.\n"
	case "contract_violation":
		return "✗ **Workflow stopped: variable contract violation.**\n\nA required variable was missing. Check `awkit analyze-next` output.\n"
	case "error_exit":
		return "✗ **Workflow stopped due to an error.**\n\nPlease check logs for details.\n"
	case "max_failures":
		return "⚠ **Workflow stopped: maximum failures reached.**\n\nPlease review failed issues and fix underlying problems.\n"
	case "escalation_triggered":
		return "⚠ **Workflow stopped: escalation triggered.**\n\nHuman intervention required.\n"
	case "interrupted":
		return "⏸ **Workflow was interrupted.**\n\nRun `awkit run` to resume.\n"
	case "worker_failed":
		return "⚠ **Workflow stopped: a Worker failed and exhausted its retries.**\n\nReview the failed issue, fix the cause, then run `awkit run`.\n"
	case "needs_human_review", "review_max_retries":
		return "⚠ **Workflow stopped: an issue needs human review.**\n\nResolve the flagged issue, then run `awkit run`.\n"
	case "no_actionable_tasks":
		return "⏸ **Workflow stopped: no actionable tasks.**\n\nNothing is ready to work on. Check the open issues, then run `awkit run`.\n"
	case "invalid_ticket":
		return "⚠ **Workflow stopped: a ticket cannot be used.**\n\nThe issue has a comment that says what is wrong with its `yaml awk-ticket` block. Correct the block, remove the `needs-human-review` label, then run `awkit run`.\n"
	case "blocked_by_dependencies":
		return "⏸ **Workflow stopped: every pending ticket waits for another issue.**\n\nA ticket starts once every issue in its `depends_on` is closed as completed. What the tickets wait for is still open and not pending, was closed as not planned, or the tickets wait for each other. Reopen or finish that issue, or correct `depends_on`, then run `awkit run`.\n"
	case "config_not_found":
		return "✗ **Workflow stopped: workflow config not found.**\n\nCheck `.ai/config/workflow.yaml`, or run `awkit doctor`.\n"
	case "loop_count_error", "github_api_error":
		return "✗ **Workflow stopped: the analyzer could not read its state.**\n\nRun `awkit doctor` and check GitHub CLI access, then run `awkit run`.\n"
	default:
		return fmt.Sprintf("? **Workflow stopped for reason: %s**\n\nPlease check logs for details.\n", reason)
	}
}

// SaveReport saves the report to the state directory
func SaveReport(stateRoot string, report *Report) (string, error) {
	stateDir := filepath.Join(stateRoot, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create state directory: %w", err)
	}

	filename := fmt.Sprintf("workflow-report-%s.md", report.GeneratedAt.Format("20060102-150405"))
	reportPath := filepath.Join(stateDir, filename)

	if err := os.WriteFile(reportPath, []byte(report.Content), 0644); err != nil {
		return "", fmt.Errorf("failed to write report: %w", err)
	}

	report.Path = reportPath
	return reportPath, nil
}

// FormatSummary generates the stderr summary output
func FormatSummary(reason string, stats *WorkflowStats, reportPath string) string {
	var sb strings.Builder

	sb.WriteString("\n==========================================\n")
	sb.WriteString("  AWK Workflow Stopped\n")
	sb.WriteString("==========================================\n\n")
	sb.WriteString(fmt.Sprintf("Exit Reason: %s\n\n", reason))
	sb.WriteString("Summary:\n")
	sb.WriteString(fmt.Sprintf("  - Total Issues: %d\n", stats.TotalIssues))
	sb.WriteString(fmt.Sprintf("  - Closed: %d\n", stats.ClosedIssues))
	sb.WriteString(fmt.Sprintf("  - Open: %d\n", stats.OpenIssues))
	sb.WriteString("\n")

	if stats.WorkerFailed > 0 || stats.NeedsReview > 0 {
		sb.WriteString("⚠ Attention Required:\n")
		if stats.WorkerFailed > 0 {
			sb.WriteString(fmt.Sprintf("  - %d issues failed (worker-failed)\n", stats.WorkerFailed))
		}
		if stats.NeedsReview > 0 {
			sb.WriteString(fmt.Sprintf("  - %d issues need human review\n", stats.NeedsReview))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Report: %s\n\n", reportPath))
	sb.WriteString("==========================================\n")

	return sb.String()
}
