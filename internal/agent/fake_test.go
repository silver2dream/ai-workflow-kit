package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The test binary is also the fake CLI: run with GO_AGENT_FAKE set, it
// answers as the claude or codex CLI would, from a script, and keeps what it
// was asked. (GO is a prefix the agent's environment keeps.)

// fakeReply is one answer of the fake CLI.
type fakeReply struct {
	Stdout string `json:"stdout"`
	// LastMessage is written to the file codex was given with -o.
	LastMessage string `json:"last_message"`
	Exit        int    `json:"exit"`
	SleepMS     int    `json:"sleep_ms"`
}

// fakeCall is what the fake CLI was asked.
type fakeCall struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin"`
	Dir   string   `json:"dir"`
	Env   []string `json:"env"`
}

func TestMain(m *testing.M) {
	if mode := os.Getenv("GO_AGENT_FAKE"); mode != "" {
		os.Exit(fakeCLI(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeCLI(args []string) int {
	dir := os.Getenv("GO_AGENT_DIR")
	stdin, _ := io.ReadAll(os.Stdin)
	cwd, _ := os.Getwd()
	var env []string
	for _, kv := range os.Environ() {
		env = append(env, strings.SplitN(kv, "=", 2)[0])
	}
	// Which call of the run this is.
	n := 0
	for {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("call-%d.json", n))); os.IsNotExist(err) {
			break
		}
		n++
	}
	data, _ := json.Marshal(fakeCall{Args: args, Stdin: string(stdin), Dir: cwd, Env: env})
	_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("call-%d.json", n)), data, 0o644)

	var replies []fakeReply
	script, _ := os.ReadFile(filepath.Join(dir, "script.json"))
	_ = json.Unmarshal(script, &replies)
	if len(replies) == 0 {
		return 3
	}
	r := replies[len(replies)-1]
	if n < len(replies) {
		r = replies[n]
	}
	if r.SleepMS > 0 {
		time.Sleep(time.Duration(r.SleepMS) * time.Millisecond)
	}
	if r.LastMessage != "" {
		for i, a := range args {
			if a == "-o" && i+1 < len(args) {
				_ = os.WriteFile(args[i+1], []byte(r.LastMessage), 0o644)
			}
		}
	}
	fmt.Print(r.Stdout)
	return r.Exit
}

// fake sets up a runner whose backends are the fake CLI, answering with
// replies in order, and returns it with the folder the calls are kept in.
func fake(t *testing.T, replies ...fakeReply) (*Runner, string) {
	t.Helper()
	dir := t.TempDir()
	data, _ := json.Marshal(replies)
	if err := os.WriteFile(filepath.Join(dir, "script.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	origClaude, origCodex := claudeProgram, codexProgram
	origSupports, origMentions, origAtLeast, origExe, origHelp := claudeSupports, claudeMentions, claudeAtLeast, awkitExecutable, codexHelpText
	origGOOS, origHome := goos, codexHome
	codexDir := t.TempDir()
	goos, codexHome = "linux", func() string { return codexDir }
	claudeProgram, codexProgram = self, self
	claudeSupports = func(string) bool { return true }
	claudeMentions = func(string) bool { return true }
	claudeAtLeast = func(int, int, int) bool { return true }
	awkitExecutable = func() (string, error) { return filepath.FromSlash("C:/tools/awkit.exe"), nil }
	codexHelpText = func() string {
		return "--ask-for-approval --sandbox --ignore-user-config --ephemeral --ignore-rules --skip-git-repo-check --json --output-last-message --output-schema"
	}
	t.Cleanup(func() {
		claudeProgram, codexProgram = origClaude, origCodex
		claudeSupports, claudeMentions, claudeAtLeast, awkitExecutable, codexHelpText = origSupports, origMentions, origAtLeast, origExe, origHelp
		goos, codexHome = origGOOS, origHome
	})
	r := NewRunner()
	// A "minute" long enough for a slow machine; the test of the time
	// limit sets its own.
	r.minute = 10 * time.Second
	r.environ = func() []string {
		return append(os.Environ(),
			"GO_AGENT_FAKE=1", "GO_AGENT_DIR="+dir,
			"GITHUB_TOKEN=not-for-agents", "ANTHROPIC_API_KEY=the-backends-own", "OPENAI_API_KEY=codex-own",
		)
	}
	return r, dir
}

// calls are what the fake CLI was asked, in order.
func calls(t *testing.T, dir string) []fakeCall {
	t.Helper()
	var out []fakeCall
	for n := 0; ; n++ {
		data, err := os.ReadFile(filepath.Join(dir, "call-"+strconv.Itoa(n)+".json"))
		if err != nil {
			return out
		}
		var c fakeCall
		if err := json.Unmarshal(data, &c); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
}

// claudeJSON is a claude --print --output-format json result.
func claudeJSON(fields map[string]any) string {
	base := map[string]any{"type": "result", "subtype": "success", "is_error": false, "num_turns": 2, "total_cost_usd": 0.0012, "result": "",
		"usage": map[string]any{"input_tokens": 100, "output_tokens": 20, "cache_read_input_tokens": 50, "cache_creation_input_tokens": 10}, "permission_denials": []any{}}
	for k, v := range fields {
		base[k] = v
	}
	data, _ := json.Marshal(base)
	return string(data)
}
