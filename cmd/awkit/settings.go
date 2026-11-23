package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/silver2dream/ai-workflow-kit/internal/generate"
	"github.com/silver2dream/ai-workflow-kit/internal/landing"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/settings"
	status "github.com/silver2dream/ai-workflow-kit/internal/status"
)

func usageSettings() {
	fmt.Fprint(os.Stderr, `Change the project's settings the way every change of it is made

Usage:
  awkit settings apply --request <file> [--state-root <dir>]

The request (JSON) changes the council's roster (.ai/roster.yaml), the
project's configuration (.ai/config.yaml), or both; the workbench writes it
when the settings are changed on its page. The change is made to the files as
the integration branch has them, with what awkit generate writes from the
configuration, and goes there through a pull request (branch `+settings.Branch+`),
merged once its checks pass. Then the working tree is brought up to the branch.
Nothing in the working tree changes before that.

Only what the request changes from the settings it was made to ("was_roster",
"was_config") is changed: what was changed on the integration branch meanwhile
is kept. What both changed, differently, stops the change.

Where it is goes to `+layout.StateDir+`/settings/`+settings.RecordFile+` as it goes.

Options:
  --request <file>    The request
  --state-root <dir>  The project (default: the root of the git repository)
  --help              Show this help
`)
}

// What applying settings asks of git, GitHub and the project. Replaced in
// tests.
var (
	settingsGit      landing.Runner = landing.Exec
	settingsGitHub   landing.GitHub // gh in the project when nil
	settingsLanding  = func(*landing.Options) {}
	settingsGenerate = func(root string) error {
		_, err := generate.Generate(generate.Options{StateRoot: root, CI: generate.CISync, Quiet: true})
		return err
	}
	settingsLocal    = generate.LocalSettings
	settingsRunGoing = func(root string) bool {
		report, err := status.Collect(root, status.Options{})
		return err == nil && report.Run.State == "running"
	}
)

func cmdSettings(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		usageSettings()
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	if args[0] != "apply" {
		errorf("Unknown settings command: %s\n\n", args[0])
		usageSettings()
		return 2
	}

	fs := flag.NewFlagSet("settings apply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageSettings
	requestFile := fs.String("request", "", "")
	stateRoot := fs.String("state-root", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *requestFile == "" {
		errorf("--request is required\n\n")
		usageSettings()
		return 2
	}
	root := *stateRoot
	if root == "" {
		var err error
		if root, err = resolveGitRoot(); err != nil {
			errorf("failed to resolve git root: %v\n", err)
			return 1
		}
	}
	request, err := settings.ReadRequest(*requestFile)
	if err != nil {
		errorf("%v\n", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	record := settings.Apply(ctx, settings.ApplyOptions{
		Root: root, Request: *request,
		Run: settingsGit, GitHub: settingsGitHub, Landing: settingsLanding,
		RunGoing: settingsRunGoing, Generate: settingsGenerate, Local: settingsLocal,
		Save: func(record *settings.Record) { _ = settings.WriteRecord(root, record) },
		Say:  func(line string) { fmt.Println(line) },
	})
	if record.State == settings.Failed {
		errorf("%s\n", record.Error)
		return 1
	}
	fmt.Println("")
	switch {
	case record.Unchanged:
		success("The integration branch has these settings already\n")
	case record.PR != 0:
		success("The settings are on the integration branch (pull request #%d)\n", record.PR)
	}
	if record.Left != "" {
		warn("The working tree was not brought up to the integration branch: %s. A run starts only from a clean working tree, with the settings as they are here.\n", record.Left)
	}
	return 0
}
