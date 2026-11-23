package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// codexAdapter runs the Codex CLI: one `codex exec` that asks nothing
// (-a never), keeps no session, reads neither the person's configuration nor
// their command rules, in the sandbox of the role's level, with its network
// off (workspace-write keeps it off), and its events as JSON lines.
//
// Codex has no hook: the commands of a role with a shell are held by its
// sandbox, and from P3 by the container and its proxy. That the command
// policy does not apply is recorded with every run.
type codexAdapter struct{}

var codexProgram = "codex"

// goos and codexHome are where the run is: variables for tests.
var (
	goos      = runtime.GOOS
	codexHome = func() string {
		if home := os.Getenv("CODEX_HOME"); home != "" {
			return home
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".codex")
	}
)

var sandboxMode = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// codexWindowsSandbox reads the Windows sandbox mode from the person's Codex
// configuration ([windows] sandbox, or windows.sandbox): "" when it names
// none. A mode that is not a plain word is refused rather than passed on.
func codexWindowsSandbox() (mode, path string, err error) {
	path = filepath.Join(codexHome(), "config.toml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", path, nil
	}
	if err != nil {
		return "", path, err
	}
	mode = tomlValue(string(data), "windows", "sandbox")
	if mode != "" && !sandboxMode.MatchString(mode) {
		return "", path, fmt.Errorf("the Windows sandbox mode in %s is %q, which is not one codex names", path, mode)
	}
	return mode, path, nil
}

// tomlValue finds a string key of a table in a TOML text: under [table], or
// as table.key at the top. It reads only what that takes.
func tomlValue(text, table, key string) string {
	current := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			// [table] or [[array]], with maybe a comment after it.
			current = ""
			if end := strings.IndexByte(line, ']'); end > 0 {
				current = strings.TrimSpace(strings.TrimLeft(line[:end], "["))
			}
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if (current == table && name == key) || (current == "" && name == table+"."+key) {
			return tomlString(strings.TrimSpace(value))
		}
	}
	return ""
}

// tomlString reads a TOML string value, quoted "…" or '…', with a comment
// after it.
func tomlString(v string) string {
	if len(v) < 2 {
		return ""
	}
	quote := v[0]
	if quote != '"' && quote != '\'' {
		return ""
	}
	end := strings.IndexByte(v[1:], quote)
	if end < 0 {
		return ""
	}
	return v[1 : end+1]
}

var (
	codexHelpOnce sync.Once
	codexHelp     string
)

// codexHelpText is what `codex --help` and `codex exec --help` say, asked
// once per process: what this codex supports. A variable for tests.
var codexHelpText = func() string {
	codexHelpOnce.Do(func() {
		ask := func(args ...string) string {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			out, _ := exec.CommandContext(ctx, codexProgram, args...).CombinedOutput()
			return string(out)
		}
		codexHelp = ask("--help") + "\n" + ask("exec", "--help")
	})
	return codexHelp
}

func codexSupports(option string) bool {
	return strings.Contains(codexHelpText(), option)
}

