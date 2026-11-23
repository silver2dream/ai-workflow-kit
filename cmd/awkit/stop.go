package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/engine"
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
  awkit stop --now

Options:
  --now   End a run of the engine (AWKIT_ENGINE=engine) at once: what is
          running is ended with it, and taken up again by the next run.
          A run of the Principal is ended at once by interrupting it
          (Ctrl-C) where it was started.
`)
}

func cmdStop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageStop
	now := fs.Bool("now", false, "")
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
	run, found := engine.ReadRun(".")
	return stopRun(os.Stdout, ".", stopAsk{
		running: report.Run.State == "running",
		engine:  found && run.Alive(time.Now()),
		now:     *now,
		wait:    10 * time.Second,
	})
}

// stopAsk is what `awkit stop` was asked, and what it found.
type stopAsk struct {
	running bool // a run is going
	engine  bool // it is the engine's
	now     bool // --now
	wait    time.Duration
}

// stopRun asks the run in the project at root to stop, when one is going.
// Without a run there is nothing to stop and nothing is written: a marker
// left behind would only be cleared by the next run. The engine is asked
// through its inbox as well, and says what it did with the stop.
func stopRun(w io.Writer, root string, ask stopAsk) int {
	if !ask.running {
		fmt.Fprintln(w, "No run is going in this project.")
		return 0
	}
	if ask.now {
		if !ask.engine {
			errorf("this run is the Principal's: interrupt it (Ctrl-C) where it was started to end it at once, or `awkit stop` to end it when its step is done\n")
			return 1
		}
		return sendStop(w, root, engine.CommandStopNow, ask.wait)
	}
	if err := stopmark.Write(root, "awkit stop", stopmark.ByUser); err != nil {
		errorf("could not write %s: %v\n", stopmark.File, err)
		return 1
	}
	if ask.engine {
		return sendStop(w, root, engine.CommandStop, ask.wait)
	}
	fmt.Fprintln(w, "Asked the run to stop. It ends when the step it is on is done: a Worker or a review that is running finishes first.")
	fmt.Fprintln(w, "awkit status shows where it is. awkit run goes on from there later.")
	return 0
}

// sendStop gives the engine a stop, and says what it answered, or that it
// has not answered yet.
func sendStop(w io.Writer, root, kind string, wait time.Duration) int {
	by := "awkit stop"
	if kind == engine.CommandStopNow {
		by = "awkit stop --now"
	}
	c, err := engine.Send(root, engine.Command{Kind: kind, By: by})
	if err != nil {
		errorf("could not give the engine the stop: %v\n", err)
		return 1
	}
	deadline := time.Now().Add(wait)
	for {
		if r, ok := engine.ReplyOf(root, c.ID); ok {
			if r.Result != engine.Applied {
				errorf("the engine did not take the stop: %s\n", r.Reason)
				return 1
			}
			fmt.Fprintf(w, "The engine took the stop: %s.\n", r.Reason)
			fmt.Fprintln(w, "awkit status shows where it is. awkit run goes on from there later.")
			return 0
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Fprintf(w, "Gave the engine the stop (%s); it has not answered yet. awkit status shows when it has stopped.\n", c.ID)
	return 0
}
