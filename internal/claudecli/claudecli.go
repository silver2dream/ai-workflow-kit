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
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

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
