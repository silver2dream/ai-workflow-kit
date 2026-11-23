package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
)

func usageGuard() {
	fmt.Fprint(os.Stderr, `Check a tool call an agent is about to make (a Claude Code PreToolUse hook)

Usage:
  awkit guard tool --role <role> [--protect <branch>]... [--allow-control] [--state-root <dir>]

The call comes on standard input, as Claude Code gives it to a PreToolUse
hook. A call the role's policy refuses ends with exit status 2 and the reason
on standard error, which Claude Code gives the agent; the verdict is written
to the project's events. A call that cannot be read is refused.

Roles:
  principal     the session of awkit run, with its reviewer and conflict resolver
  implementer   a Worker

Options:
  --protect <branch>   A branch nothing is pushed to (repeatable); main,
                       master, develop and trunk always are
  --allow-control      The Worker's ticket allows changes to .claude and .ai
  --state-root <dir>   The project, where verdicts are recorded (default: the
                       repository of the call's directory)

awkit writes this hook into the settings it gives each agent; it is not run by hand.
`)
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// hookInput is what Claude Code gives a PreToolUse hook on standard input.
type hookInput struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	Cwd       string         `json:"cwd"`
}

func cmdGuard(args []string) int {
	if len(args) == 0 || args[0] != "tool" {
		usageGuard()
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			return 0
		}
		return 2
	}
	fs := flag.NewFlagSet("guard tool", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageGuard
	role := fs.String("role", "", "")
	stateRoot := fs.String("state-root", "", "")
	allowControl := fs.Bool("allow-control", false, "")
	var protect stringList
	fs.Var(&protect, "protect", "")
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	refuse := func(reason string) int {
		fmt.Fprintln(os.Stderr, "AWK guard: "+reason)
		return 2
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	if err != nil {
		return refuse("the call cannot be read: " + err.Error())
	}
	var in hookInput
	if err := json.Unmarshal(data, &in); err != nil || in.ToolName == "" {
		return refuse("the call cannot be read, so it is not allowed")
	}
	policy := guard.Policy{Role: *role, Protected: protect, AllowControl: *allowControl}
	verdict := policy.Check(guard.ToolCall{Tool: in.ToolName, Input: in.ToolInput, Cwd: in.Cwd})
	if verdict.Result != guard.Block {
		return 0
	}
	root := *stateRoot
	if root == "" {
		root = guard.ProjectRoot(in.Cwd)
	}
	guard.Record(root, []guard.Verdict{verdict}, guard.IssueOfDir(in.Cwd))
	return refuse(verdict.Reason + " (" + verdict.Rule + "). If the task needs it, say so in your answer instead.")
}
