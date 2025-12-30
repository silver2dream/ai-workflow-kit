package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

func usageAnalyzeNext() {
	fmt.Fprint(os.Stderr, `Analyze and decide the next workflow action

Usage:
  awkit analyze-next [options]

Options:
  --json          Output as JSON instead of bash variables
  --state-root    Override state root (default: current directory)
  --help          Show this help

Output Variables (bash):
  NEXT_ACTION     generate_tasks | create_task | dispatch_worker | check_result | review_pr | all_complete | none
  ISSUE_NUMBER    Issue number (if applicable)
  PR_NUMBER       PR number (if applicable)
  SPEC_NAME       Spec name (if applicable)
  TASK_LINE       Task line number (if applicable)
  EXIT_REASON     Reason for none action

Examples:
  awkit analyze-next
  awkit analyze-next --json
  eval "$(awkit analyze-next)"
`)
}

func cmdAnalyzeNext(args []string) int {
	fs := flag.NewFlagSet("analyze-next", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageAnalyzeNext

	jsonOutput := fs.Bool("json", false, "")
	stateRoot := fs.String("state-root", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageAnalyzeNext()
		return 0
	}

	if *stateRoot == "" {
		root, err := resolveGitRoot()
		if err != nil {
			errorf("failed to resolve git root: %v\n", err)
			return 1
		}
		*stateRoot = root
	}

	// Create analyzer
	a := analyzer.New(*stateRoot, nil)

	// Decide next action
	ctx := context.Background()
	decision, err := a.Decide(ctx)
	if err != nil {
		errorf("analyze-next failed: %v\n", err)
		return 1
	}

	// Output
	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(decision); err != nil {
			errorf("failed to encode JSON: %v\n", err)
			return 1
		}
	} else {
		fmt.Print(decision.FormatBashOutput())
	}

	return 0
}
