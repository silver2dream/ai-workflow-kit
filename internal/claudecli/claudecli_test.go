package claudecli

import (
	"reflect"
	"strings"
	"sync"
	"testing"
)

// A trimmed copy of the option list `claude --help` prints.
const helpWithIsolation = `Usage: claude [options] [command] [prompt]

Options:
  --disallowedTools, --disallowed-tools <tools...>
                                        Comma or space-separated list of tool names to deny
  --effort <level>                      Effort level for the current session
                                        (low, medium, high, xhigh, max)
  --model <model>                       Model for the current session.
  -p, --print                           Print response and exit
  --safe-mode                           Start with all customizations
                                        (CLAUDE.md, skills, installed plugins,
                                        hooks, MCP servers) disabled
  --tools <tools...>                    Specify the list of available tools from
                                        the built-in set. Use "" to disable all
`

// An older CLI: neither flag exists, though --tools is mentioned in passing.
const helpWithoutIsolation = `Usage: claude [options] [command] [prompt]

Options:
  --allowedTools, --allowed-tools <tools...>
                                        Tools that run without a prompt; unlike --tools
                                        this does not restrict the tool list
  --model <model>                       Model for the current session.
  -p, --print                           Print response and exit
`

func TestIsolationFlags(t *testing.T) {
	tests := []struct {
		name string
		help string
		want []string
	}{
		{"current CLI", helpWithIsolation, []string{"--safe-mode", "--tools", ""}},
		{"older CLI", helpWithoutIsolation, nil},
		{"help unavailable", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isolationFlags(tt.help); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("isolationFlags() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasOption(t *testing.T) {
	if !hasOption(helpWithIsolation, "--disallowed-tools") {
		t.Error("an alias in the option column should count")
	}
	if !hasOption(helpWithIsolation, "--print") {
		t.Error("the long form after a short alias should count")
	}
	if hasOption(helpWithoutIsolation, "--tools") {
		t.Error("an option only mentioned in a description must not count")
	}
}

func stubHelp(t *testing.T, help string) {
	t.Helper()
	orig := helpTextFunc
	helpTextFunc = func() string { return help }
	t.Cleanup(func() { helpTextFunc = orig })
}

func TestOneShotArgs(t *testing.T) {
	t.Setenv(IsolationEnv, "")

	stubHelp(t, helpWithIsolation)
	want := []string{"--print", "--model", "sonnet", "--max-turns", "1", "--safe-mode", "--tools", ""}
	if got := OneShotArgs("sonnet"); !reflect.DeepEqual(got, want) {
		t.Errorf("OneShotArgs() = %q, want %q", got, want)
	}

	stubHelp(t, helpWithoutIsolation)
	want = []string{"--print", "--model", "opus", "--max-turns", "1"}
	if got := OneShotArgs("opus"); !reflect.DeepEqual(got, want) {
		t.Errorf("OneShotArgs() on an older CLI = %q, want %q", got, want)
	}
}

func TestValidEffort(t *testing.T) {
	for _, effort := range []string{"", "low", "medium", "high", "xhigh", "max"} {
		if !ValidEffort(effort) {
			t.Errorf("ValidEffort(%q) = false, want true", effort)
		}
	}
	for _, effort := range []string{"hgih", "High", "ultra", " high", "none"} {
		if ValidEffort(effort) {
			t.Errorf("ValidEffort(%q) = true, want false", effort)
		}
	}
}

// heard collects what the package warns about.
func heard(t *testing.T) *[]string {
	t.Helper()
	var said []string
	orig := warn
	warn = func(message string) { said = append(said, message) }
	t.Cleanup(func() { warn = orig; effortWarning = sync.Once{} })
	effortWarning = sync.Once{}
	return &said
}

func TestEffortArgs(t *testing.T) {
	said := heard(t)
	stubHelp(t, helpWithIsolation)
	if got := EffortArgs(""); got != nil {
		t.Errorf("EffortArgs(\"\") = %q, want nothing: the CLI's own default", got)
	}
	if got, want := EffortArgs("xhigh"), []string{"--effort", "xhigh"}; !reflect.DeepEqual(got, want) {
		t.Errorf("EffortArgs(xhigh) = %q, want %q", got, want)
	}
	if len(*said) != 0 {
		t.Errorf("warned %q about efforts the CLI takes", *said)
	}

	// A level the CLI does not know is not passed on, and is said.
	if got := EffortArgs("hgih"); got != nil {
		t.Errorf("EffortArgs(hgih) = %q, want nothing", got)
	}
	if len(*said) != 1 || !strings.Contains((*said)[0], `"hgih"`) {
		t.Errorf("warned %q, want one warning naming the level", *said)
	}
}

// An older CLI has no --effort: the call runs without it, and that is said once.
func TestEffortArgs_OlderCLI(t *testing.T) {
	said := heard(t)
	stubHelp(t, helpWithoutIsolation)
	for range 3 {
		if got := EffortArgs("high"); got != nil {
			t.Errorf("EffortArgs(high) on an older CLI = %q, want nothing", got)
		}
	}
	if len(*said) != 1 || !strings.Contains((*said)[0], "--effort") {
		t.Errorf("warned %q, want one warning that the CLI takes no --effort", *said)
	}
	if got := EffortArgs(""); got != nil || len(*said) != 1 {
		t.Errorf("an empty effort asked for %q and warned %q; it asks for nothing", got, *said)
	}
}

func TestOneShotArgs_IsolationSwitchedOff(t *testing.T) {
	stubHelp(t, helpWithIsolation)
	for _, value := range []string{"0", "off", "false", "OFF"} {
		t.Setenv(IsolationEnv, value)
		want := []string{"--print", "--model", "sonnet", "--max-turns", "1"}
		if got := OneShotArgs("sonnet"); !reflect.DeepEqual(got, want) {
			t.Errorf("%s=%s: OneShotArgs() = %q, want %q", IsolationEnv, value, got, want)
		}
	}
}
