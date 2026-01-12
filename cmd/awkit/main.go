package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	awkit "github.com/silver2dream/ai-workflow-kit"
	"github.com/silver2dream/ai-workflow-kit/internal/buildinfo"
	"github.com/silver2dream/ai-workflow-kit/internal/install"
	"github.com/silver2dream/ai-workflow-kit/internal/updatecheck"
	"github.com/silver2dream/ai-workflow-kit/internal/upgrade"
)

// ANSI color codes
var (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

func init() {
	// Disable colors on Windows or when not a terminal
	if runtime.GOOS == "windows" || os.Getenv("NO_COLOR") != "" {
		colorReset = ""
		colorRed = ""
		colorGreen = ""
		colorYellow = ""
		colorCyan = ""
		colorBold = ""
	}
}

var presets = []PresetInfo{
	// Single-repo presets
	{Name: "generic", Description: "Generic project (alias for node)", Category: "single-repo"},
	{Name: "go", Description: "Go single-repo project", Category: "single-repo"},
	{Name: "python", Description: "Python single-repo project", Category: "single-repo"},
	{Name: "rust", Description: "Rust single-repo project", Category: "single-repo"},
	{Name: "dotnet", Description: ".NET single-repo project", Category: "single-repo"},
	{Name: "node", Description: "Node.js/TypeScript single-repo project", Category: "single-repo"},
	// Monorepo presets
	{Name: "react-go", Description: "React frontend + Go backend", Category: "monorepo"},
	{Name: "react-python", Description: "React frontend + Python backend", Category: "monorepo"},
	{Name: "unity-go", Description: "Unity frontend + Go backend", Category: "monorepo"},
	{Name: "godot-go", Description: "Godot frontend + Go backend", Category: "monorepo"},
	{Name: "unreal-go", Description: "Unreal frontend + Go backend", Category: "monorepo"},
}

type PresetInfo struct {
	Name        string
	Description string
	Category    string // "single-repo" | "monorepo"
}

func availablePresetNames() string {
	names := make([]string, len(presets))
	for i, p := range presets {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}

func main() {
	os.Exit(run())
}

func run() int {
	// Handle --version and -v at top level
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println(buildinfo.Version)
			return 0
		case "--help", "-h":
			usage()
			return 0
		}
	}

	if len(os.Args) < 2 {
		usage()
		return 2
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(buildinfo.Version)
		warnUpdateAvailable()
		return 0
	case "check-update":
		return cmdCheckUpdate(os.Args[2:])
	case "init":
		return cmdInit(os.Args[2:])
	case "install":
		// Alias for init (backward compatibility)
		return cmdInit(os.Args[2:])
	case "uninstall":
		return cmdUninstall(os.Args[2:])
	case "upgrade":
		return cmdUpgrade(os.Args[2:])
	case "kickoff":
		return cmdKickoff(os.Args[2:])
	case "validate":
		return cmdValidate(os.Args[2:])
	case "status":
		return cmdStatus(os.Args[2:])
	case "next":
		return cmdNext(os.Args[2:])
	case "check-result":
		return cmdCheckResult(os.Args[2:])
	case "dispatch-worker":
		return cmdDispatchWorker(os.Args[2:])
	case "run-issue":
		return cmdRunIssue(os.Args[2:])
	case "session":
		return cmdSession(os.Args[2:])
	case "analyze-next":
		return cmdAnalyzeNext(os.Args[2:])
	case "stop-workflow":
		return cmdStopWorkflow(os.Args[2:])
	case "prepare-review":
		return cmdPrepareReview(os.Args[2:])
	case "submit-review":
		return cmdSubmitReview(os.Args[2:])
	case "create-task":
		return cmdCreateTask(os.Args[2:])
	case "doctor":
		return cmdDoctor(os.Args[2:])
	case "reset":
		return cmdReset(os.Args[2:])
	case "generate":
		return cmdGenerate(os.Args[2:])
	case "list-presets":
		return cmdListPresets()
	case "completion":
		return cmdCompletion(os.Args[2:])
	case "events":
		return cmdEvents(os.Args[2:])
	case "help":
		if len(os.Args) >= 3 {
			return cmdHelp(os.Args[2])
		}
		usage()
		return 0
	default:
		errorf("Unknown command: %s\n\n", os.Args[1])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `awkit - AI Workflow Kit CLI

Usage:
  awkit <command> [options]

Commands:
  init          Initialize AWK in a project (or current directory)
  upgrade       Upgrade AWK kit files (preserves workflow.yaml by default)
  uninstall     Remove AWK from a project
  kickoff       Start the AI workflow with PTY and progress monitoring
  validate      Validate workflow configuration
  status        Show offline workflow status
  next          Show suggested next actions (offline)
  check-result    Check worker execution result for an issue
  dispatch-worker Dispatch an issue to a worker
  run-issue       Run a worker for a single issue
  session         Manage Principal/Worker sessions
  analyze-next    Analyze and determine next workflow action
  stop-workflow   Stop the workflow and generate a report
  prepare-review  Prepare PR review context
  submit-review   Submit PR review and handle result
  create-task     Create GitHub Issue from tasks.md entry
  doctor          Check project health and identify issues
  reset           Reset project state for fresh start
  generate        Generate helper docs and scaffolding
  list-presets    Show available project presets
  events          Query unified event stream for debugging
  check-update  Check for CLI updates
  completion    Generate shell completion script
  version       Show version
  help          Show help for a command

Examples:
  awkit init
  awkit init --preset react-go
  awkit kickoff
  awkit kickoff --dry-run
  awkit validate
  awkit upgrade
  awkit init /path/to/project
  awkit uninstall .
  awkit check-update

Run 'awkit help <command>' for more information.
`)
}