func (codexAdapter) prepare(env Envelope, files runFiles) (invocation, error) {
	p := env.Profile
	var missing, unenforced []string
	for _, option := range []string{"--ask-for-approval", "--sandbox", "--ignore-user-config"} {
		if !codexSupports(option) {
			missing = append(missing, option)
		}
	}
	if len(missing) > 0 {
		return invocation{}, fmt.Errorf("this codex cannot be held to the %s's settings (it lacks %s): update Codex", p.Role, strings.Join(missing, ", "))
	}

	sandbox := "read-only"
	if p.Tools == EditTools || p.Tools == ShellTools {
		sandbox = "workspace-write"
	}
	// On Windows codex runs a command only in its Windows sandbox, which a
	// person sets up once and keeps in their Codex configuration. The run
	// reads none of that configuration but this: the machine's sandbox.
	var windowsSandbox string
	if goos == "windows" && p.Tools != NoTools {
		mode, path, err := codexWindowsSandbox()
		if err != nil {
			return invocation{}, err
		}
		if mode == "" {
			return invocation{}, fmt.Errorf("codex on Windows runs commands only in its Windows sandbox, and none is set up in %s: run codex once in a project and let it set the sandbox up, or give the %s another backend", path, p.Role)
		}
		windowsSandbox = mode
	}
	args := []string{"-a", "never", "exec"}
	for _, option := range []string{"--ephemeral", "--ignore-rules", "--skip-git-repo-check"} {
		if codexSupports(option) {
			args = append(args, option)
		}
	}
	args = append(args, "--ignore-user-config", "--sandbox", sandbox)
	if windowsSandbox != "" {
		args = append(args, "-c", `windows.sandbox="`+windowsSandbox+`"`)
	}
	if codexSupports("--json") {
		args = append(args, "--json")
	}
	if p.Model != "" {
		args = append(args, "-m", p.Model)
	}
	if p.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+p.Effort)
	}
	if codexSupports("--output-last-message") {
		args = append(args, "-o", files.path("last-message.txt"))
	}
	prompt := env.Prompt
	if env.Schema != nil {
		if codexSupports("--output-schema") {
			path := files.path("schema.json")
			if err := os.WriteFile(path, compact(env.Schema), 0o644); err != nil {
				return invocation{}, err
			}
			args = append(args, "--output-schema", path)
		} else {
			unenforced = append(unenforced, "the answer's schema in the CLI (no --output-schema): checked by the runtime")
			prompt += "\n\nAnswer with only a JSON value that matches this JSON Schema:\n" + string(env.Schema)
		}
	}
	if p.Tools != NoTools {
		unenforced = append(unenforced, "the command policy (codex has no hook): its "+sandbox+" sandbox holds the commands")
	}
	if p.Tools == ReadTools {
		unenforced = append(unenforced, "no commands at all: a read-only codex may still run commands that read")
	}
	if p.USD > 0 {
		unenforced = append(unenforced, "the spend limit (codex reports no cost): only the time limit holds")
	}
	// The prompt comes on standard input ("-").
	args = append(args, "-")
	return invocation{program: codexProgram, args: args, stdin: prompt, unenforced: unenforced}, nil
}

// codexEvent is the part of a `codex exec --json` event that is read.
type codexEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
	Item *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
	Usage *struct {
		InputTokens     int `json:"input_tokens"`
		CachedTokens    int `json:"cached_input_tokens"`
		OutputTokens    int `json:"output_tokens"`
		ReasoningTokens int `json:"reasoning_output_tokens"`
	} `json:"usage"`
}

func (codexAdapter) parse(stdout, stderr []byte, files runFiles) (answer, error) {
	_ = os.WriteFile(files.path("events.jsonl"), stdout, 0o644)
	var a answer
	var said []string
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	scanner.Buffer(make([]byte, 0, 64*1024), 16<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var e codexEvent
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		switch {
		case e.Type == "turn.completed" && e.Usage != nil:
			a.usage.Turns++
			a.usage.InputTokens += e.Usage.InputTokens - e.Usage.CachedTokens
			a.usage.CachedTokens += e.Usage.CachedTokens
			a.usage.OutputTokens += e.Usage.OutputTokens + e.Usage.ReasoningTokens
		case e.Type == "item.completed" && e.Item != nil && e.Item.Type == "agent_message":
			a.text = e.Item.Text
		case strings.HasSuffix(e.Type, "failed") || e.Type == "error":
			msg := e.Message
			if e.Error != nil && e.Error.Message != "" {
				msg = e.Error.Message
			}
			said = append(said, strings.TrimSpace(e.Type+": "+msg))
		}
	}
	if last, err := os.ReadFile(files.path("last-message.txt")); err == nil && len(bytes.TrimSpace(last)) > 0 {
		a.text = strings.TrimSpace(string(last))
	}
	if len(said) > 0 {
		a.failed = "codex ended with " + strings.Join(said, "; ")
	}
	if a.text == "" && a.failed == "" && a.usage.Turns == 0 {
		return a, fmt.Errorf("codex did not answer: %s", head(stdout, stderr))
	}
	return a, nil
}
