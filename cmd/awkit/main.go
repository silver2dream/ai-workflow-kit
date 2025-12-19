package main

import (
	"flag"
	"fmt"
	"os"

	awkit "github.com/silver2dream/ai-workflow-kit"
	"github.com/silver2dream/ai-workflow-kit/internal/buildinfo"
	"github.com/silver2dream/ai-workflow-kit/internal/install"
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
	return 0
}
