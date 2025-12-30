package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/session"
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

	// 3. Generate report
	report := GenerateReport(opts.Reason, stats, sessionID)

	// 4. Save report
	reportPath, err := SaveReport(opts.StateRoot, report)
	if err != nil {
		return nil, fmt.Errorf("failed to save report: %w", err)
	}

	// 5. Cleanup state files
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
