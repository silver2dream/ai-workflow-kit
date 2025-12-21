package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	awkit "github.com/silver2dream/ai-workflow-kit"
	"github.com/silver2dream/ai-workflow-kit/internal/buildinfo"
	"github.com/silver2dream/ai-workflow-kit/internal/install"
	"github.com/silver2dream/ai-workflow-kit/internal/updatecheck"
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
	{Name: "generic", Description: "Generic single-repo project"},
	{Name: "react-go", Description: "React frontend + Go backend monorepo"},
}

type PresetInfo struct {
	Name        string
	Description string
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
	case "list-presets":
		return cmdListPresets()
	case "completion":
		return cmdCompletion(os.Args[2:])
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
  uninstall     Remove AWK from a project
  list-presets  Show available project presets
  check-update  Check for CLI updates
  completion    Generate shell completion script
  version       Show version
  help          Show help for a command

Examples:
  awkit init
  awkit init --preset react-go
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
	case "uninstall":
		usageUninstall()
	case "list-presets":
		fmt.Println("Show available project presets with descriptions.")
		fmt.Println("\nUsage: awkit list-presets")
	case "check-update":
		usageCheckUpdate()
	case "completion":
		usageCompletion()
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
  --force         Overwrite all existing kit files
  --force-config  Overwrite only workflow.yaml (apply preset to existing project)
  --dry-run       Show what would be done without making changes
  --no-generate   Skip running generate.sh after init
  --with-ci       Create CI workflow if missing [default: true]
  --project-name  Override project name in config

Examples:
  awkit init
  awkit init --preset react-go
  awkit init /path/to/project --force
  awkit init --preset react-go --force-config  # Apply preset to existing project
  awkit init --dry-run

Run 'awkit list-presets' to see available presets.
`, availablePresetNames())
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
	noGenerate := fs.Bool("no-generate", false, "")
	withCI := fs.Bool("with-ci", true, "")
	force := fs.Bool("force", false, "")
	forceConfig := fs.Bool("force-config", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	projectName := fs.String("project-name", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
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
	if *force {
		fmt.Printf("  Mode:    %s\n", cyan("force (overwrite all)"))
	} else if *forceConfig {
		fmt.Printf("  Mode:    %s\n", cyan("force-config (overwrite config only)"))
	}
	fmt.Println("")

	if *dryRun {
		fmt.Println("Would create/update:")
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
		if *withCI {
			fmt.Println("  .github/workflows/ci.yml (if missing)")
		}
		fmt.Println("  .gitignore (append AWK entries)")
		fmt.Println("  .gitattributes (if missing)")
		fmt.Println("")
		success("Dry run complete. No changes made.\n")
		return 0
	}

	result, err := install.Install(awkit.KitFS, targetDir, install.Options{
		Preset:      *preset,
		ProjectName: *projectName,
		Force:       *force,
		ForceConfig: *forceConfig,
		NoGenerate:  *noGenerate,
		WithCI:      *withCI,
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

	fmt.Println("")
	fmt.Println(bold("Next steps:"))
	fmt.Printf("  1. %s\n", cyan("cd "+displayTarget))
	fmt.Printf("  2. %s\n", cyan("Edit .ai/config/workflow.yaml"))
	fmt.Printf("  3. %s\n", cyan("bash .ai/scripts/generate.sh"))
	fmt.Println("")

	warnUpdateAvailable()
	return 0
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
	for _, p := range presets {
		fmt.Printf("  %s\n", bold(p.Name))
		fmt.Printf("    %s\n", p.Description)
		fmt.Println("")
	}
	fmt.Println("Usage: awkit init --preset <name>")
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
    
    commands="init install uninstall list-presets check-update completion version help"
    
    case "${prev}" in
        awkit)
            COMPREPLY=( $(compgen -W "${commands}" -- ${cur}) )
            return 0
            ;;
        init|install)
            local opts="--preset --force --force-config --dry-run --no-generate --with-ci --project-name --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) $(compgen -d -- ${cur}) )
            return 0
            ;;
        uninstall)
            local opts="--dry-run --keep-config --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) $(compgen -d -- ${cur}) )
            return 0
            ;;
        --preset)
            COMPREPLY=( $(compgen -W "generic react-go" -- ${cur}) )
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
        'uninstall:Remove AWK from a project'
        'list-presets:Show available presets'
        'check-update:Check for CLI updates'
        'completion:Generate shell completion'
        'version:Show version'
        'help:Show help'
    )

    local -a presets
    presets=(generic react-go)

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
                        '--preset[Project preset]:preset:(generic react-go)' \
                        '--force[Overwrite all existing files]' \
                        '--force-config[Overwrite only workflow.yaml]' \
                        '--dry-run[Show what would be done]' \
                        '--no-generate[Skip generate.sh]' \
                        '--with-ci[Create CI workflow]' \
                        '--project-name[Override project name]:name:' \
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
complete -c awkit -n __fish_use_subcommand -a uninstall -d 'Remove AWK from a project'
complete -c awkit -n __fish_use_subcommand -a list-presets -d 'Show available presets'
complete -c awkit -n __fish_use_subcommand -a check-update -d 'Check for CLI updates'
complete -c awkit -n __fish_use_subcommand -a completion -d 'Generate shell completion'
complete -c awkit -n __fish_use_subcommand -a version -d 'Show version'
complete -c awkit -n __fish_use_subcommand -a help -d 'Show help'

# init/install options
complete -c awkit -n '__fish_seen_subcommand_from init install' -l preset -d 'Project preset' -xa 'generic react-go'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l force -d 'Overwrite all existing files'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l force-config -d 'Overwrite only workflow.yaml'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l dry-run -d 'Show what would be done'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l no-generate -d 'Skip generate.sh'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l with-ci -d 'Create CI workflow'
complete -c awkit -n '__fish_seen_subcommand_from init install' -l project-name -d 'Override project name'

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