// Helper functions for colored output
func success(format string, args ...interface{}) {
	fmt.Printf("%s✓%s %s", colorGreen, colorReset, fmt.Sprintf(format, args...))
}

func warn(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s⚠%s %s", colorYellow, colorReset, fmt.Sprintf(format, args...))
}

func errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%sError:%s %s", colorRed, colorReset, fmt.Sprintf(format, args...))
}

func info(format string, args ...interface{}) {
	fmt.Printf("%s", fmt.Sprintf(format, args...))
}

func bold(s string) string {
	return colorBold + s + colorReset
}

func cyan(s string) string {
	return colorCyan + s + colorReset
}

func cmdHelp(command string) int {
	switch command {
	case "init", "install":
		usageInit()
	case "upgrade":
		usageUpgrade()
	case "uninstall":
		usageUninstall()
	case "kickoff":
		usageKickoff()
	case "validate":
		usageValidate()
	case "status":
		usageStatus()
	case "next":
		usageNext()
	case "check-result":
		usageCheckResult()
	case "dispatch-worker":
		usageDispatchWorker()
	case "run-issue":
		usageRunIssue()
	case "session":
		usageSession()
	case "analyze-next":
		usageAnalyzeNext()
	case "stop-workflow":
		usageStopWorkflow()
	case "prepare-review":
		usagePrepareReview()
	case "submit-review":
		usageSubmitReview()
	case "create-task":
		usageCreateTask()
	case "doctor":
		usageDoctor()
	case "reset":
		usageReset()
	case "generate":
		usageGenerate()
	case "list-presets":
		fmt.Println("Show available project presets with descriptions.")
		fmt.Println("\nUsage: awkit list-presets")
	case "check-update":
		usageCheckUpdate()
	case "completion":
		usageCompletion()
	case "events":
		usageEvents()
	case "version":
		fmt.Println("Show the awkit version.")
	default:
		errorf("Unknown command: %s\n", command)
		return 2
	}
	return 0
}

func usageInit() {
	fmt.Fprintf(os.Stderr, `Initialize AWK in a project

Usage:
  awkit init [project_path] [options]

Arguments:
  project_path    Path to the project (default: current directory)

Options:
  --preset        Project preset (%s) [default: generic]
  --scaffold      Create minimal project structure for the preset
  --force         Overwrite all existing kit files (and scaffold files if --scaffold)
  --force-config  Overwrite only workflow.yaml (apply preset to existing project)
  --dry-run       Show what would be done without making changes
  --no-generate   Skip running generate.sh after init
  --project-name  Override project name in config

Examples:
  awkit init
  awkit init --preset react-go
  awkit init --preset go --scaffold
  awkit init /path/to/project --force
  awkit init --preset react-go --force-config  # Apply preset to existing project
  awkit init --dry-run

Run 'awkit list-presets' to see available presets.
`, availablePresetNames())
}

func usageUpgrade() {
	fmt.Fprint(os.Stderr, `Upgrade AWK kit files in a project

This command updates scripts, templates, commands, rules, and docs
while preserving your workflow.yaml configuration by default.

CI workflow is automatically migrated (removes deprecated awk job).

Usage:
  awkit upgrade [project_path] [options]

Arguments:
  project_path    Path to the project (default: current directory)

Options:
  --scaffold      Supplement scaffold files for a preset (requires --preset)
  --preset        Preset to use for scaffold (required with --scaffold)
  --force-config  Overwrite .ai/config/workflow.yaml using the preset (requires --preset)
  --force         Overwrite scaffold files (only affects scaffold, not kit files)
  --dry-run       Show what would be updated without making changes
  --no-generate   Skip running generate.sh after upgrade
  --no-commit     Skip auto-commit of upgrade changes

Examples:
  awkit upgrade
  awkit upgrade /path/to/project
  awkit upgrade --dry-run
  awkit upgrade --scaffold --preset go
  awkit upgrade --scaffold --preset react-go --force
  awkit upgrade --no-commit  # Manual commit control
`)
}

func usageUninstall() {
	fmt.Fprint(os.Stderr, `Remove AWK from a project

Usage:
  awkit uninstall [project_path] [options]

Arguments:
  project_path    Path to the project (default: current directory)

Options:
  --dry-run       Show what would be removed without making changes
  --keep-config   Keep .ai/config/workflow.yaml

Examples:
  awkit uninstall
  awkit uninstall /path/to/project
  awkit uninstall --dry-run
`)
}

func usageCheckUpdate() {
	fmt.Fprint(os.Stderr, `Check for awkit CLI updates

Usage:
  awkit check-update [options]

Options:
  --repo      GitHub repo to check (owner/name)
  --quiet     Only print if update is available
  --json      Output as JSON
  --no-cache  Skip cache and fetch latest

Examples:
  awkit check-update
  awkit check-update --json
`)
}

