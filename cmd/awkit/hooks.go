package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

func usageHooks() {
	fmt.Fprint(os.Stderr, `Manage lifecycle hooks

Usage:
  awkit hooks <subcommand>

Subcommands:
  list    Show configured hooks

Events:
  pre_dispatch   Before dispatching a worker
  post_dispatch  After worker completes
  pre_review     Before PR review
  post_review    After PR review decision
  on_merge       After successful PR merge
  on_failure     When worker fails after max retries

Examples:
  awkit hooks list
`)
}

func cmdHooks(args []string) int {
	if len(args) == 0 {
		usageHooks()
		return 2
	}

	switch args[0] {
	case "list":
		return cmdHooksList(args[1:])
	case "--help", "-h":
		usageHooks()
		return 0
	default:
		errorf("Unknown hooks subcommand: %s\n", args[0])
		usageHooks()
		return 2
	}
}

func cmdHooksList(args []string) int {
	fs := flag.NewFlagSet("hooks list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	stateRoot := fs.String("state-root", "", "")
	showHelp := fs.Bool("help", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp {
		fmt.Fprint(os.Stderr, "Show configured hooks\n\nUsage: awkit hooks list [--state-root <path>]\n")
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

	configPath := filepath.Join(*stateRoot, ".ai", "config", "workflow.yaml")
	cfg, err := analyzer.LoadConfig(configPath)
	if err != nil {
		errorf("failed to load config: %v\n", err)
		return 1
	}

	events := []string{"pre_dispatch", "post_dispatch", "pre_review", "post_review", "on_merge", "on_failure"}
	totalHooks := 0

	for _, event := range events {
		hooks := cfg.Hooks.GetHooks(event)
		if len(hooks) == 0 {
			continue
		}
		totalHooks += len(hooks)
		fmt.Printf("%s%s%s (%d hook%s):\n", colorBold, event, colorReset, len(hooks), plural(len(hooks)))
		for i, h := range hooks {
			policy := h.OnFailure
			if policy == "" {
				policy = "warn"
			}
			timeout := h.Timeout
			if timeout == "" {
				timeout = "none"
			}
			fmt.Printf("  [%d] %s\n", i, h.Command)
			fmt.Printf("      timeout: %s, on_failure: %s\n", timeout, policy)
			if len(h.Env) > 0 {
				fmt.Printf("      env: %v\n", h.Env)
			}
		}
		fmt.Println()
	}

	if totalHooks == 0 {
		fmt.Println("No hooks configured.")
		fmt.Println("")
		fmt.Println("Add hooks to .ai/config/workflow.yaml:")
		fmt.Println("  hooks:")
		fmt.Println("    pre_dispatch:")
		fmt.Println("      - command: \"echo dispatching\"")
		fmt.Println("        timeout: \"30s\"")
		fmt.Println("        on_failure: warn")
	} else {
		fmt.Printf("Total: %d hook%s configured\n", totalHooks, plural(totalHooks))
	}

	return 0
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
