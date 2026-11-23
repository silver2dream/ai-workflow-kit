package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/claudecli"
)

// claudeAdapter runs the Claude Code CLI: one `claude --print` with the
// settings the runtime writes for the run and no others (not the person's,
// not the project's), the tools of the role's level and no more, every tool
// call held to the command policy by a PreToolUse hook, nothing asked, no
// session kept, and the answer as JSON.
type claudeAdapter struct{}

// What is asked of the claude CLI and of this awkit: variables for tests.
var (
	claudeProgram   = "claude"
	claudeSupports  = claudecli.Supports
	claudeMentions  = claudecli.Mentions
	claudeAtLeast   = claudecli.AtLeast
	awkitExecutable = os.Executable
)

// claudeTools are the tools of each level. A name this CLI does not have is
// passed over by it.
var claudeTools = map[Tools][]string{
	NoTools:    {},
	ReadTools:  {"Read", "Grep", "Glob", "LS"},
	EditTools:  {"Read", "Grep", "Glob", "LS", "Edit", "Write", "MultiEdit", "NotebookEdit", "TodoWrite"},
	ShellTools: {"Read", "Grep", "Glob", "LS", "Edit", "Write", "MultiEdit", "NotebookEdit", "TodoWrite", "Bash", "PowerShell", "BashOutput", "KillShell"},
}

// claudeDenied are refused by the CLI itself, before the hook.
var claudeDenied = map[Tools][]string{
	ShellTools: {"WebFetch", "WebSearch", "Bash(gh:*)", "Bash(git push:*)", "Bash(curl:*)", "Bash(wget:*)"},
}

// hookTimeoutSeconds bounds one check of the command policy.
const hookTimeoutSeconds = 30

func (claudeAdapter) prepare(env Envelope, files runFiles) (invocation, error) {
	p := env.Profile
	var missing, unenforced []string
	for _, option := range []string{"--output-format", "--settings", "--setting-sources", "--tools"} {
		if !claudeSupports(option) {
			missing = append(missing, option)
		}
	}
	if !claudeMentions("dontAsk") {
		missing = append(missing, "the dontAsk permission mode")
	}
	if len(missing) > 0 {
		return invocation{}, fmt.Errorf("this claude CLI cannot be held to the %s's settings (it lacks %s): update Claude Code", p.Role, strings.Join(missing, ", "))
	}

	tools := claudeTools[p.Tools]
	settings := map[string]any{
		"permissions": map[string]any{"allow": tools, "deny": append([]string{"WebFetch", "WebSearch"}, claudeDenied[p.Tools]...)},
	}
	if p.Tools != NoTools {
		hook, err := hookCommand(env)
		if err != nil {
			return invocation{}, err
		}
		entry := map[string]any{"type": "command", "command": hook, "timeout": hookTimeoutSeconds}
		if claudeAtLeast(2, 1, 295) {
			entry["onFailure"] = "block"
		} else {
			unenforced = append(unenforced, "a command-policy check that fails or times out lets the call through (Claude Code before 2.1.295)")
		}
		settings["hooks"] = map[string]any{"PreToolUse": []any{map[string]any{"matcher": "*", "hooks": []any{entry}}}}
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return invocation{}, err
	}
	settingsFile := files.path("claude-settings.json")
	if err := os.WriteFile(settingsFile, data, 0o644); err != nil {
		return invocation{}, err
	}

	args := []string{"--print", "--output-format", "json"}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.Effort != "" {
		if claudeSupports("--effort") {
			args = append(args, "--effort", p.Effort)
		} else {
			unenforced = append(unenforced, "effort "+p.Effort+" (no --effort)")
		}
	}
	args = append(args, "--settings", settingsFile, "--setting-sources", "")
	if claudeSupports("--strict-mcp-config") {
		args = append(args, "--strict-mcp-config")
	} else {
		unenforced = append(unenforced, "MCP servers outside the run's settings (no --strict-mcp-config)")
	}
	args = append(args, "--permission-mode", "dontAsk", "--tools", strings.Join(tools, ","))
	if claudeSupports("--no-session-persistence") {
		args = append(args, "--no-session-persistence")
	}
	if p.USD > 0 {
		if claudeSupports("--max-budget-usd") {
			args = append(args, "--max-budget-usd", strconv.FormatFloat(p.USD, 'f', -1, 64))
		} else {
			unenforced = append(unenforced, "the spend limit while it runs (no --max-budget-usd): checked after the run")
		}
	}
	prompt := env.Prompt
	if env.Schema != nil {
		if claudeSupports("--json-schema") {
			args = append(args, "--json-schema", string(compact(env.Schema)))
		} else {
			unenforced = append(unenforced, "the answer's schema in the CLI (no --json-schema): checked by the runtime")
			prompt += "\n\nAnswer with only a JSON value that matches this JSON Schema:\n" + string(env.Schema)
		}
	}
	return invocation{program: claudeProgram, args: args, stdin: prompt, settings: data, unenforced: unenforced}, nil
}