func usageCompletion() {
	fmt.Fprint(os.Stderr, `Generate shell completion script

Usage:
  awkit completion <shell>

Supported shells:
  bash    Bash completion
  zsh     Zsh completion
  fish    Fish completion

Examples:
  # Bash
  awkit completion bash > /etc/bash_completion.d/awkit
  
  # Zsh
  awkit completion zsh > "${fpath[1]}/_awkit"
  
  # Fish
  awkit completion fish > ~/.config/fish/completions/awkit.fish
`)
}

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageInit

	preset := fs.String("preset", "generic", "")
	scaffold := fs.Bool("scaffold", false, "")
	noGenerate := fs.Bool("no-generate", false, "")
	force := fs.Bool("force", false, "")
	forceConfig := fs.Bool("force-config", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	projectName := fs.String("project-name", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	// Reorder args to put flags before positional arguments
	// This is needed because flag.Parse stops at the first non-flag argument
	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// Check if this flag takes a value
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && strings.Contains(arg, "=") == false {
				// Flags that take values: --preset, --project-name
				if arg == "--preset" || arg == "--project-name" {
					i++
					flags = append(flags, args[i])
				}
			}
		} else {
			positional = append(positional, arg)
		}
	}
	reorderedArgs := append(flags, positional...)

	if err := fs.Parse(reorderedArgs); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageInit()
		return 0
	}

	// Default to current directory if no path provided
	targetDir := "."
	if fs.NArg() >= 1 {
		targetDir = fs.Arg(0)
	}

	// Validate preset
	validPreset := false
	for _, p := range presets {
		if p.Name == *preset {
			validPreset = true
			break
		}
	}
	if !validPreset {
		errorf("Unknown preset %q\n", *preset)
		fmt.Fprintf(os.Stderr, "Available presets: %s\n", availablePresetNames())
		fmt.Fprintf(os.Stderr, "\nRun 'awkit list-presets' for details.\n")
		return 2
	}

	// Resolve target for display
	displayTarget := targetDir
	if targetDir == "." || targetDir == "./" {
		if cwd, err := os.Getwd(); err == nil {
			displayTarget = cwd
		}
	}

	fmt.Println("")
	if *dryRun {
		fmt.Printf("%s[DRY RUN]%s Would initialize AWK:\n", colorYellow, colorReset)
	} else {
		fmt.Println("Initializing AWK...")
	}
	fmt.Printf("  Target:  %s\n", cyan(displayTarget))
	fmt.Printf("  Preset:  %s\n", cyan(*preset))
	if *scaffold && *preset == "generic" {
		fmt.Printf("  Using default preset: %s\n", cyan("generic (node)"))
	}
	if *force {
		fmt.Printf("  Mode:    %s\n", cyan("force (overwrite all)"))
	} else if *forceConfig {
		fmt.Printf("  Mode:    %s\n", cyan("force-config (overwrite config only)"))
	}
	if *scaffold {
		fmt.Printf("  Scaffold: %s\n", cyan("yes"))
	}
	fmt.Println("")

	if *dryRun {
		fmt.Println(bold("AWK Kit files:"))
		fmt.Println("  .ai/config/workflow.yaml")
		fmt.Println("  .ai/scripts/")
		fmt.Println("  .ai/scripts/lib/")
		fmt.Println("  .ai/templates/")
		fmt.Println("  .ai/rules/")
		fmt.Println("  .ai/commands/")
		fmt.Println("  .ai/docs/")
		fmt.Println("  .ai/tests/")
		fmt.Println("  .ai/specs/")
		fmt.Println("  .ai/state/ (runtime, gitignored)")
		fmt.Println("  .ai/results/ (runtime, gitignored)")
		fmt.Println("  .ai/runs/ (runtime, gitignored)")
		fmt.Println("  .ai/exe-logs/ (runtime, gitignored)")
		fmt.Println("  .worktrees/ (gitignored)")
		fmt.Println("  .claude/rules -> .ai/rules (symlink)")
		fmt.Println("  .claude/commands -> .ai/commands (symlink)")
		fmt.Println("  .github/workflows/ci.yml (if missing)")
		fmt.Println("  .gitignore (append AWK entries)")
		fmt.Println("  .gitattributes (if missing)")

		// Scaffold dry-run
		if *scaffold {
			resolvedProjectName := *projectName
			if resolvedProjectName == "" {
				resolvedProjectName = displayTarget
				if resolvedProjectName == "." || resolvedProjectName == "./" {
					if cwd, err := os.Getwd(); err == nil {
						resolvedProjectName = cwd
					}
				}
				resolvedProjectName = filepath.Base(resolvedProjectName)
			}

			scaffoldResult, _ := install.Scaffold(displayTarget, install.ScaffoldOptions{
				Preset:      *preset,
				TargetDir:   displayTarget,
				ProjectName: resolvedProjectName,
				Force:       *force,
				DryRun:      true,
			})

			fmt.Println("")
			fmt.Println(bold("Scaffold files:"))
			for _, f := range scaffoldResult.Created {
				fmt.Printf("  %s\n", f)
			}
			if len(scaffoldResult.Skipped) > 0 {
				fmt.Println("")
				fmt.Println(bold("Skipped (exists):"))
				for _, f := range scaffoldResult.Skipped {
					fmt.Printf("  %s\n", f)
				}
				fmt.Println("")
				warn("%d files would be skipped. Use --force to overwrite.\n", len(scaffoldResult.Skipped))
			}
		}

		fmt.Println("")
		success("Dry run complete. No changes made.\n")
		return 0
	}

	result, err := install.Install(awkit.KitFS, targetDir, install.Options{
		Preset:      *preset,
		ProjectName: *projectName,
		Force:       *force,
		ForceConfig: *forceConfig,
		ForceCI:     *force, // --force also overwrites CI
		NoGenerate:  *noGenerate,
		WithCI:      true, // CI is always created by default
		Scaffold:    *scaffold,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "")
		errorf("%v\n", err)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Troubleshooting:")
		fmt.Fprintln(os.Stderr, "  - Ensure the target directory exists")
		fmt.Fprintln(os.Stderr, "  - Use --force to overwrite existing files")
		fmt.Fprintln(os.Stderr, "  - Check write permissions")
		return 1
	}

	fmt.Println("")
	success("AWK initialized successfully!\n")

	// Warn if config was skipped
	if result != nil && result.ConfigSkipped {
		fmt.Println("")
		warn("Skipped: .ai/config/workflow.yaml (already exists)\n")
		fmt.Printf("  Preset '%s' was NOT applied to config.\n", *preset)
		fmt.Println("")
		fmt.Println("  To apply preset config, use one of:")
		fmt.Printf("    awkit init --preset %s --force        # Overwrite all files\n", *preset)
		fmt.Printf("    awkit init --preset %s --force-config # Overwrite config only\n", *preset)
	}

	// Show scaffold results
	if *scaffold && result != nil && result.ScaffoldResult != nil {
		sr := result.ScaffoldResult
		if len(sr.Created) > 0 {
			fmt.Println("")
			fmt.Println(bold("Scaffold files created:"))
			for _, f := range sr.Created {
				fmt.Printf("  %s\n", f)
			}
		}
		if len(sr.Skipped) > 0 {
			fmt.Println("")
			warn("Scaffold files skipped (already exist):\n")
			for _, f := range sr.Skipped {
				fmt.Printf("  %s\n", f)
			}
		}
		if len(sr.Failed) > 0 {
			fmt.Println("")
			errorf("Scaffold files failed:\n")
			for i, f := range sr.Failed {
				fmt.Printf("  %s: %v\n", f, sr.Errors[i])
			}
			fmt.Println("")
			warn("Some files failed to create. Run 'awkit init --scaffold' again to retry.\n")
		}
	}

	// Handle scaffold error
	if result != nil && result.ScaffoldError != nil {
		fmt.Println("")
		warn("Scaffold completed with errors: %v\n", result.ScaffoldError)
	}

	fmt.Println("")
	fmt.Println(bold("Next steps:"))
	fmt.Printf("  1. %s\n", cyan("cd "+displayTarget))
	fmt.Printf("  2. %s\n", cyan("Edit .ai/config/workflow.yaml"))
	fmt.Printf("  3. %s\n", cyan("awkit generate"))
	fmt.Println("")

	warnUpdateAvailable()
	return 0
}
func cmdUpgrade(args []string) int {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageUpgrade

	scaffold := fs.Bool("scaffold", false, "")
	preset := fs.String("preset", "", "")
	forceConfig := fs.Bool("force-config", false, "")
	force := fs.Bool("force", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	noGenerate := fs.Bool("no-generate", false, "")
	noCommit := fs.Bool("no-commit", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	// Reorder args to put flags before positional arguments
	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// Check if this flag takes a value
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && strings.Contains(arg, "=") == false {
				if arg == "--preset" {
					i++
					flags = append(flags, args[i])
				}
			}
		} else {
			positional = append(positional, arg)
		}
	}
	reorderedArgs := append(flags, positional...)

	if err := fs.Parse(reorderedArgs); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageUpgrade()
		return 0
	}

	// Validate: --scaffold requires --preset
	if *scaffold && *preset == "" {
		errorf("--preset required for upgrade --scaffold\n")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: awkit upgrade --scaffold --preset <name>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Run 'awkit list-presets' to see available presets.")
		return 2
	}

	// Validate: --force-config requires --preset
	if *forceConfig && *preset == "" {
		errorf("--preset required for upgrade --force-config\n")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage: awkit upgrade --force-config --preset <name>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Run 'awkit list-presets' to see available presets.")
		return 2
	}

	// Validate preset if provided
	if *preset != "" {
		validPreset := false
		for _, p := range presets {
			if p.Name == *preset {
				validPreset = true
				break
			}
		}
		if !validPreset {
			errorf("Unknown preset %q\n", *preset)
			fmt.Fprintf(os.Stderr, "Available presets: %s\n", availablePresetNames())
			fmt.Fprintf(os.Stderr, "\nRun 'awkit list-presets' for details.\n")
			return 2
		}
	}

	// Default to current directory
	targetDir := "."
	if fs.NArg() >= 1 {
		targetDir = fs.Arg(0)
	}

	// Resolve target for display
	displayTarget := targetDir
	if targetDir == "." || targetDir == "./" {
		if cwd, err := os.Getwd(); err == nil {
			displayTarget = cwd
		}
	}

	// Check if AWK is installed
	aiDir := targetDir + "/.ai"
	if _, err := os.Stat(aiDir); os.IsNotExist(err) {
		errorf("AWK is not installed in %s\n", displayTarget)
		fmt.Fprintln(os.Stderr, "Run 'awkit init' first to initialize AWK.")
		return 1
	}

	fmt.Println("")
	if *dryRun {
		fmt.Printf("%s[DRY RUN]%s Would upgrade AWK:\n", colorYellow, colorReset)
	} else {
		fmt.Println("Upgrading AWK...")
	}
	fmt.Printf("  Target: %s\n", cyan(displayTarget))
	if *scaffold {
		fmt.Printf("  Scaffold: %s (preset: %s)\n", cyan("yes"), cyan(*preset))
	}
	fmt.Println("")

	if *dryRun {
		fmt.Println("Would update:")
		fmt.Println("  .ai/scripts/")
		fmt.Println("  .ai/scripts/lib/")
		fmt.Println("  .ai/templates/")
		fmt.Println("  .ai/rules/_kit/")
		fmt.Println("  .ai/rules/_examples/")
		fmt.Println("  .ai/commands/")
		fmt.Println("  .ai/docs/")
		fmt.Println("  .ai/tests/")
		fmt.Println("  .github/workflows/ci.yml (migrate deprecated awk job)")
		fmt.Println("")
		if *forceConfig {
			fmt.Println("Would overwrite:")
			fmt.Printf("  .ai/config/workflow.yaml (preset: %s)\n", *preset)
		} else {
			fmt.Println("Would preserve:")
			fmt.Println("  .ai/config/workflow.yaml")
		}
		fmt.Println("  .ai/specs/")
		fmt.Println("  .ai/rules/ (user rules)")

		// Scaffold dry-run
		if *scaffold {
			projectName := filepath.Base(displayTarget)
			scaffoldResult, _ := install.Scaffold(displayTarget, install.ScaffoldOptions{
				Preset:      *preset,
				TargetDir:   displayTarget,
				ProjectName: projectName,
				Force:       *force,
				DryRun:      true,
			})

			fmt.Println("")
			fmt.Println(bold("Scaffold files:"))
			for _, f := range scaffoldResult.Created {
				fmt.Printf("  %s\n", f)
			}
			if len(scaffoldResult.Skipped) > 0 {
				fmt.Println("")
				fmt.Println(bold("Skipped (exists):"))
				for _, f := range scaffoldResult.Skipped {
					fmt.Printf("  %s\n", f)
				}
				fmt.Println("")
				warn("%d files would be skipped. Use --force to overwrite.\n", len(scaffoldResult.Skipped))
			}
		}

		// Check permissions (dry-run)
		permResult := upgrade.UpgradePermissions(targetDir, true)
		if !permResult.Skipped {
			fmt.Println("")
			fmt.Println(bold("Permissions:"))
			fmt.Printf("  %s\n", permResult.Message)
		}

		// Check agents (dry-run)
		agentsResult := upgrade.UpgradeAgents(targetDir, true)
		if !agentsResult.Skipped {
			fmt.Println("")
			fmt.Println(bold("Agents:"))
			fmt.Printf("  %s\n", agentsResult.Message)
		}

		fmt.Println("")
		success("Dry run complete. No changes made.\n")
		return 0
	}

	// Upgrade: force overwrite kit files. By default we skip workflow.yaml unless --force-config is provided.
	result, err := install.Install(awkit.KitFS, targetDir, install.Options{
		Preset:      *preset,
		Force:       true, // Overwrite kit files
		ForceConfig: *forceConfig,
		SkipConfig:  !*forceConfig, // Preserve workflow.yaml by default
		NoGenerate:  *noGenerate,
		WithCI:      true,  // Always migrate CI
		ForceCI:     false, // Never force-replace CI on upgrade (only migrate)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "")
		errorf("%v\n", err)
		return 1
	}

	fmt.Println("")
	success("AWK upgraded successfully!\n")

	if result != nil && result.ConfigSkipped {
		fmt.Println("")
		info("  Config preserved: .ai/config/workflow.yaml\n")
	} else if *forceConfig {
		fmt.Println("")
		info("  Config overwritten: .ai/config/workflow.yaml\n")
	}

	// Upgrade permissions in settings.local.json
	permResult := upgrade.UpgradePermissions(targetDir, *dryRun)
	if !permResult.Skipped {
		fmt.Println("")
		if permResult.Success {
			success("Permissions upgraded: %s\n", permResult.Message)
		} else {
			warn("Permissions upgrade: %s\n", permResult.Message)
		}
	}

	// Upgrade agents in .claude/agents/
	agentsResult := upgrade.UpgradeAgents(targetDir, *dryRun)
	if !agentsResult.Skipped {
		fmt.Println("")
		if agentsResult.Success {
			success("Agents installed: %s\n", agentsResult.Message)
		} else {
			warn("Agents install: %s\n", agentsResult.Message)
		}
	}

	// Auto-commit upgrade changes (unless --no-commit)
	if !*noCommit && !*dryRun {
		if err := autoCommitUpgrade(targetDir); err != nil {
			fmt.Println("")
			warn("Failed to auto-commit changes: %v\n", err)
			fmt.Println("")
			fmt.Println("Please commit manually:")
			fmt.Printf("  %s\n", cyan("git add .ai/ .claude/ CLAUDE.md AGENTS.md"))
			fmt.Printf("  %s\n", cyan(`git commit -m "[chore] upgrade awkit"`))
		} else {
			fmt.Println("")
			success("Changes committed automatically\n")
			fmt.Printf("  Commit message: %s\n", cyan("[chore] upgrade awkit"))
		}
	}

	// Handle scaffold for upgrade
	if *scaffold {
		projectName := filepath.Base(displayTarget)
		scaffoldResult, scaffoldErr := install.Scaffold(displayTarget, install.ScaffoldOptions{
			Preset:      *preset,
			TargetDir:   displayTarget,
			ProjectName: projectName,
			Force:       *force,
			DryRun:      false,
		})

		if scaffoldResult != nil {
			if len(scaffoldResult.Created) > 0 {
				fmt.Println("")
				fmt.Println(bold("Scaffold files created:"))
				for _, f := range scaffoldResult.Created {
					fmt.Printf("  %s\n", f)
				}
			}
			if len(scaffoldResult.Skipped) > 0 {
				fmt.Println("")
				warn("Scaffold files skipped (already exist):\n")
				for _, f := range scaffoldResult.Skipped {
					fmt.Printf("  %s\n", f)
				}
			}
			if len(scaffoldResult.Failed) > 0 {
				fmt.Println("")
				errorf("Scaffold files failed:\n")
				for i, f := range scaffoldResult.Failed {
					fmt.Printf("  %s: %v\n", f, scaffoldResult.Errors[i])
				}
			}
		}

		if scaffoldErr != nil {
			fmt.Println("")
			warn("Scaffold completed with errors: %v\n", scaffoldErr)
		}
	}

	fmt.Println("")
	fmt.Println(bold("Next steps:"))
	fmt.Printf("  %s\n", cyan("awkit kickoff"))
	fmt.Println("")

	warnUpdateAvailable()
	return 0
}

