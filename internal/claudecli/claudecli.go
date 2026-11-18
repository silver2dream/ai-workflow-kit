// Package claudecli builds the arguments for the single-turn, text-only
// `claude --print` calls AWK makes outside the Principal and Worker sessions:
// the lessons distiller, the secondary reviewers and the JiT test generator.
//
// Without extra flags such a call starts like an interactive session in the
// project directory: it loads the project's CLAUDE.md (which tells the model
// it is the Principal Engineer), its skills, subagents, hooks and MCP servers,
// and the user's own ~/.claude configuration. None of that belongs in a call
// whose whole job is to turn one prompt into one text answer, and a tool call
// would spend the only turn the call has.
package claudecli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

// Efforts are the levels the claude CLI takes with --effort: how long and how
// carefully the model thinks before it answers.
var Efforts = []string{"low", "medium", "high", "xhigh", "max"}

// EffortHint says what an effort may be, for an error about one that may not.
const EffortHint = "low, medium, high, xhigh or max, or no value for the claude CLI's own default"

// ValidEffort reports whether an effort can be asked for: one of Efforts, or
// empty for the CLI's own default. The CLI takes a level it does not know
// with a warning and goes on with its default, so a misspelt setting would
// otherwise change nothing without anyone noticing.
func ValidEffort(effort string) bool {
	return effort == "" || slices.Contains(Efforts, effort)
}

var effortWarning sync.Once

// warn tells the person running AWK something it did not do. A variable so
// tests can hear it.
var warn = func(message string) { fmt.Fprintln(os.Stderr, "warning: "+message) }

// EffortArgs returns the flag that asks for an effort: nothing for an empty
// effort. A CLI too old to take the flag gets nothing either, so the call
// still runs; that the effort was not used is said once.
func EffortArgs(effort string) []string {
	if effort == "" {
		return nil
	}
	if !ValidEffort(effort) {
		warn(fmt.Sprintf("effort %q is not one the claude CLI takes (%s); it is not used", effort, EffortHint))
		return nil
	}
	if !hasOption(helpTextFunc(), "--effort") {
		effortWarning.Do(func() {
			warn(fmt.Sprintf("the claude CLI on this machine takes no --effort; effort %q is not used (update Claude Code)", effort))
		})
		return nil
	}
	return []string{"--effort", effort}
}

// IsolationEnv switches the isolation flags off when set to "0", "off" or
// "false" - an escape hatch for an environment where they misbehave.
const IsolationEnv = "AWKIT_ONESHOT_ISOLATION"

var (
	helpOnce sync.Once
	helpText string
)

// helpTextFunc returns the output of `claude --help`, fetched once per
// process. It is a variable so tests can supply a help text.
var helpTextFunc = func() string {
	helpOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "claude", "--help").CombinedOutput()
		if err == nil {
			helpText = string(out)
		}
	})
	return helpText
}

// OneShotArgs returns the arguments for a single-turn `claude --print` call on
// the given model. The prompt goes in on stdin.
//
// Isolation flags are added only when the installed CLI lists them in its
// help, so an older CLI keeps working exactly as before.
func OneShotArgs(model string) []string {
	args := []string{"--print", "--model", model, "--max-turns", "1"}
	if isolationDisabled() {
		return args
	}
	return append(args, isolationFlags(helpTextFunc())...)
}

// isolationFlags picks, from the CLI's help text, the flags that keep a
// one-shot call to its prompt:
//
//	--safe-mode  load no CLAUDE.md, skills, subagents, hooks, plugins or MCP
//	             servers; authentication and model selection work as usual
//	--tools ""   no built-in tools, so the single turn is always a text answer
func isolationFlags(help string) []string {
	var flags []string
	if hasOption(help, "--safe-mode") {
		flags = append(flags, "--safe-mode")
	}
	if hasOption(help, "--tools") {
		flags = append(flags, "--tools", "")
	}
	return flags
}

// hasOption reports whether the help text declares the option. Only the
// option column is read - the leading tokens of a line that start with "-" -
// so an option merely mentioned in another option's description does not count.
func hasOption(help, option string) bool {
	for _, line := range strings.Split(help, "\n") {
		for _, field := range strings.Fields(line) {
			if !strings.HasPrefix(field, "-") {
				break
			}
			if strings.TrimRight(field, ",") == option {
				return true
			}
		}
	}
	return false
}

func isolationDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(IsolationEnv))) {
	case "0", "off", "false", "no":
		return true
	}
	return false
}
