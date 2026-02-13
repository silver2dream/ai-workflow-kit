package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/task"
)

func usageCreateEpic() {
	fmt.Fprint(os.Stderr, `Create GitHub Tracking Issue (Epic) from body file

Usage:
  awkit create-epic --spec <name> --body-file <path> [options]

Required:
  --spec        Spec name (e.g., "my-project")
  --body-file   Path to epic body markdown file

Options:
  --title       Override epic title (default: "[epic] <spec> task tracking")
  --repo        GitHub repo (owner/repo), uses config if not specified
  --state-root  Override state root (default: git root)
  --dry-run     Show epic body without creating
  --help        Show this help

Examples:
  awkit create-epic --spec snake-arena --body-file .ai/temp/create-epic-body.md
  awkit create-epic --spec snake-arena --body-file epic-body.md --dry-run
  awkit create-epic --spec snake-arena --body-file epic-body.md --title "[epic] Snake Arena v2"
`)
}

func cmdCreateEpic(args []string) int {
	fs := flag.NewFlagSet("create-epic", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageCreateEpic

	spec := fs.String("spec", "", "")
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
		usageCreateEpic()
		return 0
	}

	if *spec == "" {
		errorf("--spec is required\n")
		usageCreateEpic()
		return 2
	}

	if *bodyFile == "" {
		errorf("--body-file is required\n")
		usageCreateEpic()
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

	result, err := task.CreateEpic(context.Background(), task.CreateEpicOptions{
		SpecName:  *spec,
		Title:     *title,
		Repo:      *repo,
		StateRoot: root,
		DryRun:    *dryRun,
		BodyFile:  *bodyFile,
	})
	if err != nil {
		errorf("%v\n", err)
		return 1
	}

	if *dryRun {
		fmt.Println("--- Epic Body Preview ---")
		fmt.Println(result.DryRunBody)
		fmt.Println("--- End Preview ---")
		return 0
	}

	fmt.Printf("Created Epic #%d\n", result.EpicNumber)
	if result.EpicURL != "" {
		fmt.Printf("URL: %s\n", result.EpicURL)
	}
	fmt.Println("Config updated: specs.tracking.mode = github_epic")
	return 0
}
