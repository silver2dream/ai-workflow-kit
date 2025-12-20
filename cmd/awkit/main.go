package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	awkit "github.com/silver2dream/ai-workflow-kit"
	"github.com/silver2dream/ai-workflow-kit/internal/buildinfo"
	"github.com/silver2dream/ai-workflow-kit/internal/install"
	"github.com/silver2dream/ai-workflow-kit/internal/updatecheck"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		usage()
		return 2
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(buildinfo.Version)
		return 0
	case "check-update":
		return cmdCheckUpdate(os.Args[2:])
	case "install":
		return cmdInstall(os.Args[2:])
	default:
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `awkit - AI Workflow Kit installer

Usage:
  awkit install <project_path> [--preset react-go] [--no-generate] [--with-ci] [--force]
  awkit check-update [--repo owner/name] [--quiet] [--json] [--no-cache]
  awkit version
`)
}

func cmdInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	preset := fs.String("preset", "generic", "preset to install (react-go|generic)")
	noGenerate := fs.Bool("no-generate", false, "do not run .ai/scripts/generate.sh after install")
	withCI := fs.Bool("with-ci", true, "create .github/workflows/ci.yml if missing")
	force := fs.Bool("force", false, "overwrite existing kit files")
	projectName := fs.String("project-name", "", "override project name in generated config")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "ERROR: project_path is required")
		return 2
	}

	targetDir := fs.Arg(0)
	if err := install.Install(awkit.KitFS, targetDir, install.Options{
		Preset:      *preset,
		ProjectName: *projectName,
		Force:       *force,
		NoGenerate:  *noGenerate,
		WithCI:      *withCI,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		return 1
	}

	fmt.Println("OK: installed AWK kit into", targetDir)
	warnUpdateAvailable()
	return 0
}

func cmdCheckUpdate(args []string) int {
	fs := flag.NewFlagSet("check-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	repo := fs.String("repo", "", "GitHub repo (owner/name)")
	quiet := fs.Bool("quiet", false, "only print when update is available")
	jsonOut := fs.Bool("json", false, "output JSON")
	noCache := fs.Bool("no-cache", false, "skip cache and fetch latest")
	if err := fs.Parse(args); err != nil {
		return 2
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
			fmt.Fprintln(os.Stderr, "ERROR:", err)
			return 1
		}
		if result.Error != "" {
			return 1
		}
		return 0
	}

	if result.Error != "" {
		if !quiet {
			fmt.Fprintln(os.Stderr, "Update check skipped:", result.Error)
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
		fmt.Printf("Update available: %s -> %s\n", result.Current, result.Latest)
		fmt.Println("Run:", updateCommand(repo))
		return 0
	}

	if !quiet {
		fmt.Printf("Up to date: %s\n", result.Current)
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
		fmt.Fprintf(os.Stderr, "Update available: %s -> %s\n", result.Current, result.Latest)
		fmt.Fprintln(os.Stderr, "Run:", updateCommand(repo))
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
	return fmt.Sprintf("curl -fsSL https://github.com/%s/releases/latest/download/install.sh | bash", repo)
}