// autoCommitUpgrade attempts to automatically commit upgrade changes
func autoCommitUpgrade(targetDir string) error {
	// Check if git is available
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git not found in PATH")
	}

	// Check if we're in a git repository
	checkCmd := exec.Command(gitPath, "rev-parse", "--git-dir")
	checkCmd.Dir = targetDir
	if err := checkCmd.Run(); err != nil {
		return fmt.Errorf("not a git repository")
	}

	// Add awkit-related files
	addCmd := exec.Command(gitPath, "add", ".ai/", ".claude/", "CLAUDE.md", "AGENTS.md")
	addCmd.Dir = targetDir
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Check if there are changes to commit
	statusCmd := exec.Command(gitPath, "diff", "--cached", "--quiet")
	statusCmd.Dir = targetDir
	if err := statusCmd.Run(); err == nil {
		// No changes to commit
		return nil
	}

	// Commit the changes
	commitCmd := exec.Command(gitPath, "commit", "-m", "[chore] upgrade awkit")
	commitCmd.Dir = targetDir
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	return nil
}

func cmdUninstall(args []string) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageUninstall

	dryRun := fs.Bool("dry-run", false, "")
	keepConfig := fs.Bool("keep-config", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageUninstall()
		return 0
	}

	// Default to current directory
	targetDir := "."
	if fs.NArg() >= 1 {
		targetDir = fs.Arg(0)
	}

	// Resolve path
	if targetDir == "." || targetDir == "./" {
		if cwd, err := os.Getwd(); err == nil {
			targetDir = cwd
		}
	}

	aiDir := targetDir + "/.ai"
	claudeDir := targetDir + "/.claude"
	worktreesDir := targetDir + "/.worktrees"

	// Check if AWK is installed
	if _, err := os.Stat(aiDir); os.IsNotExist(err) {
		errorf("AWK is not installed in %s\n", targetDir)
		return 1
	}

	fmt.Println("")
	if *dryRun {
		fmt.Printf("%s[DRY RUN]%s Would remove:\n", colorYellow, colorReset)
	} else {
		fmt.Println("Removing AWK...")
	}

	toRemove := []string{}

	// List what will be removed
	if !*keepConfig {
		toRemove = append(toRemove, aiDir)
		fmt.Printf("  %s\n", aiDir)
	} else {
		// Remove everything except config
		entries := []string{"scripts", "templates", "rules", "commands", "docs", "tests", "specs", "state", "results", "runs", "exe-logs"}
		for _, e := range entries {
			path := aiDir + "/" + e
			if _, err := os.Stat(path); err == nil {
				toRemove = append(toRemove, path)
				fmt.Printf("  %s\n", path)
			}
		}
		fmt.Printf("  %s (keeping .ai/config/)\n", cyan("partial"))
	}

	if _, err := os.Stat(claudeDir); err == nil {
		toRemove = append(toRemove, claudeDir)
		fmt.Printf("  %s\n", claudeDir)
	}

	if _, err := os.Stat(worktreesDir); err == nil {
		toRemove = append(toRemove, worktreesDir)
		fmt.Printf("  %s\n", worktreesDir)
	}

	fmt.Println("")

	if *dryRun {
		success("Dry run complete. No changes made.\n")
		return 0
	}

	// Actually remove
	for _, path := range toRemove {
		if err := os.RemoveAll(path); err != nil {
			errorf("Failed to remove %s: %v\n", path, err)
		}
	}

	success("AWK removed from %s\n", targetDir)
	return 0
}

