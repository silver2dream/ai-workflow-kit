package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

func usageAnalyzeNext() {
	fmt.Fprint(os.Stderr, `Analyze and decide the next workflow action

Usage:
  awkit analyze-next [options]

Options:
  --json          Output as JSON instead of bash variables
  --issue N       Decide for ticket N only. Inside "awkit run --issue N" this
                  is already so: the run passes its limit on (AWKIT_RUN_ISSUE)
  --state-root    Override state root (default: current directory)
  --help          Show this help

Output Variables (bash):
  NEXT_ACTION     dispatch_worker | check_result | review_pr | all_complete | none
  ISSUE_NUMBER    Issue number (if applicable)
  PR_NUMBER       PR number (if applicable)
  EXIT_REASON     Reason for none action
  MERGE_ISSUE     conflict | rebase (when dispatch_worker needs to fix merge issues)

Examples:
  awkit analyze-next
  awkit analyze-next --json
  eval "$(awkit analyze-next)"
`)
}

// analyzerFor returns the decision engine for a project. It decides for one
// ticket only when the caller names one, or else when the run this command
// was started in is limited to one (analyzer.ScopeEnv).
func analyzerFor(stateRoot string, issue int) (*analyzer.Analyzer, error) {
	if issue <= 0 {
		fromRun, err := analyzer.ScopeFromEnv()
		if err != nil {
			return nil, err
		}
		issue = fromRun
	}
	a := analyzer.New(stateRoot, nil)
	a.LimitTo(issue)
	return a, nil
}

func cmdAnalyzeNext(args []string) int {
	fs := flag.NewFlagSet("analyze-next", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageAnalyzeNext

	jsonOutput := fs.Bool("json", false, "")
	issue := fs.Int("issue", 0, "")
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

	if *issue < 0 {
		errorf("--issue takes an issue number, got %d\n", *issue)
		return 2
	}

	if *stateRoot == "" {
		root, err := resolveGitRoot()
		if err != nil {
			errorf("failed to resolve git root: %v\n", err)
			return 1
		}
		*stateRoot = root
	}

	// The decision engine, limited to one ticket when the flag names one or
	// when the run this command was started in is.
	a, err := analyzerFor(*stateRoot, *issue)
	if err != nil {
		errorf("%v\n", err)
		return 1
	}

	// Initialize event writer for tracing
	eventSessionID := readCurrentPrincipalSession(*stateRoot)
	if eventSessionID != "" {
		if err := trace.InitGlobalWriter(*stateRoot, eventSessionID); err != nil {
			fmt.Fprintf(os.Stderr, "[analyze-next] warning: failed to init event writer: %v\n", err)
		} else {
			defer trace.CloseGlobalWriter()
		}
	}

	// Self-heal structural bad states before deciding, so a repaired base is in
	// place before any worker is dispatched. Most importantly this restores an
	// integration branch that lost its scaffold to an early false-completion —
	// otherwise every worker branches from an empty tree and the loop stalls.
	// Output is confined to stderr/trace so it never corrupts the decision on
	// stdout that the principal evals.
	reconcileStructuralState(*stateRoot)

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
