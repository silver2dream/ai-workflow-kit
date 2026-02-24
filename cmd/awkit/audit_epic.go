package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/epicaudit"
)

func usageAuditEpic() {
	fmt.Fprint(os.Stderr, `Audit Epic coverage against design.md

Usage:
  awkit audit-epic --spec <name> [options]

Required:
  --spec        Spec name

Options:
  --state-root  Override state root (default: git root)
  --help        Show this help

Output:
  JSON report with coverage analysis, gap hints, and suggested action.

Examples:
  awkit audit-epic --spec snake-arena
`)
}

func cmdAuditEpic(args []string) int {
	fs := flag.NewFlagSet("audit-epic", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageAuditEpic

	spec := fs.String("spec", "", "")
	stateRoot := fs.String("state-root", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageAuditEpic()
		return 0
	}

	if *spec == "" {
		errorf("--spec is required\n")
		usageAuditEpic()
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

	ghClient := analyzer.NewGitHubClient(30 * time.Second)

	report, err := epicaudit.RunAudit(context.Background(), epicaudit.AuditOptions{
		SpecName:  *spec,
		StateRoot: root,
	}, ghClient)
	if err != nil {
		errorf("%v\n", err)
		return 1
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		errorf("failed to encode JSON: %v\n", err)
		return 1
	}

	return 0
}
