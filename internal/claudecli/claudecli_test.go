package claudecli

import (
	"reflect"
	"testing"
)

// A trimmed copy of the option list `claude --help` prints.
const helpWithIsolation = `Usage: claude [options] [command] [prompt]

Options:
  --disallowedTools, --disallowed-tools <tools...>
                                        Comma or space-separated list of tool names to deny
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
