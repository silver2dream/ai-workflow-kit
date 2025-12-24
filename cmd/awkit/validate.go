package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
)

func usageValidate() {
	fmt.Fprint(os.Stderr, `Validate workflow configuration

Usage:
  awkit validate [options]

Options:
  --config      Path to workflow.yaml (default: .ai/config/workflow.yaml)

Examples:
  awkit validate
  awkit validate --config /path/to/workflow.yaml
`)
}

func cmdValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageValidate

	configPath := fs.String("config", filepath.Join(".ai", "config", "workflow.yaml"), "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageValidate()
		return 0
	}

	output := kickoff.NewOutputFormatter(os.Stdout)

	fmt.Println("")
	fmt.Println("Validating workflow configuration...")
	fmt.Println("")

	// Load config
	config, err := kickoff.LoadConfig(*configPath)
	if err != nil {
		output.Error(fmt.Sprintf("Failed to load config: %v", err))
		return 1
	}

	output.Success(fmt.Sprintf("Loaded: %s", *configPath))

	// Validate required fields
	errors := config.Validate()
	if len(errors) > 0 {
		fmt.Println("")
		output.Error("Validation errors:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e.Error())
		}
		return 1
	}

	output.Success("Required fields: OK")

	// Validate paths
	// baseDir should be the project root (parent of .ai directory)
	baseDir := filepath.Dir(filepath.Dir(filepath.Dir(*configPath))) // Go up from .ai/config/workflow.yaml
	if baseDir == "." || baseDir == "" {
		baseDir, _ = os.Getwd()
	}

	pathErrors := config.ValidatePaths(baseDir)
	if len(pathErrors) > 0 {
		fmt.Println("")
		output.Warning("Path warnings:")
		for _, e := range pathErrors {
			fmt.Printf("  - %s\n", e.Error())
		}
	} else {
		output.Success("Referenced paths: OK")
	}

	fmt.Println("")
	fmt.Printf("Project: %s\n", output.Cyan(config.Project.Name))
	fmt.Printf("Type:    %s\n", output.Cyan(config.Project.Type))
	fmt.Printf("Repos:   %d\n", len(config.Repos))
	for _, repo := range config.Repos {
		fmt.Printf("  - %s (%s)\n", repo.Name, repo.Path)
	}

	fmt.Println("")
	output.Success("Configuration is valid")

	return 0
}