func cmdListPresets() int {
	fmt.Println("")
	fmt.Println(bold("Available presets:"))
	fmt.Println("")

	fmt.Println(bold("Single-Repo:"))
	for _, p := range presets {
		if p.Category == "single-repo" {
			fmt.Printf("  %-12s %s\n", p.Name, p.Description)
		}
	}
	fmt.Println("")

	fmt.Println(bold("Monorepo:"))
	for _, p := range presets {
		if p.Category == "monorepo" {
			fmt.Printf("  %-12s %s\n", p.Name, p.Description)
		}
	}
	fmt.Println("")
	fmt.Println("Usage: awkit init --preset <name> [--scaffold]")
	fmt.Println("")
	return 0
}

func cmdCompletion(args []string) int {
	if len(args) < 1 {
		usageCompletion()
		return 2
	}

	shell := args[0]
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		errorf("Unsupported shell: %s\n", shell)
		fmt.Fprintln(os.Stderr, "Supported: bash, zsh, fish")
		return 2
	}
	return 0
}

const bashCompletion = `# awkit bash completion
_awkit() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    commands="init install upgrade uninstall kickoff validate status next check-result dispatch-worker run-issue list-presets check-update completion version help"
    
    case "${prev}" in
        awkit)
            COMPREPLY=( $(compgen -W "${commands}" -- ${cur}) )
            return 0
            ;;
        init|install)
            local opts="--preset --scaffold --force --force-config --dry-run --no-generate --project-name --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) $(compgen -d -- ${cur}) )
            return 0
            ;;
        upgrade)
            local opts="--scaffold --preset --force-config --force --dry-run --no-generate --no-commit --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) $(compgen -d -- ${cur}) )
            return 0
            ;;
        uninstall)
            local opts="--dry-run --keep-config --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) $(compgen -d -- ${cur}) )
            return 0
            ;;
        --preset)
            COMPREPLY=( $(compgen -W "generic go python rust dotnet node react-go react-python unity-go godot-go unreal-go" -- ${cur}) )
            return 0
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- ${cur}) )
            return 0
            ;;
        help)
            COMPREPLY=( $(compgen -W "${commands}" -- ${cur}) )
            return 0
            ;;
    esac
}
complete -F _awkit awkit
`

