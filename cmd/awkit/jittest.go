package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/jittest"
)

func usageJiTTest() {
	fmt.Fprint(os.Stderr, `Run JiT (Just-in-Time) tests for a PR

JiT tests are generated from the PR diff, executed once, then discarded.
They provide an independent verification layer beyond Worker-written tests.

Usage:
  awkit jittest --pr <N> --issue <N> [options]

Required:
  --pr          PR number
  --issue       Issue number

Options:
  --state-root  Override state root (default: git root)
  --dry-run     Show what would be done without executing
  --help        Show this help

Output:
  JSON report with generated test results.

Examples:
  awkit jittest --pr 42 --issue 10
  awkit jittest --pr 42 --issue 10 --dry-run
`)
}

func cmdJiTTest(args []string) int {
	fs := flag.NewFlagSet("jittest", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageJiTTest

	pr := fs.Int("pr", 0, "")
	issue := fs.Int("issue", 0, "")
	stateRoot := fs.String("state-root", "", "")
	dryRun := fs.Bool("dry-run", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageJiTTest()
		return 0
	}

	if *pr == 0 {
		errorf("--pr is required\n")
		usageJiTTest()
		return 2
	}
	if *issue == 0 {
		errorf("--issue is required\n")
		usageJiTTest()
		return 2
	}

	root := *stateRoot
	if root == "" {
		var err error
		root, err = resolveGitRoot()
		if err != nil {
			errorf("failed to resolve git root: %v\n", err)
			return 1
		}
	}

	// Load config
	cfg, err := analyzer.LoadConfig(root + "/.ai/config/workflow.yaml")
	if err != nil {
		errorf("failed to load config: %v\n", err)
		return 1
	}

	jitCfg := cfg.Review.JiTTest
	if !jitCfg.IsEnabled() {
		errorf("jittest is not enabled in workflow.yaml (set review.jittest.enabled: true)\n")
		return 1
	}

	if *dryRun {
		fmt.Printf("JiTTest dry-run:\n")
		fmt.Printf("  PR: #%d\n", *pr)
		fmt.Printf("  Issue: #%d\n", *issue)
		fmt.Printf("  Max tests: %d\n", jitCfg.MaxTests)
		fmt.Printf("  Timeout: %ds\n", jitCfg.TimeoutSeconds)
		fmt.Printf("  Failure policy: %s\n", jitCfg.FailurePolicy)
		fmt.Printf("  Model: %s\n", jitCfg.Model)
		return 0
	}

	input := jittest.Input{
		PRNumber:    *pr,
		IssueNumber: *issue,
		WorkDir:     root,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(jitCfg.TimeoutSeconds)*time.Second)
	defer cancel()

	result, err := jittest.Run(ctx, input, jitCfg)
	if err != nil {
		errorf("jittest: %v\n", err)
		return 1
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		errorf("failed to encode JSON: %v\n", err)
		return 1
	}

	if result.IsBlocking(jitCfg.FailurePolicy) {
		return 1
	}

	return 0
}