// hookCommand is the command line of the command-policy hook: this awkit's
// `guard tool`, with the role's policy, the project, and its branches.
func hookCommand(env Envelope) (string, error) {
	exe := env.Awkit
	if exe == "" {
		self, err := awkitExecutable()
		if err != nil {
			return "", fmt.Errorf("awkit cannot find itself for the command-policy hook: %w", err)
		}
		exe = self
	}
	quote := func(s string) string { return `"` + filepath.ToSlash(s) + `"` }
	parts := []string{quote(exe), "guard", "tool", "--role", env.Profile.guardRole()}
	if env.StateRoot != "" {
		parts = append(parts, "--state-root", quote(env.StateRoot))
	}
	for _, branch := range env.Protected {
		if branch != "" {
			parts = append(parts, "--protect", quote(branch))
		}
	}
	if env.AllowControl {
		parts = append(parts, "--allow-control")
	}
	return strings.Join(parts, " "), nil
}

// claudeResult is the part of `claude --print --output-format json` that is
// read.
type claudeResult struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	NumTurns         int             `json:"num_turns"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	Usage            struct {
		InputTokens         int `json:"input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		CacheReadTokens     int `json:"cache_read_input_tokens"`
		CacheCreationTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	PermissionDenials []json.RawMessage `json:"permission_denials"`
}

func (claudeAdapter) parse(stdout, stderr []byte, files runFiles) (answer, error) {
	_ = os.WriteFile(files.path("answer.json"), stdout, 0o644)
	var r claudeResult
	if err := json.Unmarshal(stdout, &r); err != nil || r.Type != "result" {
		return answer{}, fmt.Errorf("the claude CLI did not answer with its JSON result: %s", head(stdout, stderr))
	}
	a := answer{
		text: r.Result,
		usage: Usage{
			Turns: r.NumTurns, InputTokens: r.Usage.InputTokens + r.Usage.CacheCreationTokens,
			CachedTokens: r.Usage.CacheReadTokens, OutputTokens: r.Usage.OutputTokens, CostUSD: r.TotalCostUSD,
		},
		denials: len(r.PermissionDenials),
	}
	if len(r.StructuredOutput) > 0 && string(r.StructuredOutput) != "null" {
		a.output = r.StructuredOutput
	}
	switch {
	case strings.Contains(r.Subtype, "budget"):
		a.budget = true
	case r.IsError || (r.Subtype != "" && r.Subtype != "success"):
		a.failed = fmt.Sprintf("the claude CLI ended with %s: %s", r.Subtype, strings.TrimSpace(r.Result))
	}
	return a, nil
}

// head is the start of what a CLI said, for an error about it.
func head(stdout, stderr []byte) string {
	said := strings.TrimSpace(string(stdout))
	if said == "" {
		said = strings.TrimSpace(string(stderr))
	}
	if r := []rune(said); len(r) > 300 {
		said = string(r[:300]) + "…"
	}
	if said == "" {
		return "nothing"
	}
	return said
}