const zshCompletion = `#compdef awkit

_awkit() {
    local -a commands
    commands=(
        'init:Initialize AWK in a project'
        'install:Alias for init'
        'upgrade:Upgrade AWK kit files (preserves config by default)'
        'uninstall:Remove AWK from a project'
        'kickoff:Start the AI workflow'
        'validate:Validate workflow configuration'
        'status:Show offline workflow status'
        'next:Show suggested next actions (offline)'
        'check-result:Check worker execution result for an issue'
        'dispatch-worker:Dispatch an issue to a worker'
        'run-issue:Run a worker for a single issue'
        'list-presets:Show available presets'
        'check-update:Check for CLI updates'
        'completion:Generate shell completion'
        'version:Show version'
        'help:Show help'
    )

    local -a presets
    presets=(generic go python rust dotnet node react-go react-python unity-go godot-go unreal-go)

    _arguments -C \
        '1: :->command' \
        '*: :->args'

    case $state in
        command)
            _describe 'command' commands
            ;;
        args)
            case $words[2] in
                init|install)
                    _arguments \
                        '--preset[Project preset]:preset:(generic go python rust dotnet node react-go react-python unity-go godot-go unreal-go)' \
                        '--scaffold[Create project structure]' \
                        '--force[Overwrite all existing files]' \
                        '--force-config[Overwrite only workflow.yaml]' \
                        '--dry-run[Show what would be done]' \
                        '--no-generate[Skip generate.sh]' \
                        '--project-name[Override project name]:name:' \
                        '*:directory:_files -/'
                    ;;
                upgrade)
                    _arguments \
                        '--scaffold[Supplement scaffold files]' \
                        '--preset[Preset for scaffold]:preset:(generic go python rust dotnet node react-go react-python unity-go godot-go unreal-go)' \
                        '--force-config[Overwrite only workflow.yaml]' \
                        '--force[Overwrite scaffold files]' \
                        '--dry-run[Show what would be done]' \
                        '--no-generate[Skip generate.sh]' \
                        '--no-commit[Skip auto-commit]' \
                        '*:directory:_files -/'
                    ;;
                uninstall)
                    _arguments \
                        '--dry-run[Show what would be removed]' \
                        '--keep-config[Keep workflow.yaml]' \
                        '*:directory:_files -/'
                    ;;
                completion)
                    _arguments '1:shell:(bash zsh fish)'
                    ;;
                help)
                    _describe 'command' commands
                    ;;
            esac
            ;;
    esac
}

_awkit "$@"
`

