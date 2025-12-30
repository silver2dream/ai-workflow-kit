package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/generate"
)

func usageGenerate() {
	fmt.Fprint(os.Stderr, `Generate AWK helper docs and scaffolding

Usage:
  awkit generate [options]

Generates:
  - CLAUDE.md
  - AGENTS.md
  - .ai/rules/_kit/git-workflow.md
  - .claude/settings.local.json
  - .claude/{rules,skills} (symlink or copy)

Options:
  --dry-run        Show what would be generated without writing files
  --generate-ci    Generate GitHub Actions workflow(s) from config
  --state-root     Override state root (default: git root)
  --help           Show this help

Examples:
  awkit generate
  awkit generate --dry-run
  awkit generate --generate-ci
`)
}

func cmdGenerate(args []string) int {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageGenerate

	dryRun := fs.Bool("dry-run", false, "")
	generateCI := fs.Bool("generate-ci", false, "")
	stateRoot := fs.String("state-root", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageGenerate()
		return 0
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

	// Run pure Go generation
	result, err := generate.Generate(generate.Options{
		StateRoot:  *stateRoot,
		GenerateCI: *generateCI,
		DryRun:     *dryRun,
	})

	if err != nil {
		errorf("generate failed: %v\n", err)
		return 1
	}

	// Report success
	if *dryRun {
		fmt.Fprintf(os.Stderr, "\n[dry-run] Would generate %d files:\n", len(result.GeneratedFiles))
		for _, f := range result.GeneratedFiles {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}
	} else {
		fmt.Fprintf(os.Stderr, "\nGenerated %d files\n", len(result.GeneratedFiles))
	}

	return 0
}
