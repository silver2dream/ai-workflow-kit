package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

func usageValidate() {
	fmt.Fprint(os.Stderr, `Validate workflow configuration

Checks `+layout.ConfigFile+` and, when the project has one, the roster of
the planning council (`+layout.RosterFile+`). It also names the keys of the
configuration that nothing reads: a key an earlier awkit wrote, or one that
is misspelt, changes nothing.

Usage:
  awkit validate [options]

Options:
  --config      Path to the configuration (default: `+layout.ConfigFile+`)

Examples:
  awkit validate
  awkit validate --config /path/to/config.yaml
`)
}

func cmdValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageValidate

	configPath := fs.String("config", config.Path("."), "")
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
	cfg, err := kickoff.LoadConfig(*configPath)
	if err != nil {
		output.Error(fmt.Sprintf("Failed to load config: %v", err))
		return 1
	}

	output.Success(fmt.Sprintf("Loaded: %s", *configPath))

	// Validate required fields
	errors := cfg.Validate()
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
	baseDir := layout.ProjectOf(*configPath)
	if baseDir == "." || baseDir == "" {
		baseDir, _ = os.Getwd()
	}

	pathErrors := cfg.ValidatePaths(baseDir)
	if len(pathErrors) > 0 {
		fmt.Println("")
		output.Warning("Path warnings:")
		for _, e := range pathErrors {
			fmt.Printf("  - %s\n", e.Error())
		}
	} else {
		output.Success("Referenced paths: OK")
	}

	// The roster of the planning council, when the project has one: a roster
	// that cannot hold a meeting is found here, not ten minutes into a plan.
	rosterFile := layout.Roster(baseDir)
	if _, err := os.Stat(rosterFile); err == nil {
		roster, err := council.LoadRoster(rosterFile)
		if err != nil {
			fmt.Println("")
			output.Error(fmt.Sprintf("%s: %v", council.RosterPath, err))
			return 1
		}
		output.Success(fmt.Sprintf("Roster: %d roles (%s decides, %s plans)", len(roster.Roles), roster.Decider().ID, roster.Planner().ID))
	}

	fmt.Println("")
	fmt.Printf("Project: %s\n", output.Cyan(cfg.Project.Name))
	fmt.Printf("Type:    %s\n", output.Cyan(cfg.Project.Type))
	fmt.Printf("Repos:   %d\n", len(cfg.Repos))
	for _, repo := range cfg.Repos {
		fmt.Printf("  - %s (%s)\n", repo.Name, repo.Path)
	}

	fmt.Println("")
	output.Success("Configuration is valid")

	if unread, err := config.UnreadKeys(*configPath); err == nil {
		printUnreadKeys(os.Stdout, unread)
	}

	return 0
}

// printUnreadKeys names the keys of the configuration that nothing reads.
// The file is valid all the same; what the list is for is the person who set
// one of them and takes it to be in force.
func printUnreadKeys(w io.Writer, unread []config.UnreadKey) {
	if len(unread) == 0 {
		return
	}
	fmt.Fprintln(w)
	if len(unread) == 1 {
		fmt.Fprintln(w, "1 key of the configuration is read by nothing, and changes nothing:")
	} else {
		fmt.Fprintf(w, "%d keys of the configuration are read by nothing, and change nothing:\n", len(unread))
	}
	for _, key := range unread {
		fmt.Fprintf(w, "  - %s\n", key)
	}
	fmt.Fprintln(w, "An earlier awkit wrote some of these, and a key that is misspelt is listed too.")
	fmt.Fprintln(w, "Delete them, or correct the spelling.")
}
