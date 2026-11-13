package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/task"
)

func usageCreateTask() {
	fmt.Fprint(os.Stderr, `Create a ticket: a GitHub issue the workflow will pick up

Usage:
  awkit create-task --title <title> --body-file <path>

Required:
  --title       Issue title, e.g. "[feat] add login"
  --body-file   Path to the ticket body (markdown). It must have the sections
                Summary, Scope, Acceptance Criteria and Testing Requirements,
                at least one unchecked checkbox, and a "yaml awk-ticket" block
                that names the ticket's repo:

                    `+"```"+`yaml awk-ticket
                    schema: 1
                    repo: backend          # a repo of `+layout.ConfigFile+`, or root
                    depends_on: [12, 15]   # optional: issues that must be done first
                    `+"```"+`

Options:
  --repo        GitHub repo (owner/repo), uses config if not specified
  --state-root  Override state root (default: git root)
  --dry-run     Show gh command without executing
  --help        Show this help

Examples:
  awkit create-task --title "[feat] add login" --body-file `+layout.TempDir+`/ticket.md
  awkit create-task --title "[feat] add login" --body-file ticket.md --dry-run
`)
}

func cmdCreateTask(args []string) int {
	fs := flag.NewFlagSet("create-task", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageCreateTask

	bodyFile := fs.String("body-file", "", "")
	title := fs.String("title", "", "")
	repo := fs.String("repo", "", "")
	stateRoot := fs.String("state-root", "", "")
	dryRun := fs.Bool("dry-run", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageCreateTask()
		return 0
	}

	// Validate required arguments
	if *title == "" {
		errorf("--title is required\n")
		usageCreateTask()
		return 2
	}
	if *bodyFile == "" {
		errorf("--body-file is required\n")
		usageCreateTask()
		return 2
	}

	// Resolve state root
	root := *stateRoot
	if root == "" {
		var err error
		root, err = resolveGitRoot()
		if err != nil {
			errorf("failed to resolve git root: %v\n", err)
			return 1
		}
	}

	result, err := task.CreateTask(context.Background(), task.CreateTaskOptions{
		Title:     *title,
		BodyFile:  *bodyFile,
		Repo:      *repo,
		StateRoot: root,
		DryRun:    *dryRun,
	})
	if err != nil {
		errorf("%v\n", err)
		return 1
	}

	if *dryRun {
		fmt.Printf("DRY_RUN: %s\n", result.DryRunCmd)
		return 0
	}

	fmt.Printf("Created Issue #%d\n", result.IssueNumber)
	return 0
}