const fishCompletion = `# awkit fish completion
complete -c awkit -e

# Commands
complete -c awkit -n __fish_use_subcommand -a init -d 'Initialize AWK in a project'
complete -c awkit -n __fish_use_subcommand -a install -d 'Alias for init'
complete -c awkit -n __fish_use_subcommand -a upgrade -d 'Upgrade AWK kit files (preserves config by default)'
complete -c awkit -n __fish_use_subcommand -a uninstall -d 'Remove AWK from a project'
complete -c awkit -n __fish_use_subcommand -a kickoff -d 'Start the AI workflow'
complete -c awkit -n __fish_use_subcommand -a validate -d 'Validate workflow configuration'
complete -c awkit -n __fish_use_subcommand -a status -d 'Show offline workflow status'
complete -c awkit -n __fish_use_subcommand -a next -d 'Show suggested next actions (offline)'
complete -c awkit -n __fish_use_subcommand -a check-result -d 'Check worker execution result for an issue'
complete -c awkit -n __fish_use_subcommand -a dispatch-worker -d 'Dispatch an issue to a worker'
complete -c awkit -n __fish_use_subcommand -a run-issue -d 'Run a worker for a single issue'
complete -c awkit -n __fish_use_subcommand -a list-presets -d 'Show available presets'
complete -c awkit -n __fish_use_subcommand -a check-update -d 'Check for CLI updates'
complete -c awkit -n __fish_use_subcommand -a completion -d 'Generate shell completion'
complete -c awkit -n __fish_use_subcommand -a version -d 'Show version'
complete -c awkit -n __fish_use_subcommand -a help -d 'Show help'

# init/install options
complete -c awkit -n '__fish_seen_subcommand_from init install' -l preset -d 'Project preset' -xa 'generic go python rust dotnet node react-go react-python unity-go godot-go unreal-go'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l scaffold -d 'Create project structure'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l force -d 'Overwrite all existing files'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l force-config -d 'Overwrite only workflow.yaml'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l dry-run -d 'Show what would be done'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l no-generate -d 'Skip generate.sh'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l project-name -d 'Override project name'

# upgrade options
complete -c awkit -n '__fish_seen_subcommand_from upgrade' -l scaffold -d 'Supplement scaffold files'
complete -c awkit -n '__fish_seen_subcommand_from upgrade' -l preset -d 'Preset for scaffold' -xa 'generic go python rust dotnet node react-go react-python unity-go godot-go unreal-go'
complete -c awkit -n '__fish_seen_subcommand_from upgrade' -l force-config -d 'Overwrite only workflow.yaml'
complete -c awkit -n '__fish_seen_subcommand_from upgrade' -l force -d 'Overwrite scaffold files'
complete -c awkit -n '__fish_seen_subcommand_from upgrade' -l dry-run -d 'Show what would be done'
complete -c awkit -n '__fish_seen_subcommand_from upgrade' -l no-generate -d 'Skip generate.sh'
complete -c awkit -n '__fish_seen_subcommand_from upgrade' -l no-commit -d 'Skip auto-commit'

# uninstall options
complete -c awkit -n '__fish_seen_subcommand_from uninstall' -l dry-run -d 'Show what would be removed'
complete -c awkit -n '__fish_seen_subcommand_from uninstall' -l keep-config -d 'Keep workflow.yaml'

# completion shells
complete -c awkit -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`

