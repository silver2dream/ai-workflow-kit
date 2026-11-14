package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	status "github.com/silver2dream/ai-workflow-kit/internal/status"
	"github.com/silver2dream/ai-workflow-kit/internal/stopmark"
)

func usageStop() {
	fmt.Fprint(os.Stderr, `Ask the run that is going to stop

The run ends when the step it is on is done: a Worker or a review that is
running finishes first, and nothing is left half done. "awkit run" goes on
from there later.

Usage:
  awkit stop

To end a run at once, interrupt it (Ctrl-C) where it was started.
`)
}

func cmdStop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageStop
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showHelp || *showHelpShort {
		usageStop()
		return 0
	}

	report, err := status.Collect(".", status.Options{})
	if err != nil {
		errorf("could not tell whether a run is going: %v\n", err)
		return 1
	}
	return stopRun(os.Stdout, ".", report.Run.State == "running")
}

// stopRun asks the run in the project at root to stop, when one is going.
// Without a run there is nothing to stop and nothing is written: a marker
// left behind would only be cleared by the next run.
func stopRun(w io.Writer, root string, running bool) int {
	if !running {
		fmt.Fprintln(w, "No run is going in this project.")
		return 0
	}
	if err := stopmark.Write(root, "awkit stop", stopmark.ByUser); err != nil {
		errorf("could not write %s: %v\n", stopmark.File, err)
		return 1
	}
	fmt.Fprintln(w, "Asked the run to stop. It ends when the step it is on is done: a Worker or a review that is running finishes first.")
	fmt.Fprintln(w, "awkit status shows where it is. awkit run goes on from there later.")
	return 0
}
