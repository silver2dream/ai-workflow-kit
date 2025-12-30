package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/task"
)

func usageCreateTask() {
	fmt.Fprint(os.Stderr, `Create GitHub Issue from tasks.md entry

Usage:
  awkit create-task --spec <name> --task-line <N> --body-file <path>

Required:
  --spec        Spec name (e.g., "user-auth")
  --task-line   Line number in tasks.md (1-based)
  --body-file   Path to ticket body markdown file

Options:
  --title       Override issue title (auto-generated if not specified)
  --repo        GitHub repo (owner/repo), uses config if not specified
  --state-root  Override state root (default: git root)
  --dry-run     Show gh command without executing
  --help        Show this help

Examples:
  awkit create-task --spec user-auth --task-line 5 --body-file .ai/temp/create-task-body.md
  awkit create-task --spec user-auth --task-line 5 --body-file body.md --title "[feat] add login"
  awkit create-task --spec user-auth --task-line 5 --body-file body.md --dry-run
`)
}

func cmdCreateTask(args []string) int {
	fs := flag.NewFlagSet("create-task", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageCreateTask

	spec := fs.String("spec", "", "")
	taskLine := fs.Int("task-line", 0, "")
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
	if *spec == "" {
		errorf("--spec is required\n")
		usageCreateTask()
		return 2
	}
	if *taskLine <= 0 {
		errorf("--task-line is required and must be positive\n")
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

	// Execute create task
	result, err := task.CreateTask(context.Background(), task.CreateTaskOptions{
		SpecName:  *spec,
		TaskLine:  *taskLine,
		BodyFile:  *bodyFile,
		Title:     *title,
		Repo:      *repo,
		StateRoot: root,
		DryRun:    *dryRun,
	})
	if err != nil {
		errorf("%v\n", err)
		return 1
	}

	// Output result
	if *dryRun {
		fmt.Printf("DRY_RUN: %s\n", result.DryRunCmd)
		return 0
	}

	if result.Skipped {
		fmt.Printf("Skipped: Issue #%d already exists\n", result.IssueNumber)
		return 0
	}

	fmt.Printf("Created Issue #%d\n", result.IssueNumber)
	return 0
}
