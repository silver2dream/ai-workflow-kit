package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/engine"
)

func usageLane() {
	fmt.Fprint(os.Stderr, `Tell the engine what to do with one ticket

Usage:
  awkit lane approve <issue>
  awkit lane reject <issue> [--note "what to change"]

Commands:
  approve   Approve the high-risk change of a ticket that waits for you: it
            goes to the merge queue.
  reject    Return it to its implementer. The note, when given, goes to the
            next Worker as your word.

The engine takes the command at once when it runs; otherwise when the next
"awkit run" starts. "awkit status" shows the tickets that wait for you, and
why.
`)
}

func cmdLane(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usageLane()
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	kind := args[0]
	switch kind {
	case "approve":
		kind = engine.CommandApprove
	case "reject":
		kind = engine.CommandReject
	default:
		errorf("awkit lane: unknown command %q (approve or reject)\n", args[0])
		return 2
	}
	fs := flag.NewFlagSet("lane "+args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageLane
	note := fs.String("note", "", "")
	// The issue may come before the flags, or after them.
	rest := args[1:]
	var positional []string
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		rest = fs.Args()
		if len(rest) > 0 {
			positional = append(positional, rest[0])
			rest = rest[1:]
		}
	}
	if len(positional) != 1 {
		errorf("awkit lane %s: name one ticket by its issue number\n", args[0])
		return 2
	}
	issue, err := strconv.Atoi(positional[0])
	if err != nil || issue < 1 {
		errorf("awkit lane %s: %q is no issue number\n", args[0], positional[0])
		return 2
	}
	if *note != "" && kind != engine.CommandReject {
		errorf("awkit lane approve: --note goes with reject\n")
		return 2
	}
	run, found := engine.ReadRun(".")
	return sendLane(os.Stdout, ".", engine.Command{Kind: kind, Issue: issue, By: "awkit lane", Args: notes(*note)}, found && run.Alive(time.Now()), 10*time.Second)
}

func notes(note string) map[string]string {
	if note == "" {
		return nil
	}
	return map[string]string{"note": note}
}

// sendLane gives the engine a command of one ticket, and says what it
// answered; when no run goes, that the next run takes it.
func sendLane(w io.Writer, root string, c engine.Command, going bool, wait time.Duration) int {
	if !going {
		sent, err := engine.Send(root, c)
		if err != nil {
			errorf("could not give the engine the command: %v\n", err)
			return 1
		}
		fmt.Fprintf(w, "No run is going: the engine takes it (%s) when the next one starts. awkit run starts one.\n", sent.ID)
		return 0
	}
	sent, r, answered, err := engine.Ask(root, c, wait)
	if err != nil {
		errorf("could not give the engine the command: %v\n", err)
		return 1
	}
	if !answered {
		fmt.Fprintf(w, "Gave the engine the command (%s); it has not answered yet. awkit status shows the ticket.\n", sent.ID)
		return 0
	}
	if r.Result != engine.Applied {
		errorf("the engine did not take it: %s\n", r.Reason)
		return 1
	}
	fmt.Fprintf(w, "#%d: %s.\n", c.Issue, r.Reason)
	return 0
}