func cmdCheckUpdate(args []string) int {
	fs := flag.NewFlagSet("check-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageCheckUpdate

	repo := fs.String("repo", "", "")
	quiet := fs.Bool("quiet", false, "")
	jsonOut := fs.Bool("json", false, "")
	noCache := fs.Bool("no-cache", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageCheckUpdate()
		return 0
	}

	repoValue := resolveRepo(*repo)
	result := updatecheck.Check(buildinfo.Version, updatecheck.Options{
		Repo:     repoValue,
		NoCache:  *noCache,
		Timeout:  5 * time.Second,
		CacheTTL: 24 * time.Hour,
	})
	return renderUpdateResult(result, repoValue, *quiet, *jsonOut)
}

func renderUpdateResult(result updatecheck.Result, repo string, quiet bool, jsonOut bool) int {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			errorf("%v\n", err)
			return 1
		}
		if result.Error != "" {
			return 1
		}
		return 0
	}

	if result.Error != "" {
		if !quiet {
			warn("Update check skipped: %s\n", result.Error)
		}
		return 1
	}

	if result.CurrentUnknown {
		if !quiet {
			if result.Latest != "" {
				fmt.Printf("Current version unknown (%s). Latest: %s\n", result.Current, result.Latest)
			} else {
				fmt.Printf("Current version unknown (%s).\n", result.Current)
			}
		}
		return 0
	}

	if result.UpdateAvailable {
		fmt.Printf("%sUpdate available:%s %s -> %s\n", colorYellow, colorReset, result.Current, result.Latest)
		fmt.Println("Run:", updateCommand(repo))
		return 0
	}

	if !quiet {
		success("Up to date: %s\n", result.Current)
	}
	return 0
}

func warnUpdateAvailable() {
	if skipUpdateCheck() {
		return
	}
	repo := resolveRepo("")
	result := updatecheck.Check(buildinfo.Version, updatecheck.Options{
		Repo:     repo,
		Timeout:  3 * time.Second,
		CacheTTL: 24 * time.Hour,
	})
	if result.Error != "" || result.CurrentUnknown {
		return
	}
	if result.UpdateAvailable {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorYellow, colorReset)
		fmt.Fprintf(os.Stderr, "%s  Update available: %s -> %s%s\n", colorYellow, result.Current, result.Latest, colorReset)
		fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorYellow, colorReset)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Run:", updateCommand(repo))
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "  Release notes: https://github.com/%s/releases/latest\n", repo)
		fmt.Fprintln(os.Stderr, "")
	}
}

func resolveRepo(flagRepo string) string {
	if flagRepo != "" {
		return flagRepo
	}
	if envRepo := strings.TrimSpace(os.Getenv("AWKIT_REPO")); envRepo != "" {
		return envRepo
	}
	return updatecheck.DefaultRepo
}

func skipUpdateCheck() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("AWKIT_SKIP_UPDATE_CHECK")))
	return value == "1" || value == "true" || value == "yes"
}

func updateCommand(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		repo = updatecheck.DefaultRepo
	}
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("irm https://github.com/%s/releases/latest/download/install.ps1 | iex", repo)
	}
	return fmt.Sprintf("curl -fsSL https://github.com/%s/releases/latest/download/install.sh | bash", repo)
}
