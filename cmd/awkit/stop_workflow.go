package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/workflow"
)

func usageStopWorkflow() {
	fmt.Fprint(os.Stderr, `Stop the AWK workflow and generate a report

Usage:
  awkit stop-workflow <reason>

Arguments:
  reason    Exit reason (required)

Valid reasons:
  all_tasks_complete      All tasks completed successfully
  user_stopped            User requested stop
  error_exit              Stopped due to error
  max_failures            Maximum failures reached
  escalation_triggered    Escalation triggered
  interrupted             Workflow interrupted
  max_loop_reached        Maximum loop count reached
  max_consecutive_failures Too many consecutive failures
  contract_violation      Variable contract violation
  none                    No specific reason

Options:
  --state-root    Override state root (default: git root)
  --help          Show this help

Examples:
  awkit stop-workflow all_tasks_complete
  awkit stop-workflow user_stopped
`)
}

func cmdStopWorkflow(args []string) int {
	fs := flag.NewFlagSet("stop-workflow", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageStopWorkflow

	stateRoot := fs.String("state-root", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageStopWorkflow()
		return 0
	}

	// Get reason from positional argument
	remaining := fs.Args()
	if len(remaining) < 1 {
		errorf("Error: reason is required\n\n")
		usageStopWorkflow()
		return 2
	}

	reason := strings.TrimSpace(remaining[0])
	if reason == "" {
		errorf("Error: reason cannot be empty\n")
		return 2
	}

	// Validate reason
	if !workflow.IsValidExitReason(reason) {
		errorf("Error: invalid reason %q\n", reason)
		errorf("Valid reasons: %s\n", strings.Join(workflow.ValidExitReasons(), ", "))
		return 2
	}

	// Resolve state root
	if *stateRoot == "" {
		root, err := resolveGitRoot()
		if err != nil {
			errorf("failed to resolve git root: %v\n", err)
			return 1
		}
		*stateRoot = root
	}

	// Check for script fallback
	if os.Getenv("AWKIT_USE_SCRIPT") == "1" {
		return runStopWorkflowScript(*stateRoot, reason)
	}

	// Run Go implementation
	ctx := context.Background()
	result, err := workflow.StopWorkflow(ctx, workflow.StopWorkflowOptions{
		Reason:    reason,
		StateRoot: *stateRoot,
		GHTimeout: 60 * time.Second,
	})

	if err != nil {
		errorf("stop-workflow failed: %v\n", err)
		return 1
	}

	// Output result (report path is already printed to stderr by StopWorkflow)
	_ = result

	return 0
}

func runStopWorkflowScript(stateRoot, reason string) int {
	scriptPath := filepath.Join(stateRoot, ".ai/scripts/stop_work.sh")
	cmd := exec.Command("bash", scriptPath, reason)
	cmd.Dir = stateRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}
