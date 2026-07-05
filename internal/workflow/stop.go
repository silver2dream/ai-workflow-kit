package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/session"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// StopWorkflowOptions configures the stop workflow operation
type StopWorkflowOptions struct {
	Reason    string
	StateRoot string
	GHTimeout time.Duration
}

// StopWorkflowResult contains the result of stopping the workflow
type StopWorkflowResult struct {
	ReportPath string
	Stats      *WorkflowStats
	SessionID  string
}

// StopWorkflow stops the workflow and generates a report
func StopWorkflow(ctx context.Context, opts StopWorkflowOptions) (*StopWorkflowResult, error) {
	if opts.StateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}

	if opts.GHTimeout == 0 {
		opts.GHTimeout = 60 * time.Second
	}

	// 1. Collect GitHub statistics
	stats, err := CollectGitHubStats(ctx, opts.GHTimeout)
	if err != nil {
		// Non-fatal, continue with zero stats
		stats = &WorkflowStats{}
	}

	// 2. Get session information
	sessionMgr := session.NewManager(opts.StateRoot)
	sessionID := sessionMgr.GetCurrentSessionID()

	// 2b. Emit workflow_stop trace event
	trace.WriteEvent(trace.ComponentPrincipal, trace.TypeWorkflowStop, trace.LevelInfo,
		trace.WithData(map[string]any{
			"reason":         opts.Reason,
			"total_issues":   stats.TotalIssues,
			"closed_issues":  stats.ClosedIssues,
			"open_issues":    stats.OpenIssues,
			"worker_failed":  stats.WorkerFailed,
			"needs_review":   stats.NeedsReview,
		}))

	// 3. Generate report
	report := GenerateReport(opts.Reason, stats, sessionID)

	// 4. Save report
	reportPath, err := SaveReport(opts.StateRoot, report)
	if err != nil {
		return nil, fmt.Errorf("failed to save report: %w", err)
	}

	// 5. Write the STOP marker. The kickoff multi-session loop polls for this
	// file at the top of every session and halts when it appears. Without it,
	// stop-workflow only wrote a report while the loop kept restarting until it
	// hit max sessions — the reason a self-blocked review burned 50 sessions.
	if err := writeStopMarker(opts.StateRoot, opts.Reason); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write STOP marker: %v\n", err)
	}

	// 6. Cleanup transient counters
	cleanupStateFiles(opts.StateRoot)

	// 6. End session
	if sessionID != "" {
		_ = sessionMgr.EndPrincipal(sessionID, opts.Reason)
	}

	// 7. Print summary to stderr
	summary := FormatSummary(opts.Reason, stats, reportPath)
	fmt.Fprint(os.Stderr, summary)

	return &StopWorkflowResult{
		ReportPath: reportPath,
		Stats:      stats,
		SessionID:  sessionID,
	}, nil
}

// writeStopMarker creates the .ai/state/STOP file the kickoff loop halts on.
// This is what actually stops the multi-session loop; the report is only a
// human-facing artifact.
func writeStopMarker(stateRoot, reason string) error {
	stateDir := filepath.Join(stateRoot, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return err
	}
	content := fmt.Sprintf("STOP requested by stop-workflow\nReason: %s\n", reason)
	return os.WriteFile(filepath.Join(stateDir, "STOP"), []byte(content), 0644)
}

// cleanupStateFiles removes temporary state files
func cleanupStateFiles(stateRoot string) {
	stateDir := filepath.Join(stateRoot, ".ai", "state")

	// Remove loop_count
	loopCountPath := filepath.Join(stateDir, "loop_count")
	_ = os.Remove(loopCountPath)

	// Remove consecutive_failures
	failuresPath := filepath.Join(stateDir, "consecutive_failures")
	_ = os.Remove(failuresPath)
}

// ValidExitReasons returns the list of valid exit reasons
func ValidExitReasons() []string {
	return []string{
		"all_tasks_complete",
		"user_stopped",
		"error_exit",
		"max_failures",
		"escalation_triggered",
		"interrupted",
		"max_loop_reached",
		"max_consecutive_failures",
		"contract_violation",
		"none",
	}
}

// IsValidExitReason checks if the given reason is valid
func IsValidExitReason(reason string) bool {
	for _, valid := range ValidExitReasons() {
		if reason == valid {
			return true
		}
	}
	return false
}
